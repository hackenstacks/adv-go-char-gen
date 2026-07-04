package main

import (
	"log"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// LLMService defines the interface for interacting with Large Language Models.
type LLMService interface {
	GenerateResponse(ctx context.Context, prompt string, model LLMModel, config APIConfig) (string, error)
	GetAvailableModels(ctx context.Context) ([]LLMModel, error)
	Provider() Provider
}


// ImageService defines the interface for interacting with Image Generation Models.
type ImageService interface {
	GenerateImage(ctx context.Context, prompt string, model ImageModel) (string, error) // Returns URL or base64
	GetAvailableModels(ctx context.Context) ([]ImageModel, error)
	Provider() Provider
}

// EmbeddingService defines the interface for text embedding models.
type EmbeddingService interface {
	CreateEmbedding(ctx context.Context, text string) ([]float32, error)
	Provider() Provider
}

// Summarizer defines the interface for text summarization.
type Summarizer interface {
	Summarize(ctx context.Context, text string) (string, error)
	Provider() Provider
}

// --- Mock Implementations ---

// MockLLMService is a mock implementation of LLMService for testing.
type MockLLMService struct{}

func (m *MockLLMService) GenerateResponse(ctx context.Context, prompt string, model LLMModel, config APIConfig) (string, error) {
	log.Printf("MockLLMService: Generating text for prompt '%s' with model '%s' (Config: %+v)\n", prompt, model, config)
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(50 * time.Millisecond):
		return fmt.Sprintf("Mock response for: '%s' (model: %s)", prompt, model), nil
	}
}

func (m *MockLLMService) GetAvailableModels(ctx context.Context) ([]LLMModel, error) {
	return []LLMModel{"mock-llm-fast", "mock-llm-standard", "mock-llm-creative"}, nil
}

func (m *MockLLMService) Provider() Provider {
	return ProviderMock
}

// MockImageService is a mock implementation of ImageService for testing.
type MockImageService struct{}

func (m *MockImageService) GenerateImage(ctx context.Context, prompt string, model ImageModel) (string, error) {
	log.Printf("MockImageService: Generating image for prompt '%s' with model '%s'\n", prompt, model)
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(100 * time.Millisecond):
		return "https://via.placeholder.com/150?text=MockImage", nil
	}
}

func (m *MockImageService) GetAvailableModels(ctx context.Context) ([]ImageModel, error) {
	return []ImageModel{"mock-image-v1", "mock-image-v2-hd"}, nil
}

func (m *MockImageService) Provider() Provider {
	return ProviderMock
}

// MockEmbeddingService is a mock implementation of EmbeddingService.
type MockEmbeddingService struct{}

func (m *MockEmbeddingService) CreateEmbedding(ctx context.Context, text string) ([]float32, error) {
	log.Printf("MockEmbeddingService: Creating embedding for text '%s'\n", text)
	return []float32{0.1, 0.2, 0.3, 0.4, 0.5}, nil
}

func (m *MockEmbeddingService) Provider() Provider {
	return ProviderMock
}

// MockSummarizer is a mock implementation of Summarizer.
type MockSummarizer struct{}

func (m *MockSummarizer) Summarize(ctx context.Context, text string) (string, error) {
	log.Printf("MockSummarizer: Summarizing text '%s'\n", text)
	return fmt.Sprintf("Mock summary of: '%s'", text), nil
}

func (m *MockSummarizer) Provider() Provider {
	return ProviderMock
}

// --- AI Horde Implementation ---

const aihordeBaseURL = "https://horde.art/api/v2"

// AIHordeImageService implements ImageService for AI Horde.
type AIHordeImageService struct {
	client  *http.Client
	apiKey  string
	baseURL string
}

// NewAIHordeImageService creates a new AIHordeImageService.
func NewAIHordeImageService(apiKey, baseURL string) *AIHordeImageService {
	if baseURL == "" {
		baseURL = aihordeBaseURL
	}
	return &AIHordeImageService{
		client:  &http.Client{Timeout: 30 * time.Second},
		apiKey:  apiKey,
		baseURL: baseURL,
	}
}

// ImageGenerationRequest represents the payload for initiating an AI Horde image generation.
type aihordeImageGenerationRequest struct {
	Prompt string `json:"prompt"`
	Models []string `json:"models"`
	Params *struct {
		Width  int `json:"width"`
		Height int `json:"height"`
		N      int `json:"n"` // Number of images to generate
	} `json:"params,omitempty"`
	Nsfw bool `json:"nsfw"`
	Shared bool `json:"shared"`
	Rk bool `json:"r_k"` // 'r_k' is typically a boolean in requests
}

// ImageGenerationResponse represents the response from initiating a generation.
type aihordeImageGenerationResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

// GenerationStatusResponse represents the response from checking generation status.
type aihordeGenerationStatusResponse struct {
	ID          string `json:"id"`
	State       string `json:"state"`
	QueuePos    int    `json:"queue_position"`
	WaitTime    int    `json:"wait_time"`
	Done        bool   `json:"done"`
	Generations []struct {
		ID        string `json:"id"`
		Image     string `json:"img"`
		Model     string `json:"model"`
		Seed      string `json:"seed"`
		WorkerID  string `json:"worker_id"`
		WorkerName string `json:"worker_name"`
	} `json:"generations"`
}

func (s *AIHordeImageService) GenerateImage(ctx context.Context, prompt string, model ImageModel) (string, error) {
	reqPayload := aihordeImageGenerationRequest{
		Prompt: prompt,
		Models: []string{string(model)},
		Params: &struct {
			Width  int `json:"width"`
			Height int `json:"height"`
			N      int `json:"n"`
		}{
			Width:  512, // Default size, can be made configurable later
			Height: 512,
			N:      1, // Request 1 image
		},
		Nsfw:   false, // Can be made configurable
		Shared: true,  // Can be made configurable
		Rk:     false, // Can be made configurable
	}

	reqBody, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal AI Horde request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/generate/async", s.baseURL), bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create AI Horde request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", s.apiKey)
	req.Header.Set("Client-Agent", "char-gen-cli:v0.1:gemini-agent") // Good practice to identify client

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send AI Horde request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("AI Horde initiation error: Status %d, Response: %s", resp.StatusCode, string(bodyBytes))
	}

	var genResp aihordeImageGenerationResponse
	if err := json.NewDecoder(resp.Body).Decode(&genResp); err != nil {
		return "", fmt.Errorf("failed to decode AI Horde initiation response: %w", err)
	}

	if genResp.ID == "" {
		return "", fmt.Errorf("AI Horde did not return a job ID")
	}

	// Poll for status
	ticker := time.NewTicker(2 * time.Second) // Poll every 2 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			statusReq, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/generate/status/%s", s.baseURL, genResp.ID), nil)
			if err != nil {
				return "", fmt.Errorf("failed to create AI Horde status request: %w", err)
			}
			statusReq.Header.Set("apikey", s.apiKey)
			statusReq.Header.Set("Client-Agent", "char-gen-cli:v0.1:gemini-agent")

			statusResp, err := s.client.Do(statusReq)
			if err != nil {
				return "", fmt.Errorf("failed to send AI Horde status request: %w", err)
			}
			defer statusResp.Body.Close()

			if statusResp.StatusCode != http.StatusOK {
				bodyBytes, _ := io.ReadAll(statusResp.Body)
				return "", fmt.Errorf("AI Horde status error: Status %d, Response: %s", statusResp.StatusCode, string(bodyBytes))
			}

			var genStatus aihordeGenerationStatusResponse
			if err := json.NewDecoder(statusResp.Body).Decode(&genStatus); err != nil {
				return "", fmt.Errorf("failed to decode AI Horde status response: %w", err)
			}

			if genStatus.Done {
				if len(genStatus.Generations) > 0 && genStatus.Generations[0].Image != "" {
					imageURL := genStatus.Generations[0].Image
					// Download the image and return as base64
					imgResp, err := http.Get(imageURL)
					if err != nil {
						return "", fmt.Errorf("failed to download image from AI Horde: %w", err)
					}
					defer imgResp.Body.Close()

					if imgResp.StatusCode != http.StatusOK {
						return "", fmt.Errorf("failed to download image, status: %d", imgResp.StatusCode)
					}

					imgBytes, err := io.ReadAll(imgResp.Body)
					if err != nil {
						return "", fmt.Errorf("failed to read image bytes: %w", err)
					}
					return base64.StdEncoding.EncodeToString(imgBytes), nil
				}
				return "", fmt.Errorf("AI Horde generation complete but no image URL found")
			}
		}
	}
}

// GetAvailableModels for AI Horde - this requires a separate API call to /api/v2/status/models
func (s *AIHordeImageService) GetAvailableModels(ctx context.Context) ([]ImageModel, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/status/models", s.baseURL), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create AI Horde models request: %w", err)
	}
	req.Header.Set("apikey", s.apiKey)
	req.Header.Set("Client-Agent", "char-gen-cli:v0.1:gemini-agent")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send AI Horde models request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("AI Horde models status error: Status %d, Response: %s", resp.StatusCode, string(bodyBytes))
	}

	var models []struct {
		Name      string `json:"name"`
		Type      string `json:"type"` // "image", "text" etc.
		Queued    int    `json:"queued"`
		Count     int    `json:"count"`
		NSFW      bool   `json:"nsfw"`
		Hires     bool   `json:"hires"`
		Preferred bool   `json:"preferred"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return nil, fmt.Errorf("failed to decode AI Horde models response: %w", err)
	}

	var imageModels []ImageModel
	for _, m := range models {
		if m.Type == "image" && m.Count > 0 { // Only add models that are image types and have workers
			imageModels = append(imageModels, ImageModel(m.Name))
		}
	}
	return imageModels, nil
}

func (s *AIHordeImageService) Provider() Provider {
	return ProviderAIHorde
}

// --- Mistral Implementation ---

const mistralBaseURL = "https://api.mistral.ai/v1"

// MistralLLMService implements LLMService for Mistral AI.
type MistralLLMService struct {
	client  *http.Client
	apiKey  string
	baseURL string
}

// NewMistralLLMService creates a new MistralLLMService.
func NewMistralLLMService(apiKey, baseURL string) *MistralLLMService {
	if baseURL == "" {
		baseURL = mistralBaseURL
	}
	return &MistralLLMService{
		client:  &http.Client{Timeout: 60 * time.Second}, // Longer timeout for LLM
		apiKey:  apiKey,
		baseURL: baseURL,
	}
}

// Mistral Chat Completion Request
type mistralChatCompletionRequest struct {
	Model    string                  `json:"model"`
	Messages []mistralChatMessage `json:"messages"`
	Temperature float64              `json:"temperature,omitempty"`
	MaxTokens   int                  `json:"max_tokens,omitempty"`
	Stream      bool                 `json:"stream,omitempty"`
}

type mistralChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Mistral Chat Completion Response
type mistralChatCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int                 `json:"index"`
		Message      mistralChatMessage `json:"message"`
		FinishReason string              `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (s *MistralLLMService) GenerateResponse(ctx context.Context, prompt string, model LLMModel, config APIConfig) (string, error) {
	mistralConfig := config.Mistral
	if !mistralConfig.Enabled {
		return "", fmt.Errorf("Mistral provider is not enabled")
	}

	reqPayload := mistralChatCompletionRequest{
		Model: string(model),
		Messages: []mistralChatMessage{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.7, // Default temperature
		MaxTokens:   500, // Default max tokens
		Stream:      false,
	}

	reqBody, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Mistral request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/chat/completions", s.baseURL), bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create Mistral request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+mistralConfig.APIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send Mistral request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Mistral API error: Status %d, Response: %s", resp.StatusCode, string(bodyBytes))
	}

	var chatCompletionResponse mistralChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatCompletionResponse); err != nil {
		return "", fmt.Errorf("failed to decode Mistral response: %w", err)
	}

	if len(chatCompletionResponse.Choices) > 0 {
		return chatCompletionResponse.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("no response from Mistral API")
}

// GetAvailableModels for Mistral - requires an API call to /v1/models
func (s *MistralLLMService) GetAvailableModels(ctx context.Context) ([]LLMModel, error) {
	if s.apiKey == "mock-key" {
		return []LLMModel{"mistral-tiny", "mistral-small", "mistral-medium"}, nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/models", s.baseURL), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Mistral models request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey) // Assuming API key is needed for models endpoint

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send Mistral models request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Mistral models API error: Status %d, Response: %s", resp.StatusCode, string(bodyBytes))
	}

	var modelsResponse struct {
		Object string `json:"object"`
		Data   []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Created int64  `json:"created"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&modelsResponse); err != nil {
		return nil, fmt.Errorf("failed to decode Mistral models response: %w", err)
	}

	var llmModels []LLMModel
	for _, m := range modelsResponse.Data {
		llmModels = append(llmModels, LLMModel(m.ID))
	}
	return llmModels, nil
}

func (s *MistralLLMService) Provider() Provider {
	return ProviderMistral
}

// --- OpenAI Implementation ---

const openaiBaseURL = "https://api.openai.com/v1"

// OpenAIEmbeddingService implements EmbeddingService for OpenAI.
type OpenAIEmbeddingService struct {
	client  *http.Client
	apiKey  string
	baseURL string
}

// NewOpenAIEmbeddingService creates a new OpenAIEmbeddingService.
func NewOpenAIEmbeddingService(apiKey, baseURL string) *OpenAIEmbeddingService {
	if baseURL == "" {
		baseURL = openaiBaseURL
	}
	return &OpenAIEmbeddingService{
		client:  &http.Client{Timeout: 30 * time.Second},
		apiKey:  apiKey,
		baseURL: baseURL,
	}
}

// OpenAIEmbeddingRequest represents the payload for an OpenAI embedding API call.
type openaiEmbeddingRequest struct {
	Input string `json:"input"`
	Model string `json:"model"`
}

// OpenAIEmbeddingResponse represents the response from an OpenAI embedding API call.
type openaiEmbeddingResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

func (s *OpenAIEmbeddingService) CreateEmbedding(ctx context.Context, text string) ([]float32, error) {
	// TODO: Make model configurable via APIConfig
	model := "text-embedding-ada-002" // Default OpenAI embedding model

	reqPayload := openaiEmbeddingRequest{
		Input: text,
		Model: model,
	}

	reqBody, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal OpenAI embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/embeddings", s.baseURL), bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send OpenAI embedding request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI embedding API error: Status %d, Response: %s", resp.StatusCode, string(bodyBytes))
	}

	var embeddingResponse openaiEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embeddingResponse); err != nil {
		return nil, fmt.Errorf("failed to decode OpenAI embedding response: %w", err)
	}

	if len(embeddingResponse.Data) > 0 {
		return embeddingResponse.Data[0].Embedding, nil
	}

	return nil, fmt.Errorf("no embedding data returned from OpenAI API")
}

func (s *OpenAIEmbeddingService) Provider() Provider {
	return ProviderOpenAI
}

// --- Factory Functions ---

// LLMFactory creates an LLMService based on the provider and API configuration.
func LLMFactory(p Provider, config APIConfig) (LLMService, error) {
	switch p {
	case ProviderMock:
		return &MockLLMService{}, nil
	case ProviderMistral:
		return NewMistralLLMService(config.Mistral.APIKey, config.Mistral.BaseURL), nil
	case ProviderOpenAI:
		base := config.OpenAI.BaseURL
		if base == "" { base = "https://api.openai.com" }
		return NewOpenAICompatService("OpenAI", base, config.OpenAI.APIKey), nil
	case ProviderPollinations:
		return NewPollinationsLLMService("", config.Pollinations.APIKey), nil
	case ProviderCustomLLM:
		// Use first enabled custom LLM API
		for _, c := range config.CustomAPIs {
			if c.Type == "llm" && c.Enabled {
				url := c.Endpoint
				if url == "" { url = c.BaseURL }
				return NewOpenAICompatService(c.Name, url, c.APIKey), nil
			}
		}
		return nil, fmt.Errorf("no custom LLM API configured — add CUSTOM_API_URL to .env or configure in API Settings")
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", p)
	}
}

// ImageFactory creates an ImageService based on the provider and API configuration.
func ImageFactory(p Provider, config APIConfig) (ImageService, error) {
	switch p {
	case ProviderMock:
		return &MockImageService{}, nil
	case ProviderAIHorde:
		return NewAIHordeImageService(config.AIHorde.APIKey, config.AIHorde.BaseURL), nil
	case ProviderPollinations:
		return NewPollinationsImageService(string(config.Pollinations.DefaultImageModel), config.Pollinations.APIKey), nil
	case ProviderCustomImage:
		for _, c := range config.CustomAPIs {
			if c.Type == "image" && c.Enabled {
				url := c.Endpoint
				if url == "" { url = c.BaseURL }
				return NewOpenAICompatImageService(c.Name, url, c.APIKey), nil
			}
		}
		return nil, fmt.Errorf("no custom image API configured — add CUSTOM_IMAGE_API_URL to .env")
	default:
		return nil, fmt.Errorf("unsupported Image provider: %s", p)
	}
}

// EmbeddingFactory creates an EmbeddingService.
func EmbeddingFactory(p Provider, config APIConfig) (EmbeddingService, error) {
	switch p {
	case ProviderMock:
		return &MockEmbeddingService{}, nil
	case ProviderOpenAI:
		return NewOpenAIEmbeddingService(config.OpenAI.APIKey, config.OpenAI.BaseURL), nil
	// TODO: Add cases for other real embedding providers
	default:
		return nil, fmt.Errorf("unsupported Embedding provider: %s", p)
	}
}

// LLMSummarizer implements Summarizer by delegating to an LLMService.
type LLMSummarizer struct {
	llmService LLMService
	llmModel   LLMModel
	provider   Provider
}

// NewLLMSummarizer creates a new LLMSummarizer.
func NewLLMSummarizer(llmService LLMService, llmModel LLMModel) *LLMSummarizer {
	return &LLMSummarizer{
		llmService: llmService,
		llmModel:   llmModel,
		provider:   llmService.Provider(), // The provider of the underlying LLM
	}
}

func (s *LLMSummarizer) Summarize(ctx context.Context, text string) (string, error) {
	// The prompt passed to Summarize already contains summarization instructions.
	// We just need to pass it to the underlying LLM service.
	config := APIConfig{} // A dummy config for now, actual config should be passed through
	summary, err := s.llmService.GenerateResponse(ctx, text, s.llmModel, config)
	if err != nil {
		return "", fmt.Errorf("LLM summarization failed: %w", err)
	}
	return summary, nil
}

func (s *LLMSummarizer) Provider() Provider {
	return s.provider
}

// SummarizerFactory creates a Summarizer.
func SummarizerFactory(p Provider, config APIConfig) (Summarizer, error) {
	switch p {
	case ProviderMock:
		return &MockSummarizer{}, nil
	// For summarization, we need an actual LLM. We will use the Mistral LLM for now.
	case ProviderMistral:
		llmService, err := LLMFactory(ProviderMistral, config)
		if err != nil {
			return nil, fmt.Errorf("failed to create LLM service for summarizer: %w", err)
		}
		// Use a default model for summarization if not specified in config
		llmModel := LLMModel(config.Mistral.DefaultLLMModel)
		if llmModel == "" {
			llmModel = "mistral-tiny" // Fallback to a common Mistral model
		}
		return NewLLMSummarizer(llmService, llmModel), nil
	// TODO: Add cases for other real summarizer providers (e.g., OpenAI)
	default:
		return nil, fmt.Errorf("unsupported Summarizer provider: %s", p)
	}
}