import { readFileSync } from "node:fs"
import path from "node:path"
import { fileURLToPath } from "node:url"

/** Parses a semantic version such as "24.22.1" or "v24.22.1". */
function parts(version: string) {
  const match = /^v?(\d+)\.(\d+)\.(\d+)$/.exec(version.trim())
  if (!match) throw new Error(`unreadable version ${version}`)
  return match.slice(1).map(Number)
}

/**
 * Reports whether a Node version meets the engines range. The project states
 * its floor as ">=major.minor.patch"; any other form is a configuration
 * error rather than a pass.
 */
export function satisfiesEngine(version: string, range: string) {
  const floor = /^>=\s*(\S+)$/.exec(range.trim())
  if (!floor) throw new Error(`unsupported engines range ${range}`)
  const have = parts(version)
  const need = parts(floor[1])
  for (let i = 0; i < 3; i++) {
    if (have[i] !== need[i]) return have[i] > need[i]
  }
  return true
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..")
  const range = JSON.parse(
    readFileSync(path.join(root, "package.json"), "utf8"),
  ).engines?.node
  if (typeof range !== "string") {
    console.error("package.json has no engines.node range")
    process.exit(1)
  }
  if (!satisfiesEngine(process.version, range)) {
    console.error(
      `Node ${process.version} does not meet the required ${range} (see .nvmrc)`,
    )
    process.exit(1)
  }
}
