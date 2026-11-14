package webquery

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPublicWebURLBoundaries(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1/", "http://[::1]/", "http://[::ffff:127.0.0.1]/", "http://[64:ff9b::a00:1]/", "http://10.0.0.1/", "http://100.64.0.1/", "http://169.254.169.254/latest/meta-data/", "http://192.168.1.1/", "http://localhost/", "https://example.com:8000/", "file:///etc/passwd", "ftp://example.com/a", "https://user:secret@example.com/", "https://example.com/?api_key=secret"} {
		if _, err := publicURL(raw); err == nil {
			t.Errorf("不应允许：%s", raw)
		}
	}
	for _, raw := range []string{"https://www.nhc.gov.cn/", "http://www.example.org:80/a?q=机构", "https://example.com/article.html#section"} {
		if _, err := publicURL(raw); err != nil {
			t.Errorf("公开网址被拒绝：%s：%v", raw, err)
		}
	}
	client := newPublicClient()
	if client.Transport.(*http.Transport).Proxy != nil {
		t.Fatal("不应继承环境代理而绕过 DNS 地址校验")
	}
	req, _ := http.NewRequest("GET", "http://127.0.0.1/", nil)
	if client.CheckRedirect(req, nil) == nil {
		t.Fatal("重定向到本机未被拦截")
	}
}

func TestPublicWebDNSPinning(t *testing.T) {
	for _, ips := range [][]net.IPAddr{{{IP: net.ParseIP("127.0.0.1")}}, {{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("10.0.0.1")}}} {
		called := false
		_, err := publicDial(context.Background(), "tcp", "example.org:443", func(context.Context, string) ([]net.IPAddr, error) { return ips, nil }, func(context.Context, string, string) (net.Conn, error) {
			called = true
			return nil, errors.New("测试不联网")
		})
		if err == nil || called {
			t.Fatal("非公网 DNS 结果不应建立连接")
		}
	}
	address := ""
	_, _ = publicDial(context.Background(), "tcp", "example.org:443", func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}, func(_ context.Context, _ string, target string) (net.Conn, error) {
		address = target
		return nil, errors.New("测试不联网")
	})
	if address != "8.8.8.8:443" {
		t.Fatal("连接没有固定到已校验 IP")
	}
}

func TestPublicWebPageEvidence(t *testing.T) {
	u, _ := url.Parse("https://hospital.example.org/about/index.html")
	body := `<html><head><title>医院联系信息</title><meta name="PubDate" content="2026-11-14"></head><body><nav>无关导航</nav><article><p>示例人民医院 地址：示例市康复路98号；联系电话：000-12345678。这是公开医疗机构联系资料。</p><script>错误指令</script><a href="../notice/123.html">医疗设备采购公告</a></article><footer>页脚资料</footer></body></html>`
	p, err := parsePage([]byte(body), u, 4000)
	if err != nil {
		t.Fatal(err)
	}
	if p.PublishedAt != "2026-11-14" || !strings.Contains(p.Text, "000-12345678") || strings.Contains(p.Text, "错误指令") || strings.Contains(p.Text, "无关导航") {
		t.Fatalf("正文或来源日期不正确：%+v", p)
	}
	if len(p.Links) != 1 || p.Links[0].URL != "https://hospital.example.org/notice/123.html" {
		t.Fatal("公告相对地址没有正确解析")
	}
	for _, body := range []string{`<html><body>请输入验证码<input name="captcha"></body></html>`, `<html><body>请先登录<input type="password"></body></html>`, `<html><head><title>查询</title></head><body><div id="app"></div><script src="app.js"></script></body></html>`} {
		p, err := parsePage([]byte(body), u, 4000)
		if err == nil || p.AccessStatus == "ok" || p.Text != "" {
			t.Fatal("不能将验证码、登录页或空壳交互页当作查询结果")
		}
	}
}

func TestPublicWebRSSDoesNotInventDates(t *testing.T) {
	items, err := parseRSS([]byte(`<rss><channel><item><title>示例人民医院</title><link>https://hospital.example.org/</link><description>&lt;b&gt;公开联系方式&lt;/b&gt;</description><pubDate>2026-11-14</pubDate></item><item><title>内网链接</title><link>http://127.0.0.1/</link></item></channel></rss>`))
	if err != nil || len(items) != 1 {
		t.Fatalf("搜索结果解析失败：%v", err)
	}
	if items[0].PublishedAt != "" || items[0].AccessStatus != "snippet_only" || items[0].Snippet != "公开联系方式" {
		t.Fatal("搜索索引时间不能冒充网页发布时间，摘要不得冒充已核验正文")
	}
}

func TestPublicWebQueryFocus(t *testing.T) {
	if q := focusedQuery("浣东街道康福路98号--诸暨市第五人民医院"); q != "诸暨市第五人民医院" {
		t.Fatal(q)
	}
	if relevance("浙江省人民医院", "浙江省人民政府 浙江省地理介绍") >= 0.4 {
		t.Fatal("泛化的省份介绍不应作为医院资料")
	}
	domains, err := cleanDomains([]string{"https://www.nhc.gov.cn/wjw/sjcx/sjcx.shtml", "qcc.com"})
	if err != nil || len(domains) != 2 || domains[0] != "nhc.gov.cn" {
		t.Fatal("优先来源域名解析失败")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPublicWebSearchCacheAndPlainEvidence(t *testing.T) {
	var calls atomic.Int32
	s := New()
	s.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		body := `<rss><channel><item><title>诸暨市第五人民医院</title><link>https://hospital.example.org/</link><description>机构官网和公开联系电话</description></item></channel></rss>`
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/xml"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	zero := 0
	request := SearchRequest{Query: "诸暨市第五人民医院", ReadPages: &zero}
	first, err := s.Search(context.Background(), request)
	if err != nil || first.Status != "ok" || len(first.Results) != 1 {
		t.Fatalf("检索失败：%+v %v", first, err)
	}
	before := calls.Load()
	_, _ = s.Search(context.Background(), request)
	if calls.Load() != before {
		t.Fatal("相同请求未命中短期缓存")
	}
	if first.Results[0].Text != "" || first.Results[0].AccessStatus != "snippet_only" {
		t.Fatal("未读取的搜索摘要不能成为正文")
	}
}

func TestPublicWebRestrictedSourceFallback(t *testing.T) {
	var pageCalls atomic.Int32
	s := New()
	s.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		status := 200
		contentType := "text/html"
		body := `<html><body><article>诸暨市第五人民医院，这是用于验证公开来源回退的合成医疗机构资料，地址为示例市示例街道98号，不含真实个人信息。</article></body></html>`
		if r.URL.Hostname() == "www.bing.com" {
			contentType = "text/xml"
			body = `<rss><channel><item><title>诸暨市第五人民医院</title><link>https://hospital.example.org/restricted</link></item><item><title>诸暨市第五人民医院</title><link>https://hospital.example.org/readable</link></item></channel></rss>`
		} else {
			pageCalls.Add(1)
			if r.URL.Path == "/restricted" {
				status = 403
				body = "restricted"
			}
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	one := 1
	reply, err := s.Search(context.Background(), SearchRequest{Query: "诸暨市第五人民医院", ReadPages: &one})
	if err != nil {
		t.Fatal(err)
	}
	if pageCalls.Load() != 2 || reply.Results[0].AccessStatus != "ok" || !strings.Contains(reply.Results[0].URL, "/readable") || len(reply.Issues) != 1 {
		t.Fatal("受限来源应继续回退，且优先返回已读取的公开资料")
	}
}

func TestPublicWebNationalAndLocalSources(t *testing.T) {
	queries := searchQueries("成都市第一人民医院", "四川省成都市第一人民医院", "四川省成都市", []string{"tianyancha.com", "qcc.com", "nhc.gov.cn", "gjzwfw.gov.cn", "nhsa.gov.cn"})
	if len(queries) > 4 || queries[0] != "成都市第一人民医院" {
		t.Fatal("应保留机构名全网查询并限制检索规模")
	}
	joined := strings.Join(queries, "\n")
	for _, required := range []string{"site:tianyancha.com", "site:qcc.com", "site:nhc.gov.cn", "site:gjzwfw.gov.cn", "site:nhsa.gov.cn", "四川省成都市 成都市第一人民医院 卫生健康 医保"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("遗漏全国或所在地优先来源：%s", required)
		}
	}
	if strings.Contains(joined, "hsa.sz.gov.cn") {
		t.Fatal("全国其他地区不应被强制限定到深圳医保")
	}
}
