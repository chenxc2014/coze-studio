package router

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/coze-dev/coze-studio/backend/api/handler/coze"
)

// API 导入沿用平台登录态，写请求另作同源 JSON 校验。
func RegisterKnowledgeAPITable(s *server.Hertz) {
	for _, action := range []string{"prepare", "source", "refresh", "schedule"} {
		s.POST("/api/knowledge/table-api/"+action, coze.KnowledgeAPITable(action))
	}
}
