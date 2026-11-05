package youtube

import (
	"context"
	"strings"
	"testing"
)

func TestYTDLPListUploadsParsesFlatPlaylist(t *testing.T) {
	y := NewYTDLP("synthetic-yt-dlp")
	var args []string
	y.runFunc = func(_ context.Context, _ []string, got ...string) ([]byte, []byte, error) {
		args = got
		return []byte(`{"entries":[{"id":"AAAAAAAAAAA","title":"new song","upload_date":"20261030","thumbnail":"https://i.ytimg.com/vi/AAAAAAAAAAA/maxresdefault.jpg","webpage_url":"https://www.youtube.com/watch?v=AAAAAAAAAAA"},{"id":"BBBBBBBBBBB","title":"scheduled","release_timestamp":1793350800,"live_status":"is_upcoming"}]}`), nil, nil
	}
	videos, err := y.ListUploads(context.Background(), "UCAAAAAAAAAAAAAAAAAAAAAA", 2)
	if err != nil || len(videos) != 2 {
		t.Fatalf("videos=%+v err=%v", videos, err)
	}
	if videos[0].ID != "AAAAAAAAAAA" || videos[0].PublishedAt != "2026-10-30T00:00:00Z" || videos[0].ThumbnailURL == "" {
		t.Fatalf("first=%+v", videos[0])
	}
	if !videos[1].IsUpcoming || videos[1].ScheduledAt == "" || videos[1].LiveStatus != LiveStatusUpcoming {
		t.Fatalf("second=%+v", videos[1])
	}
	joined := strings.Join(args, " ")
	for _, flag := range []string{"--flat-playlist", "--dump-single-json", "--skip-download", "--playlist-end", "2"} {
		if !strings.Contains(joined, flag) {
			t.Fatalf("args=%v missing %q", args, flag)
		}
	}
}

func TestYTDLPInspectKeepsRawObjectPrivateToCaller(t *testing.T) {
	y := NewYTDLP("synthetic-yt-dlp")
	var args []string
	y.runFunc = func(_ context.Context, _ []string, got ...string) ([]byte, []byte, error) {
		args = got
		return []byte(`{"id":"CCCCCCCCCCC","title":"full info","timestamp":1793361600,"duration":61,"description":"private source text","webpage_url":"https://www.youtube.com/watch?v=CCCCCCCCCCC"}`), nil, nil
	}
	video, raw, err := y.Inspect(context.Background(), "https://www.youtube.com/watch?v=CCCCCCCCCCC")
	if err != nil || video.ID != "CCCCCCCCCCC" || raw["description"] != "private source text" {
		t.Fatalf("video=%+v raw=%v err=%v", video, raw, err)
	}
	if video.DurationSeconds != 61 || video.Title != "full info" {
		t.Fatalf("video=%+v", video)
	}
	// The target follows "--", so one that looks like an option stays a target.
	if _, _, err = y.Inspect(context.Background(), "--exec=synthetic"); err != nil {
		t.Fatal(err)
	}
	if len(args) < 2 || args[len(args)-2] != "--" || args[len(args)-1] != "--exec=synthetic" {
		t.Fatalf("args=%v", args)
	}
}

func TestYTDLPReadMovesToTheNextRouteOnlyWhenItCouldNotConnect(t *testing.T) {
	const playlist = `{"entries":[{"id":"AAAAAAAAAAA","title":"new song","upload_date":"20261030"}]}`
	run := func(outcomes map[string]string) (*YTDLP, *[]string, map[string]bool) {
		y := NewYTDLP("synthetic-yt-dlp")
		var tried []string
		done := map[string]bool{}
		y.Routes = func(context.Context) ([]ProcessRoute, error) {
			routes := []ProcessRoute{}
			for _, name := range []string{"first", "second"} {
				routes = append(routes, ProcessRoute{Env: []string{"HTTPS_PROXY=http://" + name + ".example.test:8080"}, Done: func(ok bool) { done[name] = ok }})
			}
			return routes, nil
		}
		y.runFunc = func(_ context.Context, env []string, _ ...string) ([]byte, []byte, error) {
			name := strings.TrimSuffix(strings.TrimPrefix(env[0], "HTTPS_PROXY=http://"), ".example.test:8080")
			tried = append(tried, name)
			switch outcomes[name] {
			case "proxy":
				return nil, []byte("ERROR: Unable to download webpage: (caused by ProxyError('Unable to connect to proxy'))"), &Error{"youtube_ytdlp_failed"}
			case "timeout":
				return nil, nil, &Error{"youtube_ytdlp_timeout"}
			case "unavailable":
				return nil, []byte("ERROR: [youtube] Video unavailable"), &Error{"youtube_ytdlp_failed"}
			}
			return []byte(playlist), nil, nil
		}
		return y, &tried, done
	}

	y, tried, done := run(map[string]string{"first": "proxy"})
	if videos, err := y.ListUploads(context.Background(), "UCAAAAAAAAAAAAAAAAAAAAAA", 1); err != nil || len(videos) != 1 {
		t.Fatalf("videos=%+v err=%v", videos, err)
	}
	if strings.Join(*tried, ",") != "first,second" || done["first"] || !done["second"] {
		t.Fatalf("tried=%v done=%v", *tried, done)
	}

	y, tried, _ = run(map[string]string{"first": "timeout", "second": "timeout"})
	if _, err := y.ListUploads(context.Background(), "UCAAAAAAAAAAAAAAAAAAAAAA", 1); err == nil || err.Error() != "youtube_ytdlp_timeout" {
		t.Fatalf("err=%v", err)
	}
	if len(*tried) != 2 {
		t.Fatalf("tried=%v", *tried)
	}

	// A failure yt-dlp reached YouTube for is the answer.
	y, tried, done = run(map[string]string{"first": "unavailable"})
	if _, _, err := y.Inspect(context.Background(), "https://www.youtube.com/watch?v=CCCCCCCCCCC"); err == nil || err.Error() != "youtube_ytdlp_failed" {
		t.Fatalf("err=%v", err)
	}
	if strings.Join(*tried, ",") != "first" || !done["first"] {
		t.Fatalf("tried=%v done=%v", *tried, done)
	}
}

func TestYTDLPReadFailsClosedWithoutARoute(t *testing.T) {
	y := NewYTDLP("synthetic-yt-dlp")
	y.Routes = func(context.Context) ([]ProcessRoute, error) { return nil, &Error{"no route"} }
	y.runFunc = func(context.Context, []string, ...string) ([]byte, []byte, error) {
		t.Fatal("yt-dlp ran without a route")
		return nil, nil, nil
	}
	if _, err := y.ListUploads(context.Background(), "UCAAAAAAAAAAAAAAAAAAAAAA", 1); err == nil || err.Error() != "youtube_ytdlp_proxy_unavailable" {
		t.Fatalf("err=%v", err)
	}
}

func TestProxyVariablesAreRecognizedInAnyCase(t *testing.T) {
	for entry, want := range map[string]bool{"HTTPS_PROXY=x": true, "all_proxy=x": true, "No_Proxy=x": true, "PATH=x": false, "PROXY=x": false} {
		if proxyVariable(entry) != want {
			t.Fatalf("%s: want %v", entry, want)
		}
	}
}
