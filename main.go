package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"deepinfra-wrapper/handlers"
	"deepinfra-wrapper/services"
)

func main() {
	fmt.Println("🚀 Starting DeepInfra proxy service...")

	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		fmt.Println("⚠️  Warning: API_KEY environment variable not set. Authentication will be disabled.")
	} else {
		fmt.Println("🔐 API key authentication enabled")
	}

	services.InitAPIKey(apiKey)

	initReady := make(chan bool)
	go initializeServices(initReady)

	<-initReady

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", handlers.CORSMiddleware(handlers.AuthMiddleware(handlers.ChatCompletionsHandler)))
	mux.HandleFunc("/v1/models", handlers.CORSMiddleware(handlers.AuthMiddleware(handlers.OpenAIModelsHandler)))
	mux.HandleFunc("/v1/models/", handlers.CORSMiddleware(handlers.AuthMiddleware(handlers.OpenAIModelHandler)))
	mux.HandleFunc("/v1/images/generations", handlers.CORSMiddleware(handlers.AuthMiddleware(handlers.ImageGenerationsHandler)))
	mux.HandleFunc("/v1/embeddings", handlers.CORSMiddleware(handlers.AuthMiddleware(handlers.EmbeddingsHandler)))
	mux.HandleFunc("/v1/completions", handlers.CORSMiddleware(handlers.AuthMiddleware(handlers.CompletionsHandler)))
	mux.HandleFunc("/models", handlers.CORSMiddleware(handlers.ModelsHandler))
	mux.HandleFunc("/docs", handlers.CORSMiddleware(handlers.SwaggerHandler))
	mux.HandleFunc("/openapi.json", handlers.CORSMiddleware(handlers.OpenAPIHandler))
	mux.HandleFunc("/health", handlers.CORSMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		fmt.Printf("✅ Server started on port %s\n", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Server error: %v", err)
		}
	}()

	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)
	<-shutdownChan

	fmt.Println("🛑 Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("❌ Server shutdown error: %v", err)
	}

	fmt.Println("👋 Server shutdown complete")
}

func initializeServices(ready chan<- bool) {
	fmt.Println("🔄 Initializing services...")

	fmt.Println("🔍 Searching for working proxies...")
	services.UpdateWorkingProxies()

	proxyCount := services.GetProxyCount()
	retries := 0

	for proxyCount == 0 && retries < 3 {
		fmt.Println("⚠️  No working proxies found. Retrying...")
		retries++
		time.Sleep(time.Duration(retries) * time.Second)
		services.UpdateWorkingProxies()
		proxyCount = services.GetProxyCount()
	}

	if proxyCount == 0 {
		fmt.Println("⚠️  Warning: Could not find working proxies. Service may not function correctly.")
	} else {
		fmt.Printf("✅ Found %d working proxies\n", proxyCount)
	}

	fmt.Println("🔍 Discovering supported models...")
	services.UpdateSupportedModels()

	modelCount := services.GetModelCount()
	retries = 0

	for modelCount == 0 && retries < 3 {
		fmt.Println("⚠️  No supported models found. Retrying...")
		retries++
		time.Sleep(time.Duration(retries) * time.Second)
		services.UpdateSupportedModels()
		modelCount = services.GetModelCount()
	}

	if modelCount == 0 {
		fmt.Println("⚠️  Warning: Could not find supported models. Service may not function correctly.")
	} else {
		fmt.Printf("✅ Found %d supported models\n", modelCount)
	}

	go manageProxiesAndModels()

	ready <- true

	fmt.Println("🎉 Service is ready to use")
}

func manageProxiesAndModels() {
	proxyTicker := time.NewTicker(services.ProxyUpdateTime)
	modelsTicker := time.NewTicker(services.ModelsUpdateTime)

	for {
		select {
		case <-proxyTicker.C:
			fmt.Println("🔄 Refreshing proxy list...")
			oldCount := services.GetProxyCount()
			services.UpdateWorkingProxies()
			newCount := services.GetProxyCount()
			fmt.Printf("✅ Proxy refresh complete: %d → %d working proxies\n", oldCount, newCount)
		case <-modelsTicker.C:
			fmt.Println("🔄 Refreshing models list...")
			oldCount := services.GetModelCount()
			services.UpdateSupportedModels()
			newCount := services.GetModelCount()
			fmt.Printf("✅ Models refresh complete: %d → %d supported models\n", oldCount, newCount)
		}
	}
}
