package coze

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	"github.com/coze-dev/coze-studio/backend/application/knowledge"
	"github.com/coze-dev/coze-studio/backend/domain/knowledge/address"
	"github.com/coze-dev/coze-studio/backend/domain/knowledge/service"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
	"strconv"
	"time"
)

// 地址查询是只读扩展接口，沿用文档权限，ID 使用字符串避免浏览器精度损失。
func QueryKnowledgeAddress(ctx context.Context, c *app.RequestContext) {
	c.Header("Cache-Control", "no-store")
	fail := func(status int, message string) { c.JSON(status, map[string]any{"code": status, "message": message}) }
	if ctxutil.GetUIDFromCtx(ctx) == nil {
		fail(401, "请先登录平台")
		return
	}
	var input struct {
		DocumentID   string         `json:"document_id"`
		Query        string         `json:"query"`
		Filter       address.Filter `json:"filter"`
		Page         int            `json:"page"`
		PageSize     int            `json:"page_size"`
		AllowPartial bool           `json:"allow_partial"`
	}
	if len(c.Request.Body()) > 16384 || json.Unmarshal(c.Request.Body(), &input) != nil {
		fail(400, "请求必须是 16 KB 以内的 JSON")
		return
	}
	id, err := strconv.ParseInt(input.DocumentID, 10, 64)
	if err != nil || id <= 0 {
		fail(400, "文档 ID 不正确")
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result, err := knowledge.KnowledgeSVC.QueryAddress(ctx, &service.AddressQueryRequest{DocumentID: id, Query: input.Query, Filter: input.Filter, Page: input.Page, PageSize: input.PageSize, AllowPartial: input.AllowPartial})
	if err != nil {
		var denied *knowledge.AddressPermissionError
		var invalid *service.AddressQueryError
		switch {
		case errors.As(err, &denied):
			fail(403, denied.Error())
		case errors.As(err, &invalid):
			fail(400, invalid.Error())
		default:
			logs.CtxErrorf(ctx, "地址查询失败：%v", err)
			fail(500, "地址查询失败，请稍后重试")
		}
		return
	}
	c.JSON(200, map[string]any{"code": 0, "data": result})
}
