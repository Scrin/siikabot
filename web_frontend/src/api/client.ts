// API client functions

import type {
  HealthCheckResponse,
  MetricsResponse,
  ChallengeResponse,
  PollResponse,
  MeResponse,
  RemindersResponse,
  RoomsResponse,
  RoomMembersResponse,
  MemoriesResponse,
  DeleteAllMemoriesResponse,
  ChatUsageResponse,
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
 * Fetch all rooms known to the bot (admin only)
 */
export async function fetchAdminRooms(token: string): Promise<RoomsResponse> {
  const response = await fetch(`${API_BASE}/admin/rooms`, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })

  if (!response.ok) {
    if (response.status === 401) {
      throw new AuthError('Token invalid or expired')
    }
    if (response.status === 403) {
      throw new Error('Admin access required')
    }
    const error = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(error.error || 'Failed to fetch admin rooms')
  }

  return response.json()
}

/**
 * Fetch members of a specific room
 */
export async function fetchRoomMembers(
  token: string,
  roomId: string
): Promise<RoomMembersResponse> {
  const response = await fetch(`${API_BASE}/rooms/${encodeURIComponent(roomId)}/members`, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })

  if (!response.ok) {
    if (response.status === 401) {
      throw new AuthError('Token invalid or expired')
    }
    if (response.status === 403) {
      throw new Error('Access denied to this room')
    }
    const error = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(error.error || 'Failed to fetch room members')
  }

  return response.json()
}

/**
 * Fetch members of any room (admin only)
 */
export async function fetchAdminRoomMembers(
  token: string,
  roomId: string
): Promise<RoomMembersResponse> {
  const response = await fetch(
    `${API_BASE}/admin/rooms/${encodeURIComponent(roomId)}/members`,
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
      throw new Error('Admin access required')
    }
    const error = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(error.error || 'Failed to fetch admin room members')
  }

  return response.json()
}

/**
 * Fetch the current user's memories
 */
export async function fetchMemories(token: string): Promise<MemoriesResponse> {
  const response = await fetch(`${API_BASE}/memories`, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })

  if (!response.ok) {
    if (response.status === 401) {
      throw new AuthError('Token invalid or expired')
    }
    const error = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(error.error || 'Failed to fetch memories')
  }

  return response.json()
}

/**
 * Delete a specific memory
 */
export async function deleteMemory(token: string, memoryId: number): Promise<void> {
  const response = await fetch(`${API_BASE}/memories/${memoryId}`, {
    method: 'DELETE',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })

  if (!response.ok) {
    if (response.status === 401) {
      throw new AuthError('Token invalid or expired')
    }
    const error = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(error.error || 'Failed to delete memory')
  }
}

/**
 * Delete all memories for the current user
 */
export async function deleteAllMemories(token: string): Promise<DeleteAllMemoriesResponse> {
  const response = await fetch(`${API_BASE}/memories`, {
    method: 'DELETE',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })

  if (!response.ok) {
    if (response.status === 401) {
      throw new AuthError('Token invalid or expired')
    }
    const error = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(error.error || 'Failed to delete memories')
  }

  return response.json()
}

/**
 * Fetch per-room chat usage (admin only)
 *
 * Room-keyed usage is deliberately absent from the Prometheus metrics, which are served without
 * authentication, so this endpoint is the only place it is available.
 */
export async function fetchChatUsage(token: string, days: number): Promise<ChatUsageResponse> {
  const response = await fetch(`${API_BASE}/admin/chat-usage?days=${days}`, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })

  if (!response.ok) {
    if (response.status === 401) {
      throw new AuthError('Token invalid or expired')
    }
    if (response.status === 403) {
      throw new Error('Admin access required')
    }
    const error = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(error.error || 'Failed to fetch chat usage')
  }

  return response.json()
}
