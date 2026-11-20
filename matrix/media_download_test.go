package matrix

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Scrin/siikabot/config"
)

// fakeHomeserver serves media at the paths in files, answers anything else with 404, and counts the
// requests it gets
func fakeHomeserver(t *testing.T, files map[string][]byte) *atomic.Int32 {
	t.Helper()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		data, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	}))
	t.Cleanup(server.Close)

	previous := config.HomeserverURL
	config.HomeserverURL = server.URL
	t.Cleanup(func() { config.HomeserverURL = previous })
	return &requests
}

// Whoever sends an image chooses the URL in it, and the bot downloads media with its access token.
// Anything but a content URI on the bot's own homeserver could send the token anywhere.
func TestMessageImageURLRefusesAnythingButAContentURI(t *testing.T) {
	for name, content := range map[string]map[string]any{
		"url":           {"msgtype": "m.image", "body": "a.png", "url": "https://attacker.example/x"},
		"file":          {"msgtype": "m.image", "body": "a.png", "file": map[string]any{"url": "https://attacker.example/x"}},
		"thumbnail_url": {"msgtype": "m.image", "body": "a.png", "info": map[string]any{"thumbnail_url": "http://10.0.0.1/x"}},
		"thumbnail_file": {"msgtype": "m.image", "body": "a.png",
			"info": map[string]any{"thumbnail_file": map[string]any{"url": "https://attacker.example/_matrix/media/x"}}},
		"path_in_media_id": {"msgtype": "m.image", "body": "a.png", "url": "mxc://example.com/../../_matrix/client/v3/x"},
		"path_in_server":   {"msgtype": "m.image", "body": "a.png", "url": "mxc://example.com/x/abc"},
	} {
		t.Run(name, func(t *testing.T) {
			msg := messageFromEvent(testRoomID, messageEvent(content))
			if url, _, _, err := MessageImageURL(context.Background(), msg); err == nil {
				t.Errorf("MessageImageURL() = %q, want an error", url)
			}
		})
	}
}

func TestParseMediaURI(t *testing.T) {
	for raw, valid := range map[string]bool{
		"mxc://example.com/AbC-123_xyz":       true,
		"mxc://matrix.example.com:8448/media": true,
		"mxc://example.com/a/b":               false,
		"mxc://example.com/../x":              false,
		"mxc://example.com/a%2Fb":             false,
		"mxc://exa mple.com/a":                false,
		"mxc://example.com/a?b=c":             false,
		"mxc://user@example.com/a":            false,
		"mxc://example.com/":                  false,
		"mxc:///abc":                          false,
		"https://example.com/abc":             false,
		"":                                    false,
	} {
		if _, err := parseMediaURI(raw); (err == nil) != valid {
			t.Errorf("parseMediaURI(%q) error = %v, want valid %v", raw, err, valid)
		}
	}
}

func TestDownloadImageRequestsNothingForAURLThatIsntAContentURI(t *testing.T) {
	requests := fakeHomeserver(t, nil)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	}))
	defer other.Close()

	for _, raw := range []string{other.URL + "/_matrix/media/v3/download/x/y", "mxc://example.com/../y"} {
		if _, err := DownloadImage(context.Background(), raw, nil, nil); err == nil {
			t.Errorf("DownloadImage(%q) gave no error", raw)
		}
		if _, err := DownloadImage(context.Background(), raw, map[string]any{}, nil); err == nil {
			t.Errorf("DownloadImage(%q) of an encrypted file gave no error", raw)
		}
	}
	if n := requests.Load(); n != 0 {
		t.Errorf("%d requests were made for URLs that aren't content URIs", n)
	}
}

func TestDownloadImageFromTheHomeserver(t *testing.T) {
	png := testPNG(t)
	fakeHomeserver(t, map[string][]byte{"/_matrix/media/v3/download/example.com/cat": png})

	data, err := DownloadImage(context.Background(), "mxc://example.com/cat", nil, nil)
	if err != nil {
		t.Fatalf("DownloadImage() error = %v", err)
	}
	if !bytes.Equal(data, png) {
		t.Error("DownloadImage() didn't return the file the homeserver served")
	}
}

// An encrypted file goes through the same endpoints as any other, falling back when one fails
func TestDownloadImageDecryptsFromAFallbackEndpoint(t *testing.T) {
	png := testPNG(t)
	ciphertext, fileInfo := encryptFile(t, png, [32]byte{1, 2, 3}, [16]byte{4, 5, 6})
	requests := fakeHomeserver(t, map[string][]byte{"/_matrix/media/r0/download/example.com/file": ciphertext})

	data, err := DownloadImage(context.Background(), "mxc://example.com/file", fileInfo, map[string]any{})
	if err != nil {
		t.Fatalf("DownloadImage() error = %v", err)
	}
	if !bytes.Equal(data, png) {
		t.Error("DownloadImage() didn't decrypt the file back to the original")
	}
	if n := requests.Load(); n != 2 {
		t.Errorf("DownloadImage() made %d requests, want 2: v3, then r0", n)
	}
}

// A download that fails at every endpoint is an error, not a panic, even when the connection
// itself fails
func TestDownloadImageWhenTheHomeserverIsUnreachable(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()
	previous := config.HomeserverURL
	config.HomeserverURL = server.URL
	defer func() { config.HomeserverURL = previous }()

	_, fileInfo := encryptFile(t, testPNG(t), [32]byte{}, [16]byte{})
	for _, info := range []map[string]any{nil, fileInfo} {
		if _, err := DownloadImage(context.Background(), "mxc://example.com/file", info, map[string]any{}); err == nil {
			t.Error("DownloadImage() from an unreachable homeserver gave no error")
		}
	}
}

func TestDownloadImageRefusesAnEncryptedFileDeclaredTooLarge(t *testing.T) {
	requests := fakeHomeserver(t, nil)
	_, fileInfo := encryptFile(t, testPNG(t), [32]byte{}, [16]byte{})
	content := map[string]any{"info": map[string]any{"size": float64(maxMediaSize + 1)}}

	if _, err := DownloadImage(context.Background(), "mxc://example.com/file", fileInfo, content); err == nil {
		t.Error("DownloadImage() of a file declared too large gave no error")
	}
	if n := requests.Load(); n != 0 {
		t.Errorf("%d requests were made for a file declared too large", n)
	}
}
