# google-spreadsheet-exporter

This example program export spreadsheet by Golang.

## Run

0. Set AWS profile.

    ```bash
    # AWS profile(初回のみ)
    aws configure set aws_access_key_id dummy     --profile local
    aws configure set aws_secret_access_key dummy --profile local
    aws configure set region ap-northeast-1       --profile local
    ```

1. Get Client Secret and save it as `client_secret.json` .

2. Create a folder and set its id in `Makefile`.

    ```Makefile
    export GOOGLE_EXPORT_ROOT_DIR="example";
    ```

3. Run docker with below commands.

    ```bash
    $ docker-compose -f docker-compose.yml up -d
    ```

4. Run `make test`.

5. Get Access token for Google Drive and seve is as `token.d.json` .

6. Get Access token for Google Spreadsheet and seve is as `token.s.json` .

7. Check a spreadsheet in your google drive.

## Deploy

If you want run this program on AWS Lambda, set below variables.

```bash
export GOOGLE_CLIENT_SECRET="" # paste your client secret
export GOOGLE_DRIVE_ACCESS_TOKEN="" # paste your google drive access token
export GOOGLE_SPREADSHEET_ACCESS_TOKEN="" # paste your google spreadsheet access token
```