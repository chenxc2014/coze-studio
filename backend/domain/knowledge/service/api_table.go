package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	knowledgeModel "github.com/coze-dev/coze-studio/backend/crossdomain/knowledge/model"
	"github.com/coze-dev/coze-studio/backend/domain/knowledge/apitable"
	"github.com/coze-dev/coze-studio/backend/domain/knowledge/entity"
	"github.com/coze-dev/coze-studio/backend/domain/knowledge/internal/convert"
	"github.com/coze-dev/coze-studio/backend/domain/knowledge/internal/dal/model"
	"github.com/coze-dev/coze-studio/backend/domain/knowledge/internal/events"
	"github.com/coze-dev/coze-studio/backend/infra/eventbus"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
	"gorm.io/gorm"
)

type APITableService interface {
	PrepareAPITable(context.Context, int64, string, int) (*APITablePreparation, error)
	GetAPITableSource(context.Context, int64, int64) (*entity.APISource, error)
	RefreshAPITable(context.Context, int64, int64, *int, bool) error
	StartAPITableRefresh(context.Context)
}

type APITablePreparation struct {
	URI     string            `json:"uri"`
	Name    string            `json:"name"`
	Rows    int               `json:"rows"`
	Columns []string          `json:"columns"`
	Size    int               `json:"size"`
	OwnerID int64             `json:"owner_id,string"`
	Source  *entity.APISource `json:"source"`
}

var apiFetchSlots = make(chan struct{}, 2)

func (k *knowledgeSVC) saveAPITable(ctx context.Context, doc *model.KnowledgeDocument) error {
	rule, err := json.Marshal(doc.ParseRule)
	if err != nil {
		return err
	}
	result := k.apiDB.WithContext(ctx).Model(&model.KnowledgeDocument{}).Where("id = ?", doc.ID).Updates(map[string]any{
		"uri": doc.URI, "parse_rule": string(rule), "status": doc.Status, "fail_reason": doc.FailReason, "updated_at": time.Now().UnixMilli(),
	})
	if result.Error != nil {
		return errors.New("保存 API 更新状态失败")
	}
	if result.RowsAffected == 0 {
		return errors.New("文档已删除或状态已变化")
	}
	return nil
}

func fetchAPITable(ctx context.Context, raw string) (*apitable.Table, error) {
	select {
	case apiFetchSlots <- struct{}{}:
		defer func() { <-apiFetchSlots }()
	case <-ctx.Done():
		return nil, errors.New("接口抓取繁忙，请稍后重试")
	}
	return apitable.Fetch(ctx, raw)
}

func (k *knowledgeSVC) PrepareAPITable(ctx context.Context, owner int64, raw string, interval int) (*APITablePreparation, error) {
	if owner <= 0 || !apitable.ValidInterval(interval) {
		return nil, errors.New("账号或更新频率不正确")
	}
	if k.cacheCli == nil {
		return nil, errors.New("导入缓存暂不可用")
	}
	table, err := fetchAPITable(ctx, raw)
	if err != nil {
		return nil, err
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	uri := fmt.Sprintf("api-table/%d/%s.csv", owner, hex.EncodeToString(nonce[:]))
	hash := sha256.Sum256(table.CSV)
	now := time.Now().UnixMilli()
	p := &APITablePreparation{URI: uri, Name: "API 数据表", Rows: table.Rows, Columns: table.Columns, Size: len(table.CSV), OwnerID: owner,
		Source: &entity.APISource{URL: strings.TrimSpace(raw), IntervalDays: interval, Columns: table.Columns, PendingHash: hex.EncodeToString(hash[:]), LastFetchedAt: now, NextRunAt: nextAPIRun(now, interval)}}
	if err = k.storage.PutObject(ctx, uri, table.CSV); err != nil {
		return nil, errors.New("暂存 API 数据失败")
	}
	data, _ := json.Marshal(p)
	if err = k.cacheCli.Set(ctx, "api_table_prepare:"+uri, string(data), 24*time.Hour).Err(); err != nil {
		_ = k.storage.DeleteObject(ctx, uri)
		return nil, errors.New("暂存 API 导入配置失败")
	}
	return p, nil
}

func (k *knowledgeSVC) bindAPIPreparations(ctx context.Context, req *CreateDocumentRequest) error {
	if req == nil {
		return errors.New("导入请求不能为空")
	}
	for _, doc := range req.Documents {
		if doc == nil {
			return errors.New("导入文档不能为空")
		}
		if !strings.HasPrefix(doc.URI, "api-table/") {
			if doc.Source == entity.DocumentSourceAPI {
				return errors.New("请先抓取 API 数据")
			}
			continue
		}
		if doc.APISource != nil {
			continue
		} // 仅内部可信流程会携带完整实体。
		if doc.IsAppend || doc.Type != knowledgeModel.DocumentTypeTable || k.cacheCli == nil {
			return errors.New("API 来源仅支持独立表格导入，不支持追加")
		}
		data, err := k.cacheCli.Get(ctx, "api_table_prepare:"+doc.URI).Bytes()
		var p APITablePreparation
		if err != nil || json.Unmarshal(data, &p) != nil || p.OwnerID != doc.CreatorID || p.URI != doc.URI || p.Source == nil {
			return errors.New("API 预览已失效或不属于当前账号，请重新抓取")
		}
		doc.APISource = p.Source
		doc.Source = entity.DocumentSourceAPI
		if strings.TrimSpace(doc.Name) == "" {
			doc.Name = p.Name
		}
	}
	return nil
}

func nextAPIRun(now int64, days int) int64 {
	if days == 0 {
		return 0
	}
	return now + int64(days)*int64(24*time.Hour/time.Millisecond)
}

func (k *knowledgeSVC) GetAPITableSource(ctx context.Context, owner, id int64) (*entity.APISource, error) {
	doc, err := k.documentRepo.GetByID(ctx, id)
	if err != nil || doc == nil || doc.CreatorID != owner || doc.ParseRule == nil || doc.ParseRule.APISource == nil {
		return nil, errors.New("API 文档不存在或无权访问")
	}
	src := *doc.ParseRule.APISource
	src.PreviousURI = ""
	src.Hash = ""
	src.PendingHash = ""
	return &src, nil
}

// MySQL 命名锁绑定专用连接，覆盖抓取、更新元数据与投递任务，避免多实例重复更新。
func (k *knowledgeSVC) RefreshAPITable(ctx context.Context, owner, id int64, interval *int, force bool) error {
	if interval != nil && !apitable.ValidInterval(*interval) {
		return errors.New("更新频率只支持 0、1、3、7、30 天")
	}
	return k.apiDB.WithContext(ctx).Connection(func(conn *gorm.DB) error {
		name := fmt.Sprintf("coze_api_table_%d", id)
		var locked int
		if err := conn.Raw("SELECT GET_LOCK(?, 0)", name).Scan(&locked).Error; err != nil {
			return err
		}
		if locked != 1 {
			return errors.New("该 API 文档正在更新，请稍后重试")
		}
		defer conn.WithContext(context.Background()).Exec("SELECT RELEASE_LOCK(?)", name)
		doc, err := k.documentRepo.GetByID(ctx, id)
		if err != nil || doc == nil || (owner != 0 && doc.CreatorID != owner) || doc.ParseRule == nil || doc.ParseRule.APISource == nil {
			return errors.New("API 文档不存在或无权访问")
		}
		src := doc.ParseRule.APISource
		now := time.Now().UnixMilli()
		if interval != nil {
			src.IntervalDays = *interval
			src.NextRunAt = nextAPIRun(now, *interval)
			return k.saveAPITable(ctx, doc)
		}
		if !force && (src.IntervalDays == 0 || src.NextRunAt > now) {
			return nil
		}
		if doc.Status != int32(entity.DocumentStatusEnable) && doc.Status != int32(entity.DocumentStatusFailed) {
			return errors.New("数据正在处理，完成后再更新")
		}
		table, fetchErr := fetchAPITable(ctx, src.URL)
		src.LastFetchedAt = now
		// 失败后最多每小时重试一次；不会因外部接口故障删除现有数据。
		src.NextRunAt = nextAPIRun(now, src.IntervalDays)
		if fetchErr == nil && !reflect.DeepEqual(src.Columns, table.Columns) {
			fetchErr = errors.New("API 字段结构发生变化，请创建新表或调整接口；原数据保留")
		}
		// 先用当前列类型完整试解析，类型不兼容时不进入清表/重新索引流程。
		if fetchErr == nil {
			candidate, e := k.fromModelDocument(ctx, doc)
			if e == nil {
				config := convert.DocumentToParseConfig(candidate)
				config.ParsingStrategy.IgnoreColumnTypeErr = false
				p, pe := k.parseManager.GetParser(config)
				e = pe
				if e == nil {
					_, e = p.Parse(ctx, bytes.NewReader(table.CSV))
				}
			}
			if e != nil {
				fetchErr = errors.New("API 数据与已配置字段类型不兼容，原数据保留，请检查接口内容")
			}
		}
		if fetchErr != nil {
			src.LastError = fetchErr.Error()
			if src.IntervalDays > 0 {
				src.NextRunAt = now + int64(time.Hour/time.Millisecond)
			}
			if err = k.saveAPITable(ctx, doc); err != nil {
				return err
			}
			return fetchErr
		}
		hash := sha256.Sum256(table.CSV)
		digest := hex.EncodeToString(hash[:])
		if digest == src.Hash && doc.Status == int32(entity.DocumentStatusEnable) {
			src.LastError = ""
			src.LastSuccessAt = now
			return k.saveAPITable(ctx, doc)
		}
		var nonce [16]byte
		if _, err = rand.Read(nonce[:]); err != nil {
			return err
		}
		uri := fmt.Sprintf("api-table/%d/%s.csv", doc.CreatorID, hex.EncodeToString(nonce[:]))
		if err = k.storage.PutObject(ctx, uri, table.CSV); err != nil {
			return errors.New("暂存更新数据失败")
		}
		if k.cacheCli != nil {
			if err = k.cacheCli.Del(ctx, fmt.Sprintf(indexDocCacheKey, doc.KnowledgeID, doc.ID)).Err(); err != nil {
				_ = k.storage.DeleteObject(ctx, uri)
				return errors.New("清理旧解析进度失败，请稍后重试")
			}
		}
		oldURI := doc.URI
		oldRule, _ := json.Marshal(doc.ParseRule)
		oldStatus := doc.Status
		src.PreviousURI = oldURI
		src.PendingHash = digest
		src.LastError = ""
		doc.URI = uri
		doc.Status = int32(entity.DocumentStatusChunking)
		doc.FailReason = ""
		if err = k.saveAPITable(ctx, doc); err != nil {
			_ = k.storage.DeleteObject(ctx, uri)
			return err
		}
		docEntity, err := k.fromModelDocument(ctx, doc)
		if err == nil {
			body, e := json.Marshal(events.NewIndexDocumentEvent(doc.KnowledgeID, docEntity))
			err = e
			if err == nil {
				err = k.producer.Send(ctx, body, eventbus.WithShardingKey(fmt.Sprint(doc.KnowledgeID)))
			}
		}
		if err != nil {
			doc.URI = oldURI
			doc.Status = oldStatus
			_ = json.Unmarshal(oldRule, &doc.ParseRule)
			doc.ParseRule.APISource.LastError = "投递更新任务失败，请稍后重试"
			_ = k.saveAPITable(ctx, doc)
			_ = k.storage.DeleteObject(ctx, uri)
			return errors.New("投递更新任务失败，请稍后重试")
		}
		return nil
	})
}

func (k *knowledgeSVC) finishAPITable(ctx context.Context, id int64) error {
	doc, err := k.documentRepo.GetByID(ctx, id)
	if err != nil || doc == nil || doc.ParseRule == nil || doc.ParseRule.APISource == nil {
		return err
	}
	src := doc.ParseRule.APISource
	if src.PendingHash != "" {
		src.Hash = src.PendingHash
		src.PendingHash = ""
	}
	src.PreviousURI = ""
	src.LastSuccessAt = time.Now().UnixMilli()
	src.LastError = ""
	if err = k.saveAPITable(ctx, doc); err != nil {
		return err
	}
	return nil
}

// 清理过期预览、被替换及已删除文档的 API CSV；被现有文档或在途更新引用的文件保留。
func (k *knowledgeSVC) cleanupAPIObjects(ctx context.Context, cursor string) string {
	page, err := k.storage.ListObjectsPaginated(ctx, &storage.ListObjectsPaginatedInput{Prefix: "api-table/", PageSize: 100, Cursor: cursor})
	if err != nil {
		return cursor
	}
	for _, file := range page.Files {
		if file.LastModified.IsZero() || time.Since(file.LastModified) < 48*time.Hour {
			continue
		}
		var references int64
		if err = k.apiDB.WithContext(ctx).Model(&model.KnowledgeDocument{}).Where("uri = ? OR JSON_UNQUOTE(JSON_EXTRACT(parse_rule, '$.api_source.previous_uri')) = ?", file.Key, file.Key).Count(&references).Error; err != nil {
			return cursor
		}
		if references == 0 {
			_ = k.storage.DeleteObject(ctx, file.Key)
		}
	}
	if page.IsTruncated {
		return page.Cursor
	}
	return ""
}

func (k *knowledgeSVC) StartAPITableRefresh(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		var cleanupCursor string
		var lastCleanup time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if time.Since(lastCleanup) >= time.Hour {
					job, cancel := context.WithTimeout(ctx, 20*time.Second)
					cleanupCursor = k.cleanupAPIObjects(job, cleanupCursor)
					cancel()
					lastCleanup = time.Now()
				}
				var docs []*model.KnowledgeDocument
				// JSON 内的 next_run_at 持久化，服务重启后继续到期任务。
				err := k.apiDB.WithContext(ctx).Where("source_type = ? AND status IN (1, 9) AND deleted_at IS NULL AND JSON_EXTRACT(parse_rule, '$.api_source.interval_days') > 0 AND JSON_EXTRACT(parse_rule, '$.api_source.next_run_at') <= ?", entity.DocumentSourceAPI, time.Now().UnixMilli()).Order("updated_at ASC").Limit(100).Find(&docs).Error
				if err != nil {
					logs.CtxErrorf(ctx, "API 表格定时扫描失败")
					continue
				}
				for _, doc := range docs {
					job, cancel := context.WithTimeout(ctx, 45*time.Second)
					err = k.RefreshAPITable(job, 0, doc.ID, nil, false)
					cancel()
					if err != nil {
						logs.CtxWarnf(ctx, "API 表格更新未完成 document_id=%d", doc.ID)
					}
				}
			}
		}
	}()
}
