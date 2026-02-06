import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  useHealthCheck,
  useMetrics,
  useReminders,
  useRooms,
  useGrafanaTemplates,
  queryKeys,
} from './queries'
import { AuthProvider } from '../context/AuthContext'
import * as client from './client'

vi.mock('./client', () => ({
  fetchHealthCheck: vi.fn(),
  fetchMetrics: vi.fn(),
  fetchReminders: vi.fn(),
  fetchRooms: vi.fn(),
  fetchGrafanaTemplates: vi.fn(),
  fetchCurrentUser: vi.fn(),
  logout: vi.fn(),
  AuthError: class AuthError extends Error {
    name = 'AuthError'
  },
}))

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
    },
  })

  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <AuthProvider>{children}</AuthProvider>
      </QueryClientProvider>
    )
  }
}

describe('React Query Hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
  })

  describe('queryKeys', () => {
    it('should have unique keys for each query', () => {
      const keys = Object.values(queryKeys)
      const uniqueKeys = new Set(keys.map((k) => JSON.stringify(k)))
      expect(uniqueKeys.size).toBe(keys.length)
    })

    it('should have correct key structure', () => {
      expect(queryKeys.healthCheck).toEqual(['healthCheck'])
      expect(queryKeys.metrics).toEqual(['metrics'])
      expect(queryKeys.reminders).toEqual(['reminders'])
      expect(queryKeys.rooms).toEqual(['rooms'])
      expect(queryKeys.grafanaTemplates).toEqual(['grafanaTemplates'])
    })
  })

  describe('useHealthCheck', () => {
    it('should fetch health data', async () => {
      const mockData = { status: 'ok', uptime: '1h30m45s' }
      vi.mocked(client.fetchHealthCheck).mockResolvedValue(mockData)

      const { result } = renderHook(() => useHealthCheck(), { wrapper: createWrapper() })

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true)
      })

      expect(result.current.data).toEqual(mockData)
      expect(client.fetchHealthCheck).toHaveBeenCalled()
    })

    it('should handle errors', async () => {
      vi.mocked(client.fetchHealthCheck).mockRejectedValue(new Error('Network error'))

      const { result } = renderHook(() => useHealthCheck(), { wrapper: createWrapper() })

      await waitFor(() => {
        expect(result.current.isError).toBe(true)
      })

      expect(result.current.error?.message).toBe('Network error')
    })
  })

  describe('useMetrics', () => {
    it('should fetch metrics data', async () => {
      const mockData = {
        memory: { resident_mb: 45.5 },
        runtime: { goroutines: 12 },
        database: { active_conns: 2, max_conns: 10, idle_conns: 8 },
        bot: { events_handled: 1234 },
      }
      vi.mocked(client.fetchMetrics).mockResolvedValue(mockData)

      const { result } = renderHook(() => useMetrics(), { wrapper: createWrapper() })

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true)
      })

      expect(result.current.data).toEqual(mockData)
    })
  })

  describe('useReminders', () => {
    it('should not fetch when not authenticated', async () => {
      const { result } = renderHook(() => useReminders(), { wrapper: createWrapper() })

      // Wait for auth to settle
      await waitFor(() => {
        expect(result.current.fetchStatus).toBe('idle')
      })

      expect(client.fetchReminders).not.toHaveBeenCalled()
    })

    it('should fetch when authenticated', async () => {
      // Setup authenticated state
      localStorage.setItem('siikabot_auth_token', 'valid-token')
      vi.mocked(client.fetchCurrentUser).mockResolvedValue({
        user_id: '@user:example.com',
        authorizations: { grafana: true, admin: false },
      })
      vi.mocked(client.fetchReminders).mockResolvedValue({
        reminders: [
          {
            id: 1,
            remind_time: '2026-01-30T12:00:00Z',
            room_id: '!room:example.com',
            message: 'Test',
          },
        ],
      })

      const { result } = renderHook(() => useReminders(), { wrapper: createWrapper() })

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true)
      })

      expect(client.fetchReminders).toHaveBeenCalledWith('valid-token')
    })
  })

  describe('useRooms', () => {
    it('should not fetch when not authenticated', async () => {
      const { result } = renderHook(() => useRooms(), { wrapper: createWrapper() })

      await waitFor(() => {
        expect(result.current.fetchStatus).toBe('idle')
      })

      expect(client.fetchRooms).not.toHaveBeenCalled()
    })

    it('should fetch when authenticated', async () => {
      localStorage.setItem('siikabot_auth_token', 'valid-token')
      vi.mocked(client.fetchCurrentUser).mockResolvedValue({
        user_id: '@user:example.com',
        authorizations: { grafana: true, admin: false },
      })
      vi.mocked(client.fetchRooms).mockResolvedValue({
        rooms: [{ room_id: '!room:example.com', room_name: 'Test Room' }],
      })

      const { result } = renderHook(() => useRooms(), { wrapper: createWrapper() })

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true)
      })

      expect(client.fetchRooms).toHaveBeenCalledWith('valid-token')
    })
  })

  describe('useGrafanaTemplates', () => {
    it('should not fetch when not authenticated', async () => {
      const { result } = renderHook(() => useGrafanaTemplates(), { wrapper: createWrapper() })

      await waitFor(() => {
        expect(result.current.fetchStatus).toBe('idle')
      })

      expect(client.fetchGrafanaTemplates).not.toHaveBeenCalled()
    })

    it('should not fetch when user lacks grafana authorization', async () => {
      localStorage.setItem('siikabot_auth_token', 'valid-token')
      vi.mocked(client.fetchCurrentUser).mockResolvedValue({
        user_id: '@user:example.com',
        authorizations: { grafana: false, admin: false },
      })

      const { result } = renderHook(() => useGrafanaTemplates(), { wrapper: createWrapper() })

      // Wait for auth to settle and authorizations to be fetched
      await waitFor(() => {
        expect(result.current.fetchStatus).toBe('idle')
      })

      expect(client.fetchGrafanaTemplates).not.toHaveBeenCalled()
    })

    it('should fetch when user has grafana authorization', async () => {
      localStorage.setItem('siikabot_auth_token', 'valid-token')
      vi.mocked(client.fetchCurrentUser).mockResolvedValue({
        user_id: '@user:example.com',
        authorizations: { grafana: true, admin: false },
      })
      vi.mocked(client.fetchGrafanaTemplates).mockResolvedValue({
        templates: [{ name: 'test', template: '<html></html>', datasources: [] }],
      })

      const { result } = renderHook(() => useGrafanaTemplates(), { wrapper: createWrapper() })

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true)
      })

      expect(client.fetchGrafanaTemplates).toHaveBeenCalledWith('valid-token')
    })
  })
})
