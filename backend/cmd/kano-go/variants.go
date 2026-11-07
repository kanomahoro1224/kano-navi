package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/yexca/kano-navi/backend/internal/config"
	"github.com/yexca/kano-navi/backend/internal/variants"
)

// runVariants makes the smaller copies of the LINE BLOG archive images,
// discography covers and Journey images in -data, writing them to the
// variant cache of -out (default -data). Images whose copies are all there
// are skipped, so an interrupted run resumes.
func runVariants(args []string) int {
	flags := flag.NewFlagSet("kano-go variants", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	data := flags.String("data", filepath.Dir(filepath.Dir(config.DefaultDatabasePath)), "runtime data directory holding lineblog/, works/ and journey/")
	out := flags.String("out", "", "data directory to write cache/variants/ into (default -data)")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return fail("invalid arguments for variants", 2)
	}
	roots := variants.Roots{Data: *data, Out: *out}
	target := *out
	if target == "" {
		target = *data
	}
	// The cache is written below an existing directory only.
	if err := os.MkdirAll(target, 0o755); err != nil {
		return fail("output directory could not be created", 1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	for _, group := range []variants.Group{variants.Lineblog, variants.Works, variants.Journey} {
		result, err := variants.Make(ctx, nil, roots, group, variants.Bounds{})
		fmt.Printf("%s: %d made, %d kept whole, %d already done, %d failed, %d remaining\n", group, result.Made, result.Kept, result.Skipped, result.Failed, result.Remaining)
		if err != nil {
			return fail("stopped before every image was done; run again to resume", 1)
		}
	}
	fmt.Println("Copies are in " + filepath.Join(target, "cache", "variants"))
	return 0
}
