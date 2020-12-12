package gsexp

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/guregu/dynamo"
	zlog "github.com/rs/zerolog/log"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/sheets/v4"
)

var (
	region                       string
	googleClientSecret           []byte
	googleDriveAccessToken       string
	googleSpreadsheetAccessToken string
	exportRootDir                string
	exportTable                  string
)

func init() {

	exportTable = os.Getenv("DYNAMO_TABLE_EXPORT")
	if exportTable == "" {
		zlog.Fatal().Msgf("unset env variable: DYNAMO_TABLE_EXPORT")
	}

	exportRootDir = os.Getenv("GOOGLE_EXPORT_ROOT_DIR")
	if exportRootDir == "" {
		zlog.Fatal().Msgf("unset env variable: GOOGLE_EXPORT_ROOT_DIR")
	}

	googleClientSecret = []byte(os.Getenv("GOOGLE_CLIENT_SECRET"))
	if os.Getenv("GOOGLE_CLIENT_SECRET") == "" {
		googleClientSecret = nil
		zlog.Debug().Msgf("unset env variable: GOOGLE_CLIENT_SECRET")
	}

	googleDriveAccessToken = os.Getenv("GOOGLE_DRIVE_ACCESS_TOKEN")
	if googleDriveAccessToken == "" {
		zlog.Debug().Msgf("unset env variable: GOOGLE_DRIVE_ACCESS_TOKEN")
	}

	googleSpreadsheetAccessToken = os.Getenv("GOOGLE_SPREADSHEET_ACCESS_TOKEN")
	if googleSpreadsheetAccessToken == "" {
		zlog.Debug().Msgf("unset env variable: GOOGLE_SPREADSHEET_ACCESS_TOKEN")
	}

	dbEndpoint := os.Getenv("DYNAMO_ENDPOINT")
	if dbEndpoint != "" {
		zlog.Info().Msgf("DYNAMO_ENDPOINT is set. %s", dbEndpoint)
		sess := session.Must(session.NewSessionWithOptions(session.Options{
			Profile:           "local",
			SharedConfigState: session.SharedConfigEnable,
			Config: aws.Config{
				Endpoint: aws.String(dbEndpoint),
			},
		}))
		gdb = dynamo.New(sess)
	} else {
		zlog.Info().Msg("DYNAMO_ENDPOINT is not set")
		gdb = dynamo.New(session.Must(session.NewSession(&aws.Config{
			Region: aws.String(region),
			// 接続に際するリトライ処理
			HTTPClient: &http.Client{
				Timeout: time.Second * 10,
			},
			MaxRetries: aws.Int(3),
		})))
	}
}

func Handle(ctx context.Context) {
	t := time.Now()
	const layout = "2006-01"
	operationMonth := t.Format(layout)
	cls, err := CreateCls(ctx)
	if err != nil {
		zlog.Error().Msgf("%v", err)
	}
	url, err := ExportGoogleSpreadSheet(ctx, operationMonth, cls)
	if err != nil {
		zlog.Error().Msgf("%v", err)
	}
	zlog.Info().Msgf("url = %s", url)
}

func CreateCls(ctx context.Context) ([][]string, error) {
	var cls [][]string
	var c []string
	maxRangeRow := 3
	maxRangeCol := 4
	// ガワの作成
	for i := 0; i < maxRangeRow; i++ {
		c = []string{}
		for j := 0; j < maxRangeCol; j++ {
			c = append(c, "")
		}
		cls = append(cls, c)
	}
	cls[0][0] = "0-0"
	cls[1][1] = "1-1"
	cls[2][1] = "2-1"
	return cls, nil
}

func ExportGoogleSpreadSheet(ctx context.Context, operationMonth string, cls [][]string) (string, error) {
	// GoogleDriveとSpreadSheetへアクセスするのに必要な情報を取得する
	var clientSecret []byte
	if googleClientSecret == nil {
		b, err := ioutil.ReadFile("client_secret.json")
		if err != nil {
			return "", fmt.Errorf("unable to read client secret file: %v", err)
		}
		clientSecret = b
	} else {
		zlog.Debug().Msgf("client secret is set.")
		clientSecret = googleClientSecret
	}
	spreadsheetConfig, err := google.ConfigFromJSON(clientSecret,
		"https://www.googleapis.com/auth/spreadsheets",
		"https://www.googleapis.com/auth/drive.file")
	if err != nil {
		return "", fmt.Errorf("unable to set config: %v", err)
	}

	// googleSpreadsheetAccsessTokenが環境変数として定義されていなければtoken.s.jsonファイルを探しに行く
	spreadsheetClient := GetClient(ctx, spreadsheetConfig, "token.s.json")
	spreadsheetService, err := sheets.New(spreadsheetClient)
	if err != nil {
		zlog.Fatal().Msgf("Unable to retrieve Sheets Client %v", err)
	}

	driveConfig, err := google.ConfigFromJSON(clientSecret, drive.DriveScope)
	if err != nil {
		zlog.Fatal().Msgf("Unable to parse client secret file to config: %v", err)
	}

	// googleDriveAccsessTokenが環境変数として定義されていなければtoken.d.jsonファイルを探しに行く
	driveClient := GetClient(ctx, driveConfig, "token.d.json")
	driveService, err := drive.New(driveClient)
	if err != nil {
		zlog.Fatal().Msgf("Unable to retrieve Drive client: %v", err)
	}

	// 対象となるスプレッドシートが有るかをDBで検索
	table := exportTable
	targetSpreadsheetID, err := GetSpreadsheetID(ctx, table, operationMonth)
	if err != nil {
		// 新規に生成するスプレッドシートの設定
		targetSpreadsheetID, err = CreateBlankSpreadsheet(ctx, operationMonth, driveService)
		if err != nil {
			return "", err
		}
		// 作ったテーブルをDBに格納する
		if err = StoreSpreadsheetID(ctx, table, targetSpreadsheetID, operationMonth); err != nil {
			return "", err
		}
	}

	// 対象のシートがあるかどうかを確認する
	var sheetName string
	sheetName = fmt.Sprintf("%s%s", operationMonth[0:4], operationMonth[5:7])
	resp, err := spreadsheetService.Spreadsheets.Get(targetSpreadsheetID).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("fail to get spreadsheet info: %w", err)
	}
	isExistedTargetSheet := false
	for _, s := range resp.Sheets {
		if sheetName == s.Properties.Title {
			isExistedTargetSheet = true
		}
	}
	if isExistedTargetSheet {
		zlog.Debug().Msgf("sheetName=%s is existed", sheetName)
	} else {
		// 対象シートを作る
		if err = AddSheetToSpreadsheet(ctx, targetSpreadsheetID, sheetName, spreadsheetService); err != nil {
			return "", err
		}

	}

	v := [][]interface{}{}
	vv := []interface{}{}
	for _, c := range cls {
		vv = []interface{}{}
		for _, cc := range c {
			vv = append(vv, cc)
		}
		v = append(v, vv)
	}
	// シート1のA1セルを起点にして書き込む
	writeRange := fmt.Sprintf("%s!A1", sheetName)
	valueRange := &sheets.ValueRange{
		Values: v,
	}
	_, err = spreadsheetService.Spreadsheets.Values.Update(targetSpreadsheetID, writeRange, valueRange).ValueInputOption("USER_ENTERED").Do()
	if err != nil {
		return "", fmt.Errorf("unable to retrieve data from sheet. %v", err)
	}

	return resp.SpreadsheetUrl, nil
}

func CreateBlankSpreadsheet(ctx context.Context, operationMonth string, service *drive.Service) (string, error) {
	var spreadsheetName string
	spreadsheetName = fmt.Sprintf("sample_%s%s", operationMonth[0:4], operationMonth[5:7])
	f := &drive.File{
		Name:     spreadsheetName,
		MimeType: "application/vnd.google-apps.spreadsheet",
		Parents: []string{
			exportRootDir,
		},
	}
	res, err := service.Files.Create(f).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("fail to create blank spreadsheet: %v", err)
	}
	return res.Id, nil
}

func AddSheetToSpreadsheet(ctx context.Context, spreadsheetID, sheetName string, service *sheets.Service) error {
	request := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			&sheets.Request{
				AddSheet: &sheets.AddSheetRequest{
					Properties: &sheets.SheetProperties{
						Title: sheetName,
					},
				},
			},
		},
	}
	_, err := service.Spreadsheets.BatchUpdate(spreadsheetID, request).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("fail to batch update spreadsheet: %v", err)
	}
	return nil
}

// Retrieve a token, saves the token, then returns the generated client.
func GetClient(ctx context.Context, config *oauth2.Config, tokFile string) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tok, err := TokenFromFile(ctx, tokFile)
	if err != nil {
		tok = GetTokenFromWeb(ctx, config)
		SaveToken(ctx, tokFile, tok)
	}
	return config.Client(ctx, tok)
}

// Request a token from the web, then returns the retrieved token.
func GetTokenFromWeb(ctx context.Context, config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	zlog.Info().Msgf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		zlog.Fatal().Msgf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(ctx, authCode)
	if err != nil {
		zlog.Fatal().Msgf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
func TokenFromFile(ctx context.Context, file string) (*oauth2.Token, error) {
	if file == "token.d.json" && googleDriveAccessToken != "" {
		tok := &oauth2.Token{}
		err := json.Unmarshal([]byte(googleDriveAccessToken), &tok)
		return tok, err
	} else if file == "token.s.json" && googleSpreadsheetAccessToken != "" {
		tok := &oauth2.Token{}
		err := json.Unmarshal([]byte(googleSpreadsheetAccessToken), &tok)
		return tok, err
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func SaveToken(ctx context.Context, path string, token *oauth2.Token) {
	zlog.Info().Msgf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		zlog.Fatal().Msgf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}
