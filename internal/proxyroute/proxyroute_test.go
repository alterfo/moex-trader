package proxyroute

import (
	"net/http"
	"net/url"
	"testing"
)

func TestFuncBypassesLocalAddresses(t *testing.T) {
	proxyURL, _ := url.Parse("socks5://127.0.0.1:3333")
	f := Func(proxyURL)
	cases := map[string]bool{
		"http://127.0.0.1:8080/x":        false,
		"http://localhost/x":             false,
		"http://[::1]:9/x":               false,
		"http://10.1.2.3/x":              false,
		"http://192.168.0.5/x":           false,
		"http://172.16.4.4/x":            false,
		"http://169.254.1.1/x":           false,
		"http://ai-box.local/x":          false,
		"https://api.telegram.org/bot/x": true,
		"https://t.me/s/channel":         true,
		"http://8.8.8.8/x":               true,
		"http://172.32.0.1/x":            true,
	}
	for raw, wantProxy := range cases {
		req, _ := http.NewRequest(http.MethodGet, raw, nil)
		got, err := f(req)
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if (got != nil) != wantProxy {
			t.Errorf("%s: proxied=%v want %v", raw, got != nil, wantProxy)
		}
	}
}
