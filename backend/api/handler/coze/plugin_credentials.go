package coze

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	"github.com/coze-dev/coze-studio/backend/bizpkg/config"
	"github.com/coze-dev/coze-studio/backend/bizpkg/config/plugincredentials"
	pluginconf "github.com/coze-dev/coze-studio/backend/domain/plugin/conf"
	"strconv"
	"time"
)

func pluginCredentialReply(c *app.RequestContext, status int, data any, message string) {
	c.Header("Cache-Control", "no-store")
	if status != 200 {
		c.JSON(status, map[string]any{"code": status, "message": message})
		return
	}
	c.JSON(200, map[string]any{"code": 0, "data": data})
}
func pluginFileCredential(provider plugincredentials.Provider) string {
	id, _ := strconv.ParseInt(provider.PluginID, 10, 64)
	if plugin, ok := pluginconf.GetPluginProduct(id); ok {
		auth := plugin.Info.GetAuthInfo()
		if auth != nil && auth.AuthOfAPIToken != nil {
			return auth.AuthOfAPIToken.ServiceToken
		}
	}
	return ""
}
func ListPluginCredentials(ctx context.Context, c *app.RequestContext) {
	c.Header("Cache-Control", "no-store")
	store := config.PluginCredentials()
	items := []plugincredentials.Metadata{}
	for _, provider := range plugincredentials.Providers() {
		item, err := store.Metadata(ctx, provider.ID, pluginFileCredential(provider))
		if err != nil {
			pluginCredentialReply(c, 503, nil, "无法读取插件授权，请检查凭据存储和原加密密钥")
			return
		}
		items = append(items, item)
	}
	pluginCredentialReply(c, 200, map[string]any{"providers": items, "encryption_ready": store.Ready()}, "")
}

type pluginCredentialInput struct {
	Provider   string `json:"provider"`
	Credential string `json:"credential"`
	Clear      bool   `json:"clear"`
}

func SavePluginCredential(ctx context.Context, c *app.RequestContext) {
	var input pluginCredentialInput
	if !bindAccountAdmin(c, &input) {
		return
	}
	provider, ok := plugincredentials.Find(input.Provider)
	if !ok {
		pluginCredentialReply(c, 400, nil, "不支持的插件供应商")
		return
	}
	store := config.PluginCredentials()
	if !store.Ready() {
		pluginCredentialReply(c, 503, nil, "服务端尚未准备好凭据加密密钥，请联系运维人员")
		return
	}
	if err := store.Save(ctx, input.Provider, input.Credential, input.Clear, ctxutil.MustGetUIDFromCtx(ctx)); err != nil {
		// 仅返回无凭据的业务说明，不向日志打印请求或底层错误对象。
		pluginCredentialReply(c, 400, nil, err.Error())
		return
	}
	meta, err := store.Metadata(ctx, input.Provider, pluginFileCredential(provider))
	if err != nil {
		pluginCredentialReply(c, 503, nil, "授权已保存，但读取状态失败，请刷新核对")
		return
	}
	pluginCredentialReply(c, 200, meta, "")
}
func TestPluginCredential(ctx context.Context, c *app.RequestContext) {
	var input struct {
		Provider string `json:"provider"`
	}
	if !bindAccountAdmin(c, &input) {
		return
	}
	provider, ok := plugincredentials.Find(input.Provider)
	if !ok {
		pluginCredentialReply(c, 400, nil, "不支持的插件供应商")
		return
	}
	credential, _, err := config.PluginCredentials().Resolve(ctx, provider.ID, pluginFileCredential(provider))
	if err != nil {
		pluginCredentialReply(c, 503, nil, "无法读取已保存的插件授权，请检查原加密密钥")
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 13*time.Second)
	defer cancel()
	pluginCredentialReply(c, 200, plugincredentials.Probe(ctx, provider.ID, credential), "")
}
