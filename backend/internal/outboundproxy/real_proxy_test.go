package outboundproxy

import (
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// TestRealProxyCarriesTheRequestWithCredentials drives the production proxy
// transport through a local forward proxy: a closed port first, then a proxy
// that answers absolute-form requests itself.
func TestRealProxyCarriesTheRequestWithCredentials(t *testing.T) {
	var auth, target string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, target = r.Header.Get("Proxy-Authorization"), r.URL.String()
		_, _ = io.WriteString(w, "via proxy")
	}))
	defer proxy.Close()
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadPort := closed.Addr().(*net.TCPAddr).Port
	_ = closed.Close()
	_, port, _ := net.SplitHostPort(proxy.Listener.Addr().String())
	livePort, _ := strconv.Atoi(port)

	normalized, err := Normalize(Config{
		Proxies: []Proxy{
			{ID: "dead", Scheme: "http", Host: "127.0.0.1", Port: deadPort, Enabled: true},
			{ID: "live", Scheme: "http", Host: "127.0.0.1", Port: livePort, Username: "user", Password: "synthetic-pass", Enabled: true},
		},
		Routes: map[Area]Route{AreaYoutube: {Enabled: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	pool := NewPool(nil, "")
	pool.set(normalized)
	client := &http.Client{Transport: pool.Transport(AreaYoutube)}
	response, err := client.Get("http://feeds.example.test/videos.xml?channel_id=synthetic")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if string(body) != "via proxy" || target != "http://feeds.example.test/videos.xml?channel_id=synthetic" {
		t.Fatalf("body=%q target=%q", body, target)
	}
	if auth != "Basic "+base64.StdEncoding.EncodeToString([]byte("user:synthetic-pass")) {
		t.Fatalf("auth=%q", auth)
	}
	if health := pool.Health(normalized); health["dead"].CoolingUntil == nil || health["live"].LastSuccessAt == nil {
		t.Fatalf("health=%+v", health)
	}
}
