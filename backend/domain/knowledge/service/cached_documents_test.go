package service

import (
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/schema"
	knowledge "github.com/coze-dev/coze-studio/backend/crossdomain/knowledge/model"
	"github.com/coze-dev/coze-studio/backend/domain/knowledge/entity"
	"github.com/coze-dev/coze-studio/backend/infra/document"
)

func TestCachedTableDocumentsPreserveMetadata(t *testing.T) {
	const id int64 = 7706726398408261633
	text := "浙江省某医院"
	number := id
	doc := &schema.Document{Content: "表格内容", MetaData: map[string]any{
		document.MetaDataKeyCreatorID: id,
		document.MetaDataKeyColumns:   []*document.Column{{ID: id, Name: "医院", Type: document.TableColumnTypeString}},
		document.MetaDataKeyColumnData: []*document.ColumnData{
			{ColumnID: id, ColumnName: "医院", Type: document.TableColumnTypeString, ValString: &text},
			{ColumnID: id + 1, ColumnName: "编号", Type: document.TableColumnTypeInteger, ValInteger: &number},
		},
	}}
	encoded, err := json.Marshal([]*schema.Document{doc})
	if err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 2; round++ {
		docs, err := decodeCachedDocuments(encoded)
		if err != nil {
			t.Fatal(err)
		}
		values, err := document.GetDocumentColumnData(docs[0])
		if err != nil || len(values) != 2 || values[0].ColumnID != id || *values[1].ValInteger != id {
			t.Fatal("缓存恢复后表格类型或大整数丢失")
		}
		columns, err := document.GetDocumentColumns(docs[0])
		if err != nil || columns[0].ID != id {
			t.Fatal("缓存恢复后表头丢失")
		}
		creator, err := document.GetDocumentCreatorID(docs[0])
		if err != nil || creator != id {
			t.Fatal("缓存恢复后创建人 ID 丢失精度")
		}
		slice, err := d2sMapping[knowledge.DocumentTypeTable](docs[0], 1, 2, id)
		if err != nil {
			t.Fatal(err)
		}
		packed, err := packInsertData([]*entity.Slice{slice})
		if err != nil || len(packed) != 1 {
			t.Fatal("缓存恢复后不能重新生成入库数据")
		}
		encoded, err = json.Marshal(docs)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestCachedDocumentsTextAndHeaders(t *testing.T) {
	text := &schema.Document{Content: "普通文本"}
	headers := document.WithDocumentColumnsOnly(&schema.Document{MetaData: map[string]any{}})
	data, _ := json.Marshal([]*schema.Document{text, headers})
	docs, err := decodeCachedDocuments(data)
	if err != nil || docs[0].Content != text.Content {
		t.Fatal("普通文本缓存恢复失败")
	}
	only, err := document.GetDocumentsColumnsOnly(docs[1:])
	if err != nil || !only {
		t.Fatal("空表头标记没有恢复")
	}
	if _, err := decodeCachedDocuments([]byte(`[null]`)); err == nil {
		t.Fatal("不应接受空缓存条目")
	}
}

func TestPackInsertDataRejectsMalformedRows(t *testing.T) {
	for _, slice := range []*entity.Slice{nil, {}, {RawContent: []*knowledge.SliceContent{nil}}, {RawContent: []*knowledge.SliceContent{{Type: knowledge.SliceContentTypeTable}}}} {
		if _, err := packInsertData([]*entity.Slice{slice}); err == nil {
			t.Fatal("缺少表格数据时应返回可定位错误")
		}
	}
}
