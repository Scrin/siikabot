import { describe, it, expect, vi, beforeEach } from 'vitest'
import userEvent from '@testing-library/user-event'
import { render, screen, within } from '../test/test-utils'
import { WebhooksCard } from './WebhooksCard'
import * as queries from '../api/queries'
import type { RoomResponse, WebhookResponse } from '../api/types'

vi.mock('../api/queries', () => ({
  useWebhooks: vi.fn(),
  useAdminRooms: vi.fn(),
  useCreateWebhook: vi.fn(),
  useUpdateWebhook: vi.fn(),
  useDeleteWebhook: vi.fn(),
}))

vi.mock('../hooks/useReducedMotion', () => ({
  useReducedMotion: () => true, // Disable animations for testing
}))

const mockClipboard = {
  writeText: vi.fn().mockResolvedValue(undefined),
}
Object.assign(navigator, { clipboard: mockClipboard })

const hermes: WebhookResponse = {
  id: 'Zk3x9QpL2mR7tY1v',
  name: 'hermes',
  room_id: '!ops:example.com',
  room_name: 'Ops',
  bot_in_room: true,
  url: 'https://bot.example.com/hooks/message/Zk3x9QpL2mR7tY1v',
  created_at: '2026-10-20T08:00:00Z',
  last_used_at: null,
}

const rooms: RoomResponse[] = [
  { room_id: '!ops:example.com', room_name: 'Ops' },
  { room_id: '!alerts:example.com', room_name: 'Alerts' },
  { room_id: '!unnamed:example.com' },
]

function mockWebhooks(webhooks: WebhookResponse[]) {
  vi.mocked(queries.useWebhooks).mockReturnValue({
    isLoading: false,
    data: { webhooks },
    error: null,
  } as unknown as ReturnType<typeof queries.useWebhooks>)
}

function mockMutation(overrides: Record<string, unknown> = {}) {
  return {
    mutateAsync: vi.fn().mockResolvedValue(undefined),
    isPending: false,
    isError: false,
    error: null,
    ...overrides,
  }
}

let createWebhook: ReturnType<typeof mockMutation>
let updateWebhook: ReturnType<typeof mockMutation>
let deleteWebhook: ReturnType<typeof mockMutation>

function mockMutationHooks() {
  vi.mocked(queries.useCreateWebhook).mockReturnValue(
    createWebhook as unknown as ReturnType<typeof queries.useCreateWebhook>
  )
  vi.mocked(queries.useUpdateWebhook).mockReturnValue(
    updateWebhook as unknown as ReturnType<typeof queries.useUpdateWebhook>
  )
  vi.mocked(queries.useDeleteWebhook).mockReturnValue(
    deleteWebhook as unknown as ReturnType<typeof queries.useDeleteWebhook>
  )
}

describe('WebhooksCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    createWebhook = mockMutation()
    updateWebhook = mockMutation()
    deleteWebhook = mockMutation()
    mockMutationHooks()
    vi.mocked(queries.useAdminRooms).mockReturnValue({
      isLoading: false,
      data: { rooms },
      error: null,
    } as unknown as ReturnType<typeof queries.useAdminRooms>)
  })

  describe('loading state', () => {
    it('should show loading spinner', () => {
      vi.mocked(queries.useWebhooks).mockReturnValue({
        isLoading: true,
        data: undefined,
        error: null,
      } as ReturnType<typeof queries.useWebhooks>)

      render(<WebhooksCard />)
      expect(screen.getByText('Loading webhooks...')).toBeInTheDocument()
    })
  })

  describe('error state', () => {
    it('should show error message', () => {
      vi.mocked(queries.useWebhooks).mockReturnValue({
        isLoading: false,
        data: undefined,
        error: new Error('Failed'),
      } as ReturnType<typeof queries.useWebhooks>)

      render(<WebhooksCard />)
      expect(screen.getByText('Failed to load webhooks')).toBeInTheDocument()
    })
  })

  describe('empty state', () => {
    it('should say there are no webhooks, and still offer to create one', () => {
      mockWebhooks([])

      render(<WebhooksCard />)
      expect(screen.getByText('No webhooks yet')).toBeInTheDocument()
      expect(screen.getByText('New webhook')).toBeInTheDocument()
    })
  })

  describe('list', () => {
    it('should show the name, room, URL and when it was created', () => {
      mockWebhooks([hermes])

      render(<WebhooksCard />)
      expect(screen.getByText('[hermes]')).toBeInTheDocument()
      expect(screen.getByText('Ops')).toBeInTheDocument()
      expect(screen.getByText('(!ops:example.com)')).toBeInTheDocument()
      expect(screen.getByText(hermes.url)).toBeInTheDocument()
      expect(screen.getByText(/^Created \d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/)).toBeInTheDocument()
    })

    it('should say a webhook that has never posted was never used', () => {
      mockWebhooks([hermes])

      render(<WebhooksCard />)
      expect(screen.getByText('Last used never')).toBeInTheDocument()
    })

    it('should say when a webhook was last used', () => {
      mockWebhooks([{ ...hermes, last_used_at: '2026-10-27T12:30:00Z' }])

      render(<WebhooksCard />)
      expect(
        screen.getByText(/^Last used \d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/)
      ).toBeInTheDocument()
    })

    it('should show the room ID when the room has no name', () => {
      mockWebhooks([{ ...hermes, room_id: '!unnamed:example.com', room_name: undefined }])

      render(<WebhooksCard />)
      expect(screen.getByText('!unnamed:example.com')).toBeInTheDocument()
    })

    it('should flag a webhook whose room the bot has left', () => {
      mockWebhooks([{ ...hermes, bot_in_room: false }])

      render(<WebhooksCard />)
      expect(screen.getByText(/The bot has left this room/)).toBeInTheDocument()
    })

    it('should not flag a webhook whose room the bot is in', () => {
      mockWebhooks([hermes])

      render(<WebhooksCard />)
      expect(screen.queryByText(/The bot has left this room/)).not.toBeInTheDocument()
    })

    it('should copy the URL', async () => {
      const user = userEvent.setup()
      const writeText = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined)
      mockWebhooks([hermes])

      render(<WebhooksCard />)
      await user.click(screen.getByRole('button', { name: 'Copy URL' }))

      expect(writeText).toHaveBeenCalledWith(hermes.url)
    })
  })

  describe('creating', () => {
    it('should create a webhook with a typed token, and then show the token once', async () => {
      const user = userEvent.setup()
      mockWebhooks([])
      createWebhook.mutateAsync.mockResolvedValue({
        webhook: {
          ...hermes,
          room_id: '!alerts:example.com',
          room_name: 'Alerts',
        },
      })

      render(<WebhooksCard />)
      await user.click(screen.getByText('New webhook'))
      await user.type(screen.getByLabelText('Name'), '  hermes ')
      await user.selectOptions(screen.getByLabelText('Room'), '!alerts:example.com')
      await user.type(screen.getByLabelText('Token'), 'my-own-secret-token-123')
      await user.click(screen.getByRole('button', { name: 'Create' }))

      expect(createWebhook.mutateAsync).toHaveBeenCalledWith({
        name: 'hermes',
        room_id: '!alerts:example.com',
        token: 'my-own-secret-token-123',
      })
      expect(screen.queryByLabelText('Name')).not.toBeInTheDocument()
      expect(screen.getByText('my-own-secret-token-123')).toBeInTheDocument()
      expect(screen.getByText(/can't be shown again/)).toBeInTheDocument()
      expect(
        screen.getByText(
          `curl -H 'Authorization: Bearer my-own-secret-token-123' --data-binary 'hello there' ${hermes.url}`
        )
      ).toBeInTheDocument()
    })

    it('should create a webhook with a generated token', async () => {
      const user = userEvent.setup()
      mockWebhooks([])
      createWebhook.mutateAsync.mockResolvedValue({ webhook: hermes })

      render(<WebhooksCard />)
      await user.click(screen.getByText('New webhook'))
      await user.type(screen.getByLabelText('Name'), 'hermes')
      await user.selectOptions(screen.getByLabelText('Room'), '!ops:example.com')
      await user.click(screen.getByRole('button', { name: 'Generate' }))

      const token = (screen.getByLabelText('Token') as HTMLInputElement).value
      expect(token).toMatch(/^[A-Za-z0-9_-]{43}$/)

      await user.click(screen.getByRole('button', { name: 'Create' }))
      expect(createWebhook.mutateAsync).toHaveBeenCalledWith({
        name: 'hermes',
        room_id: '!ops:example.com',
        token,
      })
      expect(screen.getByText(token)).toBeInTheDocument()
    })

    it('should offer only the rooms the bot is in, and name them', async () => {
      const user = userEvent.setup()
      mockWebhooks([])

      render(<WebhooksCard />)
      await user.click(screen.getByText('New webhook'))

      const options = within(screen.getByLabelText('Room'))
        .getAllByRole('option')
        .map((option) => option.textContent)
      expect(options).toEqual([
        'Choose a room',
        'Ops (!ops:example.com)',
        'Alerts (!alerts:example.com)',
        '!unnamed:example.com',
      ])
    })

    it('should say what is wrong instead of sending an invalid webhook', async () => {
      const user = userEvent.setup()
      mockWebhooks([])

      render(<WebhooksCard />)
      await user.click(screen.getByText('New webhook'))
      await user.click(screen.getByRole('button', { name: 'Create' }))

      expect(screen.getByText('The name is required')).toBeInTheDocument()
      expect(screen.getByText('The room is required')).toBeInTheDocument()
      expect(screen.getByText('The token is required')).toBeInTheDocument()
      expect(createWebhook.mutateAsync).not.toHaveBeenCalled()
    })

    it('should refuse a token the server would refuse', async () => {
      const user = userEvent.setup()
      mockWebhooks([])

      render(<WebhooksCard />)
      await user.click(screen.getByText('New webhook'))
      await user.type(screen.getByLabelText('Name'), 'hermes')
      await user.selectOptions(screen.getByLabelText('Room'), '!ops:example.com')
      await user.type(screen.getByLabelText('Token'), 'too short')
      await user.click(screen.getByRole('button', { name: 'Create' }))

      expect(screen.getByText('The token must be at least 16 characters')).toBeInTheDocument()
      expect(createWebhook.mutateAsync).not.toHaveBeenCalled()
    })

    it("should show the server's error", async () => {
      const user = userEvent.setup()
      mockWebhooks([])
      createWebhook = mockMutation({
        isError: true,
        error: new Error('A webhook named hermes already exists'),
      })
      mockMutationHooks()

      render(<WebhooksCard />)
      await user.click(screen.getByText('New webhook'))

      expect(screen.getByText('A webhook named hermes already exists')).toBeInTheDocument()
    })

    it('should close the form on cancel without sending anything', async () => {
      const user = userEvent.setup()
      mockWebhooks([])

      render(<WebhooksCard />)
      await user.click(screen.getByText('New webhook'))
      await user.click(screen.getByRole('button', { name: 'Cancel' }))

      expect(screen.queryByLabelText('Name')).not.toBeInTheDocument()
      expect(createWebhook.mutateAsync).not.toHaveBeenCalled()
    })

    it('should hide the token once dismissed', async () => {
      const user = userEvent.setup()
      mockWebhooks([])
      createWebhook.mutateAsync.mockResolvedValue({ webhook: hermes })

      render(<WebhooksCard />)
      await user.click(screen.getByText('New webhook'))
      await user.type(screen.getByLabelText('Name'), 'hermes')
      await user.selectOptions(screen.getByLabelText('Room'), '!ops:example.com')
      await user.type(screen.getByLabelText('Token'), 'my-own-secret-token-123')
      await user.click(screen.getByRole('button', { name: 'Create' }))
      await user.click(screen.getByRole('button', { name: 'Done' }))

      expect(screen.queryByText('my-own-secret-token-123')).not.toBeInTheDocument()
    })
  })

  describe('editing', () => {
    it('should fill the form in with the current name and room, and leave the token empty', async () => {
      const user = userEvent.setup()
      mockWebhooks([hermes])

      render(<WebhooksCard />)
      await user.click(screen.getByRole('button', { name: 'Edit' }))

      expect(screen.getByLabelText('Name')).toHaveValue('hermes')
      expect(screen.getByLabelText('Room')).toHaveValue('!ops:example.com')
      expect(screen.getByLabelText('Token')).toHaveValue('')
      expect(
        screen.getByPlaceholderText('Leave empty to keep the current token')
      ).toBeInTheDocument()
    })

    it('should not save when nothing has changed', async () => {
      const user = userEvent.setup()
      mockWebhooks([hermes])

      render(<WebhooksCard />)
      await user.click(screen.getByRole('button', { name: 'Edit' }))

      expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
    })

    it('should send only the new name when renaming, and show no token', async () => {
      const user = userEvent.setup()
      mockWebhooks([hermes])
      updateWebhook.mutateAsync.mockResolvedValue({
        webhook: { ...hermes, name: 'iris' },
      })

      render(<WebhooksCard />)
      await user.click(screen.getByRole('button', { name: 'Edit' }))
      await user.clear(screen.getByLabelText('Name'))
      await user.type(screen.getByLabelText('Name'), 'iris')
      await user.click(screen.getByRole('button', { name: 'Save' }))

      expect(updateWebhook.mutateAsync).toHaveBeenCalledWith({
        id: hermes.id,
        changes: { name: 'iris' },
      })
      expect(screen.queryByLabelText('Name')).not.toBeInTheDocument()
      expect(screen.queryByText(/can't be shown again/)).not.toBeInTheDocument()
    })

    it('should send only the new room when moving', async () => {
      const user = userEvent.setup()
      mockWebhooks([hermes])
      updateWebhook.mutateAsync.mockResolvedValue({
        webhook: { ...hermes, room_id: '!alerts:example.com' },
      })

      render(<WebhooksCard />)
      await user.click(screen.getByRole('button', { name: 'Edit' }))
      await user.selectOptions(screen.getByLabelText('Room'), '!alerts:example.com')
      await user.click(screen.getByRole('button', { name: 'Save' }))

      expect(updateWebhook.mutateAsync).toHaveBeenCalledWith({
        id: hermes.id,
        changes: { room_id: '!alerts:example.com' },
      })
    })

    it('should send a new token, and then show it once', async () => {
      const user = userEvent.setup()
      mockWebhooks([hermes])
      updateWebhook.mutateAsync.mockResolvedValue({ webhook: hermes })

      render(<WebhooksCard />)
      await user.click(screen.getByRole('button', { name: 'Edit' }))
      await user.type(screen.getByLabelText('Token'), 'a-brand-new-token-456')
      await user.click(screen.getByRole('button', { name: 'Save' }))

      expect(updateWebhook.mutateAsync).toHaveBeenCalledWith({
        id: hermes.id,
        changes: { token: 'a-brand-new-token-456' },
      })
      expect(screen.getByText('a-brand-new-token-456')).toBeInTheDocument()
      expect(screen.getByText(/can't be shown again/)).toBeInTheDocument()
    })

    it('should keep offering the room the bot has left, flagged, so it can be renamed without moving', async () => {
      const user = userEvent.setup()
      const left = {
        ...hermes,
        room_id: '!old:example.com',
        room_name: 'Old room',
        bot_in_room: false,
      }
      mockWebhooks([left])
      updateWebhook.mutateAsync.mockResolvedValue({
        webhook: { ...left, name: 'iris' },
      })

      render(<WebhooksCard />)
      await user.click(screen.getByRole('button', { name: 'Edit' }))

      const room = screen.getByLabelText('Room')
      expect(room).toHaveValue('!old:example.com')
      expect(
        within(room).getByRole('option', {
          name: 'Old room (!old:example.com) (the bot has left this room)',
        })
      ).toBeInTheDocument()

      await user.clear(screen.getByLabelText('Name'))
      await user.type(screen.getByLabelText('Name'), 'iris')
      await user.click(screen.getByRole('button', { name: 'Save' }))

      expect(updateWebhook.mutateAsync).toHaveBeenCalledWith({
        id: left.id,
        changes: { name: 'iris' },
      })
    })
  })

  describe('deleting', () => {
    it('should ask before deleting', async () => {
      const user = userEvent.setup()
      mockWebhooks([hermes])

      render(<WebhooksCard />)
      await user.click(screen.getByRole('button', { name: 'Delete' }))

      expect(screen.getByText('Delete this webhook?')).toBeInTheDocument()
      expect(deleteWebhook.mutateAsync).not.toHaveBeenCalled()

      await user.click(screen.getByRole('button', { name: 'Confirm' }))
      expect(deleteWebhook.mutateAsync).toHaveBeenCalledWith(hermes.id)
    })

    it('should not delete when cancelled', async () => {
      const user = userEvent.setup()
      mockWebhooks([hermes])

      render(<WebhooksCard />)
      await user.click(screen.getByRole('button', { name: 'Delete' }))
      await user.click(screen.getByRole('button', { name: 'Cancel' }))

      expect(deleteWebhook.mutateAsync).not.toHaveBeenCalled()
      expect(screen.queryByText('Delete this webhook?')).not.toBeInTheDocument()
    })
  })
})
