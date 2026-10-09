package webfetch

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

// makePDF builds a PDF with a title and a page for each list of words. The words are placed apart without
// spaces between them, as many PDFs place them.
func makePDF(title string, pages [][]string) []byte {
	// Objects 1 to 4 are the catalog, the page tree, the font and the document info. Each page then has an
	// object of its own and one for its content.
	kids := make([]string, len(pages))
	for i := range pages {
		kids[i] = fmt.Sprintf("%d 0 R", 5+2*i)
	}
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pages)),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf("<< /Title (%s) >>", title),
	}
	for i, words := range pages {
		var content strings.Builder
		content.WriteString("BT /F1 12 Tf 72 720 Td")
		for j, word := range words {
			if j > 0 {
				content.WriteString(" 60 0 Td")
			}
			fmt.Fprintf(&content, " (%s) Tj", word)
		}
		content.WriteString(" ET")
		objects = append(objects,
			fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents %d 0 R "+
				"/Resources << /Font << /F1 3 0 R >> >> >>", 6+2*i),
			fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", content.Len(), content.String()),
		)
	}

	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, object := range objects {
		offsets[i] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R /Info 4 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return out.Bytes()
}

// readTestPDF reads a PDF as if it had been fetched
func readTestPDF(data []byte, truncated bool) Result {
	pageURL, _ := url.Parse("https://example.com/document.pdf")
	page := response{url: pageURL, statusCode: 200, contentType: "application/pdf", body: data, truncated: truncated}
	return readPDF(context.Background(), page, Result{URL: pageURL.String()})
}

func TestReadPDF(t *testing.T) {
	words := []string{"Hello", "World"}
	for range 40 {
		words = append(words, "more")
	}
	result := readTestPDF(makePDF("Test document", [][]string{words, {"Second", "page"}}), false)

	if result.Outcome != OutcomeOK || result.Title != "Test document" {
		t.Fatalf("outcome %q with title %q, want ok with the document's title", result.Outcome, result.Title)
	}
	// The words have no spaces between them in the PDF: the gaps between them make the spaces
	if !strings.HasPrefix(result.Content, "Hello World more more") || !strings.HasSuffix(result.Content, "\n\nSecond page") {
		t.Errorf("content:\n%s", result.Content)
	}
}

func TestReadPDFReadsOnlyTheFirstPages(t *testing.T) {
	pages := make([][]string, maxPDFPages+2)
	for i := range pages {
		pages[i] = []string{fmt.Sprintf("Page%d", i+1), "has", "some", "words", "on", "it", "to", "read"}
	}
	result := readTestPDF(makePDF("Long", pages), false)

	if !strings.Contains(result.Content, fmt.Sprintf("Page%d has", maxPDFPages)) ||
		strings.Contains(result.Content, fmt.Sprintf("Page%d has", maxPDFPages+1)) {
		t.Errorf("the PDF wasn't read up to page %d exactly:\n%s", maxPDFPages, result.Content)
	}
	if want := fmt.Sprintf("[The PDF has %d pages. Only the first %d were read.]", maxPDFPages+2, maxPDFPages); !strings.HasSuffix(result.Content, want) {
		t.Errorf("the content doesn't end with %q", want)
	}
}

// A scanned PDF has pictures of its pages and no text
func TestReadPDFWithoutText(t *testing.T) {
	result := readTestPDF(makePDF("Scan", [][]string{{}}), false)

	if result.Outcome != OutcomeThin || result.Reason != "no_text" {
		t.Errorf("outcome %q (%q), want thin with no text", result.Outcome, result.Reason)
	}
}

func TestReadPDFThatIsBroken(t *testing.T) {
	valid := makePDF("Broken", [][]string{{"Hello"}})
	for name, data := range map[string][]byte{
		"not_a_pdf":     []byte("%PDF-1.4\nthis is not really a PDF"),
		"cut_in_half":   valid[:len(valid)/2],
		"bad_xref":      bytes.Replace(valid, []byte("startxref\n"), []byte("startxref\n9"), 1),
		"empty":         nil,
		"garbage_in_it": append(append([]byte{}, valid[:200]...), bytes.Repeat([]byte{0xff}, 500)...),
	} {
		t.Run(name, func(t *testing.T) {
			result := readTestPDF(data, false)
			if result.Outcome != OutcomeUnsupported || result.Reason != "unreadable_pdf" {
				t.Errorf("outcome %q (%q), want an unreadable PDF", result.Outcome, result.Reason)
			}
		})
	}
}

// A PDF cut at the read limit has lost its cross-reference table, which is at the end
func TestReadPDFThatIsTooLarge(t *testing.T) {
	result := readTestPDF(makePDF("Big", [][]string{{"Hello"}}), true)

	if result.Outcome != OutcomeUnsupported || result.Reason != "too_large" {
		t.Errorf("outcome %q (%q), want a PDF too large to read", result.Outcome, result.Reason)
	}
}
