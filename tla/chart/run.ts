// Charting a spec checks it with tla-ts in two kinds of run.
//
// Verdict runs decide each rule as TLC would: one run checks every rule, and a
// rule that breaks is decided before the rest rerun, bounded once the first
// run did not reach every state.
//
// The chart run explores the reachable graph in a wrapper module, held in
// memory and served through loadModule. Its CONSTRAINT prints every state
// with the invariants it breaks, and its ACTION_CONSTRAINT prints every step
// with the names of the actions it satisfies, each conjoined with the
// config's own constraints. tla-ts evaluates the CONSTRAINT on every
// successor, before the ACTION_CONSTRAINT, so each step's states print before
// the step. A first wrapper run reads TLCGet("spec").actions, so the second
// can test each action by its source text.
//
// Under VIEW or SYMMETRY, tla-ts keeps one state for each class it counts as
// the same; the graph keeps the first state printed in each class, as TLC's
// state graph does.

import { randomBytes } from 'node:crypto'
import fs from 'node:fs'
import path from 'node:path'
import { inspect } from 'node:util'
import {
  type CheckResult as TlaResult,
  check,
  formatValue,
  parseModule,
  type TraceStep,
  type Value as TlaValue,
  type Violation,
} from 'tla-ts'
import { type Model, moduleLoader } from '../check.js'
import { readConfig } from './frontends/config.js'
import { parseValue } from './frontends/value.js'
import { checkView } from './frontends/view.js'
import {
  type Bug,
  CHART_STATES,
  type CheckResult,
  type Config,
  type ConfigSection,
  type Graph,
  hasSection,
  type Lasso,
  key,
  type Mark,
  must,
  type Outcome,
  type Property,
  RERUN_STATES,
  type Rule,
  ruleKey,
  type Spec,
  stateKey,
  stateRules,
  type Stop,
  type Trace,
  type TraceState,
  type Value,
  type Vars,
  type Verdict,
} from './ir.js'
import { compile } from './pipeline.js'
import { addPast, deadEnds, lasso, mark } from './passes/annotate.js'
import { pathGraph } from './passes/paths.js'

type Load = (name: string) => string | undefined

type Tuple = Extract<Value, { kind: 'function' }>

// Halt is the Stop of a run that could not finish, with the trace to the
// state it stopped in when tla-ts gives one.
type Halt = Stop & { trace?: TraceStep[] }

// Run is a finished tla-ts run, or the Halt of one that could not finish.
type Run = TlaResult | Halt

const stopped = (r: Run): r is Halt => 'message' in r

// run checks source under config with the pinned options of a TLC run: one
// worker, so states print in one order, and liveness checked at the end.
async function run(
  source: string,
  config: string,
  loadModule: Load,
  onPrint?: (text: string) => void,
): Promise<Run> {
  try {
    const r = await check({
      source,
      config,
      loadModule,
      onPrint,
      workers: 1,
      livenessCheck: 'final',
    })
    if (r.status === 'success' || r.violation) return r
    return {
      status: r.failure?.exitCode ?? null,
      message: r.failure?.message ?? r.status,
      trace: r.failure?.trace,
    }
  } catch (err) {
    return {
      status: null,
      message: err instanceof Error ? err.message : String(err),
    }
  }
}

const exitClass = (r: Run): 0 | 1 | 2 =>
  stopped(r) ? 2 : r.status === 'success' ? 0 : 1

const configText = (sections: ConfigSection[], extra: string[]) => {
  const lines: string[] = []
  for (const s of sections) lines.push(`${s.key} ${s.tokens.join(' ')}`)
  lines.push(...extra)
  return lines.join('\n') + '\n'
}

function readChecks(file: string, config: Config) {
  const refuse = (words: string) =>
    new Error(`TLC refuses the config ${file}. ${words}`)
  if (config.spec && (config.init || config.next))
    throw refuse(
      'The configuration file cannot specify both INIT/NEXT and SPECIFICATION fields.',
    )
  if (!config.spec && !config.init)
    throw refuse(
      'The configuration file did not specify the initial state predicate.',
    )
  const written: ConfigSection[] = []
  for (const s of config.sections)
    if (!['INVARIANT', 'PROPERTY', 'CHECK_DEADLOCK'].includes(s.key))
      written.push(s)
  return {
    invariants: config.invariants,
    properties: config.properties,
    deadlock: config.checkDeadlock !== false,
    written,
  }
}

// variablesOf lists the module's variables in TLC's order: those of the
// modules it extends first, then its own.
function variablesOf(load: Load, name: string, seen = new Set<string>()) {
  const found: string[] = []
  const src = seen.has(name) ? undefined : load(name)
  seen.add(name)
  if (!src) return found
  const m = parseModule(src)
  for (const e of m.extends) found.push(...variablesOf(load, e, seen))
  for (const v of m.variables) found.push(v.name)
  return found
}

const TEMPORAL = new Set([
  'Always',
  'Eventually',
  'BoxAction',
  'AngleAction',
  'WeakFairness',
  'StrongFairness',
  'Prime',
])

// initialOnlyProperties names the properties that TLC checks only in the
// initial states: a constant, or a definition that reaches no temporal
// operator and no prime through the definitions it uses.
function initialOnlyProperties(
  load: Load,
  { module, written, properties }: Spec,
) {
  const definitions = new Map<string, unknown>()
  const constants = new Set<string>()
  const read = (name: string, root: boolean, seen: Set<string>) => {
    const src = seen.has(name) ? undefined : load(name)
    seen.add(name)
    if (!src) return
    const m = parseModule(src)
    for (const e of m.extends) read(e, false, seen)
    for (const c of m.constants) constants.add(c.name)
    for (const u of m.units)
      if (u.kind === 'Definition' && (root || !u.definition.isLocal))
        definitions.set(u.definition.name, u.definition.body)
  }
  read(module, true, new Set())
  const replaced = new Map<string, string>()
  for (const s of written)
    if (s.key === 'CONSTANT')
      for (let i = 1; i + 1 < s.tokens.length; i++)
        if (s.tokens[i] === '<-') replaced.set(s.tokens[i - 1], s.tokens[i + 1])
  const temporal = (node: unknown, seen: Set<string>): boolean => {
    if (Array.isArray(node)) return node.some((n) => temporal(n, seen))
    if (typeof node !== 'object' || node === null) return false
    const n = node as { kind?: string; name?: string; operator?: string }
    if (n.kind && TEMPORAL.has(n.kind)) return true
    if (n.kind === 'Apply' && n.operator === '~>') return true
    for (const used of [n.name, n.operator])
      if (used && definitions.has(used) && !seen.has(used)) {
        seen.add(used)
        if (temporal(definitions.get(used), seen)) return true
      }
    return Object.values(node).some((v) => temporal(v, seen))
  }
  const names = new Set<string>()
  for (const name of properties) {
    const target = replaced.get(name) ?? name
    if (constants.has(target)) names.add(name)
    else if (
      definitions.has(target) &&
      !temporal(definitions.get(target), new Set([target]))
    )
      names.add(name)
  }
  return names
}

// sourceRange matches the source position tla-ts appends to an action label.
const sourceRange = / line \d+, col \d+ to line \d+, col \d+ of module \S+$/

const actionName = (label: string | undefined) =>
  (label ?? '').replace(sourceRange, '')

// varsOf converts a tla-ts state, naming its variables in order of name. TLC
// prints them in the order of its internal string table, which is usually,
// but not always, the same.
const varsOf = (state: Record<string, TlaValue>) => {
  const vars: Vars = {}
  for (const name of Object.keys(state).sort())
    vars[name] = parseValue(formatValue(state[name]))
  return vars
}

// traceOf reads a violation as TLC prints it: the initial state with no
// action, each step's action, and for a lasso either a last step back to the
// loop's first state or, when the run stutters, a loop at its last state.
function traceOf({ trace, loopStart, loopLabel }: Violation) {
  const seq: TraceState[] = []
  for (const [i, step] of trace.entries())
    seq.push({
      action: i ? actionName(step.label ?? step.action) : '',
      vars: varsOf(step.state),
    })
  if (loopStart === undefined) return { seq, loop: undefined }
  if (loopLabel === undefined) return { seq, loop: seq.length - 1 }
  seq.push({ action: actionName(loopLabel), vars: seq[loopStart].vars })
  return { seq, loop: loopStart }
}

const brokenRule = (group: Rule[], r: TlaResult) => {
  const name = r.violation?.name
  const kinds: Rule['kind'][] =
    r.status === 'deadlock'
      ? ['deadlock']
      : r.status === 'invariant-violation'
        ? ['invariant', 'property']
        : ['property']
  for (const kind of kinds)
    for (const rule of group)
      if (rule.kind === kind && (kind === 'deadlock' || rule.name === name))
        return rule
  return undefined
}

const names = (rules: Rule[], kind: Rule['kind']) => {
  const found: string[] = []
  for (const rule of rules) if (rule.kind === kind) found.push(rule.name)
  return found
}

const wrapper = (name: string, module: string, lines: string[]) =>
  [
    `---- MODULE ${name} ----`,
    `EXTENDS ${module}`,
    'ChartTLC == INSTANCE TLC',
    'ChartNaturals == INSTANCE Naturals',
    ...lines,
    '====',
  ].join('\n')

const field = (record: Value, name: string) => {
  const { keys, values } = record as Tuple
  return values[keys.indexOf(name)]
}

const sliceOf = (src: string, location: Value) => {
  const at = (k: string) => field(location, k) as number
  const lines = src.split('\n').slice(at('beginLine') - 1, at('endLine'))
  lines[lines.length - 1] = lines[lines.length - 1].slice(0, at('endColumn'))
  // Pad the first line to its column, so junction lists keep their alignment.
  lines[0] =
    ' '.repeat(at('beginColumn') - 1) + lines[0].slice(at('beginColumn') - 1)
  return lines.join('\n')
}

// recordText writes a record of strings and numbers, such as an action's
// location, as a TLA+ record literal.
const recordText = (record: Value) => {
  const { keys, values } = record as Tuple
  return `[${keys.map((k, i) => `${String(k)} |-> ${JSON.stringify(values[i])}`).join(', ')}]`
}

// actionTests defines one predicate per action TLC names, grouped by name and
// location: an action defined with parameters binds them from its context.
function actionTests(load: Load, actions: Value[]) {
  const groups = new Map<
    string,
    { name: string; location: Value; context: string[] }
  >()
  for (const a of actions) {
    const name = field(a, 'name') as string
    const location = field(a, 'location')
    const k = key(name) + key(location)
    const context = field(a, 'context') as Tuple | undefined
    if (!groups.has(k))
      groups.set(k, {
        name,
        location,
        context: context?.keys.map(String) ?? [],
      })
  }
  const defs: string[] = []
  const tests: string[] = []
  for (const [i, g] of [...groups.values()].entries()) {
    const src = load(field(g.location, 'module') as string)
    if (src === undefined)
      throw new Error(`tla-ts names an action in a module it cannot load.`)
    // An action from an INSTANCE is located at the INSTANCE; its name is the
    // expression that tests it.
    const body = g.name.includes('!') ? g.name : sliceOf(src, g.location)
    if (g.context.length) {
      defs.push(
        `ChartG${i} == {chartAction \\in ChartTLC!TLCGet("spec").actions : chartAction.name = ${JSON.stringify(g.name)} /\\ chartAction.location = ${recordText(g.location)}}`,
        `ChartA${i} ==\n  \\E chartAction \\in ChartG${i} :\n  LET ${g.context.map((k) => `${k} == chartAction.context.${k}`).join('\n      ')}\n  IN\n${body}`,
      )
    } else defs.push(`ChartA${i} ==\n${body}`)
    tests.push(`(IF ChartA${i} THEN {${JSON.stringify(g.name)}} ELSE {})`)
  }
  return { defs, tests }
}

const permute = (v: Value, perm: Map<string, Value>): Value => {
  if (typeof v !== 'object') return v
  switch (v.kind) {
    case 'model':
      return perm.get(v.name) ?? v
    case 'interval':
      return v
    case 'set':
      return { kind: 'set', members: v.members.map((m) => permute(m, perm)) }
    case 'function':
      return {
        ...v,
        keys: v.keys.map((k) => permute(k, perm)),
        values: v.values.map((m) => permute(m, perm)),
      }
  }
}

// symmetryKey is the least state key over the permutations, so states that
// a permutation maps onto each other share it.
const symmetryKey = (perms: Map<string, Value>[]) => (values: Value[]) => {
  let least = stateKey(values)
  for (const perm of perms) {
    const k = stateKey(values.map((v) => permute(v, perm)))
    if (k < least) least = k
  }
  return least
}

type Drawn = {
  graph: Graph
  marks: Mark[]
  initial: number
  distinct: number
  drawing: { status: number | null } | undefined
  find: ((values: Value[]) => string | undefined) & {
    add: (id: string, values: Value[]) => Map<string, string> | undefined
  }
}

// drawGraph runs the two wrapper runs and builds the state graph.
async function drawGraph(
  spec: Spec,
  config: Config,
  load: Load,
  extra: Map<string, string>,
): Promise<Drawn> {
  const tag = `chart-${randomBytes(16).toString('hex')}`
  const tagged = new RegExp(`^<<\\s*"${tag}"`)
  const drawn: ConfigSection[] = []
  for (const s of spec.written)
    if (s.key !== 'CONSTRAINT' && s.key !== 'ACTION_CONSTRAINT') drawn.push(s)
  extra.set(
    'ChartSpec',
    wrapper('ChartSpec', spec.module, [
      `ChartSpec == ChartTLC!PrintT(<<"${tag}", ChartTLC!TLCGet("spec").actions, ${config.symmetry ?? '{}'}>>) /\\ FALSE`,
    ]),
  )
  let actions: Value[] = []
  let perms: Map<string, Value>[] = []
  const first = await run(
    must(extra, 'ChartSpec'),
    configText(drawn, ['CONSTRAINT ChartSpec', 'CHECK_DEADLOCK FALSE']),
    load,
    (text) => {
      if (!tagged.test(text)) return
      const [, a, p] = (parseValue(text) as Tuple).values
      actions = (a as { members: Value[] }).members
      perms = []
      for (const f of (p as { members: Value[] }).members) {
        const perm = new Map<string, Value>()
        const { keys, values } = f as Tuple
        for (let i = 0; i < keys.length; i++)
          perm.set((keys[i] as { name: string }).name, values[i])
        perms.push(perm)
      }
    },
  )
  if (stopped(first))
    throw new Error(
      `TLC could not list the actions of ${spec.module} (exit ${first.status}):\n${first.message}`,
    )
  const { defs, tests } = actionTests(load, actions)
  const both = (list: string[]) => (list.length ? list.join(' /\\ ') : 'TRUE')
  const inModel = both(config.constraints)
  const broken: string[] = []
  for (const name of spec.invariants)
    broken.push(`(IF ~${name} THEN {${JSON.stringify(name)}} ELSE {})`)
  const vars = `<<${spec.variables.join(', ')}>>`
  extra.set(
    'ChartRun',
    wrapper('ChartRun', spec.module, [
      ...defs,
      `ChartBroken == ${[...broken, '{}'].join(' \\cup ')}`,
      `ChartMark == ChartTLC!PrintT(<<"${tag}", "s", ChartTLC!TLCGet("level"), ${vars}, ${config.view ?? 'TRUE'}, ${inModel}>>) /\\ ChartTLC!PrintT(<<"${tag}", "b", ${vars}, ChartBroken>>) /\\ ${inModel} /\\ ChartNaturals!<=(ChartTLC!TLCGet("distinct"), ${CHART_STATES})`,
      `ChartStep == ${both(config.actionConstraints)} /\\ ChartTLC!PrintT(<<"${tag}", "t", ${vars}, ${vars}', ${[...tests, '{}'].join(' \\cup ')}>>)`,
    ]),
  )
  const classOf = config.view
    ? (_: Value[], view: Value) => `view ${key(view)}`
    : perms.length
      ? symmetryKey(perms)
      : (values: Value[]) => stateKey(values)
  const printed = [...spec.variables].sort()
  type Class = { vars: Vars; inModel: boolean; id?: string }
  const classes = new Map<string, Class>()
  const classByState = new Map<string, string>()
  const graph: Graph = { states: new Map(), steps: [] }
  const marks: Mark[] = []
  const taken = new Set<string>()
  const judged = new Set<string>()
  const initial = new Set<string>()
  const place = (c: Class) => {
    if (c.id === undefined) {
      c.id = String(graph.states.size)
      graph.states.set(c.id, { vars: c.vars, start: false, breaks: [] })
    }
    return c.id
  }
  const step = (source: Class, target: Class, action: string) => {
    const [f, t] = [place(source), place(target)]
    const k = `${f} ${t} ${action}`
    if (taken.has(k)) return
    taken.add(k)
    graph.steps.push({ from: f, to: t, action })
  }
  const classAt = (tuple: Value) =>
    classes.get(must(classByState, stateKey((tuple as Tuple).values)))
  const second = await run(
    must(extra, 'ChartRun'),
    configText(drawn, [
      'CONSTRAINT ChartMark',
      'ACTION_CONSTRAINT ChartStep',
      'CHECK_DEADLOCK FALSE',
    ]),
    load,
    (text) => {
      if (!tagged.test(text)) return
      const [, kind, ...rest] = (parseValue(text) as Tuple).values
      if (kind === 'b') {
        const [tuple, rules] = rest
        const values = (tuple as Tuple).values
        const ck = must(classByState, stateKey(values))
        if (judged.has(ck)) return
        judged.add(ck)
        const names: string[] = []
        for (const r of (rules as { members: Value[] }).members)
          names.push(r as string)
        if (names.length) marks.push({ values, broken: names })
        return
      }
      if (kind === 's') {
        const [level, tuple, view, kept] = rest
        const values = (tuple as Tuple).values
        const k = stateKey(values)
        let ck = classByState.get(k)
        if (ck === undefined) classByState.set(k, (ck = classOf(values, view)))
        let c = classes.get(ck)
        if (!c) {
          const vars: Vars = {}
          for (const name of printed)
            vars[name] = values[spec.variables.indexOf(name)]
          classes.set(ck, (c = { vars, inModel: kept === true }))
        }
        if (level === 1) {
          initial.add(ck)
          if (c.inModel) must(graph.states, place(c)).start = true
        }
        return
      }
      const [from, to, named] = rest
      const target = classAt(to)
      if (!target?.inModel) return
      const source = must(
        classes,
        must(classByState, stateKey((from as Tuple).values)),
      )
      for (const action of (named as { members: Value[] }).members)
        step(source, target, action as string)
    },
  )
  // A rule that fails to evaluate stops the run in the successor's
  // CONSTRAINT, before the step into it prints; the failure's trace ends with
  // that step.
  const last = stopped(second) ? (second.trace ?? []).slice(-2) : []
  if (last.length === 2) {
    const classOfStep = ({ state }: TraceStep) =>
      classes.get(
        classByState.get(
          stateKey(
            spec.variables.map((n) => parseValue(formatValue(state[n]))),
          ),
        ) ?? '',
      )
    const [source, target] = last.map(classOfStep)
    if (source && target?.inModel)
      step(source, target, actionName(last[1].action))
  }
  const ids = new Map<string, string>()
  const classKeyOf = (values: Value[]) => {
    const k = stateKey(values)
    return classByState.get(k) ?? k
  }
  const find = Object.assign(
    (values: Value[]) => {
      const ck = classKeyOf(values)
      return ids.get(ck) ?? classes.get(ck)?.id
    },
    { add: (id: string, values: Value[]) => ids.set(classKeyOf(values), id) },
  )
  return {
    graph,
    marks,
    initial: initial.size,
    distinct: stopped(second) ? graph.states.size : second.distinctStates,
    drawing: stopped(second) ? { status: second.status } : undefined,
    find,
  }
}

function* only(marks: Iterable<Mark>, rules: Set<string>) {
  for (const { values, broken } of marks) {
    const kept: string[] = []
    for (const name of broken) if (rules.has(name)) kept.push(name)
    if (kept.length) yield { values, broken: kept }
  }
}

// checkSpec checks model with tla-ts and builds the CheckResult that
// statechart's passes compile into a chart and a report.
export async function checkSpec(
  model: Model,
  load: Load = moduleLoader(model.spec),
): Promise<CheckResult> {
  const file = model.config
  const config = readConfig({ file, text: fs.readFileSync(file, 'utf8') })
  const checks = readChecks(file, config)
  const module = path.basename(model.spec, '.tla')
  const extra = new Map<string, string>()
  const loadModule: Load = (name) => extra.get(name) ?? load(name)
  const spec = {
    module,
    variables: variablesOf(loadModule, module),
    ...checks,
    folder: path.dirname(path.resolve(model.spec)),
  }
  const source = fs.readFileSync(model.spec, 'utf8')
  extra.set(
    'ChartBound',
    wrapper('ChartBound', module, [
      `ChartBound == ChartNaturals!<(ChartTLC!TLCGet("distinct"), ${RERUN_STATES})`,
    ]),
  )
  const rules = stateRules(spec)
  const all = [...rules]
  for (const name of spec.properties) all.push({ kind: 'property', name })
  const verdicts = new Map<string, Verdict>()
  const decide = (rule: Rule, outcome: Outcome, trace?: Trace) =>
    verdicts.set(ruleKey(rule), { rule, outcome, trace })

  // Judge every rule.
  const groups = [all]
  let first: Run | undefined
  let stop: Stop | undefined
  const states = { distinct: 0, initial: 0, complete: false, cut: false }
  while (groups.length) {
    const group: Rule[] = []
    for (const rule of groups[0])
      if (!verdicts.has(ruleKey(rule))) group.push(rule)
    if (first && !group.length) {
      groups.shift()
      continue
    }
    const lines: string[] = []
    const invariants = names(group, 'invariant')
    const properties = names(group, 'property')
    if (invariants.length) lines.push(`INVARIANT ${invariants.join(' ')}`)
    if (properties.length) lines.push(`PROPERTY ${properties.join(' ')}`)
    if (!names(group, 'deadlock').length) lines.push('CHECK_DEADLOCK FALSE')
    const bounded = first !== undefined && !states.complete
    if (bounded) lines.push('CONSTRAINT ChartBound')
    const r = await run(
      bounded ? must(extra, 'ChartBound') : source,
      configText(spec.written, lines),
      loadModule,
    )
    first ??= r
    if (stopped(r)) {
      stop = r
      break
    }
    states.distinct = Math.max(states.distinct, r.distinctStates)
    const rule = brokenRule(group, r)
    const cut = bounded && r.distinctStates >= RERUN_STATES
    const liveness = r.status === 'liveness-violation'
    if (cut && (r.status === 'success' || liveness)) {
      states.cut = true
      for (const rule of group) decide(rule, 'not checked')
    } else if (rule) {
      decide(rule, 'broken', r.violation && traceOf(r.violation))
      // Liveness is checked after every state is reached, so the rest held.
      if (liveness) {
        states.complete = true
        for (const rule of group)
          if (rule.kind !== 'property' && !verdicts.has(ruleKey(rule)))
            decide(rule, 'holds')
      }
    } else if (r.status === 'success') {
      states.complete = true
      for (const rule of group) decide(rule, 'holds')
    } else
      throw new Error(
        `TLC reported ${r.status} on ${model.spec}, but names no rule in ${group.map((g) => g.name).join(', ')}.`,
      )
  }
  for (const rule of all)
    if (!verdicts.has(ruleKey(rule))) decide(rule, 'not checked')
  const exit = exitClass(first!)
  if (stopped(first!))
    throw new Error(
      `TLC could not check ${model.spec} (exit ${first.status}):\n${first.message}`,
    )

  const initialOnly = initialOnlyProperties(loadModule, spec)
  const propertiesWith = (bugOf: (trace: Trace) => Lasso | undefined) => {
    const found: Property[] = []
    for (const rule of all) {
      if (rule.kind !== 'property') continue
      const v = must(verdicts, ruleKey(rule))
      found.push({
        ...v,
        bug: v.outcome === 'broken' && v.trace ? bugOf(v.trace) : undefined,
        initialOnly: initialOnly.has(rule.name),
      })
    }
    return found
  }
  const checked: Verdict[] = []
  for (const rule of rules) checked.push(must(verdicts, ruleKey(rule)))
  const common = { spec, exit, stop, states, rules: checked }

  if (states.distinct <= CHART_STATES) {
    const d = await drawGraph(spec, config, loadModule, extra)
    states.initial = d.initial
    if (d.distinct <= CHART_STATES) {
      const { graph, find, drawing } = d
      const broken = new Set<string>()
      for (const { rule, outcome } of verdicts.values())
        if (rule.kind === 'invariant' && outcome === 'broken')
          broken.add(rule.name)
      const past = mark(
        graph,
        only(d.marks, broken),
        spec.variables,
        rules,
        find,
      )
      const excludes = hasSection(spec, 'ACTION_CONSTRAINT')
      for (const rule of rules) {
        const { outcome, trace } = must(verdicts, ruleKey(rule))
        if (
          rule.kind === 'deadlock' &&
          outcome === 'broken' &&
          !drawing &&
          !hasSection(spec, 'CONSTRAINT', 'ACTION_CONSTRAINT')
        )
          deadEnds(graph, rule)
        if (outcome !== 'broken' || !trace) continue
        const seq = trace.seq as TraceState[]
        if (
          !drawing ||
          lasso(graph, { seq, loop: undefined }, spec.variables, find)
        )
          addPast(graph, seq, spec.variables, rule, past, excludes, find)
      }
      return {
        ...common,
        graph,
        paths: undefined,
        properties: propertiesWith((t) =>
          lasso(graph, t, spec.variables, find),
        ),
        drawing,
      }
    }
  }
  const drawn = pathGraph(spec.variables)
  const paths: Bug[] = []
  for (const rule of rules) {
    const { outcome, trace } = must(verdicts, ruleKey(rule))
    if (outcome === 'broken' && trace)
      paths.push(drawn.bug(rule, trace.seq as TraceState[]))
  }
  return {
    ...common,
    graph: drawn.graph,
    paths,
    properties: propertiesWith(drawn.lasso),
    drawing: undefined,
  }
}

// chartModel checks model and compiles its chart, when it has one, and its
// report, in the words of view: a module with statechart's state, step,
// group, colors and title exports, or none for the default labels.
export async function chartModel(
  model: Model,
  view: { readonly [name: string]: unknown } = {},
  load?: Load,
) {
  return compile(await checkSpec(model, load), checkView(view, inspect))
}
