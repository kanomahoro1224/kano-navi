import assert from "node:assert/strict"
import test from "node:test"
import { emptyRuns } from "../src/dashboard/week-days.ts"

test("the week list folds each run of two or more empty days into its first", () => {
  const cases: [empty: string, runs: number[]][] = [
    // E is an empty day other than today, x a day with entries or today.
    ["xxxxxxx", [1, 1, 1, 1, 1, 1, 1]],
    ["ExxxxxE", [1, 1, 1, 1, 1, 1, 1]],
    ["EExEEEx", [2, 0, 1, 3, 0, 0, 1]],
    ["xEEEEEE", [1, 6, 0, 0, 0, 0, 0]],
    ["EEEEEEE", [7, 0, 0, 0, 0, 0, 0]],
  ]
  for (const [empty, runs] of cases) {
    assert.deepEqual(
      emptyRuns([...empty].map((day) => day === "E")),
      runs,
      empty,
    )
  }
})
