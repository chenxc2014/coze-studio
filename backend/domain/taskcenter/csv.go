package taskcenter

import (
	"bytes"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func decodeJSON(text string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, invalid("JSON 格式不正确")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, invalid("JSON 只能包含一个值")
	}
	return value, nil
}

func NormalizeInput(fields []Field, input map[string]any) (map[string]any, error) {
	known := make(map[string]bool, len(fields))
	result := make(map[string]any, len(fields))
	for _, field := range fields {
		known[field.Name] = true
		value, present := input[field.Name]
		if !present || value == nil {
			if len(field.DefaultValue) > 0 && string(field.DefaultValue) != "null" {
				var err error
				value, err = decodeJSON(string(field.DefaultValue))
				if err != nil {
					return nil, err
				}
				present = true
			}
		}
		if !present || value == nil {
			if field.Required {
				return nil, invalid("参数 %s 不能为空", field.Name)
			}
			continue
		}
		v, err := normalize(field, value)
		if err != nil {
			return nil, invalid("参数 %s：%s", field.Name, err.Error())
		}
		result[field.Name] = v
	}
	for key := range input {
		if !known[key] {
			return nil, invalid("存在未知参数 %s", key)
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, invalid("参数包含无效或超出范围的数值")
	}
	if len(encoded) > MaxInputBytes {
		return nil, invalid("单条参数超过 16 KiB")
	}
	return result, nil
}

func normalize(field Field, value any) (any, error) {
	text, ok := value.(string)
	switch field.Type {
	case "string":
		if !ok {
			return nil, invalid("应为字符串")
		}
		if field.Required && strings.TrimSpace(text) == "" {
			return nil, invalid("不能为空")
		}
		return text, nil
	case "integer":
		switch v := value.(type) {
		case int64:
			return v, nil
		case json.Number:
			text = string(v)
			ok = true
		case float64:
			if math.IsNaN(v) || math.IsInf(v, 0) || v != math.Trunc(v) || v >= 9223372036854775808 || v < -9223372036854775808 {
				return nil, invalid("应为 64 位整数")
			}
			return int64(v), nil
		}
		if ok {
			n, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
			if err == nil {
				return n, nil
			}
		}
		return nil, invalid("应为 64 位整数")
	case "float", "number":
		if n, yes := value.(json.Number); yes {
			text = string(n)
			ok = true
		}
		if n, yes := value.(int64); yes {
			return float64(n), nil
		}
		if n, yes := value.(float64); yes {
			if !math.IsNaN(n) && !math.IsInf(n, 0) {
				return n, nil
			}
		}
		if ok {
			n, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
			if err == nil && !math.IsNaN(n) && !math.IsInf(n, 0) {
				return n, nil
			}
		}
		return nil, invalid("应为有限数值")
	case "boolean":
		if v, yes := value.(bool); yes {
			return v, nil
		}
		if ok {
			switch strings.ToLower(strings.TrimSpace(text)) {
			case "true":
				return true, nil
			case "false":
				return false, nil
			}
		}
		return nil, invalid("应为 true 或 false")
	case "object", "list", "array":
		if ok {
			var err error
			value, err = decodeJSON(text)
			if err != nil {
				return nil, err
			}
		}
		if field.Type == "object" {
			object, yes := value.(map[string]any)
			if !yes {
				return nil, invalid("应为 JSON 对象")
			}
			if len(field.Schema) > 0 && field.Schema[0] == '[' {
				var children []Field
				if json.Unmarshal(field.Schema, &children) != nil {
					return nil, invalid("对象结构配置不正确")
				}
				return NormalizeInput(children, object)
			}
			return normalizeJSONNumbers(object), nil
		}
		list, yes := value.([]any)
		if !yes {
			return nil, invalid("应为 JSON 数组")
		}
		if len(field.Schema) > 0 && field.Schema[0] == '{' {
			var element Field
			if json.Unmarshal(field.Schema, &element) != nil {
				return nil, invalid("数组结构配置不正确")
			}
			for i, v := range list {
				n, err := normalize(element, v)
				if err != nil {
					return nil, invalid("数组第 %d 项：%s", i+1, err.Error())
				}
				list[i] = n
			}
		}
		return normalizeJSONNumbers(list), nil
	default:
		return nil, invalid("暂不支持类型 %s", field.Type)
	}
}

func normalizeJSONNumbers(value any) any {
	switch v := value.(type) {
	case json.Number:
		if n, e := v.Int64(); e == nil {
			return n
		}
		n, _ := v.Float64()
		return n
	case map[string]any:
		for k, x := range v {
			v[k] = normalizeJSONNumbers(x)
		}
	case []any:
		for i, x := range v {
			v[i] = normalizeJSONNumbers(x)
		}
	}
	return value
}

func InputFromJSON(fields []Field, text string) (map[string]any, error) {
	if len(text) > MaxInputBytes {
		return nil, invalid("单条参数超过 16 KiB")
	}
	value, err := decodeJSON(text)
	if err != nil {
		return nil, err
	}
	input, ok := value.(map[string]any)
	if !ok {
		return nil, invalid("输入参数必须为 JSON 对象")
	}
	return NormalizeInput(fields, input)
}

func ParseCSV(fields []Field, encoded string) ([]map[string]any, error) {
	if len(fields) == 0 {
		return nil, invalid("没有输入参数的工作流不支持 CSV 批量任务，请使用异步任务")
	}
	if len(encoded) > base64.StdEncoding.EncodedLen(MaxCSVBytes) {
		return nil, invalid("CSV 文件不能超过 2 MiB")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) == 0 || len(data) > MaxCSVBytes {
		return nil, invalid("CSV 文件内容或大小不正确")
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if !utf8.Valid(data) {
		data, _, err = transform.Bytes(simplifiedchinese.GB18030.NewDecoder(), data)
		if err != nil || !utf8.Valid(data) {
			return nil, invalid("CSV 应使用 UTF-8 或 GB18030 编码")
		}
	}
	reader := csv.NewReader(bytes.NewReader(data))
	header, err := reader.Read()
	if err != nil {
		return nil, invalid("无法读取 CSV 表头")
	}
	known := map[string]Field{}
	for _, field := range fields {
		known[field.Name] = field
	}
	seen := map[string]bool{}
	for i, name := range header {
		name = strings.TrimSpace(name)
		header[i] = name
		if _, ok := known[name]; !ok {
			return nil, invalid("CSV 列 %s 不是工作流参数", name)
		}
		if seen[name] {
			return nil, invalid("CSV 列 %s 重复", name)
		}
		seen[name] = true
	}
	for _, field := range fields {
		if field.Required && !seen[field.Name] && len(field.DefaultValue) == 0 {
			return nil, invalid("CSV 缺少必填列 %s", field.Name)
		}
	}
	rows := make([]map[string]any, 0)
	for number := 2; ; number++ {
		record, e := reader.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, invalid("CSV 第 %d 行列数或引号不正确", number)
		}
		if len(rows) >= MaxRows {
			return nil, invalid("每次批量任务最多 1000 行")
		}
		input := map[string]any{}
		empty := true
		for i, cell := range record {
			if strings.TrimSpace(cell) != "" {
				empty = false
				input[header[i]] = cell
			}
		}
		if empty {
			return nil, invalid("CSV 第 %d 行为空", number)
		}
		row, e := NormalizeInput(fields, input)
		if e != nil {
			return nil, invalid("CSV 第 %d 行：%s", number, e.Error())
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, invalid("CSV 至少需要一行数据")
	}
	return rows, nil
}

func safeCell(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if len(trimmed) > 0 && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}

func TemplateCSV(fields []Field) []byte {
	buffer := bytes.NewBuffer([]byte{0xef, 0xbb, 0xbf})
	writer := csv.NewWriter(buffer)
	names := make([]string, len(fields))
	for i, f := range fields {
		names[i] = safeCell(f.Name)
	}
	_ = writer.Write(names)
	writer.Flush()
	return buffer.Bytes()
}

func ResultsCSV(fields []Field, items []*Item) ([]byte, error) {
	buffer := bytes.NewBuffer([]byte{0xef, 0xbb, 0xbf})
	writer := csv.NewWriter(buffer)
	header := []string{"行号", "状态"}
	for _, f := range fields {
		header = append(header, "输入："+f.Name)
	}
	header = append(header, "执行结果", "错误信息", "执行ID")
	_ = writer.Write(header)
	for _, item := range items {
		input := map[string]json.RawMessage{}
		_ = json.Unmarshal([]byte(item.InputJSON), &input)
		row := []string{strconv.Itoa(item.Position), item.Status}
		for _, field := range fields {
			raw := input[field.Name]
			var text string
			if json.Unmarshal(raw, &text) != nil {
				text = string(raw)
			}
			row = append(row, safeCell(text))
		}
		row = append(row, safeCell(item.OutputJSON), safeCell(item.Error), strconv.FormatInt(item.ExecutionID, 10))
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	return buffer.Bytes(), writer.Error()
}
