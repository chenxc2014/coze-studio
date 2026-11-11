package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	knowledgeModel "github.com/coze-dev/coze-studio/backend/crossdomain/knowledge/model"
	"github.com/coze-dev/coze-studio/backend/domain/knowledge/address"
	"github.com/coze-dev/coze-studio/backend/domain/knowledge/entity"
	"github.com/coze-dev/coze-studio/backend/domain/knowledge/internal/consts"
	"github.com/coze-dev/coze-studio/backend/domain/knowledge/internal/convert"
	"github.com/coze-dev/coze-studio/backend/domain/knowledge/internal/dal/model"
	"github.com/coze-dev/coze-studio/backend/infra/document"
	"github.com/coze-dev/coze-studio/backend/infra/rdb"
	"github.com/coze-dev/coze-studio/backend/pkg/lang/ptr"
	"github.com/coze-dev/coze-studio/backend/pkg/sonic"
)

type AddressQueryError struct{ Message string }

func (e *AddressQueryError) Error() string { return e.Message }
func addressInvalid(message string) error  { return &AddressQueryError{Message: message} }

type AddressQueryRequest struct {
	DocumentID     int64
	Filter         address.Filter
	Query          string
	Page, PageSize int
	AllowPartial   bool
	CountOnly      bool
	AnswerQuery    string
}
type AddressRow struct {
	SliceID string            `json:"slice_id"`
	Values  map[string]string `json:"values"`
}
type AddressQueryResponse struct {
	DocumentID    string         `json:"document_id"`
	KnowledgeID   string         `json:"dataset_id"`
	DocumentName  string         `json:"document_name"`
	KnowledgeName string         `json:"dataset_name"`
	Filter        address.Filter `json:"filter"`
	Page          int            `json:"page"`
	PageSize      int            `json:"page_size"`
	Total         int64          `json:"total"`
	HasMore       bool           `json:"has_more"`
	Complete      bool           `json:"import_complete"`
	CountOnly     bool           `json:"count_only"`
	Columns       []string       `json:"columns"`
	Rows          []AddressRow   `json:"rows"`
}

var addressIdentifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,63}$`)

type addressColumns struct{ province, city, district, address string }

func getAddressColumns(info *entity.TableInfo) (addressColumns, error) {
	cols := addressColumns{}
	if info == nil || !addressIdentifier.MatchString(info.PhysicalTableName) {
		return cols, addressInvalid("该文档没有有效的表格数据")
	}
	for _, c := range info.Columns {
		if c == nil || c.ID <= 0 || c.Type != document.TableColumnTypeString {
			continue
		}
		physical := convert.ColumnIDToRDBField(c.ID)
		switch strings.TrimSpace(c.Name) {
		case "省", "省份":
			cols.province = physical
		case "市", "城市", "地级市":
			cols.city = physical
		case "区县", "区/县", "区（县）", "区(县)":
			cols.district = physical
		case "地址", "详细地址", "机构地址":
			cols.address = physical
		}
	}
	if cols.address == "" {
		return cols, addressInvalid("该表格需要有字符串类型的地址列")
	}
	return cols, nil
}

func addressString(v any) string {
	if v == nil {
		return ""
	}
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return fmt.Sprint(v)
}

// 字段名只来自服务器保存的表结构，用户值始终作为参数传递。
func locationSQL(column string) string {
	s := "`" + column + "`"
	for _, pair := range [][2]string{{" ", ""}, {"　", ""}, {"０", "0"}, {"１", "1"}, {"２", "2"}, {"３", "3"}, {"４", "4"}, {"５", "5"}, {"６", "6"}, {"７", "7"}, {"８", "8"}, {"９", "9"}, {"（", "("}, {"）", ")"}} {
		s = "REPLACE(" + s + ", '" + pair[0] + "', '" + pair[1] + "')"
	}
	for _, code := range []string{"9", "10", "13"} {
		s = "REPLACE(" + s + ", CHAR(" + code + "), '')"
	}
	return s
}

func addressWhere(cols addressColumns, filter address.Filter) (string, []any, error) {
	conditions := []string{}
	args := []any{}
	for _, item := range []struct {
		column, value string
		contains      bool
	}{{cols.province, filter.Province, false}, {cols.city, filter.City, false}, {cols.district, filter.District, false}, {cols.address, filter.Address, true}} {
		if item.value == "" {
			continue
		}
		if item.column == "" {
			return "", nil, addressInvalid("该表格缺少查询所需的行政区字段")
		}
		if len([]rune(item.value)) > 150 {
			return "", nil, addressInvalid("单个地址条件不能超过 150 个字符")
		}
		expr := locationSQL(item.column)
		if item.contains {
			conditions = append(conditions, "INSTR("+expr+", ?) > 0")
		} else {
			conditions = append(conditions, expr+" = ?")
		}
		args = append(args, address.Normalize(item.value))
	}
	if len(conditions) == 0 {
		return "", nil, addressInvalid("请填写至少一个省、市、区县或地址条件")
	}
	return strings.Join(conditions, " AND "), args, nil
}

func (k *knowledgeSVC) addressRegions(ctx context.Context, doc *model.KnowledgeDocument, cols addressColumns) ([]address.Region, error) {
	fields := []string{}
	aliases := []string{"province", "city", "district"}
	for i, c := range []string{cols.province, cols.city, cols.district} {
		if c == "" {
			fields = append(fields, "'' AS "+aliases[i])
		} else {
			fields = append(fields, "`"+c+"` AS "+aliases[i])
		}
	}
	result, err := k.rdb.ExecuteSQL(ctx, &rdb.ExecuteSQLRequest{SQL: "SELECT DISTINCT " + strings.Join(fields, ",") + " FROM `" + doc.TableInfo.PhysicalTableName + "` LIMIT 5001"})
	if err != nil {
		return nil, err
	}
	if result == nil || result.ResultSet == nil {
		return nil, errors.New("地址行政区查询返回为空")
	}
	if len(result.ResultSet.Rows) > 5000 {
		return nil, addressInvalid("行政区数据较多，请使用明确的地址字段条件")
	}
	regions := make([]address.Region, 0, len(result.ResultSet.Rows))
	for _, r := range result.ResultSet.Rows {
		regions = append(regions, address.Region{Province: addressString(r["province"]), City: addressString(r["city"]), District: addressString(r["district"])})
	}
	return regions, nil
}

func (k *knowledgeSVC) QueryAddress(ctx context.Context, request *AddressQueryRequest) (*AddressQueryResponse, error) {
	if request == nil || request.DocumentID <= 0 {
		return nil, addressInvalid("文档 ID 不正确")
	}
	doc, err := k.documentRepo.GetByID(ctx, request.DocumentID)
	if err != nil {
		return nil, err
	}
	if doc == nil || doc.DeletedAt.Valid || doc.DocumentType != int32(knowledgeModel.DocumentTypeTable) {
		return nil, addressInvalid("文档不存在或不是表格知识")
	}
	kn, err := k.knowledgeRepo.GetByID(ctx, doc.KnowledgeID)
	if err != nil {
		return nil, err
	}
	if kn == nil || kn.DeletedAt.Valid || kn.Status != int32(knowledgeModel.KnowledgeStatusEnable) {
		return nil, addressInvalid("知识库不可用")
	}
	complete := doc.Status == int32(entity.DocumentStatusEnable)
	if !complete && (!request.AllowPartial || doc.Status != int32(entity.DocumentStatusChunking)) {
		return nil, addressInvalid("文档尚未处理完成；如需预览，显式选择查看已导入的部分数据")
	}
	cols, err := getAddressColumns(doc.TableInfo)
	if err != nil {
		return nil, err
	}
	q := address.Query{Filter: request.Filter, Page: request.Page, PageSize: request.PageSize, CountOnly: request.CountOnly}
	regions, err := k.addressRegions(ctx, doc, cols)
	if err != nil {
		return nil, err
	}
	if request.Query != "" {
		parsed, ok, parseErr := address.Parse(request.Query, regions)
		if parseErr != nil {
			return nil, addressInvalid(parseErr.Error())
		}
		if !ok {
			return nil, addressInvalid("请使用明确的地址、街道或区县问法，或直接填写筛选字段")
		}
		q = parsed
		if request.Page != 0 {
			q.Page = request.Page
		}
		if request.PageSize != 0 {
			q.PageSize = request.PageSize
		}
		for _, pair := range []struct {
			explicit string
			target   *string
			level    int
		}{{request.Filter.Province, &q.Province, 0}, {request.Filter.City, &q.City, 1}, {request.Filter.District, &q.District, 2}, {request.Filter.Address, &q.Address, 3}} {
			if pair.explicit == "" {
				continue
			}
			v := address.Normalize(pair.explicit)
			if pair.level < 3 {
				v = address.Canonical(v, regions, pair.level)
			}
			if *pair.target != "" && *pair.target != v {
				return nil, addressInvalid("问题与筛选字段中的地址条件冲突")
			}
			*pair.target = v
		}
	}
	q.Province = address.Canonical(q.Province, regions, 0)
	q.City = address.Canonical(q.City, regions, 1)
	q.District = address.Canonical(q.District, regions, 2)
	q.Address = address.Normalize(q.Address)
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
	if q.Page < 1 || q.Page > 10000 || q.PageSize < 1 || q.PageSize > 100 {
		return nil, addressInvalid("页码须为 1～10000，每页须为 1～100 条")
	}
	where, args, err := addressWhere(cols, q.Filter)
	if err != nil {
		return nil, err
	}
	table := "`" + doc.TableInfo.PhysicalTableName + "`"
	count, err := k.rdb.ExecuteSQL(ctx, &rdb.ExecuteSQLRequest{SQL: "SELECT COUNT(*) AS matched_count FROM " + table + " WHERE " + where, Params: args})
	if err != nil {
		return nil, err
	}
	if count == nil || count.ResultSet == nil || len(count.ResultSet.Rows) != 1 {
		return nil, errors.New("地址匹配计数返回异常")
	}
	total, err := strconv.ParseInt(addressString(count.ResultSet.Rows[0]["matched_count"]), 10, 64)
	if err != nil {
		return nil, err
	}
	response := &AddressQueryResponse{DocumentID: strconv.FormatInt(doc.ID, 10), KnowledgeID: strconv.FormatInt(doc.KnowledgeID, 10), DocumentName: doc.Name, KnowledgeName: kn.Name, Filter: q.Filter, Page: q.Page, PageSize: q.PageSize, Total: total, Complete: complete, CountOnly: q.CountOnly, Columns: []string{}, Rows: []AddressRow{}}
	for _, column := range doc.TableInfo.Columns {
		if column != nil && column.ID > 0 && column.Name != consts.RDBFieldID {
			response.Columns = append(response.Columns, column.Name)
		}
	}
	if q.CountOnly {
		return response, nil
	}
	queryArgs := append(append([]any{}, args...), q.PageSize, (q.Page-1)*q.PageSize)
	result, err := k.rdb.ExecuteSQL(ctx, &rdb.ExecuteSQLRequest{SQL: "SELECT * FROM " + table + " WHERE " + where + " ORDER BY `" + consts.RDBFieldID + "` ASC LIMIT ? OFFSET ?", Params: queryArgs})
	if err != nil {
		return nil, err
	}
	if result == nil || result.ResultSet == nil {
		return nil, errors.New("地址查询结果异常")
	}
	for _, r := range result.ResultSet.Rows {
		row := AddressRow{SliceID: addressString(r[consts.RDBFieldID]), Values: map[string]string{}}
		for _, column := range doc.TableInfo.Columns {
			if column != nil && column.ID > 0 && column.Name != consts.RDBFieldID {
				row.Values[column.Name] = addressString(r[convert.ColumnIDToRDBField(column.ID)])
			}
		}
		response.Rows = append(response.Rows, row)
	}
	response.HasMore = int64((q.Page-1)*q.PageSize+len(response.Rows)) < total
	return response, nil
}

// 非智能体调用保留试点文档配置；智能体只从其实际绑定的知识范围取数。
func addressAutoEnabled(id int64) bool {
	for _, value := range strings.Split(os.Getenv("COZE_ADDRESS_DOCUMENT_IDS"), ",") {
		if strings.TrimSpace(value) == strconv.FormatInt(id, 10) {
			return true
		}
	}
	return false
}

func (k *knowledgeSVC) retrieveAddress(ctx context.Context, request *RetrieveContext) (*RetrieveResponse, bool, error) {
	response := &RetrieveResponse{}
	if address.UnsupportedConstraints(request.OriginQuery) {
		return response, false, nil
	}
	var resolved *knowledgeModel.AddressRecall
	var resolveErr error
	resolveCalled := false
	for _, doc := range request.Documents {
		if !request.AutoAddress && !addressAutoEnabled(doc.ID) {
			continue
		}
		cols, err := getAddressColumns(doc.TableInfo)
		if err != nil {
			continue
		}
		regions, err := k.addressRegions(ctx, doc, cols)
		if err != nil {
			return nil, true, err
		}
		_, ok, err := address.Parse(request.OriginQuery, regions)
		if err != nil {
			return nil, true, err
		}
		queryRequest := &AddressQueryRequest{DocumentID: doc.ID, Query: request.OriginQuery}
		if !ok {
			if request.ResolveAddress == nil {
				continue
			}
			if !resolveCalled {
				resolved, resolveErr = request.ResolveAddress(ctx)
				resolveCalled = true
			}
			if resolveErr != nil {
				continue
			}
			if resolved == nil || !resolved.Handled {
				continue
			}
			if resolved.Clarification != "" {
				body, _ := sonic.Marshal(map[string]any{"需要澄清": resolved.Clarification, "说明": "尚未执行地址查询，请先向用户确认。"})
				response.RetrieveSlices = append(response.RetrieveSlices, addressRecallSlice(doc, string(body), ""))
				continue
			}
			queryRequest = &AddressQueryRequest{DocumentID: doc.ID, Filter: address.Filter{Province: resolved.Province, City: resolved.City, District: resolved.District, Address: resolved.Address}, Page: resolved.Page, PageSize: resolved.PageSize, CountOnly: resolved.CountOnly}
		}
		queryRequest.AnswerQuery = request.OriginQuery
		result, records, complete, err := k.readAddressForAgent(ctx, queryRequest)
		if err != nil {
			return nil, true, err
		}
		info := map[string]any{"查询条件": result.Filter, "匹配记录总数": result.Total, "已读取记录数": len(records), "资料是否完整": complete, "检索资料": records, "说明": "系统已自动完成本次地址检索。匹配总数是记录数，不保证机构去重；资料不完整时只能作为摘要，不得声称完整名单。"}
		if result.CountOnly {
			info["说明"] = "这是按地址字段筛选的匹配记录数，不保证机构去重；计数问题不返回机构明细。"
		}
		body, err := sonic.Marshal(info)
		if err != nil {
			return nil, true, err
		}
		response.RetrieveSlices = append(response.RetrieveSlices, addressRecallSlice(doc, string(body), result.KnowledgeName))
	}
	return response, len(response.RetrieveSlices) > 0, nil
}

// 分批读取留在服务内部，不把数据库页码变成用户与智能体的交互要求。
func (k *knowledgeSVC) readAddressForAgent(ctx context.Context, input *AddressQueryRequest) (*AddressQueryResponse, []map[string]string, bool, error) {
	request := *input
	request.Page = 1
	request.PageSize = 100
	first, err := k.QueryAddress(ctx, &request)
	if err != nil {
		return nil, nil, false, err
	}
	records := []map[string]string{}
	characters := 0
	current := first
	if first.CountOnly {
		return first, records, true, nil
	}
	for {
		for _, row := range current.Rows {
			projected := projectAddressRecord(row.Values, input.AnswerQuery)
			encoded, _ := sonic.Marshal(projected)
			size := len([]rune(string(encoded)))
			if len(records) >= 200 || characters+size > 16000 {
				return first, records, false, nil
			}
			characters += size
			records = append(records, projected)
		}
		if !current.HasMore {
			break
		}
		if len(current.Rows) < request.PageSize || len(records) >= 200 {
			return first, records, false, nil
		}
		request.Page++
		current, err = k.QueryAddress(ctx, &request)
		if err != nil {
			return nil, nil, false, err
		}
		if current.Total != first.Total {
			return first, records, false, nil
		}
	}
	return first, records, int64(len(records)) == first.Total, nil
}

// 名单默认保留名称和地址；问题明确要求的属性也要提供，避免丢失知识库原有字段。
func projectAddressRecord(values map[string]string, question string) map[string]string {
	result := map[string]string{}
	q := address.Normalize(question)
	all := strings.Contains(q, "详细信息") || strings.Contains(q, "完整信息") || strings.Contains(q, "全部信息") || strings.Contains(q, "详细资料")
	for name, value := range values {
		include := all || strings.Contains(name, "名称") || name == "名称" || name == "省" || name == "省份" || name == "市" || name == "城市" || name == "地级市" || name == "区县" || strings.Contains(name, "地址")
		if len([]rune(name)) >= 2 && strings.Contains(q, address.Normalize(name)) {
			include = true
		}
		for _, keyword := range []string{"电话", "联系人", "邮箱", "网址", "邮编", "法人", "床位"} {
			if strings.Contains(q, keyword) && strings.Contains(name, keyword) {
				include = true
			}
		}
		if strings.Contains(q, "联系方式") && (strings.Contains(name, "电话") || strings.Contains(name, "邮箱") || strings.Contains(name, "联系人")) {
			include = true
		}
		if include {
			result[name] = value
		}
	}
	return result
}

func addressRecallSlice(doc *model.KnowledgeDocument, body, knowledgeName string) *knowledgeModel.RetrieveSlice {
	return &knowledgeModel.RetrieveSlice{Score: 1, Slice: &knowledgeModel.Slice{Info: knowledgeModel.Info{SpaceID: doc.SpaceID, Name: knowledgeName, CreatorID: doc.CreatorID}, KnowledgeID: doc.KnowledgeID, DocumentID: doc.ID, DocumentName: doc.Name, RawContent: []*knowledgeModel.SliceContent{{Type: knowledgeModel.SliceContentTypeText, Text: ptr.Of(body)}}, Extra: map[string]string{"structured_address": "true"}}}
}
