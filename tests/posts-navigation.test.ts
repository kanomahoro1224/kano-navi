import assert from "node:assert/strict"
import test from "node:test"
import { postsHref, postsView } from "../src/posts/navigation.ts"

test("the post archive's address names its month and account", () => {
  assert.deepEqual(postsView("?month=2018-04&account=kano_2525"), {
    month: "2018-04",
    account: "kano_2525",
  })
  assert.equal(
    postsHref({ month: "2018-04", account: "kano_2525" }),
    "/posts?month=2018-04&account=kano_2525",
  )
  assert.equal(postsHref({ account: "10001" }), "/posts?account=10001")
  assert.equal(
    postsHref({ account: "kano_2525", kind: "reply" }),
    "/posts?account=kano_2525&kind=reply",
  )
  assert.equal(postsHref({ kind: "all" }), "/posts")
  assert.deepEqual(postsView("?account=_Kanotic&kind=repost"), {
    month: null,
    account: "_Kanotic",
    kind: "repost",
  })
  assert.equal(postsView("?kind=unknown").kind, undefined)
  assert.equal(postsHref(), "/posts")
})

test("a malformed month or account shows the default view", () => {
  for (const search of [
    "?month=2018-13",
    "?month=18-04",
    "?account=kano%25",
    "?account=" + "a".repeat(31),
    "",
  ]) {
    const view = postsView(search)
    assert.equal(view.month === null || view.month === "2018-04", true)
    assert.ok(view.account === null, search)
  }
  assert.equal(postsView("?month=2018-13").month, null)
})
