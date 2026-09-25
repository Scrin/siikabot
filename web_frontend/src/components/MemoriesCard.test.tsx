import { describe, it, expect, vi, beforeEach } from 'vitest'
import userEvent from '@testing-library/user-event'
import { render, screen } from '../test/test-utils'
import { MemoriesCard } from './MemoriesCard'
import * as queries from '../api/queries'
import type { MemoryResponse } from '../api/types'

vi.mock('../api/queries', () => ({
  useMemories: vi.fn(),
  useDeleteMemory: vi.fn(),
  useDeleteAllMemories: vi.fn(),
}))

vi.mock('../hooks/useReducedMotion', () => ({
  useReducedMotion: () => true, // Disable animations for testing
}))

const createdAt = '2026-09-25T14:02:00Z'

function mockMemories(memories: MemoryResponse[]) {
  vi.mocked(queries.useMemories).mockReturnValue({
    isLoading: false,
    data: { memories },
    error: null,
  } as unknown as ReturnType<typeof queries.useMemories>)
}

describe('MemoriesCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    const idleMutation = { mutateAsync: vi.fn(), isPending: false, isError: false, error: null }
    vi.mocked(queries.useDeleteMemory).mockReturnValue(
      idleMutation as unknown as ReturnType<typeof queries.useDeleteMemory>
    )
    vi.mocked(queries.useDeleteAllMemories).mockReturnValue(
      idleMutation as unknown as ReturnType<typeof queries.useDeleteAllMemories>
    )
  })

  describe('loading state', () => {
    it('should show loading spinner', () => {
      vi.mocked(queries.useMemories).mockReturnValue({
        isLoading: true,
        data: undefined,
        error: null,
      } as ReturnType<typeof queries.useMemories>)

      render(<MemoriesCard />)
      expect(screen.getByText('Loading memories...')).toBeInTheDocument()
    })
  })

  describe('error state', () => {
    it('should show error message', () => {
      vi.mocked(queries.useMemories).mockReturnValue({
        isLoading: false,
        data: undefined,
        error: new Error('Failed'),
      } as ReturnType<typeof queries.useMemories>)

      render(<MemoriesCard />)
      expect(screen.getByText('Failed to load memories')).toBeInTheDocument()
    })
  })

  describe('empty state', () => {
    it('should show no memories message', () => {
      mockMemories([])

      render(<MemoriesCard />)
      expect(screen.getByText('No memories stored')).toBeInTheDocument()
    })
  })

  describe('where a memory is used', () => {
    it('should say a memory saved in a direct chat is used in direct chats only', () => {
      mockMemories([{ id: 1, memory: 'likes tea', room_id: null, created_at: createdAt }])

      render(<MemoriesCard />)
      expect(screen.getByText('likes tea')).toBeInTheDocument()
      expect(screen.getByText('direct chats only')).toBeInTheDocument()
      expect(screen.queryByText('and direct chats')).not.toBeInTheDocument()
    })

    it('should name the room a group memory is used in, along with direct chats', () => {
      mockMemories([
        {
          id: 2,
          memory: 'prefers metric units',
          room_id: '!group:example.com',
          room_name: 'Siika HQ',
          created_at: createdAt,
        },
      ])

      render(<MemoriesCard />)
      expect(screen.getByText('Siika HQ')).toBeInTheDocument()
      expect(screen.getByText('(!group:example.com)')).toBeInTheDocument()
      expect(screen.getByText('and direct chats')).toBeInTheDocument()
      expect(screen.queryByText('direct chats only')).not.toBeInTheDocument()
    })

    it('should show the room ID when the room has no name', () => {
      mockMemories([
        { id: 3, memory: 'owes Bob a coffee', room_id: '!unnamed:example.com', created_at: createdAt },
      ])

      render(<MemoriesCard />)
      expect(screen.getByText('!unnamed:example.com')).toBeInTheDocument()
      expect(screen.getByText('and direct chats')).toBeInTheDocument()
    })

    it('should explain how memories are scoped', () => {
      mockMemories([{ id: 1, memory: 'likes tea', room_id: null, created_at: createdAt }])

      render(<MemoriesCard />)
      expect(
        screen.getByText(/A memory saved in a group room is used in that room and in direct chats/)
      ).toBeInTheDocument()
    })
  })

  describe('clearing', () => {
    // Clearing from a group room only clears that room's memories. Clearing here clears every
    // memory, wherever it was saved, so the confirmation says so.
    it('should warn that clearing all removes memories in every room', async () => {
      const user = userEvent.setup()
      mockMemories([
        { id: 1, memory: 'likes tea', room_id: null, created_at: createdAt },
        { id: 2, memory: 'prefers metric units', room_id: '!group:example.com', created_at: createdAt },
      ])

      render(<MemoriesCard />)
      await user.click(screen.getByText('Clear all'))

      expect(screen.getByText('Clear all, in every room?')).toBeInTheDocument()
    })
  })
})
