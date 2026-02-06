package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var remindersCreated = makeCollector(prometheus.NewCounter(prometheus.CounterOpts{
	Name: metricPrefix + "reminders_created_count",
	Help: "Total number of reminders created",
}))

var remindersTriggered = makeCollector(prometheus.NewCounter(prometheus.CounterOpts{
	Name: metricPrefix + "reminders_triggered_count",
	Help: "Total number of reminders triggered and delivered",
}))

var remindersActive = makeCollector(prometheus.NewGauge(prometheus.GaugeOpts{
	Name: metricPrefix + "reminders_active",
	Help: "Current number of active (pending) reminders",
}))

// RecordReminderCreated records a reminder being created
func RecordReminderCreated() {
	remindersCreated.Inc()
	remindersActive.Inc()
}

// RecordReminderTriggered records a reminder being triggered and delivered
func RecordReminderTriggered() {
	remindersTriggered.Inc()
	remindersActive.Dec()
}

// SetRemindersActive sets the current number of active reminders
func SetRemindersActive(count int) {
	remindersActive.Set(float64(count))
}
