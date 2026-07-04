package main

import (
	"encoding/json"
	"fmt"
)

// Provider represents a specific API provider.
type Provider string

const (
	ProviderAIHorde      Provider = "AIHorde"
	ProviderPollinations Provider = "Pollinations.ai"
	ProviderImageRouter  Provider = "ImageRouter"
	ProviderMistral      Provider = "Mistral"
	ProviderGroq         Provider = "Groq"
	ProviderHuggingFace  Provider = "HuggingFace"
	ProviderOpenAI       Provider = "OpenAI"
	ProviderClaude       Provider = "Claude"
	ProviderCustomLLM    Provider = "CustomLLM"
	ProviderCustomImage  Provider = "CustomImage"
	ProviderMock         Provider = "Mock" // For testing and development
)

// LLMModel represents a large language model.
type LLMModel string

// ImageModel represents an image generation model.
type ImageModel string

// Common API fields
type CommonAPIConfig struct {
	Enabled bool   `json:"enabled"`
	APIKey  string `json:"apiKey"`
	BaseURL string `json:"baseURL,omitempty"` // For custom or self-hosted instances
	DefaultLLMModel LLMModel `json:"defaultLLMModel,omitempty"`
	DefaultImageModel ImageModel `json:"defaultImageModel,omitempty"`
}

// AIHordeConfig for AIHorde API
type AIHordeConfig struct {
	CommonAPIConfig
	DefaultImageModel ImageModel `json:"defaultImageModel,omitempty"`
	// AI Horde has a concept of workers and kudos, but for a basic config, APIKey is enough
}

// PollinationsConfig for Pollinations.ai API
type PollinationsConfig struct {
	CommonAPIConfig
	DefaultLLMModel   LLMModel   `json:"defaultLLMModel,omitempty"`
	DefaultImageModel ImageModel `json:"defaultImageModel,omitempty"`
	// Pollinations.ai is often unauthenticated, but a key might be used for higher limits
}

// ImageRouterConfig for ImageRouter API (placeholder, assume it routes to other image APIs)
type ImageRouterConfig struct {
	CommonAPIConfig
	DefaultImageModel ImageModel `json:"defaultImageModel,omitempty"`
}

// MistralConfig for Mistral API
type MistralConfig struct {
	CommonAPIConfig
	DefaultLLMModel LLMModel `json:"defaultLLMModel,omitempty"`
}

// GroqConfig for Groq API
type GroqConfig struct {
	CommonAPIConfig
	DefaultLLMModel LLMModel `json:"defaultLLMModel,omitempty"`
}

// HuggingFaceConfig for HuggingFace Inference API
type HuggingFaceConfig struct {
	CommonAPIConfig
	DefaultLLMModel   LLMModel `json:"defaultLLMModel,omitempty"`
	DefaultImageModel ImageModel `json:"defaultImageModel,omitempty"`
	// HuggingFace often uses model names as part of the URL
}

// OpenAIConfig for OpenAI API
type OpenAIConfig struct {
	CommonAPIConfig
	DefaultLLMModel   LLMModel `json:"defaultLLMModel,omitempty"`
	DefaultImageModel ImageModel `json:"defaultImageModel,omitempty"` // DALL-E models
	OrganizationID    string   `json:"organizationId,omitempty"`
}

// ClaudeConfig for Anthropic Claude API
type ClaudeConfig struct {
	CommonAPIConfig
	DefaultLLMModel LLMModel `json:"defaultLLMModel,omitempty"`
}

// CustomAPIConfig for user-defined LLM or Image APIs
type CustomAPIConfig struct {
	CommonAPIConfig
	Name              string     `json:"name"` // User-defined name for the custom API
	Type              string     `json:"type"` // "llm" or "image"
	DefaultLLMModel   LLMModel   `json:"defaultLLMModel,omitempty"`
	DefaultImageModel ImageModel `json:"defaultImageModel,omitempty"`
	Endpoint          string     `json:"endpoint"` // Custom API endpoint
	// Potentially add fields for custom headers, auth methods etc.
}

// APIConfig holds the configurations for all supported API providers.
type APIConfig struct {
	AIHorde      AIHordeConfig      `json:"aihorde"`
	Pollinations PollinationsConfig `json:"pollinations"`
	ImageRouter  ImageRouterConfig  `json:"imagerouter"`
	Mistral      MistralConfig      `json:"mistral"`
	Groq         GroqConfig         `json:"groq"`
	HuggingFace  HuggingFaceConfig  `json:"huggingface"`
	OpenAI       OpenAIConfig       `json:"openai"`
	Claude       ClaudeConfig       `json:"claude"`
	CustomAPIs   []CustomAPIConfig  `json:"customApis"` // Allow multiple custom APIs
	// Currently selected providers for LLM and Image generation
	SelectedLLMProvider   Provider `json:"selectedLLMProvider"`
	SelectedImageProvider Provider `json:"selectedImageProvider"`
}

// NewAPIConfig creates a new APIConfig with default values.
func NewAPIConfig() APIConfig {
	return APIConfig{
		// Default to some free providers or mock for now
		SelectedLLMProvider:   ProviderMock,
		SelectedImageProvider: ProviderMock,
		AIHorde: AIHordeConfig{
			CommonAPIConfig: CommonAPIConfig{Enabled: true}, // AIHorde is often free/community-driven
		},
		Pollinations: PollinationsConfig{
			CommonAPIConfig: CommonAPIConfig{Enabled: true}, // Pollinations is often free
		},
	}
}

// GetLLMProviderConfig retrieves the specific configuration for a given LLM Provider.
func (c *APIConfig) GetLLMProviderConfig(provider Provider) (CommonAPIConfig, bool) {
	switch provider {
	case ProviderMistral:
		return c.Mistral.CommonAPIConfig, true
	case ProviderGroq:
		return c.Groq.CommonAPIConfig, true
	case ProviderHuggingFace:
		return c.HuggingFace.CommonAPIConfig, true
	case ProviderOpenAI:
		return c.OpenAI.CommonAPIConfig, true
	case ProviderClaude:
		return c.Claude.CommonAPIConfig, true
	case ProviderMock:
		return CommonAPIConfig{Enabled: true, APIKey: "mock-key"}, true
	default:
		// Check custom APIs
		for _, custom := range c.CustomAPIs {
			if custom.Name == string(provider) && custom.Type == "llm" {
				return custom.CommonAPIConfig, true
			}
		}
		return CommonAPIConfig{}, false
	}
}

// GetImageProviderConfig retrieves the specific configuration for a given Image Provider.
func (c *APIConfig) GetImageProviderConfig(provider Provider) (CommonAPIConfig, bool) {
	switch provider {
	case ProviderAIHorde:
		return c.AIHorde.CommonAPIConfig, true
	case ProviderPollinations:
		return c.Pollinations.CommonAPIConfig, true
	case ProviderImageRouter:
		return c.ImageRouter.CommonAPIConfig, true
	case ProviderHuggingFace:
		return c.HuggingFace.CommonAPIConfig, true
	case ProviderOpenAI: // DALL-E etc.
		return c.OpenAI.CommonAPIConfig, true
	case ProviderMock:
		return CommonAPIConfig{Enabled: true, APIKey: "mock-key"}, true
	default:
		// Check custom APIs
		for _, custom := range c.CustomAPIs {
			if custom.Name == string(provider) && custom.Type == "image" {
				return custom.CommonAPIConfig, true
			}
		}
		return CommonAPIConfig{}, false
	}
}

// GetCustomAPIConfig retrieves a custom API configuration by name and type.
func (c *APIConfig) GetCustomAPIConfig(name string, apiType string) (CustomAPIConfig, bool) {
	for _, custom := range c.CustomAPIs {
		if custom.Name == name && custom.Type == apiType {
			return custom, true
		}
	}
	return CustomAPIConfig{}, false
}

// SaveAPIConfig saves the API configuration for a user.
func SaveAPIConfig(user *User, config APIConfig) error {
	if user == nil || user.Username == "" {
		return fmt.Errorf("invalid user for saving API config")
	}
	configBytes, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal API config: %w", err)
	}
	// Use the generic SaveEncryptedData
	return SaveEncryptedData(user, user.Username, "configs", "api_config", configBytes)
}

// LoadAPIConfig loads the API configuration for a user.
func LoadAPIConfig(user *User) (APIConfig, error) {
	if user == nil || user.Username == "" {
		return APIConfig{}, fmt.Errorf("invalid user for loading API config")
	}
	configBytes, err := LoadEncryptedData(user, user.Username, "configs", "api_config")
	if err != nil {
		// If config not found, return a new default one
		if err.Error() == fmt.Sprintf("data 'configs/api_config' not found for user '%s'", user.Username) {
			return NewAPIConfig(), nil
		}
		return APIConfig{}, fmt.Errorf("failed to load encrypted API config: %w", err)
	}
	var config APIConfig
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return APIConfig{}, fmt.Errorf("failed to unmarshal API config: %w", err)
	}
	// Apply .env overrides on top of stored config
	LoadEnvOverlay(&config)
	return config, nil
}