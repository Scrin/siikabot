//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/Scrin/siikabot/testutil/fakenet"
	"github.com/Scrin/siikabot/testutil/leftovers"
	"github.com/Scrin/siikabot/testutil/pgtest"
	"github.com/Scrin/siikabot/testutil/synapsetest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// The bot's credentials and configuration in the tests
const (
	botLocalpart  = "siikabot"
	botName       = "siikabot"
	adminLocal    = "admin"
	hookSecret    = "e2e-github-secret"
	alertUser     = "alertmanager"
	alertPassword = "e2e-alert-password"
)

// waitTimeout bounds every wait for the bot. The bot answers in well under a second; the rest is for a slow
// machine.
const waitTimeout = 30 * time.Second

// environment is everything the tests run against, started once for the whole package by the first test that
// needs it, and stopped after the last
type environment struct {
	synapse *synapsetest.Server
	net     *fakenet.Net
	// Model is the scripted model, which records what the bot showed it
	Model *fakenet.Model
	botID id.UserID
	// apiURL is where the bot's HTTP server is
	apiURL string
	// database is the bot's database
	database string
	dropDB   func()
	dir      string
	binary   string
	botEnv   []string
	bot      *exec.Cmd
	botLog   string
	exited   chan struct{}
	admin    *User
	users    atomic.Int64
}

var (
	envOnce sync.Once
	env     *environment
	envErr  error
)

func TestMain(m *testing.M) {
	os.Exit(pgtest.Run(m, func() {
		if env != nil {
			env.stop()
		}
	}))
}

// setUp returns the environment, starting it for the first test that asks. A failing test shows the end of the
// bot's log.
func setUp(t *testing.T) *environment {
	t.Helper()
	envOnce.Do(func() { env, envErr = start() })
	if envErr != nil {
		t.Fatalf("starting the environment: %v", envErr)
	}
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("the bot's log, ending:\n%s", tail(env.botLog, 12000))
		}
	})
	return env
}

func start() (*environment, error) {
	e := &environment{}
	var err error
	leftovers.Remove("siikabot-e2e-", "harness.pid")
	if e.dir, err = os.MkdirTemp("", "siikabot-e2e-"); err != nil {
		return nil, err
	}
	if err := leftovers.WritePID(e.dir, "harness.pid"); err != nil {
		return nil, err
	}
	e.net = fakenet.Start()
	e.Model = e.net.Model
	if e.synapse, err = synapsetest.Start(); err != nil {
		e.stop()
		return nil, err
	}
	if e.database, e.dropDB, err = pgtest.NewDatabase(); err != nil {
		e.stop()
		return nil, err
	}

	ctx := context.Background()
	botPassword := randomString()
	botAccount, err := e.synapse.Register(ctx, botLocalpart, botPassword, false)
	if err != nil {
		e.stop()
		return nil, err
	}
	e.botID = id.UserID(botAccount.UserID)
	if e.admin, err = e.register(adminLocal); err != nil {
		e.stop()
		return nil, err
	}

	binary, err := buildBot(e.dir)
	if err != nil {
		e.stop()
		return nil, err
	}
	port, err := freePort()
	if err != nil {
		e.stop()
		return nil, err
	}
	e.apiURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	e.botLog = filepath.Join(e.dir, "bot.log")
	e.binary = binary
	e.botEnv = append(os.Environ(),
		"SIIKABOT_HOMESERVER_URL="+e.synapse.URL,
		"SIIKABOT_USER_ID="+botAccount.UserID,
		"SIIKABOT_PASSWORD="+botPassword,
		"SIIKABOT_HOOK_SECRET="+hookSecret,
		"SIIKABOT_ADMIN="+e.admin.ID.String(),
		"SIIKABOT_CLOUDFLARE_ACCOUNT_ID=e2e-account",
		"SIIKABOT_CLOUDFLARE_API_TOKEN=e2e-token",
		"SIIKABOT_CLOUDFLARE_AI_GATEWAY_ID=e2e-gateway",
		"SIIKABOT_POSTGRES_CONNECTION_STRING="+e.database,
		"SIIKABOT_PICKLE_KEY="+randomString(),
		"SIIKABOT_TIMEZONE=Europe/Helsinki",
		"SIIKABOT_GOOGLE_API_KEY=e2e-google-key",
		"SIIKABOT_GOOGLE_SEARCH_ENGINE_ID=e2e-search-engine",
		"SIIKABOT_ALERTMANAGER_USER="+alertUser,
		"SIIKABOT_ALERTMANAGER_PASSWORD="+alertPassword,
		"SIIKABOT_TEMPO_ENDPOINT=http://127.0.0.1:1",
		"SIIKABOT_TEMPO_USER=e2e",
		"SIIKABOT_TEMPO_PASSWORD=e2e",
		"SIIKABOT_SOURCE_URL=https://github.com/Scrin/siikabot",
		"SIIKABOT_FRONTEND_URL=https://siikabot.example.org/",
		fmt.Sprintf("SIIKABOT_HTTP_ADDR=127.0.0.1:%d", port),
		"SIIKABOT_E2E_NET="+e.net.URL(),
	)
	if dir := os.Getenv("SIIKABOT_E2E_COVERDIR"); dir != "" {
		e.botEnv = append(e.botEnv, "GOCOVERDIR="+dir)
	}
	if err := e.startBot(); err != nil {
		e.stop()
		return nil, err
	}
	if err := e.waitUntilReady(); err != nil {
		log := tail(e.botLog, 12000)
		e.stop()
		return nil, fmt.Errorf("%w\nthe bot's log, ending:\n%s", err, log)
	}
	return e, nil
}

// startBot starts the bot, its log going on where the last run's left off
func (e *environment) startBot() error {
	logFile, err := os.OpenFile(e.botLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	e.bot = exec.Command(e.binary)
	// Its own directory, so it can't pick up a .env from wherever the tests run
	e.bot.Dir = e.dir
	e.bot.Env = e.botEnv
	e.bot.Stdout, e.bot.Stderr = logFile, logFile
	e.bot.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
	if err := e.bot.Start(); err != nil {
		_ = logFile.Close()
		return err
	}
	exited := make(chan struct{})
	e.exited = exited
	go func(cmd *exec.Cmd) {
		_ = cmd.Wait()
		_ = logFile.Close()
		close(exited)
	}(e.bot)
	return nil
}

// stopBot stops the bot the way the deployment does, with SIGTERM
func (e *environment) stopBot() {
	if e.bot == nil || e.bot.Process == nil {
		return
	}
	_ = e.bot.Process.Signal(syscall.SIGTERM)
	select {
	case <-e.exited:
	case <-time.After(20 * time.Second):
		_ = e.bot.Process.Kill()
		<-e.exited
	}
	e.bot = nil
}

// StopBot stops the bot for a test that has things happen while it's down. Every test sharing the bot then
// waits, so only a test that doesn't run in parallel may.
func (e *environment) StopBot(t *testing.T) {
	t.Helper()
	e.stopBot()
	t.Cleanup(func() {
		if e.bot == nil {
			if err := e.startBot(); err != nil {
				t.Fatalf("starting the bot again: %v", err)
			}
		}
	})
}

// StartBot starts the bot again after StopBot, and waits until it's ready
func (e *environment) StartBot(t *testing.T) {
	t.Helper()
	if err := e.startBot(); err != nil {
		t.Fatalf("starting the bot again: %v", err)
	}
	if err := e.waitUntilReady(); err != nil {
		t.Fatalf("the bot wasn't ready after starting again: %v", err)
	}
}

// waitUntilReady waits for the bot's HTTP server, then for it to join a room it's invited to, which it does
// only once its sync loop runs
func (e *environment) waitUntilReady() error {
	deadline := time.Now().Add(60 * time.Second)
	for {
		resp, err := http.Get(e.apiURL + "/api/healthcheck")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if err := e.checkAlive(deadline); err != nil {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}

	ctx := context.Background()
	created, err := e.admin.CreateRoom(ctx, &mautrix.ReqCreateRoom{Preset: "private_chat", Invite: []id.UserID{e.botID}})
	if err != nil {
		return err
	}
	for {
		if joined, _ := e.admin.JoinedMembers(ctx, created.RoomID); joined != nil {
			if _, ok := joined.Joined[e.botID]; ok {
				return nil
			}
		}
		if err := e.checkAlive(deadline); err != nil {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (e *environment) checkAlive(deadline time.Time) error {
	select {
	case <-e.exited:
		return fmt.Errorf("the bot stopped while starting")
	default:
	}
	if time.Now().After(deadline) {
		return fmt.Errorf("the bot wasn't ready in 60 s")
	}
	return nil
}

// stop stops everything that started
func (e *environment) stop() {
	e.stopBot()
	if e.synapse != nil {
		e.synapse.Stop()
	}
	if e.net != nil {
		e.net.Close()
	}
	if e.dropDB != nil {
		e.dropDB()
	}
	if e.dir != "" {
		_ = os.RemoveAll(e.dir)
	}
}

// buildBot builds the bot with the e2e tag, with libolm if it's installed and with goolm if it isn't
func buildBot(dir string) (string, error) {
	binary := filepath.Join(dir, "siikabot")
	tags := []string{"e2e"}
	if _, err := os.Stat("/usr/include/olm/olm.h"); err != nil {
		tags = append(tags, "goolm")
	}
	args := []string{"build", "-tags", strings.Join(tags, ","), "-o", binary}
	if os.Getenv("SIIKABOT_E2E_COVERDIR") != "" {
		args = append(args, "-cover", "-coverpkg=github.com/Scrin/siikabot/...")
	}
	args = append(args, "github.com/Scrin/siikabot")
	if out, err := exec.Command("go", args...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("building the bot: %w: %s", err, out)
	}
	return binary, nil
}

// User is a test user, a real Matrix client
type User struct {
	*mautrix.Client
	ID id.UserID
}

// User registers a new test user, named after name and a number no other test user has
func (e *environment) User(t *testing.T, name string) *User {
	t.Helper()
	u, err := e.register(fmt.Sprintf("%s%d", name, e.users.Add(1)))
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func (e *environment) register(localpart string) (*User, error) {
	account, err := e.synapse.Register(context.Background(), localpart, randomString(), false)
	if err != nil {
		return nil, err
	}
	client, err := mautrix.NewClient(e.synapse.URL, id.UserID(account.UserID), account.AccessToken)
	if err != nil {
		return nil, err
	}
	client.DeviceID = id.DeviceID(account.DeviceID)
	return &User{Client: client, ID: id.UserID(account.UserID)}, nil
}

// Encrypted gives a test user end-to-end encryption until the test ends: a crypto store of its own, in a database
// of its own, and a sync loop that keeps its keys and its view of the rooms up to date. Messages it sends to an
// encrypted room are then encrypted, and the messages it reads decrypted.
func (u *User) Encrypted(t *testing.T) *User {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	config, err := pgx.ParseConfig(pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB := stdlib.OpenDB(*config)
	database, err := dbutil.NewWithDB(sqlDB, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	helper, err := cryptohelper.NewCryptoHelper(u.Client, []byte("e2e-pickle-key"), database)
	if err != nil {
		t.Fatal(err)
	}
	if err := helper.Init(ctx); err != nil {
		t.Fatalf("setting up encryption for %s: %v", u.ID, err)
	}
	u.Crypto = helper
	synced := make(chan struct{})
	go func() {
		defer close(synced)
		_ = u.SyncWithContext(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-synced
		_ = helper.Close()
		_ = sqlDB.Close()
	})
	return u
}

// Room is a room with the bot in it
type Room struct {
	ID    id.RoomID
	owner *User
	env   *environment
}

// roomOptions are what a room is created with
type roomOptions struct {
	historyVisibility event.HistoryVisibility
	withoutBot        bool
	encrypted         bool
}

// WithEncryption creates an end-to-end encrypted room. Its owner has to be Encrypted.
func WithEncryption() RoomOption {
	return func(o *roomOptions) { o.encrypted = true }
}

// RoomOption changes how a room is created
type RoomOption func(*roomOptions)

// WithHistoryVisibility creates the room with a history visibility other than "shared"
func WithHistoryVisibility(v event.HistoryVisibility) RoomOption {
	return func(o *roomOptions) { o.historyVisibility = v }
}

// WithoutBot creates the room without the bot in it
func WithoutBot() RoomOption {
	return func(o *roomOptions) { o.withoutBot = true }
}

// Room creates a room with owner, the members and the bot in it, and waits for them all to join
func (e *environment) Room(t *testing.T, owner *User, members []*User, options ...RoomOption) *Room {
	t.Helper()
	opts := roomOptions{historyVisibility: event.HistoryVisibilityShared}
	for _, option := range options {
		option(&opts)
	}
	ctx := context.Background()
	invite := []id.UserID{}
	if !opts.withoutBot {
		invite = append(invite, e.botID)
	}
	for _, m := range members {
		invite = append(invite, m.ID)
	}
	initialState := []*event.Event{{
		Type:    event.StateHistoryVisibility,
		Content: event.Content{Parsed: &event.HistoryVisibilityEventContent{HistoryVisibility: opts.historyVisibility}},
	}}
	if opts.encrypted {
		initialState = append(initialState, &event.Event{
			Type:    event.StateEncryption,
			Content: event.Content{Parsed: &event.EncryptionEventContent{Algorithm: id.AlgorithmMegolmV1}},
		})
	}
	created, err := owner.CreateRoom(ctx, &mautrix.ReqCreateRoom{Preset: "private_chat", Invite: invite, InitialState: initialState})
	if err != nil {
		t.Fatalf("creating a room: %v", err)
	}
	room := &Room{ID: created.RoomID, owner: owner, env: e}
	for _, m := range members {
		room.Join(t, m)
	}
	if !opts.withoutBot {
		room.waitForMember(t, e.botID)
	}
	if opts.encrypted {
		// Until its sync has seen the room is encrypted, a member's messages would go out in the clear
		for _, u := range append([]*User{owner}, members...) {
			room.WaitUntilEncryptedFor(t, u)
		}
	}
	return room
}

// WaitUntilEncryptedFor waits until a user's client knows the room is encrypted, so that it encrypts what it sends
func (r *Room) WaitUntilEncryptedFor(t *testing.T, u *User) {
	t.Helper()
	if u.Crypto == nil {
		t.Fatalf("%s has no encryption", u.ID)
	}
	ctx := context.Background()
	eventually(t, "the room to be encrypted for "+u.ID.String(), func() bool {
		encrypted, err := u.StateStore.IsEncrypted(ctx, r.ID)
		return err == nil && encrypted
	})
	// A client shares the keys to what it sends only with the members it knows of, as a real one has synced the
	// room's members by the time anyone types into it
	eventually(t, u.ID.String()+" to know the bot is in the room", func() bool {
		member, err := u.StateStore.GetMember(ctx, r.ID, r.env.botID)
		return err == nil && member.Membership == event.MembershipJoin
	})
}

// Invite invites someone into the room, as its owner
func (r *Room) Invite(t *testing.T, u *User) {
	t.Helper()
	if _, err := r.owner.InviteUser(context.Background(), r.ID, &mautrix.ReqInviteUser{UserID: u.ID}); err != nil {
		t.Fatalf("inviting %s: %v", u.ID, err)
	}
}

// Join has someone join the room, inviting them first
func (r *Room) Join(t *testing.T, u *User) {
	t.Helper()
	if u != r.owner {
		r.Invite(t, u)
	}
	if _, err := u.JoinRoomByID(context.Background(), r.ID); err != nil {
		t.Fatalf("%s joining: %v", u.ID, err)
	}
}

func (r *Room) waitForMember(t *testing.T, userID id.UserID) {
	t.Helper()
	eventually(t, "the bot to join", func() bool {
		joined, err := r.owner.JoinedMembers(context.Background(), r.ID)
		if err != nil {
			return false
		}
		_, ok := joined.Joined[userID]
		return ok
	})
}

// Send sends a message with the given content into a room, and returns its event ID
func (u *User) Send(t *testing.T, room *Room, content map[string]any) id.EventID {
	t.Helper()
	resp, err := u.SendMessageEvent(context.Background(), room.ID, event.EventMessage, content)
	if err != nil {
		t.Fatalf("sending a message: %v", err)
	}
	return resp.EventID
}

// Say sends a plain text message that doesn't address the bot
func (u *User) Say(t *testing.T, room *Room, text string) id.EventID {
	t.Helper()
	return u.Send(t, room, map[string]any{"msgtype": "m.text", "body": text})
}

// Ask addresses the bot by name at the start of a message, as people do
func (u *User) Ask(t *testing.T, room *Room, text string) id.EventID {
	t.Helper()
	return u.Say(t, room, botName+": "+text)
}

// Reply replies to a message, with the reply's fallback quoting it the way clients do
func (u *User) Reply(t *testing.T, room *Room, to id.EventID, text string) id.EventID {
	t.Helper()
	return u.Send(t, room, map[string]any{
		"msgtype":      "m.text",
		"body":         text,
		"m.relates_to": map[string]any{"m.in_reply_to": map[string]any{"event_id": to}},
	})
}

// Edit edits a message the user sent
func (u *User) Edit(t *testing.T, room *Room, original id.EventID, text string, extra map[string]any) id.EventID {
	t.Helper()
	newContent := map[string]any{"msgtype": "m.text", "body": text}
	for k, v := range extra {
		newContent[k] = v
	}
	content := map[string]any{
		"msgtype":       "m.text",
		"body":          "* " + text,
		"m.new_content": newContent,
		"m.relates_to":  map[string]any{"rel_type": "m.replace", "event_id": original},
	}
	for k, v := range extra {
		content[k] = v
	}
	return u.Send(t, room, content)
}

// Redact redacts a message
func (u *User) Redact(t *testing.T, room *Room, eventID id.EventID) {
	t.Helper()
	if _, err := u.RedactEvent(context.Background(), room.ID, eventID); err != nil {
		t.Fatalf("redacting: %v", err)
	}
}

// Link is a matrix.to link to a message in a room
func Link(room *Room, eventID id.EventID) string {
	return fmt.Sprintf("https://matrix.to/#/%s/%s", room.ID, eventID)
}

// Pill is a message's content that mentions the bot with a pill, as clients write a mention
func (e *environment) Pill(text string) map[string]any {
	return map[string]any{
		"msgtype":        "m.text",
		"body":           botName + ": " + text,
		"format":         "org.matrix.custom.html",
		"formatted_body": fmt.Sprintf(`<a href="https://matrix.to/#/%s">%s</a>: %s`, e.botID, botName, text),
		"m.mentions":     map[string]any{"user_ids": []string{e.botID.String()}},
	}
}

// messages returns the room's messages, oldest first
func (r *Room) messages(t *testing.T) []*event.Event {
	t.Helper()
	resp, err := r.owner.Messages(context.Background(), r.ID, "", "", mautrix.DirectionBackward, nil, 200)
	if err != nil {
		t.Fatalf("reading the room: %v", err)
	}
	var events []*event.Event
	for i := len(resp.Chunk) - 1; i >= 0; i-- {
		evt := resp.Chunk[i]
		if evt.Type == event.EventEncrypted && r.owner.Crypto != nil {
			// Its key may not have arrived yet, in which case it's left out until it has
			_ = evt.Content.ParseRaw(evt.Type)
			decrypted, err := r.owner.Crypto.Decrypt(context.Background(), evt)
			if err != nil {
				continue
			}
			evt = decrypted
			evt.Content.Raw = decryptedRaw(evt)
		}
		if evt.Type == event.EventMessage {
			events = append(events, evt)
		}
	}
	return events
}

// Latest returns the room's latest message, or nothing if it has none, for waiting on what comes after it
func (r *Room) Latest(t *testing.T) id.EventID {
	t.Helper()
	messages := r.messages(t)
	if len(messages) == 0 {
		return ""
	}
	return messages[len(messages)-1].ID
}

// decryptedRaw is a decrypted event's content as JSON fields, as the tests read every message's
func decryptedRaw(evt *event.Event) map[string]any {
	if evt.Content.Raw != nil {
		return evt.Content.Raw
	}
	data, err := json.Marshal(evt.Content.Parsed)
	if err != nil {
		return nil
	}
	var raw map[string]any
	_ = json.Unmarshal(data, &raw)
	return raw
}

// botMessagesAfter returns the bot's messages after an event, oldest first
func (r *Room) botMessagesAfter(t *testing.T, after id.EventID) []*event.Event {
	t.Helper()
	var found []*event.Event
	seen := after == ""
	for _, evt := range r.messages(t) {
		if evt.ID == after {
			seen = true
			continue
		}
		if seen && evt.Sender == r.env.botID {
			found = append(found, evt)
		}
	}
	return found
}

// BotMessage is a message the bot sent
type BotMessage struct {
	ID id.EventID
	// Body is the message's plain text, without the whitespace around it
	Body    string
	Content map[string]any
}

// WaitForBot waits for the bot's first message after an event
func (r *Room) WaitForBot(t *testing.T, after id.EventID) BotMessage {
	t.Helper()
	var message BotMessage
	eventually(t, "the bot to answer", func() bool {
		if found := r.botMessagesAfter(t, after); len(found) > 0 {
			body, _ := found[0].Content.Raw["body"].(string)
			message = BotMessage{ID: found[0].ID, Body: strings.TrimSpace(body), Content: found[0].Content.Raw}
			return true
		}
		return false
	})
	return message
}

// WaitForBotMessage waits for a message of the bot's after an event for which match holds, and returns it
func (r *Room) WaitForBotMessage(t *testing.T, after id.EventID, match func(content map[string]any) bool) map[string]any {
	t.Helper()
	var content map[string]any
	eventually(t, "a message from the bot", func() bool {
		for _, evt := range r.botMessagesAfter(t, after) {
			if match(evt.Content.Raw) {
				content = evt.Content.Raw
				return true
			}
		}
		return false
	})
	return content
}

// bodyContains matches a message whose body contains text
func bodyContains(text string) func(content map[string]any) bool {
	return func(content map[string]any) bool {
		body, _ := content["body"].(string)
		return strings.Contains(body, text)
	}
}

// ExpectSilence checks that the bot says nothing after an event for a while
func (r *Room) ExpectSilence(t *testing.T, after id.EventID, within time.Duration) {
	t.Helper()
	time.Sleep(within)
	if found := r.botMessagesAfter(t, after); len(found) > 0 {
		t.Errorf("the bot said %q, want nothing", found[0].Content.Raw["body"])
	}
}

// eventually waits until done holds, failing the test if it doesn't in waitTimeout
func eventually(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Query runs SQL on the bot's database and returns the rows' first column as text
func (e *environment) Query(t *testing.T, sql string, args ...any) []string {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, e.database)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, sql, args...)
	if err != nil {
		t.Fatal(err)
	}
	values, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (string, error) {
		var v any
		if err := row.Scan(&v); err != nil {
			return "", err
		}
		return fmt.Sprint(v), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return values
}

// Dashboard is someone logged in to the bot's web dashboard
type Dashboard struct {
	env   *environment
	Token string
}

// Login logs someone in to the dashboard the way the browser does: it asks for a challenge, the user sends it
// to the bot with !auth in a room they share, and it polls until the bot has completed it
func (e *environment) Login(t *testing.T, u *User, room *Room) *Dashboard {
	t.Helper()
	var challenge struct {
		Challenge  string `json:"challenge"`
		PollSecret string `json:"poll_secret"`
	}
	if code := e.api(t, http.MethodPost, "/api/auth/challenge", "", nil, &challenge); code != http.StatusOK {
		t.Fatalf("asking for a challenge: %d", code)
	}
	sent := u.Say(t, room, "!auth "+challenge.Challenge)
	// Its confirmation, which a test waiting on the next message mustn't take for the answer to that
	room.WaitForBotMessage(t, sent, bodyContains("Authenticated for web access"))

	var poll struct {
		Status string `json:"status"`
		Token  string `json:"token"`
	}
	eventually(t, "the login to complete", func() bool {
		e.api(t, http.MethodPost, "/api/auth/poll", "", map[string]string{"challenge": challenge.Challenge,
			"poll_secret": challenge.PollSecret}, &poll)
		return poll.Status == "authenticated"
	})
	return &Dashboard{env: e, Token: poll.Token}
}

// Call calls the dashboard's API, and reads the answer into into if it isn't nil
func (d *Dashboard) Call(t *testing.T, method, path string, body, into any) int {
	t.Helper()
	return d.env.api(t, method, path, d.Token, body, into)
}

// api calls the bot's HTTP API
func (e *environment) api(t *testing.T, method, path, token string, body, into any) int {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, e.apiURL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	if into != nil {
		_ = json.NewDecoder(resp.Body).Decode(into)
	}
	return resp.StatusCode
}

// PostHook posts to one of the bot's webhooks with the given headers
func (e *environment) PostHook(t *testing.T, path string, body []byte, headers map[string]string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, e.apiURL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func randomString() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// tail returns the end of a file
func tail(path string, n int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return err.Error()
	}
	if len(data) > n {
		data = data[len(data)-n:]
	}
	return string(data)
}
