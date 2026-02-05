import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { AuthProvider, useAuth } from './AuthContext'
import * as client from '../api/client'

vi.mock('../api/client', () => ({
  fetchCurrentUser: vi.fn(),
  logout: vi.fn(),
  AuthError: class AuthError extends Error {
    constructor(message: string) {
      super(message)
      this.name = 'AuthError'
    }
  },
}))

const wrapper = ({ children }: { children: ReactNode }) => (
  <AuthProvider>{children}</AuthProvider>
)

describe('AuthContext', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
  })

  describe('initial state', () => {
    it('should start with isLoading true when token exists', async () => {
      localStorage.setItem('siikabot_auth_token', 'some-token')
      vi.mocked(client.fetchCurrentUser).mockImplementation(() => new Promise(() => {}))

      const { result } = renderHook(() => useAuth(), { wrapper })
      expect(result.current.isLoading).toBe(true)
      expect(result.current.isAuthenticated).toBe(false)
    })

    it('should finish loading with no token', async () => {
      const { result } = renderHook(() => useAuth(), { wrapper })

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false)
      })

      expect(result.current.isAuthenticated).toBe(false)
      expect(result.current.token).toBeNull()
    })

    it('should validate stored token on mount', async () => {
      localStorage.setItem('siikabot_auth_token', 'stored-token')
      vi.mocked(client.fetchCurrentUser).mockResolvedValue({
        user_id: '@user:example.com',
        authorizations: { grafana: true },
      })

      const { result } = renderHook(() => useAuth(), { wrapper })

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false)
      })

      expect(result.current.isAuthenticated).toBe(true)
      expect(result.current.userId).toBe('@user:example.com')
      expect(result.current.token).toBe('stored-token')
      expect(result.current.authorizations).toEqual({ grafana: true })
    })

    it('should clear token on AuthError', async () => {
      localStorage.setItem('siikabot_auth_token', 'invalid-token')
      vi.mocked(client.fetchCurrentUser).mockRejectedValue(new client.AuthError('Token expired'))

      const { result } = renderHook(() => useAuth(), { wrapper })

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false)
      })

      expect(result.current.isAuthenticated).toBe(false)
      expect(result.current.token).toBeNull()
      expect(localStorage.getItem('siikabot_auth_token')).toBeNull()
    })

    it('should keep token on server error (non-AuthError)', async () => {
      localStorage.setItem('siikabot_auth_token', 'valid-token')
      vi.mocked(client.fetchCurrentUser).mockRejectedValue(new Error('Server error'))

      const { result } = renderHook(() => useAuth(), { wrapper })

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false)
      })

      // Should assume authenticated but userId unknown
      expect(result.current.isAuthenticated).toBe(true)
      expect(result.current.token).toBe('valid-token')
      expect(result.current.userId).toBeNull()
    })
  })

  describe('login', () => {
    it('should store token and update state', async () => {
      vi.mocked(client.fetchCurrentUser).mockResolvedValue({
        user_id: '@user:example.com',
        authorizations: { grafana: false },
      })

      const { result } = renderHook(() => useAuth(), { wrapper })

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false)
      })

      act(() => {
        result.current.login('new-token', '@newuser:example.com')
      })

      expect(result.current.isAuthenticated).toBe(true)
      expect(result.current.token).toBe('new-token')
      expect(result.current.userId).toBe('@newuser:example.com')
      expect(localStorage.getItem('siikabot_auth_token')).toBe('new-token')
    })

    it('should fetch authorizations after login', async () => {
      vi.mocked(client.fetchCurrentUser).mockResolvedValue({
        user_id: '@user:example.com',
        authorizations: { grafana: true },
      })

      const { result } = renderHook(() => useAuth(), { wrapper })

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false)
      })

      act(() => {
        result.current.login('new-token', '@user:example.com')
      })

      await waitFor(() => {
        expect(result.current.authorizations).toEqual({ grafana: true })
      })
    })
  })

  describe('logout', () => {
    it('should clear state and localStorage', async () => {
      localStorage.setItem('siikabot_auth_token', 'token')
      vi.mocked(client.fetchCurrentUser).mockResolvedValue({
        user_id: '@user:example.com',
        authorizations: { grafana: true },
      })
      vi.mocked(client.logout).mockResolvedValue()

      const { result } = renderHook(() => useAuth(), { wrapper })

      await waitFor(() => {
        expect(result.current.isAuthenticated).toBe(true)
      })

      await act(async () => {
        await result.current.logout()
      })

      expect(result.current.isAuthenticated).toBe(false)
      expect(result.current.token).toBeNull()
      expect(result.current.userId).toBeNull()
      expect(localStorage.getItem('siikabot_auth_token')).toBeNull()
    })

    it('should call API logout', async () => {
      localStorage.setItem('siikabot_auth_token', 'token')
      vi.mocked(client.fetchCurrentUser).mockResolvedValue({
        user_id: '@user:example.com',
        authorizations: { grafana: true },
      })
      vi.mocked(client.logout).mockResolvedValue()

      const { result } = renderHook(() => useAuth(), { wrapper })

      await waitFor(() => {
        expect(result.current.isAuthenticated).toBe(true)
      })

      await act(async () => {
        await result.current.logout()
      })

      expect(client.logout).toHaveBeenCalledWith('token')
    })

    it('should clear local state even if API logout fails', async () => {
      localStorage.setItem('siikabot_auth_token', 'token')
      vi.mocked(client.fetchCurrentUser).mockResolvedValue({
        user_id: '@user:example.com',
        authorizations: { grafana: true },
      })
      vi.mocked(client.logout).mockRejectedValue(new Error('Network error'))

      const { result } = renderHook(() => useAuth(), { wrapper })

      await waitFor(() => {
        expect(result.current.isAuthenticated).toBe(true)
      })

      await act(async () => {
        await result.current.logout()
      })

      // Local state should be cleared regardless of API error
      expect(result.current.isAuthenticated).toBe(false)
      expect(localStorage.getItem('siikabot_auth_token')).toBeNull()
    })
  })

  describe('validateToken', () => {
    it('should return false if no token stored', async () => {
      const { result } = renderHook(() => useAuth(), { wrapper })

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false)
      })

      const isValid = await result.current.validateToken()
      expect(isValid).toBe(false)
    })

    it('should return true and update state on valid token', async () => {
      localStorage.setItem('siikabot_auth_token', 'valid-token')
      vi.mocked(client.fetchCurrentUser).mockResolvedValue({
        user_id: '@user:example.com',
        authorizations: { grafana: true },
      })

      const { result } = renderHook(() => useAuth(), { wrapper })

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false)
      })

      const isValid = await result.current.validateToken()
      expect(isValid).toBe(true)
      expect(result.current.isAuthenticated).toBe(true)
    })

    it('should logout and return false on AuthError', async () => {
      localStorage.setItem('siikabot_auth_token', 'expired-token')
      vi.mocked(client.fetchCurrentUser)
        .mockResolvedValueOnce({
          user_id: '@user:example.com',
          authorizations: { grafana: true },
        })
        .mockRejectedValueOnce(new client.AuthError('Token expired'))
      vi.mocked(client.logout).mockResolvedValue()

      const { result } = renderHook(() => useAuth(), { wrapper })

      await waitFor(() => {
        expect(result.current.isAuthenticated).toBe(true)
      })

      let isValid: boolean
      await act(async () => {
        isValid = await result.current.validateToken()
      })

      expect(isValid!).toBe(false)
      await waitFor(() => {
        expect(result.current.isAuthenticated).toBe(false)
      })
    })

    it('should return true on server error (keep user logged in)', async () => {
      localStorage.setItem('siikabot_auth_token', 'token')
      vi.mocked(client.fetchCurrentUser)
        .mockResolvedValueOnce({
          user_id: '@user:example.com',
          authorizations: { grafana: true },
        })
        .mockRejectedValueOnce(new Error('Server error'))

      const { result } = renderHook(() => useAuth(), { wrapper })

      await waitFor(() => {
        expect(result.current.isAuthenticated).toBe(true)
      })

      const isValid = await result.current.validateToken()
      expect(isValid).toBe(true)
      expect(result.current.isAuthenticated).toBe(true)
    })
  })

  describe('useAuth hook', () => {
    it('should throw error when used outside provider', () => {
      expect(() => {
        renderHook(() => useAuth())
      }).toThrow('useAuth must be used within an AuthProvider')
    })
  })
})
