package service

import (
	"context"
	"fmt"

	wfmodel "github.com/coze-dev/coze-studio/backend/crossdomain/workflow/model"
	"github.com/coze-dev/coze-studio/backend/domain/systemresource"
)

// 非维护者执行系统工作流时固定到管理员已经公开的版本，禁止读取未发布草稿。
func systemWorkflowConfig(ctx context.Context, config wfmodel.ExecuteConfig) (wfmodel.ExecuteConfig, error) {
	r := systemresource.Default()
	p, err := r.Lookup(ctx, systemresource.Workflow, config.ID)
	if err != nil {
		return config, err
	}
	if p == nil {
		return config, nil
	}
	if config.Operator == p.SourceOwnerID && config.Mode == wfmodel.ExecuteModeDebug {
		admin, e := r.IsAdmin(ctx, config.Operator)
		if e != nil {
			return config, e
		}
		if admin {
			return config, nil
		}
	}
	if p.Status != "published" {
		return config, fmt.Errorf("系统工作流已下架，请联系管理员")
	}
	config.From = wfmodel.FromSpecificVersion
	config.Mode = wfmodel.ExecuteModeRelease
	if !systemresource.IsVersionPinned(ctx) || config.Version == "" {
		config.Version = p.Version
	}
	return config, nil
}
