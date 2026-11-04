package httpapi

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/config"
)

// A media body may take longer than the listener's write timeout as long as
// the client keeps reading.
func TestMediaBodyOutlastsTheListenerWriteTimeout(t *testing.T) {
	body := bytes.Repeat([]byte("kano"), 8<<20) // 32 MiB, beyond the socket buffers
	media := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		serveContent(w, r, "clip.mp4", time.Time{}, bytes.NewReader(body))
	})
	cfg := config.Default()
	cfg.WriteTimeout = 100 * time.Millisecond
	server := NewServer(RequestLog(Compress(media), io.Discard), cfg)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})

	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	response, err := client.Get("http://" + listener.Addr().String() + "/media/clip")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	// The client stalls past the listener's write timeout, then reads on.
	time.Sleep(4 * cfg.WriteTimeout)
	got, err := io.ReadAll(response.Body)
	if err != nil || len(got) != len(body) {
		t.Fatalf("read %d of %d bytes: %v", len(got), len(body), err)
	}
}
