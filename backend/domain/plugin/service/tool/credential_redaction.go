package tool

import (
	"net/http"
	"net/url"
	"strings"
)

func sensitiveCredentialField(name string) bool {
	name = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(name, "-", ""), "_", ""))
	return name == "key" || name == "authorization" || name == "cookie" || strings.Contains(name, "token") || strings.Contains(name, "apikey") || strings.Contains(name, "secret") || strings.Contains(name, "password")
}

// 仅清理诊断副本；真正发往供应商的请求仍保留认证信息。
func redactedPluginRequest(req *http.Request) *http.Request {
	copied := req.Clone(req.Context())
	endpoint := *req.URL
	copied.URL = &endpoint
	query := copied.URL.Query()
	for key := range query {
		if sensitiveCredentialField(key) {
			query.Set(key, "[已隐藏]")
		}
	}
	copied.URL.RawQuery = query.Encode()
	for key := range copied.Header {
		if sensitiveCredentialField(key) {
			copied.Header.Set(key, "[已隐藏]")
		}
	}
	return copied
}
func redactPluginSecrets(text string, req *http.Request) string {
	values := []string{}
	for key, items := range req.Header {
		if sensitiveCredentialField(key) {
			values = append(values, items...)
		}
	}
	for key, items := range req.URL.Query() {
		if sensitiveCredentialField(key) {
			values = append(values, items...)
		}
	}
	for _, value := range values {
		candidates := []string{value, strings.TrimPrefix(value, "Bearer ")}
		for _, candidate := range candidates {
			if len(candidate) < 8 {
				continue
			}
			text = strings.ReplaceAll(text, candidate, "[已隐藏]")
			text = strings.ReplaceAll(text, url.QueryEscape(candidate), "[已隐藏]")
		}
	}
	return text
}
