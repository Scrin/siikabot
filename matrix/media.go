package matrix

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Scrin/siikabot/config"
	"github.com/rs/zerolog/log"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// GetEventImageURL retrieves the URL of an image from an event.
// Returns the URL of the image as a string, or an empty string if the event is not an image.
// For encrypted images, it also returns the encryption info needed for decryption and the full content.
func GetEventImageURL(ctx context.Context, roomID string, eventID string) (string, map[string]interface{}, map[string]interface{}, error) {
	// Get the event from the server
	evt, err := client.GetEvent(ctx, id.RoomID(roomID), id.EventID(eventID))
	if err != nil {
		return "", nil, nil, err
	}

	// Check if the event is encrypted and decrypt it if necessary
	if evt.Type == event.EventEncrypted {
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("event_id", eventID).
			Msg("Attempting to decrypt event")

		err = evt.Content.ParseRaw(evt.Type)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Str("event_id", eventID).
				Msg("Failed to parse encrypted event content")
		}

		// Try to decrypt the event using OlmMachine
		decryptedEvt, err := olmMachine.DecryptMegolmEvent(ctx, evt)
		if err != nil {
			// If we can't decrypt it, log the error and return a specific error
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Str("event_id", eventID).
				Msg("Failed to decrypt event")
			return "", nil, nil, fmt.Errorf("cannot decrypt encrypted event: %w", err)
		}

		// Use the decrypted event
		evt = decryptedEvt
	}

	// Check if the event is a message
	if evt.Type != event.EventMessage {
		return "", nil, nil, fmt.Errorf("event is not a message (type: %s)", evt.Type)
	}

	// Check if the message is an image
	msgtype, ok := evt.Content.Raw["msgtype"].(string)
	if !ok || msgtype != "m.image" {
		return "", nil, nil, fmt.Errorf("event is not an image (msgtype: %s)", msgtype)
	}

	// Get the full content for mimetype information
	fullContent := evt.Content.Raw

	// For encrypted images, the URL and encryption info are in the file section
	if file, ok := evt.Content.Raw["file"].(map[string]interface{}); ok {
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
	if info, ok := evt.Content.Raw["info"].(map[string]interface{}); ok {
		if thumbnailFile, ok := info["thumbnail_file"].(map[string]interface{}); ok {
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
	if url, ok := evt.Content.Raw["url"].(string); ok {
		return url, nil, fullContent, nil
	}

	// Check if there's a thumbnail available for unencrypted images
	if info, ok := evt.Content.Raw["info"].(map[string]interface{}); ok {
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
		Interface("content", evt.Content.Raw).
		Msg("Image content does not contain URL")

	return "", nil, nil, fmt.Errorf("image URL not found")
}

// GetEventType retrieves the type of an event.
// Returns the msgtype of the event as a string, or an empty string if the event doesn't have a msgtype.
func GetEventType(ctx context.Context, roomID string, eventID string) (string, error) {
	// Get the event from the server
	evt, err := client.GetEvent(ctx, id.RoomID(roomID), id.EventID(eventID))
	if err != nil {
		return "", err
	}

	// Check if the event is encrypted and decrypt it if necessary
	if evt.Type == event.EventEncrypted {
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("event_id", eventID).
			Msg("Attempting to decrypt event")

		err = evt.Content.ParseRaw(evt.Type)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Str("event_id", eventID).
				Msg("Failed to parse encrypted event content")
		}

		// Try to decrypt the event using OlmMachine
		decryptedEvt, err := olmMachine.DecryptMegolmEvent(ctx, evt)
		if err != nil {
			// If we can't decrypt it, log the error and return a specific error
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Str("event_id", eventID).
				Msg("Failed to decrypt event")
			return "", fmt.Errorf("cannot decrypt encrypted event: %w", err)
		}

		// Use the decrypted event
		evt = decryptedEvt
	}

	// Check if the event is a message
	if evt.Type != event.EventMessage {
		return "", fmt.Errorf("event is not a message (type: %s)", evt.Type)
	}

	// Extract the message type
	if msgtype, ok := evt.Content.Raw["msgtype"].(string); ok {
		return msgtype, nil
	}

	return "", fmt.Errorf("message type not found")
}

// DownloadImageAsBase64 downloads an image from a Matrix URL and returns it as a base64 data URL.
func DownloadImageAsBase64(ctx context.Context, imageURL string, encryptionInfo map[string]interface{}, fullContent map[string]interface{}) (string, error) {
	originalURL := imageURL
	var lastError error

	// Handle encrypted media
	if encryptionInfo != nil {
		log.Debug().Ctx(ctx).
			Str("url", imageURL).
			Msg("Handling encrypted media with encryption info")

		return downloadAndDecryptMedia(ctx, imageURL, encryptionInfo, fullContent)
	}

	// Convert MXC URLs to HTTP URLs
	if strings.HasPrefix(imageURL, "mxc://") {
		// Extract the server name and media ID from the MXC URL
		// Format: mxc://<server-name>/<media-id>
		parts := strings.SplitN(strings.TrimPrefix(imageURL, "mxc://"), "/", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid MXC URL format: %s", imageURL)
		}
		serverName, mediaID := parts[0], parts[1]

		// Convert to HTTP URL using the homeserver's media endpoint
		// Use v3 API endpoint for better compatibility
		imageURL = fmt.Sprintf("%s/_matrix/media/v3/download/%s/%s",
			config.HomeserverURL, serverName, mediaID)

		log.Debug().Ctx(ctx).
			Str("mxc_url", fmt.Sprintf("mxc://%s/%s", serverName, mediaID)).
			Str("http_url", imageURL).
			Msg("Converted MXC URL to HTTP URL")
	}

	// Try to download the image using v3 endpoint
	dataURL, err := downloadAndEncodeImage(ctx, imageURL)
	if err == nil {
		return dataURL, nil
	}

	lastError = err

	// If original URL was an MXC URL, try fallbacks
	if strings.HasPrefix(originalURL, "mxc://") {
		parts := strings.SplitN(strings.TrimPrefix(originalURL, "mxc://"), "/", 2)
		if len(parts) == 2 {
			serverName, mediaID := parts[0], parts[1]

			// Try fallback 1: r0 endpoint
			fallbackURL := fmt.Sprintf("%s/_matrix/media/r0/download/%s/%s",
				config.HomeserverURL, serverName, mediaID)

			log.Debug().Ctx(ctx).
				Str("original_url", imageURL).
				Str("fallback_url", fallbackURL).
				Msg("First attempt failed, trying r0 media endpoint")

			dataURL, err = downloadAndEncodeImage(ctx, fallbackURL)
			if err == nil {
				return dataURL, nil
			}

			lastError = err

			// Try fallback 2: direct download URL (some homeservers support this)
			directURL := fmt.Sprintf("%s/_matrix/media/download/%s/%s",
				config.HomeserverURL, serverName, mediaID)

			log.Debug().Ctx(ctx).
				Str("original_url", imageURL).
				Str("direct_url", directURL).
				Msg("Second attempt failed, trying direct download URL")

			dataURL, err = downloadAndEncodeImage(ctx, directURL)
			if err == nil {
				return dataURL, nil
			}

			lastError = err
		}
	}

	// If we got here, all attempts failed
	return "", fmt.Errorf("all download attempts failed, last error: %w", lastError)
}

// downloadAndDecryptMedia downloads and decrypts an encrypted media file
func downloadAndDecryptMedia(ctx context.Context, encryptedURL string, encryptionInfo map[string]interface{}, fullContent map[string]interface{}) (string, error) {
	// Check if the size info is available in the encryption info
	if encryptionInfo != nil {
		if size, ok := encryptionInfo["size"].(float64); ok {
			const maxSize = 5 * 1024 * 1024 // 5MB
			if int(size) > maxSize {
				return "", fmt.Errorf("encrypted media is too large: %.0f bytes (max: 5MB)", size)
			}
		}
	}

	// Also check the info section of the full content for size
	if fullContent != nil {
		if info, ok := fullContent["info"].(map[string]interface{}); ok {
			if size, ok := info["size"].(float64); ok {
				const maxSize = 5 * 1024 * 1024 // 5MB
				if int(size) > maxSize {
					return "", fmt.Errorf("encrypted media is too large: %.0f bytes (max: 5MB)", size)
				}
			}
		}
	}

	// Convert MXC URLs to HTTP URLs
	if strings.HasPrefix(encryptedURL, "mxc://") {
		// Extract the server name and media ID from the MXC URL
		// Format: mxc://<server-name>/<media-id>
		parts := strings.SplitN(strings.TrimPrefix(encryptedURL, "mxc://"), "/", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid MXC URL format: %s", encryptedURL)
		}
		serverName, mediaID := parts[0], parts[1]

		// Convert to HTTP URL using the homeserver's media endpoint
		// Use v3 API endpoint for better compatibility
		encryptedURL = fmt.Sprintf("%s/_matrix/media/v3/download/%s/%s",
			config.HomeserverURL, serverName, mediaID)

		log.Debug().Ctx(ctx).
			Str("mxc_url", fmt.Sprintf("mxc://%s/%s", serverName, mediaID)).
			Str("http_url", encryptedURL).
			Msg("Converted MXC URL to HTTP URL for encrypted media")
	}

	// Create a new HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", encryptedURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Add authorization header for encrypted media
	if client.AccessToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.AccessToken))
	}

	// Send the request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// If the v3 endpoint failed, try the r0 endpoint as fallback
		if strings.HasPrefix(encryptedURL, config.HomeserverURL+"/_matrix/media/v3/") {
			fallbackURL := strings.Replace(encryptedURL, "/_matrix/media/v3/", "/_matrix/media/r0/", 1)

			log.Debug().Ctx(ctx).
				Str("original_url", encryptedURL).
				Str("fallback_url", fallbackURL).
				Int("status_code", resp.StatusCode).
				Msg("First attempt failed with non-OK status, trying r0 media endpoint for encrypted media")

			return downloadAndDecryptMediaWithURL(ctx, fallbackURL, encryptionInfo, fullContent)
		}

		// Try direct download URL as a second fallback
		if strings.HasPrefix(encryptedURL, config.HomeserverURL+"/_matrix/media/v3/") {
			directURL := strings.Replace(encryptedURL, "/_matrix/media/v3/", "/_matrix/media/download/", 1)

			log.Debug().Ctx(ctx).
				Str("original_url", encryptedURL).
				Str("direct_url", directURL).
				Int("status_code", resp.StatusCode).
				Msg("First attempt failed with non-OK status, trying direct download URL for encrypted media")

			return downloadAndDecryptMediaWithURL(ctx, directURL, encryptionInfo, fullContent)
		}

		return "", fmt.Errorf("failed to download encrypted image: %w", err)
	}
	defer resp.Body.Close()

	// Check if the response is successful
	if resp.StatusCode != http.StatusOK {
		// If the v3 endpoint failed, try the r0 endpoint as fallback
		if strings.HasPrefix(encryptedURL, config.HomeserverURL+"/_matrix/media/v3/") {
			fallbackURL := strings.Replace(encryptedURL, "/_matrix/media/v3/", "/_matrix/media/r0/", 1)

			log.Debug().Ctx(ctx).
				Str("original_url", encryptedURL).
				Str("fallback_url", fallbackURL).
				Int("status_code", resp.StatusCode).
				Msg("First attempt failed with non-OK status, trying r0 media endpoint for encrypted media")

			return downloadAndDecryptMediaWithURL(ctx, fallbackURL, encryptionInfo, fullContent)
		}

		// Try direct download URL as a second fallback
		if strings.HasPrefix(encryptedURL, config.HomeserverURL+"/_matrix/media/v3/") {
			directURL := strings.Replace(encryptedURL, "/_matrix/media/v3/", "/_matrix/media/download/", 1)

			log.Debug().Ctx(ctx).
				Str("original_url", encryptedURL).
				Str("direct_url", directURL).
				Int("status_code", resp.StatusCode).
				Msg("First attempt failed with non-OK status, trying direct download URL for encrypted media")

			return downloadAndDecryptMediaWithURL(ctx, directURL, encryptionInfo, fullContent)
		}

		return "", fmt.Errorf("failed to download encrypted image: status code %d", resp.StatusCode)
	}

	return decryptAndEncodeMedia(ctx, resp, encryptionInfo, fullContent)
}

// downloadAndDecryptMediaWithURL is a helper function to download and decrypt media with a specific URL
func downloadAndDecryptMediaWithURL(ctx context.Context, url string, encryptionInfo map[string]interface{}, fullContent map[string]interface{}) (string, error) {
	// Create a new HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Add authorization header for encrypted media
	if client.AccessToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.AccessToken))
	}

	// Send the request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download encrypted image: %w", err)
	}
	defer resp.Body.Close()

	// Check if the response is successful
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download encrypted image: status code %d", resp.StatusCode)
	}

	// Check the Content-Length header to avoid downloading large files
	contentLength := resp.ContentLength
	if contentLength > 5*1024*1024 { // 5MB
		return "", fmt.Errorf("encrypted media is too large: %d bytes (max: 5MB)", contentLength)
	}

	return decryptAndEncodeMedia(ctx, resp, encryptionInfo, fullContent)
}

// decryptAndEncodeMedia decrypts and encodes media data from an HTTP response
func decryptAndEncodeMedia(ctx context.Context, resp *http.Response, encryptionInfo map[string]interface{}, fullContent map[string]interface{}) (string, error) {
	// Read the encrypted data with a size limit
	const maxSize = 5 * 1024 * 1024 // 5MB
	limitedReader := io.LimitReader(resp.Body, maxSize+1)
	encryptedData, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("failed to read encrypted data: %w", err)
	}

	// Double-check the size after reading
	if len(encryptedData) > maxSize {
		return "", fmt.Errorf("encrypted media is too large: %d bytes (max: 5MB)", len(encryptedData))
	}

	// Extract the encryption parameters from the encryption info
	if encryptionInfo == nil {
		return "", fmt.Errorf("encryption info is nil")
	}

	// Declare decryptedData variable
	var decryptedData []byte

	// Extract the key object
	keyObj, ok := encryptionInfo["key"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("missing key in encryption info")
	}

	// Extract the IV (initialization vector)
	ivStr, ok := encryptionInfo["iv"].(string)
	if !ok {
		return "", fmt.Errorf("missing iv in encryption info")
	}

	// Extract the key algorithm
	keyAlgorithm, ok := keyObj["alg"].(string)
	if !ok || keyAlgorithm != "A256CTR" {
		return "", fmt.Errorf("unsupported key algorithm: %s", keyAlgorithm)
	}

	// Extract the key data
	keyData, ok := keyObj["k"].(string)
	if !ok {
		return "", fmt.Errorf("missing key data in encryption info")
	}

	// Log the key data for debugging
	log.Debug().Ctx(ctx).
		Str("key_data", keyData).
		Msg("Extracted key data from encryption info")

	// Decode the base64 key
	key, err := tryBase64Decode(ctx, keyData, "key_data")
	if err != nil {
		return "", err
	}

	// Log the decoded key for debugging
	log.Debug().Ctx(ctx).
		Str("key_data", keyData).
		Int("key_length", len(key)).
		Msg("Successfully decoded key")

	// Decode the base64 IV
	iv, err := tryBase64Decode(ctx, ivStr, "iv_str")
	if err != nil {
		// Special handling for IV: try to decode it as a raw binary string
		// Some Matrix clients might encode the IV in a non-standard way
		log.Warn().Ctx(ctx).
			Str("iv_str", ivStr).
			Msg("Failed to decode IV with standard base64 encodings, trying as raw binary")

		// Try to use the raw bytes of the IV string
		iv = []byte(ivStr)

		// Log the raw IV for debugging
		log.Debug().Ctx(ctx).
			Str("iv_str", ivStr).
			Int("iv_length", len(iv)).
			Msg("Using raw bytes of IV string")
	}

	// Ensure IV is 16 bytes (AES block size)
	if len(iv) < 16 {
		paddedIV := make([]byte, 16)
		copy(paddedIV, iv)
		iv = paddedIV

		log.Debug().Ctx(ctx).
			Int("original_length", len(iv)).
			Msg("Padded IV to 16 bytes")
	} else if len(iv) > 16 {
		// Truncate to 16 bytes if longer
		iv = iv[:16]

		log.Debug().Ctx(ctx).
			Int("original_length", len(iv)).
			Msg("Truncated IV to 16 bytes")
	}

	// Create a new AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create a CTR stream
	stream := cipher.NewCTR(block, iv)

	// Decrypt the data
	decryptedData = make([]byte, len(encryptedData))
	stream.XORKeyStream(decryptedData, encryptedData)

	// Validate that the decrypted data is a valid image
	contentType := http.DetectContentType(decryptedData)
	if !strings.HasPrefix(contentType, "image/") {
		log.Warn().Ctx(ctx).
			Str("content_type", contentType).
			Int("data_size", len(decryptedData)).
			Msg("Decrypted data does not appear to be a valid image")

		// Try to extract image info from the full content
		if fullContent != nil {
			// First check if there's an info section with mimetype
			if info, ok := fullContent["info"].(map[string]interface{}); ok {
				if mimetype, ok := info["mimetype"].(string); ok && strings.HasPrefix(mimetype, "image/") {
					contentType = mimetype
					log.Debug().Ctx(ctx).
						Str("content_type", contentType).
						Msg("Using mimetype from info section")
				}
			}

			// If we still don't have a valid image content type, check if there's a direct mimetype
			if !strings.HasPrefix(contentType, "image/") {
				if mimetype, ok := fullContent["mimetype"].(string); ok && strings.HasPrefix(mimetype, "image/") {
					contentType = mimetype
					log.Debug().Ctx(ctx).
						Str("content_type", contentType).
						Msg("Using mimetype from content")
				}
			}
		}

		// If we still don't have a valid image content type, check if there's a direct mimetype
		if !strings.HasPrefix(contentType, "image/") && encryptionInfo != nil {
			if info, ok := encryptionInfo["info"].(map[string]interface{}); ok {
				if mimetype, ok := info["mimetype"].(string); ok && strings.HasPrefix(mimetype, "image/") {
					contentType = mimetype
					log.Debug().Ctx(ctx).
						Str("content_type", contentType).
						Msg("Using mimetype from encryption info")
				}
			}
		}

		// If we still don't have a valid image content type, default to PNG
		if !strings.HasPrefix(contentType, "image/") {
			contentType = "image/png"
			log.Debug().Ctx(ctx).
				Msg("Using default image/png content type")
		}
	}

	// Encode the image data as base64
	base64Data := base64.StdEncoding.EncodeToString(decryptedData)

	// Create the data URL
	dataURL := fmt.Sprintf("data:%s;base64,%s", contentType, base64Data)

	// Log the data URL length for debugging
	log.Debug().Ctx(ctx).
		Str("content_type", contentType).
		Int("data_url_length", len(dataURL)).
		Msg("Created data URL for image")

	return dataURL, nil
}

// tryBase64Decode tries to decode a base64 string using different encodings
func tryBase64Decode(ctx context.Context, data string, name string) ([]byte, error) {
	// Special case for Matrix IV format (e.g., "7xsJDaEec+UAAAAAAAAAAA")
	if name == "iv_str" && strings.Contains(data, "+") {
		// Matrix sometimes uses a special format for IVs
		// Try to decode the part before the + as a base64 string
		parts := strings.Split(data, "+")
		if len(parts) == 2 {
			// Try to decode the first part
			firstPart, err := base64.RawURLEncoding.DecodeString(parts[0])
			if err == nil {
				// If successful, create a 16-byte IV
				iv := make([]byte, 16)
				copy(iv, firstPart)

				log.Debug().Ctx(ctx).
					Str(name, data).
					Int("length", len(iv)).
					Str("encoding", "Matrix special format").
					Msg("Successfully decoded with Matrix special format")

				return iv, nil
			}
		}
	}

	// Special case for Matrix IV format with padding characters
	if name == "iv_str" {
		// Try to clean up the IV string by removing padding characters
		cleanData := strings.ReplaceAll(data, "=", "")
		cleanData = strings.ReplaceAll(cleanData, "-", "+")
		cleanData = strings.ReplaceAll(cleanData, "_", "/")

		// Add padding if needed
		switch len(cleanData) % 4 {
		case 2:
			cleanData += "=="
		case 3:
			cleanData += "="
		}

		// Try to decode the cleaned data
		decoded, err := base64.StdEncoding.DecodeString(cleanData)
		if err == nil {
			log.Debug().Ctx(ctx).
				Str(name, data).
				Str("cleaned", cleanData).
				Int("length", len(decoded)).
				Str("encoding", "Cleaned StdEncoding").
				Msg("Successfully decoded with cleaned StdEncoding")
			return decoded, nil
		}
	}

	// Try with RawURLEncoding first (most common for Matrix)
	decoded, err := base64.RawURLEncoding.DecodeString(data)
	if err == nil {
		log.Debug().Ctx(ctx).
			Str(name, data).
			Int("length", len(decoded)).
			Str("encoding", "RawURLEncoding").
			Msg("Successfully decoded with RawURLEncoding")
		return decoded, nil
	}

	// Try with standard base64 encoding if RawURLEncoding fails
	decoded, err = base64.StdEncoding.DecodeString(data)
	if err == nil {
		log.Debug().Ctx(ctx).
			Str(name, data).
			Int("length", len(decoded)).
			Str("encoding", "StdEncoding").
			Msg("Successfully decoded with StdEncoding")
		return decoded, nil
	}

	// Try with RawStdEncoding if StdEncoding fails
	decoded, err = base64.RawStdEncoding.DecodeString(data)
	if err == nil {
		log.Debug().Ctx(ctx).
			Str(name, data).
			Int("length", len(decoded)).
			Str("encoding", "RawStdEncoding").
			Msg("Successfully decoded with RawStdEncoding")
		return decoded, nil
	}

	// Try with URLEncoding if RawStdEncoding fails
	decoded, err = base64.URLEncoding.DecodeString(data)
	if err == nil {
		log.Debug().Ctx(ctx).
			Str(name, data).
			Int("length", len(decoded)).
			Str("encoding", "URLEncoding").
			Msg("Successfully decoded with URLEncoding")
		return decoded, nil
	}

	// Log the failure
	log.Error().Ctx(ctx).
		Str(name, data).
		Msg("Failed to decode with any base64 encoding")

	return nil, fmt.Errorf("failed to decode %s with any base64 encoding", name)
}

// downloadAndEncodeImage downloads an image from a URL and encodes it as a base64 data URL
func downloadAndEncodeImage(ctx context.Context, imageURL string) (string, error) {
	// Create a new HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Add authorization header if needed (for media endpoints)
	if strings.Contains(imageURL, "/_matrix/media/") && client.AccessToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.AccessToken))
	}

	// Send the request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	// Check if the response is successful
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download image: status code %d", resp.StatusCode)
	}

	// Check the Content-Length header to avoid downloading large files
	contentLength := resp.ContentLength
	if contentLength > 5*1024*1024 { // 5MB
		return "", fmt.Errorf("image is too large: %d bytes (max: 5MB)", contentLength)
	}

	// Read the image data with a size limit
	const maxSize = 5 * 1024 * 1024 // 5MB
	limitedReader := io.LimitReader(resp.Body, maxSize+1)
	imageData, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("failed to read image data: %w", err)
	}

	// Double-check the size after reading
	if len(imageData) > maxSize {
		return "", fmt.Errorf("image is too large: %d bytes (max: 5MB)", len(imageData))
	}

	// Determine the content type
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		// Try to detect content type from the image data
		contentType = http.DetectContentType(imageData)
	}

	// Encode the image data as base64
	base64Data := base64.StdEncoding.EncodeToString(imageData)

	// Create the data URL
	dataURL := fmt.Sprintf("data:%s;base64,%s", contentType, base64Data)

	return dataURL, nil
}
