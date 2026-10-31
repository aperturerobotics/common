// Finds TLA+ models and loads the modules they name.

import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

// Directories never walked for specs. A testdata directory holds fixtures,
// such as deliberately broken specs, that its own tests check by path.
const skipDirs = new Set([
  'node_modules',
  'vendor',
  'dist',
  'build',
  'testdata',
])

// Model is one spec checked under one config.
export interface Model {
  spec: string
  config: string
}

// sharedModulesDir returns this package's tla/ directory, from the source tree
// or from the built dist/tla/.
export function sharedModulesDir(): string {
  let dir = path.dirname(fileURLToPath(import.meta.url))
  for (;;) {
    for (const candidate of [dir, path.join(dir, 'tla')]) {
      if (fs.existsSync(path.join(candidate, 'Surface.tla'))) return candidate
    }
    const parent = path.dirname(dir)
    if (parent === dir)
      throw new Error('@aptre/common: tla/Surface.tla not found')
    dir = parent
  }
}

// modelsInDir yields the Models whose configs sit directly in dir.
function* modelsInDir(dir: string, names: string[]): Generator<Model> {
  const stems = names
    .filter((n) => n.endsWith('.tla'))
    .map((n) => n.slice(0, -4))
    .sort((a, b) => b.length - a.length)
  for (const name of names) {
    if (!name.endsWith('.cfg')) continue
    const cfg = name.slice(0, -4)
    const stem = stems.find((s) => cfg === s || cfg.startsWith(s + '-'))
    if (stem === undefined) continue
    yield { spec: path.join(dir, stem + '.tla'), config: path.join(dir, name) }
  }
}

// findModels yields every Model under target, a directory, a .tla or a .cfg.
export function* findModels(target: string): Generator<Model> {
  const stat = fs.statSync(target)
  if (stat.isFile()) {
    const dir = path.dirname(target)
    const base = path.basename(target)
    for (const model of modelsInDir(dir, fs.readdirSync(dir))) {
      if (model.spec === target || model.config === target) yield model
      else if (base.endsWith('.tla') && path.basename(model.spec) === base)
        yield model
    }
    return
  }
  const entries = fs.readdirSync(target, { withFileTypes: true })
  yield* modelsInDir(
    target,
    entries.filter((e) => e.isFile()).map((e) => e.name),
  )
  for (const e of entries) {
    if (!e.isDirectory() || e.name.startsWith('.') || skipDirs.has(e.name))
      continue
    yield* findModels(path.join(target, e.name))
  }
}

// moduleLoader resolves a module name for spec: from the spec's own directory,
// then from the shared modules.
export function moduleLoader(spec: string, shared = sharedModulesDir()) {
  const dirs = [path.dirname(spec), shared]
  return (name: string) => {
    for (const dir of dirs) {
      const file = path.join(dir, `${name}.tla`)
      if (fs.existsSync(file)) return fs.readFileSync(file, 'utf8')
    }
    return undefined
  }
}
