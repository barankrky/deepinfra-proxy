package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"deepinfra-wrapper/services"
	"deepinfra-wrapper/types"
	"deepinfra-wrapper/utils"
)

func EmbeddingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fmt.Printf("📊 Embeddings request from %s\n", r.RemoteAddr)

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		fmt.Printf("❌ Failed to read request body: %v\n", err)
		utils.SendErrorResponse(w, "Failed to read request body", "invalid_request_error", http.StatusBadRequest)
		return
	}
	r.Body.Close()

	var embReq types.EmbeddingRequest
	err = json.Unmarshal(bodyBytes, &embReq)
	if err != nil {
		fmt.Printf("❌ Failed to parse request: %v\n", err)
		utils.SendErrorResponse(w, "Failed to parse request body", "invalid_request_error", http.StatusBadRequest)
		return
	}

	if embReq.Input == nil {
		utils.SendErrorResponse(w, "Input is required", "invalid_request_error", http.StatusBadRequest)
		return
	}

	if embReq.Model == "" {
		embReq.Model = "BAAI/bge-large-en-v1.5"
	}

	fmt.Printf("🤖 Embedding model requested: %s\n", embReq.Model)

	if !services.IsModelSupported(embReq.Model) {
		fmt.Printf("❌ Unsupported model: %s\n", embReq.Model)
		utils.SendErrorResponse(w, "Unsupported model. Please use one of the supported models.", "invalid_request_error", http.StatusBadRequest, "model_not_found")
		return
	}

	data, err := json.Marshal(embReq)
	if err != nil {
		fmt.Printf("❌ Failed to marshal request: %v\n", err)
		utils.SendErrorResponse(w, "Failed to marshal request", "internal_error", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	var lastErr error
	var currentProxy string

	for i := 0; i < services.MaxProxyAttempts; i++ {
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

			if i > 0 && !isTimeoutError(lastErr) {
				proxy = currentProxy
			} else {
				currentProxy = proxy
			}

			fmt.Printf("🌐 Attempt %d: Using proxy %s\n", i+1, proxy)

			result, err, isProxyErr := sendEmbeddingRequest(ctx, proxy, services.DeepInfraBaseURL+"/embeddings", data, w)
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
				fmt.Printf("✅ Embedding successful using proxy %s (attempt %d)\n", proxy, i+1)
				return
			}
		}
	}

	utils.SendErrorResponse(w, "Unable to process the request after multiple attempts", "internal_error", http.StatusInternalServerError)
}

func sendEmbeddingRequest(ctx context.Context, proxy, endpoint string, data []byte, w http.ResponseWriter) (bool, error, bool) {
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

	resp, err := client.Do(req)
	if err != nil {
		return false, err, true
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		isProxyErr := resp.StatusCode >= 500 || resp.StatusCode == 408 || resp.StatusCode == 429
		return false, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body)), isProxyErr
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read response body: %v", err), false
	}

	var deepInfraResp map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &deepInfraResp); err != nil {
		return false, fmt.Errorf("failed to parse response: %v", err), false
	}

	openAIResp := types.EmbeddingResponse{
		Object: "list",
		Model:  "",
		Data:   []types.EmbeddingData{},
		Usage:  types.EmbeddingUsage{},
	}

	if model, ok := deepInfraResp["model"].(string); ok {
		openAIResp.Model = model
	}

	if embeddings, ok := deepInfraResp["embeddings"].([]interface{}); ok {
		for i, emb := range embeddings {
			if embArr, ok := emb.([]interface{}); ok {
				floats := make([]float64, len(embArr))
				for j, v := range embArr {
					if f, ok := v.(float64); ok {
						floats[j] = f
					}
				}
				openAIResp.Data = append(openAIResp.Data, types.EmbeddingData{
					Object:    "embedding",
					Index:     i,
					Embedding: floats,
				})
			}
		}
	}

	if dataRaw, ok := deepInfraResp["data"].([]interface{}); ok {
		for i, d := range dataRaw {
			if dMap, ok := d.(map[string]interface{}); ok {
				embData := types.EmbeddingData{
					Object: "embedding",
					Index:  i,
				}
				if embedding, ok := dMap["embedding"].([]interface{}); ok {
					floats := make([]float64, len(embedding))
					for j, v := range embedding {
						if f, ok := v.(float64); ok {
							floats[j] = f
						}
					}
					embData.Embedding = floats
				}
				openAIResp.Data = append(openAIResp.Data, embData)
			}
		}
		openAIResp.Object = "list"
	}

	if usageRaw, ok := deepInfraResp["usage"].(map[string]interface{}); ok {
		if pt, ok := usageRaw["prompt_tokens"].(float64); ok {
			openAIResp.Usage.PromptTokens = int(pt)
		}
		if tt, ok := usageRaw["total_tokens"].(float64); ok {
			openAIResp.Usage.TotalTokens = int(tt)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(openAIResp)

	return true, nil, false
}

func CompletionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fmt.Printf("📝 Completions request from %s\n", r.RemoteAddr)

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		fmt.Printf("❌ Failed to read request body: %v\n", err)
		utils.SendErrorResponse(w, "Failed to read request body", "invalid_request_error", http.StatusBadRequest)
		return
	}
	r.Body.Close()

	var compReq types.CompletionRequest
	err = json.Unmarshal(bodyBytes, &compReq)
	if err != nil {
		fmt.Printf("❌ Failed to parse request: %v\n", err)
		utils.SendErrorResponse(w, "Failed to parse request body", "invalid_request_error", http.StatusBadRequest)
		return
	}

	if compReq.Prompt == nil {
		utils.SendErrorResponse(w, "Prompt is required", "invalid_request_error", http.StatusBadRequest)
		return
	}

	fmt.Printf("🤖 Model requested: %s\n", compReq.Model)

	if !services.IsModelSupported(compReq.Model) {
		fmt.Printf("❌ Unsupported model: %s\n", compReq.Model)
		utils.SendErrorResponse(w, "Unsupported model. Please use one of the supported models.", "invalid_request_error", http.StatusBadRequest, "model_not_found")
		return
	}

	if compReq.Temperature == 0 {
		compReq.Temperature = 0.7
	}
	if compReq.MaxTokens == 0 {
		compReq.MaxTokens = 15000
	}
	if compReq.TopP == 0 {
		compReq.TopP = 1.0
	}

	chatReq := types.ChatCompletionRequest{
		Model:       compReq.Model,
		Messages:    promptToMessages(compReq.Prompt),
		Temperature: compReq.Temperature,
		MaxTokens:   compReq.MaxTokens,
		TopP:        compReq.TopP,
		N:           compReq.N,
		Stream:      compReq.Stream,
		Stop:        compReq.Stop,
		User:        compReq.User,
	}

	data, err := json.Marshal(chatReq)
	if err != nil {
		fmt.Printf("❌ Failed to marshal request: %v\n", err)
		utils.SendErrorResponse(w, "Failed to marshal request", "internal_error", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	var lastErr error
	var currentProxy string

	for i := 0; i < services.MaxProxyAttempts; i++ {
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

			if i > 0 && !isTimeoutError(lastErr) {
				proxy = currentProxy
			} else {
				currentProxy = proxy
			}

			fmt.Printf("🌐 Attempt %d: Using proxy %s\n", i+1, proxy)

			result, err, isProxyErr := sendCompletionRequest(ctx, proxy, services.DeepInfraBaseURL+services.ChatEndpoint, data, compReq.Stream, w)
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
				fmt.Printf("✅ Completion successful using proxy %s (attempt %d)\n", proxy, i+1)
				return
			}
		}
	}

	utils.SendErrorResponse(w, "Unable to process the request after multiple attempts", "internal_error", http.StatusInternalServerError)
}

func promptToMessages(prompt interface{}) []types.ChatMessage {
	var messages []types.ChatMessage

	switch p := prompt.(type) {
	case string:
		messages = append(messages, types.ChatMessage{
			Role:    "user",
			Content: p,
		})
	case []interface{}:
		for _, item := range p {
			if str, ok := item.(string); ok {
				messages = append(messages, types.ChatMessage{
					Role:    "user",
					Content: str,
				})
			}
		}
	}

	return messages
}

func sendCompletionRequest(ctx context.Context, proxy, endpoint string, data []byte, isStream bool, w http.ResponseWriter) (bool, error, bool) {
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

	resp, err := client.Do(req)
	if err != nil {
		return false, err, true
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		isProxyErr := resp.StatusCode >= 500 || resp.StatusCode == 408 || resp.StatusCode == 429
		return false, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body)), isProxyErr
	}

	if isStream {
		ok, streamErr := handleCompletionStreamResponse(w, resp)
		return ok, streamErr, false
	}
	ok, normalErr := handleCompletionNormalResponse(w, resp)
	return ok, normalErr, false
}

func handleCompletionStreamResponse(w http.ResponseWriter, resp *http.Response) (bool, error) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	responseID := generateCompletionID()
	modelName := ""

	flusher, ok := w.(http.Flusher)
	if !ok {
		return false, fmt.Errorf("response writer does not support flushing")
	}

	buf := make([]byte, 64*1024)
	reader := io.Reader(resp.Body)
	chunkCount := 0

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			line := string(buf[:n])
			lines := splitLines(line)
			for _, l := range lines {
				if l == "" {
					continue
				}

				var dataStr string
				if strings.HasPrefix(l, "data: ") {
					dataStr = strings.TrimPrefix(l, "data: ")
				} else {
					dataStr = l
				}

				if dataStr == "[DONE]" {
					fmt.Fprintf(w, "data: [DONE]\n\n")
					flusher.Flush()
					chunkCount++
					continue
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

				completionChunk := map[string]interface{}{
					"id":      responseID,
					"object":  "text_completion",
					"created": time.Now().Unix(),
					"model":   modelName,
					"choices": []interface{}{},
				}

				if choices, ok := streamChunk["choices"].([]interface{}); ok {
					for _, choiceRaw := range choices {
						if choice, ok := choiceRaw.(map[string]interface{}); ok {
							completionChoice := map[string]interface{}{
								"text":          "",
								"index":         choice["index"],
								"logprobs":      nil,
								"finish_reason": choice["finish_reason"],
							}

							if delta, ok := choice["delta"].(map[string]interface{}); ok {
								if content, ok := delta["content"].(string); ok {
									completionChoice["text"] = content
								}
							}

							completionChunk["choices"] = append(completionChunk["choices"].([]interface{}), completionChoice)
						}
					}
				}

				chunkBytes, _ := json.Marshal(completionChunk)
				fmt.Fprintf(w, "data: %s\n\n", string(chunkBytes))
				flusher.Flush()
				chunkCount++
			}
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return false, err
		}
	}

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()

	fmt.Printf("✅ Completion stream complete, sent %d chunks\n", chunkCount)
	return true, nil
}

func handleCompletionNormalResponse(w http.ResponseWriter, resp *http.Response) (bool, error) {
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read response body: %v", err)
	}

	var chatResp map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return false, fmt.Errorf("failed to parse response: %v", err)
	}

	responseID := generateCompletionID()
	modelName := ""
	if model, ok := chatResp["model"].(string); ok {
		modelName = model
	}

	completionResp := types.CompletionResponse{
		ID:      responseID,
		Object:  "text_completion",
		Created: time.Now().Unix(),
		Model:   modelName,
		Choices: []types.CompletionChoice{},
	}

	if choices, ok := chatResp["choices"].([]interface{}); ok {
		for _, choiceRaw := range choices {
			if choice, ok := choiceRaw.(map[string]interface{}); ok {
				text := ""
				if msg, ok := choice["message"].(map[string]interface{}); ok {
					if content, ok := msg["content"].(string); ok {
						text = content
					}
				}

				finishReason := ""
				if fr, ok := choice["finish_reason"].(string); ok {
					finishReason = fr
				}

				completionResp.Choices = append(completionResp.Choices, types.CompletionChoice{
					Text:         text,
					Index:        0,
					FinishReason: finishReason,
				})
			}
		}
	}

	if usageRaw, ok := chatResp["usage"].(map[string]interface{}); ok {
		completionResp.Usage = &types.ChatCompletionUsage{}
		if pt, ok := usageRaw["prompt_tokens"].(float64); ok {
			completionResp.Usage.PromptTokens = int(pt)
		}
		if ct, ok := usageRaw["completion_tokens"].(float64); ok {
			completionResp.Usage.CompletionTokens = int(ct)
		}
		if tt, ok := usageRaw["total_tokens"].(float64); ok {
			completionResp.Usage.TotalTokens = int(tt)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(completionResp)

	return true, nil
}

func generateCompletionID() string {
	b := make([]byte, 12)
	rand.Read(b)
	return "cmpl-" + hex.EncodeToString(b)
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(s, "\n")
}
