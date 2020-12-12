package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	gsexp "github.com/montblanc18/google-spreadsheet-exporter"
)

func main() {
	lambda.Start(gsexp.Handle)
}
