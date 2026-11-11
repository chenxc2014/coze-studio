package address

import "testing"

func TestAddressQueries(t *testing.T) {
	regions := []Region{{"浙江省", "杭州市", "西湖区"}, {"浙江省", "杭州市", "滨江区"}, {"浙江省", "宁波市", "海曙区"}}
	cases := []struct {
		input, city, district, address string
		page                           int
	}{
		{"查一下杭州市西湖区有哪些医疗机构", "杭州市", "西湖区", "", 1},
		{"帮我查一下杭州市西湖区范围内的医疗机构有哪些。", "杭州市", "西湖区", "", 1},
		{"请列出浙江省杭州市西湖区文一路的医疗机构。", "杭州市", "西湖区", "文一路", 1},
		{"我想找杭州市文一路２３号这边的医疗机构，有哪些？", "杭州市", "", "文一路23号", 1},
		{"杭州西湖区医疗机构第2页", "杭州市", "西湖区", "", 2},
		{"宁波市滨江路有哪些医疗机构", "宁波市", "", "滨江路", 1},
		{"北京市朝阳区有哪些医疗机构", "北京市", "朝阳区", "", 1},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			q, ok, err := Parse(c.input, regions)
			if err != nil || !ok || q.City != c.city || q.District != c.district || q.Address != c.address || q.Page != c.page {
				t.Fatalf("解析结果不符合地址条件：%+v, %v, %v", q, ok, err)
			}
		})
	}
}

func TestAddressDoesNotDropUnsupportedConstraints(t *testing.T) {
	regions := []Region{{"浙江省", "杭州市", "西湖区"}}
	for _, input := range []string{"西湖区附近的医院", "杭州市西湖区有哪些三甲医院", "杭州市西湖区哪家医院最好", "杭州市西湖区有多少张床位", "你好", "查杭州市西湖区和滨江区有哪些医院"} {
		if q, ok, err := Parse(input, regions); ok && err == nil {
			t.Fatalf("不应丢弃未支持的条件 %q: %+v", input, q)
		}
	}
	q, ok, err := Parse("查杭州市西湖区有多少家医疗机构", regions)
	if !ok || err != nil || !q.CountOnly {
		t.Fatalf("计数意图未保留：%+v %v", q, err)
	}
	_, ok, err = Parse("杭州市西湖区医疗机构第0页", regions)
	if !ok || err == nil {
		t.Fatal("未拒绝非法页码")
	}
}
