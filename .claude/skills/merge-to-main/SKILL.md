---
name: merge-to-main
description: Merge a kano-navi branch into main when the user asks. Chooses squash or merge commit and runs the checks on the result.
---

# Merge into Main

Run only when the user explicitly asks to merge. The rules are in
[Merging into Main](../../../docs/development/commit-and-release.md#merging-into-main).

1. Run `git status --short` and stop if unrelated user changes would be
   affected.
2. Read `git log main..<branch> --oneline` and judge whether each commit is a
   focused, conventional step that passes on its own and is worth reading,
   bisecting or reverting separately.
3. Squash (`git merge --squash`, then one conventional commit describing the
   whole branch) unless every commit passes that test; then use
   `git merge --no-ff` with a conventional subject. When unsure, squash. Do not
   rewrite the branch's commits unless the user asks.
4. Run the checks in
   [Before Committing](../../../docs/development/commit-and-release.md#before-committing)
   on the result, including `make sensitive-check`, and review the diff.
5. Keep the configured signing path. Push only when the user asks.
