import assert from "node:assert/strict"
import { spawnSync } from "node:child_process"
import fs from "node:fs"
import os from "node:os"
import path from "node:path"
import test, { type TestContext } from "node:test"

import {
  checkPatterns,
  countLines,
  formatPatternConfig,
  isRequestScope,
  parsePatternConfig,
  patternBaselineFile,
  requestLines,
  runPatternCheck,
  tightenBaseline,
  type PatternConfig,
  type Sources,
} from "./check-patterns.ts"

const lines = (count: number) => "const value = 1\n".repeat(count)

function config(overrides: Partial<PatternConfig["fileSize"]> = {}) {
  return {
    version: 1,
    requests: {
      allowed: [{ file: "src/owner.ts", reason: "Fixture request owner." }],
    },
    fileSize: {
      limit: 10,
      slack: 2,
      exempt: [{ file: "src/messages.ts", reason: "Fixture dictionary." }],
      baseline: { "src/large.ts": 15 },
      ...overrides,
    },
  } satisfies PatternConfig
}

function messages(sources: Sources, value: PatternConfig = config()) {
  return checkPatterns(value, sources).findings.map(
    (finding) => `${finding.file}:${finding.line ?? "-"}`,
  )
}

test("counts lines as an editor numbers them", () => {
  assert.equal(countLines(""), 0)
  assert.equal(countLines("one"), 1)
  assert.equal(countLines("one\n"), 1)
  assert.equal(countLines("one\r\ntwo\r\n"), 2)
  assert.equal(countLines("one\n\n"), 2)
})

test("finds global fetch calls but not methods, names or comments", () => {
  const text = [
    "await fetch(url)",
    "await window.fetch(url)",
    "const send = globalThis.fetch",
    "refetch()",
    "client.fetch(url)",
    "// fetch(url) happens in the owner",
    " * fetch(url)",
    "const prefetch = (url) => url",
  ].join("\n")
  assert.deepEqual(requestLines(text), [1, 2, 3])
})

test("limits request ownership to frontend sources outside tests", () => {
  assert.equal(isRequestScope("src/feature/use-data.ts"), true)
  assert.equal(isRequestScope("src/feature/view.tsx"), true)
  assert.equal(isRequestScope("src/feature/view.test.tsx"), false)
  assert.equal(isRequestScope("src/__tests__/view.ts"), false)
  assert.equal(isRequestScope("tests/requests.test.ts"), false)
  assert.equal(isRequestScope("scripts/smoke.ts"), false)
  assert.equal(isRequestScope("src/styles/app.css"), false)
})

test("allows fetch only in listed owners", () => {
  const sources: Sources = new Map([
    ["src/owner.ts", "await fetch('/api/fixture')\n"],
    ["src/view.tsx", "const a = 1\nawait fetch('/api/fixture')\n"],
    ["src/view.test.tsx", "await fetch('/api/fixture')\n"],
    ["tests/view.test.ts", "await fetch('/api/fixture')\n"],
  ])
  const report = checkPatterns(config(), sources)
  assert.deepEqual(messages(sources), ["src/view.tsx:2"])
  assert.match(
    report.findings[0].message,
    /docs\/architecture\/frontend\/request-ownership\.md/,
  )
  assert.equal(report.requestSites, 2)
})

test("reports a listed owner without requests and drops it on update", () => {
  const sources: Sources = new Map([["src/owner.ts", "export {}\n"]])
  const report = checkPatterns(config(), sources)
  assert.deepEqual(report.findings, [])
  assert.match(report.notices.join("\n"), /src\/owner\.ts .*calls no fetch/)
  assert.deepEqual(tightenBaseline(config(), sources).requests.allowed, [])
})

test("fails a new file over the limit and accepts exempt ones", () => {
  const sources: Sources = new Map([
    ["src/new.ts", lines(11)],
    ["src/limit.ts", lines(10)],
    ["src/messages.ts", lines(40)],
    ["docs/long.md", lines(40)],
  ])
  const report = checkPatterns(config(), sources)
  assert.deepEqual(messages(sources), ["src/new.ts:-"])
  assert.match(report.findings[0].message, /over the 10-line limit/)
  assert.equal(report.codeFiles, 3)
})

test("lets a baselined file grow within the slack and fails past it", () => {
  const within: Sources = new Map([["src/large.ts", lines(17)]])
  assert.deepEqual(messages(within), [])
  const past: Sources = new Map([["src/large.ts", lines(18)]])
  const report = checkPatterns(config(), past)
  assert.deepEqual(messages(past), ["src/large.ts:-"])
  assert.match(report.findings[0].message, /baseline of 15 plus 2/)
  assert.deepEqual(messages(past, config({ slack: 3 })), [])
})

test("tightening lowers counts, drops small or missing files and never raises", () => {
  const value = config({
    baseline: { "src/large.ts": 15, "src/small.ts": 14, "src/gone.ts": 30 },
  })
  const shrunk: Sources = new Map([
    ["src/large.ts", lines(12)],
    ["src/small.ts", lines(9)],
  ])
  const notices = checkPatterns(value, shrunk).notices.join("\n")
  assert.match(notices, /src\/large\.ts shrank from 15 to 12/)
  assert.match(notices, /src\/small\.ts is down to 9 lines/)
  assert.match(notices, /src\/gone\.ts is baselined but not in the repository/)
  assert.deepEqual(tightenBaseline(value, shrunk).fileSize.baseline, {
    "src/large.ts": 12,
  })

  const grown: Sources = new Map([["src/large.ts", lines(25)]])
  assert.deepEqual(
    tightenBaseline(config(), grown).fileSize.baseline,
    config().fileSize.baseline,
  )
})

test("rejects malformed baseline documents", () => {
  const document = (value: unknown) => JSON.stringify(value)
  assert.doesNotThrow(() => parsePatternConfig(document(config())))
  assert.throws(
    () => parsePatternConfig(document({ ...config(), extra: true })),
    /root must contain only/,
  )
  assert.throws(
    () => parsePatternConfig(document(config({ baseline: { "src/a.ts": 9 } }))),
    /over the 10-line limit/,
  )
  assert.throws(
    () =>
      parsePatternConfig(
        document(config({ baseline: { "src/messages.ts": 20 } })),
      ),
    /also exempt/,
  )
  assert.throws(
    () =>
      parsePatternConfig(
        document(config({ exempt: [{ file: "src/x.ts", reason: " " }] })),
      ),
    /must have a reason/,
  )
  assert.throws(
    () =>
      parsePatternConfig(
        document(config({ exempt: [{ file: "../x.ts", reason: "Fixture." }] })),
      ),
    /repository-relative path/,
  )
  assert.throws(
    () => parsePatternConfig(document(config({ slack: -1 }))),
    /slack must be a non-negative integer/,
  )
  const value = config()
  assert.deepEqual(parsePatternConfig(formatPatternConfig(value)), value)
})

function fixtureRepository(t: TestContext, value: PatternConfig) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "kano-patterns-"))
  t.after(() => fs.rmSync(root, { recursive: true, force: true }))
  const write = (file: string, text: string) => {
    fs.mkdirSync(path.dirname(path.join(root, file)), { recursive: true })
    fs.writeFileSync(path.join(root, file), text)
  }
  assert.equal(spawnSync("git", ["init", "--quiet"], { cwd: root }).status, 0)
  write(".gitignore", "ignored/\n")
  write(patternBaselineFile, formatPatternConfig(value))
  return { root, write }
}

function run(root: string, update = false) {
  const output: string[] = []
  const status = runPatternCheck({
    root,
    update,
    log: (message) => output.push(message),
    error: (message) => output.push(message),
  })
  return { status, output: output.join("\n") }
}

test("checks Git-visible files and tightens the baseline file on request", (t) => {
  const { root, write } = fixtureRepository(
    t,
    config({ baseline: { "src/large.ts": 15, "src/gone.ts": 30 } }),
  )
  write("src/owner.ts", "await fetch('/api/fixture')\n")
  write("src/large.ts", lines(12))
  write("ignored/huge.ts", lines(50))

  const checked = run(root)
  assert.equal(checked.status, 0, checked.output)
  assert.match(checked.output, /Pattern check passed \(1 request call site/)
  assert.match(checked.output, /src\/large\.ts shrank from 15 to 12/)

  const updated = run(root, true)
  assert.equal(updated.status, 0, updated.output)
  const stored = parsePatternConfig(
    fs.readFileSync(path.join(root, patternBaselineFile), "utf8"),
  )
  assert.deepEqual(stored.fileSize.baseline, { "src/large.ts": 12 })
  assert.equal(run(root).output.includes("note:"), false)

  write("src/view.tsx", "await fetch('/api/fixture')\n")
  write("src/large.ts", lines(15))
  const failed = run(root, true)
  assert.equal(failed.status, 1)
  assert.match(failed.output, /Pattern check found 2 item\(s\)/)
  assert.match(failed.output, /src\/view\.tsx:1 calls fetch/)
  assert.match(failed.output, /src\/large\.ts grew to 15 lines/)
  assert.deepEqual(
    parsePatternConfig(
      fs.readFileSync(path.join(root, patternBaselineFile), "utf8"),
    ).fileSize.baseline,
    { "src/large.ts": 12 },
  )
})
