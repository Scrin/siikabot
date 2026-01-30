// API client functions

import type {
  HealthCheckResponse,
  ChallengeResponse,
  PollResponse,
  MeResponse,
  RemindersResponse,
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
