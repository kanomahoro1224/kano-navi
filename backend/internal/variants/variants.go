// Package variants makes the smaller copies the server answers w requests
// with, ahead of those requests, and reports how complete they are. One core
// serves the media step, the variants command and later callers: each names
// a group of originals and bounds the work, and requests never decode.
package variants

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// Group names a set of originals with copies.
type Group string

const (
	// Lineblog is the recovered LINE BLOG images in <data>/lineblog/.
	Lineblog Group = "lineblog"
	// History is the history images of the built site in
	// <static>/assets/history/.
	History Group = "history"
	// Media is the current files of ready X, YouTube and Bilibili assets.
	Media Group = "media"
)

// Groups lists every group in the order the command makes them.
var Groups = []Group{Lineblog, History, Media}

// LineblogName is an archive media name: 12 hex digits of a SHA-256 and the
// extension that fixes its type. Only .jpg and .png have copies.
var LineblogName = regexp.MustCompile(`^([0-9a-f]{12})\.(jpg|png|mp4)$`)

// HistoryImage matches a history image of the built site by its path
// relative to the site, the only static files with copies.
var HistoryImage = regexp.MustCompile(`^assets/history/[^/]+\.(?:jpe?g|png|webp)$`)

// maxSource is the largest original that has copies, the cached-media bound.
const maxSource = 10 * 1024 * 1024

// workers matches the variant cache's two source slots, so both are busy
// without a third original waiting in memory.
const workers = 2

// Roots locate the originals and the cache. Data holds lineblog/ and the
// remembered file hashes, Static the built site, and Out the variant cache
// that is written (Data when empty). Media needs a store; its cache is the
// store's data directory unless Out is set.
type Roots struct {
	Data, Static, Out string
}

func (r Roots) out() string {
	if r.Out != "" {
		return r.Out
	}
	return r.Data
}

// Bounds limit one Make call: at most Files originals are loaded to make
// copies and none is started once Budget has passed since the call began.
// Zero means no limit. Now reads the clock (time.Now when nil).
type Bounds struct {
	Files  int
	Budget time.Duration
	Now    func() time.Time
}

// Stop reasons recorded in Result.Stopped.
const (
	StoppedFileCap    = "variants_file_cap"
	StoppedTimeBudget = "variants_time_budget"
)

// Result counts one Make call in the shape of a step result. Attempted
// originals were loaded: Made got at least one copy, Kept only
// keep-original markers, and Failed could not be read, did not match their
// hash or could not be stored. Skipped originals were already done.
// Remaining were not examined because a bound stopped the call, which
// Stopped names; Failed originals are tried again by the next call.
type Result struct {
	Attempted int    `json:"attempted"`
	Made      int    `json:"made"`
	Kept      int    `json:"kept"`
	Skipped   int    `json:"skipped"`
	Failed    int    `json:"failed"`
	Remaining int    `json:"remaining"`
	Stopped   string `json:"stopped,omitempty"`
}

// original is one file of a group. hash is known for media and read for
// fixed files.
type original struct {
	path  string
	hash  string
	media sqlite.ReadyMedia
}

// Make makes the missing copies of group's originals, two at a time within
// the variant cache's source slots, newest media first. Done originals are
// skipped without being loaded, so a stopped call resumes where it left off.
// A fixed file is hashed by streaming it, and the hash is remembered under
// roots.Data. It returns early with ctx's error when ctx ends.
func Make(ctx context.Context, store *sqlite.Store, roots Roots, group Group, bounds Bounds) (Result, error) {
	originals, err := list(ctx, store, roots, group)
	if err != nil {
		return Result{}, err
	}
	out := roots.out()
	if group == Media && roots.Out == "" {
		out = store.DataRoot()
	}
	now := bounds.Now
	if now == nil {
		now = time.Now
	}
	deadline := now().Add(bounds.Budget)
	var mu sync.Mutex
	result := Result{}
	next := 0
	// claim hands out the next original, or reports why none is left.
	claim := func() (original, bool) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case next >= len(originals) || result.Stopped != "" || ctx.Err() != nil:
			return original{}, false
		case bounds.Budget > 0 && !now().Before(deadline):
			result.Stopped = StoppedTimeBudget
			return original{}, false
		}
		next++
		return originals[next-1], true
	}
	// reserve counts an attempt, or stops the call at the file cap and puts
	// the original back as remaining.
	reserve := func() bool {
		mu.Lock()
		defer mu.Unlock()
		if bounds.Files > 0 && result.Attempted >= bounds.Files {
			if result.Stopped == "" {
				result.Stopped = StoppedFileCap
			}
			result.Remaining++
			return false
		}
		result.Attempted++
		return true
	}
	count := func(apply func(*Result)) {
		mu.Lock()
		apply(&result)
		mu.Unlock()
	}
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for {
				item, ok := claim()
				if !ok {
					return
				}
				hash, load, err := source(ctx, store, roots, item)
				if err != nil {
					count(func(r *Result) {
						if ctx.Err() == nil {
							r.Failed++
						}
					})
					continue
				}
				if sqlite.VariantsDone(out, hash) {
					count(func(r *Result) { r.Skipped++ })
					continue
				}
				if !reserve() {
					continue
				}
				made, err := sqlite.MakeVariants(ctx, out, hash, load)
				count(func(r *Result) {
					switch {
					case err != nil && ctx.Err() != nil:
						r.Attempted--
						r.Remaining++
					case err != nil:
						r.Failed++
					case made.Copies > 0:
						r.Made++
					case made.Kept > 0:
						r.Kept++
					default:
						// Another maker finished the image first.
						r.Attempted--
						r.Skipped++
					}
				})
			}
		})
	}
	wg.Wait()
	result.Remaining += len(originals) - next
	return result, ctx.Err()
}

// source returns item's hash and the loader of its bytes. A fixed file is
// checked to be a regular file within the bound and hashed (or found
// remembered) under roots.Data.
func source(ctx context.Context, store *sqlite.Store, roots Roots, item original) (string, func() ([]byte, error), error) {
	if item.path == "" {
		return item.media.Hash, func() ([]byte, error) { return store.LoadReadyMedia(item.media) }, nil
	}
	file, err := os.Open(item.path)
	if err != nil {
		return "", nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxSource {
		return "", nil, errors.New("original unavailable")
	}
	hash, err := sqlite.FileHash(ctx, roots.Data, item.path, info, file)
	if err != nil {
		return "", nil, err
	}
	return hash, func() ([]byte, error) { return readBounded(item.path) }, nil
}

// readBounded reads at most maxSource bytes of path; a file that grew past
// the bound then fails the hash check instead of being read whole.
func readBounded(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, maxSource))
}

// list returns group's originals. Fixed files are listed by the absolute
// path the server keys their hashes by: the archive path as is, a history
// path with symlinks resolved, as the static site resolves it.
func list(ctx context.Context, store *sqlite.Store, roots Roots, group Group) ([]original, error) {
	switch group {
	case Lineblog:
		return listFixed(roots.Data, lineblogDir, func(name string) bool {
			match := LineblogName.FindStringSubmatch(name)
			return match != nil && match[2] != "mp4"
		}, false)
	case History:
		return listFixed(roots.Static, historyDir, func(name string) bool {
			return HistoryImage.MatchString(historyDir + "/" + name)
		}, true)
	case Media:
		if store == nil {
			return nil, errors.New("media copies need the store")
		}
		ready, err := store.ReadyMedia(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]original, len(ready))
		for i, item := range ready {
			out[i] = original{hash: item.Hash, media: item}
		}
		return out, nil
	}
	return nil, errors.New("unknown variant group")
}

const (
	lineblogDir = "lineblog"
	historyDir  = "assets/history"
)

// listFixed lists the regular files directly in root's dir whose names
// accept takes, skipping symlinks and dotfiles. An empty or missing root
// lists nothing.
func listFixed(root, dir string, accept func(name string) bool, resolve bool) ([]original, error) {
	if root == "" {
		return nil, nil
	}
	base, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(dir)))
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(base)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if resolve {
		if base, err = filepath.EvalSymlinks(base); err != nil {
			return nil, err
		}
	}
	out := []original{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type().IsRegular() && !strings.HasPrefix(name, ".") && accept(name) {
			out = append(out, original{path: filepath.Join(base, name)})
		}
	}
	return out, nil
}
