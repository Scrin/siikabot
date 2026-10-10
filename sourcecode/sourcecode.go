// Package sourcecode reads the bot's own source code, so the chat model can look up how the bot works and
// link to the code. The code comes from the bot's public repository on GitHub: the default branch is
// downloaded as one archive, kept in memory for a while, and searched and read from there.
package sourcecode

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Scrin/siikabot/metrics"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	// maxAge is how long a download is used before a lookup downloads the branch again
	maxAge = 15 * time.Minute
	// downloadTimeout bounds a download, redirects included
	downloadTimeout = 20 * time.Second
	// maxArchiveBytes caps the download, and maxUnpackedBytes what it unpacks to. The bot's repository is
	// well under a megabyte either way.
	maxArchiveBytes  = 50 << 20
	maxUnpackedBytes = 200 << 20
	// maxFileBytes is the largest file kept. Anything larger is data or generated, not code to read.
	maxFileBytes = 1 << 20
)

// githubURL is where repositories are downloaded from. Tests point it at a server of their own.
var githubURL = "https://github.com"

var client = &http.Client{
	Timeout:   downloadTimeout,
	Transport: otelhttp.NewTransport(http.DefaultTransport),
}

// Repository is a repository on GitHub
type Repository struct {
	Owner string
	Name  string
}

// repositoryPart is what an owner or a repository name is made of
var repositoryPart = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9_.-]*$`)

// ParseRepository reads a repository from its address, such as https://github.com/owner/name
func ParseRepository(rawURL string) (Repository, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return Repository{}, fmt.Errorf("%q isn't a valid URL", rawURL)
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if (u.Scheme != "https" && u.Scheme != "http") || host != "github.com" {
		return Repository{}, fmt.Errorf("%s isn't on GitHub", rawURL)
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 2 {
		repo := Repository{Owner: parts[0], Name: strings.TrimSuffix(parts[1], ".git")}
		if repositoryPart.MatchString(repo.Owner) && repositoryPart.MatchString(repo.Name) {
			return repo, nil
		}
	}
	return Repository{}, fmt.Errorf("%s isn't the address of a GitHub repository, such as https://github.com/owner/name", rawURL)
}

// URL is the repository's page on GitHub
func (r Repository) URL() string {
	return "https://github.com/" + r.Owner + "/" + r.Name
}

// FileURL links to a file as it is at ref, a commit or a branch
func (r Repository) FileURL(ref, filePath string) string {
	return r.URL() + "/blob/" + ref + "/" + escapePath(filePath)
}

// DirectoryURL links to a directory as it is at ref, a commit or a branch. An empty directory is the root of
// the repository.
func (r Repository) DirectoryURL(ref, dir string) string {
	if dir == "" {
		return r.URL() + "/tree/" + ref
	}
	return r.URL() + "/tree/" + ref + "/" + escapePath(dir)
}

// escapePath escapes each part of a path for a URL, keeping the slashes between them
func escapePath(filePath string) string {
	parts := strings.Split(filePath, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

// Snapshot is the files of a repository at one commit
type Snapshot struct {
	Repository Repository
	// Commit is the commit the files are from, or "" when the archive didn't say
	Commit string
	// Files are the repository's text files, sorted by path
	Files []File
}

// File is a text file in a repository
type File struct {
	// Path is the file's path from the root of the repository, such as commands/remind/remind.go
	Path  string
	Lines []string
}

// Ref is what links to the snapshot's files point at: its commit, or the default branch when the commit
// isn't known
func (s *Snapshot) Ref() string {
	if s.Commit == "" {
		return "HEAD"
	}
	return s.Commit
}

// File finds a file by its path
func (s *Snapshot) File(filePath string) (File, bool) {
	i, found := slices.BinarySearchFunc(s.Files, filePath, func(file File, target string) int {
		return strings.Compare(file.Path, target)
	})
	if !found {
		return File{}, false
	}
	return s.Files[i], true
}

// Under returns the files in a directory and the directories below it, or the file itself when the path is
// a file's. An empty path is the whole repository.
func (s *Snapshot) Under(path string) []File {
	if path == "" {
		return s.Files
	}
	if file, ok := s.File(path); ok {
		return []File{file}
	}
	var files []File
	for _, file := range s.Files {
		if strings.HasPrefix(file.Path, path+"/") {
			files = append(files, file)
		}
	}
	return files
}

// Match is a line that matched a search
type Match struct {
	Path string
	// Line is the line's number, counting from 1
	Line int
	Text string
}

// Search finds the lines that match a regular expression, in the files under a path (see Under)
func (s *Snapshot) Search(re *regexp.Regexp, path string) []Match {
	var matches []Match
	for _, file := range s.Under(path) {
		for i, line := range file.Lines {
			if re.MatchString(line) {
				matches = append(matches, Match{Path: file.Path, Line: i + 1, Text: line})
			}
		}
	}
	return matches
}

// cache holds the latest download
var cache struct {
	sync.Mutex
	repository Repository
	snapshot   *Snapshot
	// tried is when a download was last tried, whether it worked or not
	tried time.Time
}

// Get returns the files on a repository's default branch. They're downloaded when there's no copy in memory
// yet, or it's older than maxAge. When a download fails, the previous copy is used for another maxAge before
// the next try, since code a little out of date is far better than none.
//
// A lookup waits for a download already under way rather than starting another, so the tool calls of one
// turn share a download.
func Get(ctx context.Context, repo Repository) (*Snapshot, error) {
	cache.Lock()
	defer cache.Unlock()

	if cache.repository != repo {
		cache.repository, cache.snapshot, cache.tried = repo, nil, time.Time{}
	}
	if cache.snapshot != nil && time.Since(cache.tried) < maxAge {
		return cache.snapshot, nil
	}

	cache.tried = time.Now()
	snapshot, err := download(ctx, repo)
	if err != nil {
		if cache.snapshot != nil {
			log.Warn().Ctx(ctx).Err(err).
				Str("owner", repo.Owner).
				Str("repository", repo.Name).
				Str("commit", cache.snapshot.Commit).
				Msg("Failed to download the source code, using the previous download")
			return cache.snapshot, nil
		}
		return nil, err
	}
	cache.snapshot = snapshot
	return snapshot, nil
}

// download downloads the files on a repository's default branch
func download(ctx context.Context, repo Repository) (*Snapshot, error) {
	ctx, span := tracer.Start(ctx, "sourcecode.download", trace.WithAttributes(
		attribute.String("siikabot.source_code.repository", repo.Owner+"/"+repo.Name),
	))
	defer span.End()

	start := time.Now()
	snapshot, archiveBytes, err := fetchArchive(ctx, repo)
	duration := time.Since(start)
	metrics.RecordSourceCodeDownload(err == nil, duration.Seconds())

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		log.Error().Ctx(ctx).Err(err).
			Str("owner", repo.Owner).
			Str("repository", repo.Name).
			Int("archive_bytes", archiveBytes).
			Float64("duration_sec", duration.Seconds()).
			Msg("Failed to download the source code")
		return nil, err
	}

	span.SetAttributes(
		attribute.String("siikabot.source_code.commit", snapshot.Commit),
		attribute.Int("siikabot.source_code.file_count", len(snapshot.Files)),
		attribute.Int("siikabot.source_code.archive_bytes", archiveBytes),
	)
	log.Info().Ctx(ctx).
		Str("owner", repo.Owner).
		Str("repository", repo.Name).
		Str("commit", snapshot.Commit).
		Int("file_count", len(snapshot.Files)).
		Int("archive_bytes", archiveBytes).
		Float64("duration_sec", duration.Seconds()).
		Msg("Downloaded the source code")
	return snapshot, nil
}

// fetchArchive downloads and unpacks the archive of a repository's default branch, returning how many bytes
// of it were read
func fetchArchive(ctx context.Context, repo Repository) (*Snapshot, int, error) {
	archiveURL := githubURL + "/" + repo.Owner + "/" + repo.Name + "/archive/HEAD.tar.gz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, archiveURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Siikabot (+"+repo.URL()+")")

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to download %s: %w", archiveURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("GitHub answered %s for %s", resp.Status, archiveURL)
	}

	archive, err := io.ReadAll(io.LimitReader(resp.Body, maxArchiveBytes+1))
	if err != nil {
		return nil, len(archive), fmt.Errorf("failed to download %s: %w", archiveURL, err)
	}
	if len(archive) > maxArchiveBytes {
		return nil, len(archive), fmt.Errorf("the archive is larger than %d bytes", maxArchiveBytes)
	}

	commit, files, err := unpack(bytes.NewReader(archive))
	if err != nil {
		return nil, len(archive), err
	}
	return &Snapshot{Repository: repo, Commit: commit, Files: files}, len(archive), nil
}

// unpack reads the text files out of a repository archive made by git archive, as GitHub's are. Every path
// in one starts with a directory named after the repository and the commit, which is dropped, and the
// archive's global header names the commit.
func unpack(r io.Reader) (commit string, files []File, err error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read the archive: %w", err)
	}
	defer gz.Close()

	archive := tar.NewReader(gz)
	var unpacked int64
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", nil, fmt.Errorf("failed to read the archive: %w", err)
		}

		switch header.Typeflag {
		case tar.TypeXGlobalHeader:
			commit = header.PAXRecords["comment"]
			continue
		case tar.TypeReg:
		default:
			// Directories, and symbolic links, whose targets are in the archive anyway
			continue
		}

		_, filePath, ok := strings.Cut(header.Name, "/")
		if !ok || filePath == "" || header.Size > maxFileBytes {
			continue
		}
		unpacked += header.Size
		if unpacked > maxUnpackedBytes {
			return "", nil, fmt.Errorf("the archive unpacks to more than %d bytes", maxUnpackedBytes)
		}
		content, err := io.ReadAll(archive)
		if err != nil {
			return "", nil, fmt.Errorf("failed to read %s from the archive: %w", filePath, err)
		}
		if isText(content) {
			files = append(files, File{Path: filePath, Lines: splitLines(string(content))})
		}
	}

	slices.SortFunc(files, func(a, b File) int {
		return strings.Compare(a.Path, b.Path)
	})
	return commit, files, nil
}

// isText reports whether a file is text, the only kind there is to read. Images and other binary files are
// left out.
func isText(content []byte) bool {
	return utf8.Valid(content) && bytes.IndexByte(content, 0) < 0
}

// splitLines splits a file into its lines, without their line endings
func splitLines(content string) []string {
	content = strings.TrimSuffix(content, "\n")
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSuffix(line, "\r")
	}
	return lines
}
