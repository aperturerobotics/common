// A wizard's Finish control in tab a, beside the wizard and its object. A
// failed tab whose own list landed shows finishing until it finds its object.

export const title = 'Wizard Finish: one atomic list'

export const state = (v) => {
  // The chart follows tab a; tab b acts as the other writer.
  const c = v.control.a
  const checking = c === 'failed' && v.lost.includes('a')
  if (c === 'gone' || (!v.wizard && c === 'idle')) {
    return 'Wizard gone, Close tab shown'
  }
  if (c === 'idle') return 'Finish shown'
  if (c === 'finishing' || (checking && !v.wizard)) return 'Finishing'
  if (c === 'failed' && !v.wizard && !checking) {
    return 'Wizard gone, Close tab shown'
  }
  if (c === 'failed') return 'Failed, says why, Retry shown'
  return 'Done, object opens'
}

export const step = (action, from, to) => {
  // A step that keeps the drawn state draws no edge.
  if (state(from) === state(to)) return null

  // Each action names the edge it draws.
  if (action === 'Press') return 'presses Finish or Retry'
  if (action === 'Commit') return 'list commits, or is refused as gone'
  if (action === 'Discard') return 'a write fails, list discarded'
  if (action === 'Reload') return 'tab reloads'
  if (action === 'OrphanEnds') return 'a list from before a reload ends'
  if (action === 'MoveIndex') return 'another writer moves the index'
  if (action === 'LostReply') return 'list commits, reply lost'
  if (action === 'Recognise') return 'finds its own object'
  return null
}
