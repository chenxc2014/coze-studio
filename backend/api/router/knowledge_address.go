package router

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/coze-dev/coze-studio/backend/api/handler/coze"
)

// 使用扩展注册入口，避免修改 IDL 生成的路由文件。
func RegisterKnowledgeAddress(s *server.Hertz) {
	s.POST("/api/knowledge/address/query", coze.QueryKnowledgeAddress)
}
