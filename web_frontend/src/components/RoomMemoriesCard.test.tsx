import { describe, it, expect, vi, beforeEach } from 'vitest'
import userEvent from '@testing-library/user-event'
import { render, screen, within } from '../test/test-utils'
import { RoomMemoriesCard } from './RoomMemoriesCard'
import * as queries from '../api/queries'
import type { ReflectResponse, RoomMemories, RoomMemoryStatus } from '../api/types'

vi.mock('../api/queries', () => ({
  useRoomMemories: vi.fn(),
  useDeleteRoomMemory: vi.fn(),
  useDeleteRoomMemories: vi.fn(),
  useReflectRoom: vi.fn(),
}))

vi.mock('../hooks/useReducedMotion', () => ({
  useReducedMotion: () => true, // Disable animations for testing
}))

const formed = '2026-11-01T12:00:00Z'
const revised = '2026-11-12T18:30:00Z'
const trace = '9d9ac29718f2f0088904516f9e8a7aea'

function status(overrides: Partial<RoomMemoryStatus> = {}): RoomMemoryStatus {
  return {
    state: 'up_to_date',
    due_at: null,
    latest_activity_at: null,
    failed_attempts: 0,
    last_attempt_at: null,
    last_outcome: null,
    last_trace_id: null,
    ...overrides,
  }
}

const eveCorp: RoomMemories = {
  room_id: '!eve:example.com',
  room_name: 'EVE corp',
  last_reflected_at: '2026-11-13T09:15:00Z',
  status: status({
    last_attempt_at: '2026-11-13T09:15:00Z',
    last_outcome: 'changed',
    last_trace_id: trace,
  }),
  memories: [
    {
      id: 1,
      memory: 'Several people here play EVE Online.',
      created_at: formed,
      updated_at: formed,
      created_trace_id: 'aaaaaaaa11111111',
      updated_trace_id: 'aaaaaaaa11111111',
    },
    {
      id: 2,
      memory: 'People here ask for answers in Finnish.',
      created_at: formed,
      updated_at: revised,
      created_trace_id: null,
      updated_trace_id: 'bbbbbbbb22222222',
    },
  ],
}

const unnamed: RoomMemories = {
  room_id: '!unnamed:example.com',
  last_reflected_at: null,
  status: status({ state: 'waiting', due_at: '2026-11-13T23:28:00Z' }),
  memories: [],
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
    reflect?: Record<string, unknown>
  } = {}
) {
  const deleteOne = vi.fn()
  const deleteRoom = vi.fn()
  const reflect = vi.fn()
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
  vi.mocked(queries.useReflectRoom).mockReturnValue({
    mutateAsync: reflect,
    isPending: false,
    isSuccess: false,
    isError: false,
    data: undefined,
    error: null,
    ...overrides.reflect,
  } as unknown as ReturnType<typeof queries.useReflectRoom>)
  return { deleteOne, deleteRoom, reflect }
}

function reflection(overrides: Partial<ReflectResponse> = {}): ReflectResponse {
  return {
    outcome: 'changed',
    skipped: false,
    failed: false,
    conversation_count: 4,
    shown_conversation_count: 4,
    added: 1,
    updated: 0,
    deleted: 0,
    rejected: 0,
    trace_id: trace,
    ...overrides,
  }
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

  it('should say when there are no rooms, and what the card is for', () => {
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

    const other = screen.getByRole('region', { name: 'Memories of !unnamed:example.com' })
    expect(within(other).getByText(/0 memories/)).toBeInTheDocument()
    expect(within(other).getByText('No memories of this room yet')).toBeInTheDocument()
    expect(within(other).queryByText(/last thought back/)).not.toBeInTheDocument()
  })

  describe('how a room stands with the passes', () => {
    it('should say a room is up to date, with how its last pass went and its trace', () => {
      mockRooms([eveCorp])

      render(<RoomMemoriesCard />)
      expect(screen.getByText('Up to date')).toBeInTheDocument()
      expect(screen.getByText(/Last pass: changed the memories/)).toBeInTheDocument()
      expect(screen.getByTitle(trace)).toHaveTextContent('trace 9d9ac297')
    })

    it('should say when a room with new conversations is due', () => {
      mockRooms([unnamed])

      render(<RoomMemoriesCard />)
      expect(screen.getByText(/waiting for the room to be quiet · due/)).toBeInTheDocument()
      expect(screen.queryByText(/Last pass/)).not.toBeInTheDocument()
    })

    it('should say a room is due at the next check', () => {
      mockRooms([{ ...unnamed, status: status({ state: 'due', due_at: formed }) }])

      render(<RoomMemoriesCard />)
      expect(screen.getByText(/Due now/)).toBeInTheDocument()
    })

    it('should say how often the passes over a room have failed, and why the last one did', () => {
      mockRooms([
        {
          ...unnamed,
          status: status({
            state: 'retrying',
            due_at: '2026-11-13T23:40:00Z',
            failed_attempts: 2,
            last_outcome: 'request_failed',
            last_trace_id: trace,
          }),
        },
      ])

      render(<RoomMemoriesCard />)
      expect(screen.getByText(/2 failed passes in a row · retrying/)).toBeInTheDocument()
      expect(screen.getByText(/Last pass: model call failed/)).toBeInTheDocument()
    })

    it('should say when the passes are off', () => {
      mockRooms([{ ...unnamed, status: status({ state: 'off' }) }])

      render(<RoomMemoriesCard />)
      expect(screen.getByText(/the passes are off \(room_memory_delay\)/)).toBeInTheDocument()
    })

    it('should show an outcome it has no description for by its name', () => {
      mockRooms([{ ...eveCorp, status: status({ last_outcome: 'something_new' }) }])

      render(<RoomMemoriesCard />)
      expect(screen.getByText(/Last pass: something_new/)).toBeInTheDocument()
    })
  })

  it('should show the trace of the pass that last wrote each memory', () => {
    mockRooms([eveCorp])

    render(<RoomMemoriesCard />)
    expect(screen.getByTitle('aaaaaaaa11111111')).toHaveTextContent('trace aaaaaaaa')
    expect(screen.getByTitle('bbbbbbbb22222222')).toHaveTextContent('trace bbbbbbbb')
  })

  describe('thinking back now', () => {
    it('should start a pass over the room', async () => {
      const user = userEvent.setup()
      mockRooms([eveCorp])
      const { reflect } = mockMutations()

      render(<RoomMemoriesCard />)
      await user.click(screen.getByRole('button', { name: 'Think back now' }))
      expect(reflect).toHaveBeenCalledWith('!eve:example.com')
    })

    it('should be offered for a room without memories', () => {
      mockRooms([unnamed])

      render(<RoomMemoriesCard />)
      expect(screen.getByRole('button', { name: 'Think back now' })).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: 'Forget this room' })).not.toBeInTheDocument()
    })

    it('should say what the pass changed, with its trace', () => {
      mockRooms([eveCorp])
      mockMutations({
        reflect: {
          isSuccess: true,
          data: reflection({
            conversation_count: 4,
            shown_conversation_count: 3,
            added: 1,
            updated: 2,
            rejected: 1,
            trace_id: 'cccccccc33333333',
          }),
        },
      })

      render(<RoomMemoriesCard />)
      expect(
        screen.getByText(
          'Thought back over 4 conversations (3 shown): 1 added, 2 updated, 1 rejected'
        )
      ).toBeInTheDocument()
      expect(screen.getByTitle('cccccccc33333333')).toBeInTheDocument()
    })

    it('should say when the pass changed nothing', () => {
      mockRooms([eveCorp])
      mockMutations({
        reflect: {
          isSuccess: true,
          data: reflection({
            outcome: 'unchanged',
            conversation_count: 1,
            shown_conversation_count: 1,
            added: 0,
          }),
        },
      })

      render(<RoomMemoriesCard />)
      expect(screen.getByText('Thought back over 1 conversation: no changes')).toBeInTheDocument()
    })

    it('should say when there was nothing new', () => {
      mockRooms([eveCorp])
      mockMutations({
        reflect: {
          isSuccess: true,
          data: reflection({ outcome: 'nothing_new', skipped: true, trace_id: '' }),
        },
      })

      render(<RoomMemoriesCard />)
      expect(screen.getByText('Skipped: nothing new')).toBeInTheDocument()
    })

    it('should say why the pass failed', () => {
      mockRooms([eveCorp])
      mockMutations({
        reflect: { isSuccess: true, data: reflection({ outcome: 'timeout', failed: true }) },
      })

      render(<RoomMemoriesCard />)
      expect(screen.getByText('The pass failed: timed out (turn_timeout)')).toBeInTheDocument()
    })

    it('should say when another pass is running', () => {
      mockRooms([eveCorp])
      mockMutations({
        reflect: {
          isError: true,
          error: new Error('Another pass is running. Try again in a moment.'),
        },
      })

      render(<RoomMemoriesCard />)
      expect(
        screen.getByText('Another pass is running. Try again in a moment.')
      ).toBeInTheDocument()
    })
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
      deleteOne: { isError: true, error: new Error('The memory no longer exists') },
    })

    render(<RoomMemoriesCard />)
    expect(screen.getAllByText('The memory no longer exists').length).toBeGreaterThan(0)
  })
})
