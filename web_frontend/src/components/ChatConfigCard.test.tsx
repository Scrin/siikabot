import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '../test/test-utils'
import { ChatConfigCard } from './ChatConfigCard'
import * as queries from '../api/queries'
import type { ChatConfigResponse, ChatSetting } from '../api/types'

vi.mock('../api/queries', () => ({
  useChatConfig: vi.fn(),
}))

vi.mock('../hooks/useReducedMotion', () => ({
  useReducedMotion: () => true, // Disable animations for testing
}))

function setting(overrides: Partial<ChatSetting> = {}): ChatSetting {
  return {
    key: 'turn_timeout',
    group: 'Time',
    value: '3m',
    unit: '',
    summary: 'the longest one turn may take, tools included',
    command: '!chat set turn_timeout 10m',
    ...overrides,
  }
}

function response(overrides: Partial<ChatConfigResponse> = {}): ChatConfigResponse {
  return {
    id: 3,
    created_at: '2026-10-21T12:00:00Z',
    settings: [
      setting({
        key: 'model',
        group: 'Model',
        value: 'openrouter/deepseek/deepseek-v4.1-flash',
        summary: 'the model that answers every turn',
        command: '!chat set model <name>',
      }),
      setting({
        key: 'context_high_tokens',
        group: 'Context window',
        value: '16384',
        unit: 'tokens',
        summary: "the most history the window holds before it's trimmed",
        command: '!chat set context_high_tokens 32k',
      }),
      setting(),
      setting({
        key: 'model_timeout',
        value: '45s',
        summary: 'the longest one attempt at a model call may take',
        command: '!chat set model_timeout 30s',
      }),
    ],
    history: [
      {
        id: 3,
        created_at: '2026-10-21T12:00:00Z',
        changes: [
          { key: 'turn_timeout', from: '10m', to: '3m', unit: '' },
          { key: 'context_high_tokens', from: '8192', to: '16384', unit: 'tokens' },
        ],
        first: false,
      },
      { id: 2, created_at: '2026-10-20T12:00:00Z', changes: [], first: false },
      { id: 1, created_at: '2026-09-25T12:27:00Z', changes: [], first: true },
    ],
    ...overrides,
  }
}

function mockConfig(value: Partial<ReturnType<typeof queries.useChatConfig>>) {
  vi.mocked(queries.useChatConfig).mockReturnValue(
    value as ReturnType<typeof queries.useChatConfig>
  )
}

describe('ChatConfigCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('loading state', () => {
    it('should show loading spinner', () => {
      mockConfig({ isLoading: true, data: undefined, error: null })

      render(<ChatConfigCard />)
      expect(screen.getByText('Loading chat configuration...')).toBeInTheDocument()
    })
  })

  describe('error state', () => {
    it('should show error message', () => {
      mockConfig({ isLoading: false, data: undefined, error: new Error('Failed') })

      render(<ChatConfigCard />)
      expect(screen.getByText('Failed to load chat configuration')).toBeInTheDocument()
    })
  })

  describe('settings', () => {
    it('should list each setting under its heading, in order', () => {
      mockConfig({ isLoading: false, data: response(), error: null })

      render(<ChatConfigCard />)

      const headings = ['Model', 'Context window', 'Time'].map((name) => screen.getByText(name))
      expect(headings[0].compareDocumentPosition(headings[1])).toBe(
        Node.DOCUMENT_POSITION_FOLLOWING
      )
      expect(headings[1].compareDocumentPosition(headings[2])).toBe(
        Node.DOCUMENT_POSITION_FOLLOWING
      )
      // Settings of one group come under one heading
      expect(screen.getAllByText('Time')).toHaveLength(1)
    })

    it('should show each value with its unit and what the setting is', () => {
      mockConfig({ isLoading: false, data: response(), error: null })

      render(<ChatConfigCard />)

      expect(screen.getByText('context_high_tokens')).toBeInTheDocument()
      expect(screen.getByText('16384 tokens')).toBeInTheDocument()
      expect(screen.getByText('45s')).toBeInTheDocument()
      expect(screen.getByText('the longest one turn may take, tools included')).toBeInTheDocument()
    })

    // The card is read-only, so what it can do is say exactly how to change each setting
    it('should show the command that changes each setting', () => {
      mockConfig({ isLoading: false, data: response(), error: null })

      render(<ChatConfigCard />)

      expect(screen.getByText('!chat set model <name>')).toBeInTheDocument()
      expect(screen.getByText('!chat set context_high_tokens 32k')).toBeInTheDocument()
    })

    it('should say which configuration is in effect', () => {
      mockConfig({ isLoading: false, data: response(), error: null })

      render(<ChatConfigCard />)
      expect(screen.getByText(/Configuration #3, set/)).toBeInTheDocument()
    })
  })

  describe('history', () => {
    it('should say what each configuration changed', () => {
      mockConfig({ isLoading: false, data: response(), error: null })

      render(<ChatConfigCard />)

      expect(
        screen.getByText(/turn_timeout 10m → 3m, context_high_tokens 8192 → 16384 tokens/)
      ).toBeInTheDocument()
      expect(screen.getByText(/no change/)).toBeInTheDocument()
      expect(screen.getByText(/the first configuration/)).toBeInTheDocument()
    })

    it('should say how to go back to an earlier configuration', () => {
      mockConfig({ isLoading: false, data: response(), error: null })

      render(<ChatConfigCard />)
      expect(screen.getByText('!chat config revert <id>')).toBeInTheDocument()
    })
  })
})
