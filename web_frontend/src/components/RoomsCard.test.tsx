import { describe, it, expect, vi, beforeEach } from 'vitest'
import userEvent from '@testing-library/user-event'
import { render, screen } from '../test/test-utils'
import { RoomsCard } from './RoomsCard'
import * as queries from '../api/queries'
import * as auth from '../context/AuthContext'
import type { RoomMemberResponse, RoomResponse, RoomStatsResponse } from '../api/types'
import { roomStatsFixture } from '../test/statsFixtures'

vi.mock('../api/queries', () => ({
  useRooms: vi.fn(),
  useRoomMembers: vi.fn(),
  useRoomStats: vi.fn(),
  useAdminRooms: vi.fn(),
  useAdminRoomMembers: vi.fn(),
  useAdminRoomStats: vi.fn(),
}))

vi.mock('../context/AuthContext', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../context/AuthContext')>()),
  useAuth: vi.fn(),
}))

vi.mock('../hooks/useReducedMotion', () => ({
  useReducedMotion: () => true, // Disable animations for testing
}))

function mockAdmin(isAdmin: boolean) {
  vi.mocked(auth.useAuth).mockReturnValue({ isAdmin } as ReturnType<typeof auth.useAuth>)
}

function roomsResult(rooms: RoomResponse[]) {
  return { isLoading: false, data: { rooms }, error: null }
}

function membersResult(members: RoomMemberResponse[]) {
  return { isLoading: false, data: { members }, error: null }
}

function statsResult(stats: RoomStatsResponse | undefined) {
  return { isLoading: false, isPlaceholderData: false, data: stats, error: null }
}

function mockRooms(rooms: RoomResponse[]) {
  vi.mocked(queries.useRooms).mockReturnValue(
    roomsResult(rooms) as unknown as ReturnType<typeof queries.useRooms>,
  )
  vi.mocked(queries.useAdminRooms).mockReturnValue(
    roomsResult(rooms) as unknown as ReturnType<typeof queries.useAdminRooms>,
  )
}

function mockMembers(members: RoomMemberResponse[]) {
  vi.mocked(queries.useRoomMembers).mockReturnValue(
    membersResult(members) as unknown as ReturnType<typeof queries.useRoomMembers>,
  )
  vi.mocked(queries.useAdminRoomMembers).mockReturnValue(
    membersResult(members) as unknown as ReturnType<typeof queries.useAdminRoomMembers>,
  )
}

function mockStats(stats: RoomStatsResponse | undefined) {
  vi.mocked(queries.useRoomStats).mockReturnValue(
    statsResult(stats) as unknown as ReturnType<typeof queries.useRoomStats>,
  )
  vi.mocked(queries.useAdminRoomStats).mockReturnValue(
    statsResult(stats) as unknown as ReturnType<typeof queries.useAdminRoomStats>,
  )
}

describe('RoomsCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockMembers([])
    mockStats(undefined)
  })

  describe('for a user', () => {
    beforeEach(() => {
      mockAdmin(false)
    })

    it('should show loading spinner', () => {
      vi.mocked(queries.useRooms).mockReturnValue({
        isLoading: true,
        data: undefined,
        error: null,
      } as ReturnType<typeof queries.useRooms>)

      render(<RoomsCard />)
      expect(screen.getByText('Loading rooms...')).toBeInTheDocument()
    })

    it('should show error message', () => {
      vi.mocked(queries.useRooms).mockReturnValue({
        isLoading: false,
        data: undefined,
        error: new Error('Failed'),
      } as ReturnType<typeof queries.useRooms>)

      render(<RoomsCard />)
      expect(screen.getByText('Failed to load rooms')).toBeInTheDocument()
    })

    it('should show no shared rooms message', () => {
      mockRooms([])

      render(<RoomsCard />)
      expect(screen.getByText('No shared rooms')).toBeInTheDocument()
    })

    it('should list the shared rooms, without a count, and never fetch every room', () => {
      mockRooms([
        { room_id: '!named:example.com', room_name: 'Siika HQ' },
        { room_id: '!unnamed:example.com' },
      ])

      render(<RoomsCard />)
      expect(queries.useRooms).toHaveBeenCalled()
      expect(queries.useAdminRooms).not.toHaveBeenCalled()
      expect(screen.queryByText(/rooms the bot is in/)).not.toBeInTheDocument()
    })

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

    it('should only fetch the members of a room once it is expanded, never through the admin API', async () => {
      const user = userEvent.setup()
      mockRooms([{ room_id: '!named:example.com', room_name: 'Siika HQ' }])

      render(<RoomsCard />)
      expect(queries.useRoomMembers).toHaveBeenLastCalledWith('!named:example.com', false)

      await user.click(screen.getByText('Siika HQ'))
      expect(queries.useRoomMembers).toHaveBeenLastCalledWith('!named:example.com', true)
      expect(queries.useAdminRoomMembers).toHaveBeenLastCalledWith('!named:example.com', false)
      expect(screen.getByText('No members found')).toBeInTheDocument()
    })

    it('should only fetch the stats of a room once it is expanded, for the last 30 days', async () => {
      const user = userEvent.setup()
      mockRooms([{ room_id: '!named:example.com', room_name: 'Siika HQ' }])

      render(<RoomsCard />)
      expect(queries.useRoomStats).toHaveBeenLastCalledWith('!named:example.com', 30, false)

      await user.click(screen.getByText('Siika HQ'))
      expect(queries.useRoomStats).toHaveBeenLastCalledWith('!named:example.com', 30, true)
      expect(queries.useAdminRoomStats).toHaveBeenLastCalledWith('!named:example.com', 30, false)
    })

    it('should show the stats of a room above its members, and fetch another period when picked', async () => {
      const user = userEvent.setup()
      // Picking a period re-renders the expanding panel, whose height framer-motion measures with
      // window.scrollTo, which jsdom doesn't have
      vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
      mockRooms([{ room_id: '!named:example.com', room_name: 'Siika HQ' }])
      mockStats(roomStatsFixture())

      render(<RoomsCard />)
      await user.click(screen.getByText('Siika HQ'))

      expect(screen.getByText('Text messages per day')).toBeInTheDocument()
      expect(screen.getByText('Most messages · all time')).toBeInTheDocument()
      expect(screen.getByText('1,204 · 34%')).toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: '90d' }))
      expect(queries.useRoomStats).toHaveBeenLastCalledWith('!named:example.com', 90, true)
    })
  })

  describe('for an admin', () => {
    beforeEach(() => {
      mockAdmin(true)
    })

    it('should show loading spinner', () => {
      vi.mocked(queries.useAdminRooms).mockReturnValue({
        isLoading: true,
        data: undefined,
        error: null,
      } as ReturnType<typeof queries.useAdminRooms>)

      render(<RoomsCard />)
      expect(screen.getByText('Loading all rooms...')).toBeInTheDocument()
    })

    it('should show error message', () => {
      vi.mocked(queries.useAdminRooms).mockReturnValue({
        isLoading: false,
        data: undefined,
        error: new Error('Failed'),
      } as ReturnType<typeof queries.useAdminRooms>)

      render(<RoomsCard />)
      expect(screen.getByText('Failed to load admin rooms')).toBeInTheDocument()
    })

    it('should show no rooms message', () => {
      mockRooms([])

      render(<RoomsCard />)
      expect(screen.getByText('No rooms found')).toBeInTheDocument()
    })

    it('should count and list every room the bot is in, and never fetch the shared rooms', () => {
      mockRooms([
        { room_id: '!named:example.com', room_name: 'Siika HQ' },
        { room_id: '!unnamed:example.com' },
      ])

      render(<RoomsCard />)
      expect(queries.useAdminRooms).toHaveBeenCalled()
      expect(queries.useRooms).not.toHaveBeenCalled()
      expect(screen.getByText('All 2 rooms the bot is in')).toBeInTheDocument()
      expect(screen.getByText('Siika HQ')).toBeInTheDocument()
      expect(screen.getByText('!unnamed:example.com')).toBeInTheDocument()
    })

    it('should count a single room in the singular', () => {
      mockRooms([{ room_id: '!named:example.com', room_name: 'Siika HQ' }])

      render(<RoomsCard />)
      expect(screen.getByText('The one room the bot is in')).toBeInTheDocument()
    })

    it('should list members by name when expanded, and by ID when they have no name', async () => {
      const user = userEvent.setup()
      mockRooms([{ room_id: '!unnamed:example.com' }])
      mockMembers([
        { user_id: '@alice:example.com', display_name: 'Alice' },
        { user_id: '@nameless:example.com' },
      ])

      render(<RoomsCard />)
      await user.click(screen.getByText('!unnamed:example.com'))

      expect(screen.getByText('Members (2)')).toBeInTheDocument()
      expect(screen.getByText('Alice')).toBeInTheDocument()
      expect(screen.getByText('(@alice:example.com)')).toBeInTheDocument()
      expect(screen.getByText('@nameless:example.com')).toBeInTheDocument()
    })

    it('should fetch the members and stats of an expanded room through the admin API only', async () => {
      const user = userEvent.setup()
      mockRooms([{ room_id: '!named:example.com', room_name: 'Siika HQ' }])

      render(<RoomsCard />)
      expect(queries.useAdminRoomMembers).toHaveBeenLastCalledWith('!named:example.com', false)
      expect(queries.useAdminRoomStats).toHaveBeenLastCalledWith('!named:example.com', 30, false)

      await user.click(screen.getByText('Siika HQ'))
      expect(queries.useAdminRoomMembers).toHaveBeenLastCalledWith('!named:example.com', true)
      expect(queries.useAdminRoomStats).toHaveBeenLastCalledWith('!named:example.com', 30, true)
      expect(queries.useRoomMembers).toHaveBeenLastCalledWith('!named:example.com', false)
      expect(queries.useRoomStats).toHaveBeenLastCalledWith('!named:example.com', 30, false)
    })

    it('should show the stats of a room above its members, and fetch another period when picked', async () => {
      const user = userEvent.setup()
      // Picking a period re-renders the expanding panel, whose height framer-motion measures with
      // window.scrollTo, which jsdom doesn't have
      vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
      mockRooms([{ room_id: '!named:example.com', room_name: 'Siika HQ' }])
      mockStats(roomStatsFixture())

      render(<RoomsCard />)
      await user.click(screen.getByText('Siika HQ'))

      expect(screen.getByText('Text messages per day')).toBeInTheDocument()
      expect(screen.getByText('Most messages · all time')).toBeInTheDocument()
      expect(screen.getByText('1,204 · 34%')).toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: '90d' }))
      expect(queries.useAdminRoomStats).toHaveBeenLastCalledWith('!named:example.com', 90, true)
    })
  })
})
