package shop

import (
	"context"
	"errors"
	"time"
)

// State is the read bookkeeping kept between runs. BlockedUntil is when a
// 429 answer's Retry-After passes; no read requests the shop before it.
type State struct {
	LastAttemptAt, LastSuccessAt, BlockedUntil time.Time
	LastError                                  string
}

// Options bound a read.
type Options struct {
	// RequestDelay spaces the requests of one read.
	RequestDelay time.Duration
	// Now reads the clock; nil is time.Now.
	Now func() time.Time
}

// DefaultRequestDelay is the pause between the requests of one read.
const DefaultRequestDelay = 3 * time.Second

type Adapter struct {
	Client  *Client
	Options Options
}

func NewAdapter(client *Client) *Adapter {
	return &Adapter{Client: client, Options: Options{RequestDelay: DefaultRequestDelay}}
}

// Chain methods of a timed read, by their stable IDs. Each lists the newest
// products once:
//
//   - MethodSigned requests the newest ArrivalsLimit products of the
//     new-arrivals collection, signed with Web Bot Auth; without a key it is
//     skipped with ErrNoSigningKey and makes no request.
//   - MethodArrivals requests the same listing unsigned.
//   - MethodAtom requests the catalog's Atom feed, the newest AtomLimit
//     products, signed when a key is configured.
const (
	MethodSigned   = "signed"
	MethodArrivals = "new_arrivals"
	MethodAtom     = "atom"
)

// DefaultMethods is the chain before the operator orders one.
var DefaultMethods = []string{MethodSigned, MethodArrivals, MethodAtom}

// MaxListingRequests bounds the listing requests of one timed read: the
// first method that can request and, after a failure of that method alone,
// one more. With the collection list a read makes at most three requests.
const MaxListingRequests = 2

// Skip reasons of a read that made no request.
const SkippedRateLimited = "rate_limited"

// ErrNoMethod is a timed read whose enabled methods could all make no
// request.
const ErrNoMethod = "shop_no_method"

// Result is what one read found. Sales are the period groups its products
// touched; they are empty when it failed.
type Result struct {
	Sales []Sale
	State State
	// Skipped is set when the read made no request.
	Skipped string
	// Requested counts requests, Listed the products the listing returned,
	// Candidates the tagged products with a dated period, and Outcomes why
	// the other products are not sale periods.
	Requested, Listed, Candidates int
	Outcomes                      map[string]int
	// Listing is the chain method that gave a timed read its products, or
	// "" when none did. Tried lists the methods the read reached, in order,
	// as "<method>" for the one that served and "<method>: <code>" for the
	// others. Fallback is set when a method that made a request failed
	// before Listing served.
	Listing  string
	Tried    []string
	Fallback bool
	// Errors are the codes of the failed requests, in order.
	Errors []string
	// Signed reports that the listing, or every catalog page, was signed.
	Signed bool
	// Pages are the catalog pages a full read requested, and ReachedEnd
	// reports that it read the last one.
	Pages       int
	ReachedEnd  bool
	Collections bool
}

func (a *Adapter) now() time.Time {
	if a.Options.Now != nil {
		return a.Options.Now()
	}
	return time.Now()
}

// endsRead reports a failure of the host rather than of the method: a 429,
// which the shop applies to the whole address, a timeout and an unreachable
// host. No other method is tried after one.
func endsRead(err error) bool {
	switch ErrorCode(err) {
	case "shop_rate_limited", "shop_timeout", "shop_unavailable":
		return true
	}
	return false
}

// Read lists the newest products through the chain methods in order (all
// false; nil methods is DefaultMethods), or the whole catalog, at most
// MaxCatalogPages pages signed when a key is configured, and groups Kano's
// sale periods.
//
// A timed read tries the next method only after a failure of the method
// itself (an HTTP status, a redirect, an oversized or unreadable answer),
// and makes at most MaxListingRequests listing requests; a failure of the
// host (endsRead) ends it. The collection list is read at most once, with
// the signing of the listing that served, and only when a product is in no
// stored sale of its period. A read whose listing or collection list fails
// returns no sales, and a 429 holds every read of the source back until its
// Retry-After passes. A read during that wait makes no request.
func (a *Adapter) Read(ctx context.Context, state State, stored []Sale, all bool, methods []string) (Result, error) {
	now := a.now()
	if state.BlockedUntil.After(now) {
		return Result{State: state, Skipped: SkippedRateLimited}, nil
	}
	if len(methods) == 0 {
		methods = DefaultMethods
	}
	state.LastAttemptAt = now
	out := Result{State: state, Outcomes: map[string]int{}}
	wait := func() error {
		if out.Requested == 0 || a.Options.RequestDelay <= 0 {
			return nil
		}
		timer := time.NewTimer(a.Options.RequestDelay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		}
	}
	fail := func(err error) (Result, error) {
		if ctx.Err() != nil {
			return Result{State: state, Requested: out.Requested}, ctx.Err()
		}
		out.State.LastError = ErrorCode(err)
		var shopErr *Error
		if errors.As(err, &shopErr) && shopErr.RetryAfter > 0 {
			out.State.BlockedUntil = a.now().Add(shopErr.RetryAfter)
		}
		out.Sales = nil
		return out, err
	}
	failed := func(err error) error {
		if ctx.Err() == nil {
			out.Errors = append(out.Errors, ErrorCode(err))
		}
		return err
	}

	products := []Product{}
	seen := map[int64]bool{}
	add := func(page []Product) {
		for _, p := range page {
			if !seen[p.ID] {
				seen[p.ID] = true
				products = append(products, p)
			}
		}
	}
	if all {
		out.Signed = a.Client.Signs()
		for page := 1; page <= MaxCatalogPages; page++ {
			if err := wait(); err != nil {
				return fail(err)
			}
			out.Requested++
			out.Pages++
			listed, size, err := a.Client.CatalogPage(ctx, page, out.Signed)
			if err != nil {
				return fail(failed(err))
			}
			add(listed)
			// The page's own size ends the catalog, so a product left out
			// as unusable does not cut a full page short.
			if size < CatalogPageSize {
				out.ReachedEnd = true
				break
			}
		}
	} else {
		listed, err := a.list(ctx, &out, methods, wait, failed)
		if err != nil {
			return fail(err)
		}
		add(listed)
	}
	out.Listed = len(products)
	candidates, outcomes := Candidates(products)
	out.Candidates, out.Outcomes = len(candidates), outcomes
	var collections []Collection
	if NeedsCollections(candidates, stored) {
		if err := wait(); err != nil {
			return fail(err)
		}
		out.Requested++
		out.Collections = true
		listed, err := a.Client.Collections(ctx, out.Signed)
		if err != nil {
			return fail(failed(err))
		}
		collections = listed
	}
	out.Sales = Group(candidates, stored, collections)
	out.State.LastSuccessAt, out.State.LastError, out.State.BlockedUntil = now, "", time.Time{}
	return out, nil
}

// list walks the chain for a timed read and returns the products of the
// first method that serves, or the failure that ended the walk.
func (a *Adapter) list(ctx context.Context, out *Result, methods []string, wait func() error, failed func(error) error) ([]Product, error) {
	var lastErr error
	for _, method := range methods {
		var sign bool
		switch method {
		case MethodSigned:
			if !a.Client.Signs() {
				out.Tried = append(out.Tried, method+": "+ErrNoSigningKey)
				continue
			}
			sign = true
		case MethodArrivals:
		case MethodAtom:
			sign = a.Client.Signs()
		default:
			continue
		}
		if out.Requested >= MaxListingRequests {
			break
		}
		if err := wait(); err != nil {
			return nil, err
		}
		out.Requested++
		var listed []Product
		var err error
		if method == MethodAtom {
			listed, err = a.Client.Atom(ctx, sign)
		} else {
			listed, err = a.Client.NewArrivals(ctx, ArrivalsLimit, sign)
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = failed(err)
			out.Tried = append(out.Tried, method+": "+ErrorCode(err))
			if endsRead(err) {
				break
			}
			continue
		}
		out.Tried = append(out.Tried, method)
		out.Listing, out.Signed, out.Fallback = method, sign, lastErr != nil
		return listed, nil
	}
	if lastErr == nil {
		lastErr = failed(&Error{Code: ErrNoMethod})
	}
	return nil, lastErr
}
