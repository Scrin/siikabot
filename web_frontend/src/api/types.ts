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

// Message stats types

/** What was counted on one day */
export interface DayCounts {
  /** A calendar date in the bot's time zone, YYYY-MM-DD. Not a moment: read its weekday with weekdayOf. */
  date: string
  messages: number
  words: number
}

/** The text messages counted per day over a period up to today */
export interface ActivityResponse {
  days: number
  /**
   * The first day shown, YYYY-MM-DD. Later than the start of the period when counting began later,
   * since the days before it weren't quiet, only not counted.
   */
  from: string
  to: string
  /** The first day anything was counted on, null if nothing has been */
  first_day: string | null
  messages: number
  words: number
  characters: number
  /** The day with the most messages, null if nothing was counted in the period */
  busiest: DayCounts | null
  /** Every day shown, including the ones on which nothing was counted */
  daily: DayCounts[]
}

/** Someone's message counts in a room, since counting began */
export interface PersonStats {
  user_id: string
  /** Their display name in the room, left out if they have none or aren't in it */
  display_name?: string
  /** Whether they are a member of the room now */
  in_room: boolean
  /** Their place by messages. People with the same count share a place. */
  rank: number
  messages: number
  words: number
  /** Their share of the room's messages, from 0 to 1 */
  share: number
  last_message: string
}

/** The counts of everyone counted in a room, since counting began. They have no breakdown by date. */
export interface PeopleResponse {
  /** When the earliest counted message was sent, null if none has been */
  counted_since: string | null
  people_count: number
  messages: number
  words: number
  characters: number
  /** The people with the most messages, most first */
  top: PersonStats[]
  /** The viewer's own counts, null if nothing of theirs has been counted in the room */
  me: PersonStats | null
}

export interface RoomStatsResponse {
  room_id: string
  activity: ActivityResponse
  people: PeopleResponse
}

/** One room's counts over a period */
export interface RoomActivity {
  room_id: string
  room_name?: string
  messages: number
  words: number
  /** The room's share of the messages counted in every room, from 0 to 1 */
  share: number
  /** How many messages the bot handled as chat in the room */
  chat_turns: number
}

/** The activity across every room over a period (admin only) */
export interface MessageActivityResponse {
  activity: ActivityResponse
  chat_turns: number
  /** The rooms with anything counted or any chat turns in the period, most messages first */
  rooms: RoomActivity[]
}
