import { describe, it, expect, vi, beforeEach } from 'vitest'
import userEvent from '@testing-library/user-event'
import { render, screen } from '../test/test-utils'
import { ChatUsageCard } from './ChatUsageCard'
import * as queries from '../api/queries'
import type { ChatUsageEntry, ChatUsageResponse } from '../api/types'

vi.mock('../api/queries', () => ({
  useChatUsage: vi.fn(),
}))

vi.mock('../hooks/useReducedMotion', () => ({
  useReducedMotion: () => true, // Disable animations for testing
}))

function entry(overrides: Partial<ChatUsageEntry> = {}): ChatUsageEntry {
  return {
    room_id: '!room:example.org',
    room_name: 'Test Room',
    model: 'openai/gpt-4o-mini',
    turns: 12,
    prompt_tokens: 24000,
    completion_tokens: 1200,
    cached_prompt_tokens: 18000,
    cache_hit_rate: 0.75,
    tool_iterations: 20,
    failures: 0,
    ...overrides,
  }
}

function response(overrides: Partial<ChatUsageResponse> = {}): ChatUsageResponse {
  const entries = overrides.entries ?? [entry()]
  return {
    since: '2026-08-01T00:00:00Z',
    days: 7,
    entries,
    totals: overrides.totals ?? entry({ model: 'all' }),
    ...overrides,
  }
}

function mockUsage(value: Partial<ReturnType<typeof queries.useChatUsage>>) {
  vi.mocked(queries.useChatUsage).mockReturnValue(
    value as ReturnType<typeof queries.useChatUsage>
  )
}

describe('ChatUsageCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('loading state', () => {
    it('should show loading spinner', () => {
      mockUsage({ isLoading: true, data: undefined, error: null })

      render(<ChatUsageCard />)
      expect(screen.getByText('Loading chat usage...')).toBeInTheDocument()
    })
  })

  describe('error state', () => {
    it('should show error message', () => {
      mockUsage({ isLoading: false, data: undefined, error: new Error('Failed') })

      render(<ChatUsageCard />)
      expect(screen.getByText('Failed to load chat usage')).toBeInTheDocument()
    })
  })

  describe('empty state', () => {
    // The table starts empty and only fills as turns happen, so this is what an admin sees first
    it('should explain that nothing has been recorded yet', () => {
      mockUsage({ isLoading: false, data: response({ entries: [] }), error: null })

      render(<ChatUsageCard />)
      expect(screen.getByText('No chat usage recorded in the last 7d')).toBeInTheDocument()
    })

    it('should still offer the period selector so the range can be widened', () => {
      mockUsage({ isLoading: false, data: response({ entries: [] }), error: null })

      render(<ChatUsageCard />)
      expect(screen.getByRole('button', { name: '30d' })).toBeInTheDocument()
    })
  })

  describe('usage list', () => {
    it('should show the room, model and token split', () => {
      mockUsage({ isLoading: false, data: response(), error: null })

      render(<ChatUsageCard />)

      expect(screen.getByText('Test Room')).toBeInTheDocument()
      expect(screen.getByText('openai/gpt-4o-mini')).toBeInTheDocument()
      expect(screen.getByText('24k in / 1.2k out')).toBeInTheDocument()
    })

    it('should fall back to the room id when the name is unknown', () => {
      mockUsage({
        isLoading: false,
        data: response({ entries: [entry({ room_name: '' })] }),
        error: null,
      })

      render(<ChatUsageCard />)
      expect(screen.getAllByText('!room:example.org').length).toBeGreaterThan(0)
    })

    it('should separate rows for the same room on different models', () => {
      mockUsage({
        isLoading: false,
        data: response({
          entries: [entry(), entry({ model: 'google/gemini-3-flash' })],
        }),
        error: null,
      })

      render(<ChatUsageCard />)

      expect(screen.getByText('openai/gpt-4o-mini')).toBeInTheDocument()
      expect(screen.getByText('google/gemini-3-flash')).toBeInTheDocument()
    })

    it('should surface failures rather than hiding them among the totals', () => {
      mockUsage({
        isLoading: false,
        data: response({ entries: [entry({ failures: 3 })] }),
        error: null,
      })

      render(<ChatUsageCard />)
      expect(screen.getByText('3 failed')).toBeInTheDocument()
    })
  })

  describe('cache hit rate', () => {
    // The hit rate is the direct evidence that the stable prompt prefix is working, so it is
    // coloured by how healthy it is rather than being left as one more number in a row
    it('should show a healthy rate in green', () => {
      mockUsage({
        isLoading: false,
        data: response({ entries: [entry({ cache_hit_rate: 0.75 })] }),
        error: null,
      })

      render(<ChatUsageCard />)
      expect(screen.getByText('75% cached')).toHaveClass('text-emerald-400')
    })

    it('should show a poor rate without the healthy colour', () => {
      mockUsage({
        isLoading: false,
        data: response({ entries: [entry({ cache_hit_rate: 0.02 })] }),
        error: null,
      })

      render(<ChatUsageCard />)
      expect(screen.getByText('2% cached')).not.toHaveClass('text-emerald-400')
    })
  })

  describe('totals', () => {
    it('should summarise the period above the per-room rows', () => {
      mockUsage({
        isLoading: false,
        data: response({
          entries: [entry()],
          totals: entry({ model: 'all', turns: 40, prompt_tokens: 100000, failures: 2 }),
        }),
        error: null,
      })

      render(<ChatUsageCard />)

      expect(screen.getByText('Turns')).toBeInTheDocument()
      expect(screen.getByText('40')).toBeInTheDocument()
      expect(screen.getByText('100k')).toBeInTheDocument()
      expect(screen.getByText('2 failed turns')).toBeInTheDocument()
    })

    it('should use the singular for a single failure', () => {
      mockUsage({
        isLoading: false,
        data: response({ totals: entry({ model: 'all', failures: 1 }) }),
        error: null,
      })

      render(<ChatUsageCard />)
      expect(screen.getByText('1 failed turn')).toBeInTheDocument()
    })

    it('should say nothing about failures when there were none', () => {
      mockUsage({
        isLoading: false,
        data: response({ totals: entry({ model: 'all', failures: 0 }) }),
        error: null,
      })

      render(<ChatUsageCard />)
      expect(screen.queryByText(/failed turn/)).not.toBeInTheDocument()
    })
  })

  describe('period selection', () => {
    it('should request the selected number of days', async () => {
      const user = userEvent.setup()
      mockUsage({ isLoading: false, data: response(), error: null })

      render(<ChatUsageCard />)

      // Defaults to a week, which is the period the model and cost decisions are made against
      expect(queries.useChatUsage).toHaveBeenCalledWith(7)

      await user.click(screen.getByRole('button', { name: '30d' }))
      expect(queries.useChatUsage).toHaveBeenLastCalledWith(30)
    })
  })
})
