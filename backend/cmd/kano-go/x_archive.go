package main

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/yexca/kano-navi/backend/internal/config"
	"github.com/yexca/kano-navi/backend/internal/dotenv"
	xsource "github.com/yexca/kano-navi/backend/internal/source/x"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

const xArchiveUsage = `Usage: kano-go x-archive <import|check|apply> [flags]

  import  -dir <export directory> [-account <handle>] [-keep-unlisted] [-dry-run]
          Writes each <handle>.json export as the account's own posts, stores the
          photos of <handle>-media.zip, and hides an earlier import's posts the
          export does not list.
  check   -out <decisions.jsonl> [-capture <directory>] [-limit <n>] [-interval <d>] [-min-interval <d>] [-max-interval <d>]
          Reads each hidden unlisted post once from the X detail source and
          appends what it is; run it against a copy of the database. The
          earlier import's capture decides its marked reposts without a request.
  apply   -in <decisions.jsonl> [-dry-run]
          Records the decisions and shows again the own posts and thread
          replies that only the import hid.

Common flag: -database <path>`

// runXArchive runs one x-archive subcommand.
func runXArchive(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, xArchiveUsage)
		return 2
	}
	if err := dotenv.Load(".env"); err != nil {
		return fail("environment file could not be read", 2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	switch args[0] {
	case "import":
		return runXArchiveImport(ctx, args[1:])
	case "check":
		return runXArchiveCheck(ctx, args[1:])
	case "apply":
		return runXArchiveApply(ctx, args[1:])
	}
	fmt.Fprintln(os.Stderr, xArchiveUsage)
	return 2
}

type accountList []string

func (a *accountList) String() string { return strings.Join(*a, ",") }
func (a *accountList) Set(value string) error {
	*a = append(*a, strings.TrimPrefix(strings.TrimSpace(value), "@"))
	return nil
}

var exportName = regexp.MustCompile(`^([A-Za-z0-9_]{1,30})\.json$`)

func runXArchiveImport(ctx context.Context, args []string) int {
	flags := flag.NewFlagSet("kano-go x-archive import", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	database := flags.String("database", config.DefaultDatabasePath, "path to the SQLite snapshot")
	dir := flags.String("dir", "", "directory holding <handle>.json and <handle>-media.zip")
	keep := flags.Bool("keep-unlisted", false, "leave an earlier import's unlisted posts shown")
	dryRun := flags.Bool("dry-run", false, "count what the import would do and write nothing")
	var only accountList
	flags.Var(&only, "account", "import only this account (repeatable)")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *dir == "" {
		return fail("invalid arguments for x-archive import", 2)
	}
	accounts, err := exportAccounts(*dir, only)
	if err != nil {
		return fail(err.Error(), 2)
	}
	synchronized := xsource.OptionsFromEnv().Handles
	imports := make([]sqlite.XArchiveImport, 0, len(accounts))
	for _, account := range accounts {
		path := filepath.Join(*dir, account+".json")
		info, statErr := os.Stat(path)
		file, openErr := os.Open(path)
		if statErr != nil || openErr != nil {
			return fail("export "+account+".json could not be read", 1)
		}
		posts, stats, parseErr := xsource.ParseArchive(file, account, info.ModTime().UTC().Format(time.RFC3339))
		file.Close()
		if parseErr != nil {
			return fail("export "+account+".json is not an account export", 1)
		}
		fmt.Printf("%s: %d records, %d posts, %d replies, %d reposts, %d of other accounts, %d unreadable\n",
			account, stats.Records, stats.Posts, stats.Replies, stats.Reposts, stats.OtherAccount, stats.Invalid)
		imports = append(imports, sqlite.XArchiveImport{
			Account:      account,
			Posts:        posts,
			Synchronized: slices.ContainsFunc(synchronized, func(h string) bool { return strings.EqualFold(h, account) }),
			HideUnlisted: !*keep,
		})
	}
	store, err := sqlite.OpenStandalone(*database)
	if err != nil {
		return fail("database unavailable", 1)
	}
	defer store.Close()
	results, err := store.ImportXArchives(ctx, imports, *dryRun)
	if err != nil {
		return fail("export could not be imported", 1)
	}
	for i, r := range results {
		fmt.Printf("%s: %d added, %d rewritten (%d moved from another account), %d kept as synchronized, %d skipped; hidden %d replies and %d unlisted for review, %d already hidden",
			imports[i].Account, r.Added, r.Rewritten, r.Reassigned, r.KeptSynchronized, r.Skipped, r.HiddenReply, r.HiddenUnlisted, r.AlreadyHidden)
		if r.ArchiveAccountAdded {
			fmt.Print("; added as an archive-only account")
		}
		fmt.Println()
	}
	for _, in := range imports {
		saved, matched, err := importArchivePhotos(ctx, store, *dir, in, *dryRun)
		if err != nil {
			return fail("photos of "+in.Account+" could not be stored", 1)
		}
		if *dryRun {
			fmt.Printf("%s: %d photos in the media archive\n", in.Account, matched)
		} else {
			fmt.Printf("%s: %d of %d photos stored from the media archive\n", in.Account, saved, matched)
		}
	}
	if *dryRun {
		fmt.Println("Dry run: nothing was written.")
	}
	return 0
}

// exportAccounts lists the directory's account exports, or checks the
// requested ones exist.
func exportAccounts(dir string, only []string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.New("export directory could not be read")
	}
	found := []string{}
	for _, entry := range entries {
		if m := exportName.FindStringSubmatch(entry.Name()); m != nil && entry.Type().IsRegular() {
			found = append(found, m[1])
		}
	}
	if len(only) == 0 {
		if len(found) == 0 {
			return nil, errors.New("no <handle>.json export in the directory")
		}
		return found, nil
	}
	for _, account := range only {
		if !slices.Contains(found, account) {
			return nil, errors.New("no export for " + account)
		}
	}
	return only, nil
}

// importArchivePhotos stores the media archive's photos of the account's
// posts. A missing archive stores none; the media step downloads them.
func importArchivePhotos(ctx context.Context, store *sqlite.Store, dir string, in sqlite.XArchiveImport, dryRun bool) (saved, matched int, err error) {
	archive, openErr := zip.OpenReader(filepath.Join(dir, in.Account+"-media.zip"))
	if openErr != nil {
		return 0, 0, nil
	}
	defer archive.Close()
	byID := make(map[string]*xsource.Post, len(in.Posts))
	for i := range in.Posts {
		byID[in.Posts[i].ID] = &in.Posts[i]
	}
	for _, file := range archive.File {
		if ctx.Err() != nil {
			return saved, matched, ctx.Err()
		}
		id, position, ok := xsource.ArchiveMediaFile(filepath.Base(file.Name))
		post := byID[id]
		if !ok || post == nil || position >= len(post.MediaURLs) || (position < len(post.MediaKinds) && post.MediaKinds[position] != xsource.MediaPhoto) {
			continue
		}
		matched++
		if dryRun || file.UncompressedSize64 > 10<<20 {
			continue
		}
		reader, e := file.Open()
		if e != nil {
			continue
		}
		content, e := io.ReadAll(io.LimitReader(reader, 10<<20+1))
		reader.Close()
		if e != nil {
			continue
		}
		stored, e := store.SaveXArchiveImage(ctx, post.MediaURLs[position], content)
		if e != nil {
			return saved, matched, e
		}
		if stored {
			saved++
		}
	}
	return saved, matched, nil
}

// pace spaces detail requests: a random spread around the interval, which
// shortens slowly after a run of answers and doubles after a refusal or
// failure, honouring a requested wait.
type pace struct {
	interval, min, max time.Duration
	streak             int
}

const paceStreak = 25

func (p *pace) next() time.Duration {
	spread := 0.7 + 0.6*rand.Float64()
	return time.Duration(float64(p.interval) * spread)
}

func (p *pace) success() {
	p.streak++
	if p.streak >= paceStreak {
		p.streak = 0
		p.interval = max(p.min, p.interval*9/10)
	}
}

func (p *pace) backoff(wait time.Duration) time.Duration {
	p.streak = 0
	p.interval = min(p.max, max(p.interval*2, p.min))
	return max(wait, p.interval)
}

func runXArchiveCheck(ctx context.Context, args []string) int {
	flags := flag.NewFlagSet("kano-go x-archive check", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	database := flags.String("database", config.DefaultDatabasePath, "path to a copy of the SQLite snapshot")
	outPath := flags.String("out", "", "decisions file to append to")
	limit := flags.Int("limit", 0, "read at most this many statuses (0 reads all)")
	interval := flags.Duration("interval", 6*time.Second, "starting time between requests")
	minInterval := flags.Duration("min-interval", 4*time.Second, "shortest time between requests")
	maxInterval := flags.Duration("max-interval", 10*time.Minute, "longest wait after failures")
	capture := flags.String("capture", "", "the earlier import's capture directory, whose reposts need no request")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *outPath == "" || *limit < 0 ||
		*minInterval < time.Second || *interval < *minInterval || *maxInterval < *interval {
		return fail("invalid arguments for x-archive check", 2)
	}
	decided, err := readDecisions(*outPath)
	if err != nil {
		return fail("decisions file could not be read", 1)
	}
	store, err := sqlite.OpenStandalone(*database)
	if err != nil {
		return fail("database unavailable", 1)
	}
	candidates, err := store.XReconcileCandidates(ctx)
	control, controlErr := store.XArchiveControl(ctx)
	store.Close()
	if err != nil || controlErr != nil {
		return fail("hidden posts could not be listed", 1)
	}
	out, err := os.OpenFile(*outPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fail("decisions file could not be opened", 1)
	}
	defer out.Close()
	// The earlier import's capture marks the other authors' statuses it
	// wrote as reposts; those are decided without a request.
	reposted := map[string]bool{}
	if *capture != "" {
		if reposted, err = captureReposts(*capture); err != nil {
			return fail("capture could not be read", 1)
		}
	}
	queue := []sqlite.XReconcileCandidate{}
	settled := []sqlite.XReconcileOutcome{}
	for _, c := range candidates {
		if _, done := decided[c.ID]; done {
			continue
		}
		if reposted[c.ID] {
			settled = append(settled, sqlite.XReconcileOutcome{ID: c.ID, Account: c.Account, Outcome: xsource.StatusForeign, CheckedAt: time.Now().UTC().Format(time.RFC3339)})
			continue
		}
		queue = append(queue, c)
	}
	if err = writeDecisions(out, settled); err != nil {
		return fail("decisions file could not be written", 1)
	}
	if len(settled) > 0 {
		fmt.Printf("%d hidden posts are reposts in the capture\n", len(settled))
	}
	waiting := len(queue)
	if *limit > 0 && len(queue) > *limit {
		queue = queue[:*limit]
	}
	fmt.Printf("%d hidden posts to read of %d undecided, %d already decided\n", len(queue), waiting, len(candidates)-waiting-len(settled))
	client := xsource.NewClient(nil)
	p := &pace{interval: *interval, min: *minInterval, max: *maxInterval}
	counts := map[string]int{}
	// A status that keeps failing is left for the next run; failures across
	// statuses in a row mean the source itself is refusing, which stops it.
	failures, attempts, skipped := 0, 0, 0
	gone := []sqlite.XReconcileOutcome{}
	for i := 0; i < len(queue); {
		c := queue[i]
		body, requestErr := client.Detail(ctx, c.Account, c.ID)
		if ctx.Err() != nil {
			break
		}
		outcome, classifyErr := xsource.ClassifyDetail(body, requestErr, c.Account, c.ID)
		wait := p.next()
		if classifyErr != nil {
			failures++
			if failures >= 8 {
				fmt.Printf("stopped after %d failures in a row; run again to resume\n", failures)
				break
			}
			wait = p.backoff(retryAfter(classifyErr))
			fmt.Printf("%s: %s, waiting %s\n", c.ID, failureCode(classifyErr), wait.Round(time.Second))
			if attempts++; attempts >= 3 {
				attempts, skipped = 0, skipped+1
				i++
			}
		} else {
			failures, attempts = 0, 0
			p.success()
			decision := sqlite.XReconcileOutcome{ID: c.ID, Account: c.Account, Outcome: outcome, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
			// The page vxtwitter shows a deleted status also stands in for
			// its own failures, so gone answers wait for a readable one: a
			// long run of them stops the check and keeps them undecided.
			if outcome == xsource.StatusGone {
				gone = append(gone, decision)
				if len(gone) >= goneLimit {
					// A post known to exist tells whether the source is
					// failing or the statuses are really gone.
					if !sourceReads(ctx, client, control) {
						fmt.Printf("stopped after %d unreadable statuses in a row; the source may be failing, run again later\n", len(gone))
						gone = nil
						break
					}
					if err = writeDecisions(out, gone); err != nil {
						return fail("decisions file could not be written", 1)
					}
					counts[xsource.StatusGone] += len(gone)
					gone = gone[:0]
				}
			} else {
				if err = writeDecisions(out, append(gone, decision)); err != nil {
					return fail("decisions file could not be written", 1)
				}
				for range gone {
					counts[xsource.StatusGone]++
				}
				gone = gone[:0]
				counts[outcome]++
			}
			i++
			if i%50 == 0 {
				fmt.Printf("%d/%d read %v\n", i, len(queue), counts)
			}
		}
		if i < len(queue) {
			select {
			case <-ctx.Done():
			case <-time.After(wait):
			}
		}
		if ctx.Err() != nil {
			break
		}
	}
	// Gone answers at the end of the queue, after the last readable one,
	// are kept only when the run finished the queue.
	if len(gone) > 0 && ctx.Err() == nil && failures < 8 {
		if err = writeDecisions(out, gone); err != nil {
			return fail("decisions file could not be written", 1)
		}
		counts[xsource.StatusGone] += len(gone)
	}
	fmt.Printf("read %v, %d left for the next run after failing\n", counts, skipped)
	if ctx.Err() != nil || failures >= 8 {
		return 1
	}
	return 0
}

// captureReposts reads the third-party capture's archive pages
// (responses/posts/**/*.json, each {"items": [...]}) and lists the posts it
// marks as reposts: the reposted status written under its own ID.
func captureReposts(dir string) (map[string]bool, error) {
	out := map[string]bool{}
	err := filepath.WalkDir(filepath.Join(dir, "responses", "posts"), func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || filepath.Ext(path) != ".json" {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		var page struct {
			Items []struct {
				ID     string `json:"id"`
				Repost *struct {
					ID string `json:"id"`
				} `json:"repost"`
			} `json:"items"`
		}
		if err = json.NewDecoder(io.LimitReader(file, 64<<20)).Decode(&page); err != nil {
			return err
		}
		for _, item := range page.Items {
			if item.Repost != nil && item.ID != "" {
				out[item.ID] = true
			}
		}
		return nil
	})
	return out, err
}

// sourceReads reports whether the detail source reads a post known to
// exist; without one it cannot tell, and reports false.
func sourceReads(ctx context.Context, client *xsource.Client, control sqlite.XReconcileCandidate) bool {
	if control.ID == "" {
		return false
	}
	body, err := client.Detail(ctx, control.Account, control.ID)
	outcome, err := xsource.ClassifyDetail(body, err, control.Account, control.ID)
	return err == nil && outcome != xsource.StatusGone
}

// goneLimit is how many unreadable statuses in a row make a check read a
// post known to exist before it goes on.
const goneLimit = 30

func writeDecisions(out io.Writer, decisions []sqlite.XReconcileOutcome) error {
	for _, decision := range decisions {
		line, _ := json.Marshal(decision)
		if _, err := out.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// retryAfter is the wait a refusal asks for; the source's controlled error
// carries none, so a rate limit waits a minute at least.
func retryAfter(err error) time.Duration {
	var e *xsource.Error
	if errors.As(err, &e) && e.Code == xsource.ErrRateLimited {
		return time.Minute
	}
	return 0
}

func failureCode(err error) string {
	var e *xsource.Error
	if errors.As(err, &e) {
		if e.Status != 0 {
			return string(e.Code) + " " + strconv.Itoa(e.Status)
		}
		return string(e.Code)
	}
	return err.Error()
}

// readDecisions reads a decisions file into its latest outcome per post; a
// missing file has none.
func readDecisions(path string) (map[string]sqlite.XReconcileOutcome, error) {
	out := map[string]sqlite.XReconcileOutcome{}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var decision sqlite.XReconcileOutcome
		if json.Unmarshal(scanner.Bytes(), &decision) == nil && decision.ID != "" && decision.Outcome != "" {
			out[decision.ID] = decision
		}
	}
	return out, scanner.Err()
}

func runXArchiveApply(ctx context.Context, args []string) int {
	flags := flag.NewFlagSet("kano-go x-archive apply", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	database := flags.String("database", config.DefaultDatabasePath, "path to the SQLite snapshot")
	inPath := flags.String("in", "", "decisions file written by check")
	dryRun := flags.Bool("dry-run", false, "count what applying would do and write nothing")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *inPath == "" {
		return fail("invalid arguments for x-archive apply", 2)
	}
	decided, err := readDecisions(*inPath)
	if err != nil {
		return fail("decisions file could not be read", 1)
	}
	outcomes := make([]sqlite.XReconcileOutcome, 0, len(decided))
	counts := map[string]int{}
	for _, decision := range decided {
		outcomes = append(outcomes, decision)
		counts[decision.Outcome]++
	}
	slices.SortFunc(outcomes, func(a, b sqlite.XReconcileOutcome) int { return strings.Compare(a.ID, b.ID) })
	store, err := sqlite.OpenStandalone(*database)
	if err != nil {
		return fail("database unavailable", 1)
	}
	defer store.Close()
	applied, err := store.ApplyXReconcile(ctx, outcomes, *dryRun)
	if err != nil {
		return fail("decisions could not be applied", 1)
	}
	fmt.Printf("decisions %v\n", counts)
	fmt.Printf("%d shown again, %d kept hidden, %d without an import hide to settle\n", applied.Shown, applied.Kept, applied.Unknown)
	if *dryRun {
		fmt.Println("Dry run: nothing was written.")
	}
	return 0
}
