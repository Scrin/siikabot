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
  admin: boolean
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

// Room member types
/** A member who has joined the room. Invited users aren't listed. */
export interface RoomMemberResponse {
  user_id: string
  /** Their display name in the room, absent if they have none */
  display_name?: string
}

export interface RoomMembersResponse {
  members: RoomMemberResponse[]
}

// Memory types
export interface MemoryResponse {
  id: number
  memory: string
  /**
   * The group room the memory was saved in, which is where it is used along with direct chats.
   * Null for a memory saved in a direct chat, which is used in direct chats only.
   */
  room_id: string | null
  room_name?: string
  created_at: string
}

export interface MemoriesResponse {
  memories: MemoryResponse[]
}

export interface DeleteAllMemoriesResponse {
  deleted_count: number
}

// Chat usage types
export interface ChatUsageEntry {
  room_id: string
  room_name: string
  model: string
  turns: number
  prompt_tokens: number
  completion_tokens: number
  cached_prompt_tokens: number
  cache_hit_rate: number
  tool_iterations: number
  failures: number
  /** Turns that ended without an answer because the message was a reply that needed none */
  silent: number
}

export interface ChatUsageResponse {
  since: string
  days: number
  entries: ChatUsageEntry[]
  totals: ChatUsageEntry
}
