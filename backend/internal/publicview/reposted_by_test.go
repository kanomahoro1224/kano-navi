package publicview

import (
	"encoding/json"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// A promoted repost names the own accounts that reposted it in their order,
// as handles without "@"; every other post has an empty list, never null.
func TestPublicPostNamesItsReposters(t *testing.T) {
	reposted := publicPost(sqlite.Post{ID: "1900000000000000350", Source: "x", Promoted: true,
		RepostedBy: []string{"_SyntheticB", "kano_2525", "@bad handle"}})
	body, _ := json.Marshal(reposted["repostedBy"])
	if string(body) != `["_SyntheticB","kano_2525"]` {
		t.Fatalf("repostedBy=%s", body)
	}
	for _, post := range []sqlite.Post{{ID: "1900000000000000351", Source: "x"}, {ID: "1900000000000000352", Source: "x", RepostedBy: []string{}}} {
		body, _ = json.Marshal(publicPost(post)["repostedBy"])
		if string(body) != `[]` {
			t.Fatalf("%s repostedBy=%s", post.ID, body)
		}
	}
}
