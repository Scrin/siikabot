// API client functions

import type {
  HealthCheckResponse,
  MetricsResponse,
  ChallengeResponse,
  PollResponse,
  MeResponse,
  RemindersResponse,
  RoomsResponse,
  GrafanaTemplatesResponse,
  GrafanaRenderResponse,
} from './types'

const API_BASE = '/api'

/**
 * Custom error class for authentication failures (401)
 * Used to distinguish "token invalid" from other errors like server errors
 */
export class AuthError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'AuthError'
  }
}

/**
 * Fetch health check status
 */
export async function fetchHealthCheck(): Promise<HealthCheckResponse> {
  const response = await fetch(`${API_BASE}/healthcheck`)

  if (!response.ok) {
    throw new Error(`Health check failed: ${response.statusText}`)
  }

  return response.json()
}

/**
 * Fetch system metrics
 */
export async function fetchMetrics(): Promise<MetricsResponse> {
  const response = await fetch(`${API_BASE}/metrics`)

  if (!response.ok) {
    throw new Error(`Metrics fetch failed: ${response.statusText}`)
  }

  return response.json()
}

/**
 * Request a new authentication challenge
 */
export async function requestAuthChallenge(): Promise<ChallengeResponse> {
  const response = await fetch(`${API_BASE}/auth/challenge`, {
    method: 'POST',
  })

  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(error.error || 'Failed to request challenge')
  }

  return response.json()
}

/**
 * Poll for authentication completion
 * Requires the poll_secret which is only known to the originating browser session
 */
export async function pollAuthStatus(
  challenge: string,
  pollSecret: string
): Promise<PollResponse> {
  const response = await fetch(
    `${API_BASE}/auth/poll?challenge=${encodeURIComponent(challenge)}&poll_secret=${encodeURIComponent(pollSecret)}`
  )

  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(error.error || 'Failed to poll auth status')
  }

  return response.json()
}

/**
 * Get the current authenticated user
 */
export async function fetchCurrentUser(token: string): Promise<MeResponse> {
  const response = await fetch(`${API_BASE}/auth/me`, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })

  if (!response.ok) {
    if (response.status === 401) {
      throw new AuthError('Token invalid or expired')
    }
    const error = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(error.error || 'Failed to fetch user')
  }

  return response.json()
}

/**
 * Logout - clears the session token from the server
 */
export async function logout(token: string): Promise<void> {
  const response = await fetch(`${API_BASE}/auth/logout`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })

  if (!response.ok && response.status !== 401) {
    const error = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(error.error || 'Failed to logout')
  }
}

/**
 * Fetch the current user's active reminders
 */
export async function fetchReminders(token: string): Promise<RemindersResponse> {
  const response = await fetch(`${API_BASE}/reminders`, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })

  if (!response.ok) {
    if (response.status === 401) {
      throw new AuthError('Token invalid or expired')
    }
    const error = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(error.error || 'Failed to fetch reminders')
  }

  return response.json()
}

/**
 * Fetch the rooms shared between the bot and the current user
 */
export async function fetchRooms(token: string): Promise<RoomsResponse> {
  const response = await fetch(`${API_BASE}/rooms`, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })

  if (!response.ok) {
    if (response.status === 401) {
      throw new AuthError('Token invalid or expired')
    }
    const error = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(error.error || 'Failed to fetch rooms')
  }

  return response.json()
}

/**
 * Fetch all Grafana templates with their datasources
 */
export async function fetchGrafanaTemplates(token: string): Promise<GrafanaTemplatesResponse> {
  const response = await fetch(`${API_BASE}/grafana/templates`, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })

  if (!response.ok) {
    if (response.status === 401) {
      throw new AuthError('Token invalid or expired')
    }
    if (response.status === 403) {
      throw new Error('Grafana access not authorized')
    }
    const error = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(error.error || 'Failed to fetch templates')
  }

  return response.json()
}

/**
 * Create a new Grafana template
 */
export async function createGrafanaTemplate(
  token: string,
  name: string,
  template: string
): Promise<void> {
  const response = await fetch(`${API_BASE}/grafana/templates`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ name, template }),
  })

  if (!response.ok) {
    if (response.status === 401) {
      throw new AuthError('Token invalid or expired')
    }
    if (response.status === 403) {
      throw new Error('Grafana access not authorized')
    }
    const error = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(error.error || 'Failed to create template')
  }
}

/**
 * Update a Grafana template's content
 */
export async function updateGrafanaTemplate(
  token: string,
  name: string,
  template: string
): Promise<void> {
  const response = await fetch(`${API_BASE}/grafana/templates/${encodeURIComponent(name)}`, {
    method: 'PUT',
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ template }),
  })

  if (!response.ok) {
    if (response.status === 401) {
      throw new AuthError('Token invalid or expired')
    }
    if (response.status === 403) {
      throw new Error('Grafana access not authorized')
    }
    const error = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(error.error || 'Failed to update template')
  }
}

/**
 * Delete a Grafana template
 */
export async function deleteGrafanaTemplate(token: string, name: string): Promise<void> {
  const response = await fetch(`${API_BASE}/grafana/templates/${encodeURIComponent(name)}`, {
    method: 'DELETE',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })

  if (!response.ok) {
    if (response.status === 401) {
      throw new AuthError('Token invalid or expired')
    }
    if (response.status === 403) {
      throw new Error('Grafana access not authorized')
    }
    const error = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(error.error || 'Failed to delete template')
  }
}

/**
 * Set or update a datasource for a Grafana template
 */
export async function setGrafanaDatasource(
  token: string,
  templateName: string,
  datasourceName: string,
  url: string
): Promise<void> {
  const response = await fetch(
    `${API_BASE}/grafana/templates/${encodeURIComponent(templateName)}/datasources/${encodeURIComponent(datasourceName)}`,
    {
      method: 'PUT',
      headers: {
        Authorization: `Bearer ${token}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ url }),
    }
  )

  if (!response.ok) {
    if (response.status === 401) {
      throw new AuthError('Token invalid or expired')
    }
    if (response.status === 403) {
      throw new Error('Grafana access not authorized')
    }
    const error = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(error.error || 'Failed to set datasource')
  }
}

/**
 * Delete a datasource from a Grafana template
 */
export async function deleteGrafanaDatasource(
  token: string,
  templateName: string,
  datasourceName: string
): Promise<void> {
  const response = await fetch(
    `${API_BASE}/grafana/templates/${encodeURIComponent(templateName)}/datasources/${encodeURIComponent(datasourceName)}`,
    {
      method: 'DELETE',
      headers: {
        Authorization: `Bearer ${token}`,
      },
    }
  )

  if (!response.ok) {
    if (response.status === 401) {
      throw new AuthError('Token invalid or expired')
    }
    if (response.status === 403) {
      throw new Error('Grafana access not authorized')
    }
    const error = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(error.error || 'Failed to delete datasource')
  }
}

/**
 * Render a Grafana template with real data from datasources
 */
export async function renderGrafanaTemplate(
  token: string,
  name: string
): Promise<GrafanaRenderResponse> {
  const response = await fetch(
    `${API_BASE}/grafana/templates/${encodeURIComponent(name)}/render`,
    {
      headers: {
        Authorization: `Bearer ${token}`,
      },
    }
  )

  if (!response.ok) {
    if (response.status === 401) {
      throw new AuthError('Token invalid or expired')
    }
    if (response.status === 403) {
      throw new Error('Grafana access not authorized')
    }
    if (response.status === 404) {
      throw new Error('Template not found')
    }
    const error = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(error.error || 'Failed to render template')
  }

  return response.json()
}
