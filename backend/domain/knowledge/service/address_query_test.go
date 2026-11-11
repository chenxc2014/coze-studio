package service

import (
	"context"
	knowledgeModel "github.com/coze-dev/coze-studio/backend/crossdomain/knowledge/model"
	"github.com/coze-dev/coze-studio/backend/domain/knowledge/address"
	"github.com/coze-dev/coze-studio/backend/domain/knowledge/entity"
	"github.com/coze-dev/coze-studio/backend/domain/knowledge/internal/consts"
	"github.com/coze-dev/coze-studio/backend/domain/knowledge/internal/dal/model"
	"github.com/coze-dev/coze-studio/backend/domain/knowledge/repository"
	"github.com/coze-dev/coze-studio/backend/infra/document"
	"github.com/coze-dev/coze-studio/backend/infra/rdb"
	rdbentity "github.com/coze-dev/coze-studio/backend/infra/rdb/entity"
	"strings"
	"testing"
)

type addressDocuments struct {
	repository.KnowledgeDocumentRepo
	doc *model.KnowledgeDocument
}

func (f *addressDocuments) GetByID(context.Context, int64) (*model.KnowledgeDocument, error) {
	return f.doc, nil
}

type addressKnowledges struct {
	repository.KnowledgeRepo
	kn *model.Knowledge
}

func (f *addressKnowledges) GetByID(context.Context, int64) (*model.Knowledge, error) {
	return f.kn, nil
}

type addressDatabase struct {
	rdb.RDB
	queries []*rdb.ExecuteSQLRequest
	total   int64
	paged   bool
}

func (f *addressDatabase) ExecuteSQL(_ context.Context, req *rdb.ExecuteSQLRequest) (*rdb.ExecuteSQLResponse, error) {
	f.queries = append(f.queries, req)
	rows := []map[string]any{}
	switch {
	case strings.HasPrefix(req.SQL, "SELECT DISTINCT"):
		rows = append(rows, map[string]any{"province": []byte("浙江省"), "city": []byte("杭州市"), "district": []byte("西湖区")})
	case strings.HasPrefix(req.SQL, "SELECT COUNT"):
		rows = append(rows, map[string]any{"matched_count": f.total})
	default:
		if f.paged {
			limit := req.Params[len(req.Params)-2].(int)
			offset := req.Params[len(req.Params)-1].(int)
			for i := offset; i < offset+limit && int64(i) < f.total; i++ {
				rows = append(rows, map[string]any{consts.RDBFieldID: int64(7706726398408261633) + int64(i), "c_1": []byte("浙江省"), "c_2": []byte("杭州市"), "c_3": []byte("西湖区"), "c_4": []byte("文一路23号")})
			}
		} else if f.total > 0 {
			rows = append(rows, map[string]any{consts.RDBFieldID: int64(7706726398408261633), "c_1": []byte("浙江省"), "c_2": []byte("杭州市"), "c_3": []byte("西湖区"), "c_4": []byte("文一路23号")})
		}
	}
	return &rdb.ExecuteSQLResponse{ResultSet: &rdbentity.ResultSet{Rows: rows}}, nil
}
func addressFixture() (*knowledgeSVC, *addressDatabase, *model.KnowledgeDocument) {
	doc := &model.KnowledgeDocument{ID: 11, KnowledgeID: 22, DocumentType: 1, Status: 1, Name: "测试机构", TableInfo: &entity.TableInfo{PhysicalTableName: "knowledge_test_11", Columns: []*entity.TableColumn{{ID: 1, Name: "省", Type: document.TableColumnTypeString}, {ID: 2, Name: "市", Type: document.TableColumnTypeString}, {ID: 3, Name: "区县", Type: document.TableColumnTypeString}, {ID: 4, Name: "地址", Type: document.TableColumnTypeString}}}}
	db := &addressDatabase{total: 45}
	return &knowledgeSVC{documentRepo: &addressDocuments{doc: doc}, knowledgeRepo: &addressKnowledges{kn: &model.Knowledge{ID: 22, Status: 1}}, rdb: db}, db, doc
}

func TestAddressQueryLiteralValuesAndPagination(t *testing.T) {
	svc, db, _ := addressFixture()
	input := "文一路%' OR 1=1--"
	resp, err := svc.QueryAddress(context.Background(), &AddressQueryRequest{DocumentID: 11, Filter: address.Filter{City: "杭州", Address: input}, Page: 2, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Total != 45 || resp.Page != 2 || !resp.HasMore || len(resp.Rows) != 1 || resp.Rows[0].SliceID != "7706726398408261633" || resp.Rows[0].Values["市"] != "杭州市" {
		t.Fatalf("分页或 ID 精度不正确：%+v", resp)
	}
	last := db.queries[len(db.queries)-1]
	if strings.Contains(last.SQL, "OR 1=1") || strings.Contains(last.SQL, "LIKE") || !strings.Contains(last.SQL, "ORDER BY `"+consts.RDBFieldID+"` ASC LIMIT ? OFFSET ?") {
		t.Fatalf("查询未使用受控表达式：%s", last.SQL)
	}
	if last.Params[0] != "杭州市" || last.Params[1] != address.Normalize(input) || last.Params[2] != 20 || last.Params[3] != 20 {
		t.Fatalf("查询参数未完整保留：%v", last.Params)
	}
}

func TestAddressQueryNoMatchAndCount(t *testing.T) {
	svc, db, _ := addressFixture()
	db.total = 0
	resp, err := svc.QueryAddress(context.Background(), &AddressQueryRequest{DocumentID: 11, Query: "北京市朝阳区有哪些医疗机构"})
	if err != nil || resp.Total != 0 || len(resp.Rows) != 0 || resp.HasMore || resp.Filter.City != "北京市" {
		t.Fatalf("无匹配时不应转查其它地址：%+v %v", resp, err)
	}
	db.total = 45
	db.queries = nil
	resp, err = svc.QueryAddress(context.Background(), &AddressQueryRequest{DocumentID: 11, Query: "杭州市西湖区有多少家医疗机构"})
	if err != nil || !resp.CountOnly || resp.Total != 45 || len(resp.Rows) != 0 || len(db.queries) != 2 {
		t.Fatalf("计数不应读取或虚构列表：%+v %v", resp, err)
	}
}

func TestAddressQueryValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*model.KnowledgeDocument, *AddressQueryRequest)
	}{
		{"空条件", func(_ *model.KnowledgeDocument, r *AddressQueryRequest) { r.Filter = address.Filter{} }},
		{"非法分页", func(_ *model.KnowledgeDocument, r *AddressQueryRequest) { r.PageSize = 101 }},
		{"非法表名", func(d *model.KnowledgeDocument, _ *AddressQueryRequest) {
			d.TableInfo.PhysicalTableName = "x`; DROP TABLE user;--"
		}},
		{"关闭文档", func(d *model.KnowledgeDocument, _ *AddressQueryRequest) { d.Status = 2 }},
		{"未显式允许部分数据", func(d *model.KnowledgeDocument, _ *AddressQueryRequest) { d.Status = 4 }},
		{"字段与问题冲突", func(_ *model.KnowledgeDocument, r *AddressQueryRequest) {
			r.Query = "杭州市西湖区有哪些医疗机构"
			r.Filter.City = "宁波市"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, doc := addressFixture()
			req := &AddressQueryRequest{DocumentID: 11, Filter: address.Filter{City: "杭州市"}}
			tc.change(doc, req)
			if _, err := svc.QueryAddress(context.Background(), req); err == nil {
				t.Fatal("未拒绝不安全或含糊的查询")
			}
		})
	}
	svc, _, doc := addressFixture()
	doc.Status = 4
	resp, err := svc.QueryAddress(context.Background(), &AddressQueryRequest{DocumentID: 11, Filter: address.Filter{District: "西湖区"}, AllowPartial: true})
	if err != nil || resp.Complete {
		t.Fatalf("部分数据预览必须显式标识：%+v %v", resp, err)
	}
}

func TestAddressAutomaticDocumentScope(t *testing.T) {
	svc, _, doc := addressFixture()
	ctx := &RetrieveContext{OriginQuery: "杭州市西湖区有哪些医疗机构", Documents: []*model.KnowledgeDocument{doc}}
	t.Setenv("COZE_ADDRESS_DOCUMENT_IDS", "")
	if _, handled, _ := svc.retrieveAddress(context.Background(), ctx); handled {
		t.Fatal("未启用时不应接管普通检索")
	}
	t.Setenv("COZE_ADDRESS_DOCUMENT_IDS", "11")
	resp, handled, err := svc.retrieveAddress(context.Background(), ctx)
	if err != nil || !handled || len(resp.RetrieveSlices) != 1 || resp.RetrieveSlices[0].Slice.KnowledgeID != 22 || resp.RetrieveSlices[0].Slice.RawContent[0].Type != knowledgeModel.SliceContentTypeText {
		t.Fatalf("指定文档地址召回失败：%+v %v", resp, err)
	}
	ctx.OriginQuery = "杭州市西湖区有哪些三甲医院"
	if _, handled, _ := svc.retrieveAddress(context.Background(), ctx); handled {
		t.Fatal("不得悄悄丢弃医院等级条件")
	}
}

func TestAddressAgentAutomaticallyUsesBoundKnowledge(t *testing.T) {
	svc, db, doc := addressFixture()
	t.Setenv("COZE_ADDRESS_DOCUMENT_IDS", "")
	ctx := &RetrieveContext{OriginQuery: "杭州市西湖区有哪些医疗机构", Documents: []*model.KnowledgeDocument{doc}, AutoAddress: true}
	resp, handled, err := svc.retrieveAddress(context.Background(), ctx)
	if err != nil || !handled || len(resp.RetrieveSlices) != 1 {
		t.Fatalf("绑定知识应自动支持地址查询，不依赖人工文档白名单：%+v %v", resp, err)
	}
	ctx.OriginQuery = "那余杭区呢"
	calls := 0
	ctx.ResolveAddress = func(context.Context) (*knowledgeModel.AddressRecall, error) {
		calls++
		return &knowledgeModel.AddressRecall{Handled: true, City: "杭州市", District: "余杭区", CountOnly: true}, nil
	}
	_, handled, err = svc.retrieveAddress(context.Background(), ctx)
	if err != nil || !handled || calls != 1 {
		t.Fatalf("追问没有自动沿用地址：%v %v", handled, err)
	}
	last := db.queries[len(db.queries)-1]
	if last.Params[len(last.Params)-1] != "余杭区" {
		t.Fatalf("追问没有使用新的地区条件：%v", last.Params)
	}
}

func TestAddressAgentReadsBatchesWithoutUserPaging(t *testing.T) {
	svc, db, _ := addressFixture()
	db.paged = true
	db.total = 145
	response, records, complete, err := svc.readAddressForAgent(context.Background(), &AddressQueryRequest{DocumentID: 11, Filter: address.Filter{City: "杭州市"}})
	if err != nil || !complete || response.Total != 145 || len(records) != 145 {
		t.Fatalf("服务应自动读完所需批次：%v %v %d", err, complete, len(records))
	}
	reads := 0
	for _, q := range db.queries {
		if strings.HasPrefix(q.SQL, "SELECT *") {
			reads++
		}
	}
	if reads != 2 {
		t.Fatalf("145条应在服务内部完成两批读取：%d", reads)
	}
	db.total = 500
	response, records, complete, err = svc.readAddressForAgent(context.Background(), &AddressQueryRequest{DocumentID: 11, Filter: address.Filter{City: "杭州市"}})
	if err != nil || complete || response.Total != 500 || len(records) != 200 {
		t.Fatalf("预算限制下必须保留准确总数并标注摘要：%v %v %d", err, complete, len(records))
	}
}

func TestAddressAmbiguousQueryAsksInsteadOfSearching(t *testing.T) {
	svc, db, doc := addressFixture()
	ctx := &RetrieveContext{OriginQuery: "那边还有哪些", Documents: []*model.KnowledgeDocument{doc}, AutoAddress: true, ResolveAddress: func(context.Context) (*knowledgeModel.AddressRecall, error) {
		return &knowledgeModel.AddressRecall{Handled: true, Clarification: "请指定城市或区县。"}, nil
	}}
	resp, handled, err := svc.retrieveAddress(context.Background(), ctx)
	if err != nil || !handled || len(resp.RetrieveSlices) != 1 {
		t.Fatalf("含糊地址没有进入澄清：%+v %v", resp, err)
	}
	for _, q := range db.queries {
		if strings.Contains(q.SQL, "COUNT(*)") {
			t.Fatal("含糊地址不得先查其它地区")
		}
	}
}

func TestAddressAgentPreservesRequestedFields(t *testing.T) {
	values := map[string]string{"医院名称": "测试机构", "地址": "测试街12号", "区县": "西湖区", "联系电话": "010-12345678", "联系人": "测试联系人", "内部备注": "测试备注"}
	basic := projectAddressRecord(values, "西湖区有哪些医疗机构")
	if basic["医院名称"] != "测试机构" || basic["地址"] != "测试街12号" || basic["联系电话"] != "" || basic["内部备注"] != "" {
		t.Fatalf("默认名单字段不正确：%v", basic)
	}
	contacts := projectAddressRecord(values, "西湖区医疗机构的联系方式")
	if contacts["联系电话"] != values["联系电话"] || contacts["联系人"] != values["联系人"] {
		t.Fatalf("明确要求的联系方式不得丢失：%v", contacts)
	}
	named := projectAddressRecord(values, "查询西湖区医疗机构的内部备注")
	if named["内部备注"] != values["内部备注"] {
		t.Fatalf("明确点名的知识字段不得丢失：%v", named)
	}
	full := projectAddressRecord(values, "查询西湖区医疗机构详细信息")
	if len(full) != len(values) {
		t.Fatalf("详细信息应保留全部字段：%v", full)
	}
}
