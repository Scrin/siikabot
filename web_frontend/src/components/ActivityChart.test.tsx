import { describe, it, expect } from 'vitest'
import userEvent from '@testing-library/user-event'
import { act, fireEvent, render, screen } from '../test/test-utils'
import { ActivityChart } from './ActivityChart'
import { activityFixture } from '../test/statsFixtures'

const { daily, to: today } = activityFixture()

function renderChart() {
  return render(<ActivityChart daily={daily} today={today} title="Text messages per day" />)
}

describe('ActivityChart', () => {
  it('should draw a column for every day, on a scale up to a round number', () => {
    renderChart()

    expect(screen.getAllByTestId('activity-day')).toHaveLength(7)
    // The busiest day has 30 messages, so the gridline sits at 40
    expect(screen.getByText('40')).toBeInTheDocument()
    expect(screen.getByText('2026-10-04')).toBeInTheDocument()
    expect(screen.getByText('today')).toBeInTheDocument()
  })

  it("should show a day's counts when its column is pointed at", () => {
    renderChart()

    fireEvent.pointerEnter(screen.getAllByTestId('activity-day')[2])
    const tooltip = screen.getByRole('status')
    expect(tooltip).toHaveTextContent('30 messages')
    expect(tooltip).toHaveTextContent('100 words')
    expect(tooltip).toHaveTextContent('Tue 2026-10-06')
  })

  it('should say that today is still being counted', () => {
    renderChart()

    fireEvent.pointerEnter(screen.getAllByTestId('activity-day')[6])
    expect(screen.getByRole('status')).toHaveTextContent('Sat 2026-10-10 (today, so far)')
  })

  it('should step through the days with the arrow keys, starting from today', async () => {
    const user = userEvent.setup()
    renderChart()

    act(() => screen.getByRole('group', { name: /^Text messages per day/ }).focus())
    expect(screen.getByRole('status')).toHaveTextContent('2 messages')

    await user.keyboard('{ArrowLeft}')
    expect(screen.getByRole('status')).toHaveTextContent('Fri 2026-10-09')

    await user.keyboard('{Home}')
    expect(screen.getByRole('status')).toHaveTextContent('Sun 2026-10-04')

    await user.keyboard('{Escape}')
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it('should show the same days as a table', async () => {
    const user = userEvent.setup()
    renderChart()

    await user.click(screen.getByRole('button', { name: 'table' }))
    const rows = screen.getAllByRole('row')
    // A header row and a row a day
    expect(rows).toHaveLength(8)
    expect(rows[3]).toHaveTextContent('Tue 2026-10-06')
    expect(rows[3]).toHaveTextContent('30')
    expect(rows[7]).toHaveTextContent('(today, so far)')

    await user.click(screen.getByRole('button', { name: 'chart' }))
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })
})
