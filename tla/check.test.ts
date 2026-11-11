import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import {
  chartOf,
  checkModel,
  checkMutant,
  configMutant,
  configMutantsOf,
  drawChart,
  findConfigMutants,
  findModels,
  mutantsOf,
} from './check.js'

const testdata = path.join(
  path.dirname(fileURLToPath(import.meta.url)),
  'testdata',
)

describe('aptre-tla', () => {
  it('passes the working designs and fails the broken ones with their rule', async () => {
    const results: Record<
      string,
      { ok: boolean; rule?: string; trace?: string[]; error?: string }
    > = {}
    for (const model of findModels(testdata)) {
      const r = await checkModel(model)
      expect(r.ms).toBeLessThan(1000)
      results[path.basename(model.config)] = r
    }
    expect(Object.keys(results).sort()).toEqual([
      'ChallengeCard-private.cfg',
      'ChallengeCard-shared-buttons.cfg',
      'ChallengeCard-shared.cfg',
      'Counter.cfg',
      'MapPicker-all.cfg',
      'MapPicker-rule.cfg',
      'Reach-stalled.cfg',
      'Reach.cfg',
      'Worker.cfg',
    ])
    // A passing model reports the size of its search.
    expect(results['Worker.cfg']).toMatchObject({
      ok: true,
      states: 580,
      depth: 16,
    })
    expect(results['ChallengeCard-private.cfg'].ok).toBe(true)
    // A _POSSIBLE goal passes when some state reaches it and fails when none
    // does; both charts draw every reachable state.
    expect(results['Reach.cfg'].ok).toBe(true)
    expect(results['Reach-stalled.cfg']).toMatchObject({
      ok: false,
      rule: 'Shipped',
    })
    expect(results['Reach-stalled.cfg'].error).not.toMatch(/could not draw/)
    expect(results['MapPicker-rule.cfg'].ok).toBe(true)
    // The shipped card: Mira, the challenger, presses Accept and is refused.
    expect(results['ChallengeCard-shared-buttons.cfg']).toMatchObject({
      ok: false,
      rule: 'EveryButtonWorks',
      trace: [
        '1. Init: refused = {}, challenge = "open"',
        '2. Press("mira","accept"): refused = {<<"mira", "accept">>}',
      ],
    })
    expect(results['ChallengeCard-shared.cfg']).toMatchObject({
      ok: false,
      rule: 'ChallengerCanWithdraw',
    })
    // A rule on constants alone throws in tla-ts; it still names the rule, and
    // Surface resolves from the shared modules.
    expect(results['MapPicker-all.cfg']).toMatchObject({
      ok: false,
      rule: 'EveryShownActionWorks',
    })
    // A spec that uses < without EXTENDS Naturals is told which module to add.
    expect(results['Counter.cfg'].error).toMatch(/\(add EXTENDS Naturals\)$/)
  })

  it('kills each listed mutant and fails one that survives or goes stale', async () => {
    const spec = path.join(testdata, 'worker', 'Worker.tla')
    const verdicts: Record<string, string> = {}
    for (const mutant of mutantsOf(spec)) {
      const r = await checkMutant(spec, mutant)
      verdicts[mutant.name] = `${r.verdict} ${r.rule}`
    }
    // A refinement failure shows the formula and the interface it broke,
    // rather than only its source range.
    expect(verdicts).toEqual({
      'take checks the queue and stops in two lock sections':
        'kill [][Next]_vars (WorkerApi line 52)',
      'Submit accepts after Stop': 'kill [][Next]_vars (WorkerApi line 52)',
      'Submit does not signal': 'kill AcceptedRuns',
      'Stop does not signal': 'kill deadlock',
    })
    const live = await checkMutant(spec, {
      name: 'no change',
      replace: [['EXTENDS Sequences', 'EXTENDS Sequences']],
    })
    expect(live.verdict).toBe('LIVE')
    const miss = await checkMutant(spec, {
      ...mutantsOf(spec)[3],
      expect: 'AcceptedRuns',
    })
    expect(miss.verdict).toBe('MISS')
    const stale = await checkMutant(spec, {
      name: 'gone',
      replace: [['no such text', '']],
    })
    expect(stale).toMatchObject({ verdict: 'stale', rule: 'stale' })
  })

  it('judges each config mutant in testdata by its expect line', async () => {
    const spec = path.join(testdata, 'worker', 'Worker.tla')
    const verdicts: Record<string, string | undefined> = {}
    for (const model of configMutantsOf(spec)) {
      const mutant = configMutant(model)
      verdicts[mutant.name] =
        mutant.expect === undefined
          ? undefined
          : (await checkMutant(spec, mutant)).verdict
    }
    // An error checks nothing, so it is never a kill, and a mutant with no
    // expect line is left for the command to fail unchecked.
    expect(verdicts).toEqual({
      'Worker-error': 'ERROR',
      'Worker-live': 'LIVE',
      'Worker-miss': 'MISS',
      'Worker-unfair': 'kill',
      'Worker-unjudged': undefined,
    })
    // The testdata directory itself names the same mutants.
    expect(
      findConfigMutants(path.join(testdata, 'worker', 'testdata')),
    ).toEqual(configMutantsOf(spec))
  })

  it('selects one spec when given its file', () => {
    const models = [
      ...findModels(path.join(testdata, 'mappicker', 'MapPicker.tla')),
    ]
    expect(models.map((m) => path.basename(m.config)).sort()).toEqual([
      'MapPicker-all.cfg',
      'MapPicker-rule.cfg',
    ])
  })

  it('fails a stale chart until it is redrawn', async () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'aptre-tla-'))
    try {
      fs.cpSync(path.join(testdata, 'challenge'), dir, { recursive: true })
      const model = {
        spec: path.join(dir, 'ChallengeCard.tla'),
        config: path.join(dir, 'ChallengeCard-private.cfg'),
      }
      expect((await checkModel(model)).ok).toBe(true)
      const view = path.join(dir, 'ChallengeCard.view.ts')
      fs.writeFileSync(
        view,
        fs
          .readFileSync(view, 'utf8')
          .replace("'ten minutes pass'", "'the challenge expires'"),
      )
      const stale = await checkModel(model)
      expect(stale).toMatchObject({ ok: false, rule: 'chart' })
      expect(stale.error).toMatch(/is stale; run: aptre-tla chart /)
      fs.writeFileSync(chartOf(model), await drawChart(model, view))
      expect((await checkModel(model)).ok).toBe(true)
    } finally {
      fs.rmSync(dir, { recursive: true, force: true })
    }
  })
})
