package user

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"

	adminconfig "github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	bizconfig "github.com/coze-dev/coze-studio/backend/bizpkg/config"
	"github.com/coze-dev/coze-studio/backend/domain/user/repository"
	"github.com/coze-dev/coze-studio/backend/domain/user/service"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

type AccountAdminError struct {
	Status  int
	Message string
}

func (e *AccountAdminError) Error() string          { return e.Message }
func accountError(status int, message string) error { return &AccountAdminError{status, message} }

type AccountAdministration struct {
	store *repository.AccountAdminStore
	oss   storage.Storage
	idgen idgen.IDGenerator
}

func NewAccountAdministration(db *gorm.DB, oss storage.Storage, generator idgen.IDGenerator) *AccountAdministration {
	return &AccountAdministration{repository.NewAccountAdminStore(db), oss, generator}
}

func adminEmails(raw string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		email := strings.ToLower(strings.TrimSpace(item))
		if email != "" && !seen[email] {
			result = append(result, email)
			seen[email] = true
		}
	}
	return result
}

func isAdministrator(email string, emails []string) bool {
	for _, admin := range emails {
		if strings.EqualFold(email, admin) {
			return true
		}
	}
	return false
}

func validateAccountPassword(password string) error {
	if n := utf8.RuneCountInString(password); n < 12 || n > 128 {
		return accountError(400, "密码需要 12 至 128 个字符")
	}
	if strings.TrimSpace(password) == "" {
		return accountError(400, "密码不能全部为空白字符")
	}
	return nil
}

func validateAccountName(name string) error {
	if n := utf8.RuneCountInString(name); n < 1 || n > 64 {
		return accountError(400, "姓名需要 1 至 64 个字符")
	}
	return nil
}

func (a *AccountAdministration) authorize(ctx context.Context, store *repository.AccountAdminStore, actor int64, cfg *adminconfig.BasicConfiguration) error {
	account, err := store.Find(ctx, actor)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return accountError(403, "当前账号没有管理权限")
		}
		return err
	}
	if account.Disabled || !isAdministrator(account.Email, adminEmails(cfg.AdminEmails)) {
		return accountError(403, "当前账号没有管理权限")
	}
	return nil
}

func (a *AccountAdministration) locked(ctx context.Context, actor int64, fn func(*repository.AccountAdminStore, *adminconfig.BasicConfiguration) error) error {
	return a.store.Transaction(ctx, func(store *repository.AccountAdminStore) error {
		cfg, err := store.LockBasic(ctx)
		if err != nil {
			return err
		}
		if err = a.authorize(ctx, store, actor, cfg); err != nil {
			return err
		}
		if err = fn(store, cfg); err != nil {
			return err
		}
		count, err := store.ActiveAdminCount(ctx, adminEmails(cfg.AdminEmails))
		if err != nil {
			return err
		}
		if count == 0 {
			return accountError(409, "必须保留至少一位已启用的管理员")
		}
		return nil
	})
}

func (a *AccountAdministration) domain(ctx context.Context, store *repository.AccountAdminStore) service.User {
	return service.NewUserDomain(ctx, &service.Components{IconOSS: a.oss, IDGen: a.idgen, UserRepo: store.Users(), SpaceRepo: store.Spaces(), AccountControls: store.Controls()})
}

func (a *AccountAdministration) List(ctx context.Context, actor int64, options repository.AccountListOptions) ([]repository.ManagedAccount, int64, error) {
	if options.Page < 1 || options.Page > 1000000 || options.PageSize < 1 || options.PageSize > 100 || utf8.RuneCountInString(options.Query) > 100 {
		return nil, 0, accountError(400, "分页或搜索参数不正确")
	}
	if options.Status != "" && options.Status != "enabled" && options.Status != "disabled" {
		return nil, 0, accountError(400, "账号状态不正确")
	}
	if options.Role != "" && options.Role != "admin" && options.Role != "user" {
		return nil, 0, accountError(400, "角色不正确")
	}
	cfg, err := bizconfig.Base().GetBaseConfig(ctx)
	if err != nil {
		return nil, 0, err
	}
	if err = a.authorize(ctx, a.store, actor, cfg); err != nil {
		return nil, 0, err
	}
	admins := adminEmails(cfg.AdminEmails)
	accounts, total, err := a.store.List(ctx, options, admins)
	for i := range accounts {
		accounts[i].IsAdmin = isAdministrator(accounts[i].Email, admins)
	}
	return accounts, total, err
}

type CreateManagedAccount struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
	IsAdmin  bool   `json:"is_admin"`
}

func (a *AccountAdministration) Create(ctx context.Context, actor int64, input CreateManagedAccount) (*repository.ManagedAccount, error) {
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.Name = strings.TrimSpace(input.Name)
	parsed, err := mail.ParseAddress(input.Email)
	if err != nil || parsed.Address != input.Email || len(input.Email) > 128 {
		return nil, accountError(400, "请输入有效的邮箱地址，长度不能超过 128 字节")
	}
	if err = validateAccountName(input.Name); err != nil {
		return nil, err
	}
	if err = validateAccountPassword(input.Password); err != nil {
		return nil, err
	}
	var result *repository.ManagedAccount
	err = a.locked(ctx, actor, func(store *repository.AccountAdminStore, cfg *adminconfig.BasicConfiguration) error {
		exists, err := store.Users().CheckEmailExist(ctx, input.Email)
		if err != nil {
			return err
		}
		if exists {
			return accountError(409, "该邮箱已经存在")
		}
		created, err := a.domain(ctx, store).Create(ctx, &service.CreateUserRequest{Email: input.Email, Name: input.Name, Password: input.Password, Locale: "zh-CN"})
		if err != nil {
			return err
		}
		if input.IsAdmin {
			cfg.AdminEmails = strings.Join(append(adminEmails(cfg.AdminEmails), input.Email), ",")
			if err = store.SaveBasic(ctx, cfg); err != nil {
				return err
			}
		} else if isAdministrator(input.Email, adminEmails(cfg.AdminEmails)) {
			// 邮箱预先出现在旧管理员配置中时，以创建表单的角色为准。
			cfg.AdminEmails = withoutAdmin(cfg.AdminEmails, input.Email)
			if err = store.SaveBasic(ctx, cfg); err != nil {
				return err
			}
		}
		result, err = store.Find(ctx, created.UserID)
		if err != nil {
			return err
		}
		result.IsAdmin = input.IsAdmin
		return store.Audit(ctx, actor, created.UserID, "create")
	})
	return result, err
}

func withoutAdmin(raw, email string) string {
	result := []string{}
	for _, item := range adminEmails(raw) {
		if !strings.EqualFold(item, email) {
			result = append(result, item)
		}
	}
	return strings.Join(result, ",")
}

func managedTarget(ctx context.Context, store *repository.AccountAdminStore, id int64) (*repository.ManagedAccount, error) {
	if id <= 0 {
		return nil, accountError(400, "账号 ID 不正确")
	}
	account, err := store.Find(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, accountError(404, "账号不存在")
	}
	return account, err
}

func (a *AccountAdministration) Update(ctx context.Context, actor, id int64, name string) error {
	name = strings.TrimSpace(name)
	if err := validateAccountName(name); err != nil {
		return err
	}
	return a.locked(ctx, actor, func(store *repository.AccountAdminStore, cfg *adminconfig.BasicConfiguration) error {
		if _, err := managedTarget(ctx, store, id); err != nil {
			return err
		}
		if err := store.Users().UpdateProfile(ctx, id, map[string]any{"name": name}); err != nil {
			return err
		}
		return store.Audit(ctx, actor, id, "update_profile")
	})
}

func (a *AccountAdministration) SetStatus(ctx context.Context, actor, id int64, disabled bool) error {
	return a.locked(ctx, actor, func(store *repository.AccountAdminStore, cfg *adminconfig.BasicConfiguration) error {
		if _, err := managedTarget(ctx, store, id); err != nil {
			return err
		}
		if actor == id && disabled {
			return accountError(409, "不能停用当前登录账号")
		}
		if err := store.SetDisabled(ctx, id, actor, disabled); err != nil {
			return err
		}
		action := "enable"
		if disabled {
			action = "disable"
		}
		return store.Audit(ctx, actor, id, action)
	})
}

func (a *AccountAdministration) ResetPassword(ctx context.Context, actor, id int64, password string) error {
	if err := validateAccountPassword(password); err != nil {
		return err
	}
	return a.locked(ctx, actor, func(store *repository.AccountAdminStore, cfg *adminconfig.BasicConfiguration) error {
		account, err := managedTarget(ctx, store, id)
		if err != nil {
			return err
		}
		if err = a.domain(ctx, store).ResetPassword(ctx, account.Email, password); err != nil {
			return err
		}
		return store.Audit(ctx, actor, id, "reset_password")
	})
}

func (a *AccountAdministration) SetRole(ctx context.Context, actor, id int64, isAdmin bool) error {
	return a.locked(ctx, actor, func(store *repository.AccountAdminStore, cfg *adminconfig.BasicConfiguration) error {
		account, err := managedTarget(ctx, store, id)
		if err != nil {
			return err
		}
		if actor == id && !isAdmin {
			return accountError(409, "不能取消自己的管理员身份")
		}
		if isAdmin && account.Disabled {
			return accountError(409, "请先启用该账号，再授予管理员身份")
		}
		cfg.AdminEmails = withoutAdmin(cfg.AdminEmails, account.Email)
		if isAdmin {
			cfg.AdminEmails = strings.Join(append(adminEmails(cfg.AdminEmails), strings.ToLower(account.Email)), ",")
		}
		if err = store.SaveBasic(ctx, cfg); err != nil {
			return err
		}
		action := "revoke_admin"
		if isAdmin {
			action = "grant_admin"
		}
		return store.Audit(ctx, actor, id, action)
	})
}

// 基础配置页与账号页共用同一事务锁，避免绕过管理员保护规则。
func (a *AccountAdministration) SaveBasicConfig(ctx context.Context, actor int64, next *adminconfig.BasicConfiguration) error {
	return a.locked(ctx, actor, func(store *repository.AccountAdminStore, cfg *adminconfig.BasicConfiguration) error {
		account, err := store.Find(ctx, actor)
		if err != nil {
			return err
		}
		emails := adminEmails(next.AdminEmails)
		for _, email := range emails {
			parsed, e := mail.ParseAddress(email)
			if e != nil || parsed.Address != email {
				return accountError(400, "管理员邮箱格式不正确")
			}
		}
		if !isAdministrator(account.Email, emails) {
			return accountError(409, "不能从管理员配置中移除当前登录账号")
		}
		next.AdminEmails = strings.Join(emails, ",")
		*cfg = *next
		if err = store.SaveBasic(ctx, cfg); err != nil {
			return err
		}
		return store.Audit(ctx, actor, actor, "save_basic_configuration")
	})
}

func (a *AccountAdministration) IsDisabled(ctx context.Context, id int64) (bool, error) {
	account, err := a.store.Find(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, nil
	}
	if err != nil {
		return true, err
	}
	return account.Disabled, nil
}
