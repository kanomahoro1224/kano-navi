package main

import (
	"fmt"
	"os"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// runWebPushKey prints a VAPID pair in the form .env expects. The private key
// is this sender's identity: it stays in the environment and never reaches the
// database or the console.
func runWebPushKey() int {
	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		fmt.Fprintf(os.Stderr, "kano-go: key generation failed: %v\n", err)
		return 1
	}
	fmt.Printf("WEB_PUSH_VAPID_PUBLIC_KEY=%s\n", publicKey)
	fmt.Printf("WEB_PUSH_VAPID_PRIVATE_KEY=%s\n", privateKey)
	return 0
}
