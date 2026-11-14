package systemlibrary

import (
	"github.com/coze-dev/coze-studio/backend/domain/systemresource"
	"testing"
)

func TestSystemLibraryFilteringAndPagination(t *testing.T) {
	items := []systemresource.Resource{{Kind: systemresource.Plugin, ID: 1, Name: "Map Lookup", Status: "published"}, {Kind: systemresource.Knowledge, ID: 2, Name: "浙江医疗机构", Description: "地址查询", Status: "published"}, {Kind: systemresource.Workflow, ID: 3, Name: "地址工作流", Status: "withdrawn"}}
	rows, total, counts := Filter(items, "all", "地址", "published", 1, 15)
	if total != 1 || len(rows) != 1 || rows[0].ID != 2 || counts["all"] != 3 || counts["plugin"] != 1 {
		t.Fatalf("资源搜索、状态或类型统计不正确：%+v %d %+v", rows, total, counts)
	}
	rows, total, _ = Filter(items, "plugin", "map", "", 1, 15)
	if total != 1 || len(rows) != 1 {
		t.Fatal("大小写搜索未匹配")
	}
	rows, total, _ = Filter(items, "all", "", "", 2, 2)
	if total != 3 || len(rows) != 1 || rows[0].ID != 3 {
		t.Fatal("分页边界不正确")
	}
	rows, total, _ = Filter(items, "database", "", "", 1, 15)
	if total != 0 || len(rows) != 0 {
		t.Fatal("空类型不应展示其他类型资源")
	}
}
