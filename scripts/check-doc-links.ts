import fs from "node:fs"
import path from "node:path"
import { fileURLToPath } from "node:url"

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url))
const repositoryRoot = path.resolve(scriptDirectory, "..")
const externalTargetPattern = /^(?:[a-z][a-z0-9+.-]*:|\/\/)/iu

function normalizePath(file: string): string {
  return file.replaceAll("\\", "/")
}

function markdownFiles(directory: string, relativeDirectory = ""): string[] {
  const files: string[] = []
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    const relativePath = normalizePath(path.join(relativeDirectory, entry.name))
    if (entry.isDirectory()) {
      files.push(
        ...markdownFiles(path.join(directory, entry.name), relativePath),
      )
    } else if (entry.isFile() && entry.name.toLowerCase().endsWith(".md")) {
      files.push(relativePath)
    }
  }
  return files
}

function decode(value: string): string {
  try {
    return decodeURIComponent(value)
  } catch {
    return value
  }
}

// The local file (empty for the same file) and heading anchor of a link.
function linkTarget(
  rawTarget: string,
): { file: string; fragment: string } | null {
  const withoutTitle = rawTarget.trim()
  const target = withoutTitle.startsWith("<")
    ? withoutTitle.slice(1, withoutTitle.indexOf(">"))
    : withoutTitle.split(/\s+/u, 1)[0]
  if (!target || externalTargetPattern.test(target)) return null
  const hash = target.indexOf("#")
  const file = (hash >= 0 ? target.slice(0, hash) : target).split("?", 1)[0]
  const fragment = hash >= 0 ? decode(target.slice(hash + 1)) : ""
  if (!file && !fragment) return null
  return { file: decode(file), fragment }
}

// GitHub-style heading anchors of a Markdown file.
const anchorCache = new Map<string, Set<string>>()
function headingAnchors(file: string): Set<string> {
  const cached = anchorCache.get(file)
  if (cached) return cached
  const anchors = new Set<string>()
  const counts = new Map<string, number>()
  let fenced = false
  for (const line of fs.readFileSync(file, "utf8").split(/\r?\n/u)) {
    if (/^\s*(```|~~~)/u.test(line)) fenced = !fenced
    if (fenced) continue
    const heading = /^#{1,6}\s+(.*?)\s*#*\s*$/u.exec(line)
    if (!heading) continue
    const slug = heading[1]
      .trim()
      .toLowerCase()
      .replace(/<[^>]+>/gu, "")
      .replace(/[^\p{L}\p{N}\s_-]/gu, "")
      .replace(/\s/gu, "-")
    const count = counts.get(slug) ?? 0
    counts.set(slug, count + 1)
    anchors.add(count ? `${slug}-${count}` : slug)
  }
  anchorCache.set(file, anchors)
  return anchors
}

export function checkMarkdownLinks() {
  const files = [
    "README.md",
    "README.zh-CN.md",
    "README.ja.md",
    "backend/README.md",
    ...markdownFiles(path.join(repositoryRoot, "docs"), "docs"),
    ".github/SECURITY.md",
    "AGENTS.md",
    "backend/AGENTS.md",
    "src/AGENTS.md",
    "scripts/AGENTS.md",
    ...(fs.existsSync(path.join(repositoryRoot, ".claude/skills"))
      ? markdownFiles(
          path.join(repositoryRoot, ".claude/skills"),
          ".claude/skills",
        )
      : []),
  ]
    .filter((file, index, all) => all.indexOf(file) === index)
    .filter((file) => fs.existsSync(path.resolve(repositoryRoot, file)))
  const missing = []
  let linksChecked = 0
  const linkPattern = /!?\[[^\]]*\]\(([^)]+)\)/gu

  for (const file of files) {
    const contents = fs.readFileSync(path.resolve(repositoryRoot, file), "utf8")
    for (const [index, line] of contents.split(/\r?\n/u).entries()) {
      linkPattern.lastIndex = 0
      for (const match of line.matchAll(linkPattern)) {
        const target = linkTarget(match[1])
        if (target === null) continue
        linksChecked += 1
        const fullTarget = target.file
          ? path.resolve(
              path.dirname(path.resolve(repositoryRoot, file)),
              target.file,
            )
          : path.resolve(repositoryRoot, file)
        const shown = target.fragment
          ? `${target.file}#${target.fragment}`
          : target.file
        if (!fs.existsSync(fullTarget)) {
          missing.push({ file, line: index + 1, target: shown })
        } else if (
          target.fragment &&
          fullTarget.toLowerCase().endsWith(".md") &&
          !headingAnchors(fullTarget).has(target.fragment)
        ) {
          missing.push({ file, line: index + 1, target: shown })
        }
      }
    }
  }

  if (missing.length > 0) {
    console.error(
      `Documentation link check found ${missing.length} missing target(s):`,
    )
    for (const item of missing)
      console.error(`- ${item.file}:${item.line}: ${item.target}`)
    return 1
  }
  console.log(
    `Documentation link check passed (${files.length} Markdown file(s), ${linksChecked} local link(s)).`,
  )
  return 0
}

if (
  process.argv[1] &&
  path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  process.exitCode = checkMarkdownLinks()
}
