package shop

import (
	"cmp"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// ProductRef is what a sale keeps of one of its products: no price, stock,
// image or description beyond the period line.
type ProductRef struct {
	ID          int64  `json:"id"`
	Handle      string `json:"handle"`
	Title       string `json:"title"`
	PublishedAt string `json:"publishedAt"`
}

// Sale is one period group: the products of one collection or title prefix
// that share a period. Key (the normalized group title) and the period's key
// are its identity; the rest is its evidence, kept as raw_json.
type Sale struct {
	Key        string       `json:"-"`
	Period     Period       `json:"period"`
	Title      string       `json:"title"`
	URL        string       `json:"url"`
	Collection string       `json:"collection,omitempty"`
	Products   []ProductRef `json:"products"`
}

// maxSaleProducts bounds the products one sale keeps.
const maxSaleProducts = 100

// minCollectionTitle is the shortest collection title (in runes, without
// whitespace) that may name a group, so a generic title never swallows one.
const minCollectionTitle = 4

// StartsAt is the opening instant: the earliest publication of the group's
// products that falls on the opening's Japan date, to the minute.
func (s Sale) StartsAt() (time.Time, bool) {
	var first time.Time
	for _, p := range s.Products {
		at, err := time.Parse(time.RFC3339, p.PublishedAt)
		if err != nil || at.In(japan).Format(time.DateOnly) != s.Period.Opens {
			continue
		}
		if first.IsZero() || at.Before(first) {
			first = at
		}
	}
	return first.Truncate(time.Minute), !first.IsZero()
}

// CollectionURL and ProductURL are the shop pages a sale links.
func CollectionURL(handle string) string {
	return "https://" + Host + "/collections/" + url.PathEscape(handle)
}

func ProductURL(handle string) string {
	return "https://" + Host + "/products/" + url.PathEscape(handle)
}

type groupID struct{ key, period string }

func (s Sale) id() groupID { return groupID{s.Key, s.Period.Key()} }

type productPeriod struct {
	id     int64
	period string
}

// NeedsCollections reports a candidate that no stored sale of its period
// holds yet, which is when the collection list may name its group.
func NeedsCollections(candidates []Candidate, stored []Sale) bool {
	known := knownProducts(stored)
	for _, c := range candidates {
		if _, ok := known[productPeriod{c.ID, c.Period.Key()}]; !ok {
			return true
		}
	}
	return false
}

func knownProducts(stored []Sale) map[productPeriod]groupID {
	known := map[productPeriod]groupID{}
	for _, s := range stored {
		for _, p := range s.Products {
			known[productPeriod{p.ID, s.Period.Key()}] = s.id()
		}
	}
	return known
}

// Group turns candidates into period groups, one per collection or title
// prefix and period:
//
//  1. A product a stored sale of the same period holds stays in it, so a
//     group keeps its identity however the listing window cuts it.
//  2. A new product joins the collection whose title (ignoring whitespace) its
//     title starts with, the longest one; the group links the collection.
//  3. Any other joins the group of its period whose title its own starts
//     with, the longest one.
//  4. Any other starts a group titled by its title without the last word,
//     linking its first product.
//
// It returns the groups the candidates touched, each with the products its
// stored sale held as well.
func Group(candidates []Candidate, stored []Sale, collections []Collection) []Sale {
	sorted := slices.Clone(candidates)
	slices.SortStableFunc(sorted, func(a, b Candidate) int {
		return cmp.Or(a.PublishedAt.Compare(b.PublishedAt), cmp.Compare(a.ID, b.ID))
	})
	saved := map[groupID]Sale{}
	for _, s := range stored {
		saved[s.id()] = s
	}
	known := knownProducts(stored)
	groups := map[groupID]*Sale{}
	order := []groupID{}
	added := map[groupID][]Candidate{}
	created := map[groupID]bool{}
	open := func(id groupID, fresh Sale) *Sale {
		if g, ok := groups[id]; ok {
			return g
		}
		g := fresh
		if s, ok := saved[id]; ok {
			g = s
			g.Products = slices.Clone(s.Products)
		} else {
			created[id] = true
		}
		groups[id] = &g
		order = append(order, id)
		return &g
	}
	join := func(id groupID, c Candidate) { added[id] = append(added[id], c) }

	rest := []Candidate{}
	for _, c := range sorted {
		if id, ok := known[productPeriod{c.ID, c.Period.Key()}]; ok {
			open(id, saved[id])
			join(id, c)
			continue
		}
		rest = append(rest, c)
	}
	others := []Candidate{}
	for _, c := range rest {
		collection, ok := matchCollection(c.Title, collections)
		if !ok {
			others = append(others, c)
			continue
		}
		id := groupID{normalize(collection.Title), c.Period.Key()}
		g := open(id, Sale{Key: id.key, Period: c.Period, Title: collection.Title, URL: CollectionURL(collection.Handle), Collection: collection.Handle})
		if g.Collection == "" {
			// A group a title prefix started links its collection once one
			// is known.
			g.Title, g.URL, g.Collection = collection.Title, CollectionURL(collection.Handle), collection.Handle
		}
		join(id, c)
	}
	// Shorter stems first, so a longer title joins the group of its prefix.
	slices.SortStableFunc(others, func(a, b Candidate) int {
		return cmp.Compare(utf8.RuneCountInString(normalize(stem(a.Title))), utf8.RuneCountInString(normalize(stem(b.Title))))
	})
	for _, c := range others {
		title, period := normalize(c.Title), c.Period.Key()
		best := groupID{}
		for _, id := range append(savedIDs(saved), order...) {
			if id.period == period && id.key != "" && strings.HasPrefix(title, id.key) && len(id.key) > len(best.key) {
				best = id
			}
		}
		if best.key != "" {
			open(best, saved[best])
			join(best, c)
			continue
		}
		head := stem(c.Title)
		id := groupID{normalize(head), period}
		open(id, Sale{Key: id.key, Period: c.Period, Title: head, URL: ProductURL(c.Handle)})
		join(id, c)
	}

	out := make([]Sale, 0, len(order))
	for _, id := range order {
		g := *groups[id]
		for _, c := range added[id] {
			ref := ProductRef{ID: c.ID, Handle: c.Handle, Title: c.Title, PublishedAt: c.PublishedAt.UTC().Format(time.RFC3339)}
			if i := slices.IndexFunc(g.Products, func(p ProductRef) bool { return p.ID == c.ID }); i >= 0 {
				g.Products[i] = ref
			} else {
				g.Products = append(g.Products, ref)
			}
		}
		slices.SortStableFunc(g.Products, func(a, b ProductRef) int {
			return cmp.Or(cmp.Compare(a.PublishedAt, b.PublishedAt), cmp.Compare(a.ID, b.ID))
		})
		g.Products = g.Products[:min(len(g.Products), maxSaleProducts)]
		// The period line is the newest one read; a group that a title
		// prefix started links its first product.
		g.Period = added[id][len(added[id])-1].Period
		if created[id] && g.Collection == "" && len(g.Products) > 0 {
			g.URL = ProductURL(g.Products[0].Handle)
		}
		out = append(out, g)
	}
	return out
}

// savedIDs lists the stored groups in a fixed order.
func savedIDs(m map[groupID]Sale) []groupID {
	keys := make([]groupID, 0, len(m))
	for id := range m {
		keys = append(keys, id)
	}
	slices.SortFunc(keys, func(a, b groupID) int { return cmp.Or(cmp.Compare(a.key, b.key), cmp.Compare(a.period, b.period)) })
	return keys
}

// matchCollection is the collection with the longest title that the product
// title starts with, ignoring whitespace.
func matchCollection(title string, collections []Collection) (Collection, bool) {
	title = normalize(title)
	best, found := Collection{}, false
	for _, c := range collections {
		key := normalize(c.Title)
		if utf8.RuneCountInString(key) < minCollectionTitle || !strings.HasPrefix(title, key) {
			continue
		}
		if !found || len(key) > len(normalize(best.Title)) || len(key) == len(normalize(best.Title)) && c.Handle < best.Handle {
			best, found = c, true
		}
	}
	return best, found
}

// stem is a title without its last word, or the whole title when it has one
// word.
func stem(title string) string {
	words := strings.Fields(title)
	if len(words) < 2 {
		return strings.Join(words, " ")
	}
	return strings.Join(words[:len(words)-1], " ")
}
