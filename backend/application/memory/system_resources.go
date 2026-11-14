package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/coze-dev/coze-studio/backend/api/model/data/database/table"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	dbmodel "github.com/coze-dev/coze-studio/backend/crossdomain/database/model"
	databaseEntity "github.com/coze-dev/coze-studio/backend/domain/memory/database/entity"
	dbsvc "github.com/coze-dev/coze-studio/backend/domain/memory/database/service"
	"github.com/coze-dev/coze-studio/backend/domain/systemresource"
)

func (d *DatabaseApplicationService) ValidateReadAccess(ctx context.Context, id int64, kind table.TableType) error {
	if ctxutil.GetUIDFromCtx(ctx) == nil {
		return fmt.Errorf("请先登录平台")
	}
	shared, err := systemresource.Published(ctx, systemresource.Database, id)
	if err != nil {
		return err
	}
	if shared {
		return nil
	}
	return d.ValidateAccess(ctx, id, kind)
}

func (d *DatabaseApplicationService) listSystemDatabases(ctx context.Context, req *table.ListDatabaseRequest) (*table.ListDatabaseResponse, error) {
	ids, err := systemresource.IDs(ctx, systemresource.Database)
	if err != nil {
		return nil, err
	}
	basics := []*dbmodel.DatabaseBasic{}
	for _, id := range ids {
		if req.TableType == table.TableType_DraftTable {
			var draft int64
			if err = systemresource.Default().DB.WithContext(ctx).Table("online_database_info").Select("related_draft_id").Where("id = ? AND deleted_at IS NULL", id).Scan(&draft).Error; err != nil {
				return nil, err
			}
			if draft <= 0 {
				continue
			}
			id = draft
		}
		basics = append(basics, &dbmodel.DatabaseBasic{ID: id, TableType: req.TableType})
	}
	if len(basics) == 0 {
		return &table.ListDatabaseResponse{DatabaseInfoList: []*table.DatabaseInfo{}}, nil
	}
	rows, err := d.DomainSVC.MGetDatabase(ctx, &dbsvc.MGetDatabaseRequest{Basics: basics})
	if err != nil {
		return nil, err
	}
	visible := rows.Databases[:0]
	for _, item := range rows.Databases {
		if req.GetTableName() != "" && !strings.Contains(strings.ToLower(item.TableName), strings.ToLower(req.GetTableName())) {
			continue
		}
		copy := *item
		copy.RwMode = table.BotTableRWMode_ReadOnly
		visible = append(visible, &copy)
	}
	total := len(visible)
	offset := int(req.GetOffset())
	if offset < 0 {
		offset = 0
	}
	offset = min(offset, total)
	size := int(req.GetLimit())
	if size < 1 || size > 100 {
		size = 50
	}
	end := min(offset+size, total)
	bound := []*databaseEntity.Database{}
	if req.GetBotID() != 0 {
		binding, err := d.DomainSVC.MGetDatabaseByAgentID(ctx, &dbsvc.MGetDatabaseByAgentIDRequest{AgentID: req.GetBotID(), TableType: req.GetTableType(), NeedSysFields: false})
		if err != nil {
			return nil, err
		}
		bound = binding.Databases
	}
	return convertListDatabaseRes(&dbsvc.ListDatabaseResponse{Databases: visible[offset:end], TotalCount: int64(total), HasMore: end < total}, bound), nil
}
