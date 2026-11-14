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
	workflowapp "github.com/coze-dev/coze-studio/backend/application/workflow"
	"github.com/coze-dev/coze-studio/backend/domain/taskcenter"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

func taskReply(c *app.RequestContext, data any, err error) {
	c.Header("Cache-Control", "no-store")
	if err != nil {
		status, message := 500, "任务中心操作失败，请稍后重试"
		var business *taskcenter.Error
		if errors.As(err, &business) {
			status = business.Status
			message = business.Message
		} else {
			logs.Errorf("task center request failed; database or workflow service unavailable")
		}
		c.JSON(status, map[string]any{"code": status, "message": message})
		return
	}
	c.JSON(200, map[string]any{"code": 0, "data": data})
}

func taskBind(c *app.RequestContext, input any) bool {
	origin, e := url.Parse(c.Request.Header.Get("Origin"))
	media, _, mediaErr := mime.ParseMediaType(c.Request.Header.Get("Content-Type"))
	if e != nil || origin.Host == "" || !strings.EqualFold(origin.Host, string(c.Request.Host())) || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		taskReply(c, nil, &taskcenter.Error{Status: 403, Message: "写操作必须来自当前平台页面"})
		return false
	}
	if mediaErr != nil || media != "application/json" || len(c.Request.Body()) > 4<<20 {
		taskReply(c, nil, &taskcenter.Error{Status: 400, Message: "请求必须为 4 MiB 以内的 JSON"})
		return false
	}
	if e = json.Unmarshal(c.Request.Body(), input); e != nil {
		taskReply(c, nil, &taskcenter.Error{Status: 400, Message: "请求参数不正确"})
		return false
	}
	return true
}

func TaskCenter(action string) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		actor := ctxutil.GetUIDFromCtx(ctx)
		if actor == nil {
			taskReply(c, nil, &taskcenter.Error{Status: 401, Message: "请先登录平台"})
			return
		}
		s := workflowapp.SVC.TaskCenter
		if s == nil || !s.Ready {
			taskReply(c, nil, &taskcenter.Error{Status: 503, Message: "任务中心尚未初始化，请联系管理员完成数据库升级"})
			return
		}
		var body struct {
			taskcenter.CreateRequest
			SpaceID   string `json:"space_id"`
			TaskID    string `json:"task_id"`
			RunID     string `json:"run_id"`
			InputJSON string `json:"input_json"`
		}
		write := action == "create" || action == "execute" || action == "cancel" || action == "retry" || action == "delete" || action == "preview"
		if write {
			if !taskBind(c, &body) {
				return
			}
		} else {
			body.SpaceID = c.Query("space_id")
			body.TaskID = c.Query("task_id")
			body.RunID = c.Query("run_id")
			body.WorkflowID = c.Query("workflow_id")
			body.Version = c.Query("version")
		}
		space, e := taskcenter.ID(body.SpaceID)
		if e != nil {
			taskReply(c, nil, e)
			return
		}
		if e = workflowapp.SVC.TaskSpaceAccess(ctx, *actor, space); e != nil {
			taskReply(c, nil, e)
			return
		}
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
		switch action {
		case "workflows":
			data, err := workflowapp.SVC.TaskWorkflows(ctx, *actor, space, c.Query("q"))
			taskReply(c, map[string]any{"workflows": data}, err)
			return
		case "schema", "template", "preview":
			id, err := taskcenter.ID(body.WorkflowID)
			if err != nil {
				taskReply(c, nil, err)
				return
			}
			data, err := workflowapp.SVC.ResolveTaskWorkflow(ctx, *actor, space, id, body.Version)
			if err != nil {
				taskReply(c, nil, err)
				return
			}
			if action == "template" {
				if len(data.Fields) == 0 {
					taskReply(c, nil, &taskcenter.Error{Status: 400, Message: "无参数工作流请使用异步任务"})
					return
				}
				c.Header("Content-Type", "text/csv; charset=utf-8")
				c.Header("Content-Disposition", "attachment; filename=workflow-input-template.csv")
				c.Header("Cache-Control", "no-store")
				c.Response.SetBody(taskcenter.TemplateCSV(data.Fields))
				return
			}
			if action == "preview" {
				rows, err := taskcenter.ParseCSV(data.Fields, body.CSVBase64)
				if err != nil {
					taskReply(c, nil, err)
					return
				}
				preview := make([][]string, min(5, len(rows)))
				for i := range preview {
					for _, field := range data.Fields {
						raw, _ := json.Marshal(rows[i][field.Name])
						var value string
						if json.Unmarshal(raw, &value) != nil {
							value = string(raw)
						}
						preview[i] = append(preview[i], value)
					}
				}
				taskReply(c, map[string]any{"total": len(rows), "rows": preview, "fields": data.Fields}, nil)
				return
			}
			taskReply(c, data, nil)
			return
		case "list":
			data, total, err := s.List(ctx, space, *actor, c.Query("kind"), c.Query("status"), c.Query("q"), c.Query("mine") == "true", page, size)
			taskReply(c, map[string]any{"tasks": data, "total": total, "current_user_id": strconv.FormatInt(*actor, 10)}, err)
			return
		case "create":
			data, err := s.Create(ctx, *actor, space, body.CreateRequest)
			taskReply(c, data, err)
			return
		}
		taskID, err := taskcenter.ID(body.TaskID)
		if err != nil {
			taskReply(c, nil, err)
			return
		}
		var runID int64
		if body.RunID != "" && body.RunID != "0" {
			runID, err = taskcenter.ID(body.RunID)
			if err != nil {
				taskReply(c, nil, err)
				return
			}
		}
		switch action {
		case "detail":
			historyPage, _ := strconv.Atoi(c.DefaultQuery("history_page", "1"))
			data, err := s.Detail(ctx, space, taskID, runID, page, size, c.Query("status"), historyPage)
			taskReply(c, data, err)
		case "execute":
			data, err := s.Trigger(ctx, *actor, space, taskID, body.InputJSON, body.RequestKey)
			taskReply(c, data, err)
		case "retry":
			data, err := s.Retry(ctx, *actor, space, taskID, runID, body.RequestKey)
			taskReply(c, data, err)
		case "cancel":
			taskReply(c, map[string]any{"accepted": true}, s.Cancel(ctx, *actor, space, taskID, runID))
		case "delete":
			taskReply(c, map[string]any{"deleted": true}, s.Delete(ctx, *actor, space, taskID))
		case "export":
			data, err := s.Export(ctx, space, taskID, runID)
			if err != nil {
				taskReply(c, nil, err)
				return
			}
			c.Header("Cache-Control", "no-store")
			c.Header("Content-Type", "text/csv; charset=utf-8")
			c.Header("Content-Disposition", "attachment; filename=task-results.csv")
			c.Response.SetBody(data)
		}
	}
}
