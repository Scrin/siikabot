import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen } from '../test/test-utils'
import { RemindersCard } from './RemindersCard'
import * as queries from '../api/queries'

vi.mock('../api/queries', () => ({
  useReminders: vi.fn(),
}))

vi.mock('../hooks/useReducedMotion', () => ({
  useReducedMotion: () => true, // Disable animations for testing
}))

describe('RemindersCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('loading state', () => {
    it('should show loading spinner', () => {
      vi.mocked(queries.useReminders).mockReturnValue({
        isLoading: true,
        data: undefined,
        error: null,
      } as ReturnType<typeof queries.useReminders>)

      render(<RemindersCard />)
      expect(screen.getByText('Loading reminders...')).toBeInTheDocument()
    })
  })

  describe('error state', () => {
    it('should show error message', () => {
      vi.mocked(queries.useReminders).mockReturnValue({
        isLoading: false,
        data: undefined,
        error: new Error('Failed'),
      } as ReturnType<typeof queries.useReminders>)

      render(<RemindersCard />)
      expect(screen.getByText('Failed to load reminders')).toBeInTheDocument()
    })
  })

  describe('empty state', () => {
    it('should show no reminders message when array is empty', () => {
      vi.mocked(queries.useReminders).mockReturnValue({
        isLoading: false,
        data: { reminders: [] },
        error: null,
      } as ReturnType<typeof queries.useReminders>)

      render(<RemindersCard />)
      expect(screen.getByText('No active reminders')).toBeInTheDocument()
    })

    it('should show no reminders message when data is undefined', () => {
      vi.mocked(queries.useReminders).mockReturnValue({
        isLoading: false,
        data: undefined,
        error: null,
      } as ReturnType<typeof queries.useReminders>)

      render(<RemindersCard />)
      expect(screen.getByText('No active reminders')).toBeInTheDocument()
    })
  })

  describe('with reminders', () => {
    it('should display reminder message', () => {
      vi.mocked(queries.useReminders).mockReturnValue({
        isLoading: false,
        data: {
          reminders: [
            {
              id: 1,
              remind_time: new Date(Date.now() + 3600000).toISOString(), // 1 hour from now
              room_id: '!room:example.com',
              message: 'Test reminder message',
            },
          ],
        },
        error: null,
      } as ReturnType<typeof queries.useReminders>)

      render(<RemindersCard />)
      expect(screen.getByText('Test reminder message')).toBeInTheDocument()
    })

    it('should display room ID', () => {
      vi.mocked(queries.useReminders).mockReturnValue({
        isLoading: false,
        data: {
          reminders: [
            {
              id: 1,
              remind_time: new Date(Date.now() + 3600000).toISOString(),
              room_id: '!room:example.com',
              message: 'Test',
            },
          ],
        },
        error: null,
      } as ReturnType<typeof queries.useReminders>)

      render(<RemindersCard />)
      expect(screen.getByText('!room:example.com')).toBeInTheDocument()
    })

    it('should display room name with ID when available', () => {
      vi.mocked(queries.useReminders).mockReturnValue({
        isLoading: false,
        data: {
          reminders: [
            {
              id: 1,
              remind_time: new Date(Date.now() + 3600000).toISOString(),
              room_id: '!room:example.com',
              room_name: 'Test Room',
              message: 'Test',
            },
          ],
        },
        error: null,
      } as ReturnType<typeof queries.useReminders>)

      render(<RemindersCard />)
      expect(screen.getByText('Test Room')).toBeInTheDocument()
      expect(screen.getByText('(!room:example.com)')).toBeInTheDocument()
    })
  })

  describe('formatTimeUntil (tested via component)', () => {
    beforeEach(() => {
      vi.useFakeTimers()
      vi.setSystemTime(new Date('2026-01-30T12:00:00Z'))
    })

    afterEach(() => {
      vi.useRealTimers()
    })

    it('should show "any moment now" for past dates', () => {
      vi.mocked(queries.useReminders).mockReturnValue({
        isLoading: false,
        data: {
          reminders: [
            {
              id: 1,
              remind_time: '2026-01-30T11:59:00Z', // 1 minute ago
              room_id: '!room:example.com',
              message: 'Test reminder',
            },
          ],
        },
        error: null,
      } as ReturnType<typeof queries.useReminders>)

      render(<RemindersCard />)
      expect(screen.getByText('any moment now')).toBeInTheDocument()
    })

    it('should show seconds for < 1 minute', () => {
      vi.mocked(queries.useReminders).mockReturnValue({
        isLoading: false,
        data: {
          reminders: [
            {
              id: 1,
              remind_time: '2026-01-30T12:00:30Z', // 30 seconds from now
              room_id: '!room:example.com',
              message: 'Test reminder',
            },
          ],
        },
        error: null,
      } as ReturnType<typeof queries.useReminders>)

      render(<RemindersCard />)
      expect(screen.getByText('in 30s')).toBeInTheDocument()
    })

    it('should show minutes for < 1 hour', () => {
      vi.mocked(queries.useReminders).mockReturnValue({
        isLoading: false,
        data: {
          reminders: [
            {
              id: 1,
              remind_time: '2026-01-30T12:45:00Z', // 45 minutes from now
              room_id: '!room:example.com',
              message: 'Test reminder',
            },
          ],
        },
        error: null,
      } as ReturnType<typeof queries.useReminders>)

      render(<RemindersCard />)
      expect(screen.getByText('in 45m')).toBeInTheDocument()
    })

    it('should show hours and minutes for < 1 day', () => {
      vi.mocked(queries.useReminders).mockReturnValue({
        isLoading: false,
        data: {
          reminders: [
            {
              id: 1,
              remind_time: '2026-01-30T14:30:00Z', // 2h 30m from now
              room_id: '!room:example.com',
              message: 'Test reminder',
            },
          ],
        },
        error: null,
      } as ReturnType<typeof queries.useReminders>)

      render(<RemindersCard />)
      expect(screen.getByText('in 2h 30m')).toBeInTheDocument()
    })

    it('should show days and hours for >= 1 day', () => {
      vi.mocked(queries.useReminders).mockReturnValue({
        isLoading: false,
        data: {
          reminders: [
            {
              id: 1,
              remind_time: '2026-02-01T18:00:00Z', // ~2d 6h from now
              room_id: '!room:example.com',
              message: 'Test reminder',
            },
          ],
        },
        error: null,
      } as ReturnType<typeof queries.useReminders>)

      render(<RemindersCard />)
      expect(screen.getByText('in 2d 6h')).toBeInTheDocument()
    })
  })
})
