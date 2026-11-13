import { describe, it, expect, vi, beforeEach } from 'vitest'
import userEvent from '@testing-library/user-event'
import { render, screen, within } from '../test/test-utils'
import { RoomMemoriesCard } from './RoomMemoriesCard'
import * as queries from '../api/queries'
import type { RoomMemories } from '../api/types'

vi.mock('../api/queries', () => ({
  useRoomMemories: vi.fn(),
  useDeleteRoomMemory: vi.fn(),
  useDeleteRoomMemories: vi.fn(),
}))

vi.mock('../hooks/useReducedMotion', () => ({
  useReducedMotion: () => true, // Disable animations for testing
}))

const formed = '2026-11-01T12:00:00Z'
const revised = '2026-11-12T18:30:00Z'

const eveCorp: RoomMemories = {
  room_id: '!eve:example.com',
  room_name: 'EVE corp',
  last_reflected_at: '2026-11-13T09:15:00Z',
  memories: [
    {
      id: 1,
      memory: 'Several people here play EVE Online.',
      created_at: formed,
      updated_at: formed,
    },
    {
      id: 2,
      memory: 'People here ask for answers in Finnish.',
      created_at: formed,
      updated_at: revised,
    },
  ],
}

const unnamed: RoomMemories = {
  room_id: '!unnamed:example.com',
  last_reflected_at: null,
  memories: [
    {
      id: 3,
      memory: 'The room plans a sauna evening.',
      created_at: formed,
      updated_at: formed,
    },
  ],
}

function mockRooms(rooms: RoomMemories[]) {
  vi.mocked(queries.useRoomMemories).mockReturnValue({
    isLoading: false,
    data: { rooms },
    error: null,
  } as unknown as ReturnType<typeof queries.useRoomMemories>)
}

function mockMutations(
  overrides: {
    deleteOne?: Record<string, unknown>
    deleteRoom?: Record<string, unknown>
  } = {}
) {
  const deleteOne = vi.fn()
  const deleteRoom = vi.fn()
  vi.mocked(queries.useDeleteRoomMemory).mockReturnValue({
    mutateAsync: deleteOne,
    isPending: false,
    isError: false,
    error: null,
    ...overrides.deleteOne,
  } as unknown as ReturnType<typeof queries.useDeleteRoomMemory>)
  vi.mocked(queries.useDeleteRoomMemories).mockReturnValue({
    mutateAsync: deleteRoom,
    isPending: false,
    isError: false,
    error: null,
    ...overrides.deleteRoom,
  } as unknown as ReturnType<typeof queries.useDeleteRoomMemories>)
  return { deleteOne, deleteRoom }
}

describe('RoomMemoriesCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockMutations()
  })

  it('should show loading spinner', () => {
    vi.mocked(queries.useRoomMemories).mockReturnValue({
      isLoading: true,
      data: undefined,
      error: null,
    } as unknown as ReturnType<typeof queries.useRoomMemories>)

    render(<RoomMemoriesCard />)
    expect(screen.getByText('Loading room memories...')).toBeInTheDocument()
  })

  it('should show error message', () => {
    vi.mocked(queries.useRoomMemories).mockReturnValue({
      isLoading: false,
      data: undefined,
      error: new Error('Failed'),
    } as unknown as ReturnType<typeof queries.useRoomMemories>)

    render(<RoomMemoriesCard />)
    expect(screen.getByText('Failed to load room memories')).toBeInTheDocument()
  })

  it('should say when the bot remembers nothing, and what the card is for', () => {
    mockRooms([])

    render(<RoomMemoriesCard />)
    expect(screen.getByText('No room memories yet')).toBeInTheDocument()
    expect(screen.getByText(/doesn't mention them unless asked/)).toBeInTheDocument()
  })

  it('should list each room with its memories', () => {
    mockRooms([eveCorp, unnamed])

    render(<RoomMemoriesCard />)
    const eve = screen.getByRole('region', { name: 'Memories of EVE corp' })
    expect(within(eve).getByText('(!eve:example.com)')).toBeInTheDocument()
    expect(within(eve).getByText(/2 memories/)).toBeInTheDocument()
    expect(within(eve).getByText(/last thought back/)).toBeInTheDocument()
    expect(within(eve).getByText('Several people here play EVE Online.')).toBeInTheDocument()
    expect(within(eve).getByText('People here ask for answers in Finnish.')).toBeInTheDocument()
    expect(within(eve).getAllByText(/revised/)).toHaveLength(1)

    const other = screen.getByRole('region', {
      name: 'Memories of !unnamed:example.com',
    })
    expect(within(other).getByText(/1 memory/)).toBeInTheDocument()
    expect(within(other).queryByText(/last thought back/)).not.toBeInTheDocument()
  })

  it('should delete a memory once confirmed', async () => {
    const user = userEvent.setup()
    mockRooms([eveCorp])
    const { deleteOne } = mockMutations()

    render(<RoomMemoriesCard />)
    await user.click(screen.getAllByRole('button', { name: 'Delete' })[1])
    expect(deleteOne).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    expect(deleteOne).toHaveBeenCalledWith(2)
  })

  it('should go back without deleting when cancelled', async () => {
    const user = userEvent.setup()
    mockRooms([eveCorp])
    const { deleteOne } = mockMutations()

    render(<RoomMemoriesCard />)
    await user.click(screen.getAllByRole('button', { name: 'Delete' })[0])
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(deleteOne).not.toHaveBeenCalled()
    expect(screen.getAllByRole('button', { name: 'Delete' })).toHaveLength(2)
  })

  it('should forget a room once confirmed, naming the room', async () => {
    const user = userEvent.setup()
    mockRooms([eveCorp])
    const { deleteRoom } = mockMutations()

    render(<RoomMemoriesCard />)
    await user.click(screen.getByRole('button', { name: 'Forget this room' }))
    expect(screen.getByText('Forget everything about EVE corp?')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Yes' }))
    expect(deleteRoom).toHaveBeenCalledWith('!eve:example.com')
  })

  it('should show why a deletion failed', () => {
    mockRooms([eveCorp])
    mockMutations({
      deleteOne: {
        isError: true,
        error: new Error('The memory no longer exists'),
      },
    })

    render(<RoomMemoriesCard />)
    expect(screen.getAllByText('The memory no longer exists').length).toBeGreaterThan(0)
  })
})
