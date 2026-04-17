package types

type ChatCompletionRequest struct {
	Model            string          `json:"model"`
	Messages         []ChatMessage   `json:"messages"`
	Stream           bool            `json:"stream"`
	Temperature      float64         `json:"temperature,omitempty"`
	MaxTokens        int             `json:"max_tokens,omitempty"`
	TopP             float64         `json:"top_p,omitempty"`
	N                int             `json:"n,omitempty"`
	PresencePenalty  float64         `json:"presence_penalty,omitempty"`
	FrequencyPenalty float64         `json:"frequency_penalty,omitempty"`
	Stop             interface{}     `json:"stop,omitempty"`
	Tools            []Tool          `json:"tools,omitempty"`
	ToolChoice       interface{}     `json:"tool_choice,omitempty"`
	ResponseFormat   *ResponseFormat `json:"response_format,omitempty"`
	User             string          `json:"user,omitempty"`
	Seed             *int            `json:"seed,omitempty"`
	Logprobs         bool            `json:"logprobs,omitempty"`
	TopLogprobs      int             `json:"top_logprobs,omitempty"`
}

type ChatMessage struct {
	Role       string      `json:"role"`
	Content    interface{} `json:"content"` // Can be string or array of ContentPart
	Name       string      `json:"name,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
}

type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

type ImageURL struct {
	URL string `json:"url"`
}

type Tool struct {
	Type     string    `json:"type"`
	Function *Function `json:"function,omitempty"`
}

type Function struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ResponseFormat struct {
	Type string `json:"type"`
}

// Image Generation Types
type ImageGenerationRequest struct {
	Prompt         string `json:"prompt"`
	Model          string `json:"model,omitempty"`
	N              int    `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	Quality        string `json:"quality,omitempty"`
	Style          string `json:"style,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
	User           string `json:"user,omitempty"`
}

type ImageGenerationResponse struct {
	Created int64                 `json:"created"`
	Data    []ImageGenerationData `json:"data"`
}

type ImageGenerationData struct {
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// Enhanced Error Response
type OpenAIErrorResponse struct {
	Error OpenAIErrorDetail `json:"error"`
}

type OpenAIErrorDetail struct {
	Message string      `json:"message"`
	Type    string      `json:"type"`
	Code    interface{} `json:"code,omitempty"`
	Param   string      `json:"param,omitempty"`
}

type OpenAIError struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

type ModelResponse struct {
	Object string `json:"object"`
	Data   []struct {
		ID     string `json:"id"`
		Object string `json:"object"`
	} `json:"data"`
}

// OpenAI-compatible model types
type OpenAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type OpenAIModelsResponse struct {
	Object string        `json:"object"`
	Data   []OpenAIModel `json:"data"`
}

type ChatCompletionResponse struct {
	ID                string                 `json:"id"`
	Object            string                 `json:"object"`
	Created           int64                  `json:"created"`
	Model             string                 `json:"model"`
	Choices           []ChatCompletionChoice `json:"choices"`
	Usage             *ChatCompletionUsage   `json:"usage,omitempty"`
	SystemFingerprint string                 `json:"system_fingerprint,omitempty"`
}

type ChatCompletionChoice struct {
	Index        int                  `json:"index"`
	Message      *ChatMessage         `json:"message,omitempty"`
	Delta        *ChatCompletionDelta `json:"delta,omitempty"`
	FinishReason string               `json:"finish_reason"`
	Logprobs     interface{}          `json:"logprobs,omitempty"`
}

type ChatCompletionDelta struct {
	Role      string      `json:"role,omitempty"`
	Content   interface{} `json:"content,omitempty"`
	ToolCalls []ToolCall  `json:"tool_calls,omitempty"`
}

type ChatCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ChatCompletionChunk struct {
	ID                string                 `json:"id"`
	Object            string                 `json:"object"`
	Created           int64                  `json:"created"`
	Model             string                 `json:"model"`
	Choices           []ChatCompletionChoice `json:"choices"`
	SystemFingerprint string                 `json:"system_fingerprint,omitempty"`
}

type EmbeddingRequest struct {
	Model          string      `json:"model"`
	Input          interface{} `json:"input"`
	EncodingFormat string      `json:"encoding_format,omitempty"`
	Dimensions     int         `json:"dimensions,omitempty"`
	User           string      `json:"user,omitempty"`
}

type EmbeddingResponse struct {
	Object string          `json:"object"`
	Data   []EmbeddingData `json:"data"`
	Model  string          `json:"model"`
	Usage  EmbeddingUsage  `json:"usage"`
}

type EmbeddingData struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

type EmbeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type CompletionRequest struct {
	Model            string      `json:"model"`
	Prompt           interface{} `json:"prompt"`
	MaxTokens        int         `json:"max_tokens,omitempty"`
	Temperature      float64     `json:"temperature,omitempty"`
	TopP             float64     `json:"top_p,omitempty"`
	N                int         `json:"n,omitempty"`
	Stream           bool        `json:"stream,omitempty"`
	Stop             interface{} `json:"stop,omitempty"`
	PresencePenalty  float64     `json:"presence_penalty,omitempty"`
	FrequencyPenalty float64     `json:"frequency_penalty,omitempty"`
	BestOf           int         `json:"best_of,omitempty"`
	Logprobs         int         `json:"logprobs,omitempty"`
	User             string      `json:"user,omitempty"`
}

type CompletionResponse struct {
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Model   string               `json:"model"`
	Choices []CompletionChoice   `json:"choices"`
	Usage   *ChatCompletionUsage `json:"usage,omitempty"`
}

type CompletionChoice struct {
	Text         string      `json:"text"`
	Index        int         `json:"index"`
	Logprobs     interface{} `json:"logprobs,omitempty"`
	FinishReason string      `json:"finish_reason"`
}
