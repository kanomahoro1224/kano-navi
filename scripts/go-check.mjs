#!/usr/bin/env node

import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  rmSync,
  writeFileSync,
} from "node:fs"
import { join } from "node:path"
import { tmpdir } from "node:os"
import { spawnSync } from "node:child_process"

const root = join(import.meta.dirname, "..")
const moduleRoot = join(root, "backend")
const go = process.env.GO || "go"
const docker = process.env.DOCKER || "docker"
const dockerImage = process.env.GO_DOCKER_IMAGE || "kano-navi-go:ci"

// Naming stages runs only those, in this order; no name runs all of them.
// Style runs the format stage, and CI runs the other two as separate jobs.
const stages = { format, builds, container }
const requested = process.argv.slice(2)
for (const stage of requested)
  if (!Object.hasOwn(stages, stage)) {
    console.error(
      `usage: node scripts/go-check.mjs [${Object.keys(stages).join("|")}]...`,
    )
    process.exit(2)
  }
const selected = Object.keys(stages).filter(
  (stage) => !requested.length || requested.includes(stage),
)
for (const stage of selected) stages[stage]()
console.log(`go-check: ${selected.join(", ")} PASS`)

// gofmt over LF copies, so a CRLF checkout reports only real differences.
function format() {
  const goFiles = walk(moduleRoot).filter((file) => file.endsWith(".go"))
  const formatRoot = mkdtempSync(join(tmpdir(), "kano-go-format-"))
  const formatFiles = goFiles.map((file) => {
    const target = join(formatRoot, file.slice(moduleRoot.length + 1))
    mkdirSync(join(target, ".."), { recursive: true })
    writeFileSync(target, readFileSync(file, "utf8").replaceAll("\r\n", "\n"))
    return target
  })
  // gofmt reads the copies in batches: Windows bounds a command line at
  // 32,767 characters, which every copy's path together exceeds.
  const formatBatch = 100
  try {
    for (let start = 0; start < formatFiles.length; start += formatBatch) {
      run(
        "gofmt -d",
        "gofmt",
        ["-d", ...formatFiles.slice(start, start + formatBatch)],
        { cwd: root, failOnStdout: true },
      )
    }
  } finally {
    rmSync(formatRoot, { recursive: true, force: true })
  }
}

// The cgo contracts and vet belong to make ci-backend; this stage covers the
// CGO-free contracts and the platform builds.
function builds() {
  run("go build ./...", go, ["build", "./..."], { cwd: moduleRoot })
  run("CGO_ENABLED=0 go test ./...", go, ["test", "./..."], {
    cwd: moduleRoot,
    env: { CGO_ENABLED: "0" },
  })
  run("CGO_ENABLED=0 go build ./...", go, ["build", "./..."], {
    cwd: moduleRoot,
    env: { CGO_ENABLED: "0" },
  })
  for (const goos of ["linux", "windows"])
    run(
      `GOOS=${goos} GOARCH=amd64 CGO_ENABLED=0 go build ./...`,
      go,
      ["build", "./..."],
      {
        cwd: moduleRoot,
        env: { GOOS: goos, GOARCH: "amd64", CGO_ENABLED: "0" },
      },
    )
}

function container() {
  run(
    "Docker independent Go service image",
    docker,
    ["build", "-f", "backend/Dockerfile", "-t", dockerImage, "backend"],
    { cwd: root },
  )
  run(
    "Docker read-only HTTP smoke",
    process.execPath,
    ["scripts/go-docker-smoke.mjs"],
    {
      cwd: root,
      env: { GO: go, DOCKER: docker, GO_DOCKER_IMAGE: dockerImage },
    },
  )
}

function run(label, command, args, options = {}) {
  console.log(`go-check: ${label}`)
  const result = spawnSync(command, args, {
    cwd: options.cwd,
    env: { ...process.env, ...(options.env || {}) },
    encoding: "utf8",
  })
  if (result.stdout) process.stdout.write(result.stdout)
  if (result.stderr) process.stderr.write(result.stderr)
  if (options.failOnStdout && result.stdout) {
    console.error(`go-check: ${label} reported formatting differences`)
    process.exit(1)
  }
  if (result.error || result.status !== 0) {
    console.error(`go-check: ${label} failed`)
    process.exit(result.status || 1)
  }
}

function walk(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name)
    return entry.isDirectory() ? walk(path) : [path]
  })
}
