// Package webhooks holds the rules for the webhooks the admin sets up on the web dashboard, which
// post messages into a room the bot is in: what their names and tokens may be, how a token is
// checked, and how the message a request brings is read.
//
// The endpoint itself is in the bot package, next to the other hooks, and managing webhooks is in
// the api package.
package webhooks

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Scrin/siikabot/auth"
)

const (
	// MaxNameLength is how many characters a webhook's name may have
	MaxNameLength = 64
	// MinTokenLength and MaxTokenLength bound a token's length. A short token typed in by hand
	// could be guessed from its hash, which is a fast one.
	MinTokenLength = 16
	MaxTokenLength = 256
	// MaxBodyBytes is the most a request may bring. A message is sent both as written and as HTML,
	// which roughly doubles it, and this keeps that well inside Matrix's 64 KiB event limit even once
	// it is encrypted.
	MaxBodyBytes = 16 << 10
	// MaxContentBytes is the most a message's content may take once it is formatted. Markdown can
	// expand far more than usual, and encryption adds a third on top of this, which still has to fit
	// in Matrix's 64 KiB event limit.
	MaxContentBytes = 40 << 10
)

// idBytes is how many random bytes a webhook's ID is made of
const idBytes = 16

var (
	ErrEmptyName       = errors.New("the name can't be empty")
	ErrNameTooLong     = fmt.Errorf("the name can be at most %d characters", MaxNameLength)
	ErrNameNotOneLine  = errors.New("the name has to be a single line, without control characters")
	ErrTokenLength     = fmt.Errorf("the token has to be %d to %d characters", MinTokenLength, MaxTokenLength)
	ErrTokenCharacters = errors.New("the token can only have printable ASCII characters, without spaces")
	ErrMessageTooLarge = fmt.Errorf("the message can be at most %d KiB", MaxBodyBytes>>10)
	ErrEmptyMessage    = errors.New("the message is empty")
	ErrInvalidUTF8     = errors.New("the message isn't valid UTF-8 text")
	ErrInvalidJSON     = errors.New(`a JSON body has to be an object with the message as a string in "message"`)
)

// ValidateName checks a webhook's name, and returns it without surrounding whitespace
func ValidateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", ErrEmptyName
	case utf8.RuneCountInString(name) > MaxNameLength:
		return "", ErrNameTooLong
	case strings.IndexFunc(name, unicode.IsControl) >= 0:
		return "", ErrNameNotOneLine
	}
	return name, nil
}

// ValidateToken checks a webhook's token. It has to fit in an Authorization header as it is, so
// unlike a name it isn't trimmed.
func ValidateToken(token string) error {
	if len(token) < MinTokenLength || len(token) > MaxTokenLength {
		return ErrTokenLength
	}
	for i := 0; i < len(token); i++ {
		if token[i] < '!' || token[i] > '~' {
			return ErrTokenCharacters
		}
	}
	return nil
}

// NewID makes a random ID for a new webhook
func NewID() (string, error) {
	return auth.RandomString(idBytes)
}

// HashToken returns the hash a token is stored as
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// TokenMatches reports whether a token is the one whose hash is stored, in constant time
func TokenMatches(hash []byte, token string) bool {
	return subtle.ConstantTimeCompare(hash, HashToken(token)) == 1
}

// ParseMessage reads the message a request brings. A JSON body gives it in its "message" field, and
// any other body is the message itself. The message comes without surrounding whitespace, such as
// the newline a file sent with curl ends with.
func ParseMessage(contentType string, body []byte) (string, error) {
	if !utf8.Valid(body) {
		return "", ErrInvalidUTF8
	}

	message := string(body)
	if mediaType, _, _ := mime.ParseMediaType(contentType); mediaType == "application/json" {
		var payload struct {
			Message *string `json:"message"`
		}
		if err := json.Unmarshal(body, &payload); err != nil || payload.Message == nil {
			return "", ErrInvalidJSON
		}
		message = *payload.Message
	}

	message = strings.TrimSpace(message)
	if message == "" {
		return "", ErrEmptyMessage
	}
	return message, nil
}
