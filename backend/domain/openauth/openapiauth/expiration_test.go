package openapiauth

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/coze-dev/coze-studio/backend/domain/openauth/openapiauth/entity"
)

func TestTokenExpirationPermission(t *testing.T) {
	for _, tc := range []struct {
		name             string
		expires          int64
		missing, allowed bool
	}{
		{"永久", entity.PermanentExpiration, false, true},
		{"未到期", time.Now().Add(time.Hour).Unix(), false, true},
		{"已到期", time.Now().Add(-time.Hour).Unix(), false, false},
		{"零时间", 0, false, false},
		{"非法负值", -2, false, false},
		{"已撤销", entity.PermanentExpiration, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			gormDB, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			rows := sqlmock.NewRows([]string{"id", "api_key", "user_id", "expired_at"})
			if !tc.missing {
				rows.AddRow(7, "test-hash", 8, tc.expires)
			}
			mock.ExpectQuery("SELECT .* FROM `api_key`").WillReturnRows(rows)
			svc := NewService(&Components{DB: gormDB})
			got, err := svc.CheckPermission(context.Background(), &entity.CheckPermission{ApiKey: "test-hash"})
			if (got != nil && err == nil) != tc.allowed {
				t.Fatal("令牌鉴权结果不符合有效期或撤销状态")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
