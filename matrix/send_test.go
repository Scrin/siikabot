package matrix

import (
	"strings"
	"testing"
)

func TestStripFormatting(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "plain text",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "paragraph tags",
			input:    "<p>hello</p>",
			expected: "\nhello\n",
		},
		{
			name:     "h1 header",
			input:    "<h1>Title</h1>",
			expected: "\nTitle\n",
		},
		{
			name:     "h2 header",
			input:    "<h2>Subtitle</h2>",
			expected: "\nSubtitle\n",
		},
		{
			name:     "h3 header",
			input:    "<h3>Section</h3>",
			expected: "\nSection\n",
		},
		{
			name:     "h4 header",
			input:    "<h4>Subsection</h4>",
			expected: "\nSubsection\n",
		},
		{
			name:     "h5 header",
			input:    "<h5>Minor</h5>",
			expected: "\nMinor\n",
		},
		{
			name:     "h6 header",
			input:    "<h6>Smallest</h6>",
			expected: "\nSmallest\n",
		},
		{
			name:     "list items",
			input:    "<ul><li>item1</li><li>item2</li></ul>",
			expected: "\n - item1\n - item2\n",
		},
		{
			name:     "table structure",
			input:    "<table><tr><td>cell1</td><td>cell2</td></tr></table>",
			expected: "cell1 cell2 \n",
		},
		{
			name:     "br tag",
			input:    "line1<br>line2",
			expected: "line1\nline2",
		},
		{
			name:     "br self-closing",
			input:    "line1<br/>line2",
			expected: "line1\nline2",
		},
		{
			name:     "br with space",
			input:    "line1<br />line2",
			expected: "line1\nline2",
		},
		{
			name:     "HTML entities ampersand",
			input:    "Tom &amp; Jerry",
			expected: "Tom & Jerry",
		},
		{
			name:     "HTML entities less than",
			input:    "a &lt; b",
			expected: "a < b",
		},
		{
			name:     "HTML entities greater than",
			input:    "a &gt; b",
			expected: "a > b",
		},
		{
			name:     "HTML entities quote",
			input:    "&quot;quoted&quot;",
			expected: "\"quoted\"",
		},
		{
			name:     "bold tag stripped",
			input:    "<b>bold</b>",
			expected: "bold",
		},
		{
			name:     "italic tag stripped",
			input:    "<i>italic</i>",
			expected: "italic",
		},
		{
			name:     "strong tag stripped",
			input:    "<strong>strong</strong>",
			expected: "strong",
		},
		{
			name:     "link tag stripped",
			input:    "<a href=\"https://example.com\">link text</a>",
			expected: "link text",
		},
		{
			name:     "code tag stripped",
			input:    "<code>code</code>",
			expected: "code",
		},
		{
			name:     "nested tags",
			input:    "<p><strong>bold in paragraph</strong></p>",
			expected: "\nbold in paragraph\n",
		},
		{
			name:     "complex HTML",
			input:    "<h1>Title</h1><p>Paragraph with <strong>bold</strong> and <a href=\"#\">link</a>.</p><ul><li>Item 1</li><li>Item 2</li></ul>",
			expected: "\nTitle\n\nParagraph with bold and link.\n\n - Item 1\n - Item 2\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripFormatting(tt.input)
			if result != tt.expected {
				t.Errorf("stripFormatting(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMarkdownToHTML(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantContains []string
	}{
		{
			name:         "plain text",
			input:        "hello world",
			wantContains: []string{"hello world"},
		},
		{
			name:         "h1 header",
			input:        "# Title",
			wantContains: []string{"<h1>", "Title", "</h1>"},
		},
		{
			name:         "h2 header",
			input:        "## Subtitle",
			wantContains: []string{"<h2>", "Subtitle", "</h2>"},
		},
		{
			name:         "bold text",
			input:        "**bold**",
			wantContains: []string{"<strong>", "bold", "</strong>"},
		},
		{
			name:         "italic text",
			input:        "*italic*",
			wantContains: []string{"<em>", "italic", "</em>"},
		},
		{
			name:         "inline code",
			input:        "`code`",
			wantContains: []string{"<code>", "code", "</code>"},
		},
		{
			name:         "link",
			input:        "[text](https://example.com)",
			wantContains: []string{"<a", "href", "https://example.com", "text", "</a>"},
		},
		{
			name:         "unordered list",
			input:        "- item1\n- item2",
			wantContains: []string{"<ul>", "<li>", "item1", "item2", "</li>", "</ul>"},
		},
		{
			name:         "ordered list",
			input:        "1. first\n2. second",
			wantContains: []string{"<ol>", "<li>", "first", "second", "</li>", "</ol>"},
		},
		{
			name:         "code block",
			input:        "```\ncode block\n```",
			wantContains: []string{"<pre>", "<code>", "code block", "</code>", "</pre>"},
		},
		{
			name:         "blockquote",
			input:        "> quoted text",
			wantContains: []string{"<blockquote>", "quoted text", "</blockquote>"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := markdownToHTML(tt.input)
			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("markdownToHTML(%q) = %q, want to contain %q", tt.input, result, want)
				}
			}
		})
	}
}

func TestMarkdownToHTMLEmpty(t *testing.T) {
	result := markdownToHTML("")
	// Empty input might produce empty output or minimal whitespace
	if len(strings.TrimSpace(result)) > 0 {
		// If there's content, it should be minimal
		if len(result) > 10 {
			t.Errorf("markdownToHTML(\"\") produced unexpected output: %q", result)
		}
	}
}
