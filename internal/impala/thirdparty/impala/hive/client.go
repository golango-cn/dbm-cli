package hive

import (
	"context"
	"log"
	"strconv"

	"github.com/golango-cn/dbm-cli/internal/impala/thirdparty/impala/services/cli_service"
	"github.com/apache/thrift/lib/go/thrift"
)

// Client represents Hive Client
type Client struct {
	client *cli_service.TCLIServiceClient
	opts   *Options
	log    *log.Logger
}

// Options for Hive Client
type Options struct {
	MaxRows                int64
	MemLimit               string
	QueryTimeout           int
	ParquetArrayResolution string
}

// NewClient creates Hive Client
func NewClient(client thrift.TClient, log *log.Logger, opts *Options) *Client {
	return &Client{
		client: cli_service.NewTCLIServiceClient(client),
		log:    log,
		opts:   opts,
	}
}

// GetOperationStatus 查询某个操作的执行状态（HiveServer2 的 GetOperationStatus RPC）。
// ExecuteStatement 是异步的，必须轮询状态直到 FINISHED/ERROR/CANCELED 才能确认执行结果。
func (c *Client) GetOperationStatus(ctx context.Context, opHandle *cli_service.TOperationHandle) (*cli_service.TGetOperationStatusResp, error) {
	req := cli_service.TGetOperationStatusReq{
		OperationHandle: opHandle,
	}
	resp, err := c.client.GetOperationStatus(ctx, &req)
	if err != nil {
		return nil, err
	}
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// OpenSession creates new hive session
func (c *Client) OpenSession(ctx context.Context) (*Session, error) {
	cfg := map[string]string{
		"MEM_LIMIT":                c.opts.MemLimit,
		"QUERY_TIMEOUT_S":          strconv.Itoa(c.opts.QueryTimeout),
		"PARQUET_ARRAY_RESOLUTION": c.opts.ParquetArrayResolution,
		"BATCH_SIZE":               strconv.FormatInt(c.opts.MaxRows, 10),
	}
	req := cli_service.TOpenSessionReq{
		ClientProtocol: cli_service.TProtocolVersion_HIVE_CLI_SERVICE_PROTOCOL_V7,
		Configuration:  cfg,
	}

	resp, err := c.client.OpenSession(ctx, &req)
	if err != nil {
		return nil, err
	}
	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	c.log.Printf("open session: %s", guid(resp.SessionHandle.GetSessionId().GUID))
	c.log.Printf("session config: %v", resp.Configuration)
	return &Session{h: resp.SessionHandle, hive: c}, nil
}
