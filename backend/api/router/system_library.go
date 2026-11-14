package router

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/coze-dev/coze-studio/backend/api/handler/coze"
	"github.com/coze-dev/coze-studio/backend/api/middleware"
)

func RegisterSystemLibrary(s *server.Hertz) {
	for _, action := range []string{"list", "detail", "plugins"} {
		s.GET("/api/system-library/"+action, coze.SystemLibrary(action, false))
	}
	admin := s.Group("/api/admin/system-library", middleware.AdminAuthMW())
	for _, action := range []string{"list", "detail", "preview"} {
		admin.GET("/"+action, coze.SystemLibrary(action, true))
	}
	for _, action := range []string{"publish", "withdraw"} {
		admin.POST("/"+action, coze.SystemLibrary(action, true))
	}
}
