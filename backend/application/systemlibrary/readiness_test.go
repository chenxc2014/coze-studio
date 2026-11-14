package systemlibrary

import (
	"github.com/coze-dev/coze-studio/backend/domain/systemresource"
	"testing"
)

func TestSystemWorkflowPublishedDependencies(t *testing.T) {
	canvas := `{"nodes":[{"id":"plugin","data":{"inputs":{"apiParam":[{"name":"pluginID","input":{"value":{"content":"12"}}},{"name":"pluginVersion","input":{"value":{"content":"v1"}}}]}}},{"id":"nested","blocks":[{"data":{"inputs":{"workflowId":"99"}}}]},{"data":{"inputs":{"fcParam":{"pluginFCParam":{"pluginList":[{"plugin_id":"22","is_draft":false}]},"workflowFCParam":{"workflowList":[{"workflow_id":"100","is_draft":false}]}}}}}]}`
	refs, err := workflowReferences(canvas)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs[systemresource.Plugin]) != 2 || len(refs[systemresource.Workflow]) != 2 {
		t.Fatalf("未完整检查已发布插件、嵌套工作流或模型工具调用：%+v", refs)
	}
	if _, err = workflowReferences(`{"nodes":[{"data":{"inputs":{"apiParam":[{"name":"pluginID"}]}}}]}`); err == nil {
		t.Fatal("缺少插件标识不应通过检查")
	}
}
