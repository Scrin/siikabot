package bot

import (
	"strings"
	"testing"
	"time"
)

func TestAlertMessage(t *testing.T) {
	startsAt := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		alert   Alert
		want    []string
		notWant []string
	}{
		{
			name: "summary heads, description under it, link to the source",
			alert: Alert{
				Status:       "firing",
				Labels:       map[string]string{"alertname": "DetectiveFinding", "severity": "warning"},
				Annotations:  map[string]string{"summary": "Disk filling on web1", "description": "Disk use rose to 91% in an hour."},
				StartsAt:     startsAt,
				GeneratorURL: "https://detective.example.com/findings/abc123",
			},
			want: []string{
				"<h4>⚠️ Disk filling on web1</h4>",
				"<p>Disk use rose to 91% in an hour.</p>",
				`<a href="https://detective.example.com/findings/abc123">Source</a>`,
				"alertname: <b>DetectiveFinding</b>",
			},
		},
		{
			name: "description alone heads, as rules that set only a description expect",
			alert: Alert{
				Status:      "firing",
				Annotations: map[string]string{"description": "Host web1 is down"},
				StartsAt:    startsAt,
			},
			want:    []string{"<h4>⚠️ Host web1 is down</h4>"},
			notWant: []string{"<p>", "Source"},
		},
		{
			name: "the same text in both shows once",
			alert: Alert{
				Status:      "firing",
				Annotations: map[string]string{"summary": "Host web1 is down", "description": "Host web1 is down"},
				StartsAt:    startsAt,
			},
			want:    []string{"<h4>⚠️ Host web1 is down</h4>"},
			notWant: []string{"<p>"},
		},
		{
			name: "no annotations: the alert's name heads",
			alert: Alert{
				Status:   "firing",
				Labels:   map[string]string{"alertname": "Watchdog"},
				StartsAt: startsAt,
			},
			want: []string{"<h4>⚠️ Watchdog</h4>"},
		},
		{
			name: "resolved, with its end",
			alert: Alert{
				Status:      "resolved",
				Annotations: map[string]string{"summary": "Disk filling on web1"},
				StartsAt:    startsAt,
				EndsAt:      startsAt.Add(time.Hour),
			},
			want: []string{"<h4>✅ Disk filling on web1</h4>", "ended at <b>Nov 18, 2026 11:00:00 UTC</b>"},
		},
		{
			name: "annotations and labels show as text, and only http links are links",
			alert: Alert{
				Status: "firing",
				Labels: map[string]string{"container": `<img src=x onerror="alert(1)">`},
				Annotations: map[string]string{
					"summary":     `<b>Bold</b> & "quoted"`,
					"description": `A log line said <a href="https://evil.example.com">click</a>`,
				},
				StartsAt:     startsAt,
				GeneratorURL: "javascript:alert(1)",
			},
			want: []string{
				"&lt;b&gt;Bold&lt;/b&gt; &amp; &#34;quoted&#34;",
				"&lt;a href=&#34;https://evil.example.com&#34;&gt;click&lt;/a&gt;",
				"&lt;img src=x onerror=&#34;alert(1)&#34;&gt;",
				`<a href="#ZgotmplZ">Source</a>`,
			},
			notWant: []string{"<b>Bold</b>", `href="https://evil.example.com"`, "<img", "javascript:"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := alertMessage(tc.alert)
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("message lacks %q:\n%s", w, got)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(got, w) {
					t.Errorf("message has %q:\n%s", w, got)
				}
			}
		})
	}
}
