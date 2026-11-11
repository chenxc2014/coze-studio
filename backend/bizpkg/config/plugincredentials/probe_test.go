package plugincredentials

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type pluginProbeTransport func(*http.Request) (*http.Response, error)

func (f pluginProbeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestPluginCredentialProbeAuthenticationAndBusinessErrors(t *testing.T) {
	for _, provider := range []string{"amap", "tianyancha"} {
		t.Run(provider, func(t *testing.T) {
			const secret = "synthetic-test-key-12345678"
			client := &http.Client{Transport: pluginProbeTransport(func(req *http.Request) (*http.Response, error) {
				body := `{"error_code":300005,"reason":"should not expose ` + secret + `"}`
				if provider == "amap" {
					if req.URL.Host != "restapi.amap.com" || req.URL.Query().Get("key") != secret {
						t.Fatal("高德认证参数不正确")
					}
					body = `{"status":"0","info":"INVALID_USER_KEY","infocode":"10001"}`
				} else if req.URL.Host != "open.api.tianyancha.com" || req.Header.Get("Authorization") != secret {
					t.Fatal("天眼查认证请求头不正确")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
			})}
			result := probe(context.Background(), provider, secret, client)
			if result.Success || strings.Contains(result.Message, secret) {
				t.Fatal("未正确识别供应商错误或泄露了响应中的密钥")
			}
			if provider == "amap" && result.Code != "10001" || provider == "tianyancha" && result.Code != "300005" {
				t.Fatal("业务错误码未保留")
			}
		})
	}
}
func TestPluginCredentialProbeSuccessAndSanitizedNetworkFailure(t *testing.T) {
	success := &http.Client{Transport: pluginProbeTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"status":"1","infocode":"10000"}`)), Header: http.Header{}}, nil
	})}
	if result := probe(context.Background(), "amap", "test-key-12345678", success); !result.Success {
		t.Fatal(result)
	}
	failed := &http.Client{Transport: pluginProbeTransport(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network error includes key=test-key-12345678")
	})}
	result := probe(context.Background(), "amap", "test-key-12345678", failed)
	if result.Success || strings.Contains(result.Message, "test-key") {
		t.Fatal("网络错误不应将带密钥的 URL 回传")
	}
}
