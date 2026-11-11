package dao

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/coze-dev/coze-studio/backend/domain/knowledge/internal/dal/query"
)

func TestDocumentStatusClearsOldFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	orm, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	dao := &KnowledgeDocumentDAO{DB: orm, Query: query.Use(orm)}
	for _, status := range []int32{4, 1, 0} {
		mock.ExpectExec("UPDATE `knowledge_document` SET `fail_reason`=\\?,`status`=\\?,`updated_at`=\\? WHERE").WithArgs("", status, sqlmock.AnyArg(), int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
		if err := dao.SetStatus(context.Background(), 7, status, ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
