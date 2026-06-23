package query

import "testing"

// TestClassify 验证按首关键字的类别判定。
// 同时作为新增只读元数据关键字（SHOW/DESCRIBE/DESC/EXPLAIN）的回归保护。
func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want Kind
	}{
		// --- 只读：SELECT / WITH ---
		{"select", "SELECT * FROM t", KindSelect},
		{"with_cte", "WITH x AS (SELECT 1) SELECT * FROM x", KindSelect},
		{"select_lowercase", "select 1", KindSelect},
		{"select_leading_ws", "   \n  SELECT 1", KindSelect},

		// --- 只读：元数据查询（本次新增） ---
		{"show_create_table", "SHOW CREATE TABLE ods.ODS_EDS_LOT_HIST_MOD_HI", KindSelect},
		{"show_databases", "SHOW DATABASES", KindSelect},
		{"show_tables", "SHOW TABLES", KindSelect},
		{"show_columns", "SHOW COLUMNS FROM t", KindSelect},
		{"show_lowercase", "show create table t", KindSelect},
		{"describe", "DESCRIBE t", KindSelect},
		{"desc", "DESC t", KindSelect},
		{"explain", "EXPLAIN SELECT * FROM t", KindSelect},
		{"explain_lowercase", "explain select * from t", KindSelect},

		// --- DML ---
		{"insert", "INSERT INTO t VALUES (1)", KindDML},
		{"update", "UPDATE t SET a=1 WHERE id=1", KindDML},
		{"delete", "DELETE FROM t WHERE id=1", KindDML},
		{"merge", "MERGE INTO t USING s ON (t.id=s.id) WHEN MATCHED THEN UPDATE SET t.a=s.a", KindDML},

		// --- DDL ---
		{"create", "CREATE TABLE t (id INT)", KindDDL},
		{"alter", "ALTER TABLE t ADD COLUMN a INT", KindDDL},
		{"drop", "DROP TABLE t", KindDDL},
		{"truncate", "TRUNCATE TABLE t", KindDDL},
		{"rename", "RENAME TABLE a TO b", KindDDL},

		// --- 事务 ---
		{"commit", "COMMIT", KindTransaction},
		{"rollback", "ROLLBACK", KindTransaction},
		{"savepoint", "SAVEPOINT sp1", KindTransaction},
		{"set", "SET TRANSACTION ISOLATION LEVEL READ COMMITTED", KindTransaction},

		// --- 未知 ---
		{"unknown", "VACUUM", KindUnknown},
		{"empty", "", KindUnknown},
		{"ws_only", "   \n  ", KindUnknown},

		// --- 前导注释应被跳过 ---
		{"line_comment_then_select", "-- 注释\nSELECT 1", KindSelect},
		{"block_comment_then_show", "/* hi */ SHOW CREATE TABLE t", KindSelect},
		{"line_comment_then_ddl", "-- 注释\nDROP TABLE t", KindDDL},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Classify(c.sql); got != c.want {
				t.Errorf("Classify(%q) = %v, want %v", c.sql, got, c.want)
			}
		})
	}
}

// TestKindIsReadOnly 验证只读判定的边界，保护 WriteGuard 守卫的触发条件。
// SHOW/DESCRIBE/EXPLAIN 归入 KindSelect 后必须 IsReadOnly()==true，
// 否则只读库会被误拒（正是本次修复的回归点）。
func TestKindIsReadOnly(t *testing.T) {
	readOnly := []Kind{KindSelect}
	for _, k := range readOnly {
		if !k.IsReadOnly() {
			t.Errorf("Kind(%v).IsReadOnly() = false, want true", k)
		}
	}
	notReadOnly := []Kind{KindDML, KindDDL, KindTransaction, KindUnknown}
	for _, k := range notReadOnly {
		if k.IsReadOnly() {
			t.Errorf("Kind(%v).IsReadOnly() = true, want false", k)
		}
	}
}

// TestShowCreateTableIsReadOnly 是本次修复的核心回归点：
// SHOW CREATE TABLE 必须被识别为只读，否则在只读数据源上会被 WriteGuard 误拒，
// 且即使放行也会走 Exec 路径丢弃结果集。
func TestShowCreateTableIsReadOnly(t *testing.T) {
	sql := "SHOW CREATE TABLE ods.ODS_EDS_LOT_HIST_MOD_HI"
	k := Classify(sql)
	if !k.IsReadOnly() {
		t.Fatalf("SHOW CREATE TABLE must be read-only; classified as %v", k)
	}
}

// TestIsDestructive 验证危险语句判定，确保元数据关键字补充不影响其行为。
func TestIsDestructive(t *testing.T) {
	destructive := []string{
		"DROP TABLE t",
		"TRUNCATE TABLE t",
		"DELETE FROM t",        // 无 WHERE
		"UPDATE t SET a = 1",   // 无 WHERE
	}
	for _, sql := range destructive {
		if !IsDestructive(sql) {
			t.Errorf("IsDestructive(%q) = false, want true", sql)
		}
	}
	safe := []string{
		"DELETE FROM t WHERE id = 1",
		"UPDATE t SET a = 1 WHERE id = 1",
		// 本次新增的只读元数据语句不应被误判为危险（防御性断言）：
		"SHOW CREATE TABLE t",
		"DESCRIBE t",
		"EXPLAIN SELECT 1",
	}
	for _, sql := range safe {
		if IsDestructive(sql) {
			t.Errorf("IsDestructive(%q) = true, want false", sql)
		}
	}
}

// TestKindString 验证 String() 的稳定输出（错误提示里引用，避免回归）。
func TestKindString(t *testing.T) {
	cases := []struct {
		k    Kind
		want string
	}{
		{KindSelect, "SELECT"},
		{KindDML, "DML"},
		{KindDDL, "DDL"},
		{KindTransaction, "TRANSACTION"},
		{KindUnknown, "UNKNOWN"},
	}
	for _, c := range cases {
		if got := c.k.String(); got != c.want {
			t.Errorf("Kind(%v).String() = %q, want %q", c.k, got, c.want)
		}
	}
}
