/**
 * The top of a chart's value scale: the smallest round number at or above the largest value, so the
 * one gridline drawn at it carries a clean label
 *
 * @example
 * niceCeiling(61) // 80
 * niceCeiling(7) // 8
 */
export function niceCeiling(value: number): number {
  if (value <= 0) {
    return 1
  }
  const magnitude = 10 ** Math.floor(Math.log10(value))
  for (const step of [1, 2, 4, 5, 6, 8, 10]) {
    if (step * magnitude >= value) {
      return step * magnitude
    }
  }
  return 10 * magnitude
}
