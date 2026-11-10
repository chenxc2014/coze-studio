package user

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	adminconfig "github.com/coze-dev/coze-studio/backend/api/model/admin/config"
)

func TestProtectCurrentAdministrator(t *testing.T) {
	for _, action := range []string{"disable", "demote", "configuration"} {
		t.Run(action, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			db, err := gorm.Open(mysql.New(mysql.Config{Conn: database, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
			if err != nil {
				t.Fatal(err)
			}
			cfg, _ := json.Marshal(&adminconfig.BasicConfiguration{AdminEmails: "admin@example.com"})
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT value_data FROM kv_entries.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"value_data"}).AddRow(cfg))
			account := func() {
				mock.ExpectQuery("SELECT .* FROM user AS account.*account.id = ").WillReturnRows(sqlmock.NewRows([]string{"id", "email", "disabled"}).AddRow(7, "admin@example.com", false))
			}
			account()
			account()
			mock.ExpectRollback()
			a := NewAccountAdministration(db, nil, nil)
			ctx := context.Background()
			switch action {
			case "disable":
				err = a.SetStatus(ctx, 7, 7, true)
			case "demote":
				err = a.SetRole(ctx, 7, 7, false)
			case "configuration":
				err = a.SaveBasicConfig(ctx, 7, &adminconfig.BasicConfiguration{AdminEmails: "other@example.com"})
			}
			var business *AccountAdminError
			if !errors.As(err, &business) || business.Status != 409 {
				t.Fatalf("应拒绝移除当前管理员，实际：%v", err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAccountPasswordValidation(t *testing.T) {
	for _, password := range []string{"", "short", "            "} {
		if validateAccountPassword(password) == nil {
			t.Fatal("应拒绝过短或空白密码")
		}
	}
	if err := validateAccountPassword("Account-test-2026!"); err != nil {
		t.Fatal(err)
	}
}
