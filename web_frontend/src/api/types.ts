// API response types

export interface HealthCheckResponse {
  status: string
  uptime: string
}

export interface MetricsResponse {
  memory: {
    resident_mb: number
  }
  runtime: {
    goroutines: number
  }
  database: {
    active_conns: number
    max_conns: number
    idle_conns: number
  }
  bot: {
    events_handled: number
  }
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
  authorizations: Authorizations
}

export interface Authorizations {
  grafana: boolean
}

export interface AuthErrorResponse {
  error: string
}

// Reminder types
export interface ReminderResponse {
  id: number
  remind_time: string
  room_id: string
  room_name?: string
  message: string
}

export interface RemindersResponse {
  reminders: ReminderResponse[]
}

// Room types
export interface RoomResponse {
  room_id: string
  room_name?: string
}

export interface RoomsResponse {
  rooms: RoomResponse[]
}

// Memory types
export interface MemoryResponse {
  id: number
  memory: string
  created_at: string
}

export interface MemoriesResponse {
  memories: MemoryResponse[]
}

export interface DeleteAllMemoriesResponse {
  deleted_count: number
}

// Grafana types
export interface GrafanaDatasource {
  name: string
  url: string
}

export interface GrafanaTemplate {
  name: string
  template: string
  datasources: GrafanaDatasource[]
}

export interface GrafanaTemplatesResponse {
  templates: GrafanaTemplate[]
}

export interface GrafanaRenderResponse {
  rendered: string
}
