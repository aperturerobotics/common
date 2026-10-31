// The challenge card in Discord: Mira challenges Jun, and Jun answers.

type Card = { challenge: string; refused: string[][] }

export const title = 'Discord challenge: Mira challenges Jun'

const cap = (s: string) => s[0].toUpperCase() + s.slice(1)

const shown: Record<string, string> = {
  open: 'Open: waiting for Jun',
  accepted: 'Accepted',
  declined: 'Declined',
  withdrawn: 'Withdrawn',
  expired: 'Expired',
}

export const state = (v: Card) =>
  v.refused.length
    ? `${shown[v.challenge]}; a press was refused`
    : shown[v.challenge]

const answered: Record<string, string> = {
  accepted: 'Jun presses Accept',
  declined: 'Jun presses Decline',
  withdrawn: 'Mira presses Withdraw',
}

export const step = (action: string, from: Card, to: Card) => {
  if (action === 'Expire') return 'ten minutes pass'
  if (action !== 'Press') return null
  if (to.refused.length > from.refused.length) {
    const p = to.refused.find(
      (r) => !from.refused.some((f) => f[1] === r[1] && f[2] === r[2]),
    )!
    return `${cap(p[1])} presses ${cap(p[2])}, which only Jun may answer`
  }
  // A repeated refusal changes nothing.
  if (to.challenge === from.challenge) return null
  return answered[to.challenge]
}
