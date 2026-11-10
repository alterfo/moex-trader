package proxyroute

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

func IsLocalHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return true
	}
	if i := strings.IndexByte(host, '%'); i >= 0 {
		host = host[:i]
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

func Func(proxyURL *url.URL) func(*http.Request) (*url.URL, error) {
	return func(req *http.Request) (*url.URL, error) {
		if IsLocalHost(req.URL.Hostname()) {
			return nil, nil
		}
		return proxyURL, nil
	}
}
