/**
 * Date and time formatting utilities
 * Application standard: YYYY-MM-DD HH:MM:SS (24-hour clock)
 */

/**
 * Format a Date object to "YYYY-MM-DD HH:MM:SS" format (24-hour clock)
 * This is the standard date-time format used throughout the application
 *
 * @param date - The Date object to format
 * @returns Formatted date string in "YYYY-MM-DD HH:MM:SS" format
 *
 * @example
 * formatDateTime(new Date('2026-01-30T15:42:30Z'))
 * // Returns: "2026-01-30 15:42:30"
 */
export function formatDateTime(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hours = String(date.getHours()).padStart(2, '0')
  const minutes = String(date.getMinutes()).padStart(2, '0')
  const seconds = String(date.getSeconds()).padStart(2, '0')

  return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`
}
