import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"
import { satisfiesEngine } from "../scripts/check-node.ts"

test("the engines floor is compared numerically", () => {
  assert.equal(satisfiesEngine("v24.22.1", ">=24.22.1"), true)
  assert.equal(satisfiesEngine("v24.22.10", ">=24.22.9"), true)
  assert.equal(satisfiesEngine("v25.0.0", ">=24.22.1"), true)
  assert.equal(satisfiesEngine("v24.22.0", ">=24.22.1"), false)
  assert.equal(satisfiesEngine("v22.23.4", ">=24.22.1"), false)
  assert.throws(() => satisfiesEngine("v24.22.1", "^24.22.1"))
})

test(".nvmrc, the engines floor and the Docker images name one version", () => {
  const nvmrc = readFileSync(".nvmrc", "utf8").trim()
  const range = JSON.parse(readFileSync("package.json", "utf8")).engines.node
  assert.equal(range, `>=${nvmrc}`)
  const images = [
    ...readFileSync("Dockerfile", "utf8").matchAll(/FROM node:(\S+?)-/g),
  ].map((match) => match[1])
  assert.ok(images.length > 0)
  for (const image of images) assert.equal(image, nvmrc)
})
