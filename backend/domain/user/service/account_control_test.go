package service

import (
	"context"
	"errors"
	"testing"

	"github.com/coze-dev/coze-studio/backend/domain/user/internal/dal/model"
	"github.com/coze-dev/coze-studio/backend/domain/user/repository"
)

type accountControlStub struct {
	disabled bool
	err      error
}

func (s accountControlStub) IsDisabled(context.Context, int64) (bool, error) {
	return s.disabled, s.err
}

type controlledUserRepository struct {
	repository.UserRepository
	user *model.User
}

func (r controlledUserRepository) GetUsersByEmail(context.Context, string) (*model.User, bool, error) {
	return r.user, true, nil
}
func (r controlledUserRepository) GetUserBySessionKey(context.Context, string) (*model.User, bool, error) {
	return r.user, true, nil
}

func TestDisabledAccountCannotLoginOrReuseSession(t *testing.T) {
	ctx := context.Background()
	key, err := generateSessionKey(123456)
	if err != nil {
		t.Fatal(err)
	}
	for _, control := range []accountControlStub{{disabled: true}, {err: errors.New("状态存储不可用")}} {
		domain := NewUserDomain(ctx, &Components{UserRepo: controlledUserRepository{user: &model.User{ID: 7, Email: "test@example.com", SessionKey: key}}, AccountControls: control})
		if _, err = domain.Login(ctx, "test@example.com", "irrelevant"); err == nil {
			t.Fatal("停用或状态检查失败时不允许登录")
		}
		if _, valid, err := domain.ValidateSession(ctx, key); valid || (control.err != nil && err == nil) {
			t.Fatal("停用账号的原会话不得继续使用")
		}
	}
}
