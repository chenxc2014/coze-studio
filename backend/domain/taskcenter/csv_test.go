package taskcenter

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func testCSV(text string) string { return base64.StdEncoding.EncodeToString([]byte(text)) }

func TestTaskCenterCSVTypesAndPrecision(t *testing.T) {
	fields := []Field{{Name: "地址", Type: "string", Required: true}, {Name: "编号", Type: "integer", Required: true}, {Name: "启用", Type: "boolean"}, {Name: "条件", Type: "object", Schema: json.RawMessage(`[{"name":"区县","type":"string","required":true}]`)}, {Name: "列表", Type: "list", Schema: json.RawMessage(`{"type":"integer"}`)}}
	text := "\ufeff地址,编号,启用,条件,列表\r\n\"杭州,某街道\",9223372036854775807,true,\"{\"\"区县\"\":\"\"测试区\"\"}\",\"[1,2]\"\r\n"
	rows, err := ParseCSV(fields, testCSV(text))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["编号"] != int64(9223372036854775807) || rows[0]["启用"] != true || rows[0]["地址"] != "杭州,某街道" {
		t.Fatal("CSV 类型、引号或整数精度错误")
	}
	encoded, _ := json.Marshal(rows[0])
	restored, err := InputFromJSON(fields, string(encoded))
	if err != nil || restored["编号"] != int64(9223372036854775807) {
		t.Fatal("持久化后整数精度丢失")
	}
}

func TestTaskCenterCSVValidation(t *testing.T) {
	fields := []Field{{Name: "name", Type: "string", Required: true}, {Name: "n", Type: "integer"}, {Name: "b", Type: "boolean"}}
	for _, text := range []string{"name,name\na,b\n", "unknown\na\n", "n\n1\n", "name,n\na,1.2\n", "name,b\na,maybe\n", "name\n", "name,n\na,1,2\n", "name\n\"\"\n"} {
		if _, err := ParseCSV(fields, testCSV(text)); err == nil {
			t.Fatalf("未拒绝错误 CSV：%q", text)
		}
	}
	if _, err := ParseCSV(fields, testCSV("name\n"+strings.Repeat("a\n", 1001))); err == nil {
		t.Fatal("未限制 1000 行")
	}
	if _, err := ParseCSV(fields, testCSV(strings.Repeat("x", MaxCSVBytes+1))); err == nil {
		t.Fatal("未限制文件大小")
	}
	if _, err := ParseCSV(nil, testCSV("x\na\n")); err == nil {
		t.Fatal("无参数工作流不应创建 CSV 批量任务")
	}
}

func TestTaskCenterJSONDefaultsAndNestedTypes(t *testing.T) {
	fields := []Field{{Name: "count", Type: "integer", Required: true, DefaultValue: json.RawMessage(`9223372036854775807`)}, {Name: "array", Type: "list", Schema: json.RawMessage(`{"type":"boolean"}`)}}
	input, err := InputFromJSON(fields, `{"array":[true,false]}`)
	if err != nil || input["count"] != int64(9223372036854775807) {
		t.Fatal("默认值未保留")
	}
	for _, text := range []string{`{"count":1.5}`, `{"array":{}}`, `{"array":["wrong"]}`, `{"extra":1}`, `{} {}`, `[]`, `{"count":9223372036854775808}`} {
		if _, err = InputFromJSON(fields, text); err == nil {
			t.Fatalf("未拒绝错误参数：%s", text)
		}
	}
	if _, err = InputFromJSON([]Field{{Name: "obj", Type: "object"}}, `{"obj":{"n":1e999}}`); err == nil {
		t.Fatal("不得保存无穷大数值")
	}
}

func TestTaskCenterChineseEncodingAndExport(t *testing.T) {
	fields := []Field{{Name: "地址", Type: "string", Required: true}}
	raw, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte("地址\r\n示例区测试路1号\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ParseCSV(fields, base64.StdEncoding.EncodeToString(raw))
	if err != nil || rows[0]["地址"] != "示例区测试路1号" {
		t.Fatal("中文编码转换失败")
	}
	result, err := ResultsCSV(fields, []*Item{{Position: 1, Status: Succeeded, InputJSON: `{"地址":"=SUM(1,2)"}`, OutputJSON: "@formula", Error: " -1+2", ExecutionID: 9223372036854775807}})
	if err != nil || !strings.HasPrefix(string(result), "\ufeff") || !strings.Contains(string(result), "'=SUM") || !strings.Contains(string(result), "'@formula") || !strings.Contains(string(result), "9223372036854775807") {
		t.Fatal("导出编码、公式保护或执行 ID 精度错误")
	}
	template := TemplateCSV(fields)
	if !strings.Contains(string(template), "地址") {
		t.Fatal("模板表头缺失")
	}
}

func TestTaskCenterRunSummary(t *testing.T) {
	tests := []struct {
		counts        map[string]int
		current, want string
	}{{map[string]int{Queued: 2}, Queued, Running}, {map[string]int{Running: 1}, Cancelling, Cancelling}, {map[string]int{Succeeded: 2}, Running, "completed"}, {map[string]int{Succeeded: 1, Failed: 1}, Running, "partial"}, {map[string]int{Failed: 1}, Running, Failed}, {map[string]int{Interrupted: 1, Succeeded: 1}, Running, Interrupted}, {map[string]int{Cancelled: 1, Succeeded: 1}, Cancelling, Cancelled}}
	for _, test := range tests {
		if got := summarize(test.counts, test.current); got != test.want {
			t.Fatalf("状态汇总错误：%s != %s", got, test.want)
		}
	}
}
