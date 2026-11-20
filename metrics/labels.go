package metrics

// Labels whose values come from outside, such as the type of an event someone sent or the method of an HTTP
// request, are kept to the values the bot knows. Anything else is counted as "other": otherwise anyone could
// make a new series for every value they send, and grow /metrics without end.

// otherLabel is the label value for anything not in a known set
const otherLabel = "other"

// knownEventTypes are the event types counted by name
var knownEventTypes = setOf(
	// Room events
	"m.room.message", "m.room.encrypted", "m.room.redaction", "m.reaction", "m.sticker",
	"m.call.invite", "m.call.answer", "m.call.hangup", "m.call.candidates",
	"m.poll.start", "m.poll.response", "m.poll.end",
	"org.matrix.msc3381.poll.start", "org.matrix.msc3381.poll.response", "org.matrix.msc3381.poll.end",
	// State events
	"m.room.member", "m.room.create", "m.room.name", "m.room.topic", "m.room.avatar", "m.room.power_levels",
	"m.room.join_rules", "m.room.history_visibility", "m.room.guest_access", "m.room.canonical_alias",
	"m.room.encryption", "m.room.pinned_events", "m.room.server_acl", "m.room.tombstone",
	"m.room.third_party_invite", "m.space.child", "m.space.parent",
	// Ephemeral events and account data
	"m.typing", "m.receipt", "m.presence", "m.direct", "m.push_rules", "m.fully_read", "m.tag",
	"m.ignored_user_list",
	// To-device events
	"m.room_key", "m.room_key_request", "m.forwarded_room_key", "m.room_key.withheld", "m.dummy",
)

// knownMessageTypes are the msgtypes counted by name
var knownMessageTypes = setOf("", "m.text", "m.notice", "m.emote", "m.image", "m.file", "m.audio", "m.video",
	"m.location", "m.key.verification.request", "m.server_notice")

// knownHTTPMethods are the HTTP methods counted by name
var knownHTTPMethods = setOf("GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "CONNECT", "TRACE")

// known returns value if it's in the set, and otherLabel if it isn't
func known(set map[string]bool, value string) string {
	if set[value] {
		return value
	}
	return otherLabel
}

func setOf(values ...string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}
