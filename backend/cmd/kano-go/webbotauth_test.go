package main

import (
	"bytes"
	"regexp"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/webbotauth"
)

func TestWebBotAuthKeyCommandPrintsAKeyAndItsKeyID(t *testing.T) {
	var out bytes.Buffer
	if code := runWebBotAuthKey(nil, &out); code != 0 {
		t.Fatalf("exit %d", code)
	}
	match := regexp.MustCompile(`^WEB_BOT_AUTH_KEY=([A-Za-z0-9_-]{43})\n# keyid \(RFC 7638 JWK thumbprint\): ([A-Za-z0-9_-]{43})\n$`).FindStringSubmatch(out.String())
	if match == nil {
		t.Fatalf("output=%q", out.String())
	}
	key, err := webbotauth.ParseKey(match[1])
	if err != nil || key.Thumbprint() != match[2] {
		t.Fatalf("key=%v err=%v", key, err)
	}
	if code := runWebBotAuthKey([]string{"-x"}, &out); code != 2 {
		t.Fatalf("argument exit %d", code)
	}
}
