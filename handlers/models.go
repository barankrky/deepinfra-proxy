package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"deepinfra-wrapper/services"
	"deepinfra-wrapper/types"
	"deepinfra-wrapper/utils"
)

func ModelsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	fmt.Printf("📋 Handling models request from %s\n", r.RemoteAddr)
	models := services.GetSupportedModels()
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(models)
	fmt.Printf("✅ Returned %d models\n", len(models))
}

// OpenAI-compatible /v1/models endpoint
func OpenAIModelsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	fmt.Printf("📋 Handling OpenAI-compatible models request from %s\n", r.RemoteAddr)
	modelInfos := services.GetAllModelInfo()
	
	// Convert to OpenAI-compatible format
	openAIModels := make([]types.OpenAIModel, len(modelInfos))
	
	for i, modelInfo := range modelInfos {
		openAIModels[i] = types.OpenAIModel{
			ID:      modelInfo.ID,
			Object:  "model",
			Created: modelInfo.Created,
			OwnedBy: modelInfo.OwnedBy,
		}
	}
	
	response := types.OpenAIModelsResponse{
		Object: "list",
		Data:   openAIModels,
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
	fmt.Printf("✅ Returned %d models in OpenAI format\n", len(modelInfos))
}

// Individual model info endpoint
func OpenAIModelHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	// Extract model ID from URL path
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 4 {
		utils.SendErrorResponse(w, "Invalid model ID", "invalid_request_error", http.StatusBadRequest)
		return
	}
	
	// The model ID is everything after /v1/models/
	modelID := strings.Join(pathParts[3:], "/")
	fmt.Printf("📋 Handling individual model request for: %s\n", modelID)
	
	modelInfo, exists := services.GetModelInfo(modelID)
	if !exists {
		utils.SendErrorResponse(w, "Model not found", "invalid_request_error", http.StatusNotFound, "model_not_found")
		return
	}
	
	// Convert to OpenAI-compatible format
	openAIModel := types.OpenAIModel{
		ID:      modelInfo.ID,
		Object:  "model",
		Created: modelInfo.Created,
		OwnedBy: modelInfo.OwnedBy,
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(openAIModel)
	fmt.Printf("✅ Returned model info for: %s\n", modelID)
}