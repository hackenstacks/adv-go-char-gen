package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// ── Pollinations.ai — unified API (gen.pollinations.ai) ───────────────────────
// Text:  POST /v1/chat/completions   (OpenAI-compatible; Bearer key)
// Image: GET  /image/{prompt}?model=&width=&height=&key=
// Generation now requires an sk_ key from enter.pollinations.ai.
// Model lists: GET /v1/models  (no auth).
const (
	pollinationsTextURL  = "https://gen.pollinations.ai/v1/chat/completions"
	pollinationsImageURL = "https://gen.pollinations.ai/image/"
)

// ── Text ──────────────────────────────────────────────────────────────────────

type PollinationsLLMService struct {
	client *http.Client
	model  string
	apiKey string
}

func NewPollinationsLLMService(model, apiKey string) *PollinationsLLMService {
	if model == "" {
		model = "openai"
	}
	return &PollinationsLLMService{
		client: &http.Client{Timeout: 120 * time.Second},
		model:  model,
		apiKey: apiKey,
	}
}

func (s *PollinationsLLMService) GenerateResponse(ctx context.Context, prompt string, model LLMModel, config APIConfig) (string, error) {
	mdl := string(model)
	if mdl == "" || mdl == "mock-model" {
		mdl = s.model
	}
	body, _ := json.Marshal(openAIRequest{
		Model:    mdl,
		Messages: []openAIMessage{{Role: "user", Content: prompt}},
		Stream:   false,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", pollinationsTextURL, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("pollinations text: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("pollinations text error %d: %s", resp.StatusCode, string(raw))
	}
	var out openAIResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("pollinations: empty response")
	}
	return out.Choices[0].Message.Content, nil
}

// GenerateResponseWithImages sends a multimodal message (text + images) to a
// vision-capable Pollinations model. Satisfies VisionLLMService. If no images are
// given it behaves like GenerateResponse.
func (s *PollinationsLLMService) GenerateResponseWithImages(ctx context.Context, prompt string, images []ImageAttachment, model LLMModel, config APIConfig) (string, error) {
	if len(images) == 0 {
		return s.GenerateResponse(ctx, prompt, model, config)
	}
	mdl := string(model)
	if mdl == "" || mdl == "mock-model" {
		mdl = s.model
	}

	parts := []contentPart{{Type: "text", Text: prompt}}
	for _, img := range images {
		parts = append(parts, contentPart{Type: "image_url", ImageURL: &imageURLRef{URL: dataURI(img)}})
	}
	body, _ := json.Marshal(openAIRequest{
		Model:    mdl,
		Messages: []openAIMessage{{Role: "user", Content: parts}},
		Stream:   false,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", pollinationsTextURL, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("pollinations vision: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("pollinations vision error %d: %s", resp.StatusCode, string(raw))
	}
	var out openAIResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("pollinations: empty response")
	}
	return out.Choices[0].Message.Content, nil
}

func (s *PollinationsLLMService) GetAvailableModels(ctx context.Context) ([]LLMModel, error) {
	return []LLMModel{
		"openai", "openai-fast", "openai-large", "gpt-5.4", "gpt-5.4-mini",
		"mistral", "mistral-large", "mistral-small-3.2",
		"gemini", "gemini-3-flash", "gemini-fast",
		"claude", "claude-fast", "claude-opus-4.7",
		"deepseek", "deepseek-pro", "grok", "llama", "qwen-coder",
	}, nil
}

func (s *PollinationsLLMService) Provider() Provider { return ProviderPollinations }

// ── Image ─────────────────────────────────────────────────────────────────────

type PollinationsImageService struct {
	model  string
	apiKey string
}

func NewPollinationsImageService(model, apiKey string) *PollinationsImageService {
	if model == "" {
		model = "flux"
	}
	return &PollinationsImageService{model: model, apiKey: apiKey}
}

// GenerateImage returns a direct image URL (Pollinations serves the image at the URL).
func (s *PollinationsImageService) GenerateImage(ctx context.Context, prompt string, model ImageModel) (string, error) {
	mdl := string(model)
	if mdl == "" || mdl == "mock-image-v1" {
		mdl = s.model
	}
	encoded := url.PathEscape(prompt)
	u := fmt.Sprintf("%s%s?width=1024&height=1024&nologo=true&model=%s", pollinationsImageURL, encoded, mdl)
	// Image generation now requires the key (anonymous returns a 401 notice).
	// Query-param auth works for the GET image endpoint.
	if s.apiKey != "" {
		u += "&key=" + url.QueryEscape(s.apiKey)
	}
	return u, nil
}

func (s *PollinationsImageService) GetAvailableModels(ctx context.Context) ([]ImageModel, error) {
	return []ImageModel{"flux", "kontext", "nanobanana", "seedream", "gptimage", "qwen-image", "zimage"}, nil
}

func (s *PollinationsImageService) Provider() Provider { return ProviderPollinations }

// downloadImage fetches a URL to a local file (used to save generated images).
func downloadImage(ctx context.Context, imgURL, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", imgURL, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("image download failed: %d", resp.StatusCode)
	}
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}
