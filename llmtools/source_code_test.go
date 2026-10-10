package llmtools

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/sourcecode"
)

const sourceTestCommit = "232d860b0ea0bafb9e536b438c55f2241ada02f0"

var sourceTestRepo = sourcecode.Repository{Owner: "owner", Name: "siikabot"}

// sourceSnapshot builds a snapshot of the test repository from files, which the caller gives sorted by path
func sourceSnapshot(files ...sourcecode.File) *sourcecode.Snapshot {
	return &sourcecode.Snapshot{Repository: sourceTestRepo, Commit: sourceTestCommit, Files: files}
}

// numberedLines makes n lines that say which line they are
func numberedLines(n int) []string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i+1)
	}
	return lines
}

var remindSnapshot = sourceSnapshot(
	sourcecode.File{Path: "commands/remind/remind.go", Lines: []string{
		"package remind",
		"",
		"// Handle handles the !remind command",
		"func Handle() {",
		"\tparseRemindTime()",
		"}",
	}},
	sourcecode.File{Path: "llmtools/reminder.go", Lines: []string{"package llmtools", "var ReminderToolDefinition"}},
	sourcecode.File{Path: "main.go", Lines: []string{"package main"}},
)

func TestCleanSourcePath(t *testing.T) {
	for given, want := range map[string]string{
		"":                        "",
		"/":                       "",
		".":                       "",
		" commands/remind/ ":      "commands/remind",
		"./main.go":               "main.go",
		"/commands//remind.go":    "commands/remind.go",
		"../../etc/passwd":        "etc/passwd",
		"commands/../config/x.go": "config/x.go",
	} {
		if got := cleanSourcePath(given); got != want {
			t.Errorf("cleanSourcePath(%q) = %q, want %q", given, got, want)
		}
	}
}

func TestSearchSource(t *testing.T) {
	got := searchSource(remindSnapshot, regexp.MustCompile(`(?i)remind`), "remind", "")

	for _, want := range []string{
		"Source code of https://github.com/owner/siikabot, commit 232d860b0e on the default branch.",
		`4 lines matching "remind" in the repository, in 2 files:`,
		"commands/remind/remind.go (https://github.com/owner/siikabot/blob/" + sourceTestCommit + "/commands/remind/remind.go)\n" +
			"1: package remind\n3: // Handle handles the !remind command\n5: parseRemindTime()\n",
		"llmtools/reminder.go (https://github.com/owner/siikabot/blob/" + sourceTestCommit + "/llmtools/reminder.go)\n" +
			"2: var ReminderToolDefinition\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("search results lack %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "main.go (") {
		t.Errorf("a file without matches is in the results:\n%s", got)
	}
}

func TestSearchSourceWithinAPath(t *testing.T) {
	got := searchSource(remindSnapshot, regexp.MustCompile(`(?i)remind`), "remind", "llmtools")
	if !strings.Contains(got, `1 line matching "remind" in llmtools, in 1 file:`) || strings.Contains(got, "commands/remind") {
		t.Errorf("a search limited to llmtools found:\n%s", got)
	}

	if got := searchSource(remindSnapshot, regexp.MustCompile(`(?i)nothing`), "nothing", ""); !strings.Contains(got, `No lines match "nothing" in the repository.`) {
		t.Errorf("a search without matches says:\n%s", got)
	}
	if got := searchSource(remindSnapshot, regexp.MustCompile(`x`), "x", "commands/reminder"); !strings.Contains(got, "There's no file or directory commands/reminder") {
		t.Errorf("a search of a path that doesn't exist says:\n%s", got)
	}
}

// Once maxSourceSearchLines lines are shown, the other files are only listed with their match counts
func TestSearchSourceShowsAFewLines(t *testing.T) {
	var files []sourcecode.File
	for i := range maxSourceSearchFiles + 10 {
		files = append(files, sourcecode.File{Path: fmt.Sprintf("pkg%03d/file.go", i), Lines: numberedLines(7)})
	}

	got := searchSource(sourceSnapshot(files...), regexp.MustCompile(`line`), "line", "")

	if shown := strings.Count(got, ": line "); shown != maxSourceSearchLines {
		t.Errorf("%d lines shown, want %d:\n%s", shown, maxSourceSearchLines, got)
	}
	for _, want := range []string{
		"770 lines matching \"line\" in the repository, in 110 files:",
		// 50 lines is seven files and one line of the eighth
		"pkg007/file.go (https://github.com/owner/siikabot/blob/" + sourceTestCommit + "/pkg007/file.go)\n1: line 1\n(6 more in this file)\n",
		"Other files with matches, not shown:\npkg008/file.go: 7 matches\n",
		"pkg107/file.go: 7 matches\n(and 2 more)\n",
		"limit the search to a path",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("capped search results lack %q:\n%s", want, got)
		}
	}
}

func TestSearchSourceCutsLongLines(t *testing.T) {
	long := strings.Repeat("ä", maxSourceSearchRunes+50)
	got := searchSource(sourceSnapshot(sourcecode.File{Path: "long.txt", Lines: []string{long}}), regexp.MustCompile(`ä`), "ä", "")
	if want := "1: " + strings.Repeat("ä", maxSourceSearchRunes) + "…\n"; !strings.Contains(got, want) {
		t.Errorf("a long line isn't cut at %d characters:\n%s", maxSourceSearchRunes, got)
	}
}

func TestReadSource(t *testing.T) {
	got, err := readSource(remindSnapshot, "commands/remind/remind.go", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := "Source code of https://github.com/owner/siikabot, commit 232d860b0e on the default branch.\n\n" +
		"commands/remind/remind.go, lines 1-6 of 6 (https://github.com/owner/siikabot/blob/" + sourceTestCommit + "/commands/remind/remind.go#L1-L6)\n\n" +
		"1: package remind\n2: \n3: // Handle handles the !remind command\n4: func Handle() {\n5: \tparseRemindTime()\n6: }\n"
	if got != want {
		t.Errorf("readSource() =\n%s\nwant\n%s", got, want)
	}

	got, err = readSource(remindSnapshot, "commands/remind/remind.go", 3, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "lines 3-4 of 6 (") || !strings.Contains(got, "#L3-L4)") || !strings.HasSuffix(got, "3: // Handle handles the !remind command\n4: func Handle() {\n") {
		t.Errorf("reading lines 3-4 gave:\n%s", got)
	}
	if strings.Contains(got, "read on") {
		t.Errorf("a read that showed every line asked for says to read on:\n%s", got)
	}

	if _, err := readSource(remindSnapshot, "main.go", 5, 0); err == nil || !strings.Contains(err.Error(), "main.go has only 1 line") {
		t.Errorf("reading past the end of a file gave %v", err)
	}
}

// A long file is read maxSourceReadLines lines at a time, and each part says where the next one starts
func TestReadSourceInParts(t *testing.T) {
	snapshot := sourceSnapshot(sourcecode.File{Path: "big.go", Lines: numberedLines(450)})

	got, err := readSource(snapshot, "big.go", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "big.go, lines 1-200 of 450") || !strings.Contains(got, "200: line 200\n") || strings.Contains(got, "201: ") {
		t.Errorf("the first part of a long file isn't its first 200 lines:\n%s", got[:200])
	}
	if !strings.HasSuffix(got, "250 lines more: read on with start_line 201.") {
		t.Errorf("the first part doesn't say where to read on:\n%s", got[len(got)-100:])
	}

	// Asking for more than a part holds still gets one part
	got, _ = readSource(snapshot, "big.go", 301, 1000)
	if !strings.Contains(got, "lines 301-450 of 450") || strings.Contains(got, "read on") {
		t.Errorf("the last part of a long file:\n%s", got[:200])
	}
}

// Long lines are cut, and a read stops once it holds maxSourceReadBytes
func TestReadSourceStaysSmall(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = strings.Repeat("x", maxSourceReadRunes+100)
	}
	got, err := readSource(sourceSnapshot(sourcecode.File{Path: "wide.txt", Lines: lines}), "wide.txt", 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, strings.Repeat("x", maxSourceReadRunes+1)) {
		t.Error("a long line isn't cut")
	}
	if len(got) > maxSourceReadBytes+1000 {
		t.Errorf("a read holds %d bytes, want about %d at most", len(got), maxSourceReadBytes)
	}
	if !strings.Contains(got, "wide.txt, lines 1-24 of 100") || !strings.HasSuffix(got, "76 lines more: read on with start_line 25.") {
		t.Errorf("a read cut short by its size:\n%s", got[len(got)-200:])
	}
}

func TestReadSourceOfSomethingElse(t *testing.T) {
	// A directory is listed
	got, err := readSource(remindSnapshot, "commands/remind", 0, 0)
	if err != nil || !strings.Contains(got, "1 file in commands/remind (") {
		t.Errorf("reading a directory gave %v:\n%s", err, got)
	}

	// A path that isn't there gets suggestions
	got, err = readSource(remindSnapshot, "commands/reminder.go", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "There's no file or directory commands/reminder.go in the repository. Paths with \"reminder.go\" in them:\nllmtools/reminder.go") {
		t.Errorf("reading a file that doesn't exist gave:\n%s", got)
	}

	got, _ = readSource(sourceSnapshot(sourcecode.File{Path: "empty.txt"}), "empty.txt", 0, 0)
	if !strings.HasSuffix(got, "empty.txt is empty.") {
		t.Errorf("reading an empty file gave:\n%s", got)
	}
}

func TestListSource(t *testing.T) {
	got := listSource(remindSnapshot, "")
	want := "Source code of https://github.com/owner/siikabot, commit 232d860b0e on the default branch.\n\n" +
		"3 files in the repository (https://github.com/owner/siikabot/tree/" + sourceTestCommit + "):\n" +
		"commands/remind/remind.go (6 lines)\nllmtools/reminder.go (2 lines)\nmain.go (1 line)\n"
	if got != want {
		t.Errorf("listSource() =\n%s\nwant\n%s", got, want)
	}

	if got := listSource(remindSnapshot, "main.go"); !strings.HasSuffix(got, "main.go is a file of 1 line (https://github.com/owner/siikabot/blob/"+sourceTestCommit+"/main.go).") {
		t.Errorf("listing a file gave:\n%s", got)
	}
	if got := listSource(remindSnapshot, "nowhere"); !strings.Contains(got, "There's no file or directory nowhere in the repository. Search or list") {
		t.Errorf("listing a path that doesn't exist gave:\n%s", got)
	}
}

// A directory with too many files to list is listed by its subdirectories
func TestListSourceOfALargeDirectory(t *testing.T) {
	files := []sourcecode.File{{Path: "CLAUDE.md", Lines: numberedLines(3)}}
	for i := range maxSourceListedFiles {
		files = append(files, sourcecode.File{Path: fmt.Sprintf("commands/cmd%03d/cmd.go", i)})
	}
	files = append(files,
		sourcecode.File{Path: "go.mod", Lines: numberedLines(2)},
		sourcecode.File{Path: "llmtools/a.go"},
		sourcecode.File{Path: "llmtools/b.go"},
	)

	got := listSource(sourceSnapshot(files...), "")
	want := "104 files in the repository (https://github.com/owner/siikabot/tree/" + sourceTestCommit + "):\n" +
		"CLAUDE.md (3 lines)\ncommands/ (100 files)\ngo.mod (2 lines)\nllmtools/ (2 files)\nList a directory to see its files.\n"
	if !strings.HasSuffix(got, want) {
		t.Errorf("listSource() of a large directory =\n%s\nwant it to end with\n%s", got, want)
	}

	got = listSource(sourceSnapshot(files...), "commands")
	if !strings.Contains(got, "100 files in commands (") || !strings.Contains(got, "commands/cmd042/cmd.go (0 lines)\n") {
		t.Errorf("a directory of exactly %d files isn't listed file by file:\n%s", maxSourceListedFiles, got[:300])
	}
}

// Arguments are checked before anything is downloaded, so these fail without a network
func TestSourceCodeToolRejectsBadArguments(t *testing.T) {
	for arguments, want := range map[string]string{
		`{"action": "delete"}`:                    `unknown action "delete"`,
		`{"action": "search"}`:                    "query is required",
		`{"action": "search", "query": "func ("}`: "isn't a valid regular expression",
		`{"action": "read"}`:                      "path is required",
		`{"action": "read", "path": "/"}`:         "path is required",
		`{"action": "read", "path": "main.go", "start_line": 10, "end_line": 5}`: "end_line is before start_line",
		`not json`: "failed to parse arguments",
	} {
		_, err := handleSourceCodeToolCall(context.Background(), arguments)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s gave %v, want an error saying %q", arguments, err, want)
		}
	}
}

// Only a repository on GitHub can be read. Elsewhere the model is told to link to it instead.
func TestSourceCodeToolOutsideGitHub(t *testing.T) {
	original := config.SourceURL
	config.SourceURL = "https://git.example.org/siikabot"
	defer func() { config.SourceURL = original }()

	_, err := handleSourceCodeToolCall(context.Background(), `{"action": "list"}`)
	if err == nil || !strings.Contains(err.Error(), "https://git.example.org/siikabot, so link to it instead") {
		t.Errorf("a repository outside GitHub gave %v", err)
	}
}
