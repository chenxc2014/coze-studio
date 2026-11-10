package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	adminconfig "github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	"github.com/coze-dev/coze-studio/backend/pkg/kvstore"
	"github.com/coze-dev/coze-studio/backend/types/consts"
)

const AccountControlNamespace = "user_account_controls"

type AccountControl struct {
	Disabled  bool  `json:"disabled"`
	UpdatedAt int64 `json:"updated_at"`
	UpdatedBy int64 `json:"updated_by,string"`
}

type AccountControlRepository interface {
	IsDisabled(context.Context, int64) (bool, error)
}

type accountControls struct {
	store *kvstore.KVStore[AccountControl]
}

func NewAccountControlRepo(db *gorm.DB) AccountControlRepository {
	return &accountControls{store: kvstore.New[AccountControl](db)}
}

func (r *accountControls) IsDisabled(ctx context.Context, id int64) (bool, error) {
	control, err := r.store.Get(ctx, AccountControlNamespace, strconv.FormatInt(id, 10))
	if errors.Is(err, kvstore.ErrKeyNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return control.Disabled, nil
}

type ManagedAccount struct {
	ID         int64  `json:"id,string"`
	Name       string `json:"name"`
	UniqueName string `json:"unique_name"`
	Email      string `json:"email"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
	Disabled   bool   `json:"disabled"`
	IsAdmin    bool   `json:"is_admin" gorm:"-"`
}

type AccountListOptions struct {
	Query    string
	Status   string
	Role     string
	Page     int
	PageSize int
}

type AccountAudit struct {
	ActorID   int64  `json:"actor_id,string"`
	TargetID  int64  `json:"target_id,string"`
	Action    string `json:"action"`
	CreatedAt int64  `json:"created_at"`
}

type AccountAdminStore struct{ db *gorm.DB }

func NewAccountAdminStore(db *gorm.DB) *AccountAdminStore       { return &AccountAdminStore{db: db} }
func (s *AccountAdminStore) Users() UserRepository              { return NewUserRepo(s.db) }
func (s *AccountAdminStore) Spaces() SpaceRepository            { return NewSpaceRepo(s.db) }
func (s *AccountAdminStore) Controls() AccountControlRepository { return NewAccountControlRepo(s.db) }
func (s *AccountAdminStore) Transaction(ctx context.Context, fn func(*AccountAdminStore) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(NewAccountAdminStore(tx)) })
}

// 所有账号与管理员配置变更锁定同一行，串行检查最后一位管理员。
func (s *AccountAdminStore) LockBasic(ctx context.Context) (*adminconfig.BasicConfiguration, error) {
	var value []byte
	err := s.db.WithContext(ctx).Raw("SELECT value_data FROM kv_entries WHERE namespace = ? AND key_data = ? FOR UPDATE", consts.BaseConfigNameSpace, "basic_config").Row().Scan(&value)
	if err != nil {
		return nil, err
	}
	var config adminconfig.BasicConfiguration
	if err = json.Unmarshal(value, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func (s *AccountAdminStore) SaveBasic(ctx context.Context, config *adminconfig.BasicConfiguration) error {
	return kvstore.New[adminconfig.BasicConfiguration](s.db).Save(ctx, consts.BaseConfigNameSpace, "basic_config", config)
}

const disabledSQL = "COALESCE(JSON_UNQUOTE(JSON_EXTRACT(CAST(account_control.value_data AS CHAR CHARACTER SET utf8mb4), '$.disabled')), 'false') = 'true'"

func (s *AccountAdminStore) query(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Table("user AS account").
		Joins("LEFT JOIN kv_entries AS account_control ON account_control.namespace = ? AND account_control.key_data = (CAST(account.id AS CHAR CHARACTER SET utf8mb4) COLLATE utf8mb4_general_ci)", AccountControlNamespace).
		Where("account.deleted_at IS NULL")
}

func (s *AccountAdminStore) List(ctx context.Context, options AccountListOptions, admins []string) ([]ManagedAccount, int64, error) {
	query := s.query(ctx)
	if options.Query != "" {
		pattern := "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(options.Query) + "%"
		query = query.Where("(account.email LIKE ? OR account.name LIKE ? OR account.unique_name LIKE ?)", pattern, pattern, pattern)
	}
	if options.Status == "disabled" {
		query = query.Where(disabledSQL)
	}
	if options.Status == "enabled" {
		query = query.Where("NOT (" + disabledSQL + ")")
	}
	if options.Role == "admin" {
		query = query.Where("LOWER(account.email) IN ?", admins)
	}
	if options.Role == "user" && len(admins) > 0 {
		query = query.Where("LOWER(account.email) NOT IN ?", admins)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	accounts := []ManagedAccount{}
	err := query.Select("account.id, account.name, account.unique_name, account.email, account.created_at, account.updated_at, (" + disabledSQL + ") AS disabled").
		Order("account.created_at DESC, account.id DESC").Offset((options.Page - 1) * options.PageSize).Limit(options.PageSize).Scan(&accounts).Error
	return accounts, count, err
}

func (s *AccountAdminStore) Find(ctx context.Context, id int64) (*ManagedAccount, error) {
	var account ManagedAccount
	result := s.query(ctx).Where("account.id = ?", id).Select("account.id, account.name, account.unique_name, account.email, account.created_at, account.updated_at, (" + disabledSQL + ") AS disabled").Take(&account)
	return &account, result.Error
}

func (s *AccountAdminStore) ActiveAdminCount(ctx context.Context, admins []string) (int64, error) {
	var count int64
	if len(admins) == 0 {
		return 0, nil
	}
	err := s.query(ctx).Where("LOWER(account.email) IN ?", admins).Where("NOT (" + disabledSQL + ")").Count(&count).Error
	return count, err
}

func (s *AccountAdminStore) SetDisabled(ctx context.Context, id, actor int64, disabled bool) error {
	control := AccountControl{Disabled: disabled, UpdatedAt: time.Now().UnixMilli(), UpdatedBy: actor}
	if err := kvstore.New[AccountControl](s.db).Save(ctx, AccountControlNamespace, strconv.FormatInt(id, 10), &control); err != nil {
		return err
	}
	// 启停状态变更都撤销旧会话；重新启用后必须重新登录。
	return s.Users().UpdateProfile(ctx, id, map[string]any{"session_key": ""})
}

func (s *AccountAdminStore) Audit(ctx context.Context, actor, target int64, action string) error {
	entry := AccountAudit{ActorID: actor, TargetID: target, Action: action, CreatedAt: time.Now().UnixMilli()}
	return kvstore.New[AccountAudit](s.db).Save(ctx, "user_account_audit", uuid.NewString(), &entry)
}
