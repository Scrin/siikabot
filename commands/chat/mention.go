package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/Scrin/siikabot/metrics"
	"github.com/rs/zerolog/log"
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

// HandleMention handles the chat command
func HandleMention(ctx context.Context, roomID, sender, msg, eventID string, relatesTo map[string]any) {
	if strings.TrimSpace(msg) == "" {
		return
	}

	// Serialise turns per room. A queued message waits here, so its own timer starts once it is
	// actually being worked on rather than while it waits.
	unlockRoom := lockRoom(roomID)
	defer unlockRoom()

	// Bound the whole turn, so a stuck model or a long tool loop fails in a knowable time instead
	// of grinding on invisibly. Applied after the lock so queueing does not eat into the budget.
	ctx, cancelTurn := context.WithTimeout(ctx, turnTimeout)
	defer cancelTurn()

	startTime := time.Now()

	log.Debug().Ctx(ctx).
		Str("room_id", roomID).
		Str("sender", sender).
		Str("chat_msg", msg).
		Msg("Processing chat command")

	// Variable to track tool iterations
	iterationCount := 0

	// Keep the typing indicator alive for as long as the turn actually runs
	stopTyping := startTypingIndicator(ctx, roomID)
	defer stopTyping()

	// Accumulates everything that happens during this turn, for the summary logged at the end
	stats := &turnStats{outcome: "ok"}

	// Build the initial messages with system prompt, history, and handle image if present
	messages, hasImage, model, composition := buildInitialMessages(ctx, roomID, sender, msg, relatesTo)

	// Get tool definitions from the registry, filtering based on user permissions
	tools := getToolsForUser(ctx, sender)

	composition.tools = estimateToolDefinitionTokens(tools)
	composition.record(ctx)

	// Cap the response length. Read once and threaded through the turn so a multi-iteration turn
	// does not re-query it per request.
	maxTokens := getMaxTokensForRoom(ctx, roomID)

	// Create the initial request
	req := aigateway.ChatRequest{
		Model:     model,
		Messages:  messages,
		Tools:     tools,
		MaxTokens: &maxTokens,
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
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			stats.outcome = "turn_timeout"
			stats.finish(ctx, roomID, sender, model, hasImage, time.Since(startTime))
			metrics.RecordChatRequestDuration(model, hasImage, time.Since(startTime).Seconds())
			matrix.SendMessage(roomID, "That took too long to answer, so I gave up. Try again, or ask something narrower.")
			return
		}
		stats.outcome = "request_failed"
		stats.finish(ctx, roomID, sender, model, hasImage, time.Since(startTime))
		metrics.RecordChatRequestDuration(model, hasImage, time.Since(startTime).Seconds())
		matrix.SendMessage(roomID, "Failed to process chat request")
		return
	}

	if len(chatResp.Choices) == 0 {
		log.Error().Ctx(ctx).
			Str("room_id", roomID).
			Str("sender", sender).
			Str("model", model).
			Bool("has_image", hasImage).
			Msg("Chat API returned no choices")
		stats.outcome = "no_choices"
		stats.finish(ctx, roomID, sender, model, hasImage, time.Since(startTime))
		metrics.RecordChatRequestDuration(model, hasImage, time.Since(startTime).Seconds())
		matrix.SendMessage(roomID, "No response from chat API")
		return
	}

	// Skipped for image turns: an image costs a number of tokens that bears no relation to the length
	// of its data URI, so comparing them would only measure something the estimator never modelled
	if chatResp.Usage != nil && !hasImage {
		recordTokenEstimateDrift(ctx, model, estimatedPromptTokens, chatResp.Usage.PromptTokens)
	}

	// Save the user message to history
	if err := db.SaveChatMessage(ctx, roomID, sender, msg, "user"); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to save user message to history")
		// Continue even if saving fails
	}

	// Get the assistant's response
	assistantResponse := extractAssistantResponse(ctx, roomID, sender, model, hasImage, chatResp)

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
		iterationCount, messages, assistantResponse = processToolCalls(
			ctx, roomID, sender, model, hasImage, maxTokens,
			chatResp, messages, tools, stats,
		)
	}
	stats.iterations = iterationCount

	// Save the assistant response to history, on a context that outlives a cancelled turn
	persistCtx, cancelPersist := persistContext(ctx)
	defer cancelPersist()
	if err := db.SaveChatMessage(persistCtx, roomID, config.UserID, assistantResponse, "assistant"); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to save assistant message to history")
		// Continue even if saving fails
	}

	stats.finish(ctx, roomID, sender, model, hasImage, time.Since(startTime))

	metrics.RecordChatRequestDuration(model, hasImage, time.Since(startTime).Seconds())
	metrics.RecordChatToolIterations(iterationCount)

	// Create debug data with model info and tool calls
	debugData := buildDebugData(model, messages, iterationCount)

	matrix.SendMarkdownFormattedNoticeWithDebugData(roomID, assistantResponse, debugData)
}

// buildInitialMessages creates the initial messages array with system prompt, history, and user
// message, along with the estimated token cost of each part of it
func buildInitialMessages(ctx context.Context, roomID, sender, msg string, relatesTo map[string]any) ([]aigateway.Message, bool, string, promptComposition) {
	// Get the bot's actual display name from the Matrix server
	botDisplayName := matrix.GetDisplayName(ctx, config.UserID)
	if botDisplayName == "" {
		// Fallback to user ID if display name can't be retrieved
		botDisplayName = strings.Split(config.UserID, ":")[0][1:] // Remove @ and domain part
	}

	// The system prompt is ordered most stable first. Providers cache the longest unchanging prefix
	// of a request, so anything that varies between requests has to come after everything that does
	// not: the current time in particular used to sit in the second sentence, which changed the
	// prefix every second and made caching impossible. It is now appended after the history instead.
	// The instruction to batch tool calls is the cheapest latency win available: each tool iteration
	// is a separate round trip carrying the whole conversation, so three facts fetched one at a time
	// cost three of them where one would do. Both supported providers can emit several tool calls in
	// a single turn, and they are executed in parallel.
	systemPrompt := fmt.Sprintf(
		"You are %s, a helpful Matrix bot. "+
			"Keep your responses concise and helpful. Use markdown formatting in your responses. "+
			"When you need several independent pieces of information, request all of the tool calls "+
			"together in one turn rather than one at a time.",
		botDisplayName,
	)

	// Fetch and append user memories to system prompt
	memories, err := db.GetUserMemories(ctx, sender)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", sender).Msg("Failed to get user memories")
		// Continue without memories if there's an error
	} else if len(memories) > 0 {
		systemPrompt += "\n\n## User Memories\nThe following things have been remembered about this user:\n"
		for _, mem := range memories {
			systemPrompt += fmt.Sprintf("- [ID: %d] %s\n", mem.ID, mem.Memory)
		}
		systemPrompt += "\nUse the memory tool to save new memories or manage existing ones when the user asks you to remember or forget something."
	}

	// Fetch and append user Grafana datasources to system prompt (only if authorized)
	if db.IsGrafanaAuthorized(ctx, sender) {
		datasources, err := db.GetUserGrafanaDatasources(ctx, sender)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).Str("user_id", sender).Msg("Failed to get user grafana datasources")
			// Continue without datasources if there's an error
		} else if len(datasources) > 0 {
			systemPrompt += "\n\n## User's Grafana Datasources\nThis user has the following custom data sources you can query:\n"
			for _, ds := range datasources {
				systemPrompt += fmt.Sprintf("- %s: %s\n", ds.Name, ds.Description)
			}
			systemPrompt += "\nUse the user_grafana tool with action 'query' and the datasource name to fetch current values."
		}
	}

	// Get the conversation history making up the current context window
	history := buildContextWindow(ctx, roomID)

	// Build messages array with system prompt, history, and current message
	messages := []aigateway.Message{{Role: "system", Content: systemPrompt}}
	composition := promptComposition{system: estimateTokens(systemPrompt)}

	// Process history to include tool calls and tool responses
	historyStart := len(messages)
	processHistoryMessages(ctx, history, &messages)
	composition.history = estimateMessageTokens(messages[historyStart:])

	// Everything from here on belongs to this specific turn rather than to the replayed history, so
	// it is all accounted for as "current": the timestamp, any reply context, and the user's message
	currentStart := len(messages)

	// The current time goes after the history, not in the system prompt, so that everything before
	// it stays byte-identical between requests and can be served from the provider's prompt cache
	loc, _ := time.LoadLocation(config.Timezone)
	messages = append(messages, aigateway.Message{
		Role:    "system",
		Content: "The current date and time is " + time.Now().In(loc).Format("Monday, January 2, 2006 15:04:05 MST"),
	})

	// Flag to track if we're handling an image
	hasImage := false
	var base64ImageURL string

	// Check if this message is a reply to another message
	if relatesTo != nil {
		base64ImageURL = processRelatedMessage(ctx, roomID, relatesTo, &messages)
		if base64ImageURL != "" {
			hasImage = true
		}
	}

	// Select the appropriate model based on whether we have an image
	var model string
	if hasImage {
		model = getImageModelForRoom(ctx, roomID)
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("model", model).
			Msg("Using image model for message with image")
	} else {
		model = getTextModelForRoom(ctx, roomID)
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("model", model).
			Msg("Using text model for message without image")
	}

	// Add the current message, handling image if present
	if hasImage {
		hasImage, messages = processImageMessage(ctx, roomID, msg, base64ImageURL, &messages)
	} else {
		// Regular text message
		messages = append(messages, aigateway.Message{Role: "user", Content: msg})
	}
	composition.current = estimateMessageTokens(messages[currentStart:])

	return messages, hasImage, model, composition
}

// processHistoryMessages processes the chat history and adds it to the messages array.
//
// History arrives in chronological order and is replayed in that order: a batch of tool calls is
// emitted as one assistant message followed by the tool responses answering it, in the position
// where it actually happened. Interleaving matters — replaying text and tool calls in separate
// groups presents the model with a conversation that never took place.
//
// Tool calls with no matching response are dropped. Such a pair should never be written now that
// they are persisted atomically, but history predating that change can contain them, and the chat
// API rejects an assistant message whose tool_calls are not all answered — which would otherwise
// break every request in the room until the rows expired.
func processHistoryMessages(ctx context.Context, history []db.ChatMessage, messages *[]aigateway.Message) {
	for i := 0; i < len(history); {
		historyMsg := history[i]

		switch historyMsg.MessageType {
		case "tool_call":
			// Collect the consecutive run of calls making up this batch, then the responses
			// answering them, which the writer always stores immediately afterwards
			calls, next := collectToolCalls(history, i)
			responses, next := collectToolResponses(history, next)

			answered := make([]aigateway.ToolCall, 0, len(calls))
			for _, call := range calls {
				if _, ok := responses[call.ID]; ok {
					answered = append(answered, call)
					continue
				}
				log.Warn().Ctx(ctx).
					Str("room_id", historyMsg.RoomID).
					Str("tool_call_id", call.ID).
					Str("tool_name", call.Function.Name).
					Msg("Dropping orphaned tool call from chat history")
			}

			if len(answered) > 0 {
				*messages = append(*messages, aigateway.Message{
					Role:      "assistant",
					Content:   "", // Content must be empty when there are tool calls
					ToolCalls: answered,
				})
				for _, call := range answered {
					*messages = append(*messages, aigateway.Message{
						Role:       "tool",
						Content:    responses[call.ID],
						ToolCallID: call.ID,
					})
				}
			}

			i = next
		case "tool_response":
			// A response whose call is not in the window, so there is nothing to attach it to
			i++
		default: // "text", or empty for rows predating the message_type column
			*messages = append(*messages, aigateway.Message{
				Role:    historyMsg.Role,
				Content: historyMsg.Message,
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
		if history[i].ToolCallID == nil || history[i].ToolName == nil {
			continue
		}
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
		if history[i].ToolCallID == nil {
			continue
		}
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
	toolName := "tool"
	if msg.ToolName != nil {
		toolName = *msg.ToolName
	}

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

// processRelatedMessage handles messages that are replies to other messages
// Returns base64ImageURL if the message is a reply to an image
func processRelatedMessage(ctx context.Context, roomID string, relatesTo map[string]any, messages *[]aigateway.Message) string {
	log.Debug().Ctx(ctx).
		Str("room_id", roomID).
		Interface("relates_to", relatesTo).
		Msg("Message has relation information")

	// Check for m.in_reply_to
	if inReplyTo, ok := relatesTo["m.in_reply_to"].(map[string]any); ok {
		if replyEventID, ok := inReplyTo["event_id"].(string); ok {
			log.Debug().Ctx(ctx).
				Str("room_id", roomID).
				Str("reply_event_id", replyEventID).
				Msg("Message is a reply to another message")

			// Check if the replied-to message is an image
			msgType, err := matrix.GetEventType(ctx, roomID, replyEventID)
			if err != nil {
				log.Error().Ctx(ctx).Err(err).
					Str("room_id", roomID).
					Str("event_id", replyEventID).
					Msg("Failed to get replied-to message type")
				return ""
			}

			if msgType == "m.image" {
				return processRepliedImage(ctx, roomID, replyEventID, messages)
			} else {
				processRepliedText(ctx, roomID, replyEventID, messages)
			}
		}
	}
	return ""
}

// processRepliedImage handles replies to image messages
// Returns the base64 encoded image URL if successful
func processRepliedImage(ctx context.Context, roomID, replyEventID string, messages *[]aigateway.Message) string {
	// Get the image URL, encryption info, and full content
	imageURL, encryptionInfo, fullContent, err := matrix.GetEventImageURL(ctx, roomID, replyEventID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Str("event_id", replyEventID).
			Msg("Failed to get image URL from replied-to message")
		return ""
	}

	// Download the image and convert to base64
	base64ImageURL, err := matrix.DownloadImageAsBase64(ctx, imageURL, encryptionInfo, fullContent)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Str("image_url", imageURL).
			Bool("is_encrypted", encryptionInfo != nil).
			Msg("Failed to download and convert image to base64")

		// Add a note about the failed attempt to process the image
		errorMsg := "Note: The user replied to an image, but I couldn't process it. Please make sure the image is accessible and try again."

		*messages = append(*messages, aigateway.Message{
			Role:    "system",
			Content: errorMsg,
		})
		return ""
	}

	log.Debug().Ctx(ctx).
		Str("room_id", roomID).
		Str("event_id", replyEventID).
		Str("image_url", imageURL).
		Bool("is_encrypted", encryptionInfo != nil).
		Msg("Message is a reply to an image")

	return base64ImageURL
}

// processRepliedText handles replies to text messages
func processRepliedText(ctx context.Context, roomID, replyEventID string, messages *[]aigateway.Message) {
	// Get the content of the replied-to message (text)
	repliedToContent, err := matrix.GetEventContent(ctx, roomID, replyEventID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Str("event_id", replyEventID).
			Msg("Failed to get replied-to message content")

		// Add a note about the failed attempt to get the replied-to message
		*messages = append(*messages, aigateway.Message{
			Role:    "system",
			Content: "Note: This message is a reply to another message, but I couldn't retrieve the content of that message.",
		})
		return
	}

	if repliedToContent != "" {
		// Add the replied-to message to the conversation
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("event_id", replyEventID).
			Str("content", repliedToContent).
			Msg("Including replied-to message in conversation")

		// Add a note about the reply context
		replyContextMsg := fmt.Sprintf("This message is a reply to: \"%s\"", repliedToContent)
		*messages = append(*messages, aigateway.Message{
			Role:    "system",
			Content: replyContextMsg,
		})
	}
}

// processImageMessage handles messages that include an image
// Returns updated hasImage flag and messages
func processImageMessage(ctx context.Context, roomID, msg, base64ImageURL string, messages *[]aigateway.Message) (bool, []aigateway.Message) {
	hasImage := true

	// Ensure the base64ImageURL is properly formatted
	if !strings.HasPrefix(base64ImageURL, "data:image/") {
		// Log a prefix of the URL for debugging, but be careful of index out of range
		urlPrefix := base64ImageURL
		if len(base64ImageURL) > 30 {
			urlPrefix = base64ImageURL[:30] + "..."
		}

		log.Warn().Ctx(ctx).
			Str("room_id", roomID).
			Str("base64_url_prefix", urlPrefix).
			Msg("Image URL is not properly formatted, attempting to fix")

		// Try to extract the content type and base64 data
		if strings.Contains(base64ImageURL, ";base64,") {
			parts := strings.SplitN(base64ImageURL, ";base64,", 2)
			if len(parts) == 2 {
				contentType := parts[0]
				if !strings.HasPrefix(contentType, "data:") {
					contentType = "data:" + contentType
				}
				if !strings.HasPrefix(contentType, "data:image/") {
					contentType = "data:image/png"
				}
				base64Data := parts[1]
				base64ImageURL = contentType + ";base64," + base64Data

				// Log a prefix of the fixed URL for debugging, but be careful of index out of range
				fixedUrlPrefix := base64ImageURL
				if len(base64ImageURL) > 30 {
					fixedUrlPrefix = base64ImageURL[:30] + "..."
				}

				log.Debug().Ctx(ctx).
					Str("room_id", roomID).
					Str("fixed_url_prefix", fixedUrlPrefix).
					Msg("Fixed image URL format")
			}
		}
	}

	// Check if the base64 image URL is too large (>5MB)
	parts := strings.SplitN(base64ImageURL, ";base64,", 2)
	if len(parts) == 2 {
		// Calculate approximate size of the decoded data
		// Base64 encoding increases size by ~33%, so we can estimate the decoded size
		base64Data := parts[1]
		estimatedSize := len(base64Data) * 3 / 4 // Approximate size after decoding

		// 5MB = 5 * 1024 * 1024 bytes
		const maxSizeBytes = 5 * 1024 * 1024

		if estimatedSize > maxSizeBytes {
			log.Warn().Ctx(ctx).
				Str("room_id", roomID).
				Int("estimated_size_bytes", estimatedSize).
				Int("max_size_bytes", maxSizeBytes).
				Msg("Image is too large, skipping image attachment")

			// Add a note about the image being too large
			*messages = append(*messages, aigateway.Message{
				Role:    "system",
				Content: "Note: An image was attached to this message, but it was too large to process (>5MB).",
			})

			// Fall back to text-only message
			*messages = append(*messages, aigateway.Message{Role: "user", Content: msg})
			hasImage = false
		} else {
			detail := getImageDetailForRoom(ctx, roomID)
			contentParts := []aigateway.ContentPart{
				{
					Type: "text",
					Text: msg,
				},
				{
					Type: "image_url",
					ImageURL: &aigateway.ImageURL{
						URL:    base64ImageURL,
						Detail: detail,
					},
				},
			}

			*messages = append(*messages, aigateway.Message{
				Role:    "user",
				Content: contentParts,
			})
			log.Debug().Ctx(ctx).
				Str("room_id", roomID).
				Str("image_detail", detail).
				Msg("Attaching image to chat request")
			metrics.RecordChatImageProcessed()
		}
	} else {
		log.Error().Ctx(ctx).
			Str("room_id", roomID).
			Msg("Image URL does not contain valid base64 data, skipping image")

		// Fall back to text-only message
		*messages = append(*messages, aigateway.Message{Role: "user", Content: msg})
		hasImage = false
	}

	return hasImage, *messages
}

// extractAssistantResponse extracts the assistant's response from the API response
func extractAssistantResponse(ctx context.Context, roomID, sender, model string, hasImage bool, chatResp *aigateway.ChatResponse) string {
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
			assistantResponse = "I processed your image, but couldn't generate a proper response."
			log.Warn().Ctx(ctx).
				Str("room_id", roomID).
				Str("sender", sender).
				Str("model", model).
				Bool("has_image", hasImage).
				Interface("content_map", contentMap).
				Msg("Response content map doesn't contain text field")
		}
	} else {
		assistantResponse = "I processed your image, but couldn't generate a proper response."
		log.Warn().Ctx(ctx).
			Str("room_id", roomID).
			Str("sender", sender).
			Str("model", model).
			Bool("has_image", hasImage).
			Interface("content", chatResp.Choices[0].Message.Content).
			Msg("Unexpected response content type")
	}

	return assistantResponse
}

// processToolCalls handles the iterative tool calling process
// Returns the iteration count, updated messages, and final assistant response
func processToolCalls(
	ctx context.Context,
	roomID, sender, model string,
	hasImage bool,
	maxTokens int,
	chatResp *aigateway.ChatResponse,
	messages []aigateway.Message,
	tools []aigateway.ToolDefinition,
	stats *turnStats,
) (int, []aigateway.Message, string) {
	// Implement iterative tool calling with a maximum of 5 iterations
	currentResp := chatResp
	iterationCount := 1 // by the time we're here, we've already made one request

	// Create a new context with room ID and sender for tool calls
	toolCtx := context.WithValue(ctx, "room_id", roomID)
	toolCtx = context.WithValue(toolCtx, "sender", sender)

	maxIterations := getMaxToolIterationsForRoom(ctx, roomID)
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
			return iterationCount, messages, "Failed to process tool calls"
		}

		// Persist the calls together with their responses, after execution, so a call is never
		// stored without the response that answers it
		saveToolCallHistory(ctx, roomID, currentResp.Choices[0].Message.ToolCalls, toolResponses, tools)

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
			return iterationCount, messages, "Failed to get response from chat API"
		} else if len(nextResp.Choices) == 0 {
			log.Error().Ctx(ctx).
				Str("room_id", roomID).
				Str("model", model).
				Bool("has_image", hasImage).
				Int("iteration", iterationCount).
				Msg("Chat API returned no choices for tool iteration")
			return iterationCount, messages, "No response from chat API"
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
		if err == nil {
			// Persist the calls together with their responses, after execution, so a call is never
			// stored without the response that answers it
			saveToolCallHistory(ctx, roomID, currentResp.Choices[0].Message.ToolCalls, toolResponses, tools)

			// Add each tool response as a separate message
			for _, toolResp := range toolResponses {
				messages = append(messages, aigateway.Message{
					Role:       "tool",
					Content:    toolResp.Response,
					ToolCallID: toolResp.ToolCallID,
				})
			}
		}

		// Final request without tools
		req := aigateway.ChatRequest{
			Model:     model,
			Messages:  messages,
			MaxTokens: &maxTokens,
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
			return iterationCount, messages, "Failed to get response from chat API"
		} else if len(finalResp.Choices) == 0 {
			log.Error().Ctx(ctx).
				Str("room_id", roomID).
				Str("model", model).
				Bool("has_image", hasImage).
				Msg("Final chat API returned no choices")
			return iterationCount, messages, "No response from chat API"
		}

		currentResp = finalResp
	}

	// Get the assistant's response from the final request
	assistantResponse := extractAssistantResponse(ctx, roomID, sender, model, hasImage, currentResp)

	return iterationCount, messages, assistantResponse
}

// saveToolCallHistory persists a batch of tool calls together with the responses they produced.
//
// Both halves are written in one transaction after the tools have run. Writing the calls first and
// the responses later leaves a window — a failed write, a cancelled context, a restart — in which
// history holds a call with no response, which the chat API then rejects on every later request in
// the room.
func saveToolCallHistory(ctx context.Context, roomID string, toolCalls []aigateway.ToolCall, toolResponses []aigateway.ToolResponse, tools []aigateway.ToolDefinition) {
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
				Str("room_id", roomID).
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

	persistCtx, cancel := persistContext(ctx)
	defer cancel()

	if err := db.SaveToolCallsWithResponses(persistCtx, roomID, config.UserID, records); err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Int("record_count", len(records)).
			Msg("Failed to save tool call history")
		// Continue even if saving fails: the transaction is atomic, so history is left consistent
	}
}

// buildDebugData creates debug data for the response
func buildDebugData(model string, messages []aigateway.Message, iterationCount int) map[string]any {
	debugData := map[string]any{
		"model":                model,
		"prompt_message_count": len(messages),
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

// getToolsForUser returns tool definitions filtered based on user permissions
func getToolsForUser(ctx context.Context, userID string) []aigateway.ToolDefinition {
	allTools := toolRegistry.GetToolDefinitions()

	// Check if user has Grafana authorization
	hasGrafana := db.IsGrafanaAuthorized(ctx, userID)

	// If user has all permissions, return all tools
	if hasGrafana {
		return allTools
	}

	// Filter out tools that require permissions the user doesn't have
	filtered := make([]aigateway.ToolDefinition, 0, len(allTools))
	for _, tool := range allTools {
		// Skip user_grafana tool if user doesn't have Grafana permission
		if tool.Function.Name == "user_grafana" && !hasGrafana {
			continue
		}
		filtered = append(filtered, tool)
	}

	return filtered
}
