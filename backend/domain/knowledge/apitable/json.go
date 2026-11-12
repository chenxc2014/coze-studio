// Package apitable 负责外部 JSON 表格的抓取和规范化，不依赖业务存储。
package apitable

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxBytes = 8 << 20
const MaxRows = 50000
const MaxColumns = 100

type Table struct {
	Columns []string
	Rows    int
	CSV     []byte
}

func ValidInterval(days int) bool {
	return days == 0 || days == 1 || days == 3 || days == 7 || days == 30
}

func ValidateURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || len(raw) > 2048 {
		return nil, errors.New("请输入有效的 HTTP 或 HTTPS 接口地址")
	}
	// 片段不是 HTTP 请求的一部分，访问接口时直接忽略。
	u.Fragment = ""
	u.RawFragment = ""
	return u, nil
}

func Fetch(ctx context.Context, raw string) (*Table, error) {
	u, err := ValidateURL(raw)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{Proxy: nil, TLSHandshakeTimeout: 8 * time.Second, ResponseHeaderTimeout: 15 * time.Second,
		// 按平台配置的网络访问接口，支持内网地址、HTTP 和自定义端口。
		DialContext: (&net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}).DialContext}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("接口重定向次数过多")
		}
		_, err := ValidateURL(req.URL.String())
		return err
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errors.New("接口地址不正确")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("接口访问失败，请检查地址、端口、网络及 HTTPS 证书")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("接口返回 HTTP %d，需要返回 200 JSON 数据", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBytes+1))
	if err != nil {
		return nil, errors.New("读取接口响应失败")
	}
	if len(body) > MaxBytes {
		return nil, errors.New("接口响应超过 8 MB 上限")
	}
	return Parse(body)
}

// 支持对象数组，以及常见的 data、rows、items、list 包装；数字全程保留原始精度。
func Parse(body []byte) (*Table, error) {
	if len(body) > MaxBytes || !utf8.Valid(body) {
		return nil, errors.New("接口数据必须是 8 MB 以内的 UTF-8 JSON")
	}
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf})))
	dec.UseNumber()
	var value any
	if dec.Decode(&value) != nil {
		return nil, errors.New("接口响应不是有效 JSON")
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return nil, errors.New("接口响应含有多段 JSON 或尾部无效内容")
	}
	for depth := 0; depth < 3; depth++ {
		obj, ok := value.(map[string]any)
		if !ok {
			break
		}
		var candidates []any
		for _, key := range []string{"data", "rows", "items", "list"} {
			if v, ok := obj[key]; ok {
				switch v.(type) {
				case []any, map[string]any:
					candidates = append(candidates, v)
				}
			}
		}
		if len(candidates) != 1 {
			return nil, errors.New("JSON 包装中需要唯一的 data、rows、items 或 list 数据字段")
		}
		value = candidates[0]
	}
	rows, ok := value.([]any)
	if !ok || len(rows) == 0 {
		return nil, errors.New("接口需要返回非空的对象数组，例如 [{\"名称\":\"示例机构\",\"地址\":\"示例路1号\"}]")
	}
	if len(rows) > MaxRows {
		return nil, errors.New("接口数据超过 50000 行上限")
	}
	keys := map[string]bool{}
	for _, r := range rows {
		obj, ok := r.(map[string]any)
		if !ok || len(obj) == 0 {
			return nil, errors.New("数组的每一行都必须是非空 JSON 对象")
		}
		for key, v := range obj {
			if strings.TrimSpace(key) == "" || utf8.RuneCountInString(key) > 100 {
				return nil, errors.New("字段名称不能为空或超过 100 字")
			}
			switch v.(type) {
			case nil, string, bool, json.Number:
			default:
				return nil, fmt.Errorf("字段 %s 含嵌套对象或数组，请先将接口整理为平面表格", key)
			}
			keys[key] = true
		}
		if len(keys) > MaxColumns {
			return nil, errors.New("接口数据超过 100 列上限")
		}
	}
	columns := make([]string, 0, len(keys))
	for key := range keys {
		columns = append(columns, key)
	}
	sort.Strings(columns)
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(columns); err != nil {
		return nil, err
	}
	for _, r := range rows {
		obj := r.(map[string]any)
		record := make([]string, len(columns))
		for i, key := range columns {
			switch v := obj[key].(type) {
			case string:
				record[i] = v
			case json.Number:
				record[i] = v.String()
			case bool:
				record[i] = strconv.FormatBool(v)
			}
			if len(record[i]) > 65535 {
				return nil, fmt.Errorf("字段 %s 单个值超过 64 KB", key)
			}
		}
		if err := w.Write(record); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	if buf.Len() > MaxBytes {
		return nil, errors.New("转换后表格超过 8 MB 上限")
	}
	return &Table{Columns: columns, Rows: len(rows), CSV: buf.Bytes()}, nil
}
