package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// seriesCount is how many series a collector has
func seriesCount(c prometheus.Collector) int {
	ch := make(chan prometheus.Metric, 10000)
	c.Collect(ch)
	close(ch)
	return len(ch)
}

// counterValue is the value of a counter
func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatal(err)
	}
	return m.GetCounter().GetValue()
}

// Anyone can send an HTTP request with any method, or an event of any type. Each value mustn't make a series
// of its own.
func TestLabelsFromOutsideAreBounded(t *testing.T) {
	before := seriesCount(httpRequestsTotal)
	for _, method := range []string{"X0", "X1", "X2", "PROPFIND"} {
		RecordHTTPRequest(method, "unmatched", 404, 0.01)
	}
	if added := seriesCount(httpRequestsTotal) - before; added != 1 {
		t.Errorf("four unknown methods added %d series, want 1", added)
	}
	if got := counterValue(t, httpRequestsTotal.WithLabelValues(otherLabel, "unmatched", "404")); got != 4 {
		t.Errorf("the other series counted %v, want 4", got)
	}

	before = seriesCount(eventsHandled)
	RecordEventHandled("com.example.custom1", "m.text", false)
	RecordEventHandled("com.example.custom2", "com.example.msgtype", false)
	RecordEventHandled("m.room.message", "x.y", false)
	if added := seriesCount(eventsHandled) - before; added != 3 {
		t.Errorf("unknown event types and msgtypes added %d series, want 3", added)
	}
	RecordEventHandled("com.example.custom3", "com.example.msgtype2", false)
	if added := seriesCount(eventsHandled) - before; added != 3 {
		t.Errorf("more unknown values added series: %d, want still 3", added)
	}
}
