package coze

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	"github.com/coze-dev/coze-studio/backend/application/knowledge"
	"github.com/coze-dev/coze-studio/backend/domain/knowledge/service"
	"strconv"
	"time"
)

func apiTableReply(c *app.RequestContext, data any, err error) {
	c.Header("Cache-Control", "no-store")
	if err != nil {
		c.JSON(400, map[string]any{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(200, map[string]any{"code": 0, "data": data})
}

func KnowledgeAPITable(action string) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		owner := ctxutil.GetUIDFromCtx(ctx)
		if owner == nil {
			c.JSON(401, map[string]any{"code": 401, "message": "请先登录平台"})
			return
		}
		var input struct {
			URL          string `json:"url"`
			IntervalDays int    `json:"interval_days"`
			DocumentID   string `json:"document_id"`
		}
		if !bindAccountAdmin(c, &input) {
			return
		}
		api, ok := knowledge.KnowledgeSVC.DomainSVC.(service.APITableService)
		if !ok {
			c.JSON(503, map[string]any{"code": 503, "message": "API 导入服务未就绪"})
			return
		}
		ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
		defer cancel()
		if action == "prepare" {
			p, err := api.PrepareAPITable(ctx, *owner, input.URL, input.IntervalDays)
			apiTableReply(c, p, err)
			return
		}
		id, err := strconv.ParseInt(input.DocumentID, 10, 64)
		if err != nil || id <= 0 {
			c.JSON(400, map[string]any{"code": 400, "message": "文档 ID 不正确"})
			return
		}
		if action == "source" {
			p, err := api.GetAPITableSource(ctx, *owner, id)
			apiTableReply(c, p, err)
			return
		}
		var interval *int
		if action == "schedule" {
			interval = &input.IntervalDays
		}
		err = api.RefreshAPITable(ctx, *owner, id, interval, true)
		apiTableReply(c, map[string]any{"accepted": err == nil}, err)
	}
}
