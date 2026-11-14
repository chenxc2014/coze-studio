package systemlibrary

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/api/model/data/database/table"
	product "github.com/coze-dev/coze-studio/backend/api/model/marketplace/product_public_api"
	"github.com/coze-dev/coze-studio/backend/application/knowledge"
	"github.com/coze-dev/coze-studio/backend/application/memory"
	"github.com/coze-dev/coze-studio/backend/application/plugin"
	"github.com/coze-dev/coze-studio/backend/application/prompt"
	"github.com/coze-dev/coze-studio/backend/application/workflow"
	"github.com/coze-dev/coze-studio/backend/bizpkg/config"
	dbmodel "github.com/coze-dev/coze-studio/backend/crossdomain/database/model"
	pluginconsts "github.com/coze-dev/coze-studio/backend/crossdomain/plugin/consts"
	wfmodel "github.com/coze-dev/coze-studio/backend/crossdomain/workflow/model"
	ksvc "github.com/coze-dev/coze-studio/backend/domain/knowledge/service"
	dbsvc "github.com/coze-dev/coze-studio/backend/domain/memory/database/service"
	pluginconf "github.com/coze-dev/coze-studio/backend/domain/plugin/conf"
	"github.com/coze-dev/coze-studio/backend/domain/systemresource"
	"github.com/coze-dev/coze-studio/backend/domain/workflow/entity/vo"
)

type Service struct{ Registry *systemresource.Registry }

var SVC *Service

func Init(db *gorm.DB) {
	r := systemresource.New(db, func(ctx context.Context, actor int64) (bool, error) {
		if actor <= 0 {
			return false, nil
		}
		var email string
		if err := db.WithContext(ctx).Table("user").Select("email").Where("id = ? AND deleted_at IS NULL", actor).Scan(&email).Error; err != nil {
			return false, err
		}
		base, err := config.Base().GetBaseConfig(ctx)
		if err != nil {
			return false, err
		}
		for _, allowed := range strings.Split(base.AdminEmails, ",") {
			if email != "" && strings.EqualFold(strings.TrimSpace(allowed), email) {
				return true, nil
			}
		}
		return false, nil
	})
	r.Builtin = func(ctx context.Context, kind systemresource.Kind, id int64) (bool, error) {
		if kind == systemresource.Prompt {
			items, err := prompt.PromptSVC.DomainSVC.ListOfficialPromptResource(ctx, "")
			if err != nil {
				return false, err
			}
			for _, p := range items {
				if p.ID == id {
					return true, nil
				}
			}
		}
		return false, nil
	}
	systemresource.SetDefault(r)
	SVC = &Service{Registry: r}
}

var usage = map[systemresource.Kind]string{
	systemresource.Plugin:    "在智能体或工作流的添加插件窗口选择独立的“资源库-系统”入口。平台发布并维护，可直接引用，无需另行申请厂商授权。原有第三方工具仍在“搜索工具”中选择。",
	systemresource.Workflow:  "在智能体的技能或工作流的添加工作流窗口选择“资源库-系统”。采用共享引用，执行管理员当前发布的版本。",
	systemresource.Prompt:    "在提示词编辑器的提示词库中选择“资源库-系统”，将系统提示词插入当前编辑器；系统原件保持只读。",
	systemresource.Knowledge: "在智能体或工作流的添加知识库窗口选择“资源库-系统”。检索共享数据，普通用户不能修改知识库或文档。",
	systemresource.Database:  "在智能体或工作流的选择数据库窗口选择“资源库-系统”。可查询共享表，普通用户不能修改公共表结构或公共数据。",
}

func (s *Service) Describe(ctx context.Context, kind systemresource.Kind, id int64) (resource *systemresource.Resource, space, owner int64, err error) {
	r := &systemresource.Resource{Kind: kind, ID: id, Origin: "published", Status: "published", ReadOnly: true, Usage: usage[kind]}
	switch kind {
	case systemresource.Plugin:
		if _, thirdParty := pluginconf.GetPluginProduct(id); thirdParty {
			return nil, 0, 0, fmt.Errorf("搜索工具中的第三方插件不能发布为系统资源，请在搜索工具中继续使用")
		}
		p, e := plugin.PluginApplicationSVC.DomainSVC.GetOnlinePlugin(ctx, id)
		if e != nil {
			return nil, 0, 0, e
		}
		space = p.SpaceID
		owner = p.DeveloperID
		r.Name = p.GetName()
		r.Description = p.GetDesc()
		r.Version = p.GetVersion()
		if p.GetAPPID() > 0 {
			return nil, 0, 0, fmt.Errorf("请先将插件移入资源库，再发布为系统资源")
		}
		if p.SpaceID == systemresource.PublicSpaceID {
			return nil, 0, 0, fmt.Errorf("搜索工具中的第三方插件不能发布为系统资源")
		}
		if auth := p.GetAuthInfo(); auth == nil || auth.Type != pluginconsts.AuthzTypeOfNone {
			return nil, 0, 0, fmt.Errorf("系统插件必须无需厂商额外授权，不支持 API Key、OAuth 或其他授权类型")
		}
		tools, e := plugin.PluginApplicationSVC.DomainSVC.GetPluginProductAllTools(ctx, id)
		if e != nil {
			return nil, 0, 0, e
		}
		for _, tool := range tools {
			params, e := tool.ToToolParameters()
			if e != nil {
				return nil, 0, 0, e
			}
			r.Tools = append(r.Tools, systemresource.Tool{ID: tool.ID, Name: tool.GetName(), Description: tool.GetDesc(), Fields: toolFields(params, 0)})
		}
	case systemresource.Workflow:
		meta, e := workflow.SVC.DomainSVC.Get(ctx, &vo.GetPolicy{ID: id, MetaOnly: true})
		if e != nil {
			return nil, 0, 0, e
		}
		space = meta.SpaceID
		owner = meta.CreatorID
		if meta.AppID != nil && *meta.AppID > 0 {
			return nil, 0, 0, fmt.Errorf("请先将工作流移入资源库，再发布为系统资源")
		}
		if meta.LatestPublishedVersion == nil || *meta.LatestPublishedVersion == "" {
			return nil, 0, 0, fmt.Errorf("工作流尚未发布，请先在开发页面发布工作流版本")
		}
		r.Version = *meta.LatestPublishedVersion
		r.Name = meta.Name
		r.Description = meta.Desc
		r.Icon = meta.IconURL
		wf, e := workflow.SVC.DomainSVC.Get(ctx, &vo.GetPolicy{ID: id, QType: wfmodel.FromSpecificVersion, Version: r.Version})
		if e != nil {
			return nil, 0, 0, e
		}
		var canvas struct {
			Nodes []struct {
				ID   string `json:"id"`
				Data struct {
					Outputs []systemresource.Field `json:"outputs"`
				} `json:"data"`
			} `json:"nodes"`
		}
		if e = json.Unmarshal([]byte(wf.Canvas), &canvas); e != nil {
			return nil, 0, 0, e
		}
		if e = s.validateWorkflowDependencies(ctx, wf.Canvas); e != nil {
			return nil, 0, 0, e
		}
		for _, node := range canvas.Nodes {
			if node.ID == "100001" {
				r.Fields = node.Data.Outputs
			}
		}
	case systemresource.Prompt:
		builtin, e := s.Registry.Builtin(ctx, kind, id)
		if e != nil {
			return nil, 0, 0, e
		}
		if builtin {
			items, e := prompt.PromptSVC.DomainSVC.ListOfficialPromptResource(ctx, "")
			if e != nil {
				return nil, 0, 0, e
			}
			for _, p := range items {
				if p.ID == id {
					r.Name = p.Name
					r.Description = p.Description
					r.Content = p.PromptText
					r.Origin = "builtin"
					r.Version = "内置"
					return r, 0, 0, nil
				}
			}
		}
		p, e := prompt.PromptSVC.DomainSVC.GetPromptResource(ctx, id)
		if e != nil {
			return nil, 0, 0, e
		}
		space = p.SpaceID
		owner = p.CreatorID
		r.Name = p.Name
		r.Description = p.Description
		r.Content = p.PromptText
		r.Version = "发布快照"
	case systemresource.Knowledge:
		items, e := knowledge.KnowledgeSVC.DomainSVC.ListKnowledge(ctx, &ksvc.ListKnowledgeRequest{IDs: []int64{id}})
		if e != nil {
			return nil, 0, 0, e
		}
		if len(items.KnowledgeList) != 1 {
			return nil, 0, 0, fmt.Errorf("知识库不存在")
		}
		p := items.KnowledgeList[0]
		var documents, unfinished int64
		q := s.Registry.DB.WithContext(ctx).Table("knowledge_document").Where("knowledge_id = ? AND deleted_at IS NULL", id)
		if e = q.Count(&documents).Error; e != nil {
			return nil, 0, 0, e
		}
		if e = q.Where("status <> 1").Count(&unfinished).Error; e != nil {
			return nil, 0, 0, e
		}
		if documents == 0 || unfinished > 0 {
			return nil, 0, 0, fmt.Errorf("知识库必须已有数据且全部处理完成，才能作为可直接使用的系统资源发布")
		}
		if p.AppID > 0 {
			return nil, 0, 0, fmt.Errorf("请先将知识库移入资源库，再发布为系统资源")
		}
		space = p.SpaceID
		owner = p.CreatorID
		r.Name = p.Name
		r.Description = p.Description
		r.Icon = p.IconURL
		r.Version = "共享实时数据"
		r.Notice = "管理员发布后，知识库中的文档和检索结果对全平台用户开放。仅发布确认可共享的数据。"
	case systemresource.Database:
		items, e := memory.DatabaseApplicationSVC.DomainSVC.MGetDatabase(ctx, &dbsvc.MGetDatabaseRequest{Basics: []*dbmodel.DatabaseBasic{{ID: id, TableType: table.TableType_OnlineTable}}})
		if e != nil {
			return nil, 0, 0, e
		}
		if len(items.Databases) != 1 {
			return nil, 0, 0, fmt.Errorf("已发布数据库不存在")
		}
		p := items.Databases[0]
		space = p.SpaceID
		owner = p.CreatorID
		r.ID = p.ID
		r.Name = p.TableName
		r.Description = p.TableDesc
		r.Icon = p.IconURL
		r.Version = "共享实时数据"
		r.Notice = "数据库以共享只读方式开放；普通用户只能查询，不能写入、修改或删除公共数据。"
		if p.AppID > 0 {
			return nil, 0, 0, fmt.Errorf("请先将数据库移入资源库，再发布为系统资源")
		}
		for _, field := range p.FieldList {
			r.Fields = append(r.Fields, systemresource.Field{Name: field.Name, Type: field.Type.String()})
		}
	default:
		return nil, 0, 0, fmt.Errorf("不支持的资源类型")
	}
	if r.Name == "" {
		return nil, 0, 0, fmt.Errorf("资源元数据不完整")
	}
	return r, space, owner, nil
}

func toolFields(params []*product.ToolParameter, depth int) []systemresource.Field {
	if depth > 12 {
		return nil
	}
	fields := []systemresource.Field{}
	for _, p := range params {
		if p != nil {
			fields = append(fields, systemresource.Field{Name: p.Name, Type: p.Type, Description: p.Description, Required: p.IsRequired, Children: toolFields(p.SubParameter, depth+1)})
		}
	}
	return fields
}

func (s *Service) Builtins(ctx context.Context) ([]systemresource.Resource, error) {
	items := []systemresource.Resource{}
	prompts, e := prompt.PromptSVC.DomainSVC.ListOfficialPromptResource(ctx, "")
	if e != nil {
		return nil, e
	}
	for _, p := range prompts {
		items = append(items, systemresource.Resource{Kind: systemresource.Prompt, ID: p.ID, Name: p.Name, Description: p.Description, Version: "内置", Origin: "builtin", Status: "published", ReadOnly: true, Usage: usage[systemresource.Prompt]})
	}
	return items, nil
}

func (s *Service) List(ctx context.Context, admin bool) ([]systemresource.Resource, error) {
	items, err := s.Builtins(ctx)
	if err != nil {
		return nil, err
	}
	records, err := s.Registry.List(ctx, true)
	if err != nil {
		return nil, err
	}
	byKey := map[string]systemresource.Resource{}
	for _, item := range items {
		byKey[fmt.Sprintf("%s:%d", item.Kind, item.ID)] = item
	}
	for _, p := range records {
		key := fmt.Sprintf("%s:%d", p.Kind, p.ResourceID)
		delete(byKey, key)
		if !admin && p.Status != "published" {
			continue
		}
		byKey[key] = systemresource.Resource{Kind: p.Kind, ID: p.ResourceID, Name: p.Name, Description: p.Description, Icon: p.Icon, Version: p.Version, Origin: p.Origin, Status: p.Status, Revision: p.Revision, PublishedAt: p.UpdatedAt, ReadOnly: true, Usage: usage[p.Kind]}
	}
	result := make([]systemresource.Resource, 0, len(byKey))
	for _, v := range byKey {
		result = append(result, v)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].PublishedAt != result[j].PublishedAt {
			return result[i].PublishedAt > result[j].PublishedAt
		}
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		return result[i].ID < result[j].ID
	})
	return result, nil
}

func (s *Service) Detail(ctx context.Context, kind systemresource.Kind, id int64, admin bool) (*systemresource.Resource, error) {
	p, err := s.Registry.Lookup(ctx, kind, id)
	if err != nil {
		return nil, err
	}
	if p != nil {
		if p.Status != "published" && !admin {
			return nil, fmt.Errorf("系统资源不存在或已下架")
		}
		var r systemresource.Resource
		if err = json.Unmarshal([]byte(p.MetadataJSON), &r); err != nil {
			return nil, err
		}
		r.Revision = p.Revision
		r.Status = p.Status
		r.PublishedAt = p.UpdatedAt
		return &r, nil
	}
	builtin, err := s.Registry.Builtin(ctx, kind, id)
	if err != nil {
		return nil, err
	}
	if !builtin {
		return nil, fmt.Errorf("系统资源不存在")
	}
	r, _, _, err := s.Describe(ctx, kind, id)
	return r, err
}

func (s *Service) Publish(ctx context.Context, actor int64, kind systemresource.Kind, id, expected int64, confirmed, readyConfirmed bool) (*systemresource.Resource, error) {
	if !confirmed {
		return nil, fmt.Errorf("请确认该资源及其数据可以向全平台用户公开共享")
	}
	if !readyConfirmed {
		return nil, fmt.Errorf("请先完成预览调试，并确认资源及其依赖可直接使用、无需厂商额外授权")
	}
	r, space, owner, err := s.Describe(ctx, kind, id)
	if err != nil {
		return nil, err
	}
	if r.Origin != "builtin" && owner != actor {
		return nil, fmt.Errorf("只能发布当前管理员自己创建和维护的资源")
	}
	data, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	p := &systemresource.Publication{Kind: kind, ResourceID: r.ID, SourceSpaceID: space, SourceOwnerID: owner, Name: r.Name, Description: r.Description, Icon: r.Icon, Version: r.Version, Origin: r.Origin, Status: "published", MetadataJSON: string(data)}
	if err = s.Registry.Save(ctx, p, actor, expected); err != nil {
		return nil, err
	}
	r.Revision = p.Revision
	r.PublishedAt = p.UpdatedAt
	return r, nil
}

func (s *Service) Withdraw(ctx context.Context, actor int64, kind systemresource.Kind, id, expected int64) (*systemresource.Resource, error) {
	r, err := s.Detail(ctx, kind, id, true)
	if err != nil {
		return nil, err
	}
	r.Status = "withdrawn"
	data, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	p, err := s.Registry.Lookup(ctx, kind, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		p = &systemresource.Publication{Kind: kind, ResourceID: id, Name: r.Name, Description: r.Description, Icon: r.Icon, Version: r.Version, Origin: r.Origin}
	}
	p.Status = "withdrawn"
	p.MetadataJSON = string(data)
	if err = s.Registry.Save(ctx, p, actor, expected); err != nil {
		return nil, err
	}
	r.Revision = p.Revision
	r.PublishedAt = p.UpdatedAt
	return r, nil
}

func ParseID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("资源 ID 不正确")
	}
	return id, nil
}

// 普通目录无需读取真实凭据；本函数只统一筛选和分页返回只读元数据。
func Filter(items []systemresource.Resource, kind, query, status string, page, size int) ([]systemresource.Resource, int, map[string]int) {
	counts := map[string]int{"all": len(items), "plugin": 0, "workflow": 0, "prompt": 0, "knowledge": 0, "database": 0}
	filtered := []systemresource.Resource{}
	query = strings.ToLower(strings.TrimSpace(query))
	for _, item := range items {
		counts[string(item.Kind)]++
		if kind != "" && kind != "all" && string(item.Kind) != kind {
			continue
		}
		if status != "" && item.Status != status {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(item.Name+" "+item.Description), query) {
			continue
		}
		filtered = append(filtered, item)
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 50 {
		size = 15
	}
	total := len(filtered)
	start := min((page-1)*size, total)
	return filtered[start:min(start+size, total)], total, counts
}
