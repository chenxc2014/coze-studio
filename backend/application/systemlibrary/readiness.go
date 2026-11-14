package systemlibrary

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/coze-dev/coze-studio/backend/domain/systemresource"
	"github.com/coze-dev/coze-studio/backend/domain/workflow/entity/vo"
)

// 系统工作流只依赖已公开的系统资源；插件市场和私人资源不随工作流被隐式公开。
func workflowReferences(canvas string) (map[systemresource.Kind][]string, error) {
	var schema vo.Canvas
	if err := json.Unmarshal([]byte(canvas), &schema); err != nil {
		return nil, err
	}
	refs := map[systemresource.Kind][]string{}
	add := func(kind systemresource.Kind, id string) {
		if id != "" {
			refs[kind] = append(refs[kind], id)
		}
	}
	var walk func([]*vo.Node) error
	walk = func(nodes []*vo.Node) error {
		for _, n := range nodes {
			if n == nil {
				continue
			}
			if n.Data != nil && n.Data.Inputs != nil {
				in := n.Data.Inputs
				if in.PluginAPIParam != nil {
					for _, param := range in.APIParams {
						if param == nil || param.Name != "pluginID" {
							continue
						}
						if param.Input == nil || param.Input.Value == nil {
							return fmt.Errorf("插件依赖标识缺失")
						}
						id, ok := param.Input.Value.Content.(string)
						if !ok {
							return fmt.Errorf("插件依赖必须是确定的系统资源 ID")
						}
						add(systemresource.Plugin, id)
					}
				}
				if in.SubWorkflow != nil {
					add(systemresource.Workflow, in.WorkflowID)
				}
				if in.DatabaseNode != nil {
					for _, table := range in.DatabaseInfoList {
						if table != nil {
							add(systemresource.Database, table.DatabaseInfoID)
						}
					}
				}
				if in.Knowledge != nil {
					for _, param := range in.DatasetParam {
						if param == nil || param.Input == nil || param.Input.Value == nil {
							continue
						}
						if ids, ok := param.Input.Value.Content.([]any); ok {
							for _, value := range ids {
								id, ok := value.(string)
								if !ok {
									return fmt.Errorf("知识库依赖标识不正确")
								}
								add(systemresource.Knowledge, id)
							}
						}
					}
				}
				if in.LLM != nil && in.FCParam != nil {
					fc := in.FCParam
					if fc.PluginFCParam != nil {
						for _, item := range fc.PluginFCParam.PluginList {
							add(systemresource.Plugin, item.PluginID)
						}
					}
					if fc.WorkflowFCParam != nil {
						for _, item := range fc.WorkflowFCParam.WorkflowList {
							add(systemresource.Workflow, item.WorkflowID)
						}
					}
					if fc.KnowledgeFCParam != nil {
						for _, item := range fc.KnowledgeFCParam.KnowledgeList {
							if item != nil {
								add(systemresource.Knowledge, item.ID)
							}
						}
					}
				}
			}
			if err := walk(n.Blocks); err != nil {
				return err
			}
		}
		return nil
	}
	return refs, walk(schema.Nodes)
}

func (s *Service) validateWorkflowDependencies(ctx context.Context, canvas string) error {
	refs, err := workflowReferences(canvas)
	if err != nil {
		return err
	}
	for kind, ids := range refs {
		for _, raw := range ids {
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || id <= 0 {
				return fmt.Errorf("系统工作流依赖的资源 ID 不正确")
			}
			shared, err := systemresource.Published(ctx, kind, id)
			if err != nil {
				return err
			}
			if !shared {
				return fmt.Errorf("工作流依赖了尚未作为系统资源发布的 %s：%d；请先发布依赖，第三方插件请保留在搜索工具中使用", kind, id)
			}
		}
	}
	return nil
}
