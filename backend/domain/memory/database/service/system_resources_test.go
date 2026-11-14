package service

import "testing"

func TestSystemDatabaseSelectIsolation(t *testing.T) {
	for _, query := range []string{
		"SELECT * FROM hospitals LIMIT 20",
		"SELECT name FROM hospitals WHERE address LIKE '%街道%'",
		"SELECT COUNT(*) FROM hospitals",
		"SELECT lower(h.name) FROM hospitals h WHERE h.id IN (SELECT id FROM hospitals)",
		"SELECT name FROM hospitals UNION SELECT name FROM hospitals",
	} {
		if err := validateSystemSelect(query, "hospitals"); err != nil {
			t.Errorf("有效只读查询被拒绝：%s：%v", query, err)
		}
	}
	for _, query := range []string{
		"UPDATE hospitals SET name='changed'",
		"DELETE FROM hospitals",
		"SELECT * FROM user",
		"SELECT * FROM private_db.hospitals",
		"SELECT name FROM hospitals UNION SELECT password FROM user",
		"SELECT * FROM hospitals WHERE id IN (SELECT id FROM user)",
		"SELECT * FROM hospitals; DELETE FROM hospitals",
		"SELECT * FROM hospitals INTO OUTFILE '/tmp/shared-leak'",
		"SELECT * FROM hospitals FOR UPDATE",
		"SELECT load_file('/etc/passwd') FROM hospitals",
		"SELECT sleep(60) FROM hospitals",
		"SELECT @private_variable FROM hospitals",
	} {
		if err := validateSystemSelect(query, "hospitals"); err == nil {
			t.Errorf("越权或写入查询未被拒绝：%s", query)
		}
	}
}
