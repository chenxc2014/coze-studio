package knowledge

import (
	"context"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	"github.com/coze-dev/coze-studio/backend/domain/knowledge/service"
)

type AddressPermissionError struct{}

func (*AddressPermissionError) Error() string { return "无权读取该知识文档" }

func (k *KnowledgeApplicationService) QueryAddress(ctx context.Context, req *service.AddressQueryRequest) (*service.AddressQueryResponse, error) {
	uid := ctxutil.GetUIDFromCtx(ctx)
	if uid == nil || req == nil {
		return nil, &AddressPermissionError{}
	}
	if err := k.checkReadPermission(ctx, uid, nil, []int64{req.DocumentID}, nil, nil); err != nil {
		return nil, &AddressPermissionError{}
	}
	return k.DomainSVC.QueryAddress(ctx, req)
}
