// API client functions

import type { HealthCheckResponse } from './types'

const API_BASE = '/api'

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
