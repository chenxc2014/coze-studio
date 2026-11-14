package service

import (
	"fmt"
	"strings"

	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	"github.com/pingcap/tidb/pkg/parser/mysql"
	_ "github.com/pingcap/tidb/pkg/parser/test_driver"
)

// 对共享数据库逐个检查语法树中的表，避免子查询、联合查询绕过单表授权。
func validateSystemSelect(query, tableName string) error {
	stmt, err := parser.New().ParseOneStmt(query, mysql.UTF8MB4Charset, mysql.UTF8MB4GeneralCICollation)
	if err != nil {
		return fmt.Errorf("系统数据库查询语法不正确")
	}
	if _, ok := stmt.(*ast.SelectStmt); !ok {
		if _, ok = stmt.(*ast.SetOprStmt); !ok {
			return fmt.Errorf("系统数据库仅支持 SELECT 查询")
		}
	}
	visitor := &systemSelectVisitor{tableName: strings.ToLower(tableName)}
	stmt.Accept(visitor)
	return visitor.err
}

type systemSelectVisitor struct {
	tableName string
	err       error
}

func (v *systemSelectVisitor) Enter(node ast.Node) (ast.Node, bool) {
	if v.err != nil {
		return node, true
	}
	switch item := node.(type) {
	case *ast.TableName:
		if item.Schema.O != "" || item.Name.L != v.tableName {
			v.err = fmt.Errorf("系统数据库只能查询当前已授权的逻辑表")
		}
	case *ast.SelectStmt:
		if item.SelectIntoOpt != nil || item.LockInfo != nil {
			v.err = fmt.Errorf("系统数据库不支持导出文件或加锁查询")
		}
	case *ast.VariableExpr:
		v.err = fmt.Errorf("系统数据库不支持会话变量")
	case *ast.FuncCallExpr:
		// 仅开放数据查询常用函数，不执行文件、锁、延时或服务器信息函数。
		allowed := map[string]bool{"abs": true, "ceil": true, "ceiling": true, "floor": true, "round": true, "truncate": true, "lower": true, "upper": true, "length": true, "char_length": true, "concat": true, "concat_ws": true, "substring": true, "substr": true, "left": true, "right": true, "trim": true, "ltrim": true, "rtrim": true, "replace": true, "coalesce": true, "ifnull": true, "nullif": true, "if": true, "date": true, "date_format": true, "year": true, "month": true, "day": true, "dayofmonth": true, "datediff": true, "now": true, "curdate": true, "current_date": true, "current_timestamp": true, "json_extract": true, "json_unquote": true, "json_contains": true, "json_length": true, "greatest": true, "least": true}
		if !allowed[item.FnName.L] {
			v.err = fmt.Errorf("系统数据库不支持该函数：%s", item.FnName.O)
		}
	}
	return node, false
}
func (v *systemSelectVisitor) Leave(node ast.Node) (ast.Node, bool) { return node, v.err == nil }
