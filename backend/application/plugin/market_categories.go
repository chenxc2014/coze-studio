package plugin

import (
	"context"
	"sort"
	"strings"

	productCommon "github.com/coze-dev/coze-studio/backend/api/model/marketplace/product_common"
	productAPI "github.com/coze-dev/coze-studio/backend/api/model/marketplace/product_public_api"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/dto"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/entity"
)

func (p *PluginApplicationService) GetMarketProductCategoryList(ctx context.Context) (*productAPI.GetProductCategoryListResponse, error) {
	result, err := p.DomainSVC.ListPluginProducts(ctx, &dto.ListPluginProductsRequest{})
	if err != nil {
		return nil, err
	}
	counts := map[int64]int32{}
	for _, plugin := range result.Plugins {
		counts[entity.MarketPluginCategory(plugin.ID).ID]++
	}
	categories := []*productAPI.ProductCategory{}
	for i, category := range entity.MarketCategories() {
		if counts[category.ID] == 0 {
			continue
		}
		categories = append(categories, &productAPI.ProductCategory{ID: category.ID, Name: category.Name, Count: counts[category.ID], Index: int32(i)})
	}
	return &productAPI.GetProductCategoryListResponse{Code: 0, Message: "success", Data: &productAPI.GetProductCategoryListData{EntityType: productCommon.ProductEntityType_Plugin, Categories: categories}}, nil
}

func filterMarketPlugins(plugins []*entity.PluginInfo, categoryID int64, keyword string, page, size int32) ([]*entity.PluginInfo, int32, bool) {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	filtered := []*entity.PluginInfo{}
	for _, plugin := range plugins {
		if categoryID != 0 && entity.MarketPluginCategory(plugin.ID).ID != categoryID {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(plugin.GetName()+" "+plugin.GetDesc()), keyword) {
			continue
		}
		filtered = append(filtered, plugin)
	}
	// 配置可能从 map 读取，且部分历史工具没有商品 ID；必须在分页前稳定排序。
	sort.Slice(filtered, func(i, j int) bool {
		left, right := filtered[i].GetRefProductID(), filtered[j].GetRefProductID()
		if left == right {
			return filtered[i].ID < filtered[j].ID
		}
		return left < right
	})
	total := len(filtered)
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	start := int64(page-1) * int64(size)
	if start >= int64(total) {
		return []*entity.PluginInfo{}, int32(total), false
	}
	end := min(start+int64(size), int64(total))
	return filtered[start:end], int32(total), end < int64(total)
}
