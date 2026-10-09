import { describe, it, expect, vi } from 'vitest'
import userEvent from '@testing-library/user-event'
import { render, screen } from '../test/test-utils'
import { RoomStats } from './RoomStats'
import type { RoomStatsResponse } from '../api/types'
import { activityFixture, personFixture, roomStatsFixture } from '../test/statsFixtures'

function renderStats(stats: RoomStatsResponse | undefined, overrides: { isRefreshing?: boolean } = {}) {
  const onDaysChange = vi.fn()
  render(
    <RoomStats
      stats={stats}
      isLoading={false}
      isRefreshing={overrides.isRefreshing ?? false}
      error={null}
      days={7}
      onDaysChange={onDaysChange}
    />
  )
  return { onDaysChange }
}

describe('RoomStats', () => {
  it("should sum up the period's activity", () => {
    renderStats(roomStatsFixture())

    expect(screen.getByText('42')).toBeInTheDocument()
    expect(screen.getByText('6.0')).toBeInTheDocument()
    expect(screen.getByText('30')).toBeInTheDocument()
    expect(screen.getByText('Tue 2026-10-06')).toBeInTheDocument()
    expect(screen.getByText('158')).toBeInTheDocument()
    expect(screen.getByText('Text messages per day')).toBeInTheDocument()
  })

  it('should rank the people since counting began, marking anyone not in the room', () => {
    renderStats(roomStatsFixture())

    expect(screen.getByText('Most messages · all time')).toBeInTheDocument()
    expect(
      screen.getByText('14 people have sent 3,541 text messages since 2026-01-31')
    ).toBeInTheDocument()

    const rows = screen.getAllByRole('listitem')
    expect(rows[0]).toHaveTextContent('Bob')
    expect(rows[0]).toHaveTextContent('(@bob:example.com)')
    expect(rows[0]).toHaveTextContent('1,204 · 34%')
    expect(rows[1]).toHaveTextContent('@carol:example.com')
    expect(rows[1]).toHaveTextContent('(not in room)')
  })

  it('should show where the viewer stands when they are not in the list', () => {
    renderStats(roomStatsFixture())

    expect(screen.getByText('You: #4 of 14 · 233 messages · 6.6%')).toBeInTheDocument()
  })

  it('should mark the viewer in the list when they are in it', () => {
    const stats = roomStatsFixture()
    stats.people.me = personFixture()
    renderStats(stats)

    expect(screen.getAllByRole('listitem')[0]).toHaveTextContent('(you)')
    expect(screen.queryByText(/^You:/)).not.toBeInTheDocument()
  })

  it('should say when the period starts later because counting began later', () => {
    const daily = activityFixture().daily.slice(4)
    renderStats(roomStatsFixture({ activity: activityFixture({ first_day: '2026-10-08', from: '2026-10-08', daily }) }))

    expect(
      screen.getByText('Counting began on 2026-10-08, so the period starts then.')
    ).toBeInTheDocument()
  })

  it('should pick another period', async () => {
    const user = userEvent.setup()
    const { onDaysChange } = renderStats(roomStatsFixture())

    await user.click(screen.getByRole('button', { name: '90d' }))
    expect(onDaysChange).toHaveBeenCalledWith(90)
  })

  it("should dim the previous period's figures while the new ones load", () => {
    renderStats(roomStatsFixture(), { isRefreshing: true })

    expect(screen.getByText('Text messages per day').closest('.opacity-50')).not.toBeNull()
  })

  it('should say when nothing has been counted in the room', () => {
    const empty = roomStatsFixture({
      activity: activityFixture({ first_day: null, messages: 0, busiest: null, daily: [] }),
    })
    empty.people = { ...empty.people, counted_since: null, people_count: 0, top: [], me: null }
    renderStats(empty)

    expect(screen.getByText('No messages counted in this room yet')).toBeInTheDocument()
  })

  it('should show loading and failure', () => {
    const { rerender } = render(
      <RoomStats stats={undefined} isLoading isRefreshing={false} error={null} days={30} onDaysChange={vi.fn()} />
    )
    expect(screen.getByText('Loading stats...')).toBeInTheDocument()

    rerender(
      <RoomStats
        stats={undefined}
        isLoading={false}
        isRefreshing={false}
        error={new Error('Failed')}
        days={30}
        onDaysChange={vi.fn()}
      />
    )
    expect(screen.getByText('Failed to load stats')).toBeInTheDocument()
  })
})
