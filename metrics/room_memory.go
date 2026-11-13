package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var roomMemoryReflections = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "room_memory_reflections_count",
	Help: "Total number of times the bot thought back over a room's conversations, by what started the pass " +
		"(schedule or dashboard), its outcome (changed, unchanged, failed or skipped) and the reason for a " +
		"failure or skip",
}, []string{"trigger", "outcome", "reason"}))

var roomMemoryChanges = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "room_memory_changes_count",
	Help: "Total number of changes the bot made to its memories of rooms, by action",
}, []string{"action"}))

var roomMemoryRejectedChanges = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "room_memory_rejected_changes_count",
	Help: "Total number of changes to its memories of rooms that the bot decided on but that broke the rules, by reason",
}, []string{"reason"}))

var roomMemoryReflectionDuration = makeCollector(prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    metricPrefix + "room_memory_reflection_duration_seconds",
	Help:    "How long a pass over a room's conversations took, from reading them to saving what it decided",
	Buckets: []float64{0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600, 1200},
}))

var roomMemoryReflectionConversations = makeCollector(prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    metricPrefix + "room_memory_reflection_conversations",
	Help:    "How many conversations a pass over a room had to think back over",
	Buckets: []float64{1, 2, 5, 10, 20, 50, 100},
}))

var roomMemories = makeCollector(prometheus.NewGauge(prometheus.GaugeOpts{
	Name: metricPrefix + "room_memories",
	Help: "How many memories the bot keeps of all rooms together",
}))

var roomMemoriesDeleted = makeCollector(prometheus.NewCounter(prometheus.CounterOpts{
	Name: metricPrefix + "room_memories_deleted_count",
	Help: "Total number of the bot's memories of rooms deleted on the dashboard",
}))

// RecordRoomMemoryReflection records a pass over a room's conversations: what started it, "schedule" or
// "dashboard", its outcome, "changed", "unchanged", "failed" or "skipped", and the reason for a failure
// or skip, "none" otherwise
func RecordRoomMemoryReflection(trigger, outcome, reason string) {
	roomMemoryReflections.WithLabelValues(trigger, outcome, reason).Inc()
}

// RecordRoomMemoryChanges records changes a pass made to its memories of a room, by action: "add",
// "update" or "delete"
func RecordRoomMemoryChanges(action string, count int) {
	if count > 0 {
		roomMemoryChanges.WithLabelValues(action).Add(float64(count))
	}
}

// RecordRoomMemoryRejectedChange records a change a pass decided on that broke the rules, by reason
func RecordRoomMemoryRejectedChange(reason string) {
	roomMemoryRejectedChanges.WithLabelValues(reason).Inc()
}

// RecordRoomMemoryReflectionDuration records how long a pass took and how many conversations it had to
// think back over
func RecordRoomMemoryReflectionDuration(seconds float64, conversations int) {
	roomMemoryReflectionDuration.Observe(seconds)
	roomMemoryReflectionConversations.Observe(float64(conversations))
}

// SetRoomMemories sets how many memories the bot keeps of all rooms together
func SetRoomMemories(count int) {
	roomMemories.Set(float64(count))
}

// RecordRoomMemoriesDeleted records memories of rooms deleted on the dashboard
func RecordRoomMemoriesDeleted(count int) {
	if count > 0 {
		roomMemoriesDeleted.Add(float64(count))
	}
}
