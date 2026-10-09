import { describe, it, expect } from 'vitest'
import { render, screen } from '../test/test-utils'
import { RankedBars } from './RankedBars'

describe('RankedBars', () => {
  it("should measure each bar against the largest, and write every value beside its bar", () => {
    render(
      <RankedBars
        label="People with the most messages"
        bars={[
          { key: 'bob', rank: 1, label: 'Bob', id: '@bob:x', value: 200, valueLabel: '200 · 80%' },
          { key: 'eve', rank: 2, label: '@eve:x', value: 50, valueLabel: '50 · 20%', muted: true },
          { key: 'zed', rank: 3, label: '@zed:x', value: 0, valueLabel: '0 · 0%' },
        ]}
      />
    )

    const rows = screen.getAllByRole('listitem')
    expect(rows[0]).toHaveTextContent('1.Bob(@bob:x)200 · 80%')
    expect(rows[1]).toHaveClass('opacity-60')

    const bars = rows.map((row) => row.querySelector('.bg-purple-500') as HTMLElement)
    expect(bars[0].style.width).toBe('max(2px, 100%)')
    expect(bars[1].style.width).toBe('max(2px, 25%)')
    expect(bars[2].style.width).toBe('0px')
  })
})
