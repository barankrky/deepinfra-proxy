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
	"sync"
	"time"

	"deepinfra-wrapper/services"
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

	var chatReq map[string]interface{}
	err = json.Unmarshal(bodyBytes, &chatReq)
	if err != nil {
		fmt.Printf("❌ Failed to parse request: %v\n", err)
		utils.SendErrorResponse(w, "Failed to parse request body", "invalid_request_error", http.StatusBadRequest)
		return
	}

	model, _ := chatReq["model"].(string)
	fmt.Printf("🤖 Model requested: %s\n", model)

	if !services.IsModelSupported(model) {
		fmt.Printf("❌ Unsupported model: %s\n", model)
		utils.SendErrorResponse(w, "Unsupported model. Please use one of the supported models.", "invalid_request_error", http.StatusBadRequest, "model_not_found")
		return
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
	usedProxies := make(map[string]bool)
	var mu sync.Mutex

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

			mu.Lock()
			if usedProxies[proxy] {
				mu.Unlock()
				continue
			}
			usedProxies[proxy] = true
			mu.Unlock()

			fmt.Printf("🌐 Attempt %d: Using proxy %s\n", i+1, proxy)

			result, err := sendChatRequest(ctx, proxy, services.DeepInfraBaseURL+services.ChatEndpoint, data, true, w)
			if err != nil {
				fmt.Printf("❌ Proxy attempt %d failed: %v\n", i+1, err)
				services.RemoveProxy(proxy)
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

func sendChatRequest(ctx context.Context, proxy, endpoint string, data []byte, isStream bool, w http.ResponseWriter) (bool, error) {
	proxyURL, err := url.Parse("http://" + proxy)
	if err != nil {
		return false, err
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
		Timeout: 60 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(data))
	if err != nil {
		return false, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Deepinfra-Source", "web-page")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/92.0.4515.107 Safari/537.36")

	fmt.Printf("📡 Sending request to %s via proxy %s\n", endpoint, proxy)
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	debugText("Upstream response status", fmt.Sprintf("status=%d proxy=%s", resp.StatusCode, proxy))

	if resp.StatusCode == http.StatusOK {
		fmt.Println("📶 Handling streaming response")
		return handleStreamResponse(w, resp)
	}

	body, _ := io.ReadAll(resp.Body)
	debugPayload("Upstream error body", body)
	return false, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
}

func handleStreamResponse(w http.ResponseWriter, resp *http.Response) (bool, error) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return false, fmt.Errorf("response writer does not support flushing")
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("❌ Stream error: %v\n", err)
		return false, err
	}

	fmt.Println("✅ Stream complete")
	return true, nil
}
