import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { checkModel, findModels } from './check.js'

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
    ])
    expect(results['ChallengeCard-private.cfg'].ok).toBe(true)
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

  it('selects one spec when given its file', () => {
    const models = [
      ...findModels(path.join(testdata, 'mappicker', 'MapPicker.tla')),
    ]
    expect(models.map((m) => path.basename(m.config)).sort()).toEqual([
      'MapPicker-all.cfg',
      'MapPicker-rule.cfg',
    ])
  })
})
