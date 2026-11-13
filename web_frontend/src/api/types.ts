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

// Room memory types

/** Something the bot remembers about a group room, formed by thinking back over its conversations there */
export interface RoomMemory {
  id: number
  memory: string
  created_at: string
  /** When a later pass last revised the memory, the same as created_at if none has */
  updated_at: string
  /** The trace of the pass that formed the memory, null when it wasn't recorded */
  created_trace_id: string | null
  /** The trace of the pass that last revised the memory, null when it wasn't recorded */
  updated_trace_id: string | null
}

/** How a room stands with the schedule of passes */
export type RoomMemoryState = 'up_to_date' | 'waiting' | 'retrying' | 'due' | 'off'

/** How a room stands with the bot's passes over it */
export interface RoomMemoryStatus {
  /**
   * up_to_date, waiting for the room to be quiet for long enough, retrying after a failed pass, due at
   * the next check, or off: new conversations while the passes are off
   */
  state: RoomMemoryState
  /** When the room is due a pass, null when it has nothing new or the passes are off */
  due_at: string | null
  /** When the newest conversation the bot hasn't thought back over happened, null if there is none */
  latest_activity_at: string | null
  /** How many passes have failed in a row */
  failed_attempts: number
  /** When the latest pass ran, null when not recorded */
  last_attempt_at: string | null
  /** How the latest pass went, such as changed, unchanged or request_failed, null when not recorded */
  last_outcome: string | null
  /** The latest pass's trace, null when not recorded */
  last_trace_id: string | null
}

/** A room the bot remembers things about or has been thinking back over */
export interface RoomMemories {
  room_id: string
  room_name?: string
  /** When a pass over the room last completed, null if none has */
  last_reflected_at: string | null
  status: RoomMemoryStatus
  memories: RoomMemory[]
}

export interface RoomMemoriesResponse {
  /** The room with the most recent activity first */
  rooms: RoomMemories[]
}

/** How a pass the admin started went */
export interface ReflectResponse {
  /**
   * changed or unchanged for a pass that completed, and otherwise why it was skipped, such as
   * nothing_new, or why it failed, such as request_failed
   */
  outcome: string
  skipped: boolean
  failed: boolean
  conversation_count: number
  shown_conversation_count: number
  added: number
  updated: number
  deleted: number
  rejected: number
  /** The pass's trace, empty without tracing */
  trace_id: string
}

export interface DeleteRoomMemoriesResponse {
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
  /** How the failed turns failed: their count by outcome, such as turn_timeout or tool_limit */
  failures_by_outcome: Record<string, number>
  /**
   * The times the bot thought back over the room's conversations, to update its memories of it. Their
   * tokens are in the token counts, while failures and silent count turns only.
   */
  reflections: number
  /** The reflections that failed */
  failed_reflections: number
}

export interface ChatUsageResponse {
  since: string
  days: number
  entries: ChatUsageEntry[]
  totals: ChatUsageEntry
}

// Chat configuration types

/** One chat setting as it stands in the configuration in effect */
export interface ChatSetting {
  key: string
  /** The heading the setting is listed under, as in !chat config */
  group: string
  /** The value without its unit */
  value: string
  unit: string
  summary: string
  /** The command that changes the setting in a room, with an example value */
  command: string
}

/** A change of one setting from one configuration to the next, the values without their unit */
export interface ChatSettingChange {
  key: string
  from: string
  to: string
  unit: string
}

/** A configuration on record, with what it changed from the one before it */
export interface ChatConfigVersion {
  id: number
  created_at: string
  changes: ChatSettingChange[]
  /** The first configuration, which had nothing to change */
  first: boolean
}

export interface ChatConfigResponse {
  id: number
  created_at: string
  settings: ChatSetting[]
  /** The latest configurations, newest first, the one in effect included */
  history: ChatConfigVersion[]
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

// Webhook types

/** A webhook that posts the messages sent to its URL into a room (admin only) */
export interface WebhookResponse {
  id: string
  /** Shown in brackets before every message the webhook posts */
  name: string
  room_id: string
  room_name?: string
  /** Whether the bot is in the room. Until it is back, or the webhook is moved, nothing can be posted. */
  bot_in_room: boolean
  /** Where messages are sent, with the webhook's token as a bearer token */
  url: string
  created_at: string
  /** When a message was last delivered, null if none has been */
  last_used_at: string | null
}

export interface WebhooksResponse {
  webhooks: WebhookResponse[]
}

/** A new webhook. The token is only ever sent: the server keeps its hash, so it can't be read back. */
export interface CreateWebhookRequest {
  name: string
  room_id: string
  token: string
}

/** The changes to a webhook. Only the fields given change, and a new token replaces the old one. */
export interface UpdateWebhookRequest {
  name?: string
  room_id?: string
  token?: string
}

/** A webhook as it was created or changed */
export interface SavedWebhookResponse {
  webhook: WebhookResponse
}
