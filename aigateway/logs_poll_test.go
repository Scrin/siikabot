package aigateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/testutil/knownbug"
	"github.com/prometheus/client_golang/prometheus"
)

// The poller against a stand-in for the logs API, which answers a page at a time, newest first, as Cloudflare does

// fakeLogsAPI serves entries, newest first, to the poller until the test ends. It returns the pages asked for.
// failing makes every answer an API error while it's set.
type fakeLogsAPI struct {
	mu      sync.Mutex
	entries []GatewayLog
	pages   []int
	failing bool
}

func useFakeLogsAPI(t *testing.T, entries ...GatewayLog) *fakeLogsAPI {
	t.Helper()
	config.CloudflareAccountID, config.CloudflareAIGatewayID, config.CloudflareAPIToken = "acct123", "siikabot", "cf-token"
	api := &fakeLogsAPI{entries: entries}
	original := httpClient
	httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.cloudflare.com" || !strings.HasSuffix(r.URL.Path, "/ai-gateway/gateways/siikabot/logs") ||
			r.Header.Get("Authorization") != "Bearer cf-token" {
			t.Errorf("the poller asked for %s %s", r.Method, r.URL)
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
		api.mu.Lock()
		defer api.mu.Unlock()
		api.pages = append(api.pages, page)
		var body []byte
		if api.failing {
			body = []byte(`{"success":false,"errors":[{"code":10000,"message":"Authentication error"}],"result":[]}`)
		} else {
			from, to := min((page-1)*perPage, len(api.entries)), min(page*perPage, len(api.entries))
			body, _ = json.Marshal(map[string]any{"success": true, "errors": []any{}, "result": api.entries[from:to]})
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}}, nil
	})}
	t.Cleanup(func() { httpClient = original })
	return api
}

// add puts newer entries in front, as new requests are logged
func (api *fakeLogsAPI) add(entries ...GatewayLog) {
	api.mu.Lock()
	defer api.mu.Unlock()
	api.entries = append(append([]GatewayLog{}, entries...), api.entries...)
}

// tokensRecorded is how many tokens of a type the metrics hold for a model
func tokensRecorded(t *testing.T, model, kind string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	total := 0.0
	for _, family := range families {
		if family.GetName() != "siikabot_ai_gateway_tokens_count" {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := map[string]string{}
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			if labels["model"] == model && labels["type"] == kind {
				total += metric.GetCounter().GetValue()
			}
		}
	}
	return total
}

// Each entry is recorded once, oldest first, however many share a timestamp, and the poll stops at the cursor
func TestPollLogsRecordsEachEntryOnce(t *testing.T) {
	model := "poll-test-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	cursor := mustTime(t, "2026-08-05T14:44:38.000Z")
	shared := mustTime(t, "2026-08-05T14:44:40.346Z")
	entry := func(id string, at time.Time, tokensIn int) GatewayLog {
		return GatewayLog{ID: id, CreatedAt: at, Model: model, Provider: "openai", TokensIn: tokensIn, TokensOut: 1, Success: true}
	}
	api := useFakeLogsAPI(t,
		entry("newest_1", shared, 100),
		entry("newest_2", shared, 200),
		entry("between", mustTime(t, "2026-08-05T14:44:39.000Z"), 400),
		entry("before", mustTime(t, "2026-08-05T14:44:37.000Z"), 800), // older than the cursor
	)

	cursor, seen := pollLogs(t.Context(), cursor, map[string]bool{})
	if got := tokensRecorded(t, model, "prompt"); got != 700 {
		t.Errorf("the first poll recorded %v prompt tokens, want 700, from the three entries after the cursor", got)
	}
	if !cursor.Equal(shared) || len(seen) != 2 || !seen["newest_1"] || !seen["newest_2"] {
		t.Errorf("the cursor moved to %v with %v, want the newest timestamp with both of its entries", cursor, seen)
	}

	// A third entry at the same timestamp is new; the two already recorded aren't
	api.add(entry("newest_3", shared, 1600))
	cursor, seen = pollLogs(t.Context(), cursor, seen)
	if got := tokensRecorded(t, model, "prompt"); got != 2300 {
		t.Errorf("after the second poll %v prompt tokens are recorded, want 2300", got)
	}

	// Nothing new leaves everything as it was
	t.Run("entries_at_the_cursor_stay_recorded", func(t *testing.T) {
		knownbug.Skip(t, "B28", "a poll that finds a new entry at the cursor's timestamp forgets the ones recorded there before")
		if len(seen) != 3 {
			t.Errorf("the entries at the cursor are %v, want all three", seen)
		}
		if again, _ := pollLogs(t.Context(), cursor, seen); !again.Equal(cursor) || tokensRecorded(t, model, "prompt") != 2300 {
			t.Errorf("a poll with nothing new recorded %v prompt tokens in all, want 2300", tokensRecorded(t, model, "prompt"))
		}
	})
}

// A burst of more entries than a page holds is read a page at a time, up to the pages one poll may read
func TestPollLogsPagesThroughABurst(t *testing.T) {
	model := "burst-test-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	start := mustTime(t, "2026-08-05T14:00:00.000Z")
	var entries []GatewayLog
	for i := 120; i > 0; i-- {
		entries = append(entries, GatewayLog{ID: fmt.Sprint("log", i), CreatedAt: start.Add(time.Duration(i) * time.Second),
			Model: model, Provider: "openai", TokensIn: 1, Success: true})
	}
	api := useFakeLogsAPI(t, entries...)

	pollLogs(t.Context(), start, map[string]bool{})
	if got := tokensRecorded(t, model, "prompt"); got != 120 {
		t.Errorf("%v entries were recorded, want all 120", got)
	}
	if fmt.Sprint(api.pages) != "[1 2 3]" {
		t.Errorf("the poller read pages %v, want 1 to 3", api.pages)
	}
}

// When the API fails, the cursor stays where it was, so the next poll reads the same entries rather than losing them
func TestPollLogsKeepsTheCursorWhenTheAPIFails(t *testing.T) {
	model := "failing-test-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	cursor := mustTime(t, "2026-08-05T14:44:38.000Z")
	api := useFakeLogsAPI(t, GatewayLog{ID: "a", CreatedAt: cursor.Add(time.Second), Model: model, Provider: "openai",
		TokensIn: 5, Success: true})
	api.failing = true

	after, seen := pollLogs(t.Context(), cursor, map[string]bool{"x": true})
	if !after.Equal(cursor) || !seen["x"] || tokensRecorded(t, model, "prompt") != 0 {
		t.Errorf("a failed poll moved the cursor to %v with %v", after, seen)
	}

	api.failing = false
	pollLogs(t.Context(), after, seen)
	if got := tokensRecorded(t, model, "prompt"); got != 5 {
		t.Errorf("the poll after the failure recorded %v prompt tokens, want the entry it missed", got)
	}
}
