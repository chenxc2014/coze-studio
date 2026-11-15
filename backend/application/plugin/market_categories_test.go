package plugin

import (
	"context"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/coze-dev/coze-studio/backend/domain/plugin/dto"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/entity"
	mockPlugin "github.com/coze-dev/coze-studio/backend/internal/mock/domain/plugin"
)

func TestMarketCategorySearchAndPaging(t *testing.T) {
	plugins := []*entity.PluginInfo{
		createTestPluginInfo(14, "飞书认证", "身份授权"),
		createTestPluginInfo(15, "飞书消息", "团队消息"),
		createTestPluginInfo(16, "飞书多维表格", "团队数据"),
		createTestPluginInfo(12, "天眼查", "企业资料"),
	}
	page, total, more := filterMarketPlugins(plugins, 1005, " 团队 ", 1, 1)
	if total != 2 || len(page) != 1 || page[0].ID != 15 || !more {
		t.Fatal("分类与描述关键词须取交集，并在筛选后分页")
	}
	page, total, more = filterMarketPlugins(plugins, 1005, "团队", 2, 1)
	if total != 2 || len(page) != 1 || page[0].ID != 16 || more {
		t.Fatal("下一页不能重复数据或出现其他类别工具")
	}
	page, total, more = filterMarketPlugins(plugins, 1005, "天眼", 1, 20)
	if len(page) != 0 || total != 0 || more {
		t.Fatal("分类内搜索不能泄漏其他分类的同名结果")
	}
	page, total, more = filterMarketPlugins(plugins, 0, "", 2147483647, 100)
	if len(page) != 0 || total != 4 || more {
		t.Fatal("超出范围的分页须返回空列表，不能溢出或丢失匹配总数")
	}
}

func TestMarketCategoryAvailableCountsAndFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	domain.EXPECT().ListPluginProducts(gomock.Any(), gomock.Any()).Return(&dto.ListPluginProductsResponse{Plugins: []*entity.PluginInfo{createTestPluginInfo(6, "搜狐热闻", "新闻"), createTestPluginInfo(14, "飞书认证", "授权"), createTestPluginInfo(15, "飞书消息", "消息"), createTestPluginInfo(987654, "新插件", "尚未分类")}}, nil)
	service := &PluginApplicationService{DomainSVC: domain}
	response, err := service.GetMarketProductCategoryList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int32{}
	for _, category := range response.Data.Categories {
		counts[category.Name] = category.Count
	}
	if len(counts) != 3 || counts["新闻阅读"] != 1 || counts["效率办公"] != 2 || counts["实用工具"] != 1 {
		t.Fatal("只展示有工具的分类，新增未归类工具仍须可见")
	}
}

func TestMarketCategoryOrderStableAcrossRequests(t *testing.T) {
	first := createTestPluginInfo(14, "飞书认证", "授权")
	second := createTestPluginInfo(15, "飞书消息", "消息")
	third := createTestPluginInfo(16, "飞书多维表格", "表格")
	for _, input := range [][]*entity.PluginInfo{{third, first, second}, {second, third, first}, {first, second, third}} {
		for page := int32(1); page <= 3; page++ {
			rows, total, more := filterMarketPlugins(input, 1005, "", page, 1)
			if total != 3 || len(rows) != 1 || rows[0].ID != int64(page)+13 || more != (page < 3) {
				t.Fatal("跨请求输入顺序变化不能导致分页重复或遗漏")
			}
		}
	}
}
