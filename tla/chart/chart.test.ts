import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { chartModel } from './run.js'

const corpus = path.join(
  path.dirname(fileURLToPath(import.meta.url)),
  'testdata',
)

// Difference is how one model's committed chart or report, drawn by statechart
// on TLC, may differ from the tla-ts port, and why.
type Difference = {
  // rewrite turns TLC's text into what tla-ts prints.
  rewrite?: [RegExp, string][]
  // order compares lines as a set: TLC prints a state's variables in the
  // order of its internal string table, which sets the order of states with
  // equal depth, and the port sorts them by name.
  order?: boolean
  // until compares the report only up to this text, where TLC's own error
  // output starts.
  until?: string
}

// The port names its own counterexamples, which statechart left in files.
const tlcFiles: [RegExp, string] = [/is in tlc-\S+\.out\. It has/g, 'has']

const differences: Record<string, Difference> = {
  // tla-ts gives its own exit codes and error text for a rule that fails to
  // evaluate.
  CounterGuard: { until: '## TypeOK breaks' },
  TypeGuard: {
    rewrite: [[/exit 76/g, 'exit 75']],
    until: '## Rules not checked',
  },
  // tla-ts names an action defined as an instanced operator by its own name.
  CounterRules: { rewrite: [[/M!Jump/g, 'Jump']] },
  HiddenClock: {
    rewrite: [[/x=(\d) clock=(\d)/g, 'clock=$2 x=$1']],
  },
  MediaPlayer: { order: true },
  MediaPlayerFixed: { order: true },
  MediaPlayerProps: { order: true },
}

const compared = (text: string, d: Difference, oracle: boolean) => {
  let out = text.trimEnd()
  if (oracle)
    for (const [from, to] of [tlcFiles, ...(d.rewrite ?? [])])
      out = out.replace(from, to)
  if (d.until !== undefined) out = out.slice(0, out.indexOf(d.until))
  return d.order ? out.split('\n').sort() : out.split('\n')
}

describe('chartModel', () => {
  it('draws the statechart corpus as statechart did on TLC', async () => {
    const drawn: string[] = []
    for (const name of fs.readdirSync(corpus).sort()) {
      const dir = path.join(corpus, name)
      const viewFile = path.join(dir, 'view.mjs')
      const view = fs.existsSync(viewFile)
        ? { ...(await import(viewFile)) }
        : {}
      const model = {
        spec: path.join(dir, `${name}.tla`),
        config: path.join(dir, `${name}.cfg`),
      }
      // A CONSTANT listed as a PROPERTY: TLC checks it, and tla-ts, like
      // check:tla, refuses the config.
      if (name === 'Shadow') {
        await expect(chartModel(model, view)).rejects.toThrow(
          /property Hold\s[^]*\snot defined/,
        )
        continue
      }
      const t0 = performance.now()
      const { chart, report } = await chartModel(model, view)
      expect(performance.now() - t0, name).toBeLessThan(2000)
      const d = differences[name] ?? {}
      const want = (file: string) =>
        fs.readFileSync(path.join(dir, file), 'utf8')
      if (fs.existsSync(path.join(dir, 'chart.mmd')))
        expect(compared(chart ?? '', d, false), name).toEqual(
          compared(want('chart.mmd'), d, true),
        )
      else expect(chart, name).toBeUndefined()
      expect(compared(report, d, false), name).toEqual(
        compared(want('report.md'), d, true),
      )
      drawn.push(name)
    }
    expect(drawn).toHaveLength(32)
  })
})
