// Finds TLA+ models and loads the modules they name.

import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

// Directories never walked for specs. A testdata directory holds fixtures,
// such as deliberately broken specs, that its own tests check by path, and
// the config mutants of the specs beside it (see configMutantsOf).
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

// modelsInDir yields the Models whose configs sit directly in dir. A config
// directly in a testdata directory is a config mutant, not a Model.
function* modelsInDir(dir: string, names: string[]): Generator<Model> {
  if (path.basename(dir) === 'testdata') return
  const stems = names
    .filter((n) => n.endsWith('.tla'))
    .map((n) => n.slice(0, -4))
  for (const name of [...names].sort()) {
    if (!name.endsWith('.cfg')) continue
    const stem = stemOf(name, stems)
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

// stemOf returns the stem of the spec among stems that the config file name
// cfg belongs to, Foo for Foo.cfg or Foo-<variant>.cfg, preferring the
// longest, or undefined.
function stemOf(cfg: string, stems: string[]): string | undefined {
  const name = cfg.slice(0, -'.cfg'.length)
  return stems
    .filter((s) => name === s || name.startsWith(s + '-'))
    .sort((a, b) => b.length - a.length)[0]
}

// findConfigMutants returns the config mutants in dir, a testdata directory:
// each config in it paired with the spec beside dir it belongs to, Foo.tla
// for testdata/Foo.cfg and testdata/Foo-<name>.cfg. A config mutant checks
// the spec under changed constants and must break the rule its expect line
// names.
export function findConfigMutants(dir: string): Model[] {
  if (path.basename(dir) !== 'testdata' || !fs.statSync(dir).isDirectory())
    return []
  const parent = path.dirname(dir)
  const stems = fs
    .readdirSync(parent)
    .filter((n) => n.endsWith('.tla'))
    .map((n) => n.slice(0, -'.tla'.length))
  const mutants: Model[] = []
  for (const name of fs.readdirSync(dir).sort()) {
    if (!name.endsWith('.cfg')) continue
    const stem = stemOf(name, stems)
    if (stem === undefined) continue
    mutants.push({
      spec: path.join(parent, stem + '.tla'),
      config: path.join(dir, name),
    })
  }
  return mutants
}

// configMutantsOf returns the config mutants of spec, from the testdata
// directory beside it.
export function configMutantsOf(spec: string): Model[] {
  const dir = path.join(path.dirname(spec), 'testdata')
  if (!fs.existsSync(dir)) return []
  return findConfigMutants(dir).filter((m) => m.spec === spec)
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
