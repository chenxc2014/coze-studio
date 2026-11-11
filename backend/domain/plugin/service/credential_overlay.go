package service

import (
	"context"
	"github.com/coze-dev/coze-studio/backend/bizpkg/config"
	"github.com/coze-dev/coze-studio/backend/bizpkg/config/plugincredentials"
	"github.com/coze-dev/coze-studio/backend/crossdomain/plugin/model"
)

func resolvePluginCredential(ctx context.Context, pluginID int64, auth *model.AuthV2) (*model.AuthV2, error) {
	provider, ok := plugincredentials.FindPlugin(pluginID)
	store := config.PluginCredentials()
	if !ok || store == nil || auth == nil || auth.AuthOfAPIToken == nil {
		return auth, nil
	}
	value, _, err := store.Resolve(ctx, provider.ID, auth.AuthOfAPIToken.ServiceToken)
	if err != nil {
		return nil, err
	}
	// 插件目录的对象被多个请求共享，必须按调用复制认证对象，避免串扰或泄露给插件详情接口。
	result := *auth
	token := *auth.AuthOfAPIToken
	token.ServiceToken = value
	result.AuthOfAPIToken = &token
	return &result, nil
}
