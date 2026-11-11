package main

import (
	"fmt"
	"io"

	"github.com/yexca/kano-navi/backend/internal/webbotauth"
)

// runWebBotAuthKey prints a new Web Bot Auth key as an environment line and
// its keyid as a comment, so the output can be added to .env as it is. It
// reads no environment and stores nothing.
func runWebBotAuthKey(args []string, out io.Writer) int {
	if len(args) != 0 {
		return fail("web-bot-auth-key takes no arguments", 2)
	}
	key, err := webbotauth.GenerateKey(nil)
	if err != nil {
		return fail("key could not be generated", 1)
	}
	fmt.Fprintf(out, "%s=%s\n# keyid (RFC 7638 JWK thumbprint): %s\n", webbotauth.EnvKey, key.Seed(), key.Thumbprint())
	return 0
}
