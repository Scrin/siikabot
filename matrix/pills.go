package matrix

import (
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gomarkdown/markdown/ast"
)

// A pill is how a formatted message mentions a user: a link to the user on matrix.to, which
// clients show as the user's name.

// pillLink matches the user a pill in a formatted body links to, with the @ written out or
// percent-encoded
var pillLink = regexp.MustCompile(`https://matrix\.to/#/((?:@|%40)[^"'<>\s?]+)`)

// PillUserIDs returns the users pilled in a formatted body, in order
func PillUserIDs(formattedBody string) []string {
	var userIDs []string
	for _, match := range pillLink.FindAllStringSubmatch(formattedBody, -1) {
		// Some clients percent-encode the user ID in the link
		if userID, err := url.PathUnescape(match[1]); err == nil {
			userIDs = append(userIDs, userID)
		}
	}
	return userIDs
}

// userIDInText matches a user ID written out in text: the localpart, then the server name, which
// ends in a letter or a digit so that punctuation after the ID isn't taken for part of it
var userIDInText = regexp.MustCompile(`@[A-Za-z0-9._=\-/+]+:` +
	`(?:[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?\.)*[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?(?::[0-9]{1,5})?`)

// insertPills turns the user IDs written in the text of a parsed markdown document into pills, for
// the users in names, each showing the name given for them. Text in code or in a link is left as
// it is.
func insertPills(doc ast.Node, names map[string]string) {
	var texts []*ast.Text
	ast.WalkFunc(doc, func(node ast.Node, entering bool) ast.WalkStatus {
		switch node := node.(type) {
		case *ast.Link, *ast.Image:
			return ast.SkipChildren
		case *ast.Text:
			texts = append(texts, node)
		}
		return ast.GoToNext
	})

	for _, text := range texts {
		nodes := withPills(text.Literal, names)
		if nodes == nil {
			continue
		}
		parent := text.GetParent()
		children := parent.GetChildren()
		i := slices.Index(children, ast.Node(text))
		for _, node := range nodes {
			node.SetParent(parent)
		}
		parent.SetChildren(slices.Concat(children[:i], nodes, children[i+1:]))
	}
}

// withPills splits a text into the nodes that show it with pills, or returns nil if it names none
// of the users
func withPills(text []byte, names map[string]string) []ast.Node {
	var nodes []ast.Node
	rest := 0
	for _, match := range userIDInText.FindAllIndex(text, -1) {
		start, end := match[0], match[1]
		name, ok := names[string(text[start:end])]
		// Something glued to either end makes it part of a longer token, such as a link or an
		// email address
		if !ok || continuesToken(lastRune(text[:start]), "/#@") || continuesToken(firstRune(text[end:]), "/@") {
			continue
		}

		pill := &ast.Link{Destination: []byte("https://matrix.to/#/" + string(text[start:end]))}
		ast.AppendChild(pill, &ast.Text{Leaf: ast.Leaf{Literal: []byte(name)}})
		nodes = append(nodes, &ast.Text{Leaf: ast.Leaf{Literal: text[rest:start]}}, pill)
		rest = end
	}
	if nodes == nil {
		return nil
	}
	return append(nodes, &ast.Text{Leaf: ast.Leaf{Literal: text[rest:]}})
}

// continuesToken reports whether a character next to a user ID makes it part of a longer token:
// a letter, a digit, or one of the joiners
func continuesToken(r rune, joiners string) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(joiners, r)
}

// lastRune returns the last character of a text, or utf8.RuneError if it has none
func lastRune(text []byte) rune {
	r, _ := utf8.DecodeLastRune(text)
	return r
}

// firstRune returns the first character of a text, or utf8.RuneError if it has none
func firstRune(text []byte) rune {
	r, _ := utf8.DecodeRune(text)
	return r
}
