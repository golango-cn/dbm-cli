package driver

import (
	"fmt"
	"sort"
	"sync"
)

// registry 是驱动的全局注册表。
// 使用「自注册 + 按名查找」模式：每个驱动包在被 import 时，
// 其 init() 调用 Register 把自己登记进来；CLI 按配置里的 type 查找。
var (
	mu       sync.RWMutex
	drivers  = map[string]Driver{}
	aliases  = map[string]string{} // alias -> canonical name，如 mariadb -> mysql
)

// Register 登记一个驱动。重复注册同名驱动会 panic（属于编程错误，应在启动期暴露）。
func Register(d Driver) {
	mu.Lock()
	defer mu.Unlock()
	if d == nil {
		panic("driver: Register called with nil Driver")
	}
	name := d.Name()
	if name == "" {
		panic("driver: Register called with empty driver name")
	}
	if _, exists := drivers[name]; exists {
		panic(fmt.Sprintf("driver: driver %q already registered", name))
	}
	drivers[name] = d
}

// RegisterAlias 为一个驱动名注册别名。
// 用于让兼容协议的数据库共用同一驱动实现——例如 MariaDB 协议兼容 MySQL，
// 注册别名 "mariadb" -> "mysql" 后，配置里 type: mariadb 即可使用 mysql 驱动。
// canonical 必须是已 Register 过的驱动名，否则 panic。
func RegisterAlias(alias, canonical string) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := drivers[canonical]; !ok {
		panic(fmt.Sprintf("driver: cannot alias %q to unknown driver %q", alias, canonical))
	}
	if _, ok := drivers[alias]; ok {
		panic(fmt.Sprintf("driver: alias %q conflicts with a registered driver name", alias))
	}
	aliases[alias] = canonical
}

// resolveName 把别名解析为规范驱动名；非别名则原样返回。
func resolveName(name string) string {
	if canon, ok := aliases[name]; ok {
		return canon
	}
	return name
}

// Get 按名获取驱动（自动解析别名），未找到返回错误。
func Get(name string) (Driver, error) {
	mu.RLock()
	defer mu.RUnlock()
	canon := resolveName(name)
	d, ok := drivers[canon]
	if !ok {
		return nil, fmt.Errorf("driver: unknown driver type %q (registered: %v)", name, Names())
	}
	return d, nil
}

// List 返回所有已注册驱动（按名字排序），供 manifest / help 展示。
func List() []Driver {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Driver, 0, len(drivers))
	for _, d := range drivers {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// Names 返回已注册驱动名（按字母序），含别名。
// 别名也一并列出，让 manifest/help 展示用户可用的全部 type 值。
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(drivers)+len(aliases))
	for n := range drivers {
		names = append(names, n)
	}
	for a := range aliases {
		names = append(names, a)
	}
	sort.Strings(names)
	return names
}

// AliasesOf 返回指向给定规范驱动名的全部别名（按字母序）。
// 供 manifest 展示「mariadb 是 mysql 的别名」这类信息。
func AliasesOf(canonical string) []string {
	mu.RLock()
	defer mu.RUnlock()
	var out []string
	for alias, canon := range aliases {
		if canon == canonical {
			out = append(out, alias)
		}
	}
	sort.Strings(out)
	return out
}
