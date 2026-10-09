/**
 * Number formatting shared by the dashboard cards
 */

/**
 * Compact a count for a stat tile
 *
 * @example
 * formatCount(950) // "950"
 * formatCount(1234) // "1.2k"
 * formatCount(12345) // "12k"
 */
export function formatCount(value: number): string {
  if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(1)}M`
  }
  if (value >= 10_000) {
    return `${Math.round(value / 1000)}k`
  }
  if (value >= 1000) {
    return `${(value / 1000).toFixed(1)}k`
  }
  return String(value)
}

/**
 * Write out a count in full, for a list where counts are compared with each other
 *
 * @example
 * formatExact(1204) // "1,204"
 */
export function formatExact(value: number): string {
  return value.toLocaleString('en-US')
}

/**
 * Write a share from 0 to 1 as a percentage, with a decimal for a share under 10%
 *
 * @example
 * formatShare(0.34) // "34%"
 * formatShare(0.042) // "4.2%"
 */
export function formatShare(share: number): string {
  const percent = share * 100
  if (percent > 0 && percent < 10) {
    return `${percent.toFixed(1)}%`
  }
  return `${Math.round(percent)}%`
}
