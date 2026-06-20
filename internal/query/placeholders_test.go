package query

import "testing"

func TestPlaceholderStyleFor(t *testing.T) {
	cases := []struct {
		driver string
		want   PlaceholderStyle
	}{
		{"mysql", StyleQuestion},
		{"mariadb", StyleQuestion},
		{"clickhouse", StyleQuestion},
		{"postgres", StyleDollar},
		{"postgresql", StyleDollar},
		{"PostgreSQL", StyleDollar},
		{"oracle", StyleColonNumeric},
		{"ORACLE", StyleColonNumeric},
		{"unknown", StyleQuestion}, // 未知驱动默认 ?
	}
	for _, c := range cases {
		if got := PlaceholderStyleFor(c.driver); got != c.want {
			t.Errorf("PlaceholderStyleFor(%q) = %v, want %v", c.driver, got, c.want)
		}
	}
}

func TestNormalizePlaceholders_Question(t *testing.T) {
	// StyleQuestion 应原样返回，不做任何替换
	in := "SELECT * FROM t WHERE a=? AND b=?"
	if got := NormalizePlaceholders(in, StyleQuestion); got != in {
		t.Errorf("StyleQuestion should be no-op, got %q", got)
	}
}

func TestNormalizePlaceholders_Dollar(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"SELECT * FROM t WHERE a=?", "SELECT * FROM t WHERE a=$1"},
		{"SELECT * FROM t WHERE a=? AND b=?", "SELECT * FROM t WHERE a=$1 AND b=$2"},
		{"SELECT * FROM t", "SELECT * FROM t"}, // 无占位符
		{"INSERT INTO t VALUES (?, ?, ?)", "INSERT INTO t VALUES ($1, $2, $3)"},
	}
	for _, c := range cases {
		if got := NormalizePlaceholders(c.in, StyleDollar); got != c.want {
			t.Errorf("NormalizePlaceholders(%q, Dollar) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizePlaceholders_Colon(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"SELECT * FROM t WHERE a=?", "SELECT * FROM t WHERE a=:1"},
		{"SELECT * FROM t WHERE a=? AND b=?", "SELECT * FROM t WHERE a=:1 AND b=:2"},
		{"SELECT * FROM t", "SELECT * FROM t"},
		{"UPDATE t SET x=? WHERE id=?", "UPDATE t SET x=:1 WHERE id=:2"},
	}
	for _, c := range cases {
		if got := NormalizePlaceholders(c.in, StyleColonNumeric); got != c.want {
			t.Errorf("NormalizePlaceholders(%q, ColonNumeric) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizePlaceholders_10Plus(t *testing.T) {
	// 10 个以上占位符：验证两位数编号（$10, :11）
	in := "SELECT ?,?,?,?,?,?,?,?,?,?,?"
	wantDollar := "SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11"
	if got := NormalizePlaceholders(in, StyleDollar); got != wantDollar {
		t.Errorf("11 placeholders (Dollar) = %q, want %q", got, wantDollar)
	}
	wantColon := "SELECT :1,:2,:3,:4,:5,:6,:7,:8,:9,:10,:11"
	if got := NormalizePlaceholders(in, StyleColonNumeric); got != wantColon {
		t.Errorf("11 placeholders (ColonNumeric) = %q, want %q", got, wantColon)
	}
}

func TestCountPlaceholders(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"SELECT * FROM t WHERE a=?", 1},
		{"SELECT * FROM t WHERE a=? AND b=?", 2},
		{"SELECT * FROM t", 0},
		{"INSERT INTO t VALUES (?,?,?)", 3},
	}
	for _, c := range cases {
		if got := CountPlaceholders(c.in); got != c.want {
			t.Errorf("CountPlaceholders(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestItoa(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{1, "1"}, {9, "9"}, {10, "10"}, {100, "100"}, {0, "0"},
	}
	for _, c := range cases {
		if got := itoa(c.in); got != c.want {
			t.Errorf("itoa(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
