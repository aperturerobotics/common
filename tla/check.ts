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
//
// A spec with Foo.mutants.json beside it also checks each mutant listed there:
// the spec's source with exact text replacements, checked in memory. Each
// mutant removes a guard the code relies on, so each must break its expected
// rule; a mutant that still passes means the spec does not model what the
// guard protects, or the guard is dead code, and fails the run.
//
// Every result carries its state counts, so a baseline and a simpler design
// can be compared over the same bounds; -json prints one object per result.

import { createHash } from 'node:crypto'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { isMainThread } from 'node:worker_threads'
import { check, formatValue, type TraceStep } from 'tla-ts'
import {
  findModels,
  type Model,
  moduleLoader,
  sharedModulesDir,
} from './models.js'
import { startRunner, type Runner } from './runner.js'

export { findModels, type Model, moduleLoader, sharedModulesDir }

// ModelResult is the outcome of checking one Model.
export interface ModelResult extends Model {
  // pass when every invariant and property held, fail when one broke or the
  // model could not be checked.
  ok: boolean
  ms: number
  // status is the tla-ts status, or error when the model could not be
  // checked; a rule false on constants alone counts as invariant-violation.
  status: string
  // states, generated and depth count the distinct states, the generated
  // states and the longest shortest path of the search.
  states: number
  generated: number
  depth: number
  // collision estimates the chance that two distinct states shared a 64-bit
  // fingerprint, so that the search skipped one, as TLC estimates it.
  collision: Collision
  // rule names the broken invariant or property, or the tla-ts status. A
  // broken temporal formula shows its text and module.
  rule?: string
  // trace is the shortest path to the broken rule, one line per state.
  trace?: string[]
  // error is the message of a model that could not be checked.
  error?: string
}

// Progress is how far a long search has got, reported every 60 s: its counts,
// and, from a checker that measures them, the bytes its seen states take and
// the bytes and states of the queue it spilled to disk.
export interface Progress {
  distinctStates: number
  generatedStates: number
  depth: number
  queuedStates: number
  seenBytes?: number
  spillBytes?: number
  spillStates?: number
}

// Collision holds TLC's two estimates of a fingerprint collision: calculated
// from the state counts, and observed from the closest pair of stored
// fingerprints. The checker measures the observed one only on a pass whose
// calculated estimate is at least 1e-10.
export interface Collision {
  calculated: number
  observed?: number
}

// collisionOf returns the estimates for a search that found states distinct
// states among generated, with the closest stored fingerprints distance apart.
export function collisionOf(
  states: number,
  generated: number,
  distance?: bigint,
): Collision {
  const calculated = states * ((generated - states) / 2 ** 64)
  if (distance === undefined || distance === 0n) return { calculated }
  return { calculated, observed: 1 / Number(distance) }
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
  const rewrite = `run: aptre-tla chart ${model.config}`
  let fresh: string
  try {
    fresh = await drawChart(model, view)
  } catch (err) {
    return `${view} could not draw the chart: ${err instanceof Error ? err.message : String(err)}`
  }
  if (!fs.existsSync(file)) return `${file} is missing; ${rewrite}`
  if (fs.readFileSync(file, 'utf8') !== fresh)
    return `${file} is stale; ${rewrite}`
  return undefined
}

// sourceRange matches the source position tla-ts appends to a step label.
const sourceRange = / line \d+, col \d+ to line \d+, col \d+ of module \w+$/

// constantRule matches the error tla-ts throws for a rule that depends on no
// variable and is false, which TLC reports as a violation.
const constantRule = /^The invariant of (\w+) is equal to FALSE$/

// location matches the name tla-ts gives a broken temporal formula, such as a
// refinement's [][Next]_vars: only its source range.
const location =
  /^line (\d+), col (\d+) to line (\d+), col (\d+) of module (\w+)$/

// ruleName names a broken rule. A rule tla-ts names by its source range shows
// the formula's text and its module instead.
function ruleName(
  name: string,
  source: string,
  loadModule: (name: string) => string | undefined,
): string {
  const m = location.exec(name)
  if (!m) return name
  const [l1, c1, l2, c2] = m.slice(1, 5).map(Number)
  const module = m[5]
  const text =
    /MODULE\s+(\w+)/.exec(source)?.[1] === module ? source : loadModule(module)
  const lines = text?.split('\n').slice(l1 - 1, l2) ?? []
  if (!lines.length) return name
  lines[lines.length - 1] = lines[lines.length - 1].slice(0, c2)
  lines[0] = lines[0].slice(c1 - 1)
  return `${lines.join(' ').replace(/\s+/g, ' ')} (${module} line ${l1})`
}

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
// variables that changed from the state before. loopStart marks where the
// cycle of a liveness counterexample begins.
function describeTrace(trace: TraceStep[], loopStart?: number): string[] {
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
    const loop = i === loopStart ? '  <- loop starts here' : ''
    return `${i + 1}. ${label}: ${changed.map((k) => `${k} = ${cur[k]}`).join(', ')}${loop}`
  })
}

// checkModel checks one Model and, when its spec has a view, that its committed
// chart is current. A model tla-ts cannot check, such as one whose config names
// a rule that depends on no variable, is reported as failed with its error
// rather than thrown. onProgress receives the search's progress every 60 s.
export async function checkModel(
  model: Model,
  shared = sharedModulesDir(),
  workers = 1,
  onProgress?: (progress: Progress) => void,
): Promise<ModelResult> {
  const r = await checkSource(
    model,
    fs.readFileSync(model.spec, 'utf8'),
    shared,
    workers,
    onProgress,
  )
  const stale = await staleChart(model)
  if (stale === undefined) return r
  return {
    ...r,
    ok: false,
    rule: r.rule ?? 'chart',
    error: r.error ? `${r.error}\n${stale}` : stale,
  }
}

// checkSource checks one Model's rules over source, the spec's text or a
// mutant of it.
async function checkSource(
  model: Model,
  source: string,
  shared: string,
  workers: number,
  onProgress?: (progress: Progress) => void,
): Promise<ModelResult> {
  const loadModule = moduleLoader(model.spec, shared)
  const start = performance.now()
  const empty = {
    states: 0,
    generated: 0,
    depth: 0,
    collision: { calculated: 0 },
  }
  try {
    const result = await check({
      source,
      config: fs.readFileSync(model.config, 'utf8'),
      loadModule,
      workers,
      onProgress: (p) => {
        if (p.kind === 'progress') onProgress?.(p.statistics as Progress)
      },
    })
    const r = {
      ...model,
      ms: performance.now() - start,
      status: result.status,
      states: result.distinctStates,
      generated: result.generatedStates,
      depth: result.depth,
      collision: collisionOf(
        result.distinctStates,
        result.generatedStates,
        result.fingerprintDistance,
      ),
    }
    if (result.status === 'success') return { ...r, ok: true }
    const v = result.violation
    return {
      ...r,
      ok: false,
      rule: v ? ruleName(v.name, source, loadModule) : result.status,
      trace: describeTrace(
        v?.trace ?? result.failure?.trace ?? [],
        v?.loopStart,
      ),
      error: v?.message ?? result.failure?.message,
    }
  } catch (err) {
    const error = explain(err instanceof Error ? err.message : String(err))
    const rule = constantRule.exec(error)?.[1]
    return {
      ...model,
      ...empty,
      ok: false,
      ms: performance.now() - start,
      status: rule ? 'invariant-violation' : 'error',
      rule: rule ?? 'error',
      error,
    }
  }
}

// Mutant is one entry of Foo.mutants.json: the spec with a guard removed.
export interface Mutant {
  name: string
  // replace lists [from, to] pairs applied to the spec's source in order. Each
  // from must occur exactly once, so a mutant cannot go stale silently.
  replace: [string, string][]
  // expect, when set, is text the broken rule's name must contain, such as an
  // invariant name, deadlock, or a module name for a refinement.
  expect?: string
  // config names the config to check, relative to the spec; the default is
  // Foo.cfg.
  config?: string
}

// MutantVerdict is kill when the mutant broke its expected rule, LIVE when it
// still passed, MISS when it broke another rule, and stale when its text no
// longer matches the spec. Only kill passes the run.
export type MutantVerdict = 'kill' | 'LIVE' | 'MISS' | 'stale'

// MutantResult is the outcome of checking one Mutant.
export interface MutantResult extends ModelResult {
  mutant: Mutant
  verdict: MutantVerdict
}

// mutantsOf returns the mutants listed beside spec, Foo.mutants.json for
// Foo.tla, or none.
export function mutantsOf(spec: string): Mutant[] {
  const file = spec.slice(0, -'.tla'.length) + '.mutants.json'
  return fs.existsSync(file)
    ? (JSON.parse(fs.readFileSync(file, 'utf8')) as Mutant[])
    : []
}

// applyMutant returns source with each of mutant's replacements applied, and
// throws when a from text does not occur exactly once.
function applyMutant(source: string, mutant: Mutant): string {
  let out = source
  for (const [from, to] of mutant.replace) {
    const n = out.split(from).length - 1
    if (n !== 1)
      throw new Error(`text occurs ${n} times, want 1: ${JSON.stringify(from)}`)
    out = out.replace(from, () => to)
  }
  return out
}

// checkMutant checks one mutant of spec. A parse or evaluation error checks
// nothing, so it never counts as a kill. onProgress receives the search's
// progress every 60 s.
export async function checkMutant(
  spec: string,
  mutant: Mutant,
  shared = sharedModulesDir(),
  workers = 1,
  onProgress?: (progress: Progress) => void,
): Promise<MutantResult> {
  const dir = path.dirname(spec)
  const config = path.join(
    dir,
    mutant.config ?? path.basename(spec).slice(0, -'.tla'.length) + '.cfg',
  )
  const model = { spec, config }
  let source: string
  try {
    source = applyMutant(fs.readFileSync(spec, 'utf8'), mutant)
  } catch (err) {
    return {
      ...model,
      ok: false,
      ms: 0,
      status: 'error',
      states: 0,
      generated: 0,
      depth: 0,
      collision: { calculated: 0 },
      rule: 'stale',
      error: err instanceof Error ? err.message : String(err),
      mutant,
      verdict: 'stale',
    }
  }
  const r = await checkSource(model, source, shared, workers, onProgress)
  const killed = !r.ok && r.status !== 'error'
  const verdict: MutantVerdict = !killed
    ? 'LIVE'
    : !mutant.expect || r.rule?.includes(mutant.expect)
      ? 'kill'
      : 'MISS'
  return { ...r, mutant, verdict }
}

const usage = `usage: aptre-tla [-json] [-workers N] [path...]
       aptre-tla chart [path...]

Checks every TLA+ spec with a .cfg beside it under each path (default: the
working directory), that the chart of each spec with a Foo.view.ts is current,
and that each mutant in Foo.mutants.json breaks its expected rule. Prints the
state counts of every result, its calculated and observed chance of a
fingerprint collision, and the shortest trace to each broken rule;
TLA_MUTANT_TRACES=1 also prints each killing trace. chart writes the charts.

A search that runs past a few seconds prints a progress line to stderr every
60 s: its states, depth, queue, memory and RSS. SIGINT and SIGTERM print the
last one before exiting. The checker spills its queue under TLA_SPILL (default:
the temporary directory) and the run removes it on exit; TLA_LIMIT_MIB stops
the run, with exit code 1, once RSS plus spilled bytes pass that many MiB.
Exits 1 when a model fails or a mutant survives, and 2 when the arguments are
wrong, such as a missing path or a spec file with no config beside it. A
directory with no models passes.`

// usageError prints message and the usage, and returns the exit code for a
// usage error.
function usageError(message: string): number {
  console.error(`aptre-tla: ${message}\n\n${usage}`)
  return 2
}

// formatCollision shows the calculated estimate, then the observed one when
// the checker measured it.
function formatCollision(c: Collision): string {
  const e = (p: number) => p.toExponential(1)
  return c.observed === undefined
    ? e(c.calculated)
    : `${e(c.calculated)}/${e(c.observed)}`
}

// printResult prints one result as a line, followed by its error and trace
// when it failed, or as one JSON object.
function printResult(
  json: boolean,
  r: ModelResult | MutantResult,
  name: string,
): void {
  const mutant = 'verdict' in r
  if (json) {
    console.log(
      JSON.stringify({ kind: mutant ? 'mutant' : 'model', name, ...r }),
    )
    return
  }
  const verdict = mutant ? r.verdict : r.ok ? 'pass' : 'FAIL'
  const stats = `${String(r.states).padStart(7)} states ${String(r.depth).padStart(3)} deep ${r.ms.toFixed(0).padStart(6)} ms  collision ${formatCollision(r.collision)}`
  console.log(
    `${verdict.padEnd(5)} ${stats}  ${name}${r.rule ? `  ${r.rule}` : ''}`,
  )
  const passed = mutant ? r.verdict === 'kill' : r.ok
  if (passed && !(mutant && process.env.TLA_MUTANT_TRACES)) return
  for (const line of r.error?.split('\n') ?? []) console.log(`      ${line}`)
  for (const line of r.trace ?? []) console.log(`      ${line}`)
}

// main checks every Model and mutant under the given paths, the working
// directory when none are given, and prints one line per result.
async function main(args: string[]): Promise<number> {
  if (args[0] === 'chart') return writeCharts(args.slice(1))
  let json = false
  let workers = 1
  const targets: string[] = []
  for (let i = 0; i < args.length; i++) {
    const a = args[i]
    if (a === '-h' || a === '-help' || a === '--help') {
      console.log(usage)
      return 0
    }
    if (a === '-json') json = true
    else if (a === '-workers') workers = Number(args[++i])
    else if (a.startsWith('-')) return usageError(`unknown flag ${a}`)
    else if (!fs.existsSync(a)) return usageError(`no such path: ${a}`)
    else targets.push(a)
  }
  if (!Number.isInteger(workers) || workers < 1)
    return usageError('-workers takes a positive integer')
  const limit = Number(process.env.TLA_LIMIT_MIB ?? Infinity)
  if (!(limit > 0)) return usageError('TLA_LIMIT_MIB takes a positive number')
  const shared = sharedModulesDir()
  const runner = startRunner(shared, workers, limit * 1024 * 1024)
  try {
    return await checkAll(runner, json, targets)
  } finally {
    await runner.close()
  }
}

// checkAll checks every Model and mutant under targets through runner and
// prints one line per result.
async function checkAll(
  runner: Runner,
  json: boolean,
  targets: string[],
): Promise<number> {
  let failed = 0
  let count = 0
  const specs = new Set<string>()
  for (const target of targets.length ? targets : ['.']) {
    let found = 0
    for (const model of findModels(target)) {
      found++
      count++
      specs.add(model.spec)
      const name = path.relative('.', model.config)
      const r = await runner.checkModel(model, name)
      printResult(json, r, name)
      if (!r.ok) failed++
    }
    if (found === 0 && fs.statSync(target).isFile())
      return usageError(`${target} has no Foo.cfg beside its Foo.tla`)
  }
  for (const spec of specs) {
    for (const mutant of mutantsOf(spec)) {
      const name = `${path.relative('.', spec)} mutant "${mutant.name}"`
      const r = await runner.checkMutant(spec, mutant, name)
      printResult(json, r, name)
      if (r.verdict !== 'kill') failed++
    }
  }
  if (!json) console.log(`${count} model(s), ${failed} failed`)
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
  isMainThread &&
  process.argv[1] &&
  fs.realpathSync(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  process.exitCode = await main(process.argv.slice(2))
}
