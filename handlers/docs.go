package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"

	"deepinfra-wrapper/services"
)

func SwaggerHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("📚 Serving Swagger UI for %s\n", r.RemoteAddr)

	const swaggerTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>DeepInfra OpenAI API Proxy - Swagger UI</title>
  <link rel="stylesheet" type="text/css" href="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/4.18.3/swagger-ui.css" />
  <style>
    html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
    *, *:before, *:after { box-sizing: inherit; }
    body { margin: 0; background: #fafafa; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/4.18.3/swagger-ui-bundle.js" charset="UTF-8"></script>
  <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/4.18.3/swagger-ui-standalone-preset.js" charset="UTF-8"></script>
  <script>
    window.onload = function() {
      const ui = SwaggerUIBundle({
        url: "/openapi.json",
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIStandalonePreset
        ],
        layout: "StandaloneLayout"
      });
      window.ui = ui;
    };
  </script>
</body>
</html>`

	tmpl, err := template.New("swagger").Parse(swaggerTemplate)
	if err != nil {
		http.Error(w, "Error generating Swagger UI", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	tmpl.Execute(w, nil)
}

func OpenAPIHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("📄 Serving OpenAPI JSON for %s\n", r.RemoteAddr)

	models := services.GetSupportedModels()

	modelEnum := make([]interface{}, len(models))
	for i, model := range models {
		modelEnum[i] = model
	}

	securitySchemes := map[string]interface{}{}
	security := []map[string]interface{}{}

	if services.IsAuthEnabled() {
		securitySchemes["ApiKeyAuth"] = map[string]interface{}{
			"type":         "http",
			"scheme":       "bearer",
			"bearerFormat": "API key",
		}
		security = []map[string]interface{}{
			{
				"ApiKeyAuth": []string{},
			},
		}
	}

	openAPISpec := map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":       "DeepInfra OpenAI API Proxy",
			"description": "A proxy service for DeepInfra's OpenAI compatible API",
			"version":     "1.0.0",
		},
		"servers": []map[string]interface{}{
			{
				"url": "/",
			},
		},
		"paths": map[string]interface{}{
			"/v1/chat/completions": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Create a chat completion",
					"operationId": "createChatCompletion",
					"security":    security,
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/ChatCompletionRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Successful response",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
									},
								},
							},
						},
						"400": map[string]interface{}{
							"description": "Bad request",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
						"500": map[string]interface{}{
							"description": "Internal server error",
						},
					},
				},
			},
			"/v1/images/generations": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Create an image generation",
					"operationId": "createImageGeneration",
					"security":    security,
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/ImageGenerationRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Successful response",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/ImageGenerationResponse",
									},
								},
							},
						},
						"400": map[string]interface{}{
							"description": "Bad request",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
						"500": map[string]interface{}{
							"description": "Internal server error",
						},
					},
				},
			},
			"/v1/models": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "List available models (OpenAI compatible)",
					"operationId": "listModelsV1",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Successful response",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/OpenAIModelsResponse",
									},
								},
							},
						},
						"405": map[string]interface{}{
							"description": "Method not allowed",
						},
					},
				},
			},
			"/v1/models/{model_id}": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Get model information",
					"operationId": "getModel",
					"parameters": []map[string]interface{}{
						{
							"name":     "model_id",
							"in":       "path",
							"required": true,
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Successful response",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/OpenAIModel",
									},
								},
							},
						},
						"404": map[string]interface{}{
							"description": "Model not found",
						},
						"405": map[string]interface{}{
							"description": "Method not allowed",
						},
					},
				},
			},
			"/models": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "List available models",
					"operationId": "listModels",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Successful response",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "array",
										"items": map[string]interface{}{
											"type": "string",
										},
									},
								},
							},
						},
					},
				},
			},
			"/v1/embeddings": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Create embeddings",
					"operationId": "createEmbeddings",
					"security":    security,
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/EmbeddingRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Successful response",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/EmbeddingResponse",
									},
								},
							},
						},
						"400": map[string]interface{}{
							"description": "Bad request",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
			"/v1/completions": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Create completions (legacy)",
					"operationId": "createCompletions",
					"security":    security,
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/CompletionRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Successful response",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/CompletionResponse",
									},
								},
							},
						},
						"400": map[string]interface{}{
							"description": "Bad request",
						},
						"401": map[string]interface{}{
							"description": "Unauthorized",
						},
					},
				},
			},
		},
		"components": map[string]interface{}{
			"schemas": map[string]interface{}{
				"ChatCompletionRequest": map[string]interface{}{
					"type": "object",
					"required": []string{
						"model",
						"messages",
					},
					"properties": map[string]interface{}{
						"model": map[string]interface{}{
							"type": "string",
							"enum": modelEnum,
						},
						"messages": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"$ref": "#/components/schemas/ChatMessage",
							},
						},
						"stream": map[string]interface{}{
							"type":    "boolean",
							"default": false,
						},
						"temperature": map[string]interface{}{
							"type":    "number",
							"format":  "float",
							"minimum": 0,
							"maximum": 2,
							"default": 0.7,
						},
						"max_tokens": map[string]interface{}{
							"type":    "integer",
							"minimum": 1,
							"default": 15000,
						},
						"top_p": map[string]interface{}{
							"type":    "number",
							"format":  "float",
							"minimum": 0,
							"maximum": 1,
							"default": 1.0,
						},
						"presence_penalty": map[string]interface{}{
							"type":    "number",
							"format":  "float",
							"minimum": -2,
							"maximum": 2,
							"default": 0,
						},
						"frequency_penalty": map[string]interface{}{
							"type":    "number",
							"format":  "float",
							"minimum": -2,
							"maximum": 2,
							"default": 0,
						},
						"stop": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type": "string",
							},
						},
						"tools": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"$ref": "#/components/schemas/Tool",
							},
						},
						"tool_choice": map[string]interface{}{
							"oneOf": []map[string]interface{}{
								{
									"type": "string",
									"enum": []string{"none", "auto", "required"},
								},
								{
									"type": "object",
								},
							},
						},
						"response_format": map[string]interface{}{
							"$ref": "#/components/schemas/ResponseFormat",
						},
						"n": map[string]interface{}{
							"type":    "integer",
							"minimum": 1,
							"default": 1,
						},
						"user": map[string]interface{}{
							"type": "string",
						},
						"seed": map[string]interface{}{
							"type": "integer",
						},
						"logprobs": map[string]interface{}{
							"type":    "boolean",
							"default": false,
						},
						"top_logprobs": map[string]interface{}{
							"type":    "integer",
							"minimum": 0,
							"maximum": 20,
						},
					},
				},
				"ChatMessage": map[string]interface{}{
					"type": "object",
					"required": []string{
						"role",
						"content",
					},
					"properties": map[string]interface{}{
						"role": map[string]interface{}{
							"type": "string",
							"enum": []string{
								"system",
								"user",
								"assistant",
							},
						},
						"content": map[string]interface{}{
							"type": "string",
						},
					},
				},
				"OpenAIModel": map[string]interface{}{
					"type": "object",
					"required": []string{
						"id",
						"object",
						"created",
						"owned_by",
					},
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "string",
							"description": "The model identifier",
						},
						"object": map[string]interface{}{
							"type":        "string",
							"description": "The object type, always 'model'",
							"example":     "model",
						},
						"created": map[string]interface{}{
							"type":        "integer",
							"format":      "int64",
							"description": "The Unix timestamp when the model was created",
						},
						"owned_by": map[string]interface{}{
							"type":        "string",
							"description": "The organization that owns the model",
						},
					},
				},
				"OpenAIModelsResponse": map[string]interface{}{
					"type": "object",
					"required": []string{
						"object",
						"data",
					},
					"properties": map[string]interface{}{
						"object": map[string]interface{}{
							"type":        "string",
							"description": "The object type, always 'list'",
							"example":     "list",
						},
						"data": map[string]interface{}{
							"type":        "array",
							"description": "List of available models",
							"items": map[string]interface{}{
								"$ref": "#/components/schemas/OpenAIModel",
							},
						},
					},
				},
				"ImageGenerationRequest": map[string]interface{}{
					"type": "object",
					"required": []string{
						"prompt",
					},
					"properties": map[string]interface{}{
						"prompt": map[string]interface{}{
							"type":        "string",
							"description": "A text description of the desired image(s)",
						},
						"model": map[string]interface{}{
							"type":        "string",
							"description": "The model to use for image generation",
							"default":     "stabilityai/stable-diffusion-xl-base-1.0",
						},
						"n": map[string]interface{}{
							"type":        "integer",
							"description": "The number of images to generate",
							"minimum":     1,
							"maximum":     10,
							"default":     1,
						},
						"size": map[string]interface{}{
							"type":        "string",
							"description": "The size of the generated images",
							"enum":        []string{"256x256", "512x512", "1024x1024", "1024x1792", "1792x1024"},
							"default":     "1024x1024",
						},
						"quality": map[string]interface{}{
							"type":        "string",
							"description": "The quality of the image",
							"enum":        []string{"standard", "hd"},
							"default":     "standard",
						},
						"style": map[string]interface{}{
							"type":        "string",
							"description": "The style of the generated images",
							"enum":        []string{"vivid", "natural"},
							"default":     "vivid",
						},
						"response_format": map[string]interface{}{
							"type":        "string",
							"description": "The format in which the generated images are returned",
							"enum":        []string{"url", "b64_json"},
							"default":     "url",
						},
						"user": map[string]interface{}{
							"type":        "string",
							"description": "A unique identifier representing your end-user",
						},
					},
				},
				"ImageGenerationResponse": map[string]interface{}{
					"type": "object",
					"required": []string{
						"created",
						"data",
					},
					"properties": map[string]interface{}{
						"created": map[string]interface{}{
							"type":        "integer",
							"format":      "int64",
							"description": "The Unix timestamp when the image was created",
						},
						"data": map[string]interface{}{
							"type":        "array",
							"description": "List of generated images",
							"items": map[string]interface{}{
								"$ref": "#/components/schemas/ImageGenerationData",
							},
						},
					},
				},
				"ImageGenerationData": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"url": map[string]interface{}{
							"type":        "string",
							"description": "The URL of the generated image",
						},
						"b64_json": map[string]interface{}{
							"type":        "string",
							"description": "The base64-encoded JSON of the generated image",
						},
						"revised_prompt": map[string]interface{}{
							"type":        "string",
							"description": "The revised prompt used for image generation",
						},
					},
				},
				"Tool": map[string]interface{}{
					"type": "object",
					"required": []string{
						"type",
					},
					"properties": map[string]interface{}{
						"type": map[string]interface{}{
							"type": "string",
							"enum": []string{"function"},
						},
						"function": map[string]interface{}{
							"$ref": "#/components/schemas/Function",
						},
					},
				},
				"Function": map[string]interface{}{
					"type": "object",
					"required": []string{
						"name",
					},
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "The name of the function",
						},
						"description": map[string]interface{}{
							"type":        "string",
							"description": "A description of the function",
						},
						"parameters": map[string]interface{}{
							"type":        "object",
							"description": "The parameters the function accepts",
						},
					},
				},
				"ResponseFormat": map[string]interface{}{
					"type": "object",
					"required": []string{
						"type",
					},
					"properties": map[string]interface{}{
						"type": map[string]interface{}{
							"type": "string",
							"enum": []string{"text", "json_object"},
						},
					},
				},
				"ChatCompletionResponse": map[string]interface{}{
					"type":     "object",
					"required": []string{"id", "object", "created", "model", "choices"},
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "string",
							"description": "A unique identifier for the chat completion",
						},
						"object": map[string]interface{}{
							"type":        "string",
							"description": "The object type, always chat.completion",
						},
						"created": map[string]interface{}{
							"type":        "integer",
							"description": "The Unix timestamp of creation",
						},
						"model": map[string]interface{}{
							"type":        "string",
							"description": "The model used for completion",
						},
						"choices": map[string]interface{}{
							"type":        "array",
							"description": "List of completion choices",
							"items": map[string]interface{}{
								"$ref": "#/components/schemas/ChatCompletionChoice",
							},
						},
						"usage": map[string]interface{}{
							"$ref": "#/components/schemas/ChatCompletionUsage",
						},
					},
				},
				"ChatCompletionChoice": map[string]interface{}{
					"type":     "object",
					"required": []string{"index", "message", "finish_reason"},
					"properties": map[string]interface{}{
						"index": map[string]interface{}{
							"type": "integer",
						},
						"message": map[string]interface{}{
							"$ref": "#/components/schemas/ChatMessage",
						},
						"finish_reason": map[string]interface{}{
							"type": "string",
							"enum": []string{"stop", "length", "content_filter"},
						},
					},
				},
				"ChatCompletionUsage": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"prompt_tokens": map[string]interface{}{
							"type": "integer",
						},
						"completion_tokens": map[string]interface{}{
							"type": "integer",
						},
						"total_tokens": map[string]interface{}{
							"type": "integer",
						},
					},
				},
				"EmbeddingRequest": map[string]interface{}{
					"type":     "object",
					"required": []string{"model", "input"},
					"properties": map[string]interface{}{
						"model": map[string]interface{}{
							"type": "string",
						},
						"input": map[string]interface{}{
							"oneOf": []map[string]interface{}{
								{"type": "string"},
								{"type": "array", "items": map[string]interface{}{"type": "string"}},
							},
						},
						"encoding_format": map[string]interface{}{
							"type": "string",
							"enum": []string{"float", "base64"},
						},
						"dimensions": map[string]interface{}{
							"type": "integer",
						},
					},
				},
				"EmbeddingResponse": map[string]interface{}{
					"type":     "object",
					"required": []string{"object", "data", "model", "usage"},
					"properties": map[string]interface{}{
						"object": map[string]interface{}{
							"type": "string",
						},
						"data": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"$ref": "#/components/schemas/EmbeddingData",
							},
						},
						"model": map[string]interface{}{
							"type": "string",
						},
						"usage": map[string]interface{}{
							"$ref": "#/components/schemas/EmbeddingUsage",
						},
					},
				},
				"EmbeddingData": map[string]interface{}{
					"type":     "object",
					"required": []string{"object", "index", "embedding"},
					"properties": map[string]interface{}{
						"object": map[string]interface{}{
							"type": "string",
						},
						"index": map[string]interface{}{
							"type": "integer",
						},
						"embedding": map[string]interface{}{
							"type":  "array",
							"items": map[string]interface{}{"type": "number"},
						},
					},
				},
				"EmbeddingUsage": map[string]interface{}{
					"type":     "object",
					"required": []string{"prompt_tokens", "total_tokens"},
					"properties": map[string]interface{}{
						"prompt_tokens": map[string]interface{}{
							"type": "integer",
						},
						"total_tokens": map[string]interface{}{
							"type": "integer",
						},
					},
				},
				"CompletionRequest": map[string]interface{}{
					"type":     "object",
					"required": []string{"model", "prompt"},
					"properties": map[string]interface{}{
						"model": map[string]interface{}{
							"type": "string",
						},
						"prompt": map[string]interface{}{
							"oneOf": []map[string]interface{}{
								{"type": "string"},
								{"type": "array", "items": map[string]interface{}{"type": "string"}},
							},
						},
						"max_tokens": map[string]interface{}{
							"type": "integer",
						},
						"temperature": map[string]interface{}{
							"type": "number",
						},
						"top_p": map[string]interface{}{
							"type": "number",
						},
						"n": map[string]interface{}{
							"type": "integer",
						},
						"stream": map[string]interface{}{
							"type": "boolean",
						},
						"stop": map[string]interface{}{
							"oneOf": []map[string]interface{}{
								{"type": "string"},
								{"type": "array", "items": map[string]interface{}{"type": "string"}},
							},
						},
					},
				},
				"CompletionResponse": map[string]interface{}{
					"type":     "object",
					"required": []string{"id", "object", "created", "model", "choices"},
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type": "string",
						},
						"object": map[string]interface{}{
							"type": "string",
						},
						"created": map[string]interface{}{
							"type": "integer",
						},
						"model": map[string]interface{}{
							"type": "string",
						},
						"choices": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"$ref": "#/components/schemas/CompletionChoice",
							},
						},
						"usage": map[string]interface{}{
							"$ref": "#/components/schemas/ChatCompletionUsage",
						},
					},
				},
				"CompletionChoice": map[string]interface{}{
					"type":     "object",
					"required": []string{"text", "index", "finish_reason"},
					"properties": map[string]interface{}{
						"text": map[string]interface{}{
							"type": "string",
						},
						"index": map[string]interface{}{
							"type": "integer",
						},
						"logprobs": map[string]interface{}{
							"type": "object",
						},
						"finish_reason": map[string]interface{}{
							"type": "string",
						},
					},
				},
			},
			"securitySchemes": securitySchemes,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(openAPISpec)
}
