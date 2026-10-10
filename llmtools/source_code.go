package llmtools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/sourcecode"
	"github.com/rs/zerolog/log"
)

// The actions of the source code tool
const (
	sourceActionSearch = "search"
	sourceActionRead   = "read"
	sourceActionList   = "list"
)

// Limits on how much one call of the source code tool returns, which keep it to a few thousand tokens. A
// search narrows down where to look, and a read then shows a function, or a small file whole.
const (
	// maxSourceSearchLines is how many matching lines a search shows. The files with more matches are listed
	// after them, up to maxSourceSearchFiles.
	maxSourceSearchLines = 50
	maxSourceSearchFiles = 100
	// maxSourceSearchRunes is where a matching line is cut
	maxSourceSearchRunes = 200
	// A read shows at most maxSourceReadLines lines and maxSourceReadBytes bytes, cutting each line at
	// maxSourceReadRunes
	maxSourceReadLines = 200
	maxSourceReadBytes = 12 << 10
	maxSourceReadRunes = 500
	// maxSourceListedFiles is how many files a directory can have and still be listed file by file. A larger
	// one is listed by its subdirectories.
	maxSourceListedFiles = 100
	// maxSimilarPaths is how many paths are suggested in place of one that isn't in the repository
	maxSimilarPaths = 10
)

// SourceCodeToolDefinition is the tool for the bot's own source code
var SourceCodeToolDefinition = aigateway.ToolDefinition{
	Type: "function",
	Function: aigateway.FunctionSchema{
		Name: "get_own_source_code",
		Description: "Search and read your own source code: the code of this bot, from the default branch of its public " +
			"repository. Use it when someone asks how one of your features works or where something is implemented. " +
			"search: the lines that match a regular expression, by file. read: a file with line numbers, 200 lines at a " +
			"time. list: the files in a directory, or in the whole repository. " +
			"Results link to the files on GitHub: link to a line by adding #L<line> to a file's link, or to several " +
			"lines with #L<first>-L<last>.",
		Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"action": {
						"type": "string",
						"enum": ["search", "read", "list"],
						"description": "What to do"
					},
					"query": {
						"type": "string",
						"description": "search: a regular expression in RE2 syntax, matched against each line, ignoring case"
					},
					"path": {
						"type": "string",
						"description": "A file or directory, from the root of the repository, such as commands/remind or commands/remind/remind.go. read: the file to read (required). search and list: limits them to this directory or file (default: the whole repository)."
					},
					"start_line": {
						"type": "integer",
						"description": "read: the first line to show (default 1)"
					},
					"end_line": {
						"type": "integer",
						"description": "read: the last line to show (default: as many as fit, up to 200 lines)"
					}
				},
				"required": ["action"]
			}`),
	},
	Handler:          handleSourceCodeToolCall,
	ValidityDuration: 30 * time.Minute,
	// The bot's own code, from the repository it's configured with, so none of it comes from a stranger
	Trusted: true,
}

func handleSourceCodeToolCall(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Action    string `json:"action"`
		Query     string `json:"query"`
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}

	log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received source code tool call")

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("arguments", arguments).Msg("Failed to parse tool arguments")
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	// The arguments are checked before anything is downloaded
	action := strings.ToLower(strings.TrimSpace(args.Action))
	filePath := cleanSourcePath(args.Path)
	var query *regexp.Regexp
	switch action {
	case sourceActionSearch:
		if strings.TrimSpace(args.Query) == "" {
			return "", errors.New("query is required for search")
		}
		var err error
		if query, err = regexp.Compile("(?i)" + args.Query); err != nil {
			return "", fmt.Errorf("query isn't a valid regular expression: %w", err)
		}
	case sourceActionRead:
		if filePath == "" {
			return "", errors.New("path is required for read")
		}
		if args.EndLine > 0 && args.EndLine < args.StartLine {
			return "", errors.New("end_line is before start_line")
		}
	case sourceActionList:
	default:
		return "", fmt.Errorf("unknown action %q, use one of: search, read, list", args.Action)
	}

	repo, err := sourcecode.ParseRepository(config.SourceURL)
	if err != nil {
		return "", fmt.Errorf("the source code can only be looked up in a GitHub repository, and yours is at %s, so link to it instead", config.SourceURL)
	}
	snapshot, err := sourcecode.Get(ctx, repo)
	if err != nil {
		// Already logged
		return "", fmt.Errorf("failed to download the source code: %w", err)
	}

	switch action {
	case sourceActionSearch:
		return searchSource(snapshot, query, args.Query, filePath), nil
	case sourceActionRead:
		return readSource(snapshot, filePath, args.StartLine, args.EndLine)
	default:
		return listSource(snapshot, filePath), nil
	}
}

// cleanSourcePath turns a path the model gives into one from the root of the repository, which is ""
func cleanSourcePath(filePath string) string {
	return strings.TrimPrefix(path.Clean("/"+strings.TrimSpace(filePath)), "/")
}

// sourceHeader says which version of the code a result shows
func sourceHeader(snapshot *sourcecode.Snapshot) string {
	version := "the latest commit"
	if snapshot.Commit != "" {
		version = "commit " + snapshot.Commit[:min(10, len(snapshot.Commit))]
	}
	return "Source code of " + snapshot.Repository.URL() + ", " + version + " on the default branch."
}

// searchSource shows the lines that match a search, by file. Once maxSourceSearchLines lines are shown, the
// other files with matches are only listed, with how many matches each has.
func searchSource(snapshot *sourcecode.Snapshot, query *regexp.Regexp, queryText, dir string) string {
	files := snapshot.Under(dir)
	if len(files) == 0 {
		return missingSourcePath(snapshot, dir)
	}

	where := "the repository"
	if dir != "" {
		where = dir
	}
	matches := snapshot.Search(query, dir)
	if len(matches) == 0 {
		return fmt.Sprintf("%s\n\nNo lines match \"%s\" in %s.", sourceHeader(snapshot), queryText, where)
	}

	// The matches come grouped by file already, in the order of the files
	type fileMatches struct {
		path    string
		matches []sourcecode.Match
	}
	var byFile []fileMatches
	for _, match := range matches {
		if len(byFile) == 0 || byFile[len(byFile)-1].path != match.Path {
			byFile = append(byFile, fileMatches{path: match.Path})
		}
		last := &byFile[len(byFile)-1]
		last.matches = append(last.matches, match)
	}

	var sb strings.Builder
	sb.WriteString(sourceHeader(snapshot) + "\n\n")
	fmt.Fprintf(&sb, "%s matching \"%s\" in %s, in %s:\n",
		countOf(len(matches), "line", "lines"), queryText, where, countOf(len(byFile), "file", "files"))

	shown := 0
	var rest []fileMatches
	for _, file := range byFile {
		if shown == maxSourceSearchLines {
			rest = append(rest, file)
			continue
		}
		fmt.Fprintf(&sb, "\n%s (%s)\n", file.path, snapshot.Repository.FileURL(snapshot.Ref(), file.path))
		for i, match := range file.matches {
			if shown == maxSourceSearchLines {
				fmt.Fprintf(&sb, "(%d more in this file)\n", len(file.matches)-i)
				break
			}
			fmt.Fprintf(&sb, "%d: %s\n", match.Line, cutLine(strings.TrimSpace(match.Text), maxSourceSearchRunes))
			shown++
		}
	}

	if len(rest) > 0 {
		sb.WriteString("\nOther files with matches, not shown:\n")
		for i, file := range rest {
			if i == maxSourceSearchFiles {
				fmt.Fprintf(&sb, "(and %d more)\n", len(rest)-i)
				break
			}
			fmt.Fprintf(&sb, "%s: %s\n", file.path, countOf(len(file.matches), "match", "matches"))
		}
		sb.WriteString("To see their lines, limit the search to a path or make the query more specific.\n")
	}
	return sb.String()
}

// readSource shows lines of a file with their numbers, from startLine to endLine, or as many as fit in a
// read. Reading a directory lists it instead.
func readSource(snapshot *sourcecode.Snapshot, filePath string, startLine, endLine int) (string, error) {
	file, ok := snapshot.File(filePath)
	if !ok {
		return listSource(snapshot, filePath), nil
	}

	total := len(file.Lines)
	if total == 0 {
		return fmt.Sprintf("%s\n\n%s is empty.", sourceHeader(snapshot), filePath), nil
	}
	start := max(startLine, 1)
	if start > total {
		return "", fmt.Errorf("%s has only %s", filePath, countOf(total, "line", "lines"))
	}
	end := start + maxSourceReadLines - 1
	if endLine > 0 {
		end = min(end, endLine)
	}
	end = min(end, total)

	var body strings.Builder
	last := start - 1
	for i := start; i <= end; i++ {
		line := fmt.Sprintf("%d: %s\n", i, cutLine(file.Lines[i-1], maxSourceReadRunes))
		if i > start && body.Len()+len(line) > maxSourceReadBytes {
			break
		}
		body.WriteString(line)
		last = i
	}

	var sb strings.Builder
	sb.WriteString(sourceHeader(snapshot) + "\n\n")
	fmt.Fprintf(&sb, "%s, lines %d-%d of %d (%s#L%d-L%d)\n\n",
		filePath, start, last, total, snapshot.Repository.FileURL(snapshot.Ref(), filePath), start, last)
	sb.WriteString(body.String())
	// Said only when the read stopped short of where it was asked to
	if last < total && (endLine <= 0 || last < endLine) {
		fmt.Fprintf(&sb, "\n%s more: read on with start_line %d.", countOf(total-last, "line", "lines"), last+1)
	}
	return sb.String(), nil
}

// listSource lists the files in a directory and the directories below it, with how many lines each has. A
// directory with more than maxSourceListedFiles files is listed by its subdirectories instead.
func listSource(snapshot *sourcecode.Snapshot, dir string) string {
	files := snapshot.Under(dir)
	if len(files) == 0 {
		return missingSourcePath(snapshot, dir)
	}
	if len(files) == 1 && files[0].Path == dir {
		return fmt.Sprintf("%s\n\n%s is a file of %s (%s).", sourceHeader(snapshot), dir,
			countOf(len(files[0].Lines), "line", "lines"), snapshot.Repository.FileURL(snapshot.Ref(), dir))
	}

	where := "the repository"
	if dir != "" {
		where = dir
	}
	var sb strings.Builder
	sb.WriteString(sourceHeader(snapshot) + "\n\n")
	fmt.Fprintf(&sb, "%s in %s (%s):\n", countOf(len(files), "file", "files"), where,
		snapshot.Repository.DirectoryURL(snapshot.Ref(), dir))

	if len(files) <= maxSourceListedFiles {
		for _, file := range files {
			fmt.Fprintf(&sb, "%s (%s)\n", file.Path, countOf(len(file.Lines), "line", "lines"))
		}
		return sb.String()
	}

	// Too many to list one by one: the subdirectories with how many files each has, and the files that are in
	// the directory itself. The files come sorted by path, so each subdirectory's files come together.
	prefix := ""
	if dir != "" {
		prefix = dir + "/"
	}
	var subdirectory string
	var fileCount int
	flush := func() {
		if subdirectory != "" {
			fmt.Fprintf(&sb, "%s/ (%s)\n", prefix+subdirectory, countOf(fileCount, "file", "files"))
		}
		subdirectory, fileCount = "", 0
	}
	for _, file := range files {
		name, _, inSubdirectory := strings.Cut(strings.TrimPrefix(file.Path, prefix), "/")
		if !inSubdirectory {
			flush()
			fmt.Fprintf(&sb, "%s (%s)\n", file.Path, countOf(len(file.Lines), "line", "lines"))
			continue
		}
		if name != subdirectory {
			flush()
			subdirectory = name
		}
		fileCount++
	}
	flush()
	sb.WriteString("List a directory to see its files.\n")
	return sb.String()
}

// missingSourcePath explains that a path isn't in the repository, suggesting paths with its last part in them
func missingSourcePath(snapshot *sourcecode.Snapshot, missing string) string {
	text := fmt.Sprintf("%s\n\nThere's no file or directory %s in the repository.", sourceHeader(snapshot), missing)

	name := strings.ToLower(path.Base(missing))
	var similar []string
	for _, file := range snapshot.Files {
		if strings.Contains(strings.ToLower(file.Path), name) {
			similar = append(similar, file.Path)
		}
	}
	switch {
	case len(similar) == 0:
		return text + " Search or list to find where things are."
	case len(similar) > maxSimilarPaths:
		return fmt.Sprintf("%s Paths with %q in them include:\n%s\n(and %d more)", text, name,
			strings.Join(similar[:maxSimilarPaths], "\n"), len(similar)-maxSimilarPaths)
	default:
		return fmt.Sprintf("%s Paths with %q in them:\n%s", text, name, strings.Join(similar, "\n"))
	}
}

// cutLine shortens a line to at most maxRunes characters, marking where it was cut
func cutLine(line string, maxRunes int) string {
	runes := []rune(line)
	if len(runes) <= maxRunes {
		return line
	}
	return string(runes[:maxRunes]) + "…"
}
