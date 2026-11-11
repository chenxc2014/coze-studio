package middleware

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/coze-dev/coze-studio/backend/bizpkg/config"
	"github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	"github.com/coze-dev/coze-studio/backend/types/consts"
	"strings"
)

// 插件密钥只允许明确配置的管理员维护；管理员列表为空时也不能放行。
func PluginCredentialAdminMW() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		session, ok := ctxcache.Get[*entity.Session](ctx, consts.SessionDataKeyInCtx)
		if !ok || session == nil {
			c.AbortWithStatusJSON(401, map[string]any{"code": 401, "message": "请先登录平台"})
			return
		}
		base, err := config.Base().GetBaseConfig(ctx)
		if err != nil {
			c.AbortWithStatusJSON(500, map[string]any{"code": 500, "message": "无法核对管理员权限"})
			return
		}
		for _, email := range strings.Split(base.AdminEmails, ",") {
			if strings.TrimSpace(email) != "" && strings.EqualFold(strings.TrimSpace(email), session.UserEmail) {
				c.Next(ctx)
				return
			}
		}
		c.AbortWithStatusJSON(403, map[string]any{"code": 403, "message": "只有平台管理员可以维护插件授权"})
	}
}
