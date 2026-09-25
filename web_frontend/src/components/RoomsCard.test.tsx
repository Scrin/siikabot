import { describe, it, expect, vi, beforeEach } from 'vitest'
import userEvent from '@testing-library/user-event'
import { render, screen } from '../test/test-utils'
import { RoomsCard } from './RoomsCard'
import * as queries from '../api/queries'
import type { RoomMemberResponse, RoomResponse } from '../api/types'

vi.mock('../api/queries', () => ({
  useRooms: vi.fn(),
  useRoomMembers: vi.fn(),
}))

vi.mock('../hooks/useReducedMotion', () => ({
  useReducedMotion: () => true, // Disable animations for testing
}))

function mockRooms(rooms: RoomResponse[]) {
  vi.mocked(queries.useRooms).mockReturnValue({
    isLoading: false,
    data: { rooms },
    error: null,
  } as unknown as ReturnType<typeof queries.useRooms>)
}

function mockMembers(members: RoomMemberResponse[]) {
  vi.mocked(queries.useRoomMembers).mockReturnValue({
    isLoading: false,
    data: { members },
    error: null,
  } as unknown as ReturnType<typeof queries.useRoomMembers>)
}

describe('RoomsCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockMembers([])
  })

  describe('loading state', () => {
    it('should show loading spinner', () => {
      vi.mocked(queries.useRooms).mockReturnValue({
        isLoading: true,
        data: undefined,
        error: null,
      } as ReturnType<typeof queries.useRooms>)

      render(<RoomsCard />)
      expect(screen.getByText('Loading rooms...')).toBeInTheDocument()
    })
  })

  describe('error state', () => {
    it('should show error message', () => {
      vi.mocked(queries.useRooms).mockReturnValue({
        isLoading: false,
        data: undefined,
        error: new Error('Failed'),
      } as ReturnType<typeof queries.useRooms>)

      render(<RoomsCard />)
      expect(screen.getByText('Failed to load rooms')).toBeInTheDocument()
    })
  })

  describe('empty state', () => {
    it('should show no shared rooms message', () => {
      mockRooms([])

      render(<RoomsCard />)
      expect(screen.getByText('No shared rooms')).toBeInTheDocument()
    })
  })

  describe('rooms', () => {
    it('should show a room by its name, and by its ID when it has none', () => {
      mockRooms([
        { room_id: '!named:example.com', room_name: 'Siika HQ' },
        { room_id: '!unnamed:example.com' },
      ])

      render(<RoomsCard />)
      expect(screen.getByText('Siika HQ')).toBeInTheDocument()
      expect(screen.getByText('(!named:example.com)')).toBeInTheDocument()
      expect(screen.getByText('!unnamed:example.com')).toBeInTheDocument()
    })

    it('should list members by name when expanded, and by ID when they have no name', async () => {
      const user = userEvent.setup()
      mockRooms([{ room_id: '!named:example.com', room_name: 'Siika HQ' }])
      mockMembers([
        { user_id: '@alice:example.com', display_name: 'Alice' },
        { user_id: '@nameless:example.com' },
      ])

      render(<RoomsCard />)
      expect(screen.queryByText('Alice')).not.toBeInTheDocument()

      await user.click(screen.getByText('Siika HQ'))
      expect(screen.getByText('Members (2)')).toBeInTheDocument()
      expect(screen.getByText('Alice')).toBeInTheDocument()
      expect(screen.getByText('(@alice:example.com)')).toBeInTheDocument()
      expect(screen.getByText('@nameless:example.com')).toBeInTheDocument()
    })

    it('should only fetch the members of a room once it is expanded', async () => {
      const user = userEvent.setup()
      mockRooms([{ room_id: '!named:example.com', room_name: 'Siika HQ' }])

      render(<RoomsCard />)
      expect(queries.useRoomMembers).toHaveBeenLastCalledWith(
        '!named:example.com',
        false,
      )

      await user.click(screen.getByText('Siika HQ'))
      expect(queries.useRoomMembers).toHaveBeenLastCalledWith(
        '!named:example.com',
        true,
      )
      expect(screen.getByText('No members found')).toBeInTheDocument()
    })
  })
})
