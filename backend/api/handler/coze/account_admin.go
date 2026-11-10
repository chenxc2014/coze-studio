package coze

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/url"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	"github.com/coze-dev/coze-studio/backend/application/user"
	"github.com/coze-dev/coze-studio/backend/domain/user/repository"
)

func accountAdminResponse(c *app.RequestContext, data any, err error) {
	c.Header("Cache-Control", "no-store")
	if err != nil {
		status := 500
		message := "账号管理操作失败，请检查服务日志或稍后重试"
		var business *user.AccountAdminError
		if errors.As(err, &business) {
			status = business.Status
			message = business.Message
		}
		c.JSON(status, map[string]any{"code": status, "message": message})
		return
	}
	c.JSON(200, map[string]any{"code": 0, "data": data})
}

// 修改账号需要同源 JSON 请求，拒绝跨站表单及不带来源的写请求。
func bindAccountAdmin(c *app.RequestContext, input any) bool {
	origin, err := url.Parse(c.Request.Header.Get("Origin"))
	media, _, mediaErr := mime.ParseMediaType(c.Request.Header.Get("Content-Type"))
	if err != nil || origin.Host == "" || !strings.EqualFold(origin.Host, string(c.Request.Host())) || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		accountAdminResponse(c, nil, &user.AccountAdminError{Status: 403, Message: "写操作必须来自当前平台页面"})
		return false
	}
	if mediaErr != nil || media != "application/json" || len(c.Request.Body()) > 16384 {
		accountAdminResponse(c, nil, &user.AccountAdminError{Status: 400, Message: "请求必须是 16 KB 以内的 JSON"})
		return false
	}
	if err = json.Unmarshal(c.Request.Body(), input); err != nil {
		accountAdminResponse(c, nil, &user.AccountAdminError{Status: 400, Message: "请求参数不正确"})
		return false
	}
	return true
}

func ListManagedAccounts(ctx context.Context, c *app.RequestContext) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	actor := ctxutil.MustGetUIDFromCtx(ctx)
	accounts, total, err := user.UserApplicationSVC.Accounts.List(ctx, actor, repository.AccountListOptions{Query: strings.TrimSpace(c.Query("q")), Status: c.Query("status"), Role: c.Query("role"), Page: page, PageSize: size})
	accountAdminResponse(c, map[string]any{"accounts": accounts, "total": total, "page": page, "page_size": size, "current_user_id": strconv.FormatInt(actor, 10)}, err)
}

func CreateManagedAccount(ctx context.Context, c *app.RequestContext) {
	var input user.CreateManagedAccount
	if !bindAccountAdmin(c, &input) {
		return
	}
	account, err := user.UserApplicationSVC.Accounts.Create(ctx, ctxutil.MustGetUIDFromCtx(ctx), input)
	accountAdminResponse(c, account, err)
}

type managedAccountChange struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Password string `json:"password"`
	Disabled *bool  `json:"disabled"`
	IsAdmin  *bool  `json:"is_admin"`
}

func ChangeManagedAccount(action string) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var input managedAccountChange
		if !bindAccountAdmin(c, &input) {
			return
		}
		id, err := strconv.ParseInt(input.ID, 10, 64)
		if err != nil || id <= 0 {
			accountAdminResponse(c, nil, &user.AccountAdminError{Status: 400, Message: "账号 ID 不正确"})
			return
		}
		actor := ctxutil.MustGetUIDFromCtx(ctx)
		accounts := user.UserApplicationSVC.Accounts
		switch action {
		case "update":
			err = accounts.Update(ctx, actor, id, input.Name)
		case "reset-password":
			err = accounts.ResetPassword(ctx, actor, id, input.Password)
		case "status":
			if input.Disabled == nil {
				err = &user.AccountAdminError{Status: 400, Message: "缺少账号状态"}
			} else {
				err = accounts.SetStatus(ctx, actor, id, *input.Disabled)
			}
		case "role":
			if input.IsAdmin == nil {
				err = &user.AccountAdminError{Status: 400, Message: "缺少账号角色"}
			} else {
				err = accounts.SetRole(ctx, actor, id, *input.IsAdmin)
			}
		}
		accountAdminResponse(c, nil, err)
	}
}
