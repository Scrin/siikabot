import { describe, it, expect, vi, beforeEach } from 'vitest'
import userEvent from '@testing-library/user-event'
import { render, screen } from '../test/test-utils'
import { MessageActivityCard } from './MessageActivityCard'
import * as queries from '../api/queries'
import { activityFixture, messageActivityFixture, roomActivityFixture } from '../test/statsFixtures'

vi.mock('../api/queries', () => ({
  useMessageActivity: vi.fn(),
}))

vi.mock('../hooks/useReducedMotion', () => ({
  useReducedMotion: () => true, // Disable animations for testing
}))

function mockActivity(value: Partial<ReturnType<typeof queries.useMessageActivity>>) {
  vi.mocked(queries.useMessageActivity).mockReturnValue({
    isLoading: false,
    isPlaceholderData: false,
    error: null,
    ...value,
  } as ReturnType<typeof queries.useMessageActivity>)
}

describe('MessageActivityCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('should show loading and failure', () => {
    mockActivity({ isLoading: true, data: undefined })
    const { unmount } = render(<MessageActivityCard />)
    expect(screen.getByText('Loading message activity...')).toBeInTheDocument()
    unmount()

    mockActivity({ data: undefined, error: new Error('Failed') })
    render(<MessageActivityCard />)
    expect(screen.getByText('Failed to load message activity')).toBeInTheDocument()
  })

  it('should sum up every room, and rank the rooms with their chat turns', () => {
    mockActivity({ data: messageActivityFixture() })
    render(<MessageActivityCard />)

    expect(screen.getByText('Messages').nextElementSibling).toHaveTextContent('40')
    expect(screen.getByText('Active rooms').nextElementSibling).toHaveTextContent('2')
    expect(screen.getByText('Chat turns').nextElementSibling).toHaveTextContent('8')
    expect(screen.getByText('Text messages per day, all rooms')).toBeInTheDocument()

    const rows = screen.getAllByRole('listitem')
    expect(rows[0]).toHaveTextContent('1.Busy room(!busy:example.com)')
    expect(rows[0]).toHaveTextContent('30 · 75%7 chat turns')
    expect(rows[1]).toHaveTextContent('2.!quiet:example.com')
    expect(rows[1]).toHaveTextContent('1 chat turn')
  })

  it('should give rooms with the same count the same place', () => {
    mockActivity({
      data: messageActivityFixture({
        rooms: [
          roomActivityFixture({ room_id: '!a:x', room_name: 'A', messages: 10, share: 0.5 }),
          roomActivityFixture({ room_id: '!b:x', room_name: 'B', messages: 10, share: 0.5 }),
          roomActivityFixture({ room_id: '!c:x', room_name: 'C', messages: 0, share: 0, chat_turns: 2 }),
        ],
      }),
    })
    render(<MessageActivityCard />)

    const ranks = screen.getAllByRole('listitem').map((row) => row.textContent?.split('.')[0])
    expect(ranks).toEqual(['1', '1', '3'])
  })

  it('should fetch another period when picked, for 30 days at first', async () => {
    const user = userEvent.setup()
    mockActivity({ data: messageActivityFixture() })
    render(<MessageActivityCard />)
    expect(queries.useMessageActivity).toHaveBeenLastCalledWith(30)

    await user.click(screen.getByRole('button', { name: '7d' }))
    expect(queries.useMessageActivity).toHaveBeenLastCalledWith(7)
  })

  it('should say when nothing has been counted yet', () => {
    mockActivity({
      data: messageActivityFixture({
        activity: activityFixture({ first_day: null, daily: [], busiest: null }),
        rooms: [],
      }),
    })
    render(<MessageActivityCard />)

    expect(screen.getByText('No messages counted yet')).toBeInTheDocument()
  })
})
