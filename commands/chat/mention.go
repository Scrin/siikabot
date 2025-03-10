package chat

import (
	"context"
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

	// Mark the message as read
	matrix.MarkRead(ctx, roomID, eventID)

	log.Debug().Ctx(ctx).
		Str("room_id", roomID).
		Str("sender", sender).
		Str("chat_msg", msg).
		Msg("Processing chat command")

	// Send typing indicator to let the user know we're processing their request
	// Set a timeout that's long enough to cover the expected processing time
	matrix.SendTyping(ctx, roomID, true, 60*time.Second)
	// Make sure we stop the typing indicator when we're done
	defer matrix.SendTyping(ctx, roomID, false, 0)

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
	history, err := db.GetChatHistory(ctx, roomID, maxHistoryMessages)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to get chat history")
		// Continue without history if there's an error
	}

	// Build messages array with system prompt, history, and current message
	messages := []openrouter.Message{{Role: "system", Content: systemPrompt}}

	// Add conversation history
	for _, historyMsg := range history {
		messages = append(messages, openrouter.Message{
			Role:    historyMsg.Role,
			Content: historyMsg.Message,
		})
	}

	// Flag to track if we're handling an image
	hasImage := false
	var base64ImageURL string

	// Check if this message is a reply to another message
	if relatesTo != nil {
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
				} else if msgType == "m.image" {
					// Get the image URL, encryption info, and full content
					imageURL, encryptionInfo, fullContent, err := matrix.GetEventImageURL(ctx, roomID, replyEventID)
					if err != nil {
						log.Error().Ctx(ctx).Err(err).
							Str("room_id", roomID).
							Str("event_id", replyEventID).
							Msg("Failed to get image URL from replied-to message")
					} else {
						// Download the image and convert to base64
						base64ImageURL, err = matrix.DownloadImageAsBase64(ctx, imageURL, encryptionInfo, fullContent)
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

							messages = append(messages, openrouter.Message{
								Role:    "system",
								Content: errorMsg,
							})
						} else {
							hasImage = true
							log.Debug().Ctx(ctx).
								Str("room_id", roomID).
								Str("event_id", replyEventID).
								Str("image_url", imageURL).
								Bool("is_encrypted", encryptionInfo != nil).
								Msg("Message is a reply to an image")
						}
					}
				} else {
					// Get the content of the replied-to message (text)
					repliedToContent, err := matrix.GetEventContent(ctx, roomID, replyEventID)
					if err != nil {
						log.Error().Ctx(ctx).Err(err).
							Str("room_id", roomID).
							Str("event_id", replyEventID).
							Msg("Failed to get replied-to message content")

						// Add a note about the failed attempt to get the replied-to message
						messages = append(messages, openrouter.Message{
							Role:    "system",
							Content: "Note: This message is a reply to another message, but I couldn't retrieve the content of that message.",
						})
					} else if repliedToContent != "" {
						// Add the replied-to message to the conversation
						log.Debug().Ctx(ctx).
							Str("room_id", roomID).
							Str("event_id", replyEventID).
							Str("content", repliedToContent).
							Msg("Including replied-to message in conversation")

						// Add a note about the reply context
						replyContextMsg := fmt.Sprintf("This message is a reply to: \"%s\"", repliedToContent)
						messages = append(messages, openrouter.Message{
							Role:    "system",
							Content: replyContextMsg,
						})
					}
				}
			}
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
		// Create a multimodal message with both text and image
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
				messages = append(messages, openrouter.Message{
					Role:    "system",
					Content: "Note: An image was attached to this message, but it was too large to process (>5MB).",
				})

				// Fall back to text-only message
				messages = append(messages, openrouter.Message{Role: "user", Content: msg})
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

				messages = append(messages, openrouter.Message{
					Role:    "user",
					Content: contentParts,
				})
			}
		} else {
			log.Error().Ctx(ctx).
				Str("room_id", roomID).
				Msg("Image URL does not contain valid base64 data, skipping image")

			// Fall back to text-only message
			messages = append(messages, openrouter.Message{Role: "user", Content: msg})
			hasImage = false
		}
	} else {
		// Regular text message
		messages = append(messages, openrouter.Message{Role: "user", Content: msg})
	}

	// Get tool definitions from the registry
	tools := toolRegistry.GetToolDefinitions()

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

	// Check if the model wants to use a tool
	if chatResp.Choices[0].FinishReason == "tool_calls" && len(chatResp.Choices[0].Message.ToolCalls) > 0 {
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("sender", sender).
			Str("model", model).
			Bool("has_image", hasImage).
			Int("tool_calls", len(chatResp.Choices[0].Message.ToolCalls)).
			Msg("Model requested tool calls")

		// Add the tool response to the conversation
		// First, add the assistant's message with tool calls
		messages = append(messages, openrouter.Message{
			Role:      "assistant",
			Content:   "", // Content should be empty when there are tool calls
			ToolCalls: chatResp.Choices[0].Message.ToolCalls,
		})

		// Create a new context with room ID and sender for tool calls
		toolCtx := context.WithValue(ctx, "room_id", roomID)
		toolCtx = context.WithValue(toolCtx, "sender", sender)

		// Then add individual tool responses for each tool call
		toolResponses, err := toolRegistry.HandleToolCallsIndividually(toolCtx, chatResp.Choices[0].Message.ToolCalls)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Str("model", model).
				Bool("has_image", hasImage).
				Msg("Failed to handle tool calls")
			matrix.SendMessage(roomID, "Failed to process tool calls")
			return
		}

		// Add each tool response as a separate message
		for _, toolResp := range toolResponses {
			messages = append(messages, openrouter.Message{
				Role:       "tool",
				Content:    toolResp.Response,
				ToolCallID: toolResp.ToolCallID,
			})
		}

		// Make a second request to get the final response, using the same model
		req.Messages = messages
		req.Tools = nil // No need for tools in the second request

		// Send typing indicator again for the second request
		matrix.SendTyping(ctx, roomID, true, 30*time.Second)

		// Log the request for debugging
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("sender", sender).
			Str("model", model).
			Bool("has_image", hasImage).
			Int("message_count", len(messages)).
			Msg("Sending second chat request")

		// Send the second request to OpenRouter
		secondChatResp, err := openrouter.SendChatRequest(ctx, req)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Str("model", model).
				Bool("has_image", hasImage).
				Msg("Failed to send second chat request")
			matrix.SendTyping(ctx, roomID, false, 0) // Stop typing indicator on error
			matrix.SendMessage(roomID, "Failed to get response from chat API")
			return
		} else if len(secondChatResp.Choices) == 0 {
			log.Error().Ctx(ctx).
				Str("room_id", roomID).
				Str("model", model).
				Bool("has_image", hasImage).
				Msg("Second chat API returned no choices")
			matrix.SendTyping(ctx, roomID, false, 0) // Stop typing indicator on error
			matrix.SendMessage(roomID, "No response from chat API")
			return
		} else {
			// Get the assistant's response from the second request
			if content, ok := secondChatResp.Choices[0].Message.Content.(string); ok {
				assistantResponse = content
				log.Debug().Ctx(ctx).
					Str("room_id", roomID).
					Str("sender", sender).
					Str("model", model).
					Bool("has_image", hasImage).
					Int("response_length", len(assistantResponse)).
					Msg("Received string response from second OpenRouter request")
			} else if contentMap, ok := secondChatResp.Choices[0].Message.Content.(map[string]interface{}); ok {
				log.Debug().Ctx(ctx).
					Str("room_id", roomID).
					Str("sender", sender).
					Str("model", model).
					Bool("has_image", hasImage).
					Interface("content_map", contentMap).
					Msg("Received map response from second OpenRouter request")

				if text, ok := contentMap["text"].(string); ok {
					assistantResponse = text
				} else {
					assistantResponse = "I processed your request, but couldn't generate a proper response."
					log.Warn().Ctx(ctx).
						Str("room_id", roomID).
						Str("sender", sender).
						Str("model", model).
						Bool("has_image", hasImage).
						Interface("content_map", contentMap).
						Msg("Second response content map doesn't contain text field")
				}
			} else {
				assistantResponse = "I processed your request, but couldn't generate a proper response."
				log.Warn().Ctx(ctx).
					Str("room_id", roomID).
					Str("sender", sender).
					Str("model", model).
					Bool("has_image", hasImage).
					Interface("content", secondChatResp.Choices[0].Message.Content).
					Msg("Unexpected second response content type")
			}
		}
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
	debugData := map[string]any{
		"model":                model,
		"prompt_message_count": len(messages),
	}

	// Add tool calls information if any were made
	if len(chatResp.Choices) > 0 && len(chatResp.Choices[0].Message.ToolCalls) > 0 {
		toolNames := make([]string, len(chatResp.Choices[0].Message.ToolCalls))
		for i, toolCall := range chatResp.Choices[0].Message.ToolCalls {
			toolNames[i] = toolCall.Function.Name
		}
		debugData["tool_calls"] = toolNames
	}

	matrix.SendMarkdownFormattedNoticeWithDebugData(roomID, assistantResponse, debugData)
}
