import { describe, it, expect } from 'vitest'
import { formatCount, formatExact, formatShare } from './format'
import { niceCeiling } from './chart'

describe('formatCount', () => {
  it('should compact large counts', () => {
    expect(formatCount(950)).toBe('950')
    expect(formatCount(1234)).toBe('1.2k')
    expect(formatCount(12345)).toBe('12k')
    expect(formatCount(1_500_000)).toBe('1.5M')
  })
})

describe('formatExact', () => {
  it('should separate thousands', () => {
    expect(formatExact(1204)).toBe('1,204')
    expect(formatExact(12)).toBe('12')
  })
})

describe('formatShare', () => {
  it('should give a small share a decimal', () => {
    expect(formatShare(0.34)).toBe('34%')
    expect(formatShare(0.042)).toBe('4.2%')
    expect(formatShare(0)).toBe('0%')
    expect(formatShare(1)).toBe('100%')
  })
})

describe('niceCeiling', () => {
  it('should round the largest value up to a round number', () => {
    expect(niceCeiling(61)).toBe(80)
    expect(niceCeiling(30)).toBe(40)
    expect(niceCeiling(7)).toBe(8)
    expect(niceCeiling(100)).toBe(100)
    expect(niceCeiling(1000)).toBe(1000)
    expect(niceCeiling(1)).toBe(1)
  })

  it('should give a chart with nothing in it a scale', () => {
    expect(niceCeiling(0)).toBe(1)
  })
})
