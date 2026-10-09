package webfetch

import (
	"context"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	readability "codeberg.org/readeck/go-readability/v2"
	"codeberg.org/readeck/go-readability/v2/render"
	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"github.com/Scrin/siikabot/tracing"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

const (
	// minArticleText is how much text readability has to find for its pick to be the page's main content.
	// With less, it has probably picked a fragment, and the whole page is converted instead.
	minArticleText = 250
	// maxArticleElements is the most elements readability scores. It compares every candidate with the
	// others, so a page built to be huge could keep it busy for a long time. Real articles have a few
	// thousand elements.
	maxArticleElements = 30000
)

// response is a page as it was read
type response struct {
	url         *url.URL
	statusCode  int
	contentType string
	body        []byte
	// truncated says the body was cut at maxBodyBytes
	truncated bool
}

// mediaType is the type of the response's content: what the response says it is, or what it looks like when
// it doesn't say
func (r response) mediaType() string {
	mediaType, _, err := mime.ParseMediaType(r.contentType)
	if err != nil || mediaType == "" || mediaType == "application/octet-stream" {
		mediaType, _, _ = mime.ParseMediaType(http.DetectContentType(r.body))
	}
	return mediaType
}

// text decodes the body to UTF-8, from the character set the response declares or, for HTML, the page's
// <meta charset>
func (r response) text() string {
	encoding, name, certain := charset.DetermineEncoding(r.body, r.contentType)
	// Without a declared character set, a page whose first kilobyte is plain ASCII is taken for
	// windows-1252, which garbles any UTF-8 further down. Text with bytes above ASCII that is still valid
	// UTF-8 is almost certainly UTF-8.
	if name == "utf-8" || (!certain && utf8.Valid(r.body)) {
		return strings.TrimPrefix(strings.ToValidUTF8(string(r.body), "\uFFFD"), "\uFEFF")
	}
	decoded, err := encoding.NewDecoder().Bytes(r.body)
	if err != nil {
		return strings.ToValidUTF8(string(r.body), "\uFFFD")
	}
	return strings.TrimPrefix(string(decoded), "\uFEFF")
}

func isHTML(mediaType string) bool {
	return mediaType == "text/html" || mediaType == "application/xhtml+xml"
}

// isText reports whether a type is text the model can read as it is: plain text, CSV, JSON, XML, feeds
func isText(mediaType string) bool {
	return strings.HasPrefix(mediaType, "text/") ||
		mediaType == "application/json" || strings.HasSuffix(mediaType, "+json") ||
		mediaType == "application/xml" || strings.HasSuffix(mediaType, "+xml")
}

// pageFacts are what the bot reads from a page's markup itself, besides its content
type pageFacts struct {
	title    string
	language string
	// paywalled says the page's structured data marks it as not free to read
	paywalled bool
	// asksForJavaScript says a <noscript> element tells the reader to turn JavaScript on
	asksForJavaScript bool
	// scripts counts the page's script elements
	scripts int
}

func factsOf(doc *html.Node) pageFacts {
	var facts pageFacts
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "html":
				facts.language = attr(n, "lang")
			case "title":
				if facts.title == "" {
					facts.title = strings.Join(strings.Fields(textOf(n)), " ")
				}
			case "noscript":
				// The parser runs with scripting on, so a <noscript> element holds its markup as text
				if strings.Contains(strings.ToLower(textOf(n)), "javascript") {
					facts.asksForJavaScript = true
				}
			case "script":
				facts.scripts++
				if strings.EqualFold(attr(n, "type"), "application/ld+json") && paywalled(textOf(n)) {
					facts.paywalled = true
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return facts
}

// attr returns the value of an element's attribute, or "" when it has none
func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

// textOf returns the text directly inside an element
func textOf(n *html.Node) string {
	var text strings.Builder
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode {
			text.WriteString(child.Data)
		}
	}
	return text.String()
}

// textLength counts the characters of text a reader would see in a node
func textLength(n *html.Node) int {
	return utf8.RuneCountInString(strings.TrimSpace(render.InnerText(n)))
}

// mainContent converts the page's main content to markdown, filling in the page's details as readability
// finds them. If readability finds little or nothing, the whole page, which holds pageText characters of text,
// is converted instead. Returns the markdown, how much text it holds, and whether it's the main content rather
// than the whole page.
func mainContent(ctx context.Context, doc *html.Node, pageURL *url.URL, result *Result, pageText int) (string, int, bool) {
	parser := readability.NewParser()
	parser.MaxElemsToParse = maxArticleElements
	// ParseDocument works on a copy, which leaves the page whole for converting if readability's pick is no good
	article, err := parser.ParseDocument(doc, pageURL)
	if err != nil {
		log.Debug().Ctx(ctx).Err(err).Str("url", tracing.RedactURL(pageURL.String())).Msg("Readability found no article, converting the whole page")
	} else {
		if title := strings.TrimSpace(article.Title()); title != "" {
			result.Title = title
		}
		result.Site = strings.TrimSpace(article.SiteName())
		// Readability sometimes takes the site's address for the author
		if byline := strings.TrimSpace(article.Byline()); !strings.Contains(byline, "://") {
			result.Byline = byline
		}
		if language := article.Language(); language != "" {
			result.Language = language
		}
		if published, err := article.PublishedTime(); err == nil {
			result.Published = published
		}
		if article.Node != nil {
			if length := textLength(article.Node); length >= minArticleText {
				if markdown := toMarkdown(ctx, article.Node, pageURL); markdown != "" {
					return markdown, length, true
				}
			}
		}
	}

	return toMarkdown(ctx, doc, pageURL), pageText, false
}

// invisibleElements hold markup a reader never sees. The parser runs with scripting on, as readability
// expects, so it keeps a <noscript> element's markup as text, which would otherwise count as the page's.
var invisibleElements = map[string]bool{"noscript": true, "template": true}

// unwantedElements hold nothing the model can read: media, embedded documents, and markup for scripts
var unwantedElements = map[string]bool{
	"img": true, "picture": true, "source": true, "svg": true, "video": true, "audio": true, "canvas": true,
	"iframe": true, "object": true, "embed": true, "script": true, "style": true, "noscript": true, "template": true,
}

// emptyLink is a markdown link left with no text, typically once the image it wrapped is gone
var emptyLink = regexp.MustCompile(`\[\s*\]\([^)]*\)`)

// blankLines matches the runs of blank lines a conversion leaves between blocks
var blankLines = regexp.MustCompile(`\n[ \t]*(\n[ \t]*)+\n`)

// toMarkdown converts a node to markdown, with links made absolute. Images go: the model can't see them,
// and their addresses would take up much of what it reads.
func toMarkdown(ctx context.Context, node *html.Node, pageURL *url.URL) string {
	removeElements(node, unwantedElements)
	conv := converter.NewConverter(converter.WithPlugins(
		base.NewBasePlugin(),
		commonmark.NewCommonmarkPlugin(),
		table.NewTablePlugin(),
	))
	markdown, err := conv.ConvertNode(node, converter.WithContext(ctx), converter.WithDomain(pageURL.String()))
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", tracing.RedactURL(pageURL.String())).Msg("Failed to convert a page to markdown")
		return strings.TrimSpace(render.InnerText(node))
	}
	text := emptyLink.ReplaceAllString(string(markdown), "")
	return strings.TrimSpace(blankLines.ReplaceAllString(text, "\n\n"))
}

// removeElements removes the elements with the given names from a node
func removeElements(n *html.Node, names map[string]bool) {
	for child := n.FirstChild; child != nil; {
		next := child.NextSibling
		if child.Type == html.ElementNode && names[child.Data] {
			n.RemoveChild(child)
		} else {
			removeElements(child, names)
		}
		child = next
	}
}
