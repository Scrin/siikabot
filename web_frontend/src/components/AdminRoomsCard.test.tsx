import { describe, it, expect, vi, beforeEach } from 'vitest'
import userEvent from '@testing-library/user-event'
import { render, screen } from '../test/test-utils'
import { AdminRoomsCard } from './AdminRoomsCard'
import * as queries from '../api/queries'
import type { RoomMemberResponse, RoomResponse } from '../api/types'

vi.mock('../api/queries', () => ({
  useAdminRooms: vi.fn(),
  useAdminRoomMembers: vi.fn(),
}))

vi.mock('../hooks/useReducedMotion', () => ({
  useReducedMotion: () => true, // Disable animations for testing
}))

function mockRooms(rooms: RoomResponse[]) {
  vi.mocked(queries.useAdminRooms).mockReturnValue({
    isLoading: false,
    data: { rooms },
    error: null,
  } as unknown as ReturnType<typeof queries.useAdminRooms>)
}

function mockMembers(members: RoomMemberResponse[]) {
  vi.mocked(queries.useAdminRoomMembers).mockReturnValue({
    isLoading: false,
    data: { members },
    error: null,
  } as unknown as ReturnType<typeof queries.useAdminRoomMembers>)
}

describe('AdminRoomsCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockMembers([])
  })

  describe('loading state', () => {
    it('should show loading spinner', () => {
      vi.mocked(queries.useAdminRooms).mockReturnValue({
        isLoading: true,
        data: undefined,
        error: null,
      } as ReturnType<typeof queries.useAdminRooms>)

      render(<AdminRoomsCard />)
      expect(screen.getByText('Loading all rooms...')).toBeInTheDocument()
    })
  })

  describe('error state', () => {
    it('should show error message', () => {
      vi.mocked(queries.useAdminRooms).mockReturnValue({
        isLoading: false,
        data: undefined,
        error: new Error('Failed'),
      } as ReturnType<typeof queries.useAdminRooms>)

      render(<AdminRoomsCard />)
      expect(screen.getByText('Failed to load admin rooms')).toBeInTheDocument()
    })
  })

  describe('empty state', () => {
    it('should show no rooms message', () => {
      mockRooms([])

      render(<AdminRoomsCard />)
      expect(screen.getByText('No rooms found')).toBeInTheDocument()
    })
  })

  describe('rooms', () => {
    it('should count and list every room the bot is in', () => {
      mockRooms([
        { room_id: '!named:example.com', room_name: 'Siika HQ' },
        { room_id: '!unnamed:example.com' },
      ])

      render(<AdminRoomsCard />)
      expect(screen.getByText('Total rooms: 2')).toBeInTheDocument()
      expect(screen.getByText('Siika HQ')).toBeInTheDocument()
      expect(screen.getByText('!unnamed:example.com')).toBeInTheDocument()
    })

    it('should list members by name when expanded, and by ID when they have no name', async () => {
      const user = userEvent.setup()
      mockRooms([{ room_id: '!unnamed:example.com' }])
      mockMembers([
        { user_id: '@alice:example.com', display_name: 'Alice' },
        { user_id: '@nameless:example.com' },
      ])

      render(<AdminRoomsCard />)
      await user.click(screen.getByText('!unnamed:example.com'))

      expect(screen.getByText('Members (2)')).toBeInTheDocument()
      expect(screen.getByText('Alice')).toBeInTheDocument()
      expect(screen.getByText('(@alice:example.com)')).toBeInTheDocument()
      expect(screen.getByText('@nameless:example.com')).toBeInTheDocument()
    })
  })
})
