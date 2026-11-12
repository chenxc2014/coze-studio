package service

import (
	"context"
	"encoding/json"
	"errors"
	knowledgeModel "github.com/coze-dev/coze-studio/backend/crossdomain/knowledge/model"
	"github.com/coze-dev/coze-studio/backend/domain/knowledge/entity"
	"github.com/coze-dev/coze-studio/backend/infra/cache"
	"testing"
	"time"
)

type apiPreparationCache struct {
	cache.Cmdable
	data []byte
}
type apiPreparationValue struct {
	cache.StringCmd
	data []byte
}

func (c apiPreparationCache) Get(context.Context, string) cache.StringCmd {
	return apiPreparationValue{data: c.data}
}
func (v apiPreparationValue) Bytes() ([]byte, error) {
	if v.data == nil {
		return nil, errors.New("expired")
	}
	return v.data, nil
}

func TestAPITablePreparationOwnership(t *testing.T) {
	p := APITablePreparation{URI: "api-table/11/random.csv", Name: "API 数据表", OwnerID: 11, Source: &entity.APISource{URL: "https://example.com/data.json", IntervalDays: 3}}
	data, _ := json.Marshal(p)
	for _, tc := range []struct {
		name            string
		owner           int64
		expired, append bool
		want            bool
	}{
		{"拥有者可以导入", 11, false, false, true},
		{"其他账号不能复用预览", 12, false, false, false},
		{"过期预览不能导入", 11, true, false, false},
		{"禁止追加破坏原表", 11, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := data
			if tc.expired {
				value = nil
			}
			svc := &knowledgeSVC{cacheCli: apiPreparationCache{data: value}}
			doc := &entity.Document{Info: knowledgeModel.Info{CreatorID: tc.owner}, URI: p.URI, Type: knowledgeModel.DocumentTypeTable, IsAppend: tc.append}
			err := svc.bindAPIPreparations(context.Background(), &CreateDocumentRequest{Documents: []*entity.Document{doc}})
			if (err == nil) != tc.want {
				t.Fatalf("导入权限不正确: %v", err)
			}
			if tc.want && (doc.Source != entity.DocumentSourceAPI || doc.APISource.URL != p.Source.URL || doc.Name == "") {
				t.Fatal("来源配置丢失")
			}
		})
	}
}

func TestAPITableSchedulePersistence(t *testing.T) {
	now := time.Now().UnixMilli()
	if nextAPIRun(now, 0) != 0 || nextAPIRun(now, 3)-now != int64(72*time.Hour/time.Millisecond) {
		t.Fatal("更新频率未按天计算")
	}
	data, _ := json.Marshal(entity.APISource{IntervalDays: 3, NextRunAt: nextAPIRun(now, 3)})
	var restored entity.APISource
	if json.Unmarshal(data, &restored) != nil || restored.NextRunAt != nextAPIRun(now, 3) {
		t.Fatal("重启所需的到期时间丢失")
	}
}
