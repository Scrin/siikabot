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
)

// MessageImageURL returns the URL of the image a message carries, or an error if it carries none.
// For an encrypted image it also returns the encryption info needed for decryption, and for any
// image the full content.
func MessageImageURL(ctx context.Context, msg *Message) (string, map[string]any, map[string]any, error) {
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

// DownloadImage downloads an image from a Matrix URL, decrypting it if it is encrypted, and returns
// it as it was uploaded. What kind of image it is, if it is one at all, is for the caller to find out.
func DownloadImage(ctx context.Context, imageURL string, encryptionInfo map[string]any, fullContent map[string]any) ([]byte, error) {
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
			return nil, fmt.Errorf("invalid MXC URL format: %s", imageURL)
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
	data, err := downloadImage(ctx, imageURL)
	if err == nil {
		return data, nil
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

			data, err = downloadImage(ctx, fallbackURL)
			if err == nil {
				return data, nil
			}

			lastError = err

			// Try fallback 2: direct download URL (some homeservers support this)
			directURL := fmt.Sprintf("%s/_matrix/media/download/%s/%s",
				config.HomeserverURL, serverName, mediaID)

			log.Debug().Ctx(ctx).
				Str("original_url", imageURL).
				Str("direct_url", directURL).
				Msg("Second attempt failed, trying direct download URL")

			data, err = downloadImage(ctx, directURL)
			if err == nil {
				return data, nil
			}

			lastError = err
		}
	}

	// If we got here, all attempts failed
	return nil, fmt.Errorf("all download attempts failed, last error: %w", lastError)
}

// downloadAndDecryptMedia downloads and decrypts an encrypted media file
func downloadAndDecryptMedia(ctx context.Context, encryptedURL string, encryptionInfo map[string]any, fullContent map[string]any) ([]byte, error) {
	// Check if the size info is available in the encryption info
	if encryptionInfo != nil {
		if size, ok := encryptionInfo["size"].(float64); ok {
			const maxSize = 5 * 1024 * 1024 // 5MB
			if int(size) > maxSize {
				return nil, fmt.Errorf("encrypted media is too large: %.0f bytes (max: 5MB)", size)
			}
		}
	}

	// Also check the info section of the full content for size
	if fullContent != nil {
		if info, ok := fullContent["info"].(map[string]any); ok {
			if size, ok := info["size"].(float64); ok {
				const maxSize = 5 * 1024 * 1024 // 5MB
				if int(size) > maxSize {
					return nil, fmt.Errorf("encrypted media is too large: %.0f bytes (max: 5MB)", size)
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
			return nil, fmt.Errorf("invalid MXC URL format: %s", encryptedURL)
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
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
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

			return downloadAndDecryptMediaWithURL(ctx, fallbackURL, encryptionInfo)
		}

		// Try direct download URL as a second fallback
		if strings.HasPrefix(encryptedURL, config.HomeserverURL+"/_matrix/media/v3/") {
			directURL := strings.Replace(encryptedURL, "/_matrix/media/v3/", "/_matrix/media/download/", 1)

			log.Debug().Ctx(ctx).
				Str("original_url", encryptedURL).
				Str("direct_url", directURL).
				Int("status_code", resp.StatusCode).
				Msg("First attempt failed with non-OK status, trying direct download URL for encrypted media")

			return downloadAndDecryptMediaWithURL(ctx, directURL, encryptionInfo)
		}

		return nil, fmt.Errorf("failed to download encrypted image: %w", err)
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

			return downloadAndDecryptMediaWithURL(ctx, fallbackURL, encryptionInfo)
		}

		// Try direct download URL as a second fallback
		if strings.HasPrefix(encryptedURL, config.HomeserverURL+"/_matrix/media/v3/") {
			directURL := strings.Replace(encryptedURL, "/_matrix/media/v3/", "/_matrix/media/download/", 1)

			log.Debug().Ctx(ctx).
				Str("original_url", encryptedURL).
				Str("direct_url", directURL).
				Int("status_code", resp.StatusCode).
				Msg("First attempt failed with non-OK status, trying direct download URL for encrypted media")

			return downloadAndDecryptMediaWithURL(ctx, directURL, encryptionInfo)
		}

		return nil, fmt.Errorf("failed to download encrypted image: status code %d", resp.StatusCode)
	}

	return decryptMedia(ctx, resp, encryptionInfo)
}

// downloadAndDecryptMediaWithURL is a helper function to download and decrypt media with a specific URL
func downloadAndDecryptMediaWithURL(ctx context.Context, url string, encryptionInfo map[string]any) ([]byte, error) {
	// Create a new HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Add authorization header for encrypted media
	if client.AccessToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.AccessToken))
	}

	// Send the request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download encrypted image: %w", err)
	}
	defer resp.Body.Close()

	// Check if the response is successful
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download encrypted image: status code %d", resp.StatusCode)
	}

	// Check the Content-Length header to avoid downloading large files
	contentLength := resp.ContentLength
	if contentLength > 5*1024*1024 { // 5MB
		return nil, fmt.Errorf("encrypted media is too large: %d bytes (max: 5MB)", contentLength)
	}

	return decryptMedia(ctx, resp, encryptionInfo)
}

// decryptMedia decrypts media data from an HTTP response
func decryptMedia(ctx context.Context, resp *http.Response, encryptionInfo map[string]any) ([]byte, error) {
	// Read the encrypted data with a size limit
	const maxSize = 5 * 1024 * 1024 // 5MB
	limitedReader := io.LimitReader(resp.Body, maxSize+1)
	encryptedData, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read encrypted data: %w", err)
	}

	// Double-check the size after reading
	if len(encryptedData) > maxSize {
		return nil, fmt.Errorf("encrypted media is too large: %d bytes (max: 5MB)", len(encryptedData))
	}

	// Extract the encryption parameters from the encryption info
	if encryptionInfo == nil {
		return nil, fmt.Errorf("encryption info is nil")
	}

	// Declare decryptedData variable
	var decryptedData []byte

	// Extract the key object
	keyObj, ok := encryptionInfo["key"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("missing key in encryption info")
	}

	// Extract the IV (initialization vector)
	ivStr, ok := encryptionInfo["iv"].(string)
	if !ok {
		return nil, fmt.Errorf("missing iv in encryption info")
	}

	// Extract the key algorithm
	keyAlgorithm, ok := keyObj["alg"].(string)
	if !ok || keyAlgorithm != "A256CTR" {
		return nil, fmt.Errorf("unsupported key algorithm: %s", keyAlgorithm)
	}

	// Extract the key data
	keyData, ok := keyObj["k"].(string)
	if !ok {
		return nil, fmt.Errorf("missing key data in encryption info")
	}

	// Log the key data for debugging
	log.Debug().Ctx(ctx).
		Str("key_data", keyData).
		Msg("Extracted key data from encryption info")

	// Decode the base64 key
	key, err := tryBase64Decode(ctx, keyData, "key_data")
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create a CTR stream
	stream := cipher.NewCTR(block, iv)

	// Decrypt the data
	decryptedData = make([]byte, len(encryptedData))
	stream.XORKeyStream(decryptedData, encryptedData)

	return decryptedData, nil
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

// downloadImage downloads an image from a URL
func downloadImage(ctx context.Context, imageURL string) ([]byte, error) {
	// Create a new HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Add authorization header if needed (for media endpoints)
	if strings.Contains(imageURL, "/_matrix/media/") && client.AccessToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.AccessToken))
	}

	// Send the request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	// Check if the response is successful
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download image: status code %d", resp.StatusCode)
	}

	// Check the Content-Length header to avoid downloading large files
	contentLength := resp.ContentLength
	if contentLength > 5*1024*1024 { // 5MB
		return nil, fmt.Errorf("image is too large: %d bytes (max: 5MB)", contentLength)
	}

	// Read the image data with a size limit
	const maxSize = 5 * 1024 * 1024 // 5MB
	limitedReader := io.LimitReader(resp.Body, maxSize+1)
	imageData, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read image data: %w", err)
	}

	// Double-check the size after reading
	if len(imageData) > maxSize {
		return nil, fmt.Errorf("image is too large: %d bytes (max: 5MB)", len(imageData))
	}

	return imageData, nil
}
