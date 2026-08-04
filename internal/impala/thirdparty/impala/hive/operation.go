package hive

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golango-cn/dbm-cli/internal/impala/thirdparty/impala/services/cli_service"
)

// Operation represents hive operation
type Operation struct {
	hive *Client
	h    *cli_service.TOperationHandle
}

// HasResultSet return if operation has result set
func (op *Operation) HasResultSet() bool {
	return op.h.GetHasResultSet()
}

// RowsAffected return number of rows affected by operation
func (op *Operation) RowsAffected() float64 {
	return op.h.GetModifiedRowCount()
}

// GetResultSetMetadata return schema
func (op *Operation) GetResultSetMetadata(ctx context.Context) (*TableSchema, error) {
	op.hive.log.Printf("fetch metadata for operation: %v", guid(op.h.OperationId.GUID))
	req := cli_service.TGetResultSetMetadataReq{
		OperationHandle: op.h,
	}

	resp, err := op.hive.client.GetResultSetMetadata(ctx, &req)
	if err != nil {
		return nil, err
	}
	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	schema := new(TableSchema)

	if resp.IsSetSchema() {
		for _, desc := range resp.Schema.Columns {
			entry := desc.TypeDesc.Types[0].PrimitiveEntry

			dbtype := strings.TrimSuffix(entry.Type.String(), "_TYPE")
			schema.Columns = append(schema.Columns, &ColDesc{
				Name:             desc.ColumnName,
				DatabaseTypeName: dbtype,
				ScanType:         typeOf(entry),
			})
		}

		for _, col := range schema.Columns {
			op.hive.log.Printf("fetch schema: %v", col)
		}
	}

	return schema, nil
}

// FetchResults fetches query result from server
func (op *Operation) FetchResults(ctx context.Context, schema *TableSchema) (*ResultSet, error) {

	resp, err := fetch(ctx, op, schema)
	if err != nil {
		return nil, err
	}

	rs := ResultSet{
		idx:     0,
		length:  length(resp.Results),
		result:  resp.Results,
		more:    resp.GetHasMoreRows(),
		schema:  schema,
		fetchfn: func() (*cli_service.TFetchResultsResp, error) { return fetch(ctx, op, schema) },
	}

	return &rs, nil
}

func fetch(ctx context.Context, op *Operation, schema *TableSchema) (*cli_service.TFetchResultsResp, error) {
	req := cli_service.TFetchResultsReq{
		OperationHandle: op.h,
		MaxRows:         op.hive.opts.MaxRows,
	}

	op.hive.log.Printf("fetch results for operation: %v", guid(op.h.OperationId.GUID))

	resp, err := op.hive.client.FetchResults(ctx, &req)
	if err != nil {
		return nil, err
	}
	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	op.hive.log.Printf("results: %v", resp.Results)
	return resp, nil
}

// WaitForCompletion 轮询操作状态直到到达终态（FINISHED / ERROR / CANCELED / CLOSED）。
// HiveServer2 的 ExecuteStatement 是异步的：对 INSERT/UPDATE/DDL 等写操作，
// 必须等待 FINISHED 才能保证副作用（数据落盘）完成；直接 Close 会取消未完成的操作。
// 返回终态响应（含 ErrorMessage / ErrorCode），由调用方判断成功或失败。
func (op *Operation) WaitForCompletion(ctx context.Context) error {
	for {
		resp, err := op.hive.GetOperationStatus(ctx, op.h)
		if err != nil {
			return err
		}
		state := resp.GetOperationState()
		switch state {
		case cli_service.TOperationState_FINISHED_STATE:
			return nil
		case cli_service.TOperationState_ERROR_STATE:
			msg := resp.GetErrorMessage()
			return fmt.Errorf("hive: operation failed: %s", msg)
		case cli_service.TOperationState_CANCELED_STATE:
			return fmt.Errorf("hive: operation canceled")
		case cli_service.TOperationState_CLOSED_STATE:
			return fmt.Errorf("hive: operation closed before completion")
		}
		// 仍在运行（INITIALIZED/RUNNING/PENDING/UKNOWN），短暂等待后重试
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// Close closes operation
func (op *Operation) Close(ctx context.Context) error {
	req := cli_service.TCloseOperationReq{
		OperationHandle: op.h,
	}
	resp, err := op.hive.client.CloseOperation(ctx, &req)
	if err != nil {
		return err
	}
	if err := checkStatus(resp); err != nil {
		return err
	}

	op.hive.log.Printf("close operation: %v", guid(op.h.OperationId.GUID))
	return nil
}
