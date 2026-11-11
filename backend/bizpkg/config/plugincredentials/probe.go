package plugincredentials

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type TestResult struct {
	Success    bool   `json:"success"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"http_status"`
}

func Probe(ctx context.Context, provider, credential string) TestResult {
	client := &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("不允许跳转") }}
	return probe(ctx, provider, credential, client)
}
func probe(ctx context.Context, provider, credential string, client *http.Client) TestResult {
	result := TestResult{Code: "not_configured", Message: "请先保存该插件的有效授权，再进行测试"}
	if credential == "" {
		return result
	}
	var endpoint string
	switch provider {
	case "amap":
		endpoint = "https://restapi.amap.com/v5/place/around?" + url.Values{"location": {"116.397128,39.916527"}, "radius": {"1000"}, "types": {"090000"}, "key": {credential}}.Encode()
	case "tianyancha":
		endpoint = "https://open.api.tianyancha.com/services/open/mr/abnormal/2.0?" + url.Values{"keyword": {"杭州阿里巴巴网络科技有限公司"}, "pageSize": {"1"}, "pageNum": {"1"}}.Encode()
	default:
		return TestResult{Code: "invalid_provider", Message: "不支持的插件供应商"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return TestResult{Code: "request_failed", Message: "无法构造测试请求"}
	}
	if provider == "tianyancha" {
		req.Header.Set("Authorization", credential)
	}
	req.Header.Set("User-Agent", "Coze/1.0")
	response, err := client.Do(req)
	if err != nil {
		return TestResult{Code: "connection_failed", Message: "供应商连接失败或超时，请检查网络后重试"}
	}
	defer response.Body.Close()
	result.HTTPStatus = response.StatusCode
	if response.StatusCode != 200 {
		result.Code = "http_" + strconv.Itoa(response.StatusCode)
		result.Message = "供应商返回非成功 HTTP 状态，请核对授权及接口权限"
		return result
	}
	var body map[string]any
	decoder := json.NewDecoder(io.LimitReader(response.Body, 256*1024))
	decoder.UseNumber()
	if err = decoder.Decode(&body); err != nil {
		result.Code = "invalid_response"
		result.Message = "供应商响应无法解析，未将原始响应返回页面"
		return result
	}
	if provider == "amap" {
		result.Code = code(body["infocode"])
		result.Success = code(body["status"]) == "1" && result.Code == "10000"
		if result.Success {
			result.Message = "测试通过，供应商已成功处理周边搜索请求"
			return result
		}
		messages := map[string]string{"10001": "Key 无效或尚未配置，请使用有效的 Web 服务 Key", "10002": "当前 Key 没有所需服务权限", "10003": "调用量已超出限制", "10004": "接口访问过于频繁", "10005": "服务器出口 IP 不在供应商白名单内", "10007": "请求签名无效，请核对供应商安全设置", "10009": "Key 与服务平台类型不匹配", "10010": "IP 访问限制导致请求失败", "10044": "服务配额或相关许可不可用，请在供应商控制台核对"}
		result.Message = messages[result.Code]
	} else {
		result.Code = code(body["error_code"])
		result.Success = result.Code == "0"
		if result.Success {
			result.Message = "测试通过，供应商已成功处理经营异常查询请求"
			return result
		}
		messages := map[string]string{"300000": "接口已响应，但本次查询无匹配结果；请继续确认所需业务接口权限", "300002": "供应商账号已失效", "300003": "供应商账号已过期", "300004": "调用频率过快", "300005": "当前 Token 没有该接口的访问权限", "300006": "供应商账号余额不足", "300007": "供应商接口剩余次数不足", "300008": "缺少必要参数，请核对授权 Token 及接口配置", "300009": "供应商账号或授权信息有误", "300011": "服务器出口 IP 没有访问该接口的权限"}
		result.Message = messages[result.Code]
	}
	if result.Message == "" {
		result.Message = "供应商拒绝请求，请核对授权、接口权限及可用额度"
	}
	return result
}
func code(value any) string {
	var text string
	switch v := value.(type) {
	case string:
		text = v
	case json.Number:
		text = v.String()
	default:
		return "unknown"
	}
	if len(text) == 0 || len(text) > 32 {
		return "unknown"
	}
	for _, r := range text {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return "unknown"
		}
	}
	return text
}
