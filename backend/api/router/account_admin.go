package router

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/coze-dev/coze-studio/backend/api/handler/coze"
	"github.com/coze-dev/coze-studio/backend/api/middleware"
)

// 独立注册扩展路由，避免 IDL 重新生成时覆盖定制代码。
func RegisterAccountAdministration(server *server.Hertz) {
	group := server.Group("/api/admin/accounts", middleware.AdminAuthMW())
	group.GET("", coze.ListManagedAccounts)
	group.POST("/create", coze.CreateManagedAccount)
	for _, action := range []string{"update", "status", "reset-password", "role"} {
		group.POST("/"+action, coze.ChangeManagedAccount(action))
	}
}
