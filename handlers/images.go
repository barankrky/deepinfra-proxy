package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"deepinfra-wrapper/services"
	"deepinfra-wrapper/types"
	"deepinfra-wrapper/utils"
)

var imageSemaphore = make(chan struct{}, 50)

func ImageGenerationsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	select {
	case imageSemaphore <- struct{}{}:
		defer func() { <-imageSemaphore }()
	default:
		utils.SendErrorResponse(w, "Server is experiencing high load. Please try again later.", "rate_limit_error", http.StatusTooManyRequests)
		return
	}

	fmt.Printf("🎨 Image generation request from %s\n", r.RemoteAddr)

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		fmt.Printf("❌ Failed to read request body: %v\n", err)
		utils.SendErrorResponse(w, "Failed to read request body", "invalid_request_error", http.StatusBadRequest)
		return
	}
	r.Body.Close()

	var imageReq types.ImageGenerationRequest
	err = json.Unmarshal(bodyBytes, &imageReq)
	if err != nil {
		fmt.Printf("❌ Failed to parse request: %v\n", err)
		utils.SendErrorResponse(w, "Failed to parse request body", "invalid_request_error", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if imageReq.Prompt == "" {
		utils.SendErrorResponse(w, "Prompt is required", "invalid_request_error", http.StatusBadRequest)
		return
	}

	// Set defaults
	if imageReq.N == 0 {
		imageReq.N = 1
	}
	if imageReq.Size == "" {
		imageReq.Size = "1024x1024"
	}
	if imageReq.Model == "" {
		// Use a default image model
		imageReq.Model = "stabilityai/stable-diffusion-xl-base-1.0"
	}

	fmt.Printf("🤖 Image model requested: %s\n", imageReq.Model)

	if !services.IsModelSupported(imageReq.Model) {
		fmt.Printf("❌ Unsupported model: %s\n", imageReq.Model)
		utils.SendErrorResponse(w, "Unsupported model. Please use one of the supported models.", "invalid_request_error", http.StatusBadRequest, "model_not_found")
		return
	}

	// Convert to DeepInfra format
	deepInfraReq := map[string]interface{}{
		"prompt":     imageReq.Prompt,
		"num_images": imageReq.N,
	}

	// Map size to DeepInfra format
	if imageReq.Size != "" {
		deepInfraReq["width"] = getWidthFromSize(imageReq.Size)
		deepInfraReq["height"] = getHeightFromSize(imageReq.Size)
	}

	data, err := json.Marshal(deepInfraReq)
	if err != nil {
		fmt.Printf("❌ Failed to marshal request: %v\n", err)
		utils.SendErrorResponse(w, "Failed to marshal request", "internal_error", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	success := false
	var lastErr error
	var currentProxy string

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

			if i > 0 && !isTimeoutError(lastErr) {
				proxy = currentProxy
			} else {
				currentProxy = proxy
			}

			fmt.Printf("🌐 Attempt %d: Using proxy %s\n", i+1, proxy)

			result, err, isProxyErr := sendImageRequest(ctx, proxy, services.DeepInfraBaseURL+services.ImageEndpoint, data, w)
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
				fmt.Printf("✅ Image generation successful using proxy %s (attempt %d)\n", proxy, i+1)
				success = true
				break
			}
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

func sendImageRequest(ctx context.Context, proxy, endpoint string, data []byte, w http.ResponseWriter) (bool, error, bool) {
	proxyURL, err := url.Parse("http://" + proxy)
	if err != nil {
		return false, err, true
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
		Timeout: 90 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(data))
	if err != nil {
		return false, err, false
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Deepinfra-Source", "web-page")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/92.0.4515.107 Safari/537.36")

	fmt.Printf("📡 Sending image request to %s via proxy %s\n", endpoint, proxy)
	resp, err := client.Do(req)
	if err != nil {
		return false, err, true
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		ok, imgErr := handleImageResponse(w, resp)
		return ok, imgErr, false
	}

	body, _ := io.ReadAll(resp.Body)
	isProxyErr := resp.StatusCode >= 500 || resp.StatusCode == 408 || resp.StatusCode == 429
	return false, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body)), isProxyErr
}

func handleImageResponse(w http.ResponseWriter, resp *http.Response) (bool, error) {
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read response body: %v", err)
	}

	// Parse DeepInfra response
	var deepInfraResp map[string]interface{}
	err = json.Unmarshal(bodyBytes, &deepInfraResp)
	if err != nil {
		return false, fmt.Errorf("failed to parse response: %v", err)
	}

	// Convert to OpenAI format
	openAIResp := types.ImageGenerationResponse{
		Created: time.Now().Unix(),
		Data:    []types.ImageGenerationData{},
	}

	// Extract images from DeepInfra response
	if images, ok := deepInfraResp["images"].([]interface{}); ok {
		for _, img := range images {
			if imgStr, ok := img.(string); ok {
				// DeepInfra returns base64 images
				openAIResp.Data = append(openAIResp.Data, types.ImageGenerationData{
					B64JSON: imgStr,
				})
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(openAIResp)

	fmt.Printf("✅ Image response sent successfully (%d images)\n", len(openAIResp.Data))
	return true, nil
}

func getWidthFromSize(size string) int {
	switch size {
	case "256x256":
		return 256
	case "512x512":
		return 512
	case "1024x1024":
		return 1024
	case "1024x1792", "1792x1024":
		return 1024
	default:
		return 1024
	}
}

func getHeightFromSize(size string) int {
	switch size {
	case "256x256":
		return 256
	case "512x512":
		return 512
	case "1024x1024":
		return 1024
	case "1024x1792":
		return 1792
	case "1792x1024":
		return 1024
	default:
		return 1024
	}
}
