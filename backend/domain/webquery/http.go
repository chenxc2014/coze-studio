package webquery

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var deniedNetworks = []string{"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4", "::/96", "::1/128", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001::/32", "2001:db8::/32", "2002::/16", "fc00::/7", "fe80::/10", "ff00::/8"}

func publicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false
	}
	for _, network := range deniedNetworks {
		if netip.MustParsePrefix(network).Contains(ip) {
			return false
		}
	}
	return true
}

func publicURL(raw string) (*url.URL, error) {
	if len(raw) > 4096 {
		return nil, fmt.Errorf("网址过长")
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return nil, fmt.Errorf("仅支持不含账号密码的公开 HTTP 或 HTTPS 网址")
	}
	if p := u.Port(); p != "" && !((u.Scheme == "http" && p == "80") || (u.Scheme == "https" && p == "443")) {
		return nil, fmt.Errorf("仅支持公开网页的标准端口")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.Contains(host, "%") {
		return nil, fmt.Errorf("禁止访问本机或内网地址")
	}
	if ip, err := netip.ParseAddr(host); err == nil && !publicIP(ip) {
		return nil, fmt.Errorf("禁止访问本机、内网或保留地址")
	}
	for key := range u.Query() {
		switch strings.ToLower(key) {
		case "api_key", "apikey", "access_token", "app_secret", "client_secret", "password":
			return nil, fmt.Errorf("联网查询只读取无需凭据的公开网页")
		}
	}
	u.Fragment = ""
	return u, nil
}

type resolver func(context.Context, string) ([]net.IPAddr, error)

// 每次建立连接都校验 DNS 结果并直接连接已校验 IP，防止重定向或 DNS 重绑定访问内网。
func publicDial(ctx context.Context, network, address string, resolve resolver, dial func(context.Context, string, string) (net.Conn, error)) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := resolve(ctx, host)
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("无法解析公开网页地址")
	}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip.IP)
		if !ok || !publicIP(addr) {
			return nil, fmt.Errorf("网页地址解析到了非公网网络，已停止访问")
		}
	}
	for _, ip := range ips {
		connection, e := dial(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if e == nil {
			return connection, nil
		}
		err = e
	}
	return nil, err
}

func newPublicClient() *http.Client {
	dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{MaxIdleConns: 16, MaxIdleConnsPerHost: 4, MaxConnsPerHost: 6, IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: 4 * time.Second, ResponseHeaderTimeout: 6 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return publicDial(ctx, network, address, net.DefaultResolver.LookupIPAddr, dialer.DialContext)
	}
	return &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 4 {
			return fmt.Errorf("网页重定向次数过多")
		}
		_, err := publicURL(req.URL.String())
		return err
	}}
}
