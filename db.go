package gsexp

import (
	"context"
	"fmt"

	"github.com/guregu/dynamo"
)

var gdb *dynamo.DB

type ResultExport struct {
	OperationMonth string `dynamo:"operation_month"`
	SpreadsheetID  string `dynamo:"spreadsheet_id"`
}

func GetSpreadsheetID(ctx context.Context, t, operationDate string) (string, error) {
	var resp ResultExport
	operationMonth := fmt.Sprintf("%s", operationDate[5:7])
	table := gdb.Table(t)
	err := table.Get("operation_month", operationMonth).
		Consistent(true).
		OneWithContext(ctx, &resp)
	if err != nil {
		return "", err
	}
	if resp.SpreadsheetID == "" {
		return "", fmt.Errorf("spreadsheet ID is not recoreded")
	}
	return resp.SpreadsheetID, nil
}

func StoreSpreadsheetID(ctx context.Context, t, id, operationDate string) error {
	operationMonth := fmt.Sprintf("%s", operationDate[5:7])
	table := gdb.Table(t)
	item := &ResultExport{
		OperationMonth: operationMonth,
		SpreadsheetID:  id,
	}
	return table.Put(item).RunWithContext(ctx)
}
