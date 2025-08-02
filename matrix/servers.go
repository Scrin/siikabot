package matrix

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// GetServerVersion returns the version of a Matrix homeserver
func GetServerVersion(ctx context.Context, serverName string) (string, error) {
	// Create a custom HTTP client that skips certificate hostname verification
	// This is needed because federation APIs often use different hostnames than the certificates
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		// Add a shorter dial timeout to avoid hanging
		DialContext: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).DialContext,
	}
	httpClient := &http.Client{
		Transport: tr,
		Timeout:   10 * time.Second,
	}

	// First try to discover the server
	target, originalServer, err := discoverServer(ctx, serverName)
	if err != nil {
		return "", fmt.Errorf("failed to discover server: %v", err)
	}

	// Try federation API first
	fedURL := fmt.Sprintf("https://%s/_matrix/federation/v1/version", target)
	fedReq, err := http.NewRequestWithContext(ctx, "GET", fedURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create federation request: %v", err)
	}

	// Set the Host header to the original server name (without port) for TLS verification
	fedReq.Host = originalServer

	var fedResp struct {
		Server struct {
			Version string `json:"version"`
			Name    string `json:"name"`
		} `json:"server"`
	}

	// Make the federation request
	httpResp, err := httpClient.Do(fedReq)
	if err != nil {
		log.Debug().Ctx(ctx).Err(err).
			Str("server_name", serverName).
			Str("target", target).
			Msg("Failed to connect to federation API")
	} else {
		defer httpResp.Body.Close()
		if httpResp.StatusCode == http.StatusOK {
			if err := json.NewDecoder(httpResp.Body).Decode(&fedResp); err == nil {
				version := fedResp.Server.Version
				if fedResp.Server.Name != "" {
					version = fmt.Sprintf("%s (%s)", fedResp.Server.Name, version)
				}
				return version, nil
			}
		} else {
			log.Debug().Ctx(ctx).
				Str("server_name", serverName).
				Str("target", target).
				Int("status", httpResp.StatusCode).
				Msg("Federation API returned non-200 status")
		}
	}

	// If federation API fails, try client API as fallback
	var clientResp struct {
		Versions []string       `json:"versions"`
		Unstable map[string]any `json:"unstable_features,omitempty"`
	}

	// Build the client API URL - use the original server name for this
	clientURL := fmt.Sprintf("https://%s/_matrix/client/versions", serverName)
	clientReq, err := http.NewRequestWithContext(ctx, "GET", clientURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create client request: %v", err)
	}

	// Make the client request with default client (with proper TLS verification)
	httpResp, err = http.DefaultClient.Do(clientReq)
	if err != nil {
		return "", fmt.Errorf("federation API failed and client API failed: %v", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("federation API failed and client API returned status %d", httpResp.StatusCode)
	}

	if err := json.NewDecoder(httpResp.Body).Decode(&clientResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %v", err)
	}

	// Return the highest unstable version or stable version
	if len(clientResp.Unstable) > 0 {
		features := make([]string, 0, len(clientResp.Unstable))
		for k := range clientResp.Unstable {
			features = append(features, k)
		}
		return fmt.Sprintf("Unstable features: %s", strings.Join(features, ", ")), nil
	}
	return strings.Join(clientResp.Versions, ", "), nil
}

// discoverServer attempts to discover the correct server and port to use
// Returns the target (with port) and the original server name (without port) for Host header
func discoverServer(ctx context.Context, serverName string) (string, string, error) {
	log.Trace().
		Ctx(ctx).
		Str("server_name", serverName).
		Msg("Starting server discovery")

	// Try .well-known first
	wellKnownURL := fmt.Sprintf("https://%s/.well-known/matrix/server", serverName)

	// Create a client with timeout for .well-known request
	wellKnownClient := &http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := wellKnownClient.Get(wellKnownURL)
	if err == nil && resp.StatusCode == 200 {
		var wellKnown struct {
			ServerName string `json:"m.server"`
		}
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(&wellKnown); err == nil && wellKnown.ServerName != "" {
			log.Debug().
				Ctx(ctx).
				Str("server_name", serverName).
				Str("delegated_server", wellKnown.ServerName).
				Msg("Found server delegation in .well-known")

			// If the delegated server contains a port, use it directly
			if strings.Contains(wellKnown.ServerName, ":") {
				parts := strings.Split(wellKnown.ServerName, ":")
				return wellKnown.ServerName, parts[0], nil
			}

			// Try default HTTPS port first (443)
			if err := checkServerPort(wellKnown.ServerName, 443); err == nil {
				return fmt.Sprintf("%s:443", wellKnown.ServerName), wellKnown.ServerName, nil
			}

			// Then try default federation port (8448)
			if err := checkServerPort(wellKnown.ServerName, 8448); err == nil {
				return fmt.Sprintf("%s:8448", wellKnown.ServerName), wellKnown.ServerName, nil
			}

			// If both default ports fail, try SRV record
			if host, port, err := lookupSRV(ctx, wellKnown.ServerName); err == nil {
				return fmt.Sprintf("%s:%d", host, port), wellKnown.ServerName, nil
			}

			// Fall back to port 8448 if everything else fails
			return fmt.Sprintf("%s:8448", wellKnown.ServerName), wellKnown.ServerName, nil
		}
	}

	log.Debug().
		Ctx(ctx).
		Err(err).
		Str("server_name", serverName).
		Msg("Failed to lookup .well-known, trying SRV record")

	// If .well-known fails, try SRV record
	if host, port, err := lookupSRV(ctx, serverName); err == nil {
		return fmt.Sprintf("%s:%d", host, port), serverName, nil
	}

	// Fall back to port 8448 with original server name
	log.Debug().
		Ctx(ctx).
		Str("server_name", serverName).
		Msg("Failed to lookup SRV record, falling back to port 8448")
	return fmt.Sprintf("%s:8448", serverName), serverName, nil
}

// checkServerPort attempts to connect to the server on the specified port
func checkServerPort(serverName string, port int) error {
	// Create a custom HTTP client that skips certificate hostname verification
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		// Add a shorter dial timeout to avoid hanging
		DialContext: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).DialContext,
	}
	httpClient := &http.Client{
		Transport: tr,
		Timeout:   10 * time.Second,
	}

	url := fmt.Sprintf("https://%s:%d/_matrix/federation/v1/version", serverName, port)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	// Set the Host header to the server name for TLS verification
	req.Host = serverName

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Only consider it a success if we get a 200 OK
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	return nil
}

// lookupSRV performs SRV record lookup for a server
func lookupSRV(ctx context.Context, serverName string) (addr string, port int, err error) {
	log.Trace().
		Ctx(ctx).
		Str("server_name", serverName).
		Msg("Starting SRV lookup for matrix-fed")

	// Use a channel to handle the timeout
	resultChan := make(chan struct {
		addrs []*net.SRV
		err   error
	}, 1)

	go func() {
		log.Trace().
			Ctx(ctx).
			Str("server_name", serverName).
			Msg("Performing net.LookupSRV for matrix-fed")

		_, addrs, err := net.LookupSRV("matrix-fed", "tcp", serverName)

		log.Trace().
			Ctx(ctx).
			Str("server_name", serverName).
			Int("addr_count", len(addrs)).
			Err(err).
			Msg("SRV lookup completed")

		resultChan <- struct {
			addrs []*net.SRV
			err   error
		}{addrs, err}
	}()

	select {
	case result := <-resultChan:
		if result.err != nil {
			log.Trace().
				Ctx(ctx).
				Str("server_name", serverName).
				Err(result.err).
				Msg("SRV lookup failed")
			return "", 0, result.err
		}
		if len(result.addrs) == 0 {
			log.Trace().
				Ctx(ctx).
				Str("server_name", serverName).
				Msg("No SRV records found")
			return "", 0, fmt.Errorf("no SRV records found")
		}

		target := strings.TrimSuffix(result.addrs[0].Target, ".")
		port := int(result.addrs[0].Port)

		log.Trace().
			Ctx(ctx).
			Str("server_name", serverName).
			Str("target", target).
			Int("port", port).
			Msg("SRV lookup successful")

		return target, port, nil
	case <-ctx.Done():
		log.Trace().
			Ctx(ctx).
			Str("server_name", serverName).
			Err(ctx.Err()).
			Msg("SRV lookup timed out")
		return "", 0, fmt.Errorf("SRV lookup timeout: %v", ctx.Err())
	}
}
