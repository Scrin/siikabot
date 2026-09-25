package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/Scrin/siikabot/metrics"
	"github.com/Scrin/siikabot/tracing"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// roomLocks serialises chat turns per room. Each message is handled in its own goroutine, and two
// turns running concurrently in one room would interleave their history writes and fight over the
// typing indicator: matrix.SendTyping is a stateless call with no reference counting, so whichever
// turn finishes first switches the indicator off while the other is still working.
var roomLocks sync.Map

// lockRoom blocks until this room has no other chat turn in flight, returning the unlock function
func lockRoom(roomID string) func() {
	value, _ := roomLocks.LoadOrStore(roomID, &sync.Mutex{})
	mutex := value.(*sync.Mutex)
	mutex.Lock()
	return mutex.Unlock
}

// turnTimeout bounds a whole turn, tools and all. Without it a turn can run for the product of the
// per-call timeout and the iteration limit, during which the user has no idea anything is wrong.
const turnTimeout = 3 * time.Minute

// persistTimeout bounds the writes that record what a turn did
const persistTimeout = 5 * time.Second

// persistContext returns a context for writes that have to complete even when the turn itself was
// cancelled or ran out of time. A turn that overruns its deadline would otherwise also lose the
// record of what it had already done, which is exactly when that record is most worth having.
func persistContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), persistTimeout)
}

// The typing indicator has to be re-sent periodically, since the server expires it. The refresh runs
// comfortably inside the timeout so the indicator never lapses mid-turn: previously it was set once
// per iteration, so an iteration that ran long left the bot looking idle while it was still working.
const typingIndicatorTimeout = 30 * time.Second
const typingIndicatorRefresh = 20 * time.Second

// startTypingIndicator shows the typing indicator and keeps it alive until the returned stop
// function is called. Safe to stop more than once.
func startTypingIndicator(ctx context.Context, roomID string) func() {
	matrix.SendTyping(ctx, roomID, true, typingIndicatorTimeout)

	return keepAlive(ctx, typingIndicatorRefresh,
		func() { matrix.SendTyping(ctx, roomID, true, typingIndicatorTimeout) },
		func() {
			// Deliberately not the turn context, which may already be cancelled or timed out by the
			// time we get here, and would take the "stop typing" call down with it
			matrix.SendTyping(context.Background(), roomID, false, 0)
		})
}

// keepAlive calls refresh on an interval until the returned stop function is called or the context
// ends, then calls onStop exactly once. Stopping more than once is safe: the stop runs from a defer
// while the error paths return early, so a double stop is easy to reach.
func keepAlive(ctx context.Context, interval time.Duration, refresh, onStop func()) func() {
	done := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				refresh()
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			onStop()
		})
	}
}

// HandleMention runs a chat turn for a message that addressed the bot
func HandleMention(ctx context.Context, trigger Trigger) {
	roomID, sender, msg := trigger.RoomID, trigger.Sender, trigger.Body
	if strings.TrimSpace(msg) == "" {
		return
	}

	// Serialise turns per room. A queued message waits here, so its own timer starts once it is
	// actually being worked on rather than while it waits.
	unlockRoom := lockRoom(roomID)
	defer unlockRoom()

	// A trigger deleted while it waited for the room gets no turn at all
	if isRedacted(trigger.EventID) {
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("event_id", trigger.EventID).
			Msg("Skipping a chat turn whose trigger was redacted")
		return
	}

	// Bound the whole turn, so a stuck model or a long tool loop fails in a knowable time instead
	// of grinding on invisibly. Applied after the lock so queueing does not eat into the budget.
	ctx, cancelTurn := context.WithTimeout(ctx, turnTimeout)
	defer cancelTurn()

	ctx, turnSpan := tracer.Start(ctx, "chat.turn", trace.WithAttributes(
		attribute.String("matrix.room_id", roomID),
		attribute.String("matrix.sender", sender),
		attribute.String("matrix.event_id", trigger.EventID),
	))
	defer turnSpan.End()

	startTime := time.Now()

	log.Debug().Ctx(ctx).
		Str("room_id", roomID).
		Str("sender", sender).
		Str("chat_msg", msg).
		Msg("Processing chat command")

	// The turn the history rows belong to, and where its answer and any failure message go
	turn := db.Turn{RoomID: roomID, EventID: trigger.EventID, ThreadRootID: trigger.ThreadRootID}
	target := answerTarget(trigger)

	// Variable to track tool iterations
	iterationCount := 0

	// Keep the typing indicator alive for as long as the turn actually runs
	stopTyping := startTypingIndicator(ctx, roomID)
	defer stopTyping()

	// Accumulates everything that happens during this turn, for the summary logged at the end
	stats := &turnStats{outcome: "ok"}

	// fail ends a turn that has no answer to give: the turn is recorded with the outcome, and the
	// room is told in place of the answer
	fail := func(outcome, model string, hasImage bool) {
		duration := time.Since(startTime)
		metrics.RecordChatRequestDuration(model, hasImage, duration.Seconds())
		stats.outcome = outcome
		stats.finish(ctx, roomID, sender, model, hasImage, duration)
		stats.recordOnSpan(turnSpan)
		matrix.SendMessageTo(ctx, roomID, target, failureMessage(outcome), failureDebugData(ctx, model, outcome))
	}

	// Read once and used for the whole turn, so a change made with !chat while a turn is running
	// cannot leave that turn on a mix of old and new settings
	cfg, err := db.GetChatConfig(ctx)
	if err != nil {
		// Already logged. There is nothing to fall back to: the database is the only source of the
		// configuration, and without it there is no model to ask.
		fail("config_unavailable", "", false)
		return
	}

	// Build the prompt: system prompt, history, and the current message with what it refers to
	buildCtx, buildSpan := tracer.Start(ctx, "chat.build_context")
	prompt := buildInitialMessages(buildCtx, trigger, cfg)
	messages, hasImage, model, composition := prompt.messages, prompt.hasImage, prompt.model, prompt.composition
	buildSpan.SetAttributes(
		attribute.Int("siikabot.chat.message_count", len(messages)),
		attribute.Int("siikabot.chat.history_tokens", composition.history),
		attribute.Bool("siikabot.chat.has_image", hasImage),
		attribute.Bool("siikabot.chat.is_dm", prompt.room.isDM()),
		attribute.Int("siikabot.chat.member_count", len(prompt.room.Members)),
		attribute.Bool("siikabot.chat.has_reply_context", prompt.userTurn.ReplyTo != nil),
		attribute.Int("siikabot.chat.link_count", len(prompt.userTurn.Links)),
		attribute.Bool("siikabot.chat.in_thread", trigger.ThreadRootID != ""),
		attribute.Bool("siikabot.chat.thread_seeded", prompt.threadSeeded),
		attribute.Int("siikabot.chat.unseen_before", trigger.UnseenBefore),
	)
	buildSpan.End()

	// Get tool definitions from the registry
	tools := toolRegistry.GetToolDefinitions()

	composition.tools = estimateToolDefinitionTokens(tools)
	composition.record(ctx)

	turnSpan.SetAttributes(
		attribute.String("siikabot.chat.model", model),
		attribute.Bool("siikabot.chat.has_image", hasImage),
		attribute.Int("siikabot.chat.prompt_tokens_estimated", composition.total()),
	)

	// Cap the response length, so a runaway generation is not billed in full
	maxTokens := cfg.MaxTokens

	// Recorded by Cloudflare as span attributes and log fields, which is the only way its side of a
	// turn knows which room and user it belonged to
	metadata := gatewayMetadata(roomID, sender, 1)

	// Create the initial request
	req := aigateway.ChatRequest{
		Model:     model,
		Messages:  messages,
		Tools:     tools,
		MaxTokens: &maxTokens,
		Metadata:  metadata,
	}

	// Estimated size of the whole prompt, compared against the reported usage below so the accuracy
	// of the heuristic driving the context window stays visible
	estimatedPromptTokens := composition.total()

	// The window metric records the replayed history alone, which is the part the token budget
	// actually governs — the rest of the prompt is fixed overhead the budget has no say over
	metrics.RecordChatContextWindowTokens(model, composition.history)

	// Send the request to the AI Gateway
	log.Debug().Ctx(ctx).
		Str("room_id", roomID).
		Str("sender", sender).
		Str("model", model).
		Bool("has_image", hasImage).
		Int("message_count", len(messages)).
		Int("estimated_prompt_tokens", estimatedPromptTokens).
		Int("context_window_tokens", composition.history).
		Msg("Sending chat request to the AI Gateway")

	callStart := time.Now()
	chatResp, err := aigateway.SendChatRequest(ctx, req)
	stats.recordModelCall(chatResp, time.Since(callStart))
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Str("sender", sender).
			Str("model", model).
			Bool("has_image", hasImage).
			Msg("Failed to send chat request")
		fail(requestFailure(ctx), model, hasImage)
		return
	}

	if len(chatResp.Choices) == 0 {
		log.Error().Ctx(ctx).
			Str("room_id", roomID).
			Str("sender", sender).
			Str("model", model).
			Bool("has_image", hasImage).
			Msg("Chat API returned no choices")
		fail("no_choices", model, hasImage)
		return
	}

	// Skipped for image turns: an image costs a number of tokens that bears no relation to the length
	// of its data URI, so comparing them would only measure something the estimator never modelled
	if chatResp.Usage != nil && !hasImage {
		recordTokenEstimateDrift(ctx, model, estimatedPromptTokens, chatResp.Usage.PromptTokens)
	}

	// Save the user message to history
	persist(ctx, turn, "user_turn", func(ctx context.Context) error {
		return db.SaveUserTurn(ctx, prompt.userTurn)
	})

	// The response the answer comes from: this one, or the last one of the tool iterations
	finalResp, failure := chatResp, ""

	// Check if the model wants to use a tool
	if chatResp.Choices[0].FinishReason == "tool_calls" && len(chatResp.Choices[0].Message.ToolCalls) > 0 {
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("sender", sender).
			Str("model", model).
			Bool("has_image", hasImage).
			Int("tool_calls", len(chatResp.Choices[0].Message.ToolCalls)).
			Msg("Model requested tool calls")

		// Process tool calls iteratively
		iterationCount, messages, finalResp, failure = processToolCalls(
			ctx, toolContext(ctx, trigger, prompt.room, target), turn, sender, model, hasImage, cfg,
			chatResp, messages, tools, stats,
		)
	}
	stats.iterations = iterationCount
	metrics.RecordChatToolIterations(iterationCount)

	var assistantResponse string
	if failure == "" {
		assistantResponse, failure = extractAssistantResponse(ctx, roomID, sender, model, hasImage, finalResp)
	}

	// A turn that fails part way keeps the question and the tool calls it stored, but what the room
	// is told is not stored as the answer. Replayed later, it would read as something the bot had
	// said; without it, the model sees a question that went unanswered, which is what happened.
	if failure != "" {
		fail(failure, model, hasImage)
		return
	}

	// What is posted, and stored, is the answer without a header the model may have copied
	assistantResponse = stripImitatedHeader(assistantResponse)

	// The turn's own work ends here. Waiting for the answer to be delivered is not part of it, so
	// the duration is taken now, although the turn is only recorded once the delivery is known.
	turnDuration := time.Since(startTime)
	metrics.RecordChatRequestDuration(model, hasImage, turnDuration.Seconds())

	// Create debug data with model info and tool calls
	debugData := buildDebugData(ctx, model, messages, iterationCount)

	// A trigger redacted while the turn was running gets no answer, and what the turn stored goes
	// with it. The turn itself did its work, so it is recorded as it went.
	if forgotten(ctx, turn) {
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("turn_event_id", turn.EventID).
			Msg("Dropping the answer to a redacted trigger")
		stats.finish(ctx, roomID, sender, model, hasImage, turnDuration)
		stats.recordOnSpan(turnSpan)
		return
	}

	// The answer is stored once it has been delivered, with the event it was delivered as, so that
	// a redaction of the answer can find it. An answer the room never saw isn't stored at all, and
	// counts as a failed turn. The room stays locked until then, so the next turn in the room
	// starts after this answer is out.
	//
	// The model writes members' user IDs, and the room sees pills with their names in their place.
	// What is stored is the answer as the model wrote it.
	delivered := matrix.SendMarkdownFormattedNoticeTo(ctx, roomID, target, assistantResponse, prompt.room.pillNames(), debugData)
	answerEventID := awaitDelivery(delivered)
	if answerEventID == "" {
		stats.outcome = "not_delivered"
	}
	stats.finish(ctx, roomID, sender, model, hasImage, turnDuration)
	stats.recordOnSpan(turnSpan)

	if answerEventID == "" {
		log.Warn().Ctx(ctx).
			Str("room_id", roomID).
			Str("turn_event_id", turn.EventID).
			Msg("Chat answer was not delivered, so it is not stored")
		return
	}
	persist(ctx, turn, "answer", func(ctx context.Context) error {
		return db.SaveAnswer(ctx, turn, config.UserID, assistantResponse, answerEventID)
	})
}

// answerTarget places the answer to a trigger, and any failure message with it: in the thread the
// trigger was sent in, or the main timeline, and as a reply to the trigger if anything was posted
// there after it. An answer still queued when its trigger is redacted is dropped.
func answerTarget(trigger Trigger) matrix.Target {
	eventID := trigger.EventID
	return matrix.Target{
		ThreadRootID: trigger.ThreadRootID,
		InReplyTo:    eventID,
		Cancelled:    func() bool { return isRedacted(eventID) },
	}
}

// deliveryTimeout bounds how long a turn waits for its answer to be delivered. The room stays
// locked meanwhile, so a send that is stuck must not hold it for long.
const deliveryTimeout = 30 * time.Second

// awaitDelivery waits for a queued message to be sent, returning its event ID, or an empty one if
// it wasn't sent in time or at all
func awaitDelivery(delivered <-chan string) string {
	timer := time.NewTimer(deliveryTimeout)
	defer timer.Stop()

	select {
	case eventID := <-delivered:
		return eventID
	case <-timer.C:
		return ""
	}
}

// failureMessage is what the room is told, in place of an answer, when a turn fails with the
// outcome
func failureMessage(outcome string) string {
	switch outcome {
	case "turn_timeout":
		return "That took too long to answer, so I gave up. Try again, or ask something narrower."
	case "no_choices", "empty_response":
		return "No response from chat API"
	case "tool_failed":
		return "Failed to process tool calls"
	default:
		return "Failed to process chat request"
	}
}

// requestFailure is the outcome of a model call that failed: the turn ran out of time, or the call
// itself failed
func requestFailure(ctx context.Context) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "turn_timeout"
	}
	return "request_failed"
}

// toolContext carries what tools need to act for the turn: the room, the sender they act for,
// whether the room is a DM (which scopes memories), and where anything they post goes
func toolContext(ctx context.Context, trigger Trigger, room roomInfo, target matrix.Target) context.Context {
	ctx = context.WithValue(ctx, "room_id", trigger.RoomID)
	ctx = context.WithValue(ctx, "sender", trigger.Sender)
	ctx = context.WithValue(ctx, "room_is_dm", room.isDM())
	return context.WithValue(ctx, "reply_target", target)
}

// turnPrompt is the prompt built for a turn, with what the rest of the turn needs from building it
type turnPrompt struct {
	messages    []aigateway.Message
	hasImage    bool
	model       string
	composition promptComposition
	// userTurn is the trigger as it is stored, once the turn gets that far
	userTurn db.UserTurn
	room     roomInfo
	// threadSeeded says the history begins with the turn the trigger's thread was started from
	threadSeeded bool
}

// buildInitialMessages creates the initial messages array with system prompt, history, and user
// message, along with the estimated token cost of each part of it
func buildInitialMessages(ctx context.Context, trigger Trigger, cfg db.ChatConfig) turnPrompt {
	roomID, sender := trigger.RoomID, trigger.Sender
	room := lookUpRoom(ctx, roomID)

	// The system prompt is ordered most stable first. Providers cache the longest unchanging prefix
	// of a request, so anything that varies between requests has to come after everything that does
	// not: the current time in particular used to sit in the second sentence, which changed the
	// prefix every second and made caching impossible. The system prompt now depends only on the
	// room, and everything that changes from turn to turn goes in the turn context after the history.
	system := systemPrompt(room.nameOf(ctx, config.UserID), room)

	// The history of the timeline the trigger was sent in: the main timeline, or its thread, which
	// begins with the turn it was started from if there is one
	turn := db.Turn{RoomID: roomID, EventID: trigger.EventID, ThreadRootID: trigger.ThreadRootID}
	seed := threadSeed(ctx, turn.Timeline(), sender)
	history := buildContextWindow(ctx, turn.Timeline(), seed, cfg.ContextHighTokens, cfg.ContextLowTokens)

	// The trigger, as the model sees it now and as the history keeps it. The first message of a
	// thread started on a stored turn refers to the turn's question or answer, which the history
	// already shows, so it isn't quoted again.
	startsSeededThread := trigger.ReplyToEventID != "" && trigger.ReplyToEventID == trigger.ThreadRootID && windowHasTurn(history, seed)
	refs := referencedMessages(ctx, trigger, room, !startsSeededThread)
	userTurn := db.UserTurn{
		Turn:         turn,
		UserID:       sender,
		SenderName:   room.nameOf(ctx, sender),
		SentAt:       trigger.Timestamp,
		Message:      trigger.Body,
		Mentions:     mentionsOf(ctx, trigger, room),
		UnseenBefore: trigger.UnseenBefore,
		HasImage:     trigger.Image != nil,
		ReplyTo:      refs.replyTo,
		Links:        refs.links,
	}

	// Build messages array with system prompt, history, and current message
	messages := []aigateway.Message{{Role: "system", Content: system}}
	composition := promptComposition{system: estimateTokens(system)}

	// Process history to include tool calls and tool responses
	historyStart := len(messages)
	processHistoryMessages(ctx, history, &messages)
	composition.history = estimateMessageTokens(messages[historyStart:])

	// Everything from here on belongs to this specific turn rather than to the replayed history, so
	// it is all accounted for as "current": the turn context and the user's message
	currentStart := len(messages)

	turnInfo := turnContext(time.Now(), person(userTurn.SenderName, sender),
		memoriesFor(ctx, sender, room), relevantMembers(room, userTurn, history), refs.notes)
	messages = append(messages, aigateway.Message{Role: "system", Content: turnInfo})

	// Select the appropriate model based on whether we have an image
	hasImage := len(refs.images) > 0
	model := cfg.TextModel
	if hasImage {
		model = cfg.ImageModel
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("model", model).
			Msg("Using image model for message with image")
	} else {
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("model", model).
			Msg("Using text model for message without image")
	}

	// Add the current message, with the images it carries or refers to attached if there are any
	content := renderUserTurn(userTurn, refs.attached)
	if hasImage {
		parts := []aigateway.ContentPart{{Type: "text", Text: content}}
		for _, imageDataURL := range refs.images {
			parts = append(parts, aigateway.ContentPart{Type: "image_url", ImageURL: &aigateway.ImageURL{
				URL:    imageDataURL,
				Detail: requestImageDetail(cfg.ImageDetail),
			}})
			metrics.RecordChatImageProcessed()
		}
		messages = append(messages, aigateway.Message{Role: "user", Content: parts})
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("image_detail", cfg.ImageDetail).
			Int("image_count", len(refs.images)).
			Msg("Attaching images to chat request")
	} else {
		messages = append(messages, aigateway.Message{Role: "user", Content: content})
	}
	composition.current = estimateMessageTokens(messages[currentStart:])

	return turnPrompt{
		messages:     messages,
		hasImage:     hasImage,
		model:        model,
		composition:  composition,
		userTurn:     userTurn,
		room:         room,
		threadSeeded: windowHasTurn(history, seed),
	}
}

// memoriesFor returns the memories of whoever is speaking that this room gets to see: all of them
// in a DM, only the ones saved here in a group room
func memoriesFor(ctx context.Context, sender string, room roomInfo) []db.UserMemory {
	memories, err := db.GetMemoriesIn(ctx, sender, room.memoryView())
	if err != nil {
		// Already logged. Continue without memories.
		return nil
	}
	return memories
}

// processHistoryMessages processes the chat history and adds it to the messages array.
//
// History arrives in chronological order and is replayed in that order: a batch of tool calls is
// emitted as one assistant message followed by the tool responses answering it, in the position
// where it actually happened. Interleaving matters — replaying text and tool calls in separate
// groups presents the model with a conversation that never took place.
//
// A user message is replayed with its header, exactly as it was shown when it was the current turn.
func processHistoryMessages(ctx context.Context, history []db.ChatMessage, messages *[]aigateway.Message) {
	for i := 0; i < len(history); {
		historyMsg := history[i]

		switch historyMsg.MessageType {
		case "tool_call":
			// Collect the run of calls making up this batch, then the responses answering them,
			// which the writer stores right after the calls in the same transaction
			calls, next := collectToolCalls(history, i)
			responses, next := collectToolResponses(history, next)

			*messages = append(*messages, aigateway.Message{
				Role:      "assistant",
				Content:   "", // Content must be empty when there are tool calls
				ToolCalls: calls,
			})
			for _, call := range calls {
				*messages = append(*messages, aigateway.Message{
					Role:       "tool",
					Content:    responses[call.ID],
					ToolCallID: call.ID,
				})
			}

			i = next
		case "tool_response":
			// A response whose call is not in the window: the read cap cut between the two, so
			// there is nothing to attach it to
			i++
		case "text":
			*messages = append(*messages, aigateway.Message{
				Role:    historyMsg.Role,
				Content: replayText(historyMsg),
			})
			i++
		}
	}
}

// collectToolCalls reads the run of consecutive tool_call rows starting at start, returning them
// and the index of the first row that follows
func collectToolCalls(history []db.ChatMessage, start int) ([]aigateway.ToolCall, int) {
	var calls []aigateway.ToolCall
	i := start
	for ; i < len(history) && history[i].MessageType == "tool_call"; i++ {
		// Both are set on every tool row, which the chat_history_tool_row_complete constraint
		// guarantees
		calls = append(calls, aigateway.ToolCall{
			ID:   *history[i].ToolCallID,
			Type: "function",
			Function: aigateway.ToolFunction{
				Name:      *history[i].ToolName,
				Arguments: history[i].Message,
			},
		})
	}
	return calls, i
}

// collectToolResponses reads the run of consecutive tool_response rows starting at start, returning
// them keyed by tool call id and the index of the first row that follows.
//
// An expired result is replaced by a marker rather than dropped. Removing it would delete the reply
// to a tool call that is still in the history, and reshape the conversation behind the model's back
// mid-thread; the marker keeps the structure intact while making it plain that the data is stale.
func collectToolResponses(history []db.ChatMessage, start int) (map[string]string, int) {
	responses := make(map[string]string)
	now := time.Now()
	i := start
	for ; i < len(history) && history[i].MessageType == "tool_response"; i++ {
		responses[*history[i].ToolCallID] = replayableToolResponse(history[i], now)
	}
	return responses, i
}

// maxReplayedToolResponseBytes caps how much of a stored tool result is replayed as history. The
// turn that fetched the data sees it in full; later turns only need the gist, and a single web fetch
// can otherwise occupy a large share of the context window for as long as it stays in scope.
const maxReplayedToolResponseBytes = 4096

// replayableToolResponse returns the content to replay for a stored tool response, substituting a
// marker once the result has expired and truncating results too large to be worth replaying whole
func replayableToolResponse(msg db.ChatMessage, now time.Time) string {
	// Set on every tool row, which the chat_history_tool_row_complete constraint guarantees
	toolName := *msg.ToolName

	if msg.Expiry != nil && !msg.Expiry.After(now) {
		return fmt.Sprintf("[expired: the %s result from this point in the conversation is no longer "+
			"current, call the tool again if you need this information]", toolName)
	}

	return truncateForReplay(msg.Message, toolName)
}

// truncateForReplay shortens an oversized tool result, cutting on a rune boundary so the result
// stays valid UTF-8, and says plainly that it was shortened rather than appearing to end mid-thought
func truncateForReplay(response, toolName string) string {
	if len(response) <= maxReplayedToolResponseBytes {
		return response
	}

	cut := maxReplayedToolResponseBytes
	for cut > 0 && !utf8.RuneStart(response[cut]) {
		cut--
	}

	return response[:cut] + fmt.Sprintf("\n\n[truncated: the full %s result is not replayed in full, "+
		"call the tool again if you need the rest]", toolName)
}

// mentionsOf returns who the trigger mentions, besides the bot, with the names they have in the
// room. m.mentions is the record if the message has one; for clients that don't send it, the pills
// in the formatted body are.
//
// A client adds the sender of the message being replied to into m.mentions as well, so that they
// get notified. The reply already says who that is, so they only count as mentioned if they are
// pilled as well.
func mentionsOf(ctx context.Context, trigger Trigger, room roomInfo) []db.Mention {
	pilled := matrix.PillUserIDs(trigger.FormattedBody)
	userIDs := trigger.Mentions
	if userIDs == nil {
		userIDs = pilled
	}

	var replyAuthor string
	if trigger.ReplyTo != nil {
		replyAuthor = trigger.ReplyTo.Sender
	}

	var mentions []db.Mention
	seen := make(map[string]bool)
	for _, userID := range userIDs {
		if userID == config.UserID || seen[userID] {
			continue
		}
		if userID == replyAuthor && !slices.Contains(pilled, userID) {
			continue
		}
		seen[userID] = true
		mentions = append(mentions, db.Mention{UserID: userID, Name: room.nameOf(ctx, userID)})
	}
	return mentions
}

// extractAssistantResponse extracts the assistant's response from the API response. A response
// without any text is a failure, returned as the "empty_response" outcome; the outcome is empty
// when there is an answer.
func extractAssistantResponse(ctx context.Context, roomID, sender, model string, hasImage bool, chatResp *aigateway.ChatResponse) (string, string) {
	var assistantResponse string

	if content, ok := chatResp.Choices[0].Message.Content.(string); ok {
		assistantResponse = content
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("sender", sender).
			Str("model", model).
			Bool("has_image", hasImage).
			Int("response_length", len(assistantResponse)).
			Msg("Received string response from the AI Gateway")
	} else if contentMap, ok := chatResp.Choices[0].Message.Content.(map[string]any); ok {
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("sender", sender).
			Str("model", model).
			Bool("has_image", hasImage).
			Interface("content_map", contentMap).
			Msg("Received map response from the AI Gateway")

		if text, ok := contentMap["text"].(string); ok {
			assistantResponse = text
		} else {
			log.Warn().Ctx(ctx).
				Str("room_id", roomID).
				Str("sender", sender).
				Str("model", model).
				Bool("has_image", hasImage).
				Interface("content_map", contentMap).
				Msg("Response content map doesn't contain text field")
		}
	} else {
		log.Warn().Ctx(ctx).
			Str("room_id", roomID).
			Str("sender", sender).
			Str("model", model).
			Bool("has_image", hasImage).
			Interface("content", chatResp.Choices[0].Message.Content).
			Msg("Unexpected response content type")
	}

	// Posted, an empty answer would be an empty message in the room. A model can finish without
	// any text, for example when it spends its whole token budget reasoning.
	if strings.TrimSpace(assistantResponse) == "" {
		log.Warn().Ctx(ctx).
			Str("room_id", roomID).
			Str("sender", sender).
			Str("model", model).
			Bool("has_image", hasImage).
			Str("finish_reason", chatResp.Choices[0].FinishReason).
			Msg("Chat API returned no answer text")
		return "", "empty_response"
	}
	return assistantResponse, ""
}

// processToolCalls handles the iterative tool calling process.
//
// Returns the iteration count, the updated messages, and the final response the answer is read
// from. If the turn fails along the way, the response is nil and the failure outcome says why;
// the outcome is empty otherwise.
func processToolCalls(
	ctx, toolCtx context.Context,
	turn db.Turn,
	sender, model string,
	hasImage bool,
	cfg db.ChatConfig,
	chatResp *aigateway.ChatResponse,
	messages []aigateway.Message,
	tools []aigateway.ToolDefinition,
	stats *turnStats,
) (int, []aigateway.Message, *aigateway.ChatResponse, string) {
	roomID := turn.RoomID

	// Keep calling tools until the model stops asking for them or the configured iteration limit
	// is reached
	currentResp := chatResp
	iterationCount := 1 // by the time we're here, we've already made one request

	maxTokens := cfg.MaxTokens
	maxIterations := cfg.MaxToolIterations
	for iterationCount < maxIterations {
		iterationCount++

		// Add the assistant's message with tool calls
		messages = append(messages, aigateway.Message{
			Role:      "assistant",
			Content:   "", // Content should be empty when there are tool calls
			ToolCalls: currentResp.Choices[0].Message.ToolCalls,
		})

		// Process tool calls
		toolStart := time.Now()
		toolResponses, err := toolRegistry.HandleToolCallsIndividually(toolCtx, currentResp.Choices[0].Message.ToolCalls)
		stats.recordToolExecution(currentResp.Choices[0].Message.ToolCalls, time.Since(toolStart))
		if err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Str("model", model).
				Bool("has_image", hasImage).
				Int("iteration", iterationCount).
				Msg("Failed to handle tool calls")
			return iterationCount, messages, nil, "tool_failed"
		}

		// Persist the calls together with their responses, after execution, so a call is never
		// stored without the response that answers it
		saveToolCallHistory(ctx, turn, currentResp.Choices[0].Message.ToolCalls, toolResponses, tools)

		// Add each tool response as a separate message
		for _, toolResp := range toolResponses {
			messages = append(messages, aigateway.Message{
				Role:       "tool",
				Content:    toolResp.Response,
				ToolCallID: toolResp.ToolCallID,
			})
		}

		// Update the request with the new messages
		req := aigateway.ChatRequest{
			Model:     model,
			Messages:  messages,
			Tools:     tools,
			MaxTokens: &maxTokens,
			Metadata:  gatewayMetadata(roomID, sender, iterationCount),
		}

		// Log the request for debugging
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("sender", sender).
			Str("model", model).
			Bool("has_image", hasImage).
			Int("message_count", len(messages)).
			Int("iteration", iterationCount).
			Msg("Sending chat request for tool iteration")

		// Send the next request to the AI Gateway
		callStart := time.Now()
		nextResp, err := aigateway.SendChatRequest(ctx, req)
		stats.recordModelCall(nextResp, time.Since(callStart))
		if err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Str("model", model).
				Bool("has_image", hasImage).
				Int("iteration", iterationCount).
				Msg("Failed to send chat request for tool iteration")
			return iterationCount, messages, nil, requestFailure(ctx)
		} else if len(nextResp.Choices) == 0 {
			log.Error().Ctx(ctx).
				Str("room_id", roomID).
				Str("model", model).
				Bool("has_image", hasImage).
				Int("iteration", iterationCount).
				Msg("Chat API returned no choices for tool iteration")
			return iterationCount, messages, nil, "no_choices"
		}

		// Update the current response for the next iteration
		currentResp = nextResp

		// Check if the model wants to use more tools
		if currentResp.Choices[0].FinishReason != "tool_calls" || len(currentResp.Choices[0].Message.ToolCalls) == 0 {
			// No more tool calls, we have our final response
			break
		}

		// Log that we're continuing with another tool iteration
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("sender", sender).
			Str("model", model).
			Bool("has_image", hasImage).
			Int("tool_calls", len(currentResp.Choices[0].Message.ToolCalls)).
			Int("iteration", iterationCount).
			Msg("Model requested additional tool calls")
	}

	// After iterations are complete or max iterations reached, get the final response
	if currentResp.Choices[0].FinishReason == "tool_calls" && iterationCount >= maxIterations {
		// We hit the maximum number of iterations but the model still wants to use tools
		// Make one final request without tools to get a text response
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("sender", sender).
			Str("model", model).
			Bool("has_image", hasImage).
			Int("iteration", iterationCount).
			Msg("Reached maximum tool iterations, making final request without tools")

		// Add the last assistant message with tool calls
		messages = append(messages, aigateway.Message{
			Role:      "assistant",
			Content:   "", // Content should be empty when there are tool calls
			ToolCalls: currentResp.Choices[0].Message.ToolCalls,
		})

		// Process the final tool calls
		toolStart := time.Now()
		toolResponses, err := toolRegistry.HandleToolCallsIndividually(toolCtx, currentResp.Choices[0].Message.ToolCalls)
		stats.recordToolExecution(currentResp.Choices[0].Message.ToolCalls, time.Since(toolStart))
		if err != nil {
			// The calls are already in the messages, and a request with calls nobody answered
			// would only be rejected
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Str("model", model).
				Bool("has_image", hasImage).
				Int("iteration", iterationCount).
				Msg("Failed to handle tool calls")
			return iterationCount, messages, nil, "tool_failed"
		}

		// Persist the calls together with their responses, after execution, so a call is never
		// stored without the response that answers it
		saveToolCallHistory(ctx, turn, currentResp.Choices[0].Message.ToolCalls, toolResponses, tools)

		// Add each tool response as a separate message
		for _, toolResp := range toolResponses {
			messages = append(messages, aigateway.Message{
				Role:       "tool",
				Content:    toolResp.Response,
				ToolCallID: toolResp.ToolCallID,
			})
		}

		// Final request without tools
		req := aigateway.ChatRequest{
			Model:     model,
			Messages:  messages,
			MaxTokens: &maxTokens,
			Metadata:  gatewayMetadata(roomID, sender, iterationCount),
		}

		// Log the final request
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("sender", sender).
			Str("model", model).
			Bool("has_image", hasImage).
			Int("message_count", len(messages)).
			Msg("Sending final request without tools")

		// Send the final request to the AI Gateway
		callStart := time.Now()
		finalResp, err := aigateway.SendChatRequest(ctx, req)
		stats.recordModelCall(finalResp, time.Since(callStart))
		if err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Str("model", model).
				Bool("has_image", hasImage).
				Msg("Failed to send final request")
			return iterationCount, messages, nil, requestFailure(ctx)
		} else if len(finalResp.Choices) == 0 {
			log.Error().Ctx(ctx).
				Str("room_id", roomID).
				Str("model", model).
				Bool("has_image", hasImage).
				Msg("Final chat API returned no choices")
			return iterationCount, messages, nil, "no_choices"
		}

		currentResp = finalResp
	}

	return iterationCount, messages, currentResp, ""
}

// saveToolCallHistory persists a batch of tool calls together with the responses they produced.
//
// Both halves are written in one transaction after the tools have run. Writing the calls first and
// the responses later leaves a window — a failed write, a cancelled context, a restart — in which
// history holds a call with no response, which the chat API then rejects on every later request in
// the room.
func saveToolCallHistory(ctx context.Context, turn db.Turn, toolCalls []aigateway.ToolCall, toolResponses []aigateway.ToolResponse, tools []aigateway.ToolDefinition) {
	if len(toolCalls) == 0 {
		return
	}

	responsesByID := make(map[string]string, len(toolResponses))
	for _, toolResp := range toolResponses {
		responsesByID[toolResp.ToolCallID] = toolResp.Response
	}

	validityByName := make(map[string]time.Duration, len(tools))
	for _, tool := range tools {
		validityByName[tool.Function.Name] = tool.ValidityDuration
	}

	records := make([]db.ToolCallRecord, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		response, ok := responsesByID[toolCall.ID]
		if !ok {
			// No response means nothing to pair the call with, so storing it would recreate the
			// orphan this function exists to prevent
			log.Warn().Ctx(ctx).
				Str("room_id", turn.RoomID).
				Str("tool_call_id", toolCall.ID).
				Str("tool_name", toolCall.Function.Name).
				Msg("Skipping tool call with no response when saving history")
			continue
		}

		records = append(records, db.ToolCallRecord{
			ToolCallID:       toolCall.ID,
			ToolName:         toolCall.Function.Name,
			Arguments:        toolCall.Function.Arguments,
			Response:         response,
			ValidityDuration: validityByName[toolCall.Function.Name],
		})
	}

	// The transaction is atomic, so history is left consistent even if saving fails
	persist(ctx, turn, "tool_calls", func(ctx context.Context) error {
		return db.SaveToolCallsWithResponses(ctx, turn, config.UserID, records)
	})
}

// buildDebugData creates debug data for the response
func buildDebugData(ctx context.Context, model string, messages []aigateway.Message, iterationCount int) map[string]any {
	debugData := map[string]any{
		"model":                model,
		"prompt_message_count": len(messages),
	}

	// The trace id makes a reply self-describing: open its source, copy the id, and the whole turn
	// can be pulled up in Tempo. It also finds Cloudflare's separate spans, which carry it as
	// siikabot_trace_id, so one value reaches both halves of the picture.
	if traceID := tracing.TraceID(ctx); traceID != "" {
		debugData["trace_id"] = traceID
	}

	// Add tool calls information if any were made during the current processing
	if iterationCount > 0 {
		// Find the index of the last user message
		lastUserMsgIndex := -1
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "user" {
				lastUserMsgIndex = i
				break
			}
		}

		// Only include tool calls that happened after the last user message
		toolCalls := make(map[string][]map[string]any)
		currentIteration := 1

		for i, msg := range messages {
			// Only process messages that come after the last user message
			if i > lastUserMsgIndex && msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
				currentIterationStr := fmt.Sprintf("iteration_%d", currentIteration)
				// Create a slice for this iteration if it doesn't exist
				if _, exists := toolCalls[currentIterationStr]; !exists {
					toolCalls[currentIterationStr] = []map[string]any{}
				}

				// Add all tool calls for this iteration
				for _, toolCall := range msg.ToolCalls {
					// Parse arguments as JSON if possible
					var args map[string]any
					if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
						// If parsing fails, use the raw string
						args = map[string]any{"raw": toolCall.Function.Arguments}
					}

					toolCalls[currentIterationStr] = append(
						toolCalls[currentIterationStr],
						map[string]any{
							"name": toolCall.Function.Name,
							"args": args,
						},
					)
				}

				// Move to the next iteration
				currentIteration++
			}
		}

		// Only add tool_calls if there are any
		if len(toolCalls) > 0 {
			debugData["tool_calls"] = toolCalls
		}

		debugData["tool_iterations"] = iterationCount
	}

	return debugData
}
