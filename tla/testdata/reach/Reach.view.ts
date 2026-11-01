export const state = (v: { left: number }) =>
  v.left === 0
    ? 'Shipped'
    : v.left === 1
      ? '1 iteration left'
      : '2 iterations left'
export const step = () => 'an iteration finishes'
