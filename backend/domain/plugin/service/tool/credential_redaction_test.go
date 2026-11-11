package tool

import (
	"net/http"
	"strings"
	"testing"
)

func TestPluginCredentialRedactionPreservesRealRequest(t *testing.T) {
	request, _ := http.NewRequest(http.MethodGet, "https://restapi.amap.com/v5/place/around?key=test-key-12345678&location=116,39", nil)
	request.Header.Set("Authorization", "test-token-987654321")
	encoded, err := genRequestString(request, nil)
	if err != nil || strings.Contains(encoded, "test-key") || strings.Contains(encoded, "test-token") {
		t.Fatal("诊断记录不应包含真实密钥")
	}
	if request.URL.Query().Get("key") != "test-key-12345678" || request.Header.Get("Authorization") != "test-token-987654321" {
		t.Fatal("脱敏不能修改实际请求")
	}
	if !strings.Contains(encoded, "116,39") {
		t.Fatal("普通查询条件应保留，便于排障")
	}
	errorText := redactPluginSecrets("Get key=test-key-12345678 header=test-token-987654321 failed", request)
	if strings.Contains(errorText, "test-key") || strings.Contains(errorText, "test-token") {
		t.Fatal("错误内容中的密钥未脱敏")
	}
}
