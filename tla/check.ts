#!/usr/bin/env node
// Checks TLA+ specs with tla-ts.
//
// A spec is checked once per model config beside it: Foo.tla pairs with
// Foo.cfg and with each Foo-<variant>.cfg in the same directory. A .tla with no
// config is a helper module and is checked only through the specs that
// INSTANCE it. Module names resolve from the spec's own directory, then from
// the shared modules this package ships in tla/, such as Surface.tla.
//
// A spec with a view beside it, Foo.view.ts for Foo.tla, also has a state
// chart per config, Foo.chart.mmd for Foo.cfg, that aptre-tla chart writes and
// the check keeps current: a committed chart that differs from a fresh one
// fails the model.

import { createHash } from 'node:crypto'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { check, formatValue, type TraceStep } from 'tla-ts'

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

// ModelResult is the outcome of checking one Model.
export interface ModelResult extends Model {
  // pass when every invariant and property held, fail when one broke or the
  // model could not be checked.
  ok: boolean
  ms: number
  // rule names the broken invariant or property, or the tla-ts status.
  rule?: string
  // trace is the shortest path to the broken rule, one line per state.
  trace?: string[]
  // error is the message of a model that could not be checked.
  error?: string
}

// viewOf returns the view beside spec, Foo.view.ts for Foo.tla, or undefined
// when the spec has none and so draws no chart.
export function viewOf(spec: string): string | undefined {
  const view = spec.slice(0, -'.tla'.length) + '.view.ts'
  return fs.existsSync(view) ? view : undefined
}

// chartOf names the chart a model draws: Foo.chart.mmd for Foo.cfg.
export const chartOf = (model: Model) =>
  model.config.slice(0, -'.cfg'.length) + '.chart.mmd'

// drawChart compiles model's state chart with the view module at view. The
// chart compiler loads only for a spec with a view.
export async function drawChart(model: Model, view: string): Promise<string> {
  const [{ chartModel }, exports] = await Promise.all([
    import('./chart/run.js'),
    // Modules cache by URL; the content hash loads an edited view afresh.
    import(
      `${pathToFileURL(path.resolve(view)).href}?${createHash('sha256').update(fs.readFileSync(view)).digest('hex')}`
    ) as Promise<object>,
  ])
  const { chart, report } = await chartModel(model, { ...exports })
  if (chart === undefined) throw new Error(`no chart to draw:\n${report}`)
  return chart
}

// staleChart compares model's committed chart with a fresh one and describes
// the difference, or returns undefined when the chart is current or the spec
// has no view.
async function staleChart(model: Model): Promise<string | undefined> {
  const view = viewOf(model.spec)
  if (view === undefined) return undefined
  const file = chartOf(model)
  const rewrite = `run: aptre-tla chart ${path.relative('.', model.config)}`
  let fresh: string
  try {
    fresh = await drawChart(model, view)
  } catch (err) {
    return `${path.relative('.', view)} could not draw the chart: ${err instanceof Error ? err.message : String(err)}`
  }
  if (!fs.existsSync(file))
    return `${path.relative('.', file)} is missing; ${rewrite}`
  if (fs.readFileSync(file, 'utf8') !== fresh)
    return `${path.relative('.', file)} is stale; ${rewrite}`
  return undefined
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

// sourceRange matches the source position tla-ts appends to a step label.
const sourceRange = / line \d+, col \d+ to line \d+, col \d+ of module \w+$/

// constantRule matches the error tla-ts throws for a rule that depends on no
// variable and is false, which TLC reports as a violation.
const constantRule = /^The invariant of (\w+) is equal to FALSE$/

// missingSymbol matches the error tla-ts throws for an operator no module in
// scope defines.
const missingSymbol =
  /Could not find declaration or definition of symbol '([^']+)'/

// standardModule names the standard module that defines each operator a spec
// most often uses without extending it.
const standardModule: Record<string, string> = {
  '+': 'Naturals',
  '-': 'Naturals',
  '*': 'Naturals',
  '<': 'Naturals',
  '>': 'Naturals',
  '<=': 'Naturals',
  '>=': 'Naturals',
  '\\leq': 'Naturals',
  '\\geq': 'Naturals',
  '..': 'Naturals',
  '%': 'Naturals',
  '\\div': 'Naturals',
  Nat: 'Naturals',
  Int: 'Integers',
  Cardinality: 'FiniteSets',
  IsFiniteSet: 'FiniteSets',
  Seq: 'Sequences',
  Len: 'Sequences',
  Append: 'Sequences',
  Head: 'Sequences',
  Tail: 'Sequences',
  SubSeq: 'Sequences',
}

// explain adds the fix to an error whose cause is a missing EXTENDS.
function explain(error: string): string {
  const module = standardModule[missingSymbol.exec(error)?.[1] ?? '']
  return module ? `${error} (add EXTENDS ${module})` : error
}

// describeTrace renders each state of a trace: its step label, then the
// variables that changed from the state before.
function describeTrace(trace: TraceStep[]): string[] {
  let prev: Record<string, string> = {}
  return trace.map((step, i) => {
    const cur: Record<string, string> = {}
    for (const [k, v] of Object.entries(step.state)) cur[k] = formatValue(v)
    const changed = Object.keys(cur).filter((k) => cur[k] !== prev[k])
    prev = cur
    const label =
      i === 0
        ? 'Init'
        : (step.label ?? step.action ?? 'Next').replace(sourceRange, '')
    return `${i + 1}. ${label}: ${changed.map((k) => `${k} = ${cur[k]}`).join(', ')}`
  })
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

// checkModel checks one Model and, when its spec has a view, that its committed
// chart is current. A model tla-ts cannot check, such as one whose config names
// a rule that depends on no variable, is reported as failed with its error
// rather than thrown.
export async function checkModel(
  model: Model,
  shared = sharedModulesDir(),
): Promise<ModelResult> {
  const r = await checkSpec(model, shared)
  const stale = await staleChart(model)
  if (stale === undefined) return r
  return {
    ...r,
    ok: false,
    rule: r.rule ?? 'chart',
    error: r.error ? `${r.error}\n${stale}` : stale,
  }
}

// checkSpec checks one Model's rules.
async function checkSpec(model: Model, shared: string): Promise<ModelResult> {
  const loadModule = moduleLoader(model.spec, shared)
  const start = performance.now()
  try {
    const result = await check({
      source: fs.readFileSync(model.spec, 'utf8'),
      config: fs.readFileSync(model.config, 'utf8'),
      loadModule,
    })
    const ms = performance.now() - start
    if (result.status === 'success') return { ...model, ok: true, ms }
    const trace = result.violation?.trace ?? result.failure?.trace ?? []
    return {
      ...model,
      ok: false,
      ms,
      rule: result.violation?.name ?? result.status,
      trace: describeTrace(trace),
      error: result.violation?.message ?? result.failure?.message,
    }
  } catch (err) {
    const error = explain(err instanceof Error ? err.message : String(err))
    return {
      ...model,
      ok: false,
      ms: performance.now() - start,
      rule: constantRule.exec(error)?.[1] ?? 'error',
      error,
    }
  }
}

// main checks every Model under the given paths, the working directory when
// none are given, prints one line per Model and the trace of each failure, and
// exits 1 when any failed.
async function main(args: string[]): Promise<number> {
  if (args[0] === '-h' || args[0] === '--help') {
    console.log(
      [
        'usage: aptre-tla [path...]',
        '       aptre-tla chart [path...]',
        'Checks every TLA+ spec with a .cfg beside it, and that the chart of each',
        'spec with a Foo.view.ts is current. chart writes those charts.',
      ].join('\n'),
    )
    return 0
  }
  if (args[0] === 'chart') return writeCharts(args.slice(1))
  const shared = sharedModulesDir()
  let failed = 0
  let count = 0
  for (const target of args.length ? args : ['.']) {
    for (const model of findModels(target)) {
      count++
      const r = await checkModel(model, shared)
      const name = path.relative('.', r.config)
      console.log(
        `${r.ok ? 'pass' : 'FAIL'}  ${r.ms.toFixed(0).padStart(5)} ms  ${name}${r.rule ? `  ${r.rule}` : ''}`,
      )
      if (r.ok) continue
      failed++
      for (const line of r.error?.split('\n') ?? [])
        console.log(`      ${line}`)
      for (const line of r.trace ?? []) console.log(`      ${line}`)
    }
  }
  console.log(`${count} model(s), ${failed} failed`)
  return failed ? 1 : 0
}

// writeCharts writes the chart of every Model under the given paths whose spec
// has a view, and exits 1 when one could not be drawn.
async function writeCharts(args: string[]): Promise<number> {
  let failed = 0
  for (const target of args.length ? args : ['.']) {
    for (const model of findModels(target)) {
      const view = viewOf(model.spec)
      if (view === undefined) continue
      const file = chartOf(model)
      try {
        const chart = await drawChart(model, view)
        const same =
          fs.existsSync(file) && fs.readFileSync(file, 'utf8') === chart
        if (!same) fs.writeFileSync(file, chart)
        console.log(
          `${same ? 'current' : 'wrote  '}  ${path.relative('.', file)}`,
        )
      } catch (err) {
        failed++
        console.log(`FAIL     ${path.relative('.', file)}`)
        console.log(
          `         ${err instanceof Error ? err.message : String(err)}`,
        )
      }
    }
  }
  return failed ? 1 : 0
}

if (
  process.argv[1] &&
  fs.realpathSync(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  process.exitCode = await main(process.argv.slice(2))
}
