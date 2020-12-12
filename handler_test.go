package gsexp

import (
	"context"
	"encoding/csv"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	zlog "github.com/rs/zerolog/log"
)

func TestExportSpreadsheet(t *testing.T) {

	tests := []struct {
		name           string
		operationMonth string
		want           string
		wantErr        error
		setupCmds      []string
		cleanupCmds    []string
	}{
		{
			name:           "[正常系] データ出力",
			operationMonth: "2020-08",
			want:           "./testdata/want_1.csv",
			wantErr:        nil,
			setupCmds: []string{
				`aws dynamodb --profile local --endpoint-url http://localhost:4566 create-table --cli-input-json file://./testdata/local_export.json`,
			},
			cleanupCmds: []string{
				`aws dynamodb --profile local --endpoint-url http://localhost:4566 delete-table --table local_export`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdExec(t, tt.setupCmds)
			csvFile, err := os.Open(tt.want)
			if err != nil {
				t.Errorf("failed to open %v: %v", tt.want, err)
			}
			defer csvFile.Close()

			reader := csv.NewReader(csvFile)
			var w [][]string
			var l []string
			for {
				l, err = reader.Read()
				if err == io.EOF {
					break
				} else if err != nil {
					t.Errorf("failed to read %v: %v", tt.want, err)
					break
				}
				// google spreadsheetには数式を入れられるため、ここでも出力可能
				// 入れたい対象にカンマがあるとcsvファイルで用意するときに困るので、
				// csvファイルでは別の文字にしておいて、
				// 違う文字にして取り込み
				// 「...」->「,」へ
				for li, ll := range l {
					l[li] = strings.Replace(ll, "...", ",", -1)
				}
				w = append(w, l)
			}
			ctx := context.Background()
			cls, err := CreateCls(ctx)
			if err != tt.wantErr {
				t.Errorf("fail CreateCls(), err=%v", err)
			}
			if tt.wantErr != nil {
				cmdExec(t, tt.cleanupCmds)
				return
			}
			if diff := cmp.Diff(w, cls); diff != "" {
				t.Errorf("fail CreateCls(), mismatch (-want, +got):\n%s", diff)
			}

			url, err := ExportGoogleSpreadSheet(ctx, tt.operationMonth, cls)
			if err != nil {
				t.Errorf("%v", err)
			}
			zlog.Info().Msgf("url = %s", url)
			cmdExec(t, tt.cleanupCmds)
		})
	}

}

func cmdExec(t *testing.T, cmds []string) {
	t.Helper()
	for _, cmd := range cmds {
		args := strings.Split(cmd, " ")
		t.Logf("[INFO] command: %s", cmd)
		if err := exec.Command(args[0], args[1:]...).Run(); err != nil {
			t.Logf("[WARN] %s %v", cmd, err)
		}
	}
}
