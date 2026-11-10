package coze

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestAccountAdminWriteOrigin(t *testing.T) {
	for _, test := range []struct {
		name, origin, contentType, body string
		status                          int
	}{
		{"同源 JSON", "http://coze.test:8888", "application/json", `{"name":"测试"}`, 200},
		{"缺少来源", "", "application/json", `{}`, 403},
		{"跨站来源", "http://other.test:8888", "application/json", `{}`, 403},
		{"不同端口", "http://coze.test:9999", "application/json", `{}`, 403},
		{"表单提交", "http://coze.test:8888", "application/x-www-form-urlencoded", `name=test`, 400},
		{"错误 JSON", "http://coze.test:8888", "application/json", `{`, 400},
		{"超大请求", "http://coze.test:8888", "application/json", strings.Repeat(" ", 16385), 400},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := server.New()
			h.POST("/test", func(ctx context.Context, c *app.RequestContext) {
				var input map[string]any
				if bindAccountAdmin(c, &input) {
					c.JSON(200, map[string]bool{"ok": true})
				}
			})
			response := ut.PerformRequest(h.Engine, "POST", "http://coze.test:8888/test", &ut.Body{Body: bytes.NewBufferString(test.body), Len: len(test.body)}, ut.Header{Key: "Origin", Value: test.origin}, ut.Header{Key: "Content-Type", Value: test.contentType})
			if response.Result().StatusCode() != test.status {
				t.Fatalf("状态应为 %d，实际为 %d", test.status, response.Result().StatusCode())
			}
		})
	}
}
