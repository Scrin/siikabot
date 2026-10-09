package webfetch

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// serve starts a test server for the rest of a test, lets fetches reach it, and empties the memory of blocks
// before and after
func serve(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	allowServer(t, server)
	forgetBlocks(t)
	return server
}

// allowServer lets fetches reach a test server. It listens on a loopback address, which the guard refuses, so
// it's let through by its port: anything else on loopback is still refused.
func allowServer(t *testing.T, server *httptest.Server) {
	t.Helper()
	allowed := netip.MustParseAddrPort(server.Listener.Addr().String())
	previous := allowedAddress
	allowedAddress = func(address netip.AddrPort) bool { return address == allowed || previous(address) }
	t.Cleanup(func() { allowedAddress = previous })
}

// forgetBlocks empties the memory of blocks before and after a test. The test servers all share a host.
func forgetBlocks(t *testing.T) {
	t.Helper()
	forget := func() {
		blockedHostsMutex.Lock()
		blockedHosts = map[string]blockedHost{}
		blockedHostsMutex.Unlock()
	}
	forget()
	t.Cleanup(forget)
}

// answer responds with a status, a content type and a body
func answer(status int, contentType, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

// htmlPage answers with an HTML page
func htmlPage(body string) http.HandlerFunc {
	return answer(http.StatusOK, "text/html; charset=utf-8", body)
}

// paragraphs makes an article's worth of paragraphs, each saying which one it is
func paragraphs(count int) string {
	var out strings.Builder
	for i := 1; i <= count; i++ {
		fmt.Fprintf(&out, "<p>This is paragraph number %d of the article. It has enough words in it to read like "+
			"real prose, which is what readability looks for when it picks the main content.</p>\n", i)
	}
	return out.String()
}

// menu makes a navigation menu with many links
func menu(count int) string {
	var out strings.Builder
	out.WriteString("<nav><ul>")
	for i := 1; i <= count; i++ {
		fmt.Fprintf(&out, `<li><a href="/section/%d">Menu item %d</a></li>`, i, i)
	}
	out.WriteString("</ul></nav>")
	return out.String()
}

func TestFetchFindsTheArticleAfterLotsOfScript(t *testing.T) {
	script := "<script>var data = \"" + strings.Repeat("x", 400*1024) + "\";</script>"
	server := serve(t, htmlPage(`<!doctype html><html lang="en"><head><title>Big page</title>`+script+
		`</head><body>`+menu(30)+`<article><h1>The heading</h1>`+paragraphs(6)+`</article></body></html>`))

	result := Fetch(context.Background(), server.URL+"/article")

	if result.Outcome != OutcomeOK {
		t.Fatalf("outcome %q (%s), want ok", result.Outcome, result.Reason)
	}
	if !strings.Contains(result.Content, "This is paragraph number 6 of the article.") {
		t.Errorf("the content lacks the article's end:\n%s", result.Content)
	}
	if result.Title != "Big page" || result.Language != "en" {
		t.Errorf("title %q and language %q, want Big page and en", result.Title, result.Language)
	}
}

func TestFetchLeavesTheMenusOut(t *testing.T) {
	server := serve(t, htmlPage(`<html><head><title>News</title></head><body><header>`+menu(100)+
		`</header><main><article><h1>Story</h1>`+paragraphs(5)+`</article></main><footer>`+menu(40)+
		`</footer></body></html>`))

	result := Fetch(context.Background(), server.URL)

	if result.Outcome != OutcomeOK {
		t.Fatalf("outcome %q, want ok", result.Outcome)
	}
	if !strings.Contains(result.Content, "This is paragraph number 1 of the article.") {
		t.Errorf("the content lacks the article:\n%s", result.Content)
	}
	if strings.Contains(result.Content, "Menu item 57") {
		t.Errorf("the content has the menus in it:\n%s", result.Content)
	}
}

// A page without enough main content for readability is converted whole, as the tool did before
func TestFetchConvertsAShortPageWhole(t *testing.T) {
	server := serve(t, htmlPage(`<html><body><h1>Opening hours</h1><ul><li>Monday to Friday 9–17</li>`+
		`<li>Saturday 10–14</li><li>Sunday closed</li></ul><p>Call us on 040 123 4567 if you have questions `+
		`about the service. We answer on weekdays and on Saturdays during the opening hours. On public holidays `+
		`the office is closed.</p></body></html>`))

	result := Fetch(context.Background(), server.URL)

	if result.Outcome != OutcomeOK {
		t.Fatalf("outcome %q, want ok", result.Outcome)
	}
	for _, want := range []string{"# Opening hours", "- Saturday 10–14", "040 123 4567"} {
		if !strings.Contains(result.Content, want) {
			t.Errorf("the content lacks %q:\n%s", want, result.Content)
		}
	}
}

func TestFetchRecognisesAnAppShell(t *testing.T) {
	server := serve(t, htmlPage(`<html><head><title>Dashboard</title></head><body>`+
		`<noscript><div class="warning">You need to enable JavaScript to run this app.</div></noscript>`+
		`<div id="root"></div><script src="/static/js/main.js"></script></body></html>`))

	result := Fetch(context.Background(), server.URL)

	if result.Outcome != OutcomeThin || result.Reason != "javascript" {
		t.Errorf("outcome %q (%q), want thin because of JavaScript", result.Outcome, result.Reason)
	}
}

func TestFetchRecognisesBotChecks(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"cloudflare", 403, `<html><head><title>Just a moment...</title></head><body><script>window._cf_chl_opt={cvId: '3'};</script></body></html>`, "Cloudflare"},
		{"cloudflare_200", 200, `<html><head><title>Just a moment...</title></head><body>Checking your browser</body></html>`, "Cloudflare"},
		{"datadome", 403, `<html><body><script src="https://ct.captcha-delivery.com/c.js"></script></body></html>`, "DataDome"},
		{"human", 403, `<html><body><div id="px-captcha"></div><p>Press and hold to confirm you are human</p></body></html>`, "HUMAN"},
		{"akamai", 403, `<HTML><HEAD><TITLE>Access Denied</TITLE></HEAD><BODY>You don't have permission to access this server.<P>Reference #18.6d3e1402.1791578893.2b7a1f</P><P>https://errors.edgesuite.net/18.6d3e1402.1791578893.2b7a1f</P></BODY></HTML>`, "Akamai"},
		{"vercel", 429, `<html><head><title>Vercel Security Checkpoint</title></head><body>We're verifying your browser</body></html>`, "Vercel"},
		{"reddit", 200, `<html><body>You've been blocked by network security.</body></html>`, "Reddit"},
		{"recaptcha", 200, `<html><body><form><div class="g-recaptcha" data-sitekey="abc"></div></form></body></html>`, "reCAPTCHA"},
		{"hcaptcha", 200, `<html><body><div class="h-captcha" data-sitekey="abc"></div></body></html>`, "hCaptcha"},
		{"turnstile", 200, `<html><body><div class="cf-turnstile" data-sitekey="abc"></div></body></html>`, "Cloudflare Turnstile"},
		{"cloudflare_503", 503, `<html><head><title>Just a moment...</title></head><body></body></html>`, "Cloudflare"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := serve(t, answer(test.status, "text/html", test.body))

			result := Fetch(context.Background(), server.URL)

			if result.Outcome != OutcomeBlocked || result.Reason != test.want {
				t.Errorf("outcome %q (%q), want blocked by %s", result.Outcome, result.Reason, test.want)
			}
			if result.Content != "" {
				t.Errorf("a bot check's text is passed on: %q", result.Content)
			}
		})
	}
}

// Ordinary pages carry the same widgets and scripts, in forms and on every page behind Cloudflare
func TestFetchDoesNotMistakeAPageForABotCheck(t *testing.T) {
	server := serve(t, htmlPage(`<html><head><script src="/cdn-cgi/challenge-platform/scripts/jsd/main.js"></script>`+
		`</head><body><article>`+paragraphs(8)+`</article><form><div class="g-recaptcha" data-sitekey="abc"></div>`+
		`<button>Send</button></form></body></html>`))

	result := Fetch(context.Background(), server.URL)

	if result.Outcome != OutcomeOK {
		t.Errorf("outcome %q (%q), want ok", result.Outcome, result.Reason)
	}
}

func TestFetchJudgesErrorStatuses(t *testing.T) {
	for _, test := range []struct {
		status      int
		contentType string
		body        string
		want        Outcome
		wantContent string
	}{
		{401, "text/plain", "Unauthorized", OutcomeBlocked, "Unauthorized"},
		{403, "text/html", "<html><body><h1>403 Forbidden</h1><hr>nginx</body></html>", OutcomeBlocked, "403 Forbidden nginx"},
		{404, "text/html", "<html><body>" + menu(10) + "<h1>Page not found</h1></body></html>", OutcomeNotFound, ""},
		{410, "text/plain", "Gone", OutcomeNotFound, ""},
		{400, "application/json", `{"error": "parameter 'place' is missing"}`, OutcomeRejected, `{"error": "parameter 'place' is missing"}`},
		{422, "text/plain", "Unprocessable", OutcomeRejected, "Unprocessable"},
		{500, "text/plain", "Internal Server Error", OutcomeError, "Internal Server Error"},
		{503, "text/plain", "Service Unavailable", OutcomeError, "Service Unavailable"},
	} {
		t.Run(fmt.Sprint(test.status), func(t *testing.T) {
			server := serve(t, answer(test.status, test.contentType, test.body))

			result := Fetch(context.Background(), server.URL)

			if result.Outcome != test.want || result.StatusCode != test.status {
				t.Errorf("outcome %q with status %d, want %q", result.Outcome, result.StatusCode, test.want)
			}
			if result.Content != test.wantContent {
				t.Errorf("content %q, want %q", result.Content, test.wantContent)
			}
		})
	}
}

// FMI's API explains a bad query in XML, after a long preamble
func TestFetchKeepsTheExplanationOfAnXMLError(t *testing.T) {
	server := serve(t, answer(400, "application/xml", `<?xml version="1.0" encoding="UTF-8"?>
<ExceptionReport xmlns="http://www.opengis.net/ows/1.1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="http://www.opengis.net/ows/1.1 http://schemas.opengis.net/ows/1.1.0/owsExceptionReport.xsd" version="2.0.0" xml:lang="eng">
  <Exception exceptionCode="OperationParsingFailed" locator="parameters">
    <ExceptionText>Unrecognized parameter 'PM25_PT1M_1'</ExceptionText>
  </Exception>
</ExceptionReport>`))

	result := Fetch(context.Background(), server.URL+"/wfs")

	if result.Outcome != OutcomeRejected || !strings.HasPrefix(result.Content, "Unrecognized parameter 'PM25_PT1M_1'") {
		t.Errorf("outcome %q with content %q, want rejected with FMI's explanation", result.Outcome, result.Content)
	}
}

func TestFetchShortensLongErrorText(t *testing.T) {
	server := serve(t, answer(400, "text/plain", strings.Repeat("ä", 2000)))

	result := Fetch(context.Background(), server.URL)

	if got := len([]rune(result.Content)); got != maxExcerpt+1 || !strings.HasSuffix(result.Content, "…") {
		t.Errorf("the excerpt has %d characters, want %d and an ellipsis", got, maxExcerpt+1)
	}
}

// Rate limits and bot checks cover a whole site for a while, so the bot doesn't ask again
func TestFetchRemembersRateLimits(t *testing.T) {
	var requests atomic.Int32
	server := serve(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))

	first := Fetch(context.Background(), server.URL+"/a")
	second := Fetch(context.Background(), server.URL+"/b")

	if first.Outcome != OutcomeBlocked || first.Remembered {
		t.Errorf("the first fetch is %q, remembered %v, want a fresh block", first.Outcome, first.Remembered)
	}
	if second.Outcome != OutcomeBlocked || !second.Remembered || second.StatusCode != 429 {
		t.Errorf("the second fetch is %q with status %d, remembered %v, want the remembered block",
			second.Outcome, second.StatusCode, second.Remembered)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("the site was asked %d times, want once", got)
	}
}

// A plain refusal may be about the one page, so it isn't remembered
func TestFetchDoesNotRememberAPlainRefusal(t *testing.T) {
	var requests atomic.Int32
	server := serve(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))

	Fetch(context.Background(), server.URL+"/private")
	result := Fetch(context.Background(), server.URL+"/public")

	if result.Remembered || requests.Load() != 2 {
		t.Errorf("remembered %v after %d requests, want the site asked twice", result.Remembered, requests.Load())
	}
}

func TestFetchReadsOtherTypes(t *testing.T) {
	for _, test := range []struct {
		name        string
		contentType string
		body        string
		want        Outcome
		wantType    string
		wantReason  string
	}{
		{"json", "application/json", `{"temperature": 12.5}`, OutcomeOK, "application/json", ""},
		{"text", "text/plain; charset=utf-8", "Plain text", OutcomeOK, "text/plain", ""},
		{"feed", "application/rss+xml", `<rss><channel><title>News</title></channel></rss>`, OutcomeOK, "application/rss+xml", ""},
		{"empty", "text/plain", "", OutcomeThin, "text/plain", "empty"},
		{"image", "image/png", "\x89PNG\r\n\x1a\n", OutcomeUnsupported, "", "image/png"},
		{"zip", "application/zip", "PK\x03\x04", OutcomeUnsupported, "", "application/zip"},
		{"sniffed_pdf", "application/octet-stream", string(makePDF("", [][]string{{strings.Repeat("Text ", 60)}})), OutcomeOK, "application/pdf", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := serve(t, answer(http.StatusOK, test.contentType, test.body))

			result := Fetch(context.Background(), server.URL)

			if result.Outcome != test.want || result.MediaType != test.wantType || result.Reason != test.wantReason {
				t.Errorf("outcome %q, type %q, reason %q; want %q, %q, %q", result.Outcome, result.MediaType,
					result.Reason, test.want, test.wantType, test.wantReason)
			}
		})
	}
}

func TestFetchDecodesCharacterSets(t *testing.T) {
	// "Hyvää päivää" in ISO-8859-1
	latin1 := "Hyv\xe4\xe4 p\xe4iv\xe4\xe4"
	article := func(text string) string {
		return "<article><p>" + strings.Repeat(text+" ", 30) + "</p></article>"
	}

	for _, test := range []struct {
		name        string
		contentType string
		body        string
	}{
		{"declared_in_header", "text/html; charset=iso-8859-1", "<html><body>" + article(latin1) + "</body></html>"},
		{"declared_in_meta", "text/html", `<html><head><meta charset="iso-8859-1"></head><body>` + article(latin1) + "</body></html>"},
		// Without a declaration, a page whose first kilobyte is ASCII would be taken for windows-1252
		{"undeclared_utf8", "text/html", "<html><head><title>" + strings.Repeat("x", 2000) + "</title></head><body>" +
			article("Hyvää päivää") + "</body></html>"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := serve(t, answer(http.StatusOK, test.contentType, test.body))

			result := Fetch(context.Background(), server.URL)

			if !strings.Contains(result.Content, "Hyvää päivää") {
				t.Errorf("the content isn't decoded:\n%.200s", result.Content)
			}
		})
	}
}

func TestFetchRecognisesPaywalls(t *testing.T) {
	jsonLD := func(data string) string {
		return `<html><head><script type="application/ld+json">` + data + `</script></head><body><article>` +
			paragraphs(3) + `<div class="paywall"></div></article></body></html>`
	}

	for _, test := range []struct {
		name string
		page string
		want Outcome
	}{
		{"not_free", jsonLD(`{"@context":"https://schema.org","@type":"NewsArticle","isAccessibleForFree":false,"hasPart":{"@type":"WebPageElement","isAccessibleForFree":false,"cssSelector":".paywall"}}`), OutcomePaywalled},
		{"not_free_as_text", jsonLD(`{"@type":"NewsArticle","isAccessibleForFree":"False"}`), OutcomePaywalled},
		{"in_a_graph", jsonLD(`{"@context":"https://schema.org","@graph":[{"@type":"WebSite","name":"News"},{"@type":"NewsArticle","isAccessibleForFree":false}]}`), OutcomePaywalled},
		{"in_a_list", jsonLD(`[{"@type":"BreadcrumbList"},{"@type":"NewsArticle","isAccessibleForFree":false}]`), OutcomePaywalled},
		// Only a part of the page is for subscribers, which free articles can have too
		{"free_with_a_paid_part", jsonLD(`{"@type":"NewsArticle","isAccessibleForFree":true,"hasPart":{"@type":"WebPageElement","isAccessibleForFree":false}}`), OutcomeOK},
		{"broken_json", jsonLD(`{"isAccessibleForFree": false,`), OutcomeOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := serve(t, htmlPage(test.page))

			result := Fetch(context.Background(), server.URL)

			if result.Outcome != test.want {
				t.Errorf("outcome %q, want %q", result.Outcome, test.want)
			}
			if !strings.Contains(result.Content, "paragraph number 3") {
				t.Errorf("the free part is missing:\n%s", result.Content)
			}
		})
	}
}

func TestFetchFollowsRedirects(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/old", http.RedirectHandler("/new", http.StatusMovedPermanently))
	mux.Handle("/new", htmlPage("<article>"+paragraphs(3)+"</article>"))
	server := serve(t, mux)

	result := Fetch(context.Background(), server.URL+"/old")

	if result.Outcome != OutcomeOK || result.URL != server.URL+"/new" {
		t.Errorf("outcome %q at %s, want ok at the redirect's end", result.Outcome, result.URL)
	}
}

func TestFetchStopsEndlessRedirects(t *testing.T) {
	server := serve(t, http.RedirectHandler("/again", http.StatusFound))

	result := Fetch(context.Background(), server.URL)

	if result.Outcome != OutcomeError || result.Reason != "too_many_redirects" {
		t.Errorf("outcome %q (%q), want an error for too many redirects", result.Outcome, result.Reason)
	}
}

// The address is checked when connecting, so a public page can't send the bot somewhere private
func TestFetchRefusesRedirectsToPrivateAddresses(t *testing.T) {
	for _, target := range []string{"http://169.254.169.254/latest/meta-data/", "http://10.0.0.1/", "http://[::1]:8080/"} {
		t.Run(target, func(t *testing.T) {
			server := serve(t, http.RedirectHandler(target, http.StatusFound))

			result := Fetch(context.Background(), server.URL)

			if result.Outcome != OutcomeDeniedAddress {
				t.Errorf("outcome %q (%q), want the address denied", result.Outcome, result.Reason)
			}
		})
	}
}

// Without a test's say-so, the guard refuses the test's own server, which listens on loopback
func TestFetchRefusesLoopback(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()

	result := Fetch(context.Background(), server.URL)

	if result.Outcome != OutcomeDeniedAddress || requests.Load() != 0 {
		t.Errorf("outcome %q after %d requests, want the address denied before any request", result.Outcome, requests.Load())
	}
}

func TestFetchDoesNotAskReddit(t *testing.T) {
	for _, rawURL := range []string{"https://www.reddit.com/r/Finland/", "https://old.reddit.com/", "https://redd.it/abc"} {
		result := Fetch(context.Background(), rawURL)
		if result.Outcome != OutcomeDeniedHost || result.Reason != "Reddit" {
			t.Errorf("%s: outcome %q (%q), want Reddit denied", rawURL, result.Outcome, result.Reason)
		}
	}
}

func TestFetchRejectsInvalidURLs(t *testing.T) {
	for _, rawURL := range []string{"ftp://example.com/file", "https:///path", "https://user:secret@example.com/", "example.com", "%zz"} {
		result := Fetch(context.Background(), rawURL)
		if result.Outcome != OutcomeInvalidURL || result.Reason == "" {
			t.Errorf("%s: outcome %q (%q), want an invalid URL with a reason", rawURL, result.Outcome, result.Reason)
		}
	}
}

func TestFetchHoldsToProvenance(t *testing.T) {
	var requests atomic.Int32
	server := serve(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		htmlPage("<article>"+paragraphs(3)+"</article>").ServeHTTP(w, nil)
	}))
	ctx := WithProvenance(context.Background(), NewProvenance(true, "See "+server.URL+"/seen for more."))

	seen := Fetch(ctx, server.URL+"/seen")
	unseen := Fetch(ctx, server.URL+"/seen?leak=secret")

	if seen.Outcome != OutcomeOK {
		t.Errorf("a URL the turn has seen is %q, want ok", seen.Outcome)
	}
	if unseen.Outcome != OutcomeDeniedProvenance || requests.Load() != 1 {
		t.Errorf("a URL the turn hasn't seen is %q after %d requests, want it denied without asking",
			unseen.Outcome, requests.Load())
	}
}

func TestFetchExplainsCertificateProblems(t *testing.T) {
	for _, test := range []struct {
		name string
		cert func(t *testing.T) tls.Certificate
		want string
	}{
		{"expired", func(t *testing.T) tls.Certificate {
			return makeCertificate(t, []string{"127.0.0.1"}, time.Now().Add(-48*time.Hour), time.Now().Add(-24*time.Hour))
		}, "expired"},
		{"wrong_host", func(t *testing.T) tls.Certificate {
			return makeCertificate(t, []string{"example.com"}, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
		}, "wrong_host"},
		{"unknown_authority", func(t *testing.T) tls.Certificate {
			return makeCertificate(t, []string{"127.0.0.1"}, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
		}, "unknown_authority"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewUnstartedServer(htmlPage("<p>Secret</p>"))
			server.TLS = &tls.Config{Certificates: []tls.Certificate{test.cert(t)}}
			server.StartTLS()
			t.Cleanup(server.Close)
			allowServer(t, server)

			result := Fetch(context.Background(), server.URL)

			if result.Outcome != OutcomeBadCertificate || result.Reason != test.want {
				t.Errorf("outcome %q (%q), want a bad certificate (%s)", result.Outcome, result.Reason, test.want)
			}
		})
	}
}

// makeCertificate makes a self-signed certificate for some names, valid between two times
func makeCertificate(t *testing.T, names []string, notBefore, notAfter time.Time) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: names[0]},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, name := range names {
		if addr, err := netip.ParseAddr(name); err == nil {
			template.IPAddresses = append(template.IPAddresses, addr.AsSlice())
		} else {
			template.DNSNames = append(template.DNSNames, name)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func TestRedactError(t *testing.T) {
	err := &url.Error{Op: "Get", URL: "https://api.example.com/data?token=hunter2&city=Oulu", Err: errors.New("timeout")}

	redacted := redactError(fmt.Errorf("fetching: %w", err)).Error()

	if strings.Contains(redacted, "hunter2") || !strings.Contains(redacted, "city=Oulu") {
		t.Errorf("redactError() = %q, want the token gone and the rest kept", redacted)
	}
	if !strings.Contains(err.Error(), "hunter2") {
		t.Error("redactError() changed the original error")
	}
}
