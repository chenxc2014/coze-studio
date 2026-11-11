package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"

	"github.com/coze-dev/coze-studio/backend/infra/document"
)

// 缓存里的 MetaData 经通用 JSON 反序列化后会丢失表格类型；从原始字节恢复，避免大整数 ID 经 float64 丢失精度。
func decodeCachedDocuments(data []byte) ([]*schema.Document, error) {
	var docs []*schema.Document
	if err := json.Unmarshal(data, &docs); err != nil {
		return nil, err
	}
	var raw []map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	for i, doc := range docs {
		if doc == nil {
			return nil, fmt.Errorf("cached document %d is nil", i)
		}
		var metadata map[string]json.RawMessage
		for key, value := range raw[i] {
			if strings.EqualFold(strings.ReplaceAll(key, "_", ""), "metadata") {
				if err := json.Unmarshal(value, &metadata); err != nil {
					return nil, err
				}
				break
			}
		}
		for key, value := range metadata {
			var target any
			switch key {
			case document.MetaDataKeyColumns:
				target = new([]*document.Column)
			case document.MetaDataKeyColumnData:
				target = new([]*document.ColumnData)
			case document.MetaDataKeyCreatorID:
				target = new(int64)
			case document.MetaDataKeyColumnsOnly:
				doc.MetaData[key] = struct{}{}
				continue
			default:
				continue
			}
			if err := json.Unmarshal(value, target); err != nil {
				return nil, fmt.Errorf("invalid cached document metadata %s: %w", key, err)
			}
			switch v := target.(type) {
			case *[]*document.Column:
				doc.MetaData[key] = *v
			case *[]*document.ColumnData:
				doc.MetaData[key] = *v
			case *int64:
				doc.MetaData[key] = *v
			}
		}
	}
	return docs, nil
}
