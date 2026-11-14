package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/coze-dev/coze-studio/backend/domain/systemresource"
	"strconv"
	"strings"

	"gorm.io/gorm"

	model "github.com/coze-dev/coze-studio/backend/crossdomain/workflow/model"
	"github.com/coze-dev/coze-studio/backend/domain/taskcenter"
	userEntity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/domain/workflow/entity"
	"github.com/coze-dev/coze-studio/backend/domain/workflow/entity/vo"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	"github.com/coze-dev/coze-studio/backend/types/consts"
)

type taskRunner struct {
	service *ApplicationService
	db      *gorm.DB
}

func (w *ApplicationService) ResolveTaskWorkflow(ctx context.Context, actor, space, id int64, version string) (*taskcenter.Workflow, error) {
	return (&taskRunner{service: w, db: w.taskDB}).Resolve(ctx, actor, space, id, version)
}

func (w *ApplicationService) InitTaskCenter(ctx context.Context, db *gorm.DB) {
	w.taskDB = db
	w.TaskCenter = taskcenter.New(db, w.IDGenerator, &taskRunner{service: w, db: db})
	w.TaskCenter.Start(ctx)
}

func (w *ApplicationService) TaskSpaceAccess(ctx context.Context, actor, space int64) error {
	if err := checkUserSpace(ctx, actor, space); err != nil {
		return &taskcenter.Error{Status: 403, Message: "没有当前空间的访问权限"}
	}
	return nil
}

func (w *ApplicationService) TaskWorkflows(ctx context.Context, actor, space int64, query string) ([]*taskcenter.Workflow, error) {
	if e := w.TaskSpaceAccess(ctx, actor, space); e != nil {
		return nil, e
	}
	rows := []struct {
		ID            int64
		Name          string
		LatestVersion string
		AppID         *int64
	}{}
	sharedIDs, err := systemresource.IDs(ctx, systemresource.Workflow)
	if err != nil {
		return nil, err
	}
	q := w.taskDB.WithContext(ctx).Table("workflow_meta").Select("id,name,latest_version,app_id").Where("(space_id = ? OR id IN ?) AND mode = 0 AND latest_version <> '' AND deleted_at IS NULL", space, sharedIDs)
	if query != "" {
		q = q.Where("name LIKE ?", "%"+query+"%")
	}
	if e := q.Order("updated_at DESC,id DESC").Limit(200).Scan(&rows).Error; e != nil {
		return nil, e
	}
	result := make([]*taskcenter.Workflow, 0, len(rows))
	for _, row := range rows {
		if publication, e := systemresource.Default().Lookup(ctx, systemresource.Workflow, row.ID); e != nil {
			return nil, e
		} else if publication != nil && publication.Status == "published" {
			row.LatestVersion = publication.Version
		}
		result = append(result, &taskcenter.Workflow{ID: row.ID, Name: row.Name, Version: row.LatestVersion, AppID: row.AppID})
	}
	return result, nil
}

func (r *taskRunner) Resolve(ctx context.Context, actor, space, id int64, version string) (*taskcenter.Workflow, error) {
	if e := r.service.TaskSpaceAccess(ctx, actor, space); e != nil {
		return nil, e
	}
	publication, e := systemresource.Default().Lookup(ctx, systemresource.Workflow, id)
	if e != nil {
		return nil, e
	}
	shared := publication != nil && publication.Status == "published"
	meta, e := r.service.DomainSVC.Get(ctx, &vo.GetPolicy{ID: id, MetaOnly: true})
	if e != nil || meta == nil || meta.Meta == nil || (meta.SpaceID != space && !shared) {
		return nil, &taskcenter.Error{Status: 404, Message: "工作流不存在或不在当前空间"}
	}
	if int(meta.Mode) != 0 {
		return nil, &taskcenter.Error{Status: 400, Message: "任务中心仅支持普通低代码工作流"}
	}
	if meta.LatestPublishedVersion == nil || *meta.LatestPublishedVersion == "" {
		return nil, &taskcenter.Error{Status: 400, Message: "请先发布工作流版本"}
	}
	if version == "" {
		version = *meta.LatestPublishedVersion
		if shared {
			version = publication.Version
		}
	}
	if shared && version != publication.Version {
		var approved int64
		e = systemresource.Default().DB.WithContext(ctx).Model(&systemresource.Release{}).Where("kind = ? AND resource_id = ? AND status = ? AND version = ?", systemresource.Workflow, id, "published", version).Count(&approved).Error
		if e != nil {
			return nil, e
		}
		if approved == 0 {
			return nil, &taskcenter.Error{Status: 400, Message: "该版本尚未发布为系统资源"}
		}
	}
	if len(version) > 128 {
		return nil, &taskcenter.Error{Status: 400, Message: "版本不正确"}
	}
	wf, e := r.service.DomainSVC.Get(ctx, &vo.GetPolicy{ID: id, QType: model.FromSpecificVersion, Version: version})
	if e != nil || wf == nil || wf.CanvasInfo == nil {
		return nil, &taskcenter.Error{Status: 400, Message: "指定的已发布版本不可用"}
	}
	var canvas struct {
		Nodes []struct {
			ID   string `json:"id"`
			Data *struct {
				Outputs []taskcenter.Field `json:"outputs"`
			} `json:"data"`
		} `json:"nodes"`
	}
	if json.Unmarshal([]byte(wf.Canvas), &canvas) != nil {
		return nil, &taskcenter.Error{Status: 400, Message: "工作流画布无法解析"}
	}
	fields := []taskcenter.Field{}
	found := false
	for _, node := range canvas.Nodes {
		if node.ID == "100001" && node.Data != nil {
			fields = node.Data.Outputs
			found = true
			break
		}
	}
	if !found || len(fields) > 100 {
		return nil, &taskcenter.Error{Status: 400, Message: "工作流开始节点或参数配置不正确"}
	}
	seen := map[string]bool{}
	for _, field := range fields {
		if strings.TrimSpace(field.Name) == "" || seen[field.Name] {
			return nil, &taskcenter.Error{Status: 400, Message: "工作流参数名称为空或重复"}
		}
		seen[field.Name] = true
	}
	versions := []string{}
	versionQuery := r.db.WithContext(ctx).Table("workflow_version").Where("workflow_id = ? AND deleted_at IS NULL", id).Order("created_at DESC,id DESC").Limit(100)
	if shared {
		versionQuery = systemresource.Default().DB.WithContext(ctx).Model(&systemresource.Release{}).Where("kind = ? AND resource_id = ? AND status = ?", systemresource.Workflow, id, "published").Group("version").Order("MAX(created_at) DESC").Limit(100)
	}
	if e = versionQuery.Pluck("version", &versions).Error; e != nil {
		return nil, e
	}
	return &taskcenter.Workflow{ID: id, Name: meta.Name, Version: version, AppID: meta.AppID, Fields: fields, Versions: versions}, nil
}

func (r *taskRunner) Execute(ctx context.Context, task *taskcenter.Task, input map[string]any) (result taskcenter.Result, err error) {
	defer func() {
		if recover() != nil {
			result = taskcenter.Result{}
			err = fmt.Errorf("工作流运行异常，请查看工作流执行日志")
		}
	}()
	// 每条执行独立初始化用户上下文，避免后台不同创建者的身份互相覆盖。
	ctx = systemresource.PinVersion(ctx)
	ctx = ctxcache.Init(ctx)
	ctxcache.Store(ctx, consts.SessionDataKeyInCtx, &userEntity.Session{UserID: task.OwnerID, Locale: "zh-CN"})
	if err = r.service.TaskSpaceAccess(ctx, task.OwnerID, task.SpaceID); err != nil {
		return result, err
	}
	meta, err := r.service.DomainSVC.Get(ctx, &vo.GetPolicy{ID: task.WorkflowID, MetaOnly: true})
	shared, lookupErr := systemresource.Published(ctx, systemresource.Workflow, task.WorkflowID)
	if lookupErr != nil {
		return result, lookupErr
	}
	if err != nil || meta == nil || (meta.SpaceID != task.SpaceID && !shared) {
		return result, fmt.Errorf("工作流已删除或访问权限已变更")
	}
	exe, _, err := r.service.DomainSVC.SyncExecute(ctx, model.ExecuteConfig{ID: task.WorkflowID, From: model.FromSpecificVersion, Version: task.Version, Operator: task.OwnerID, Mode: model.ExecuteModeRelease, TaskType: model.TaskTypeBackground, SyncPattern: model.SyncPatternSync, InputFailFast: true, Cancellable: true, ConnectorID: consts.CozeConnectorID, ConnectorUID: strconv.FormatInt(task.OwnerID, 10), BizType: model.BizTypeWorkflow}, input)
	if err != nil {
		return result, err
	}
	if exe == nil {
		return result, fmt.Errorf("工作流没有返回执行结果")
	}
	result.ExecutionID = exe.ID
	if exe.Status != entity.WorkflowSuccess {
		if exe.FailReason != nil {
			return result, fmt.Errorf("%s", *exe.FailReason)
		}
		return result, fmt.Errorf("工作流未正常结束；交互式中断工作流需要在编辑器处理")
	}
	if exe.Output != nil {
		result.Output = *exe.Output
	}
	return result, nil
}
