package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var roomMemoryReflections = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "room_memory_reflections_count",
	Help: "Total number of times the bot thought back over a room's conversations, by outcome: changed or " +
		"unchanged memories, failed, or skipped without a model call",
}, []string{"outcome"}))

var roomMemoryChanges = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metricPrefix + "room_memory_changes_count",
	Help: "Total number of changes the bot made to its memories of rooms, by action, with the changes it " +
		"decided on but that broke the rules as rejected",
}, []string{"action"}))

var roomMemoriesDeleted = makeCollector(prometheus.NewCounter(prometheus.CounterOpts{
	Name: metricPrefix + "room_memories_deleted_count",
	Help: "Total number of the bot's memories of rooms deleted on the dashboard",
}))

// RecordRoomMemoryReflection records a pass over a room's conversations: "changed", "unchanged",
// "failed" or "skipped"
func RecordRoomMemoryReflection(outcome string) {
	roomMemoryReflections.WithLabelValues(outcome).Inc()
}

// RecordRoomMemoryChanges records changes a pass made to its memories of a room, by action: "add",
// "update", "delete", or "rejected" for the ones that broke the rules
func RecordRoomMemoryChanges(action string, count int) {
	if count > 0 {
		roomMemoryChanges.WithLabelValues(action).Add(float64(count))
	}
}

// RecordRoomMemoriesDeleted records memories of rooms deleted on the dashboard
func RecordRoomMemoriesDeleted(count int64) {
	if count > 0 {
		roomMemoriesDeleted.Add(float64(count))
	}
}
