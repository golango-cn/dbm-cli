// Package dbmcli 是对外暴露的稳定高层 API（Facade）。
//
// 它封装 internal/driver 与 internal/config 的常用组合，
// 让外部 Go 程序或集成测试能以最小代价使用 dbm 的能力，
// 而不必直接依赖 internal 包（Go 的 internal 机制禁止跨模块访问）。
//
// 用法示例：
//
//	cfg := &dbm.Datasource{
//	    Type:        "oracle",
//	    Host:        "10.0.0.1", Port: 1521,
//	    ServiceName: "ORCLPDB1", User: "ro", Password: os.Getenv("PWD"),
//	}
//	conn, err := dbm.Open(ctx, cfg)
//	...
//	rows, _ := conn.Metadata().Tables(ctx, "HR")
package dbmcli

import (
	"context"
	"fmt"

	"github.com/golango-cn/dbm-cli/internal/config"
	"github.com/golango-cn/dbm-cli/internal/driver"

	// 触发已实现 driver 的自注册。
	_ "github.com/golango-cn/dbm-cli/internal/clickhouse"
	_ "github.com/golango-cn/dbm-cli/internal/mysql"
	_ "github.com/golango-cn/dbm-cli/internal/oracle"
	_ "github.com/golango-cn/dbm-cli/internal/postgresql"
)

// Datasource 是对外暴露的数据源配置（internal 类型的稳定投影）。
type Datasource struct {
	Type         string
	Host         string
	Port         int
	User         string
	Password     string
	Database     string // MySQL 等的数据库名
	TLS          string // 可选 TLS：skip-verify / true / 自定义
	ServiceName  string // Oracle
	SID          string // Oracle
	AllowWrite   bool
	ForceVersion string
	FetchSize    int
	MaxOpenConns int
	MaxIdleConns int
}

// Conn 是 internal.driver.Conn 的对外别名。
type Conn = driver.Conn

// MetadataProvider 是对外别名。
type MetadataProvider = driver.MetadataProvider

// Open 依据 Datasource 配置打开连接（带写守卫）。
func Open(ctx context.Context, ds *Datasource) (Conn, func(), error) {
	if ds == nil {
		return nil, nil, fmt.Errorf("dbm: nil datasource")
	}
	cfg := &driver.DatasourceConfig{
		Type:         ds.Type,
		Host:         ds.Host,
		Port:         ds.Port,
		User:         ds.User,
		Password:     ds.Password,
		Database:     ds.Database,
		TLS:          ds.TLS,
		AllowWrite:   ds.AllowWrite,
		ForceVersion: ds.ForceVersion,
		ServiceName:  ds.ServiceName,
		SID:          ds.SID,
		FetchSize:    ds.FetchSize,
		MaxOpenConns: ds.MaxOpenConns,
		MaxIdleConns: ds.MaxIdleConns,
	}
	d, err := driver.Get(cfg.Type)
	if err != nil {
		return nil, nil, err
	}
	c, err := d.Open(cfg)
	if err != nil {
		return nil, nil, err
	}
	guarded := driver.NewWriteGuard(c, cfg.AllowWrite)
	cleanup := func() { _ = guarded.Close() }
	return guarded, cleanup, nil
}

// OpenFromConfigFile 从 dbm 配置文件按名称打开数据源。
func OpenFromConfigFile(ctx context.Context, configPath, datasource string) (Conn, func(), error) {
	f, err := config.Load(configPath)
	if err != nil {
		return nil, nil, err
	}
	dsCfg, ok := f.Datasources[datasource]
	if !ok {
		if f.Default != "" {
			dsCfg = f.Datasources[f.Default]
		}
	}
	if dsCfg == nil {
		return nil, nil, fmt.Errorf("dbm: datasource %q not found", datasource)
	}
	d, err := driver.Get(dsCfg.Type)
	if err != nil {
		return nil, nil, err
	}
	c, err := d.Open(dsCfg)
	if err != nil {
		return nil, nil, err
	}
	guarded := driver.NewWriteGuard(c, dsCfg.AllowWrite)
	cleanup := func() { _ = guarded.Close() }
	return guarded, cleanup, nil
}
