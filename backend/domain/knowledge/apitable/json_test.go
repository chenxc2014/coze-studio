package apitable

import (
	"context"
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPITableNumbersAndEscaping(t *testing.T) {
	table, err := Parse([]byte(`{"data":{"rows":[{"id":9007199254740993,"名称":"甲,乙\n丙","有效":true},{"id":7706531520575438848,"名称":"丁"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(table.CSV))).ReadAll()
	if err != nil || table.Rows != 2 || !strings.Contains(string(table.CSV), "9007199254740993") || !strings.Contains(string(table.CSV), "7706531520575438848") {
		t.Fatalf("精度或 CSV 转义丢失: %v %s", err, table.CSV)
	}
	for i, key := range table.Columns {
		if key == "名称" && rows[1][i] != "甲,乙\n丙" {
			t.Fatal(rows)
		}
	}
}

func TestAPITableInvalidPayloads(t *testing.T) {
	for _, body := range []string{`[]`, `{}`, `[1,2]`, `[{"a":{}}]`, `[{"a":[]}]`, `[{"":1}]`, `[{"a":1}] []`, `{"data":[],"rows":[]}`, `[{"a":"` + strings.Repeat("x", 65536) + `"}]`, strings.Repeat("x", MaxBytes+1)} {
		if _, err := Parse([]byte(body)); err == nil {
			t.Fatalf("错误数据被接收: %.50s", body)
		}
	}
}

func TestAPITableHTTPDestinations(t *testing.T) {
	for _, value := range []string{"http://192.168.1.21:8000/query/da_custom?page=2&page_size=20", "http://localhost:8080/data", "https://example.com:8443/a", "http://[::1]:8080/data", "https://user:pwd@example.com/a#preview"} {
		if _, err := ValidateURL(value); err != nil {
			t.Fatalf("有效接口被拒绝 %s: %v", value, err)
		}
	}
	for _, value := range []string{"file:///etc/passwd", "ftp://example.com/data", "/relative/path", "http://", "https://example.com:bad/a"} {
		if _, err := ValidateURL(value); err == nil {
			t.Fatal(value)
		}
	}
}

func TestAPITableFetchLocalHTTPAndRedirect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Query().Get("page") != "2" || r.URL.Query().Get("page_size") != "20" || r.Header.Get("Accept") != "application/json" {
			t.Error("抓取时丢失了方法、参数或 JSON 请求头")
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"名称":"内网测试机构","地址":"测试路1号"}]}`))
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/data?page=2&page_size=20", http.StatusFound)
	})
	mux.HandleFunc("/bad", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("<html>不是 JSON</html>")) })
	mux.HandleFunc("/unavailable", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
	server := httptest.NewServer(mux)
	defer server.Close()
	for _, path := range []string{"/data?page=2&page_size=20#preview", "/redirect"} {
		table, err := Fetch(context.Background(), server.URL+path)
		if err != nil || table.Rows != 1 || !strings.Contains(string(table.CSV), "内网测试机构") {
			t.Fatalf("HTTP 自定义端口或重定向抓取失败: %v", err)
		}
	}
	for _, path := range []string{"/bad", "/unavailable"} {
		if _, err := Fetch(context.Background(), server.URL+path); err == nil {
			t.Fatalf("无效响应不应进入表格处理: %s", path)
		}
	}
}

func TestAPITableIntervals(t *testing.T) {
	for _, days := range []int{0, 1, 3, 7, 30} {
		if !ValidInterval(days) {
			t.Fatal(days)
		}
	}
	for _, days := range []int{-1, 2, 24, 31} {
		if ValidInterval(days) {
			t.Fatal(days)
		}
	}
}
