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
  fetchGrafanaTemplates,
  createGrafanaTemplate,
  updateGrafanaTemplate,
  deleteGrafanaTemplate,
  setGrafanaDatasource,
  deleteGrafanaDatasource,
  renderGrafanaTemplate,
  AuthError,
} from './client'

describe('API Client', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    global.fetch = vi.fn()
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
          Promise.resolve({ user_id: '@user:example.com', authorizations: { grafana: true } }),
      } as Response)

      await fetchCurrentUser('my-token')
      expect(fetch).toHaveBeenCalledWith('/api/auth/me', {
        headers: { Authorization: 'Bearer my-token' },
      })
    })

    it('should return user data on success', async () => {
      const mockResponse = { user_id: '@user:example.com', authorizations: { grafana: true } }
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

  describe('fetchGrafanaTemplates', () => {
    it('should throw AuthError on 401', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: false,
        status: 401,
        json: () => Promise.resolve({ error: 'Unauthorized' }),
      } as Response)

      await expect(fetchGrafanaTemplates('token')).rejects.toThrow(AuthError)
    })

    it('should throw specific error for 403 Forbidden', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: false,
        status: 403,
        json: () => Promise.resolve({ error: 'Forbidden' }),
      } as Response)

      await expect(fetchGrafanaTemplates('token')).rejects.toThrow('Grafana access not authorized')
    })

    it('should return templates on success', async () => {
      const mockResponse = {
        templates: [{ name: 'test', template: '<html></html>', datasources: {} }],
      }
      vi.mocked(fetch).mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      } as Response)

      const result = await fetchGrafanaTemplates('token')
      expect(result).toEqual(mockResponse)
    })
  })

  describe('createGrafanaTemplate', () => {
    it('should send correct payload', async () => {
      vi.mocked(fetch).mockResolvedValue({ ok: true } as Response)

      await createGrafanaTemplate('token', 'my-template', '<html></html>')
      expect(fetch).toHaveBeenCalledWith('/api/grafana/templates', {
        method: 'POST',
        headers: {
          Authorization: 'Bearer token',
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ name: 'my-template', template: '<html></html>' }),
      })
    })

    it('should throw AuthError on 401', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: false,
        status: 401,
        json: () => Promise.resolve({ error: 'Unauthorized' }),
      } as Response)

      await expect(createGrafanaTemplate('token', 'name', 'template')).rejects.toThrow(AuthError)
    })

    it('should throw specific error for 403', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: false,
        status: 403,
        json: () => Promise.resolve({ error: 'Forbidden' }),
      } as Response)

      await expect(createGrafanaTemplate('token', 'name', 'template')).rejects.toThrow(
        'Grafana access not authorized'
      )
    })
  })

  describe('updateGrafanaTemplate', () => {
    it('should encode template name in URL', async () => {
      vi.mocked(fetch).mockResolvedValue({ ok: true } as Response)

      await updateGrafanaTemplate('token', 'my template', '<html></html>')
      expect(fetch).toHaveBeenCalledWith('/api/grafana/templates/my%20template', expect.any(Object))
    })

    it('should send correct payload', async () => {
      vi.mocked(fetch).mockResolvedValue({ ok: true } as Response)

      await updateGrafanaTemplate('token', 'template-name', '<html>updated</html>')
      expect(fetch).toHaveBeenCalledWith('/api/grafana/templates/template-name', {
        method: 'PUT',
        headers: {
          Authorization: 'Bearer token',
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ template: '<html>updated</html>' }),
      })
    })
  })

  describe('deleteGrafanaTemplate', () => {
    it('should use DELETE method', async () => {
      vi.mocked(fetch).mockResolvedValue({ ok: true } as Response)

      await deleteGrafanaTemplate('token', 'template-name')
      expect(fetch).toHaveBeenCalledWith('/api/grafana/templates/template-name', {
        method: 'DELETE',
        headers: { Authorization: 'Bearer token' },
      })
    })

    it('should encode template name in URL', async () => {
      vi.mocked(fetch).mockResolvedValue({ ok: true } as Response)

      await deleteGrafanaTemplate('token', 'my/template')
      expect(fetch).toHaveBeenCalledWith('/api/grafana/templates/my%2Ftemplate', expect.any(Object))
    })
  })

  describe('setGrafanaDatasource', () => {
    it('should encode both template and datasource names', async () => {
      vi.mocked(fetch).mockResolvedValue({ ok: true } as Response)

      await setGrafanaDatasource('token', 'my template', 'my source', 'http://example.com')
      expect(fetch).toHaveBeenCalledWith(
        '/api/grafana/templates/my%20template/datasources/my%20source',
        expect.any(Object)
      )
    })

    it('should send correct payload', async () => {
      vi.mocked(fetch).mockResolvedValue({ ok: true } as Response)

      await setGrafanaDatasource('token', 'template', 'source', 'http://example.com')
      expect(fetch).toHaveBeenCalledWith('/api/grafana/templates/template/datasources/source', {
        method: 'PUT',
        headers: {
          Authorization: 'Bearer token',
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ url: 'http://example.com' }),
      })
    })
  })

  describe('deleteGrafanaDatasource', () => {
    it('should use DELETE method with correct path', async () => {
      vi.mocked(fetch).mockResolvedValue({ ok: true } as Response)

      await deleteGrafanaDatasource('token', 'template', 'source')
      expect(fetch).toHaveBeenCalledWith('/api/grafana/templates/template/datasources/source', {
        method: 'DELETE',
        headers: { Authorization: 'Bearer token' },
      })
    })
  })

  describe('renderGrafanaTemplate', () => {
    it('should fetch render endpoint', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ html: '<table></table>' }),
      } as Response)

      await renderGrafanaTemplate('token', 'my-template')
      expect(fetch).toHaveBeenCalledWith('/api/grafana/templates/my-template/render', {
        headers: { Authorization: 'Bearer token' },
      })
    })

    it('should return rendered HTML on success', async () => {
      const mockResponse = { html: '<table><tr><td>Data</td></tr></table>' }
      vi.mocked(fetch).mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      } as Response)

      const result = await renderGrafanaTemplate('token', 'template')
      expect(result).toEqual(mockResponse)
    })

    it('should throw specific error for 404', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: false,
        status: 404,
        json: () => Promise.resolve({ error: 'Not found' }),
      } as Response)

      await expect(renderGrafanaTemplate('token', 'missing')).rejects.toThrow('Template not found')
    })

    it('should throw AuthError on 401', async () => {
      vi.mocked(fetch).mockResolvedValue({
        ok: false,
        status: 401,
        json: () => Promise.resolve({ error: 'Unauthorized' }),
      } as Response)

      await expect(renderGrafanaTemplate('token', 'template')).rejects.toThrow(AuthError)
    })
  })
})
