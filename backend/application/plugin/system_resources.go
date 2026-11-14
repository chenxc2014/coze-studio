package plugin

import (
	"context"
	"strconv"
	"strings"

	product "github.com/coze-dev/coze-studio/backend/api/model/marketplace/product_public_api"
	"github.com/coze-dev/coze-studio/backend/domain/systemresource"
)

// 系统插件选择器与原有搜索工具分开取数，仅返回平台明确发布的插件。
func (p *PluginApplicationService) SystemPluginProducts(ctx context.Context, query, rawPage string) (*product.GetProductListData, error) {
	ids, err := systemresource.IDs(ctx, systemresource.Plugin)
	if err != nil {
		return nil, err
	}
	result := &product.GetProductListData{Products: []*product.ProductInfo{}}
	if len(ids) == 0 {
		return result, nil
	}
	plugins, err := p.DomainSVC.MGetOnlinePlugins(ctx, ids)
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	for _, item := range plugins {
		if query != "" && !strings.Contains(strings.ToLower(item.GetName()+" "+item.GetDesc()), query) {
			continue
		}
		tools, err := p.toolRepo.GetPluginAllOnlineTools(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		info, err := p.buildProductInfo(ctx, item, tools)
		if err != nil {
			return nil, err
		}
		info.MetaInfo.UserInfo.Name = "平台系统资源"
		result.Products = append(result.Products, info)
	}
	page, _ := strconv.Atoi(rawPage)
	if page < 1 || page > 1000000 {
		page = 1
	}
	count := len(result.Products)
	start := min((page-1)*20, count)
	end := min(start+20, count)
	result.Total = int32(count)
	result.HasMore = end < count
	result.Products = result.Products[start:end]
	return result, nil
}
