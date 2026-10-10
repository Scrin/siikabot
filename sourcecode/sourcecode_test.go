package sourcecode

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

const testCommit = "232d860b0ea0bafb9e536b438c55f2241ada02f0"

var testRepo = Repository{Owner: "owner", Name: "siikabot"}

// archiveEntry is a file, directory or link in a test archive
type archiveEntry struct {
	name     string
	typeflag byte
	content  string
}

// testArchive builds a repository archive the way git archive does: a global header naming the commit,
// then everything under a directory named after the repository and the commit
func testArchive(t *testing.T, commit string, entries ...archiveEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	if commit != "" {
		if err := tw.WriteHeader(&tar.Header{
			Typeflag:   tar.TypeXGlobalHeader,
			Name:       "pax_global_header",
			PAXRecords: map[string]string{"comment": commit},
		}); err != nil {
			t.Fatal(err)
		}
	}
	top := "siikabot-" + commit + "/"
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: top, Mode: 0o755}); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		header := &tar.Header{Typeflag: entry.typeflag, Name: top + entry.name, Mode: 0o644}
		switch entry.typeflag {
		case tar.TypeReg:
			header.Size = int64(len(entry.content))
		case tar.TypeSymlink:
			header.Linkname = entry.content
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if entry.typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(entry.content)); err != nil {
				t.Fatal(err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func file(name, content string) archiveEntry {
	return archiveEntry{name: name, typeflag: tar.TypeReg, content: content}
}

func TestParseRepository(t *testing.T) {
	for _, rawURL := range []string{
		"https://github.com/owner/siikabot",
		"https://github.com/owner/siikabot/",
		"https://github.com/owner/siikabot.git",
		"https://www.github.com/owner/siikabot",
		"http://GitHub.com/owner/siikabot",
		" https://github.com/owner/siikabot ",
	} {
		repo, err := ParseRepository(rawURL)
		if err != nil {
			t.Errorf("ParseRepository(%q) failed: %v", rawURL, err)
			continue
		}
		if repo != testRepo {
			t.Errorf("ParseRepository(%q) = %+v, want %+v", rawURL, repo, testRepo)
		}
	}

	for _, rawURL := range []string{
		"",
		"github.com/owner/siikabot",
		"https://gitlab.com/owner/siikabot",
		"https://github.com.example.org/owner/siikabot",
		"https://github.com/owner",
		"https://github.com/owner/siikabot/tree/master",
		"https://github.com/owner/.git",
		"https://github.com/../siikabot",
		"https://github.com/owner/siika%20bot",
		"ftp://github.com/owner/siikabot",
	} {
		if repo, err := ParseRepository(rawURL); err == nil {
			t.Errorf("ParseRepository(%q) = %+v, want an error", rawURL, repo)
		}
	}
}

func TestLinks(t *testing.T) {
	if got, want := testRepo.FileURL(testCommit, "commands/chat/prompt.go"),
		"https://github.com/owner/siikabot/blob/"+testCommit+"/commands/chat/prompt.go"; got != want {
		t.Errorf("FileURL() = %q, want %q", got, want)
	}
	if got, want := testRepo.FileURL("HEAD", "docs/a file#1.md"),
		"https://github.com/owner/siikabot/blob/HEAD/docs/a%20file%231.md"; got != want {
		t.Errorf("FileURL() = %q, want %q", got, want)
	}
	if got, want := testRepo.DirectoryURL(testCommit, "commands/remind"),
		"https://github.com/owner/siikabot/tree/"+testCommit+"/commands/remind"; got != want {
		t.Errorf("DirectoryURL() = %q, want %q", got, want)
	}
	if got, want := testRepo.DirectoryURL(testCommit, ""), "https://github.com/owner/siikabot/tree/"+testCommit; got != want {
		t.Errorf("DirectoryURL() of the root = %q, want %q", got, want)
	}
}

// The archive's top directory is dropped from every path, the commit comes from its global header, and only
// text files are kept
func TestUnpack(t *testing.T) {
	archive := testArchive(t, testCommit,
		file("main.go", "package main\n\nfunc main() {}\n"),
		archiveEntry{name: "commands/", typeflag: tar.TypeDir},
		file("commands/remind/remind.go", "package remind\r\n// Handle handles !remind\r\n"),
		file("README.md", "# SiikaBot"),
		file("empty.txt", ""),
		file("web_frontend/public/logo.png", "\x89PNG\r\n\x1a\n\x00\x00"),
		file("latin1.txt", "p\xe4iv\xe4\n"),
		archiveEntry{name: "link.go", typeflag: tar.TypeSymlink, content: "main.go"},
	)

	commit, files, err := unpack(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	if commit != testCommit {
		t.Errorf("commit = %q, want %q", commit, testCommit)
	}

	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	if want := []string{"README.md", "commands/remind/remind.go", "empty.txt", "main.go"}; !slices.Equal(paths, want) {
		t.Errorf("files = %v, want %v", paths, want)
	}

	snapshot := &Snapshot{Repository: testRepo, Commit: commit, Files: files}
	remind, _ := snapshot.File("commands/remind/remind.go")
	if want := []string{"package remind", "// Handle handles !remind"}; !slices.Equal(remind.Lines, want) {
		t.Errorf("lines of a CRLF file = %q, want %q", remind.Lines, want)
	}
	if readme, _ := snapshot.File("README.md"); !slices.Equal(readme.Lines, []string{"# SiikaBot"}) {
		t.Errorf("a file without a final newline has lines %q", readme.Lines)
	}
	if empty, _ := snapshot.File("empty.txt"); len(empty.Lines) != 0 {
		t.Errorf("an empty file has lines %q", empty.Lines)
	}
}

// Without the global header the commit isn't known, and links point at the default branch instead
func TestUnpackWithoutCommit(t *testing.T) {
	commit, files, err := unpack(bytes.NewReader(testArchive(t, "", file("main.go", "package main\n"))))
	if err != nil {
		t.Fatal(err)
	}
	if commit != "" || len(files) != 1 {
		t.Fatalf("commit = %q, files = %v", commit, files)
	}
	if ref := (&Snapshot{Files: files}).Ref(); ref != "HEAD" {
		t.Errorf("Ref() without a commit = %q, want HEAD", ref)
	}
}

func TestUnpackRejectsWhatIsNotAnArchive(t *testing.T) {
	if _, _, err := unpack(bytes.NewReader([]byte("<html>Not Found</html>"))); err == nil {
		t.Error("a page that isn't an archive was unpacked")
	}
}

func testSnapshot() *Snapshot {
	return &Snapshot{Repository: testRepo, Commit: testCommit, Files: []File{
		{Path: "commands/remind/parse.go", Lines: []string{"package remind", "func parseTime() {}"}},
		{Path: "commands/remind/remind.go", Lines: []string{"package remind", "// Handle handles !remind", "func Handle() {}"}},
		{Path: "commands/reminder.txt", Lines: []string{"not in the directory"}},
		{Path: "llmtools/reminder.go", Lines: []string{"package llmtools", "var ReminderToolDefinition"}},
		{Path: "main.go", Lines: []string{"package main"}},
	}}
}

func TestUnder(t *testing.T) {
	snapshot := testSnapshot()
	paths := func(files []File) []string {
		var paths []string
		for _, f := range files {
			paths = append(paths, f.Path)
		}
		return paths
	}

	if got := paths(snapshot.Under("commands/remind")); !slices.Equal(got, []string{"commands/remind/parse.go", "commands/remind/remind.go"}) {
		t.Errorf("Under(a directory) = %v", got)
	}
	if got := paths(snapshot.Under("main.go")); !slices.Equal(got, []string{"main.go"}) {
		t.Errorf("Under(a file) = %v", got)
	}
	if got := snapshot.Under(""); len(got) != len(snapshot.Files) {
		t.Errorf("Under(the root) has %d files, want all %d", len(got), len(snapshot.Files))
	}
	// A directory's name is a whole path segment, not a prefix of one
	if got := snapshot.Under("commands/rem"); len(got) != 0 {
		t.Errorf("Under(part of a name) = %v", paths(got))
	}
}

func TestSearch(t *testing.T) {
	snapshot := testSnapshot()

	got := snapshot.Search(regexp.MustCompile(`(?i)handle`), "commands")
	want := []Match{
		{Path: "commands/remind/remind.go", Line: 2, Text: "// Handle handles !remind"},
		{Path: "commands/remind/remind.go", Line: 3, Text: "func Handle() {}"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("Search() = %+v, want %+v", got, want)
	}

	if got := snapshot.Search(regexp.MustCompile(`package`), "llmtools"); len(got) != 1 || got[0].Path != "llmtools/reminder.go" {
		t.Errorf("Search() limited to a directory = %+v", got)
	}
}

// resetCache empties the cache and points downloads at a test server for the rest of a test
func resetCache(t *testing.T, serverURL string) {
	t.Helper()
	original := githubURL
	githubURL = serverURL
	clear := func() {
		cache.Lock()
		defer cache.Unlock()
		cache.repository, cache.snapshot, cache.tried = Repository{}, nil, time.Time{}
	}
	clear()
	t.Cleanup(func() {
		githubURL = original
		clear()
	})
}

// ageCache makes the cached download look older than maxAge
func ageCache() {
	cache.Lock()
	defer cache.Unlock()
	cache.tried = time.Now().Add(-maxAge - time.Second)
}

func TestGetDownloadsTheDefaultBranchOnce(t *testing.T) {
	archive := testArchive(t, testCommit, file("main.go", "package main\n"))
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/owner/siikabot/archive/HEAD.tar.gz" {
			http.NotFound(w, r)
			return
		}
		w.Write(archive)
	}))
	defer server.Close()
	resetCache(t, server.URL)

	for range 3 {
		snapshot, err := Get(context.Background(), testRepo)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.Commit != testCommit || snapshot.Repository != testRepo || len(snapshot.Files) != 1 {
			t.Fatalf("Get() = %+v", snapshot)
		}
	}
	if n := requests.Load(); n != 1 {
		t.Errorf("the branch was downloaded %d times, want once", n)
	}

	ageCache()
	if _, err := Get(context.Background(), testRepo); err != nil {
		t.Fatal(err)
	}
	if n := requests.Load(); n != 2 {
		t.Errorf("an old download wasn't replaced: %d downloads", n)
	}
}

// When GitHub fails, the previous download is used, and the next try waits for maxAge
func TestGetFallsBackOnThePreviousDownload(t *testing.T) {
	archive := testArchive(t, testCommit, file("main.go", "package main\n"))
	var failing atomic.Bool
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if failing.Load() {
			http.Error(w, "unicorn", http.StatusServiceUnavailable)
			return
		}
		w.Write(archive)
	}))
	defer server.Close()
	resetCache(t, server.URL)

	if _, err := Get(context.Background(), testRepo); err != nil {
		t.Fatal(err)
	}
	failing.Store(true)
	ageCache()

	for range 2 {
		snapshot, err := Get(context.Background(), testRepo)
		if err != nil {
			t.Fatalf("a failed download wasn't covered by the previous one: %v", err)
		}
		if snapshot.Commit != testCommit {
			t.Errorf("Get() = %+v", snapshot)
		}
	}
	if n := requests.Load(); n != 2 {
		t.Errorf("%d downloads, want a single retry after the first", n)
	}
}

func TestGetFailsWithoutAnyDownload(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	resetCache(t, server.URL)

	if snapshot, err := Get(context.Background(), testRepo); err == nil {
		t.Errorf("Get() = %+v, want an error", snapshot)
	}
}
