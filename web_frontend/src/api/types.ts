// API response types

export interface HealthCheckResponse {
  status: string
  uptime: string
}

// Auth types
export interface ChallengeResponse {
  challenge: string
  poll_secret: string // Private - only for polling, never shown to user
  expires_at: string
}

export interface PollResponse {
  status: 'pending' | 'authenticated'
  token?: string
  user_id?: string
}

export interface MeResponse {
  user_id: string
}

export interface AuthErrorResponse {
  error: string
}
