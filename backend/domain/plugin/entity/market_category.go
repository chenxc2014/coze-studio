package entity

type MarketCategory struct {
	ID   int64
	Name string
}

// 分类 ID 保持稳定；新增搜索工具时在此补充归类，不影响独立的系统资源库。
var marketCategories = []MarketCategory{
	{1001, "网页搜索"},
	{1002, "新闻阅读"},
	{1003, "便利生活"},
	{1004, "图像"},
	{1005, "效率办公"},
	{1006, "科学与教育"},
	{1007, "金融与商业"},
	{1008, "实用工具"},
}

var marketPluginCategoryIDs = map[int64]int64{
	2: 1006, 3: 1001, 4: 1006, 5: 1004, 6: 1002,
	9: 1004, 10: 1003, 11: 1005, 12: 1007,
	14: 1005, 15: 1005, 16: 1005, 17: 1005,
	18: 1005, 19: 1005, 20: 1005, 21: 1005, 22: 1003,
}

func MarketCategories() []MarketCategory {
	return append([]MarketCategory(nil), marketCategories...)
}

// 未配置的新工具归入“实用工具”，始终保留在“全部”及对应分类中。
func MarketPluginCategory(pluginID int64) MarketCategory {
	id, ok := marketPluginCategoryIDs[pluginID]
	if !ok {
		id = 1008
	}
	for _, category := range marketCategories {
		if category.ID == id {
			return category
		}
	}
	return marketCategories[len(marketCategories)-1]
}
