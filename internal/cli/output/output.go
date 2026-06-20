// Package output 实现查询结果的多种格式化输出。
//
// 支持格式：table（默认对齐表格）、json、csv、yaml、vertical（\G 风格，每行一记录）。
// 输入统一为 driver.Result，与具体数据库解耦。
package output

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/golango-cn/dbm-cli/internal/driver"
)

// Format 是输出格式枚举。
type Format string

const (
	FormatTable    Format = "table"
	FormatJSON     Format = "json"
	FormatCSV      Format = "csv"
	FormatYAML     Format = "yaml"
	FormatVertical Format = "vertical"
)

// ParseFormat 把字符串转为 Format，非法值回退到 table。
func ParseFormat(s string) Format {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "json":
		return FormatJSON
	case "csv":
		return FormatCSV
	case "yaml":
		return FormatYAML
	case "vertical", "vert", "v":
		return FormatVertical
	default:
		return FormatTable
	}
}

// Options 控制输出行为。
type Options struct {
	Format   Format
	NoHeader bool // table/csv 时是否省略表头
}

// Write 把结果按指定格式写到 w。
func Write(w io.Writer, r *driver.Result, opt Options) error {
	switch opt.Format {
	case FormatJSON:
		return writeJSON(w, r)
	case FormatCSV:
		return writeCSV(w, r, opt.NoHeader)
	case FormatYAML:
		return writeYAML(w, r)
	case FormatVertical:
		return writeVertical(w, r)
	default:
		return writeTable(w, r, opt.NoHeader)
	}
}

// writeTable 输出对齐的 ASCII 表格。
// writeTable 输出带边框的对齐表格（+-----+ 风格）。
// 这是比纯空格对齐更易读的格式：每列有明确的边框，行/列清晰。
func writeTable(w io.Writer, r *driver.Result, noHeader bool) error {
	widths := make([]int, len(r.Columns))
	for i, c := range r.Columns {
		widths[i] = displayWidth(c)
	}
	// 转字符串行，同时计算各列最大宽度
	strRows := make([][]string, len(r.Rows))
	for ri, row := range r.Rows {
		strRows[ri] = make([]string, len(row))
		for ci, v := range row {
			s := formatCell(v)
			strRows[ri][ci] = s
			if l := displayWidth(s); l > widths[ci] {
				widths[ci] = l
			}
		}
	}
	var b bytes.Buffer
	// 顶部分隔线
	writeBorder(&b, widths)
	// 表头
	if !noHeader && len(r.Columns) > 0 {
		writeTableRow(&b, r.Columns, widths)
		writeBorder(&b, widths)
	}
	// 数据行
	for _, row := range strRows {
		writeTableRow(&b, row, widths)
	}
	if len(r.Columns) > 0 {
		writeBorder(&b, widths)
	}
	_, err := w.Write(b.Bytes())
	return err
}

// writeBorder 输出一行 +-----+ 分隔线。
func writeBorder(b *bytes.Buffer, widths []int) {
	b.WriteByte('+')
	for _, wv := range widths {
		for j := 0; j < wv+2; j++ {
			b.WriteByte('-')
		}
		b.WriteByte('+')
	}
	b.WriteByte('\n')
}

// writeTableRow 输出一行 | a | b |。
func writeTableRow(b *bytes.Buffer, cells []string, widths []int) {
	b.WriteByte('|')
	for i, c := range cells {
		pad := widths[i] - displayWidth(c)
		if pad < 0 {
			pad = 0
		}
		fmt.Fprintf(b, " %s%s |", c, strings.Repeat(" ", pad))
	}
	b.WriteByte('\n')
}

// writeJSON 输出对象数组的 JSON。
func writeJSON(w io.Writer, r *driver.Result) error {
	out := make([]map[string]any, 0, len(r.Rows))
	for _, row := range r.Rows {
		obj := make(map[string]any, len(r.Columns))
		for i, col := range r.Columns {
			if i < len(row) {
				obj[col] = normalizeJSONValue(row[i])
			}
		}
		out = append(out, obj)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}

// writeYAML 输出对象数组的 YAML。
func writeYAML(w io.Writer, r *driver.Result) error {
	out := make([]map[string]any, 0, len(r.Rows))
	for _, row := range r.Rows {
		obj := make(map[string]any, len(r.Columns))
		for i, col := range r.Columns {
			if i < len(row) {
				obj[col] = normalizeJSONValue(row[i])
			}
		}
		out = append(out, obj)
	}
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(out); err != nil {
		return err
	}
	return enc.Close()
}

// writeCSV 输出 CSV。
func writeCSV(w io.Writer, r *driver.Result, noHeader bool) error {
	cw := csv.NewWriter(w)
	if !noHeader {
		if err := cw.Write(r.Columns); err != nil {
			return err
		}
	}
	for _, row := range r.Rows {
		rec := make([]string, len(row))
		for i, v := range row {
			rec[i] = formatCell(v)
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// writeVertical 输出 \G 风格（每记录一段，列名: 值）。
func writeVertical(w io.Writer, r *driver.Result) error {
	keyWidth := 0
	for _, c := range r.Columns {
		if l := displayWidth(c); l > keyWidth {
			keyWidth = l
		}
	}
	for i, row := range r.Rows {
		fmt.Fprintf(w, "***************************[ %d. row ]***************************\n", i+1)
		for ci, col := range r.Columns {
			val := ""
			if ci < len(row) {
				val = formatCell(row[ci])
			}
			fmt.Fprintf(w, "%s%s | %s\n", col, strings.Repeat(" ", keyWidth-displayWidth(col)), val)
		}
	}
	return nil
}

// formatCell 把任意单元格值转为可显示字符串。
func formatCell(v any) string {
	if v == nil {
		return "NULL"
	}
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	case fmt.Stringer:
		return x.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// normalizeJSONValue 让 []byte 等类型在 JSON 里以字符串表示。
func normalizeJSONValue(v any) any {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case []byte:
		return string(x)
	}
	return v
}

// displayWidth 计算字符串在等宽终端中的显示宽度。
// 对东亚宽字符（CJK 中文/日文/韩文等）按 2 列计算，避免含中文的列边框错位。
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeDisplayWidth(r)
	}
	return w
}

// runeDisplayWidth 返回单个 rune 的显示列宽：宽字符返回 2，其余返回 1。
// 覆盖常见的 CJK 范围（全角字母、CJK 统一表意、CJK 标点、平假名/片假名、谚文等）。
func runeDisplayWidth(r rune) int {
	switch {
	// 全角 ASCII 与 Latin 补充（U+FF01..U+FF60）
	case r >= 0x1100 && r <= 0x115F, // 谚文 Jamo
		r >= 0x2E80 && r <= 0x303E, // CJK 部首/标点
		r >= 0x3041 && r <= 0x33FF, // 平假名/片假名/CJK 符号
		r >= 0x3400 && r <= 0x4DBF, // CJK 扩展 A
		r >= 0x4E00 && r <= 0x9FFF, // CJK 统一表意文字
		r >= 0xA000 && r <= 0xA4CF, // 彝文
		r >= 0xAC00 && r <= 0xD7A3, // 谚文音节
		r >= 0xF900 && r <= 0xFAFF, // CJK 兼容表意
		r >= 0xFE30 && r <= 0xFE4F, // CJK 兼容形式
		r >= 0xFF00 && r <= 0xFF60, // 全角 ASCII
		r >= 0xFFE0 && r <= 0xFFE6: // 全角符号
		return 2
	}
	return 1
}
