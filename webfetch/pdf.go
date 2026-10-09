package webfetch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Scrin/siikabot/tracing"
	"github.com/ledongthuc/pdf"
	"github.com/rs/zerolog/log"
)

const (
	// maxPDFPages is how many pages of a PDF are read
	maxPDFPages = 50
	// pdfTimeout bounds reading a PDF's text. PDFs are untrusted input to a parser that can't be cancelled.
	pdfTimeout = 10 * time.Second
	// wordGap is the share of the font size that the gap between two glyphs on a line has to exceed to count
	// as a space. PDFs place glyphs rather than words, and many have no space characters at all.
	wordGap = 0.15
)

var errPDFTimeout = errors.New("reading the PDF took too long")

// pdfText is the text read from a PDF
type pdfText struct {
	text  string
	title string
	// pages is how many pages the PDF has, and read how many of them were read
	pages, read int
}

// readPDF reads a PDF's text
func readPDF(ctx context.Context, page response, result Result) Result {
	if page.truncated {
		result.Outcome, result.Reason = OutcomeUnsupported, "too_large"
		return result
	}

	text, err := extractPDF(ctx, page.body)
	if err != nil {
		log.Warn().Ctx(ctx).Err(err).Str("url", tracing.RedactURL(page.url.String())).Msg("Failed to read a PDF")
		result.Outcome, result.Reason = OutcomeUnsupported, "unreadable_pdf"
		return result
	}

	result.Title = text.title
	result.Content = text.text
	if text.read < text.pages {
		result.Content += fmt.Sprintf("\n\n[The PDF has %d pages. Only the first %d were read.]", text.pages, text.read)
	}
	result.Outcome = OutcomeOK
	if len(strings.TrimSpace(text.text)) < thinText {
		// Most likely a scan: pictures of pages, with no text in them
		result.Outcome, result.Reason = OutcomeThin, "no_text"
	}
	return result
}

// extractPDF reads a PDF's text in a goroutine of its own, so a PDF that sends the parser into a loop can't
// hold up the turn, and a panic in the parser doesn't reach the bot
func extractPDF(ctx context.Context, data []byte) (pdfText, error) {
	ctx, cancel := context.WithTimeout(ctx, pdfTimeout)
	defer cancel()

	type outcome struct {
		text pdfText
		err  error
	}
	done := make(chan outcome, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				done <- outcome{err: fmt.Errorf("the PDF parser panicked: %v", recovered)}
			}
		}()
		text, err := pdfTextOf(ctx, data)
		done <- outcome{text, err}
	}()

	select {
	case result := <-done:
		return result.text, result.err
	case <-ctx.Done():
		return pdfText{}, errPDFTimeout
	}
}

// pdfTextOf reads the text of a PDF's first pages, stopping early once ctx is done
func pdfTextOf(ctx context.Context, data []byte) (pdfText, error) {
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return pdfText{}, err
	}

	result := pdfText{pages: reader.NumPage()}
	if info := reader.Trailer().Key("Info"); !info.IsNull() {
		result.title = strings.TrimSpace(info.Key("Title").Text())
	}

	var text strings.Builder
	for i := 1; i <= result.pages && i <= maxPDFPages; i++ {
		if ctx.Err() != nil {
			return pdfText{}, errPDFTimeout
		}
		page := reader.Page(i)
		if page.V.IsNull() {
			continue
		}
		writePageText(&text, page.Content().Text)
		text.WriteString("\n\n")
		result.read = i
	}
	result.text = strings.TrimSpace(text.String())
	return result, nil
}

// writePageText writes out a page's glyphs, starting a new line where the glyphs move to another line and
// putting spaces where the gap between two glyphs is wide enough to be one. Some PDFs end their lines with a
// line break of their own, which the parser passes on as a glyph.
func writePageText(out *strings.Builder, glyphs []pdf.Text) {
	lineStart := true
	for i, glyph := range glyphs {
		if i > 0 && !lineStart {
			previous := glyphs[i-1]
			if glyph.Y != previous.Y {
				out.WriteString("\n")
			} else if glyph.X-(previous.X+previous.W) > previous.FontSize*wordGap {
				out.WriteString(" ")
			}
		}
		out.WriteString(glyph.S)
		lineStart = strings.HasSuffix(glyph.S, "\n")
	}
}
