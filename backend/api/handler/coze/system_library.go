package coze

import (
	"context"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	"github.com/coze-dev/coze-studio/backend/application/plugin"
	"github.com/coze-dev/coze-studio/backend/application/systemlibrary"
	"github.com/coze-dev/coze-studio/backend/domain/systemresource"
)

func SystemLibrary(action string, admin bool) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		uid := ctxutil.GetUIDFromCtx(ctx)
		if uid == nil {
			pluginCredentialReply(c, 401, nil, "请先登录平台")
			return
		}
		s := systemlibrary.SVC
		if s == nil {
			pluginCredentialReply(c, 503, nil, "系统资源服务尚未就绪")
			return
		}
		if admin {
			allowed, err := s.Registry.IsAdmin(ctx, *uid)
			if err != nil || !allowed {
				pluginCredentialReply(c, 403, nil, "只有管理员可以维护系统资源")
				return
			}
		}
		var input struct {
			Kind           systemresource.Kind `json:"kind"`
			ID             string              `json:"id"`
			Revision       int64               `json:"revision"`
			Confirmed      bool                `json:"confirmed"`
			ReadyConfirmed bool                `json:"ready_confirmed"`
		}
		if action == "publish" || action == "withdraw" {
			if !bindAccountAdmin(c, &input) {
				return
			}
		} else {
			input.Kind = systemresource.Kind(c.Query("kind"))
			input.ID = c.Query("id")
		}
		if action == "plugins" {
			result, err := plugin.PluginApplicationSVC.SystemPluginProducts(ctx, c.Query("q"), c.Query("page"))
			if err != nil {
				pluginCredentialReply(c, 503, nil, "暂时无法读取系统插件")
				return
			}
			pluginCredentialReply(c, 200, result, "")
			return
		}
		if action == "list" {
			if input.Kind != "" && input.Kind != "all" && !systemresource.ValidKind(input.Kind) {
				pluginCredentialReply(c, 400, nil, "资源类型不正确")
				return
			}
			query := strings.TrimSpace(c.Query("q"))
			if len([]rune(query)) > 100 {
				pluginCredentialReply(c, 400, nil, "搜索内容最多 100 字")
				return
			}
			items, err := s.List(ctx, admin)
			if err != nil {
				pluginCredentialReply(c, 503, nil, "暂时无法读取系统资源")
				return
			}
			page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
			size, _ := strconv.Atoi(c.DefaultQuery("page_size", "15"))
			if page < 1 || page > 1000000 {
				page = 1
			}
			rows, total, counts := systemlibrary.Filter(items, string(input.Kind), query, c.Query("status"), page, size)
			pluginCredentialReply(c, 200, map[string]any{"resources": rows, "total": total, "counts": counts, "publishing_ready": s.Registry.Ready}, "")
			return
		}
		if !systemresource.ValidKind(input.Kind) {
			pluginCredentialReply(c, 400, nil, "资源类型不正确")
			return
		}
		id, err := systemlibrary.ParseID(input.ID)
		if err != nil {
			pluginCredentialReply(c, 400, nil, err.Error())
			return
		}
		if action == "preview" {
			r, _, owner, e := s.Describe(ctx, input.Kind, id)
			if e != nil || r == nil {
				pluginCredentialReply(c, 400, nil, "源资源不可用，请确认资源已发布且 ID 正确")
				return
			}
			if r.Origin != "builtin" && owner != *uid {
				pluginCredentialReply(c, 403, nil, "只能发布当前管理员自己维护的资源")
				return
			}
			p, e := s.Registry.Lookup(ctx, input.Kind, r.ID)
			if e != nil {
				pluginCredentialReply(c, 503, nil, "无法读取发布状态")
				return
			}
			if p != nil {
				r.Revision = p.Revision
			}
			pluginCredentialReply(c, 200, r, "")
			return
		}
		var resource *systemresource.Resource
		switch action {
		case "detail":
			resource, err = s.Detail(ctx, input.Kind, id, admin)
		case "publish":
			resource, err = s.Publish(ctx, *uid, input.Kind, id, input.Revision, input.Confirmed, input.ReadyConfirmed)
		case "withdraw":
			resource, err = s.Withdraw(ctx, *uid, input.Kind, id, input.Revision)
		default:
			pluginCredentialReply(c, 404, nil, "操作不存在")
			return
		}
		if err != nil {
			if action == "detail" {
				pluginCredentialReply(c, 404, nil, "系统资源不存在或已下架")
			} else {
				pluginCredentialReply(c, 400, nil, err.Error())
			}
			return
		}
		pluginCredentialReply(c, 200, resource, "")
	}
}
