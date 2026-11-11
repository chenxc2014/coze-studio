package router

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/coze-dev/coze-studio/backend/api/handler/coze"
	"github.com/coze-dev/coze-studio/backend/api/middleware"
)

func RegisterPluginCredentials(s *server.Hertz) {
	group := s.Group("/api/admin/plugins", middleware.PluginCredentialAdminMW())
	group.GET("", coze.ListPluginCredentials)
	group.POST("/save", coze.SavePluginCredential)
	group.POST("/test", coze.TestPluginCredential)
}
