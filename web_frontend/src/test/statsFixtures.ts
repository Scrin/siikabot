import type {
  ActivityResponse,
  MessageActivityResponse,
  PersonStats,
  RoomActivity,
  RoomStatsResponse,
} from '../api/types'

/** A week up to Saturday 2026-10-10, with three days on which anything was counted */
export function activityFixture(overrides: Partial<ActivityResponse> = {}): ActivityResponse {
  return {
    days: 7,
    from: '2026-10-04',
    to: '2026-10-10',
    first_day: '2026-02-06',
    messages: 42,
    words: 158,
    characters: 790,
    busiest: { date: '2026-10-06', messages: 30, words: 100 },
    daily: [
      { date: '2026-10-04', messages: 10, words: 50 },
      { date: '2026-10-05', messages: 0, words: 0 },
      { date: '2026-10-06', messages: 30, words: 100 },
      { date: '2026-10-07', messages: 0, words: 0 },
      { date: '2026-10-08', messages: 0, words: 0 },
      { date: '2026-10-09', messages: 0, words: 0 },
      { date: '2026-10-10', messages: 2, words: 8 },
    ],
    ...overrides,
  }
}

export function personFixture(overrides: Partial<PersonStats> = {}): PersonStats {
  return {
    user_id: '@bob:example.com',
    display_name: 'Bob',
    in_room: true,
    rank: 1,
    messages: 1204,
    words: 9800,
    share: 0.34,
    last_message: '2026-10-09T18:00:00Z',
    ...overrides,
  }
}

/** Bob leads, Carol has left the room, and the viewer, Alice, is further down */
export function roomStatsFixture(overrides: Partial<RoomStatsResponse> = {}): RoomStatsResponse {
  return {
    room_id: '!room:example.com',
    activity: activityFixture(),
    people: {
      counted_since: '2026-01-31T12:00:00Z',
      people_count: 14,
      messages: 3541,
      words: 30000,
      characters: 150000,
      top: [
        personFixture(),
        personFixture({
          user_id: '@carol:example.com',
          display_name: undefined,
          in_room: false,
          rank: 2,
          messages: 611,
          words: 4000,
          share: 0.17,
        }),
      ],
      me: personFixture({
        user_id: '@alice:example.com',
        display_name: 'Alice',
        rank: 4,
        messages: 233,
        words: 1500,
        share: 0.066,
      }),
    },
    ...overrides,
  }
}

export function roomActivityFixture(overrides: Partial<RoomActivity> = {}): RoomActivity {
  return {
    room_id: '!busy:example.com',
    room_name: 'Busy room',
    messages: 30,
    words: 120,
    share: 0.75,
    chat_turns: 7,
    ...overrides,
  }
}

export function messageActivityFixture(
  overrides: Partial<MessageActivityResponse> = {}
): MessageActivityResponse {
  return {
    activity: activityFixture({ messages: 40 }),
    chat_turns: 8,
    rooms: [
      roomActivityFixture(),
      roomActivityFixture({
        room_id: '!quiet:example.com',
        room_name: undefined,
        messages: 10,
        words: 38,
        share: 0.25,
        chat_turns: 1,
      }),
    ],
    ...overrides,
  }
}
