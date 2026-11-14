package workflow

import (
	"context"
	"fmt"

	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	wfmodel "github.com/coze-dev/coze-studio/backend/crossdomain/workflow/model"
	"github.com/coze-dev/coze-studio/backend/domain/systemresource"
	"github.com/coze-dev/coze-studio/backend/domain/workflow/entity"
	"github.com/coze-dev/coze-studio/backend/domain/workflow/entity/vo"
)

func (w *ApplicationService) checkReadableWorkflows(ctx context.Context, ids []int64) error {
	uid := ctxutil.GetUIDFromCtx(ctx)
	if uid == nil {
		return fmt.Errorf("请先登录平台")
	}
	for _, id := range ids {
		shared, err := systemresource.Published(ctx, systemresource.Workflow, id)
		if err != nil {
			return err
		}
		if shared {
			continue
		}
		meta, err := w.DomainSVC.Get(ctx, &vo.GetPolicy{ID: id, MetaOnly: true})
		if err != nil {
			return err
		}
		if err = checkUserSpace(ctx, *uid, meta.SpaceID); err != nil {
			return err
		}
	}
	return nil
}

func (w *ApplicationService) usePublishedSystemWorkflows(ctx context.Context, items []*entity.Workflow) error {
	for index, item := range items {
		p, err := systemresource.Default().Lookup(ctx, systemresource.Workflow, item.ID)
		if err != nil {
			return err
		}
		if p == nil || p.Status != "published" {
			continue
		}
		published, err := w.DomainSVC.Get(ctx, &vo.GetPolicy{ID: item.ID, QType: wfmodel.FromSpecificVersion, Version: p.Version})
		if err != nil {
			return err
		}
		items[index] = published
	}
	return nil
}
