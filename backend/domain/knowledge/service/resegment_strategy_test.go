package service

import (
	"github.com/coze-dev/coze-studio/backend/domain/knowledge/entity"
	"testing"
)

func TestResegmentTablePreservesSheetRange(t *testing.T) {
	original := &entity.ParsingStrategy{SheetID: 2, HeaderLine: 3, DataStartLine: 4, RowsCount: 5}
	requested := &entity.ParsingStrategy{ImageOCR: true}
	got := retainTableSheetRange(original, requested)
	if got.SheetID != 2 || got.HeaderLine != 3 || got.DataStartLine != 4 || got.RowsCount != 5 || !got.ImageOCR {
		t.Fatal("表格重试丢失原始工作表范围或新解析设置")
	}
	if requested.DataStartLine != 0 || original.ImageOCR {
		t.Fatal("不应修改输入策略")
	}
	if retainTableSheetRange(original, nil) != original {
		t.Fatal("未提供新策略时应保留原始设置")
	}
	if retainTableSheetRange(nil, requested) != requested {
		t.Fatal("新表格应保留请求设置")
	}
}
