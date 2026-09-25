package matrix

import "testing"

func TestStripReplyFallback(t *testing.T) {
	tests := []struct {
		name  string
		plain string
		want  string
	}{
		{
			name:  "no fallback",
			plain: "hello there",
			want:  "hello there",
		},
		{
			name:  "single quoted line",
			plain: "> <@someone:example.com> an earlier message\n\nhello there",
			want:  "hello there",
		},
		{
			name:  "multiple quoted lines",
			plain: "> <@someone:example.com> first line\n> second line\n\nhello there",
			want:  "hello there",
		},
		{
			name:  "quote without a reply",
			plain: "> <@someone:example.com> an earlier message\n\n",
			want:  "",
		},
		{
			name:  "quote later in the message is left alone",
			plain: "as they said:\n> an earlier message",
			want:  "as they said:\n> an earlier message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripReplyFallback(tt.plain); got != tt.want {
				t.Errorf("StripReplyFallback() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStripMxReply(t *testing.T) {
	tests := []struct {
		name      string
		formatted string
		want      string
	}{
		{
			name:      "no reply block",
			formatted: "<b>hello</b> there",
			want:      "<b>hello</b> there",
		},
		{
			name:      "reply block removed",
			formatted: "<mx-reply><blockquote>an earlier message</blockquote></mx-reply><b>hello</b> there",
			want:      "<b>hello</b> there",
		},
		{
			name:      "empty",
			formatted: "",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripMxReply(tt.formatted); got != tt.want {
				t.Errorf("StripMxReply() = %q, want %q", got, tt.want)
			}
		})
	}
}
