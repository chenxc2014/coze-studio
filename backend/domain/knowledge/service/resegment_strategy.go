package service

import "github.com/coze-dev/coze-studio/backend/domain/knowledge/entity"

// 重新处理接口没有工作表和行范围字段，保留原始选择，避免重试时把表头当作数据或切换工作表。
func retainTableSheetRange(original, requested *entity.ParsingStrategy) *entity.ParsingStrategy {
	if requested == nil {
		return original
	}
	if original == nil {
		return requested
	}
	merged := *requested
	merged.SheetID = original.SheetID
	merged.HeaderLine = original.HeaderLine
	merged.DataStartLine = original.DataStartLine
	merged.RowsCount = original.RowsCount
	return &merged
}
