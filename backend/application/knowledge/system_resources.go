package knowledge

import (
	"context"
	"fmt"

	dataset "github.com/coze-dev/coze-studio/backend/api/model/data/knowledge"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	"github.com/coze-dev/coze-studio/backend/domain/knowledge/service"
	"github.com/coze-dev/coze-studio/backend/domain/systemresource"
	"github.com/coze-dev/coze-studio/backend/pkg/lang/conv"
)

func protectKnowledgeWrite(ctx context.Context, actor int64, knowledgeID *int64, documents, slices []int64) error {
	ids := []int64{}
	if knowledgeID != nil {
		ids = append(ids, *knowledgeID)
	}
	for _, source := range []struct {
		ids   []int64
		table string
	}{{documents, "knowledge_document"}, {slices, "knowledge_document_slice"}} {
		parents, err := systemresource.KnowledgeParents(ctx, source.ids, source.table)
		if err != nil {
			return err
		}
		ids = append(ids, parents...)
	}
	for _, id := range ids {
		if err := systemresource.ProtectWrite(ctx, systemresource.Knowledge, id, actor); err != nil {
			return err
		}
	}
	return nil
}

func (k *KnowledgeApplicationService) checkReadPermission(ctx context.Context, uid *int64, space *int64, documents []int64, knowledgeID *int64, slices []int64) error {
	if uid == nil {
		return fmt.Errorf("请先登录平台")
	}
	if knowledgeID != nil {
		shared, err := systemresource.Published(ctx, systemresource.Knowledge, *knowledgeID)
		if err != nil {
			return err
		}
		if shared {
			knowledgeID = nil
		}
	}
	filter := func(ids []int64, table string) ([]int64, error) {
		var left []int64
		for _, id := range ids {
			parents, err := systemresource.KnowledgeParents(ctx, []int64{id}, table)
			if err != nil {
				return nil, err
			}
			shared := false
			if len(parents) == 1 {
				shared, err = systemresource.Published(ctx, systemresource.Knowledge, parents[0])
				if err != nil {
					return nil, err
				}
			}
			if !shared {
				left = append(left, id)
			}
		}
		return left, nil
	}
	var err error
	documents, err = filter(documents, "knowledge_document")
	if err != nil {
		return err
	}
	slices, err = filter(slices, "knowledge_document_slice")
	if err != nil {
		return err
	}
	return k.checkPermission(ctx, uid, space, documents, knowledgeID, slices)
}

func (k *KnowledgeApplicationService) datasetDetailWithShared(ctx context.Context, req *dataset.DatasetDetailRequest) (*dataset.DatasetDetailResponse, error) {
	uid := ctxutil.GetUIDFromCtx(ctx)
	if uid == nil {
		return nil, fmt.Errorf("请先登录平台")
	}
	if len(req.GetDatasetIDs()) > 100 {
		return nil, fmt.Errorf("每次最多读取 100 个知识库")
	}
	ids := []int64{}
	for _, raw := range req.GetDatasetIDs() {
		id, err := conv.StrToInt64(raw)
		if err != nil {
			return nil, err
		}
		if err = k.checkReadPermission(ctx, uid, nil, nil, &id, nil); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	resp := dataset.NewDatasetDetailResponse()
	if len(ids) == 0 {
		return resp, nil
	}
	rows, err := k.DomainSVC.ListKnowledge(ctx, &service.ListKnowledgeRequest{IDs: ids})
	if err != nil {
		return nil, err
	}
	mapped, err := batchConvertKnowledgeEntity2Model(ctx, rows.KnowledgeList)
	if err != nil {
		return nil, err
	}
	resp.DatasetDetails = map[string]*dataset.Dataset{}
	for id, item := range mapped {
		resp.DatasetDetails[conv.Int64ToStr(id)] = item
	}
	return resp, nil
}
