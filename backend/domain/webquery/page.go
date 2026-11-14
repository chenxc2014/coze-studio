package webquery

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

type Link struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}
type Page struct {
	Title          string `json:"title"`
	URL            string `json:"url"`
	Domain         string `json:"domain"`
	Snippet        string `json:"snippet,omitempty"`
	Text           string `json:"text,omitempty"`
	ContactContext string `json:"contact_context,omitempty"`
	PublishedAt    string `json:"published_at,omitempty"`
	FetchedAt      string `json:"fetched_at,omitempty"`
	AccessStatus   string `json:"access_status"`
	Truncated      bool   `json:"truncated,omitempty"`
	Links          []Link `json:"links,omitempty"`
}
type Issue struct {
	URL     string `json:"url,omitempty"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func trimText(text string, limit int) (string, bool) {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > limit {
		return string(runes[:limit]), true
	}
	return string(runes), false
}
func compact(text string) string { return strings.Join(strings.FieldsFunc(text, unicode.IsSpace), " ") }
func attr(node *html.Node, name string) string {
	for _, a := range node.Attr {
		if strings.EqualFold(a.Key, name) {
			return a.Val
		}
	}
	return ""
}
func nodeText(node *html.Node) string {
	if node == nil {
		return ""
	}
	var out strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "noscript", "template", "svg", "iframe", "nav", "footer", "header", "form":
				return
			}
			if attr(n, "aria-hidden") == "true" || strings.Contains(strings.ReplaceAll(attr(n, "style"), " ", ""), "display:none") {
				return
			}
		}
		if n.Type == html.TextNode {
			out.WriteString(n.Data)
			out.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(node)
	return compact(out.String())
}

func parsePage(body []byte, base *url.URL, limit int) (Page, error) {
	root, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return Page{}, err
	}
	p := Page{URL: base.String(), Domain: base.Hostname(), AccessStatus: "ok", FetchedAt: time.Now().UTC().Format(time.RFC3339)}
	var content *html.Node
	var fallback *html.Node
	challenge := false
	password := false
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "title":
				p.Title, _ = trimText(nodeText(n), 200)
			case "body":
				fallback = n
			case "footer":
				var footer strings.Builder
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					footer.WriteString(nodeText(c))
					footer.WriteByte(' ')
				}
				text := compact(footer.String())
				if strings.Contains(text, "地址") || strings.Contains(text, "电话") {
					p.ContactContext, _ = trimText(text, 1200)
				}
			case "article", "main":
				if content == nil || len(nodeText(n)) > len(nodeText(content)) {
					content = n
				}
			case "input":
				s := strings.ToLower(attr(n, "name") + " " + attr(n, "id") + " " + attr(n, "placeholder"))
				if strings.Contains(s, "captcha") || strings.Contains(s, "验证码") {
					challenge = true
				}
				if attr(n, "type") == "password" {
					password = true
				}
			case "meta":
				key := strings.ToLower(attr(n, "property") + attr(n, "name"))
				if strings.Contains(key, "article:published_time") || key == "pubdate" || key == "publishdate" {
					p.PublishedAt = pageDate(attr(n, "content"))
				}
			case "a":
				if len(p.Links) < 12 {
					title := nodeText(n)
					if strings.Contains(title, "公告") || strings.Contains(title, "公示") || strings.Contains(title, "通知") || strings.Contains(title, "招聘") || strings.Contains(title, "采购") {
						if u, e := url.Parse(attr(n, "href")); e == nil {
							u = base.ResolveReference(u)
							if _, e = publicURL(u.String()); e == nil && title != "" {
								label, _ := trimText(title, 140)
								p.Links = append(p.Links, Link{Title: label, URL: u.String()})
							}
						}
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(root)
	if content == nil || len(nodeText(content)) < 80 {
		content = fallback
	}
	if content == nil {
		content = root
	}
	text := nodeText(content)
	lower := strings.ToLower(text)
	if challenge || (len([]rune(text)) < 2000 && (strings.Contains(lower, "verify you are human") || strings.Contains(lower, "unusual traffic") || strings.Contains(lower, "人机验证") || strings.Contains(lower, "安全验证") || strings.Contains(lower, "请输入验证码"))) {
		p.AccessStatus = "verification_required"
		return p, fmt.Errorf("该网页需要验证码或人机验证，已跳过并继续使用其他公开来源")
	}
	if len([]rune(text)) < 1500 && (password || strings.Contains(text, "请先登录") || strings.Contains(text, "登录后查看") || strings.Contains(text, "登录查看更多")) {
		p.AccessStatus = "login_required"
		return p, fmt.Errorf("该网页要求登录或授权，已跳过")
	}
	if len([]rune(text)) < 40 && (bytes.Contains(body, []byte("<script")) || bytes.Contains(body, []byte("id=\"app\""))) {
		p.AccessStatus = "javascript_required"
		return p, fmt.Errorf("该网页需要交互式加载，当前未取得可核验正文")
	}
	if text == "" {
		p.AccessStatus = "empty"
		return p, fmt.Errorf("该网页未提供可读取正文")
	}
	p.Text, p.Truncated = trimText(text, limit)
	return p, nil
}

func (s *Service) read(ctx context.Context, raw string, limit int) (Page, *Issue) {
	u, err := publicURL(raw)
	if err != nil {
		return Page{AccessStatus: "invalid_url"}, &Issue{Status: "invalid_url", Message: err.Error()}
	}
	page := Page{URL: u.String(), Domain: u.Hostname(), AccessStatus: "unavailable"}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return page, &Issue{URL: u.String(), Status: "invalid_url", Message: "网址格式不正确"}
	}
	req.Header.Set("User-Agent", "Yaoxiaoyi-PublicWeb/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain,application/json;q=0.8,*/*;q=0.2")
	resp, err := s.client.Do(req)
	if err != nil {
		return page, &Issue{URL: u.String(), Status: "unreachable", Message: "公开网页连接失败或超时，未读取到正文"}
	}
	defer resp.Body.Close()
	page.URL = resp.Request.URL.String()
	page.Domain = resp.Request.URL.Hostname()
	if resp.StatusCode != 200 {
		status := "http_error"
		if resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode == 429 {
			status = "restricted"
		}
		page.AccessStatus = status
		return page, &Issue{URL: page.URL, Status: status, Message: fmt.Sprintf("来源返回 HTTP %d，未尝试登录、授权或绕过访问限制", resp.StatusCode)}
	}
	typeValue := strings.ToLower(resp.Header.Get("Content-Type"))
	if !strings.HasPrefix(typeValue, "text/") && !strings.Contains(typeValue, "json") && !strings.Contains(typeValue, "xml") {
		page.AccessStatus = "unsupported_content"
		return page, &Issue{URL: page.URL, Status: page.AccessStatus, Message: "该链接不是当前支持的网页或文本内容；保留链接供核对"}
	}
	reader, err := charset.NewReader(resp.Body, resp.Header.Get("Content-Type"))
	if err != nil {
		return page, &Issue{URL: page.URL, Status: "decode_error", Message: "网页编码暂不支持"}
	}
	body, err := io.ReadAll(io.LimitReader(reader, 2*1024*1024+1))
	if err != nil || len(body) > 2*1024*1024 {
		page.AccessStatus = "too_large"
		return page, &Issue{URL: page.URL, Status: "too_large", Message: "网页超过 2 MiB 或读取中断，未返回不完整正文"}
	}
	if strings.Contains(typeValue, "html") {
		parsed, e := parsePage(body, resp.Request.URL, limit)
		if e != nil {
			return parsed, &Issue{URL: parsed.URL, Status: parsed.AccessStatus, Message: e.Error()}
		}
		return parsed, nil
	}
	page.Text, page.Truncated = trimText(string(body), limit)
	page.AccessStatus = "ok"
	page.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	return page, nil
}

var institutionName = regexp.MustCompile(`[\p{Han}A-Za-z0-9（）()·]{2,60}(?:医院|卫生院|卫生服务中心|卫生服务站|门诊部|诊所)`)

func pageDate(value string) string {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02", "2006/01/02", "2006年01月02日"} {
		if t, e := time.Parse(layout, strings.TrimSpace(value)); e == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}
func focusedQuery(query string) string {
	parts := strings.FieldsFunc(query, func(r rune) bool {
		return unicode.IsSpace(r) || r == '—' || r == '-' || r == '：' || r == ':' || r == '，' || r == ','
	})
	for _, part := range parts {
		for _, prefix := range []string{"请帮我查询", "帮我查询", "请查询", "查询", "查找", "搜索", "查一下", "请问"} {
			part = strings.TrimPrefix(part, prefix)
		}
		if name := institutionName.FindString(part); name != "" {
			return name
		}
	}
	return strings.TrimSpace(query)
}
