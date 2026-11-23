// Package synapsetest runs a throwaway, non-federated Synapse for the end-to-end tests: from the venv
// scripts/install-synapse.sh makes, or the one SIIKABOT_TEST_SYNAPSE names, on a free loopback port, with its data
// in SQLite in a temporary directory of its own. SIIKABOT_TEST_HOMESERVER, with its registration shared secret in
// SIIKABOT_TEST_HOMESERVER_SECRET, points the tests at a homeserver that's already running instead.
package synapsetest

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Scrin/siikabot/testutil/leftovers"
)

// Version is the Synapse the tests run, production's. Keep it in step with scripts/install-synapse.sh.
const Version = "1.165.0"

// Environment variables that change where the homeserver comes from
const (
	VenvEnv   = "SIIKABOT_TEST_SYNAPSE"
	ServerEnv = "SIIKABOT_TEST_HOMESERVER"
	SecretEnv = "SIIKABOT_TEST_HOMESERVER_SECRET"
)

// Server is a homeserver the tests use
type Server struct {
	// URL is where its client API is
	URL    string
	secret string
	dir    string
	cmd    *exec.Cmd
	logs   string
	exited chan struct{}

	// adminToken is the token of a server admin the package registers for itself, to exempt the tests' users from
	// rate limits
	adminOnce  sync.Once
	adminToken string
	adminErr   error
}

// Start starts a homeserver for the tests, or finds the one ServerEnv names
func Start() (*Server, error) {
	if url := os.Getenv(ServerEnv); url != "" {
		secret := os.Getenv(SecretEnv)
		if secret == "" {
			return nil, fmt.Errorf("synapsetest: %s needs its registration shared secret in %s", ServerEnv, SecretEnv)
		}
		return &Server{URL: strings.TrimSuffix(url, "/"), secret: secret}, nil
	}

	venv := os.Getenv(VenvEnv)
	if venv == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return nil, err
		}
		venv = filepath.Join(cache, "siikabot", "synapse-"+Version)
	}
	python := filepath.Join(venv, "bin", "python")
	if _, err := os.Stat(python); err != nil {
		return nil, fmt.Errorf("synapsetest: no Synapse in %s: run scripts/install-synapse.sh, or set %s to a venv with Synapse "+
			"in it, or %s to a homeserver", venv, VenvEnv, ServerEnv)
	}

	leftovers.Remove("siikabot-synapse-", "homeserver.pid")
	dir, err := os.MkdirTemp("", "siikabot-synapse-")
	if err != nil {
		return nil, err
	}
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	s := &Server{
		URL:    fmt.Sprintf("http://127.0.0.1:%d", port),
		secret: "synapsetest-" + filepath.Base(dir),
		dir:    dir,
		logs:   filepath.Join(dir, "homeserver.log"),
		exited: make(chan struct{}),
	}
	config := filepath.Join(dir, "homeserver.yaml")
	if err := os.WriteFile(config, []byte(s.config(port)), 0o600); err != nil {
		s.Stop()
		return nil, err
	}

	// The signing key, which Synapse refuses to start without
	if out, err := exec.Command(python, "-m", "synapse.app.homeserver", "-c", config, "--generate-keys").CombinedOutput(); err != nil {
		s.Stop()
		return nil, fmt.Errorf("synapsetest: generating keys: %w: %s", err, out)
	}

	logFile, err := os.Create(s.logs)
	if err != nil {
		s.Stop()
		return nil, err
	}
	s.cmd = exec.Command(python, "-m", "synapse.app.homeserver", "-c", config)
	s.cmd.Dir = dir
	s.cmd.Stdout, s.cmd.Stderr = logFile, logFile
	// A test process that ends without stopping it, such as with a panic, takes it along
	s.cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
	if err := s.cmd.Start(); err != nil {
		_ = logFile.Close()
		s.Stop()
		return nil, fmt.Errorf("synapsetest: starting Synapse: %w", err)
	}
	go func() {
		_ = s.cmd.Wait()
		_ = logFile.Close()
		close(s.exited)
	}()

	for deadline := time.Now().Add(60 * time.Second); ; {
		resp, err := http.Get(s.URL + "/_matrix/client/versions")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return s, nil
			}
		}
		select {
		case <-s.exited:
			log := s.Log()
			s.Stop()
			return nil, fmt.Errorf("synapsetest: Synapse stopped while starting:\n%s", log)
		case <-time.After(100 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			log := s.Log()
			s.Stop()
			return nil, fmt.Errorf("synapsetest: Synapse didn't answer in 60 s:\n%s", log)
		}
	}
}

// config is Synapse's configuration: one client listener on loopback and nothing else, no federation, and every
// client rate limit out of the way. The bot makes far more requests than a person, in parallel tests far more
// still, and Synapse limits more than messages: joins, invites, profile lookups and so on. A new Synapse that adds a
// limit shows up as M_LIMIT_EXCEEDED in the bot's log.
func (s *Server) config(port int) string {
	limit := "{per_second: 1000, burst_count: 1000}"
	return fmt.Sprintf(`server_name: localhost
pid_file: %[1]s/homeserver.pid
listeners:
  - port: %[2]d
    bind_addresses: ['127.0.0.1']
    type: http
    tls: false
    x_forwarded: false
    resources:
      - names: [client]
        compress: false
database:
  name: sqlite3
  args:
    database: %[1]s/homeserver.db
media_store_path: %[1]s/media
signing_key_path: %[1]s/signing.key
registration_shared_secret: %[3]q
enable_registration: false
report_stats: false
trusted_key_servers: []
suppress_key_server_warning: true
federation_domain_whitelist: []
presence:
  enabled: false
rc_message: %[4]s
rc_registration: %[4]s
rc_login:
  address: %[4]s
  account: %[4]s
  failed_attempts: %[4]s
rc_joins:
  local: %[4]s
  remote: %[4]s
rc_joins_per_room: %[4]s
rc_invites:
  per_room: %[4]s
  per_user: %[4]s
  per_issuer: %[4]s
rc_admin_redaction: %[4]s
rc_room_creation: %[4]s
rc_media_create: %[4]s
rc_profile: %[4]s
rc_presence:
  per_user: %[4]s
rc_key_requests: %[4]s
rc_delayed_event_mgmt: %[4]s
rc_reports: %[4]s
rc_user_directory: %[4]s
rc_third_party_invite: %[4]s
`, s.dir, port, s.secret, limit)
}

// Stop stops the homeserver and removes its data, if the tests started it
func (s *Server) Stop() {
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-s.exited:
		case <-time.After(15 * time.Second):
			_ = s.cmd.Process.Kill()
			<-s.exited
		}
	}
	if s.dir != "" {
		_ = os.RemoveAll(s.dir)
	}
}

// FullLog returns the homeserver's whole log, for keeping
func (s *Server) FullLog() string {
	if s.logs == "" {
		return ""
	}
	data, _ := os.ReadFile(s.logs)
	return string(data)
}

// Log returns the end of the homeserver's log, for a failing test to show
func (s *Server) Log() string {
	if s.logs == "" {
		return "(the homeserver isn't the tests' own)"
	}
	data, _ := os.ReadFile(s.logs)
	if len(data) > 20000 {
		data = data[len(data)-20000:]
	}
	return string(data)
}

// Account is a user the tests registered
type Account struct {
	UserID      string `json:"user_id"`
	AccessToken string `json:"access_token"`
	DeviceID    string `json:"device_id"`
}

// Register registers a user through the shared-secret admin API and logs them in. The user is exempt from Synapse's
// limit on messages: the bot makes far more requests than a person, and tests running in parallel far more still,
// and the configuration alone doesn't keep Synapse from limiting them.
func (s *Server) Register(ctx context.Context, localpart, password string, admin bool) (Account, error) {
	account, err := s.register(ctx, localpart, password, admin)
	if err != nil {
		return Account{}, err
	}
	s.adminOnce.Do(func() {
		var admin Account
		admin, s.adminErr = s.register(ctx, "synapsetest-admin", "synapsetest-"+filepath.Base(s.dir), true)
		s.adminToken = admin.AccessToken
	})
	if s.adminErr != nil {
		return Account{}, fmt.Errorf("synapsetest: registering its admin: %w", s.adminErr)
	}
	// No messages per second at all means no limit
	var override map[string]any
	if err := s.callAs(ctx, s.adminToken, http.MethodPost, "/_synapse/admin/v1/users/"+url.PathEscape(account.UserID)+"/override_ratelimit",
		map[string]int{"messages_per_second": 0, "burst_count": 0}, &override); err != nil {
		return Account{}, fmt.Errorf("synapsetest: exempting %s from rate limits: %w", account.UserID, err)
	}
	return account, nil
}

// register registers a user through the shared-secret admin API and logs them in
func (s *Server) register(ctx context.Context, localpart, password string, admin bool) (Account, error) {
	var nonce struct {
		Nonce string `json:"nonce"`
	}
	if err := s.call(ctx, http.MethodGet, "/_synapse/admin/v1/register", nil, &nonce); err != nil {
		return Account{}, err
	}

	adminFlag := "notadmin"
	if admin {
		adminFlag = "admin"
	}
	mac := hmac.New(sha1.New, []byte(s.secret))
	mac.Write([]byte(nonce.Nonce + "\x00" + localpart + "\x00" + password + "\x00" + adminFlag))

	var account Account
	err := s.call(ctx, http.MethodPost, "/_synapse/admin/v1/register", map[string]any{
		"nonce": nonce.Nonce, "username": localpart, "password": password, "admin": admin,
		"mac": hex.EncodeToString(mac.Sum(nil)),
	}, &account)
	return account, err
}

// call makes a request to the homeserver and reads its JSON answer
func (s *Server) call(ctx context.Context, method, path string, body, into any) error {
	return s.callAs(ctx, "", method, path, body, into)
}

// callAs makes a request to the homeserver with an access token, if it isn't empty, and reads its JSON answer
func (s *Server) callAs(ctx context.Context, token, method, path string, body, into any) error {
	var reader *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.URL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&failure)
		return fmt.Errorf("synapsetest: %s %s: %d %s", method, path, resp.StatusCode, failure.Error)
	}
	return json.NewDecoder(resp.Body).Decode(into)
}

// freePort finds a loopback port nothing listens on
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("synapsetest: not a TCP address")
	}
	return addr.Port, nil
}
