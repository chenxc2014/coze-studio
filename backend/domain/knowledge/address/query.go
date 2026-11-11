// Package address 识别明确的地址范围查询，不推断距离或医疗机构属性。
package address

import (
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type Region struct{ Province, City, District string }
type Filter struct {
	Province string `json:"province,omitempty"`
	City     string `json:"city,omitempty"`
	District string `json:"district,omitempty"`
	Address  string `json:"address,omitempty"`
}
type Query struct {
	Filter
	Page, PageSize int
	CountOnly      bool
}

var pageRE = regexp.MustCompile(`第([0-9]{1,5})页`)
var roadRE = regexp.MustCompile(`^[\p{Han}A-Za-z0-9()\-#]*(大道|街道|胡同|公路|路|街|巷|弄|镇|乡|村)[\p{Han}A-Za-z0-9()\-#]*$`)
var provinceRE = regexp.MustCompile(`^[\p{Han}]{2,12}?(省|自治区|特别行政区)`)
var cityRE = regexp.MustCompile(`^[\p{Han}]{2,12}?市`)
var districtRE = regexp.MustCompile(`^[\p{Han}]{2,12}?(区|县|旗)`)

// Normalize 保留原地址文字，仅统一空白、全角数字和括号。
func Normalize(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		if r >= '０' && r <= '９' {
			return r - '０' + '0'
		}
		if r == '（' {
			return '('
		}
		if r == '）' {
			return ')'
		}
		return r
	}, strings.TrimSpace(s))
}

// UnsupportedConstraints 防止识别器丢弃尚未实现的类型、等级或距离条件。
func UnsupportedConstraints(input string) bool {
	s := Normalize(input)
	if strings.Contains(s, "医院") && !strings.Contains(s, "医疗机构") {
		return true
	}
	for _, v := range []string{"附近", "公里", "距离", "最近", "三甲", "二甲", "三级", "二级", "公立", "民营", "最好", "排名"} {
		if strings.Contains(s, v) {
			return true
		}
	}
	return false
}

func alias(s string) string {
	for _, suffix := range []string{"特别行政区", "自治区", "省", "市", "区", "县", "旗"} {
		if strings.HasSuffix(s, suffix) {
			return strings.TrimSuffix(s, suffix)
		}
	}
	return s
}

func regionValues(regions []Region, level int) []string {
	seen := map[string]bool{}
	values := []string{}
	for _, r := range regions {
		v := Normalize(r.Province)
		if level == 1 {
			v = Normalize(r.City)
		}
		if level == 2 {
			v = Normalize(r.District)
		}
		if v != "" && !seen[v] {
			seen[v] = true
			values = append(values, v)
		}
	}
	sort.Slice(values, func(i, j int) bool {
		if len(values[i]) == len(values[j]) {
			return values[i] < values[j]
		}
		return len(values[i]) > len(values[j])
	})
	return values
}

func takeRegion(s string, values []string, allowPrefixAlias bool) (string, string) {
	for _, value := range values {
		if strings.HasPrefix(s, value) {
			return value, strings.TrimPrefix(s, value)
		}
	}
	for _, value := range values {
		a := alias(value)
		if len([]rune(a)) >= 2 && (s == a || allowPrefixAlias && strings.HasPrefix(s, a)) {
			return value, strings.TrimPrefix(s, a)
		}
	}
	return "", s
}

// Canonical 使用表内真实行政区名称规范明确字段，不把未知地址映射到邻近地区。
func Canonical(s string, regions []Region, level int) string {
	s = Normalize(s)
	found := ""
	for _, value := range regionValues(regions, level) {
		if s == value {
			return value
		}
		if s == alias(value) {
			if found != "" && found != value {
				return s
			}
			found = value
		}
	}
	if found != "" {
		return found
	}
	return s
}

// Parse 只接管可完整解释的地址列表或计数问题；其它业务条件交还原检索链路。
func Parse(input string, regions []Region) (Query, bool, error) {
	q := Query{Page: 1, PageSize: 10}
	s := Normalize(input)
	s = strings.NewReplacer("，", "", ",", "", "。", "", "？", "", "?", "", "！", "", "!", "").Replace(s)
	// “医院”可能是机构类型条件，不能把所有医疗机构的数量当作医院数量。
	if strings.Contains(s, "医院") && !strings.Contains(s, "医疗机构") {
		return q, false, nil
	}
	if len([]rune(s)) > 300 {
		return q, false, nil
	}
	for _, unsupported := range []string{"附近", "公里", "距离", "最近", "三甲", "二甲", "三级", "二级", "公立", "民营", "最好", "排名"} {
		if strings.Contains(s, unsupported) {
			return q, false, nil
		}
	}
	if matches := pageRE.FindAllStringSubmatch(s, -1); len(matches) > 0 {
		if len(matches) != 1 {
			return q, true, errors.New("请只指定一个页码")
		}
		q.Page, _ = strconv.Atoi(matches[0][1])
		if q.Page < 1 || q.Page > 10000 {
			return q, true, errors.New("页码必须在 1 到 10000 之间")
		}
		s = pageRE.ReplaceAllString(s, "")
	}
	q.CountOnly = strings.Contains(s, "多少") || strings.Contains(s, "几家") || strings.Contains(s, "数量") || strings.Contains(s, "总数")
	s = strings.Trim(s, "，,。？?!！：:；;“”\" ")
	for {
		before := s
		for _, prefix := range []string{"请帮我", "帮我", "我想要", "我想", "麻烦", "请", "统计一下", "统计", "查找一下", "查询一下", "查一下", "列出", "列一下", "查找", "查询", "找一下", "找", "查", "看看", "在", "地址是", "地址为", "位于"} {
			if strings.HasPrefix(s, prefix) {
				s = strings.TrimPrefix(s, prefix)
				break
			}
		}
		if s == before {
			break
		}
	}
	intent := false
	for _, suffix := range []string{"这个地址有哪些医疗机构", "范围内的医疗机构有哪些", "范围内有哪些医疗机构", "这边的医疗机构有哪些", "的医疗机构有哪些", "的医院有哪些", "有哪些医疗机构", "有哪些医院", "有多少家医疗机构", "有多少家医院", "有几家医院", "医疗机构数量", "医疗机构总数", "的医疗机构", "的医院", "医疗机构名单", "医院名单", "医疗机构", "医院"} {
		if strings.HasSuffix(s, suffix) {
			s = strings.TrimSuffix(s, suffix)
			intent = true
			break
		}
	}
	if !intent {
		return q, false, nil
	}
	s = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(s, "范围内"), "内"), "的")
	q.Province, s = takeRegion(s, regionValues(regions, 0), true)
	if q.Province == "" {
		if v := provinceRE.FindString(s); v != "" {
			q.Province = v
			s = strings.TrimPrefix(s, v)
		}
	}
	q.City, s = takeRegion(s, regionValues(regions, 1), true)
	if q.City == "" {
		if v := cityRE.FindString(s); v != "" {
			q.City = v
			s = strings.TrimPrefix(s, v)
		}
	}
	q.District, s = takeRegion(s, regionValues(regions, 2), false)
	if q.District == "" {
		if v := districtRE.FindString(s); v != "" {
			q.District = v
			s = strings.TrimPrefix(s, v)
		}
	}
	if s != "" {
		if !roadRE.MatchString(s) {
			return q, false, nil
		}
		q.Address = s
	}
	if q.Province == "" && q.City == "" && q.District == "" && q.Address == "" {
		return q, false, nil
	}
	return q, true, nil
}
