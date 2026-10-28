import { describe, it, expect, vi, beforeEach } from 'vitest'
import {
  fetchHealthCheck,
  fetchMetrics,
  requestAuthChallenge,
  pollAuthStatus,
  fetchCurrentUser,
  logout,
  fetchReminders,
  fetchRooms,
  fetchRoomStats,
  fetchAdminRoomStats,
  fetchMessageActivity,
  fetchWebhooks,
  createWebhook,
  updateWebhook,
  deleteWebhook,
  AuthError,
} from './client'

describe('API Client', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    globalThis.fetch = vi.fn()
  })

  describe('AuthError', () => {
    it('should be an instance of Error', () => {
      const error = new AuthError('Token expired')
      expect(error).toBeInstanceOf(Error)
      expect(error.name).toBe('AuthError')
      expect(error.message).toBe('Token expired')
    })
  })

  describe('fetchHealthCheck', () => {
    it('should return health data on success', async () => {
      const mockResponse = { status: 'ok', uptime: '1h30m45s' }
      vi.mocked(fetch).mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      } as Response)

      const result = await fetchHealthCheck()
      expect(result).toEqual(mockResponse)
      expect(fetch).toHaveBeenCalledWith('/api/healthcheck')
    })

    it('should throw error on non-ok response', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: false,
        statusText: 'Internal Server Error',
      } as Response)

      await expect(fetchHealthCheck()).rejects.toThrow(
        'Health check failed: Internal Server Error'
      )
    })
  })

  describe('fetchMetrics', () => {
    it('should return metrics data on success', async () => {
      const mockResponse = {
        memory: { resident_mb: 45.5 },
        runtime: { goroutines: 12 },
        database: { active_conns: 2, max_conns: 10, idle_conns: 8 },
        bot: { events_handled: 1234 },
      }
      vi.mocked(fetch).mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      } as Response)

      const result = await fetchMetrics()
      expect(result).toEqual(mockResponse)
      expect(fetch).toHaveBeenCalledWith('/api/metrics')
    })

    it('should throw error on non-ok response', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: false,
        statusText: 'Service Unavailable',
      } as Response)

      await expect(fetchMetrics()).rejects.toThrow('Metrics fetch failed: Service Unavailable')
    })
  })

  describe('requestAuthChallenge', () => {
    it('should return challenge data on success', async () => {
      const mockResponse = {
        challenge: 'abc123',
        poll_secret: 'secret',
        expires_at: '2026-01-30T12:00:00Z',
      }
      vi.mocked(fetch).mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      } as Response)

      const result = await requestAuthChallenge()
      expect(result).toEqual(mockResponse)
      expect(fetch).toHaveBeenCalledWith('/api/auth/challenge', { method: 'POST' })
    })

    it('should parse error from response body on failure', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: false,
        json: () => Promise.resolve({ error: 'Rate limited' }),
      } as Response)

      await expect(requestAuthChallenge()).rejects.toThrow('Rate limited')
    })

    it('should use statusText when JSON parsing fails', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: false,
        statusText: 'Bad Gateway',
        json: () => Promise.reject(new Error('Invalid JSON')),
      } as Response)

      await expect(requestAuthChallenge()).rejects.toThrow('Bad Gateway')
    })
  })

  describe('pollAuthStatus', () => {
    it('should encode challenge and pollSecret in URL', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ status: 'pending' }),
      } as Response)

      await pollAuthStatus('chal lenge', 'sec/ret')
      expect(fetch).toHaveBeenCalledWith(
        '/api/auth/poll?challenge=chal%20lenge&poll_secret=sec%2Fret'
      )
    })

    it('should return poll response on success', async () => {
      const mockResponse = { status: 'authenticated', token: 'token123', user_id: '@user:example.com' }
      vi.mocked(fetch).mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      } as Response)

      const result = await pollAuthStatus('challenge', 'secret')
      expect(result).toEqual(mockResponse)
    })

    it('should throw error on failure', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: false,
        json: () => Promise.resolve({ error: 'Invalid challenge' }),
      } as Response)

      await expect(pollAuthStatus('bad', 'secret')).rejects.toThrow('Invalid challenge')
    })
  })

  describe('fetchCurrentUser', () => {
    it('should include Authorization header', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: true,
        json: () =>
          Promise.resolve({ user_id: '@user:example.com', authorizations: { admin: true } }),
      } as Response)

      await fetchCurrentUser('my-token')
      expect(fetch).toHaveBeenCalledWith('/api/auth/me', {
        headers: { Authorization: 'Bearer my-token' },
      })
    })

    it('should return user data on success', async () => {
      const mockResponse = { user_id: '@user:example.com', authorizations: { admin: true } }
      vi.mocked(fetch).mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      } as Response)

      const result = await fetchCurrentUser('token')
      expect(result).toEqual(mockResponse)
    })

    it('should throw AuthError on 401', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: false,
        status: 401,
        json: () => Promise.resolve({ error: 'Unauthorized' }),
      } as Response)

      await expect(fetchCurrentUser('bad-token')).rejects.toThrow(AuthError)
      await expect(fetchCurrentUser('bad-token')).rejects.toThrow('Token invalid or expired')
    })

    it('should throw regular Error on other status codes', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: false,
        status: 500,
        json: () => Promise.resolve({ error: 'Server error' }),
      } as Response)

      const error = await fetchCurrentUser('token').catch((e) => e)
      expect(error).not.toBeInstanceOf(AuthError)
      expect(error.message).toBe('Server error')
    })
  })

  describe('logout', () => {
    it('should call logout endpoint with token', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: true,
      } as Response)

      await logout('my-token')
      expect(fetch).toHaveBeenCalledWith('/api/auth/logout', {
        method: 'POST',
        headers: { Authorization: 'Bearer my-token' },
      })
    })

    it('should not throw on 401 (already logged out)', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: false,
        status: 401,
        json: () => Promise.resolve({ error: 'Unauthorized' }),
      } as Response)

      await expect(logout('token')).resolves.not.toThrow()
    })

    it('should throw on other errors', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: false,
        status: 500,
        json: () => Promise.resolve({ error: 'Server error' }),
      } as Response)

      await expect(logout('token')).rejects.toThrow('Server error')
    })
  })

  describe('fetchReminders', () => {
    it('should include Authorization header', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ reminders: [] }),
      } as Response)

      await fetchReminders('my-token')
      expect(fetch).toHaveBeenCalledWith('/api/reminders', {
        headers: { Authorization: 'Bearer my-token' },
      })
    })

    it('should throw AuthError on 401', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: false,
        status: 401,
        json: () => Promise.resolve({ error: 'Unauthorized' }),
      } as Response)

      await expect(fetchReminders('token')).rejects.toThrow(AuthError)
    })

    it('should return reminders on success', async () => {
      const mockResponse = {
        reminders: [
          { id: 1, remind_time: '2026-01-30T12:00:00Z', room_id: '!room:example.com', message: 'Test' },
        ],
      }
      vi.mocked(fetch).mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      } as Response)

      const result = await fetchReminders('token')
      expect(result).toEqual(mockResponse)
    })
  })

  describe('fetchRooms', () => {
    it('should throw AuthError on 401', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: false,
        status: 401,
        json: () => Promise.resolve({ error: 'Unauthorized' }),
      } as Response)

      await expect(fetchRooms('token')).rejects.toThrow(AuthError)
    })

    it('should return rooms on success', async () => {
      const mockResponse = {
        rooms: [{ room_id: '!room:example.com', name: 'Test Room' }],
      }
      vi.mocked(fetch).mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      } as Response)

      const result = await fetchRooms('token')
      expect(result).toEqual(mockResponse)
    })
  })


  describe('fetchRoomStats', () => {
    it('should ask for the period of the room, with its ID encoded', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ room_id: '!room:example.com' }),
      } as Response)

      await fetchRoomStats('token', '!room:example.com', 30)
      expect(fetch).toHaveBeenCalledWith('/api/rooms/!room%3Aexample.com/stats?days=30', {
        headers: { Authorization: 'Bearer token' },
      })
    })

    it('should say when the room is not shared with the user', async () => {
      vi.mocked(fetch).mockResolvedValue({ ok: false, status: 403 } as Response)

      await expect(fetchRoomStats('token', '!room:example.com', 30)).rejects.toThrow(
        'Access denied to this room'
      )
    })

    it('should throw AuthError on 401', async () => {
      vi.mocked(fetch).mockResolvedValue({ ok: false, status: 401 } as Response)

      await expect(fetchRoomStats('token', '!room:example.com', 30)).rejects.toThrow(AuthError)
    })
  })

  describe('fetchAdminRoomStats', () => {
    it('should ask the admin endpoint', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ room_id: '!room:example.com' }),
      } as Response)

      await fetchAdminRoomStats('token', '!room:example.com', 7)
      expect(fetch).toHaveBeenCalledWith('/api/admin/rooms/!room%3Aexample.com/stats?days=7', {
        headers: { Authorization: 'Bearer token' },
      })
    })
  })

  describe('fetchMessageActivity', () => {
    it('should ask for the period', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ rooms: [] }),
      } as Response)

      await fetchMessageActivity('token', 90)
      expect(fetch).toHaveBeenCalledWith('/api/admin/message-activity?days=90', {
        headers: { Authorization: 'Bearer token' },
      })
    })

    it('should say when admin access is required', async () => {
      vi.mocked(fetch).mockResolvedValue({ ok: false, status: 403 } as Response)

      await expect(fetchMessageActivity('token', 90)).rejects.toThrow('Admin access required')
    })
  })

  describe('fetchWebhooks', () => {
    it('should ask the admin endpoint', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ webhooks: [] }),
      } as Response)

      await fetchWebhooks('token')
      expect(fetch).toHaveBeenCalledWith('/api/admin/webhooks', {
        headers: { Authorization: 'Bearer token' },
      })
    })

    it('should say when admin access is required', async () => {
      vi.mocked(fetch).mockResolvedValue({ ok: false, status: 403 } as Response)

      await expect(fetchWebhooks('token')).rejects.toThrow('Admin access required')
    })

    it('should throw AuthError on 401', async () => {
      vi.mocked(fetch).mockResolvedValue({ ok: false, status: 401 } as Response)

      await expect(fetchWebhooks('token')).rejects.toThrow(AuthError)
    })
  })

  describe('createWebhook', () => {
    it('should send the webhook as JSON', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ webhook: { id: 'abc' } }),
      } as Response)

      const webhook = { name: 'hermes', room_id: '!ops:example.com', token: 'my-own-secret-token' }
      const result = await createWebhook('token', webhook)
      expect(result).toEqual({ webhook: { id: 'abc' } })
      expect(fetch).toHaveBeenCalledWith('/api/admin/webhooks', {
        method: 'POST',
        headers: { Authorization: 'Bearer token', 'Content-Type': 'application/json' },
        body: JSON.stringify(webhook),
      })
    })

    it("should pass the server's error on", async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: false,
        status: 409,
        json: () => Promise.resolve({ error: 'A webhook named hermes already exists' }),
      } as Response)

      await expect(
        createWebhook('token', { name: 'hermes', room_id: '!ops:example.com', token: 'x'.repeat(16) })
      ).rejects.toThrow('A webhook named hermes already exists')
    })
  })

  describe('updateWebhook', () => {
    it('should send only the changes, to the webhook with its ID encoded', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ webhook: { id: 'a/b' } }),
      } as Response)

      await updateWebhook('token', 'a/b', { name: 'iris' })
      expect(fetch).toHaveBeenCalledWith('/api/admin/webhooks/a%2Fb', {
        method: 'PATCH',
        headers: { Authorization: 'Bearer token', 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: 'iris' }),
      })
    })

    it("should pass the server's error on", async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: false,
        status: 404,
        json: () => Promise.resolve({ error: 'Webhook not found' }),
      } as Response)

      await expect(updateWebhook('token', 'abc', { name: 'iris' })).rejects.toThrow(
        'Webhook not found'
      )
    })
  })

  describe('deleteWebhook', () => {
    it('should delete the webhook', async () => {
      vi.mocked(fetch).mockResolvedValue({ ok: true, status: 204 } as Response)

      await deleteWebhook('token', 'abc')
      expect(fetch).toHaveBeenCalledWith('/api/admin/webhooks/abc', {
        method: 'DELETE',
        headers: { Authorization: 'Bearer token' },
      })
    })

    it('should throw AuthError on 401', async () => {
      vi.mocked(fetch).mockResolvedValue({ ok: false, status: 401 } as Response)

      await expect(deleteWebhook('token', 'abc')).rejects.toThrow(AuthError)
    })
  })
})
