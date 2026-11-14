package router

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/coze-dev/coze-studio/backend/api/handler/coze"
)

func RegisterTaskCenter(s *server.Hertz) {
	for _, action := range []string{"workflows", "schema", "template", "list", "detail", "export"} {
		s.GET("/api/task-center/"+action, coze.TaskCenter(action))
	}
	for _, action := range []string{"create", "execute", "preview", "cancel", "retry", "delete"} {
		s.POST("/api/task-center/"+action, coze.TaskCenter(action))
	}
}
