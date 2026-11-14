package systemresource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

const PublicSpaceID int64 = 999999

type Kind string
type pinnedVersionKey struct{}

func PinVersion(ctx context.Context) context.Context {
	return context.WithValue(ctx, pinnedVersionKey{}, true)
}
func IsVersionPinned(ctx context.Context) bool {
	value, _ := ctx.Value(pinnedVersionKey{}).(bool)
	return value
}

const (
	Plugin    Kind = "plugin"
	Workflow  Kind = "workflow"
	Prompt    Kind = "prompt"
	Knowledge Kind = "knowledge"
	Database  Kind = "database"
)

func ValidKind(kind Kind) bool {
	return kind == Plugin || kind == Workflow || kind == Prompt || kind == Knowledge || kind == Database
}

type Field struct {
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Description string  `json:"description,omitempty"`
	Required    bool    `json:"required,omitempty"`
	Children    []Field `json:"children,omitempty"`
}

type Tool struct {
	ID          int64   `json:"id,string"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Fields      []Field `json:"fields,omitempty"`
}

// 对外元数据只包含使用说明和参数结构，禁止放入凭据、连接串或请求默认值。
type Resource struct {
	Kind        Kind    `json:"kind"`
	ID          int64   `json:"id,string"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Icon        string  `json:"icon,omitempty"`
	Version     string  `json:"version"`
	Origin      string  `json:"origin"`
	Status      string  `json:"status"`
	Revision    int64   `json:"revision"`
	PublishedAt int64   `json:"published_at"`
	ReadOnly    bool    `json:"read_only"`
	Usage       string  `json:"usage"`
	Fields      []Field `json:"fields,omitempty"`
	Tools       []Tool  `json:"tools,omitempty"`
	Content     string  `json:"content,omitempty"`
	Notice      string  `json:"notice,omitempty"`
}

type Publication struct {
	Kind          Kind   `gorm:"primaryKey" json:"kind"`
	ResourceID    int64  `gorm:"primaryKey;autoIncrement:false" json:"resource_id,string"`
	SourceSpaceID int64  `json:"source_space_id,string"`
	SourceOwnerID int64  `json:"source_owner_id,string"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Icon          string `json:"icon"`
	Version       string `json:"version"`
	Origin        string `json:"origin"`
	Status        string `json:"status"`
	Revision      int64  `json:"revision"`
	MetadataJSON  string `json:"-"`
	PublishedBy   int64  `json:"published_by,string"`
	UpdatedBy     int64  `json:"updated_by,string"`
	CreatedAt     int64  `gorm:"autoCreateTime:milli" json:"created_at"`
	UpdatedAt     int64  `gorm:"autoUpdateTime:milli" json:"updated_at"`
}

func (Publication) TableName() string { return "system_resource_publications" }

type Release struct {
	Kind         Kind  `gorm:"primaryKey"`
	ResourceID   int64 `gorm:"primaryKey;autoIncrement:false"`
	Revision     int64 `gorm:"primaryKey;autoIncrement:false"`
	Status       string
	Version      string
	MetadataJSON string
	OperatorID   int64
	CreatedAt    int64 `gorm:"autoCreateTime:milli"`
}

func (Release) TableName() string { return "system_resource_releases" }

type Registry struct {
	DB      *gorm.DB
	Ready   bool
	IsAdmin func(context.Context, int64) (bool, error)
	Builtin func(context.Context, Kind, int64) (bool, error)
}

var current atomic.Pointer[Registry]

func SetDefault(registry *Registry) { current.Store(registry) }
func Default() *Registry            { return current.Load() }

func New(db *gorm.DB, admin func(context.Context, int64) (bool, error)) *Registry {
	r := &Registry{DB: db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}), IsAdmin: admin}
	r.Ready = db.Migrator().HasTable(&Publication{}) && db.Migrator().HasTable(&Release{})
	return r
}

func (r *Registry) Lookup(ctx context.Context, kind Kind, id int64) (*Publication, error) {
	if r == nil || !r.Ready {
		return nil, nil
	}
	var item Publication
	err := r.DB.WithContext(ctx).Where("kind = ? AND resource_id = ?", kind, id).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) && kind == Database {
		var online int64
		if e := r.DB.WithContext(ctx).Table("draft_database_info").Select("related_online_id").Where("id = ? AND deleted_at IS NULL", id).Scan(&online).Error; e != nil {
			return nil, e
		}
		if online > 0 {
			err = r.DB.WithContext(ctx).Where("kind = ? AND resource_id = ?", kind, online).First(&item).Error
		}
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &item, err
}

func Published(ctx context.Context, kind Kind, id int64) (bool, error) {
	r := Default()
	if r == nil {
		return false, nil
	}
	p, err := r.Lookup(ctx, kind, id)
	if err != nil {
		return false, err
	}
	if p != nil {
		return p.Status == "published", nil
	}
	if r.Builtin != nil {
		return r.Builtin(ctx, kind, id)
	}
	return false, nil
}

// 已登记资源下架后禁止继续执行，避免绕过目录沿用旧的全局授权。
func CheckActive(ctx context.Context, kind Kind, id int64) error {
	p, err := Default().Lookup(ctx, kind, id)
	if err != nil {
		return err
	}
	if p != nil && p.Status != "published" {
		return fmt.Errorf("系统资源已下架，请联系管理员：%s", p.Name)
	}
	return nil
}

func ProtectWrite(ctx context.Context, kind Kind, id, actor int64) error {
	r := Default()
	if r == nil {
		return nil
	}
	p, err := r.Lookup(ctx, kind, id)
	if err != nil {
		return err
	}
	builtin := false
	if r.Builtin != nil {
		builtin, err = r.Builtin(ctx, kind, id)
		if err != nil {
			return err
		}
	}
	if builtin {
		return fmt.Errorf("平台内置资源只读，请通过系统资源发布管理维护")
	}
	if p == nil {
		return nil
	}
	admin, err := r.IsAdmin(ctx, actor)
	if err != nil {
		return err
	}
	if !admin || actor != p.SourceOwnerID {
		return fmt.Errorf("系统资源原件只允许发布管理员维护")
	}
	return nil
}

func (r *Registry) Save(ctx context.Context, item *Publication, actor, expected int64) error {
	if !r.Ready {
		return fmt.Errorf("系统资源发布表尚未初始化")
	}
	admin, err := r.IsAdmin(ctx, actor)
	if err != nil {
		return err
	}
	if !admin {
		return fmt.Errorf("只有管理员可以发布系统资源")
	}
	if !ValidKind(item.Kind) || item.ResourceID <= 0 || len(item.MetadataJSON) > 512<<10 {
		return fmt.Errorf("资源类型、标识或元数据大小不正确")
	}
	if item.Status != "published" && item.Status != "withdrawn" {
		return fmt.Errorf("发布状态不正确")
	}
	if !json.Valid([]byte(item.MetadataJSON)) {
		return fmt.Errorf("资源元数据格式不正确")
	}
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 使用目录首行串行化全局容量检查；首次发布由唯一键处理并发冲突。
		var guard []Publication
		if e := tx.Model(&Publication{}).Select("kind,resource_id").Order("kind,resource_id").Limit(1).Clauses(clause.Locking{Strength: "UPDATE"}).Find(&guard).Error; e != nil {
			return e
		}
		// 同一管理员串行发布，资源唯一键和修订号防止并发覆盖。
		var owner int64
		if e := tx.Raw("SELECT id FROM user WHERE id = ? FOR UPDATE", actor).Scan(&owner).Error; e != nil {
			return e
		}
		if owner != actor {
			return fmt.Errorf("管理员账号不存在")
		}
		var previous Publication
		e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("kind = ? AND resource_id = ?", item.Kind, item.ResourceID).First(&previous).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if previous.Revision != expected {
			return fmt.Errorf("资源已被其他操作更新，请刷新后重试")
		}
		var used int64
		if e = tx.Raw("SELECT (SELECT COALESCE(SUM(OCTET_LENGTH(metadata_json)),0) FROM system_resource_publications) + (SELECT COALESCE(SUM(OCTET_LENGTH(metadata_json)),0) FROM system_resource_releases)").Scan(&used).Error; e != nil {
			return e
		}
		if used+int64(len(item.MetadataJSON)*2-len(previous.MetadataJSON)) > 256<<20 {
			return fmt.Errorf("系统资源元数据和发布历史已达到 256 MiB 配额，请联系管理员归档历史后再发布")
		}
		if previous.Revision == 0 {
			var count int64
			if e = tx.Model(&Publication{}).Count(&count).Error; e != nil {
				return e
			}
			if count >= 5000 {
				return fmt.Errorf("系统资源登记最多 5000 项，请联系管理员归档")
			}
		}
		item.Revision = previous.Revision + 1
		item.UpdatedBy = actor
		item.UpdatedAt = time.Now().UnixMilli()
		if previous.Revision == 0 {
			item.PublishedBy = actor
			item.CreatedAt = item.UpdatedAt
			if e = tx.Create(item).Error; e != nil {
				return e
			}
		} else {
			item.PublishedBy = previous.PublishedBy
			item.CreatedAt = previous.CreatedAt
			result := tx.Model(&Publication{}).Where("kind = ? AND resource_id = ? AND revision = ?", item.Kind, item.ResourceID, expected).Select("*").Updates(item)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("发布冲突，请刷新后重试")
			}
		}
		return tx.Create(&Release{Kind: item.Kind, ResourceID: item.ResourceID, Revision: item.Revision, Status: item.Status, Version: item.Version, MetadataJSON: item.MetadataJSON, OperatorID: actor}).Error
	})
}

func (r *Registry) List(ctx context.Context, includeWithdrawn bool) ([]Publication, error) {
	items := []Publication{}
	if r == nil || !r.Ready {
		return items, nil
	}
	q := r.DB.WithContext(ctx).Omit("metadata_json")
	if !includeWithdrawn {
		q = q.Where("status = ?", "published")
	}
	err := q.Order("updated_at DESC, resource_id DESC").Find(&items).Error
	return items, err
}

func IDs(ctx context.Context, kind Kind) ([]int64, error) {
	ids := []int64{}
	r := Default()
	if r == nil || !r.Ready {
		return ids, nil
	}
	err := r.DB.WithContext(ctx).Model(&Publication{}).Where("kind = ? AND status = ?", kind, "published").Order("updated_at DESC,resource_id DESC").Pluck("resource_id", &ids).Error
	return ids, err
}

func KnowledgeParents(ctx context.Context, ids []int64, table string) ([]int64, error) {
	parents := []int64{}
	r := Default()
	if r == nil || !r.Ready || len(ids) == 0 {
		return parents, nil
	}
	if table != "knowledge_document" && table != "knowledge_document_slice" {
		return nil, fmt.Errorf("知识资源类型不正确")
	}
	err := r.DB.WithContext(ctx).Table(table).Distinct("knowledge_id").Where("id IN ? AND deleted_at IS NULL", ids).Pluck("knowledge_id", &parents).Error
	return parents, err
}
