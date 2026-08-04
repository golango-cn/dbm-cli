package impala

// Kerberos 凭据获取：用 keytab 换取 TGT，写成 MIT ccache 文件供驱动读取。
//
// 背景：go-impala/v3 驱动的 SASL/GSSAPI（经 golang-auth/go-gssapi）只认票据缓存
// （KRB5CCNAME 指向的 ccache 文件），Options 结构里没有 keytab 字段。而上游
// jcmturner/gokrb5 只能读 ccache、不能写。因此这里用 jfjallid/gokrb5 fork
// （补了 Marshal / SaveAllTicketsToCCache API）来完成 keytab→ccache 的换取。
//
// 流程：
//   keytab.Load → client.NewWithKeytab → Login() → SaveAllTicketsToCCache
//   → CCache.Marshal() → 写临时文件 → os.Setenv("KRB5CCNAME", path)
//
// 全程纯 Go，不依赖系统 kinit 命令。临时 ccache 文件由返回的 cleanup 删除。

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jfjallid/gokrb5/v8/client"
	"github.com/jfjallid/gokrb5/v8/config"
	"github.com/jfjallid/gokrb5/v8/credentials"
	"github.com/jfjallid/gokrb5/v8/keytab"
)

// acquireTicketCache 用 keytab 登录 KDC 换取 TGT，写成临时 ccache 文件，
// 并设置 KRB5CCNAME 环境变量，使驱动（go-gssapi krb5 backend）能读到票据。
//
// 参数：
//   - principal   用户主体，如 "cdptest@TESTBOE.COM"（含 realm）
//   - keytabPath  keytab 文件路径
//   - krb5ConfPath krb5.conf 路径
//
// 返回：
//   - ccachePath 写出的 ccache 文件路径
//   - cleanup    回收函数（删除临时文件、恢复 KRB5CCNAME），由调用方在 Close 时 defer
//   - err        失败原因
func acquireTicketCache(principal, keytabPath, krb5ConfPath string) (ccachePath string, cleanup func(), err error) {
	// 解析 principal -> username + realm（形如 cdptest@TESTBOE.COM）
	username, realm, ok := splitPrincipal(principal)
	if !ok {
		return "", nil, fmt.Errorf("impala: invalid kerberos principal %q (expected user@REALM)", principal)
	}

	// 1. 加载 keytab
	kt, err := keytab.Load(keytabPath)
	if err != nil {
		return "", nil, fmt.Errorf("impala: load keytab %s: %w", keytabPath, err)
	}

	// 2. 加载 krb5.conf（含 KDC 地址）
	cfg, err := config.Load(krb5ConfPath)
	if err != nil {
		return "", nil, fmt.Errorf("impala: load krb5.conf %s: %w", krb5ConfPath, err)
	}

	// 3. 用 keytab 构造 client 并登录换取 TGT
	cl, err := client.NewWithKeytab(username, realm, kt, cfg)
	if err != nil {
		return "", nil, fmt.Errorf("impala: new kerberos client: %w", err)
	}
	if err := cl.Login(); err != nil {
		return "", nil, fmt.Errorf("impala: kerberos login (kinit) for %s: %w", principal, err)
	}

	// 4. 把 TGT + 已有 service tickets 导入 CCache 结构
	cc := credentials.NewV4CCache()
	clientCName := cl.Credentials.CName()
	// 必须显式设置 default principal：go-gssapi 的 NewFromCCache 用它来定位 TGT，
	// 而 SaveAllTicketsToCCache 内部只 AddCredential，不设 default principal。
	cc.SetDefaultPrincipal(credentials.NewPrincipal(clientCName, realm))
	if err := cl.SaveAllTicketsToCCache(cc, clientCName, realm); err != nil {
		return "", nil, fmt.Errorf("impala: save tickets to ccache: %w", err)
	}

	// 5. 序列化为 MIT ccache v4 二进制
	data, err := cc.Marshal()
	if err != nil {
		return "", nil, fmt.Errorf("impala: marshal ccache: %w", err)
	}

	// 6. 写临时文件（0600，含票据，必须限制权限）
	f, err := os.CreateTemp("", "dbm-cli-krb5cc-*")
	if err != nil {
		return "", nil, fmt.Errorf("impala: create temp ccache: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", nil, fmt.Errorf("impala: write temp ccache: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", nil, fmt.Errorf("impala: close temp ccache: %w", err)
	}
	ccachePath, _ = filepath.Abs(f.Name())

	// 7. 设置 KRB5CCNAME，驱动（go-gssapi）会从这里读票据
	prevCCName, hadPrev := os.LookupEnv("KRB5CCNAME")
	os.Setenv("KRB5CCNAME", "FILE:"+ccachePath)

	cleanup = func() {
		os.Remove(ccachePath)
		if hadPrev {
			os.Setenv("KRB5CCNAME", prevCCName)
		} else {
			os.Unsetenv("KRB5CCNAME")
		}
	}
	return ccachePath, cleanup, nil
}

// splitPrincipal 把 "user@REALM" 拆成 (user, realm)。
// 不允许 @ 出现在 user 部分（Kerberos 主体不含字面 @）。
func splitPrincipal(p string) (user, realm string, ok bool) {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '@' {
			return p[:i], p[i+1:], true
		}
	}
	return "", "", false
}
