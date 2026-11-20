package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Scrin/siikabot/config"
	"github.com/rs/zerolog/log"
	"maunium.net/go/mautrix/crypto/attachment"
	"maunium.net/go/mautrix/id"
)

// MessageImageURL returns the URL of the image a message carries, or an error if it carries none.
// For an encrypted image it also returns the encryption info needed for decryption, and for any
// image the full content. The URL is always a valid Matrix content URI (see parseMediaURI).
func MessageImageURL(ctx context.Context, msg *Message) (string, map[string]any, map[string]any, error) {
	imageURL, encryptionInfo, fullContent, err := messageImageURL(ctx, msg)
	if err != nil {
		return "", nil, nil, err
	}
	if _, err := parseMediaURI(imageURL); err != nil {
		log.Warn().Ctx(ctx).
			Str("room_id", msg.RoomID).
			Str("event_id", msg.EventID).
			Str("sender", msg.Sender).
			Msg("Image message names a URL that isn't a Matrix content URI")
		return "", nil, nil, err
	}
	return imageURL, encryptionInfo, fullContent, nil
}

// messageImageURL finds the URL of the image a message carries, as MessageImageURL does, without
// checking it
func messageImageURL(ctx context.Context, msg *Message) (string, map[string]any, map[string]any, error) {
	roomID, eventID := msg.RoomID, msg.EventID

	// Check if the message is an image
	if msg.MsgType != "m.image" {
		return "", nil, nil, fmt.Errorf("event is not an image (msgtype: %s)", msg.MsgType)
	}

	// Get the full content for mimetype information
	fullContent := msg.content

	// For encrypted images, the URL and encryption info are in the file section
	if file, ok := fullContent["file"].(map[string]any); ok {
		// This is an encrypted file
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("event_id", eventID).
			Msg("Found encrypted file data")

		// Extract the URL from the encrypted file data
		if url, ok := file["url"].(string); ok {
			return url, file, fullContent, nil
		}

		// Log the file content for debugging
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("event_id", eventID).
			Interface("file", file).
			Msg("Encrypted file data does not contain URL")

		return "", nil, nil, fmt.Errorf("encrypted image URL not found")
	}

	// Check if there's a thumbnail available for encrypted images
	if info, ok := fullContent["info"].(map[string]any); ok {
		if thumbnailFile, ok := info["thumbnail_file"].(map[string]any); ok {
			// This is an encrypted thumbnail
			log.Debug().Ctx(ctx).
				Str("room_id", roomID).
				Str("event_id", eventID).
				Msg("Found encrypted thumbnail file data")

			// Extract the URL from the encrypted thumbnail file data
			if url, ok := thumbnailFile["url"].(string); ok {
				return url, thumbnailFile, fullContent, nil
			}

			// Log the thumbnail file content for debugging
			log.Debug().Ctx(ctx).
				Str("room_id", roomID).
				Str("event_id", eventID).
				Interface("thumbnail_file", thumbnailFile).
				Msg("Encrypted thumbnail file data does not contain URL")
		}
	}

	// For unencrypted images, the URL is directly in the content
	if url, ok := fullContent["url"].(string); ok {
		return url, nil, fullContent, nil
	}

	// Check if there's a thumbnail available for unencrypted images
	if info, ok := fullContent["info"].(map[string]any); ok {
		if thumbnailUrl, ok := info["thumbnail_url"].(string); ok {
			log.Debug().Ctx(ctx).
				Str("room_id", roomID).
				Str("event_id", eventID).
				Str("thumbnail_url", thumbnailUrl).
				Msg("Using thumbnail URL for unencrypted image")
			return thumbnailUrl, nil, fullContent, nil
		}
	}

	// Log the content for debugging
	log.Debug().Ctx(ctx).
		Str("room_id", roomID).
		Str("event_id", eventID).
		Interface("content", fullContent).
		Msg("Image content does not contain URL")

	return "", nil, nil, fmt.Errorf("image URL not found")
}

// maxMediaSize is the largest image that is downloaded, encrypted or not
const maxMediaSize = 5 * 1024 * 1024 // 5MB

// errNotContentURI is what parseMediaURI says about anything but a valid content URI
var errNotContentURI = errors.New("not a valid Matrix content URI")

// parseMediaURI reads a Matrix content URI, mxc://<server-name>/<media-id>. Nothing else is ever
// downloaded. Whoever sends a message chooses the URL in it, and the bot downloads media from its
// homeserver with its access token, so any other URL could send the request, and the token, wherever
// the sender liked. The server name and media ID are checked too, as they become part of the path.
func parseMediaURI(raw string) (id.ContentURI, error) {
	uri, err := id.ParseContentURI(raw)
	if err != nil || !uri.IsValid() {
		return id.ContentURI{}, errNotContentURI
	}
	return uri, nil
}

// mediaDownloadURLs are the bot's homeserver's URLs for a content URI, in the order they're tried
func mediaDownloadURLs(uri id.ContentURI) []string {
	base := strings.TrimSuffix(config.HomeserverURL, "/")
	path := uri.Homeserver + "/" + uri.FileID
	return []string{
		base + "/_matrix/media/v3/download/" + path,
		base + "/_matrix/media/r0/download/" + path,
		// Some homeservers serve media here too
		base + "/_matrix/media/download/" + path,
	}
}

// DownloadImage downloads an image from its content URI, decrypting it if it is encrypted, and
// returns it as it was uploaded. What kind of image it is, if it is one at all, is for the caller to
// find out. Only a valid content URI is downloaded, and only from the bot's own homeserver.
func DownloadImage(ctx context.Context, imageURL string, encryptionInfo map[string]any, fullContent map[string]any) ([]byte, error) {
	uri, err := parseMediaURI(imageURL)
	if err != nil {
		return nil, err
	}
	if encryptionInfo != nil {
		if err := checkDeclaredSize(encryptionInfo, fullContent); err != nil {
			return nil, err
		}
	}

	var lastError error
	for _, downloadURL := range mediaDownloadURLs(uri) {
		data, err := downloadMedia(ctx, downloadURL)
		if err != nil {
			log.Debug().Ctx(ctx).Err(err).Str("url", downloadURL).Msg("Failed to download media from an endpoint")
			lastError = err
			continue
		}
		if encryptionInfo == nil {
			return data, nil
		}
		if err := decryptAttachment(data, encryptionInfo); err != nil {
			log.Error().Ctx(ctx).Err(err).Int("size_bytes", len(data)).Msg("Failed to decrypt an encrypted file")
			return nil, err
		}
		return data, nil
	}
	return nil, fmt.Errorf("all download attempts failed, last error: %w", lastError)
}

// checkDeclaredSize refuses an encrypted file that its event says is too large, before any of it is
// downloaded
func checkDeclaredSize(encryptionInfo, fullContent map[string]any) error {
	sizes := []any{encryptionInfo["size"]}
	if info, ok := fullContent["info"].(map[string]any); ok {
		sizes = append(sizes, info["size"])
	}
	for _, declared := range sizes {
		if size, ok := declared.(float64); ok && size > maxMediaSize {
			return fmt.Errorf("encrypted media is too large: %.0f bytes (max: 5MB)", size)
		}
	}
	return nil
}

// downloadMedia downloads media from one of the homeserver's URLs for it, reading at most
// maxMediaSize bytes
func downloadMedia(ctx context.Context, downloadURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	if client != nil && client.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+client.AccessToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download media: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download media: status code %d", resp.StatusCode)
	}
	if resp.ContentLength > maxMediaSize {
		return nil, fmt.Errorf("media is too large: %d bytes (max: 5MB)", resp.ContentLength)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxMediaSize+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read media: %w", err)
	}
	if len(data) > maxMediaSize {
		return nil, fmt.Errorf("media is too large: %d bytes (max: 5MB)", len(data))
	}
	return data, nil
}

// decryptAttachment decrypts an encrypted file in place, given the file object of the event that
// carries it. The Matrix spec fixes how the object is encoded: the AES-CTR key is a JSON Web Key in
// unpadded base64url, and the IV and the SHA-256 hash of the encrypted file are unpadded standard
// base64. The hash is checked before anything is decrypted, so a damaged download, or a file other
// than the one the event names, is an error rather than bytes that look like an unreadable image.
//
// Padding is the only leeway given. Any other way of reading the values gives a different key or IV,
// and decrypting with those turns the whole file into noise.
func decryptAttachment(data []byte, fileInfo map[string]any) error {
	if fileInfo == nil {
		return fmt.Errorf("encryption info is nil")
	}
	raw, err := json.Marshal(fileInfo)
	if err != nil {
		return fmt.Errorf("failed to read the encryption info: %w", err)
	}
	var file attachment.EncryptedFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return fmt.Errorf("failed to read the encryption info: %w", err)
	}

	// A client that pads the values still means the same bytes
	file.Key.Key = strings.TrimRight(file.Key.Key, "=")
	file.InitVector = strings.TrimRight(file.InitVector, "=")
	file.Hashes.SHA256 = strings.TrimRight(file.Hashes.SHA256, "=")

	if err := file.DecryptInPlace(data); err != nil {
		return fmt.Errorf("failed to decrypt the file: %w", err)
	}
	return nil
}
