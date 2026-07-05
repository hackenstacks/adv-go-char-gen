package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ── OpenAI-compatible LLM service ────────────────────────────────────────────
// Works with aichat --serve, Ollama (/v1), LM Studio, OpenAI, Mistral, Anthropic
// proxy, and any other OpenAI-compatible endpoint.

type OpenAICompatService struct {
	name    string
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewOpenAICompatService(name, baseURL, apiKey string) *OpenAICompatService {
	base := strings.TrimRight(baseURL, "/")
	// Normalise: if base doesn't end in /v1, append it unless it already has a path
	if !strings.Contains(base, "/v1") {
		base = base + "/v1"
	}
	return &OpenAICompatService{
		name:    name,
		baseURL: base,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

// openAIRequest is the standard chat completions request body.
type openAIRequest struct {
	Model    string           `json:"model"`
	Messages []openAIMessage  `json:"messages"`
	Stream   bool             `json:"stream"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
}

// openAIMessage carries either a plain-text prompt (Content is a string) or, for
// vision requests, an array of content parts (text + image_url). Content is an
// interface so the same struct serves both; existing callers pass a string and
// are unaffected.
type openAIMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

// contentPart is one element of a vision message's content array, per the
// OpenAI chat-completions multimodal format.
type contentPart struct {
	Type     string       `json:"type"`               // "text" | "image_url"
	Text     string       `json:"text,omitempty"`
	ImageURL *imageURLRef `json:"image_url,omitempty"`
}

type imageURLRef struct {
	URL string `json:"url"` // data URI: data:<media>;base64,<...>
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

type openAIModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func (s *OpenAICompatService) GenerateResponse(ctx context.Context, prompt string, model LLMModel, config APIConfig) (string, error) {
	reqBody := openAIRequest{
		Model:       string(model),
		Messages:    []openAIMessage{{Role: "user", Content: prompt}},
		Stream:      false,
		MaxTokens:   2048,
		Temperature: 0.7,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/chat/completions", bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request to %s: %w", s.baseURL, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error %d from %s: %s", resp.StatusCode, s.name, string(respBytes))
	}

	var result openAIResponse
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("API error: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response from %s", s.name)
	}
	return result.Choices[0].Message.Content, nil
}

func (s *OpenAICompatService) GetAvailableModels(ctx context.Context) ([]LLMModel, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("create models request: %w", err)
	}
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch models from %s: %w", s.name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models endpoint returned %d", resp.StatusCode)
	}

	var result openAIModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode models: %w", err)
	}
	models := make([]LLMModel, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, LLMModel(m.ID))
	}
	return models, nil
}

func (s *OpenAICompatService) Provider() Provider { return ProviderCustomLLM }

// ── OpenAI-compatible image service ──────────────────────────────────────────
// Supports DALL-E style /v1/images/generations and Pollinations-style endpoints.

type OpenAICompatImageService struct {
	name    string
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewOpenAICompatImageService(name, baseURL, apiKey string) *OpenAICompatImageService {
	return &OpenAICompatImageService{
		name:    name,
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (s *OpenAICompatImageService) GenerateImage(ctx context.Context, prompt string, model ImageModel) (string, error) {
	reqBody := map[string]interface{}{
		"prompt": prompt,
		"model":  string(model),
		"n":      1,
		"size":   "1024x1024",
	}
	body, _ := json.Marshal(reqBody)

	endpoint := s.baseURL + "/v1/images/generations"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("create image request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("image request to %s: %w", s.name, err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("image API error %d: %s", resp.StatusCode, string(respBytes))
	}

	var result struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return "", fmt.Errorf("decode image response: %w", err)
	}
	if len(result.Data) == 0 {
		return "", fmt.Errorf("no images in response from %s", s.name)
	}
	if result.Data[0].URL != "" {
		return result.Data[0].URL, nil
	}
	return result.Data[0].B64JSON, nil
}

func (s *OpenAICompatImageService) GetAvailableModels(ctx context.Context) ([]ImageModel, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.baseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return []ImageModel{"dall-e-3", "dall-e-2"}, nil // fallback
	}
	defer resp.Body.Close()

	var result openAIModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	models := make([]ImageModel, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, ImageModel(m.ID))
	}
	return models, nil
}

func (s *OpenAICompatImageService) Provider() Provider { return ProviderCustomImage }

// ── .env loader ───────────────────────────────────────────────────────────────
// Reads KEY=VALUE pairs from a .env file and overlays them onto APIConfig.
// Supports: CUSTOM_API_URL, CUSTOM_API_KEY, CUSTOM_MODEL, CUSTOM_API_NAME,
//           OPENAI_API_KEY, MISTRAL_API_KEY, ANTHROPIC_API_KEY, GROQ_API_KEY

func LoadEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err // caller treats os.ErrNotExist as non-fatal
	}
	defer f.Close()

	vars := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 1 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.Trim(strings.TrimSpace(line[idx+1:]), `"'`)
		vars[key] = val
	}
	return vars, scanner.Err()
}

// ApplyEnvToConfig overlays .env values onto an existing APIConfig.
// Call after LoadAPIConfig so stored settings are the baseline.
func ApplyEnvToConfig(cfg *APIConfig, env map[string]string) {
	get := func(k string) string { return env[k] }

	if v := get("OPENAI_API_KEY"); v != "" {
		cfg.OpenAI.APIKey = v
		cfg.OpenAI.Enabled = true
	}
	if v := get("OPENAI_BASE_URL"); v != "" {
		cfg.OpenAI.BaseURL = v
	}
	if v := get("MISTRAL_API_KEY"); v != "" {
		cfg.Mistral.APIKey = v
		cfg.Mistral.Enabled = true
	}
	if v := get("ANTHROPIC_API_KEY"); v != "" {
		cfg.Claude.APIKey = v
		cfg.Claude.Enabled = true
	}
	if v := get("GROQ_API_KEY"); v != "" {
		cfg.Groq.APIKey = v
		cfg.Groq.Enabled = true
	}
	// Gemini via its OpenAI-compatible endpoint (registered as a custom API)
	if v := get("GEMINI_API_KEY"); v != "" {
		upsertCustomAPI(cfg, "gemini", "llm",
			"https://generativelanguage.googleapis.com/v1beta/openai", v, "gemini-2.0-flash")
	}

	// Pollinations — free text + image (key optional; enables higher limits)
	if v := get("POLLINATIONS_API_KEY"); v != "" {
		cfg.Pollinations.APIKey = v
		cfg.Pollinations.Enabled = true
	}
	if get("POLLINATIONS") == "true" || get("POLLINATIONS_API_KEY") != "" {
		cfg.SelectedLLMProvider = ProviderPollinations
		cfg.SelectedImageProvider = ProviderPollinations
		if m := get("POLLINATIONS_TEXT_MODEL"); m != "" {
			cfg.Pollinations.DefaultLLMModel = LLMModel(m)
		}
		if m := get("POLLINATIONS_IMAGE_MODEL"); m != "" {
			cfg.Pollinations.DefaultImageModel = ImageModel(m)
		}
	}

	// Custom OpenAI-compatible LLM endpoint
	if url := get("CUSTOM_API_URL"); url != "" {
		name  := get("CUSTOM_API_NAME")
		if name == "" { name = "custom" }
		key   := get("CUSTOM_API_KEY")
		model := get("CUSTOM_MODEL")
		upsertCustomAPI(cfg, name, "llm", url, key, model)
		cfg.SelectedLLMProvider = ProviderCustomLLM
	}

	// Custom image generation endpoint (OpenAI-compatible or Pollinations-style)
	if url := get("CUSTOM_IMAGE_API_URL"); url != "" {
		name  := get("CUSTOM_IMAGE_API_NAME")
		if name == "" { name = "custom-image" }
		key   := get("CUSTOM_IMAGE_API_KEY")
		model := get("CUSTOM_IMAGE_MODEL")
		upsertCustomAPI(cfg, name, "image", url, key, model)
		cfg.SelectedImageProvider = ProviderCustomImage
	}
}

func upsertCustomAPI(cfg *APIConfig, name, apiType, url, key, model string) {
	for i, c := range cfg.CustomAPIs {
		if c.Name == name && c.Type == apiType {
			cfg.CustomAPIs[i].Endpoint = url
			cfg.CustomAPIs[i].BaseURL  = url
			cfg.CustomAPIs[i].APIKey   = key
			cfg.CustomAPIs[i].Enabled  = true
			if model != "" {
				cfg.CustomAPIs[i].DefaultLLMModel = LLMModel(model)
			}
			return
		}
	}
	cfg.CustomAPIs = append(cfg.CustomAPIs, CustomAPIConfig{
		CommonAPIConfig: CommonAPIConfig{
			APIKey:  key,
			BaseURL: url,
			Enabled: true,
		},
		Name:            name,
		Type:            apiType,
		Endpoint:        url,
		DefaultLLMModel: LLMModel(model),
	})
}

// LoadEnvOverlay is a convenience that loads .env from common locations and
// applies it to cfg. Silent if no .env file is found.
func LoadEnvOverlay(cfg *APIConfig) {
	home, _ := os.UserHomeDir()
	candidates := []string{
		".env",
		home + "/.config/char-gen-cli/.env",
		home + "/.config/nexus/char-gen.env",
	}
	for _, path := range candidates {
		env, err := LoadEnvFile(path)
		if err == nil && len(env) > 0 {
			ApplyEnvToConfig(cfg, env)
			break // first file wins
		}
	}
}
