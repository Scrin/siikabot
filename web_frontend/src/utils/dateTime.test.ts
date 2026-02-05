import { describe, it, expect } from 'vitest'
import { formatDateTime } from './dateTime'

describe('formatDateTime', () => {
  it('should format date in YYYY-MM-DD HH:MM:SS format', () => {
    // Using a fixed date to avoid timezone issues
    const date = new Date(2026, 0, 30, 15, 42, 30) // Jan 30, 2026, 15:42:30
    expect(formatDateTime(date)).toBe('2026-01-30 15:42:30')
  })

  it('should pad single-digit months with zeros', () => {
    const date = new Date(2026, 0, 15, 12, 30, 45) // January (month 0)
    expect(formatDateTime(date)).toMatch(/^2026-01-/)
  })

  it('should pad single-digit days with zeros', () => {
    const date = new Date(2026, 5, 5, 12, 30, 45) // June 5
    expect(formatDateTime(date)).toMatch(/-06-05 /)
  })

  it('should pad single-digit hours with zeros', () => {
    const date = new Date(2026, 0, 15, 3, 30, 45) // 3 AM
    expect(formatDateTime(date)).toMatch(/ 03:/)
  })

  it('should pad single-digit minutes with zeros', () => {
    const date = new Date(2026, 0, 15, 12, 7, 45)
    expect(formatDateTime(date)).toMatch(/:07:/)
  })

  it('should pad single-digit seconds with zeros', () => {
    const date = new Date(2026, 0, 15, 12, 30, 9)
    expect(formatDateTime(date)).toMatch(/:09$/)
  })

  it('should handle midnight', () => {
    const date = new Date(2026, 5, 15, 0, 0, 0) // June 15, 2026, 00:00:00
    expect(formatDateTime(date)).toBe('2026-06-15 00:00:00')
  })

  it('should handle end of day', () => {
    const date = new Date(2026, 11, 31, 23, 59, 59) // Dec 31, 2026, 23:59:59
    expect(formatDateTime(date)).toBe('2026-12-31 23:59:59')
  })

  it('should handle leap year dates', () => {
    const date = new Date(2024, 1, 29, 12, 30, 45) // Feb 29, 2024 (leap year)
    expect(formatDateTime(date)).toBe('2024-02-29 12:30:45')
  })

  it('should handle all double-digit values', () => {
    const date = new Date(2026, 10, 22, 14, 35, 48) // Nov 22, 2026, 14:35:48
    expect(formatDateTime(date)).toBe('2026-11-22 14:35:48')
  })
})
