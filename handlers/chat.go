package handlers

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"deepinfra-wrapper/services"
	"deepinfra-wrapper/types"
	"deepinfra-wrapper/utils"
)

var chatSemaphore = make(chan struct{}, 100)

const maxDebugPayloadChars = 12000

func isChatDebugEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("CHAT_DEBUG")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func debugPayload(label string, payload []byte) {
	if !isChatDebugEnabled() {
		return
	}

	payloadStr := string(payload)
	if len(payloadStr) > maxDebugPayloadChars {
		payloadStr = payloadStr[:maxDebugPayloadChars] + "...<truncated>"
	}

	fmt.Printf("🐞 %s (%d bytes): %s\n", label, len(payload), payloadStr)
}

func debugText(label, text string) {
	if !isChatDebugEnabled() {
		return
	}

	if len(text) > maxDebugPayloadChars {
		text = text[:maxDebugPayloadChars] + "...<truncated>"
	}

	fmt.Printf("🐞 %s: %s\n", label, text)
}

func generateID(prefix string) string {
	b := make([]byte, 12)
	rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded") ||
		strings.Contains(errStr, "context deadline")
}

func ChatCompletionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	select {
	case chatSemaphore <- struct{}{}:
		defer func() { <-chatSemaphore }()
	default:
		utils.SendErrorResponse(w, "Server is experiencing high load. Please try again later.", "rate_limit_error", http.StatusTooManyRequests)
		return
	}

	fmt.Printf("💬 Chat completion request from %s\n", r.RemoteAddr)

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		fmt.Printf("❌ Failed to read request body: %v\n", err)
		utils.SendErrorResponse(w, "Failed to read request body", "invalid_request_error", http.StatusBadRequest)
		return
	}
	r.Body.Close()
	debugPayload("Incoming request body", bodyBytes)

	var chatReq types.ChatCompletionRequest
	err = json.Unmarshal(bodyBytes, &chatReq)
	if err != nil {
		fmt.Printf("❌ Failed to parse request: %v\n", err)
		utils.SendErrorResponse(w, "Failed to parse request body", "invalid_request_error", http.StatusBadRequest)
		return
	}

	fmt.Printf("🤖 Model requested: %s\n", chatReq.Model)

	if !services.IsModelSupported(chatReq.Model) {
		fmt.Printf("❌ Unsupported model: %s\n", chatReq.Model)
		utils.SendErrorResponse(w, "Unsupported model. Please use one of the supported models.", "invalid_request_error", http.StatusBadRequest, "model_not_found")
		return
	}

	// Set default values if not provided
	if chatReq.Temperature == 0 {
		chatReq.Temperature = 0.7
	}
	if chatReq.MaxTokens == 0 {
		chatReq.MaxTokens = 15000
	}
	if chatReq.TopP == 0 {
		chatReq.TopP = 1.0
	}

	// Handle message content normalization
	for i := range chatReq.Messages {
		if chatReq.Messages[i].Role == "content" {
			if contentStr, ok := chatReq.Messages[i].Content.(string); ok && contentStr == "user" {
				chatReq.Messages[i].Role, chatReq.Messages[i].Content = contentStr, chatReq.Messages[i].Role
			}
		}

		// Handle content that might be an array (for multimodal)
		if contentArray, ok := chatReq.Messages[i].Content.([]interface{}); ok {
			// Convert to string if it's a simple array with text
			if len(contentArray) == 1 {
				if textPart, ok := contentArray[0].(map[string]interface{}); ok {
					if textType, ok := textPart["type"].(string); ok && textType == "text" {
						if text, ok := textPart["text"].(string); ok {
							chatReq.Messages[i].Content = text
						}
					}
				}
			}
		}
	}

	// Handle response format for JSON mode
	if chatReq.ResponseFormat != nil && chatReq.ResponseFormat.Type == "json_object" {
		// Ensure system message instructs JSON output
		hasSystemMessage := false
		for _, msg := range chatReq.Messages {
			if msg.Role == "system" {
				hasSystemMessage = true
				break
			}
		}
		if !hasSystemMessage {
			// Prepend system message to ensure JSON output
			systemMessage := types.ChatMessage{
				Role:    "system",
				Content: "You are a helpful assistant that always responds with valid JSON objects.",
			}
			chatReq.Messages = append([]types.ChatMessage{systemMessage}, chatReq.Messages...)
		}
	}

	data, err := json.Marshal(chatReq)
	if err != nil {
		fmt.Printf("❌ Failed to marshal request: %v\n", err)
		utils.SendErrorResponse(w, "Failed to marshal request", "internal_error", http.StatusInternalServerError)
		return
	}
	debugPayload("Forwarded request body", data)

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	success := false
	var lastErr error
	var currentProxy string

	fmt.Println("🔄 Beginning proxy attempts...")

	for i := 0; i < services.MaxProxyAttempts && !success; i++ {
		select {
		case <-ctx.Done():
			fmt.Println("⏱️ Request timeout")
			utils.SendErrorResponse(w, "Request timeout", "timeout", http.StatusGatewayTimeout)
			return
		default:
			proxy := services.GetWorkingProxy()
			if proxy == "" {
				fmt.Println("⚠️ No working proxy available, waiting for refresh...")
				if i > 0 {
					time.Sleep(500 * time.Millisecond)
				}
				continue
			}

			if i > 0 && !isTimeoutError(lastErr) && currentProxy != "" {
				proxy = currentProxy
			} else {
				currentProxy = proxy
			}

			fmt.Printf("🌐 Attempt %d: Using proxy %s\n", i+1, proxy)

			result, err, isProxyErr := sendChatRequest(ctx, proxy, services.DeepInfraBaseURL+services.ChatEndpoint, data, chatReq.Stream, w)
			if err != nil {
				fmt.Printf("❌ Proxy attempt %d failed: %v\n", i+1, err)
				if isProxyErr || isTimeoutError(err) {
					services.MarkProxyFailed(proxy)
					currentProxy = ""
				}
				lastErr = err
				continue
			}

			if result {
				fmt.Printf("✅ Chat completion successful using proxy %s (attempt %d)\n", proxy, i+1)
				success = true
				break
			}

			lastErr = fmt.Errorf("proxy request failed without error")
		}
	}

	if !success {
		errMsg := "Unable to process the request after multiple attempts"
		if lastErr != nil {
			errMsg = "Error: " + lastErr.Error()
		}
		fmt.Printf("❌ All proxy attempts failed: %s\n", errMsg)
		utils.SendErrorResponse(w, errMsg, "internal_error", http.StatusInternalServerError)
	}
}

func sendChatRequest(ctx context.Context, proxy, endpoint string, data []byte, isStream bool, w http.ResponseWriter) (bool, error, bool) {
	proxyURL, err := url.Parse("http://" + proxy)
	if err != nil {
		return false, err, true
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
		Timeout: 60 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(data))
	if err != nil {
		return false, err, false
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Deepinfra-Source", "web-page")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/92.0.4515.107 Safari/537.36")

	fmt.Printf("📡 Sending request to %s via proxy %s\n", endpoint, proxy)
	resp, err := client.Do(req)
	if err != nil {
		return false, err, true
	}
	defer resp.Body.Close()

	debugText("Upstream response status", fmt.Sprintf("status=%d proxy=%s", resp.StatusCode, proxy))

	if resp.StatusCode == http.StatusOK {
		if isStream {
			fmt.Println("📶 Handling streaming response")
			ok, streamErr := handleStreamResponse(w, resp)
			if streamErr != nil {
				errStr := strings.ToLower(streamErr.Error())
				if strings.Contains(errStr, "context canceled") || strings.Contains(errStr, "broken pipe") || strings.Contains(errStr, "connection reset by peer") {
					// Client disconnected after stream started; do not retry or write a second error response.
					fmt.Printf("⚠️ Stream ended due to client disconnect: %v\n", streamErr)
					return true, nil, false
				}
			}
			return ok, streamErr, false
		} else {
			fmt.Println("📄 Handling normal response")
			ok, normalErr := handleNormalResponse(w, resp)
			return ok, normalErr, false
		}
	}

	body, _ := io.ReadAll(resp.Body)
	debugPayload("Upstream error body", body)

	isProxyErr := resp.StatusCode >= 500 || resp.StatusCode == 408 || resp.StatusCode == 429
	return false, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body)), isProxyErr
}

func handleStreamResponse(w http.ResponseWriter, resp *http.Response) (bool, error) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	responseID := generateID("chatcmpl-")
	modelName := ""
	scanner := bufio.NewScanner(resp.Body)
	var buf bytes.Buffer
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	chunkCount := 0
	doneSent := false

	flusher, ok := w.(http.Flusher)
	if !ok {
		return false, fmt.Errorf("response writer does not support flushing")
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Ignore SSE metadata/comments/heartbeat lines.
		if strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") || strings.HasPrefix(line, "id:") || strings.HasPrefix(line, "retry:") {
			continue
		}

		var dataStr string
		if strings.HasPrefix(line, "data: ") {
			dataStr = strings.TrimPrefix(line, "data: ")
		} else {
			dataStr = line
		}

		dataStr = strings.TrimSpace(dataStr)
		if dataStr == "" {
			continue
		}

		// DeepInfra may send heartbeat as `data: : ping - ...`; not JSON, should be ignored.
		if strings.HasPrefix(dataStr, ":") {
			continue
		}

		if dataStr == "[DONE]" {
			debugText("Stream inbound chunk", dataStr)
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			chunkCount++
			doneSent = true
			continue
		}

		if chunkCount < 12 {
			debugText("Stream inbound chunk", dataStr)
		}

		var streamChunk map[string]interface{}
		if err := json.Unmarshal([]byte(dataStr), &streamChunk); err != nil {
			fmt.Fprintf(w, "data: %s\n\n", dataStr)
			flusher.Flush()
			chunkCount++
			continue
		}

		if model, ok := streamChunk["model"].(string); ok && model != "" {
			modelName = model
		}

		normalizedChunk := types.ChatCompletionChunk{
			ID:      responseID,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   modelName,
			Choices: []types.ChatCompletionChoice{},
		}

		if choices, ok := streamChunk["choices"].([]interface{}); ok {
			for i, choiceRaw := range choices {
				if choice, ok := choiceRaw.(map[string]interface{}); ok {
					normalizedChoice := types.ChatCompletionChoice{
						Index: i,
					}

					if deltaRaw, ok := choice["delta"].(map[string]interface{}); ok {
						delta := &types.ChatCompletionDelta{}
						if role, ok := deltaRaw["role"].(string); ok {
							delta.Role = role
						}
						if content, ok := deltaRaw["content"].(string); ok {
							delta.Content = content
						}
						if reasoningContent, exists := deltaRaw["reasoning_content"]; exists && reasoningContent != nil {
							delta.ReasoningContent = reasoningContent
						}
						if functionCallRaw, ok := deltaRaw["function_call"].(map[string]interface{}); ok {
							functionCall := &types.DeltaFunctionCall{}
							if name, ok := functionCallRaw["name"].(string); ok {
								nameCopy := name
								functionCall.Name = &nameCopy
							}
							if arguments, ok := functionCallRaw["arguments"].(string); ok {
								argumentsCopy := arguments
								functionCall.Arguments = &argumentsCopy
							}
							if functionCall.Name != nil || functionCall.Arguments != nil {
								delta.FunctionCall = functionCall
							}
						}
						if toolCallsRaw, ok := deltaRaw["tool_calls"].([]interface{}); ok {
							delta.ToolCalls = make([]types.DeltaToolCall, 0, len(toolCallsRaw))
							for _, tcRaw := range toolCallsRaw {
								tcMap, ok := tcRaw.(map[string]interface{})
								if !ok {
									continue
								}

								toolCall := types.DeltaToolCall{}
								if idx, ok := tcMap["index"].(float64); ok {
									idxInt := int(idx)
									toolCall.Index = &idxInt
								}
								if id, ok := tcMap["id"].(string); ok {
									idCopy := id
									toolCall.ID = &idCopy
								}
								if tcType, ok := tcMap["type"].(string); ok {
									typeCopy := tcType
									toolCall.Type = &typeCopy
								}
								if fnRaw, ok := tcMap["function"].(map[string]interface{}); ok {
									deltaFunction := &types.DeltaFunctionCall{}
									if name, ok := fnRaw["name"].(string); ok {
										nameCopy := name
										deltaFunction.Name = &nameCopy
									}
									if arguments, ok := fnRaw["arguments"].(string); ok {
										argumentsCopy := arguments
										deltaFunction.Arguments = &argumentsCopy
									}
									if deltaFunction.Name != nil || deltaFunction.Arguments != nil {
										toolCall.Function = deltaFunction
									}
								}

								if toolCall.Index != nil || toolCall.ID != nil || toolCall.Type != nil || toolCall.Function != nil {
									delta.ToolCalls = append(delta.ToolCalls, toolCall)
								}
							}
							if len(delta.ToolCalls) == 0 {
								delta.ToolCalls = nil
							}
						}
						normalizedChoice.Delta = delta
					} else if text, ok := choice["text"].(string); ok {
						normalizedChoice.Delta = &types.ChatCompletionDelta{
							Content: text,
						}
					}

					if fr, ok := choice["finish_reason"].(string); ok {
						normalizedChoice.FinishReason = fr
					}

					normalizedChunk.Choices = append(normalizedChunk.Choices, normalizedChoice)
				}
			}
		}

		chunkBytes, err := json.Marshal(normalizedChunk)
		if err != nil {
			fmt.Fprintf(w, "data: %s\n\n", dataStr)
		} else {
			if chunkCount < 12 {
				debugPayload("Stream outbound chunk", chunkBytes)
			}
			fmt.Fprintf(w, "data: %s\n\n", string(chunkBytes))
		}
		flusher.Flush()
		chunkCount++
	}

	if !doneSent {
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
		chunkCount++
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("❌ Stream error: %v\n", err)
		return false, err
	}

	fmt.Printf("✅ Stream complete, sent %d chunks\n", chunkCount)
	buf.Reset()
	return true, nil
}

func handleNormalResponse(w http.ResponseWriter, resp *http.Response) (bool, error) {
	w.Header().Set("Content-Type", "application/json")

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read response body: %v", err)
	}
	debugPayload("Upstream normal response body", bodyBytes)

	var deepInfraResp map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &deepInfraResp); err != nil {
		return false, fmt.Errorf("failed to parse response: %v", err)
	}

	responseID := generateID("chatcmpl-")
	choices := []types.ChatCompletionChoice{}

	if choicesRaw, ok := deepInfraResp["choices"].([]interface{}); ok {
		for i, choiceRaw := range choicesRaw {
			if choice, ok := choiceRaw.(map[string]interface{}); ok {
				finishReason := ""
				if fr, ok := choice["finish_reason"].(string); ok {
					finishReason = fr
				}

				message := &types.ChatMessage{}
				if msgRaw, ok := choice["message"].(map[string]interface{}); ok {
					if role, ok := msgRaw["role"].(string); ok {
						message.Role = role
					}
					if content, ok := msgRaw["content"].(string); ok {
						message.Content = content
					}
					if reasoningContent, exists := msgRaw["reasoning_content"]; exists && reasoningContent != nil {
						message.ReasoningContent = reasoningContent
					}
					if functionCallRaw, ok := msgRaw["function_call"].(map[string]interface{}); ok {
						functionCall := &types.FunctionCall{}
						if name, ok := functionCallRaw["name"].(string); ok {
							functionCall.Name = name
						}
						if arguments, ok := functionCallRaw["arguments"].(string); ok {
							functionCall.Arguments = arguments
						}
						message.FunctionCall = functionCall
					}
					if toolCallsRaw, ok := msgRaw["tool_calls"].([]interface{}); ok {
						message.ToolCalls = make([]types.ToolCall, 0, len(toolCallsRaw))
						for _, tcRaw := range toolCallsRaw {
							tcMap, ok := tcRaw.(map[string]interface{})
							if !ok {
								continue
							}

							toolCall := types.ToolCall{}
							if idx, ok := tcMap["index"].(float64); ok {
								idxInt := int(idx)
								toolCall.Index = &idxInt
							}
							if id, ok := tcMap["id"].(string); ok {
								toolCall.ID = id
							}
							if tcType, ok := tcMap["type"].(string); ok {
								toolCall.Type = tcType
							}
							if fnRaw, ok := tcMap["function"].(map[string]interface{}); ok {
								if name, ok := fnRaw["name"].(string); ok {
									toolCall.Function.Name = name
								}
								if arguments, ok := fnRaw["arguments"].(string); ok {
									toolCall.Function.Arguments = arguments
								}
							}

							message.ToolCalls = append(message.ToolCalls, toolCall)
						}
					}
				}

				choices = append(choices, types.ChatCompletionChoice{
					Index:        i,
					Message:      message,
					FinishReason: finishReason,
				})
			}
		}
	}

	modelName := ""
	if model, ok := deepInfraResp["model"].(string); ok {
		modelName = model
	}

	var usage *types.ChatCompletionUsage
	if usageRaw, ok := deepInfraResp["usage"].(map[string]interface{}); ok {
		usage = &types.ChatCompletionUsage{}
		if pt, ok := usageRaw["prompt_tokens"].(float64); ok {
			usage.PromptTokens = int(pt)
		}
		if ct, ok := usageRaw["completion_tokens"].(float64); ok {
			usage.CompletionTokens = int(ct)
		}
		if tt, ok := usageRaw["total_tokens"].(float64); ok {
			usage.TotalTokens = int(tt)
		}
	}

	openAIResp := types.ChatCompletionResponse{
		ID:      responseID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   modelName,
		Choices: choices,
		Usage:   usage,
	}

	respBytes, err := json.Marshal(openAIResp)
	if err != nil {
		return false, fmt.Errorf("failed to marshal response: %v", err)
	}
	debugPayload("Client normal response body", respBytes)

	w.WriteHeader(http.StatusOK)
	_, err = w.Write(respBytes)
	if err != nil {
		fmt.Printf("❌ Error writing response: %v\n", err)
		return false, err
	}

	fmt.Printf("✅ Response sent successfully (%d bytes)\n", len(respBytes))
	return true, nil
}
