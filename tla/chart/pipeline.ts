import { mermaid } from './backends/mermaid.js'
import { report } from './backends/report.js'
import {
  type Answers,
  type Bug,
  type CheckedView,
  type CheckResult,
  type Graph,
  type Story,
  type View,
  type Walk,
} from './ir.js'
import { asParents, ask, labelGraph, storiesOf } from './passes/label.js'
import { lift } from './passes/lift.js'
import { loopCheck } from './passes/loop-check.js'
import { bugs, drawOrder, walk } from './passes/reach.js'
import { regionsLie } from './passes/region-check.js'
import { drawRegions } from './passes/regions.js'

const shortestFirst = (a: { path: unknown[] }, b: { path: unknown[] }) =>
  a.path.length - b.path.length

export function project(graph: Graph, walked: Walk, view: View, found: Bug[]) {
  const { order, rank } = drawOrder(walked)
  const draw = (answers: Answers) => {
    const labels = labelGraph(graph, walked, answers, rank, found)
    return { labels, chart: drawRegions(walked, labels, order, found, view) }
  }
  const answers = ask(graph, walked, view)
  const first = draw(answers)
  const lie = regionsLie(first.chart.regions, first.labels, walked, order)
  const { labels, chart } = lie ? draw(asParents(walked, answers)) : first
  loopCheck(chart, labels, walked, found)
  return {
    chart: lift(lie ? { ...chart, lie } : chart),
    stories: storiesOf(graph, found, walked, first.labels),
  }
}

// compile turns a checked model into its Mermaid chart, when it has one, and
// its report.
export function compile(result: CheckResult, view: CheckedView) {
  const walked = walk(result.graph)
  const found = [...(result.paths ?? bugs(result.graph, walked))].sort(
    shortestFirst,
  )
  for (const p of result.properties)
    if (p.bug) found.push({ rule: p.rule, ...p.bug })
  const { chart, stories } =
    found.length || !result.paths
      ? project(result.graph, walked, view, found)
      : { chart: undefined, stories: new Map<Bug, Story>() }
  return {
    chart: chart && `${mermaid(chart)}\n`,
    report: report(result, found, chart, stories),
    exit: result.exit,
  }
}
