package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/Scrin/siikabot/openrouter"
	"github.com/rs/zerolog/log"
)

// HandleMention handles the chat command
func HandleMention(ctx context.Context, roomID, sender, msg, eventID string, relatesTo map[string]interface{}) {
	if strings.TrimSpace(msg) == "" {
		return
	}

	log.Debug().Ctx(ctx).
		Str("room_id", roomID).
		Str("sender", sender).
		Str("chat_msg", msg).
		Msg("Processing chat command")

	// Variable to track tool iterations
	iterationCount := 0

	// Send typing indicator to let the user know we're processing their request
	// Set a timeout that's long enough to cover the expected processing time
	matrix.SendTyping(ctx, roomID, true, 60*time.Second)
	// Make sure we stop the typing indicator when we're done
	defer matrix.SendTyping(ctx, roomID, false, 0)

	// Build the initial messages with system prompt, history, and handle image if present
	messages, hasImage, model := buildInitialMessages(ctx, roomID, sender, msg, relatesTo)

	// Get tool definitions from the registry
	tools := toolRegistry.GetToolDefinitions()

	// Create the initial request
	req := openrouter.ChatRequest{
		Model:    model,
		Messages: messages,
		Tools:    tools,
	}

	// Send the request to OpenRouter
	log.Debug().Ctx(ctx).
		Str("room_id", roomID).
		Str("sender", sender).
		Str("model", model).
		Bool("has_image", hasImage).
		Int("message_count", len(messages)).
		Msg("Sending chat request to OpenRouter")

	chatResp, err := openrouter.SendChatRequest(ctx, req)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Str("sender", sender).
			Str("model", model).
			Bool("has_image", hasImage).
			Msg("Failed to send chat request")
		matrix.SendTyping(ctx, roomID, false, 0) // Stop typing indicator on error
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
		matrix.SendTyping(ctx, roomID, false, 0) // Stop typing indicator on error
		matrix.SendMessage(roomID, "No response from chat API")
		return
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
			ctx, roomID, sender, model, hasImage,
			chatResp, messages, tools,
		)
	}

	// Save the assistant response to history
	if err := db.SaveChatMessage(ctx, roomID, config.UserID, assistantResponse, "assistant"); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to save assistant message to history")
		// Continue even if saving fails
	}

	log.Debug().Ctx(ctx).
		Str("room_id", roomID).
		Str("sender", sender).
		Str("model", model).
		Bool("has_image", hasImage).
		Int("response_length", len(assistantResponse)).
		Msg("Chat command completed")

	// Create debug data with model info and tool calls
	debugData := buildDebugData(model, messages, iterationCount)

	matrix.SendMarkdownFormattedNoticeWithDebugData(roomID, assistantResponse, debugData)
}

// buildInitialMessages creates the initial messages array with system prompt, history, and user message
func buildInitialMessages(ctx context.Context, roomID, sender, msg string, relatesTo map[string]interface{}) ([]openrouter.Message, bool, string) {
	// Create a system prompt with bot identity and current time
	loc, _ := time.LoadLocation(config.Timezone)
	currentTime := time.Now().In(loc).Format("Monday, January 2, 2006 15:04:05 MST")

	// Get the bot's actual display name from the Matrix server
	botDisplayName := matrix.GetDisplayName(ctx, config.UserID)
	if botDisplayName == "" {
		// Fallback to user ID if display name can't be retrieved
		botDisplayName = strings.Split(config.UserID, ":")[0][1:] // Remove @ and domain part
	}

	systemPrompt := fmt.Sprintf(
		"You are %s, a helpful Matrix bot. The current date and time is %s. "+
			"Keep your responses concise and helpful. Use markdown formatting in your responses.",
		botDisplayName,
		currentTime,
	)

	// Get conversation history
	maxHistory := getMaxHistoryMessagesForRoom(ctx, roomID)
	history, err := db.GetChatHistory(ctx, roomID, maxHistory)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to get chat history")
		// Continue without history if there's an error
	}

	// Build messages array with system prompt, history, and current message
	messages := []openrouter.Message{{Role: "system", Content: systemPrompt}}

	// Process history to include tool calls and tool responses
	processHistoryMessages(ctx, history, &messages)

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
		messages = append(messages, openrouter.Message{Role: "user", Content: msg})
	}

	return messages, hasImage, model
}

// processHistoryMessages processes the chat history and adds it to the messages array
func processHistoryMessages(ctx context.Context, history []db.ChatMessage, messages *[]openrouter.Message) {
	// Group tool calls and their responses
	toolCallMap := make(map[string]openrouter.ToolCall)
	toolResponseMap := make(map[string]string)

	// First pass: collect tool calls and tool responses
	for _, historyMsg := range history {

		messageType := historyMsg.MessageType

		if messageType == "tool_call" && historyMsg.ToolCallID != nil && historyMsg.ToolName != nil {
			// Create a tool call object
			toolCallMap[*historyMsg.ToolCallID] = openrouter.ToolCall{
				ID:   *historyMsg.ToolCallID,
				Type: "function",
				Function: openrouter.ToolFunction{
					Name:      *historyMsg.ToolName,
					Arguments: historyMsg.Message,
				},
			}
		} else if messageType == "tool_response" && historyMsg.ToolCallID != nil {
			// Store the tool response
			toolResponseMap[*historyMsg.ToolCallID] = historyMsg.Message
		} else if messageType == "text" || messageType == "" {
			// Regular text message
			*messages = append(*messages, openrouter.Message{
				Role:    historyMsg.Role,
				Content: historyMsg.Message,
			})
		}
	}

	// If no tool calls were found, we're done
	if len(toolCallMap) == 0 {
		return
	}

	// Second pass: add messages in order, grouping tool calls and responses
	var currentToolCalls []openrouter.ToolCall
	var pendingToolCallIDs []string

	for i, historyMsg := range history {

		messageType := historyMsg.MessageType

		if messageType == "tool_call" && historyMsg.ToolCallID != nil {
			// Add to current batch of tool calls
			toolCallID := *historyMsg.ToolCallID
			if toolCall, ok := toolCallMap[toolCallID]; ok {
				currentToolCalls = append(currentToolCalls, toolCall)
				pendingToolCallIDs = append(pendingToolCallIDs, toolCallID)
			}

			// Check if this is the last message or if the next message is not a tool call
			isLastMessage := i == len(history)-1
			isNextMessageNotToolCall := !isLastMessage && (history[i+1].MessageType != "tool_call")

			if isLastMessage || isNextMessageNotToolCall {
				// Add the assistant message with all collected tool calls
				if len(currentToolCalls) > 0 {
					*messages = append(*messages, openrouter.Message{
						Role:      "assistant",
						Content:   "",
						ToolCalls: currentToolCalls,
					})

					// Add tool responses for these tool calls
					for _, toolCallID := range pendingToolCallIDs {
						if response, ok := toolResponseMap[toolCallID]; ok {
							*messages = append(*messages, openrouter.Message{
								Role:       "tool",
								Content:    response,
								ToolCallID: toolCallID,
							})
						}
					}

					// Reset for next batch
					currentToolCalls = nil
					pendingToolCallIDs = nil
				}
			}
		}
		// Skip tool_response messages as they're handled with their corresponding tool calls
		// Skip text messages as they're handled in the first pass
	}
}

// processRelatedMessage handles messages that are replies to other messages
// Returns base64ImageURL if the message is a reply to an image
func processRelatedMessage(ctx context.Context, roomID string, relatesTo map[string]interface{}, messages *[]openrouter.Message) string {
	log.Debug().Ctx(ctx).
		Str("room_id", roomID).
		Interface("relates_to", relatesTo).
		Msg("Message has relation information")

	// Check for m.in_reply_to
	if inReplyTo, ok := relatesTo["m.in_reply_to"].(map[string]interface{}); ok {
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
func processRepliedImage(ctx context.Context, roomID, replyEventID string, messages *[]openrouter.Message) string {
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

		// Add more detailed error information for debugging
		if config.Debug {
			errorMsg += fmt.Sprintf(" Technical details: %v", err)
		}

		*messages = append(*messages, openrouter.Message{
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
func processRepliedText(ctx context.Context, roomID, replyEventID string, messages *[]openrouter.Message) {
	// Get the content of the replied-to message (text)
	repliedToContent, err := matrix.GetEventContent(ctx, roomID, replyEventID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Str("event_id", replyEventID).
			Msg("Failed to get replied-to message content")

		// Add a note about the failed attempt to get the replied-to message
		*messages = append(*messages, openrouter.Message{
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
		*messages = append(*messages, openrouter.Message{
			Role:    "system",
			Content: replyContextMsg,
		})
	}
}

// processImageMessage handles messages that include an image
// Returns updated hasImage flag and messages
func processImageMessage(ctx context.Context, roomID, msg, base64ImageURL string, messages *[]openrouter.Message) (bool, []openrouter.Message) {
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
			*messages = append(*messages, openrouter.Message{
				Role:    "system",
				Content: "Note: An image was attached to this message, but it was too large to process (>5MB).",
			})

			// Fall back to text-only message
			*messages = append(*messages, openrouter.Message{Role: "user", Content: msg})
			hasImage = false
		} else {
			contentParts := []openrouter.ContentPart{
				{
					Type: "text",
					Text: msg,
				},
				{
					Type: "image_url",
					ImageURL: &struct {
						URL string `json:"url"`
					}{
						URL: base64ImageURL,
					},
				},
			}

			*messages = append(*messages, openrouter.Message{
				Role:    "user",
				Content: contentParts,
			})
		}
	} else {
		log.Error().Ctx(ctx).
			Str("room_id", roomID).
			Msg("Image URL does not contain valid base64 data, skipping image")

		// Fall back to text-only message
		*messages = append(*messages, openrouter.Message{Role: "user", Content: msg})
		hasImage = false
	}

	return hasImage, *messages
}

// extractAssistantResponse extracts the assistant's response from the API response
func extractAssistantResponse(ctx context.Context, roomID, sender, model string, hasImage bool, chatResp *openrouter.ChatResponse) string {
	var assistantResponse string

	if content, ok := chatResp.Choices[0].Message.Content.(string); ok {
		assistantResponse = content
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("sender", sender).
			Str("model", model).
			Bool("has_image", hasImage).
			Int("response_length", len(assistantResponse)).
			Msg("Received string response from OpenRouter")
	} else if contentMap, ok := chatResp.Choices[0].Message.Content.(map[string]interface{}); ok {
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("sender", sender).
			Str("model", model).
			Bool("has_image", hasImage).
			Interface("content_map", contentMap).
			Msg("Received map response from OpenRouter")

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
	chatResp *openrouter.ChatResponse,
	messages []openrouter.Message,
	tools []openrouter.ToolDefinition,
) (int, []openrouter.Message, string) {
	// Implement iterative tool calling with a maximum of 5 iterations
	currentResp := chatResp
	iterationCount := 1 // by the time we're here, we've already made one request

	// Create a new context with room ID and sender for tool calls
	toolCtx := context.WithValue(ctx, "room_id", roomID)
	toolCtx = context.WithValue(toolCtx, "sender", sender)

	// Map to store expiry timestamps for tool calls
	toolCallExpiries := make(map[string]*time.Time)

	maxIterations := getMaxToolIterationsForRoom(ctx, roomID)
	for iterationCount < maxIterations {
		iterationCount++

		// Add the assistant's message with tool calls
		messages = append(messages, openrouter.Message{
			Role:      "assistant",
			Content:   "", // Content should be empty when there are tool calls
			ToolCalls: currentResp.Choices[0].Message.ToolCalls,
		})

		// Save each tool call to the database
		for _, toolCall := range currentResp.Choices[0].Message.ToolCalls {
			// Find the tool definition to get the validity duration
			var validityDuration time.Duration
			for _, tool := range tools {
				if tool.Function.Name == toolCall.Function.Name {
					validityDuration = tool.ValidityDuration
					break
				}
			}

			// Save the tool call to the database with validity duration
			expiry, err := db.SaveToolCall(ctx, roomID, config.UserID, toolCall.ID, toolCall.Function.Name, toolCall.Function.Arguments, validityDuration)
			if err != nil {
				log.Error().Ctx(ctx).Err(err).
					Str("room_id", roomID).
					Str("tool_call_id", toolCall.ID).
					Str("tool_name", toolCall.Function.Name).
					Msg("Failed to save tool call to history")
				// Continue even if saving fails
			}

			// Store the expiry timestamp for later use with the response
			if expiry != nil {
				toolCallExpiries[toolCall.ID] = expiry
			}
		}

		// Process tool calls
		toolResponses, err := toolRegistry.HandleToolCallsIndividually(toolCtx, currentResp.Choices[0].Message.ToolCalls)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Str("model", model).
				Bool("has_image", hasImage).
				Int("iteration", iterationCount).
				Msg("Failed to handle tool calls")
			return iterationCount, messages, "Failed to process tool calls"
		}

		// Add each tool response as a separate message
		for _, toolResp := range toolResponses {
			messages = append(messages, openrouter.Message{
				Role:       "tool",
				Content:    toolResp.Response,
				ToolCallID: toolResp.ToolCallID,
			})

			// Save the tool response to the database
			// Find the tool name from the tool calls
			var toolName string
			for _, toolCall := range currentResp.Choices[0].Message.ToolCalls {
				if toolCall.ID == toolResp.ToolCallID {
					toolName = toolCall.Function.Name
					break
				}
			}

			// Use the same expiry timestamp as the tool call
			expiry := toolCallExpiries[toolResp.ToolCallID]
			err := db.SaveToolResponse(ctx, roomID, config.UserID, toolResp.ToolCallID, toolName, toolResp.Response, expiry)
			if err != nil {
				log.Error().Ctx(ctx).Err(err).
					Str("room_id", roomID).
					Str("tool_call_id", toolResp.ToolCallID).
					Str("tool_name", toolName).
					Msg("Failed to save tool response to history")
				// Continue even if saving fails
			}
		}

		// Update the request with the new messages
		req := openrouter.ChatRequest{
			Model:    model,
			Messages: messages,
			Tools:    tools,
		}

		// Send typing indicator for the next request
		matrix.SendTyping(ctx, roomID, true, 30*time.Second)

		// Log the request for debugging
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("sender", sender).
			Str("model", model).
			Bool("has_image", hasImage).
			Int("message_count", len(messages)).
			Int("iteration", iterationCount).
			Msg("Sending chat request for tool iteration")

		// Send the next request to OpenRouter
		nextResp, err := openrouter.SendChatRequest(ctx, req)
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
		messages = append(messages, openrouter.Message{
			Role:      "assistant",
			Content:   "", // Content should be empty when there are tool calls
			ToolCalls: currentResp.Choices[0].Message.ToolCalls,
		})

		// Save each tool call to the database
		for _, toolCall := range currentResp.Choices[0].Message.ToolCalls {
			// Find the tool definition to get the validity duration
			var validityDuration time.Duration
			for _, tool := range tools {
				if tool.Function.Name == toolCall.Function.Name {
					validityDuration = tool.ValidityDuration
					break
				}
			}

			// Save the tool call to the database with validity duration
			expiry, err := db.SaveToolCall(ctx, roomID, config.UserID, toolCall.ID, toolCall.Function.Name, toolCall.Function.Arguments, validityDuration)
			if err != nil {
				log.Error().Ctx(ctx).Err(err).
					Str("room_id", roomID).
					Str("tool_call_id", toolCall.ID).
					Str("tool_name", toolCall.Function.Name).
					Msg("Failed to save tool call to history")
				// Continue even if saving fails
			}

			// Store the expiry timestamp for later use with the response
			if expiry != nil {
				toolCallExpiries[toolCall.ID] = expiry
			}
		}

		// Process the final tool calls
		toolResponses, err := toolRegistry.HandleToolCallsIndividually(toolCtx, currentResp.Choices[0].Message.ToolCalls)
		if err == nil {
			// Add each tool response as a separate message
			for _, toolResp := range toolResponses {
				messages = append(messages, openrouter.Message{
					Role:       "tool",
					Content:    toolResp.Response,
					ToolCallID: toolResp.ToolCallID,
				})

				// Save the tool response to the database
				// Find the tool name from the tool calls
				var toolName string
				for _, toolCall := range currentResp.Choices[0].Message.ToolCalls {
					if toolCall.ID == toolResp.ToolCallID {
						toolName = toolCall.Function.Name
						break
					}
				}

				// Use the same expiry timestamp as the tool call
				expiry := toolCallExpiries[toolResp.ToolCallID]
				err := db.SaveToolResponse(ctx, roomID, config.UserID, toolResp.ToolCallID, toolName, toolResp.Response, expiry)
				if err != nil {
					log.Error().Ctx(ctx).Err(err).
						Str("room_id", roomID).
						Str("tool_call_id", toolResp.ToolCallID).
						Str("tool_name", toolName).
						Msg("Failed to save tool response to history")
					// Continue even if saving fails
				}
			}
		}

		// Final request without tools
		req := openrouter.ChatRequest{
			Model:    model,
			Messages: messages,
		}

		// Send typing indicator for the final request
		matrix.SendTyping(ctx, roomID, true, 30*time.Second)

		// Log the final request
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("sender", sender).
			Str("model", model).
			Bool("has_image", hasImage).
			Int("message_count", len(messages)).
			Msg("Sending final request without tools")

		// Send the final request to OpenRouter
		finalResp, err := openrouter.SendChatRequest(ctx, req)
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

// buildDebugData creates debug data for the response
func buildDebugData(model string, messages []openrouter.Message, iterationCount int) map[string]any {
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
