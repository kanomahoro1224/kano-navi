import fs from "node:fs"
import path from "node:path"
import { fileURLToPath } from "node:url"

import { repositoryFiles, repositoryRoot } from "./check-sensitive.ts"

// Pattern ratchet: rules that code review alone keeps losing to precedent.
// Each rule starts from a recorded baseline of the existing exceptions and
// fails only when a change adds one. The baseline only ever tightens:
// --update-baseline lowers recorded counts and drops entries that no longer
// apply, and nothing raises a count automatically.

export const patternBaselineFile = "scripts/pattern-baseline.json"
export const requestOwnershipDoc =
  "docs/architecture/frontend/request-ownership.md"
export const validationDoc = "docs/development/testing/validation.md"

export type FileEntry = { file: string; reason: string }

export type PatternConfig = {
  version: 1
  requests: { allowed: FileEntry[] }
  fileSize: {
    limit: number
    slack: number
    exempt: FileEntry[]
    baseline: Record<string, number>
  }
}

export type PatternFinding = {
  file: string
  line: number | null
  message: string
}

export type PatternReport = {
  findings: PatternFinding[]
  // Entries the baseline can drop or lower; they never fail the check.
  notices: string[]
  requestSites: number
  codeFiles: number
  oversizedFiles: number
}

const codeExtensions = new Set([".ts", ".tsx", ".go", ".mjs"])
const requestPattern =
  /(?<![\w$.])fetch\s*\(|\b(?:window|globalThis|self)\.fetch\b/u

function normalizePath(file: string): string {
  return file.replaceAll("\\", "/")
}

export function isCodeFile(file: string): boolean {
  return codeExtensions.has(path.posix.extname(normalizePath(file)))
}

// Frontend sources outside tests, where browser requests have owners.
export function isRequestScope(file: string): boolean {
  const normalized = normalizePath(file)
  return (
    normalized.startsWith("src/") &&
    /\.(?:ts|tsx|mjs)$/u.test(normalized) &&
    !/\.(?:test|spec)\.[^/]+$/u.test(normalized) &&
    !normalized.split("/").includes("__tests__")
  )
}

// Line numbers as an editor shows them; a final newline adds no line.
export function countLines(text: string): number {
  if (text === "") return 0
  const lines = text.split(/\r?\n/u)
  return lines.at(-1) === "" ? lines.length - 1 : lines.length
}

// The 1-based lines that call or reference the global fetch. Comment lines are
// skipped so prose about requests is not a finding.
export function requestLines(text: string): number[] {
  const lines: number[] = []
  for (const [index, line] of text.split(/\r?\n/u).entries()) {
    const trimmed = line.trimStart()
    if (/^(?:\/\/|\/\*|\*)/u.test(trimmed)) continue
    if (requestPattern.test(line)) lines.push(index + 1)
  }
  return lines
}

function fail(message: string): never {
  throw new Error(message)
}

function isObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value)
}

function keysAre(value: Record<string, unknown>, keys: string[]): boolean {
  return Object.keys(value).sort().join(",") === [...keys].sort().join(",")
}

function isRepositoryPath(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value !== "" &&
    value === normalizePath(value) &&
    !value.startsWith("/") &&
    !value.split("/").some((part) => part === "" || part === "..")
  )
}

function parseEntries(value: unknown, label: string): FileEntry[] {
  if (!Array.isArray(value)) fail(`${label} must be an array`)
  const seen = new Set<string>()
  return value.map((entry, index) => {
    const name = `${label} entry ${index + 1}`
    if (!isObject(entry) || !keysAre(entry, ["file", "reason"]))
      fail(`${name} must contain only file and reason`)
    if (!isRepositoryPath(entry.file))
      fail(`${name} must be a repository-relative path`)
    if (typeof entry.reason !== "string" || entry.reason.trim() === "")
      fail(`${name} must have a reason`)
    if (seen.has(entry.file)) fail(`${name} duplicates ${entry.file}`)
    seen.add(entry.file)
    return { file: entry.file, reason: entry.reason }
  })
}

function isCount(value: unknown, minimum: number): value is number {
  return (
    typeof value === "number" && Number.isInteger(value) && value >= minimum
  )
}

export function parsePatternConfig(contents: string): PatternConfig {
  let parsed: unknown
  try {
    parsed = JSON.parse(contents)
  } catch (error) {
    fail(
      `invalid JSON: ${error instanceof Error ? error.message : String(error)}`,
    )
  }
  if (
    !isObject(parsed) ||
    !keysAre(parsed, ["version", "requests", "fileSize"]) ||
    parsed.version !== 1
  )
    fail("root must contain only version 1, requests and fileSize")

  const { requests, fileSize } = parsed
  if (!isObject(requests) || !keysAre(requests, ["allowed"]))
    fail("requests must contain only allowed")
  const allowed = parseEntries(requests.allowed, "requests.allowed")

  if (
    !isObject(fileSize) ||
    !keysAre(fileSize, ["limit", "slack", "exempt", "baseline"])
  )
    fail("fileSize must contain only limit, slack, exempt and baseline")
  if (!isCount(fileSize.limit, 1))
    fail("fileSize.limit must be a positive integer")
  if (!isCount(fileSize.slack, 0))
    fail("fileSize.slack must be a non-negative integer")
  const exempt = parseEntries(fileSize.exempt, "fileSize.exempt")
  if (!isObject(fileSize.baseline)) fail("fileSize.baseline must be an object")
  const baseline: Record<string, number> = {}
  for (const [file, count] of Object.entries(fileSize.baseline)) {
    if (!isRepositoryPath(file))
      fail(`fileSize.baseline key ${file} must be a repository-relative path`)
    if (!isCount(count, fileSize.limit + 1))
      fail(
        `fileSize.baseline ${file} must record a line count over the ${fileSize.limit}-line limit`,
      )
    if (exempt.some((entry) => entry.file === file))
      fail(`fileSize.baseline ${file} is also exempt`)
    baseline[file] = count
  }

  return {
    version: 1,
    requests: { allowed },
    fileSize: {
      limit: fileSize.limit,
      slack: fileSize.slack,
      exempt,
      baseline,
    },
  }
}

// The repository files the rules read, as path -> text. Unreadable or deleted
// paths are skipped.
export type Sources = Map<string, string>

export function checkPatterns(
  config: PatternConfig,
  sources: Sources,
): PatternReport {
  const findings: PatternFinding[] = []
  const notices: string[] = []

  const allowed = new Set(config.requests.allowed.map((entry) => entry.file))
  const owners = new Set<string>()
  let requestSites = 0
  for (const [file, text] of sources) {
    if (!isRequestScope(file)) continue
    const lines = requestLines(text)
    requestSites += lines.length
    if (lines.length > 0) owners.add(file)
    if (allowed.has(file)) continue
    for (const line of lines) {
      findings.push({
        file,
        line,
        message: `calls fetch outside the request owners. Route the request through an owning hook or module (see ${requestOwnershipDoc}); a new owner needs a reviewed reason in ${patternBaselineFile}.`,
      })
    }
  }
  for (const file of allowed) {
    if (!owners.has(file))
      notices.push(
        `${file} is a listed request owner but calls no fetch; --update-baseline drops it.`,
      )
  }

  const { limit, slack, baseline } = config.fileSize
  const exempt = new Set(config.fileSize.exempt.map((entry) => entry.file))
  let codeFiles = 0
  let oversizedFiles = 0
  for (const [file, text] of sources) {
    if (!isCodeFile(file)) continue
    codeFiles += 1
    if (exempt.has(file)) continue
    const lines = countLines(text)
    const recorded = baseline[file]
    if (lines > limit) oversizedFiles += 1
    if (recorded === undefined) {
      if (lines > limit)
        findings.push({
          file,
          line: null,
          message: `has ${lines} lines, over the ${limit}-line limit. Split it into cohesive modules below ${limit} lines (see ${validationDoc}#pattern-ratchet); a renamed file moves its entry in ${patternBaselineFile}.`,
        })
      continue
    }
    if (lines > recorded + slack) {
      findings.push({
        file,
        line: null,
        message: `grew to ${lines} lines, past its baseline of ${recorded} plus ${slack} lines of slack. Move the new code into a separate module or shrink the file; do not raise the baseline to pass.`,
      })
    } else if (lines <= limit) {
      notices.push(
        `${file} is down to ${lines} lines, under the limit; --update-baseline drops it.`,
      )
    } else if (lines < recorded) {
      notices.push(
        `${file} shrank from ${recorded} to ${lines} lines; --update-baseline records it.`,
      )
    }
  }
  for (const file of Object.keys(baseline)) {
    if (!sources.has(file))
      notices.push(
        `${file} is baselined but not in the repository; --update-baseline drops it.`,
      )
  }

  return { findings, notices, requestSites, codeFiles, oversizedFiles }
}

// Lowers recorded counts and drops entries that no longer apply. It never
// adds an entry or raises a count, so a finding still fails afterwards.
export function tightenBaseline(
  config: PatternConfig,
  sources: Sources,
): PatternConfig {
  const allowed = config.requests.allowed.filter(
    (entry) =>
      sources.has(entry.file) &&
      requestLines(sources.get(entry.file) ?? "").length > 0,
  )
  const baseline: Record<string, number> = {}
  for (const file of Object.keys(config.fileSize.baseline).sort()) {
    const text = sources.get(file)
    if (text === undefined) continue
    const lines = countLines(text)
    if (lines <= config.fileSize.limit) continue
    baseline[file] = Math.min(lines, config.fileSize.baseline[file])
  }
  return {
    ...config,
    requests: { allowed },
    fileSize: { ...config.fileSize, baseline },
  }
}

export function formatPatternConfig(config: PatternConfig): string {
  return `${JSON.stringify(config, null, 2)}\n`
}

export function readSources(root = repositoryRoot): Sources {
  const sources: Sources = new Map()
  for (const file of repositoryFiles(root)) {
    if (!isCodeFile(file)) continue
    try {
      const fullPath = path.resolve(root, file)
      if (!fs.lstatSync(fullPath).isFile()) continue
      sources.set(file, fs.readFileSync(fullPath, "utf8"))
    } catch {
      // A tracked path deleted from the working tree has nothing to check.
    }
  }
  return sources
}

function loadConfig(root: string): PatternConfig {
  const fullPath = path.resolve(root, patternBaselineFile)
  try {
    return parsePatternConfig(fs.readFileSync(fullPath, "utf8"))
  } catch (error) {
    throw new Error(
      `${patternBaselineFile}: ${error instanceof Error ? error.message : String(error)}`,
    )
  }
}

export function runPatternCheck({
  root = repositoryRoot,
  update = false,
  log = console.log,
  error = console.error,
}: {
  root?: string
  update?: boolean
  log?: (message: string) => void
  error?: (message: string) => void
} = {}): number {
  let config = loadConfig(root)
  const sources = readSources(root)

  if (update) {
    const tightened = tightenBaseline(config, sources)
    const next = formatPatternConfig(tightened)
    if (next !== formatPatternConfig(config)) {
      fs.writeFileSync(path.resolve(root, patternBaselineFile), next)
      log(`Tightened ${patternBaselineFile}.`)
    } else {
      log(`${patternBaselineFile} is already as tight as the sources allow.`)
    }
    config = tightened
  }

  const report = checkPatterns(config, sources)
  for (const notice of report.notices) log(`note: ${notice}`)
  const baselined = Object.keys(config.fileSize.baseline).length
  const summary = `${report.requestSites} request call site(s) in ${config.requests.allowed.length} owner file(s); ${report.codeFiles} code file(s), ${report.oversizedFiles} over ${config.fileSize.limit} lines (${baselined} baselined, ${config.fileSize.exempt.length} exempt)`

  if (report.findings.length > 0) {
    error(`Pattern check found ${report.findings.length} item(s):`)
    for (const finding of report.findings) {
      const location =
        finding.line === null ? finding.file : `${finding.file}:${finding.line}`
      error(`- ${location} ${finding.message}`)
    }
    error(`Checked ${summary}.`)
    return 1
  }
  log(`Pattern check passed (${summary}).`)
  return 0
}

function main() {
  const flags = process.argv.slice(2)
  const unknown = flags.filter((flag) => flag !== "--update-baseline")
  if (unknown.length > 0) {
    console.error(
      `Unknown argument(s): ${unknown.join(" ")}. Usage: check-patterns.ts [--update-baseline]`,
    )
    process.exitCode = 2
    return
  }
  try {
    process.exitCode = runPatternCheck({
      update: flags.includes("--update-baseline"),
    })
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error))
    process.exitCode = 2
  }
}

if (
  process.argv[1] &&
  path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  main()
}
