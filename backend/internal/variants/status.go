package variants

import (
	"context"
	"os"

	"github.com/yexca/kano-navi/backend/internal/mediavariant"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// WidthStatus counts one width of a group's originals: Copies stored, Kept
// marked to be served whole, and Missing with neither.
type WidthStatus struct {
	Width   int `json:"width"`
	Copies  int `json:"copies"`
	Kept    int `json:"kept"`
	Missing int `json:"missing"`
}

// GroupStatus describes how complete one group's copies are. Ready counts
// originals with every width stored, as a copy or a keep marker. Unhashed
// counts fixed files whose hash was never read, so their widths count as
// missing.
type GroupStatus struct {
	Group     Group         `json:"group"`
	Originals int           `json:"originals"`
	Ready     int           `json:"ready"`
	Unhashed  int           `json:"unhashed"`
	Widths    []WidthStatus `json:"widths"`
}

// Status reports groups (every group when none is named) without reading or
// hashing an original: fixed files are matched to their copies by the hash
// remembered for their path, size and modification time, media by their
// row's hash, and each variant cache's names are listed once, without the
// metadata of its files. Media needs a store.
func Status(ctx context.Context, store *sqlite.Store, roots Roots, groups ...Group) ([]GroupStatus, error) {
	if len(groups) == 0 {
		groups = Groups
	}
	caches := map[string]map[string]map[int]sqlite.VariantEntry{}
	out := []GroupStatus{}
	for _, group := range groups {
		originals, err := list(ctx, store, roots, group)
		if err != nil {
			return nil, err
		}
		root := roots.out()
		if group == Media && roots.Out == "" {
			root = store.DataRoot()
		}
		cache, ok := caches[root]
		if !ok {
			if cache, err = sqlite.ListVariants(root); err != nil {
				return nil, err
			}
			caches[root] = cache
		}
		status := GroupStatus{Group: group, Originals: len(originals), Widths: make([]WidthStatus, len(mediavariant.Widths))}
		for i, width := range mediavariant.Widths {
			status.Widths[i].Width = width
		}
		for _, item := range originals {
			hash := item.hash
			if item.path != "" {
				hash = ""
				if info, statErr := os.Stat(item.path); statErr == nil {
					hash, _ = sqlite.KnownFileHash(roots.Data, item.path, info)
				}
			}
			if hash == "" {
				status.Unhashed++
			}
			entries := cache[hash]
			ready := hash != ""
			for i, width := range mediavariant.Widths {
				entry, stored := entries[width]
				switch {
				case !stored:
					status.Widths[i].Missing++
					ready = false
				case entry.Kept:
					status.Widths[i].Kept++
				default:
					status.Widths[i].Copies++
				}
			}
			if ready {
				status.Ready++
			}
		}
		out = append(out, status)
	}
	return out, nil
}
