package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// API Settings Styles
var (
	apiSettingsTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFD700")). // Gold
				Padding(1, 4).
				Align(lipgloss.Center).
				Bold(true)

	apiSettingsItemStyle = lipgloss.NewStyle().
				PaddingLeft(2).
				Foreground(lipgloss.Color("#B0C4DE")) // Light Steel Blue

	apiSettingsSelectedItemStyle = lipgloss.NewStyle().
					PaddingLeft(2).
					Foreground(lipgloss.Color("#FFFF00")). // Yellow
					Border(lipgloss.RoundedBorder(), false, false, false, true).
					BorderForeground(lipgloss.Color("#FFFF00"))

	apiSettingsHelpStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#757575")). // Grey
				PaddingTop(1)

	apiSettingsBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#ADD8E6")). // Light Blue
				Padding(1, 2).
				Width(80)

	apiSettingsInputPromptStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#ADD8E6"))

	apiSettingsInputTextStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#FFFF00")).
					BorderBottom(true).
					BorderBottomForeground(lipgloss.Color("#FFFF00"))

	apiSettingsErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF0000")).
				Bold(true)
)

// apiSettingsModel manages the API configuration UI.
type apiSettingsModel struct {
	user        *User
	apiConfig   APIConfig
	currentView apiSettingsView
	providerList list.Model
	apiKeyInput  textinput.Model
	baseURLInput textinput.Model
	customNameInput textinput.Model
	customTypeInput textinput.Model // "llm" or "image"
	selectedProvider Provider
	err          error
	saveSuccess  bool

	// New fields for model selection
	llmModelsList list.Model
	imageModelsList list.Model
	currentModelListType string // "llm" or "image"
	modelSelectionOrigin Provider // To know which provider's models we are selecting
	modelCursor int
}

type apiSettingsView int

const (
	providerListView apiSettingsView = iota
	providerDetailsView
	customProviderDetailsView
	setProviderDefaultView
	modelSelectionView
)

type providerItem struct {
	provider Provider
	title    string
	desc     string
}

func (i providerItem) FilterValue() string { return i.title }
func (i providerItem) Title() string       { return i.title }
func (i providerItem) Description() string  { return i.desc }

func initialApiSettingsModel(u *User) apiSettingsModel {
	// Initialize API config
	apiConfig, err := LoadAPIConfig(u)
	if err != nil {
		fmt.Printf("Error loading API config: %v\n", err)
		apiConfig = NewAPIConfig() // Use default if error
	}

	// Initialize provider list
	providers := []list.Item{
		providerItem{provider: ProviderAIHorde, title: "AIHorde ✨", desc: "Decentralized image generation"},
		providerItem{provider: ProviderPollinations, title: "Pollinations.ai 🌳", desc: "Creative AI models"},
		providerItem{provider: ProviderImageRouter, title: "ImageRouter 📸", desc: "Routes image generation requests"},
		providerItem{provider: ProviderMistral, title: "Mistral AI 💨", desc: "Powerful LLM models"},
		providerItem{provider: ProviderGroq, title: "Groq ⚡", desc: "Fast LLM inference"},
		providerItem{provider: ProviderHuggingFace, title: "HuggingFace 🤗", desc: "Many open-source models"},
		providerItem{provider: ProviderOpenAI, title: "OpenAI 🤖", desc: "GPT, DALL-E, etc."},
		providerItem{provider: ProviderClaude, title: "Anthropic Claude 📜", desc: "Anthropic's LLM"},
		providerItem{provider: ProviderCustomLLM, title: "Custom LLM API 🔧", desc: "Add your own LLM endpoint"},
		providerItem{provider: ProviderCustomImage, title: "Custom Image API 🎨", desc: "Add your own image endpoint"},
	}

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = apiSettingsSelectedItemStyle
	delegate.Styles.SelectedDesc = apiSettingsSelectedItemStyle.Copy().Faint(true)
	delegate.Styles.NormalTitle = apiSettingsItemStyle
	delegate.Styles.NormalDesc = apiSettingsItemStyle.Copy().Faint(true)

	l := list.New(providers, delegate, 0, 0)
	l.Title = "Select API Provider"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = apiSettingsTitleStyle

	// Initialize text inputs
	apiKeyInput := textinput.New()
	apiKeyInput.Placeholder = "Enter API Key (leave empty for unauthenticated)"
	apiKeyInput.CharLimit = 100
	apiKeyInput.Width = 60
	apiKeyInput.PromptStyle = apiSettingsInputPromptStyle
	apiKeyInput.TextStyle = apiSettingsInputTextStyle

	baseURLInput := textinput.New()
	baseURLInput.Placeholder = "Enter Base URL (e.g., https://api.example.com/v1)"
	baseURLInput.CharLimit = 150
	baseURLInput.Width = 60
	baseURLInput.PromptStyle = apiSettingsInputPromptStyle
	baseURLInput.TextStyle = apiSettingsInputTextStyle

	customNameInput := textinput.New()
	customNameInput.Placeholder = "Enter a name for your custom API"
	customNameInput.CharLimit = 50
	customNameInput.Width = 40
	customNameInput.PromptStyle = apiSettingsInputPromptStyle
	customNameInput.TextStyle = apiSettingsInputTextStyle

	customTypeInput := textinput.New()
	customTypeInput.Placeholder = "Type 'llm' or 'image'"
	customTypeInput.CharLimit = 5
	customTypeInput.Width = 10
	customTypeInput.PromptStyle = apiSettingsInputPromptStyle
	customTypeInput.TextStyle = apiSettingsInputTextStyle

	// Initialize model lists (empty initially, populated on demand)
	modelDelegate := list.NewDefaultDelegate()
	modelDelegate.Styles.SelectedTitle = apiSettingsSelectedItemStyle
	modelDelegate.Styles.SelectedDesc = apiSettingsSelectedItemStyle.Copy().Faint(true)
	modelDelegate.Styles.NormalTitle = apiSettingsItemStyle
	modelDelegate.Styles.NormalDesc = apiSettingsItemStyle.Copy().Faint(true)

	llmModelList := list.New([]list.Item{}, modelDelegate, 0, 0)
	llmModelList.Title = "Select LLM Model"
	llmModelList.SetShowStatusBar(false)
	llmModelList.SetFilteringEnabled(true) // Allow filtering for models
	llmModelList.Styles.Title = apiSettingsTitleStyle

	imageModelList := list.New([]list.Item{}, modelDelegate, 0, 0)
	imageModelList.Title = "Select Image Model"
	imageModelList.SetShowStatusBar(false)
	imageModelList.SetFilteringEnabled(true) // Allow filtering for models
	imageModelList.Styles.Title = apiSettingsTitleStyle


	return apiSettingsModel{
		user:         u,
		apiConfig:    apiConfig,
		currentView:  providerListView,
		providerList: l,
		apiKeyInput:  apiKeyInput,
		baseURLInput: baseURLInput,
		customNameInput: customNameInput,
		customTypeInput: customTypeInput,
		llmModelsList: llmModelList,
		imageModelsList: imageModelList,
	}
}

func (m apiSettingsModel) Init() tea.Cmd {
	return nil
}

func (m apiSettingsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := apiSettingsBoxStyle.GetHorizontalFrameSize(), apiSettingsBoxStyle.GetVerticalFrameSize()
		m.providerList.SetSize(msg.Width-h, msg.Height-v)

	case tea.KeyMsg:
		log.Printf("apiSettingsModel received key press: %s, currentView: %d", msg.String(), m.currentView)
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			m.err = nil // Clear error on escape
			m.saveSuccess = false // Clear success message
			if m.currentView == providerListView {
				return m, func() tea.Msg { return BackToMainAppMsg{} }
			} else if m.currentView == modelSelectionView {
				m.currentView = providerDetailsView // Go back from model selection
				return m, nil
			}
			// Go back to provider list from details view
			m.currentView = providerListView
			// Clear inputs
			m.apiKeyInput.Reset()
			m.baseURLInput.Reset()
			m.customNameInput.Reset()
			m.customTypeInput.Reset()
			m.apiKeyInput.Blur()
			m.baseURLInput.Blur()
			m.customNameInput.Blur()
			m.customTypeInput.Blur()
			return m, nil

		case "enter":
			switch m.currentView {
			case providerListView:
				selected := m.providerList.SelectedItem().(providerItem)
				m.selectedProvider = selected.provider
				m.err = nil // Clear error on new selection
				m.saveSuccess = false // Clear success message

				// Load existing values for the selected provider
				switch m.selectedProvider {
				case ProviderAIHorde:
					m.apiKeyInput.SetValue(m.apiConfig.AIHorde.APIKey)
					m.baseURLInput.SetValue(m.apiConfig.AIHorde.BaseURL)
				case ProviderPollinations:
					m.apiKeyInput.SetValue(m.apiConfig.Pollinations.APIKey)
					m.baseURLInput.SetValue(m.apiConfig.Pollinations.BaseURL)
				case ProviderImageRouter:
					m.apiKeyInput.SetValue(m.apiConfig.ImageRouter.APIKey)
					m.baseURLInput.SetValue(m.apiConfig.ImageRouter.BaseURL)
				case ProviderMistral:
					m.apiKeyInput.SetValue(m.apiConfig.Mistral.APIKey)
					m.baseURLInput.SetValue(m.apiConfig.Mistral.BaseURL)
				case ProviderGroq:
					m.apiKeyInput.SetValue(m.apiConfig.Groq.APIKey)
					m.baseURLInput.SetValue(m.apiConfig.Groq.BaseURL)
				case ProviderHuggingFace:
					m.apiKeyInput.SetValue(m.apiConfig.HuggingFace.APIKey)
					m.baseURLInput.SetValue(m.apiConfig.HuggingFace.BaseURL)
				case ProviderOpenAI:
					m.apiKeyInput.SetValue(m.apiConfig.OpenAI.APIKey)
					m.baseURLInput.SetValue(m.apiConfig.OpenAI.BaseURL)
				case ProviderClaude:
					m.apiKeyInput.SetValue(m.apiConfig.Claude.APIKey)
					m.baseURLInput.SetValue(m.apiConfig.Claude.BaseURL)
				case ProviderCustomLLM, ProviderCustomImage:
					m.currentView = customProviderDetailsView
					m.customNameInput.Focus()
					return m, nil
				}
				if m.currentView != customProviderDetailsView {
					m.currentView = providerDetailsView
					m.apiKeyInput.Focus()
					return m, nil
				}

			case providerDetailsView, customProviderDetailsView:
				if m.apiKeyInput.Focused() {
					m.apiKeyInput.Blur()
					m.baseURLInput.Focus()
					return m, nil
				} else if m.baseURLInput.Focused() || m.customTypeInput.Focused() {
					// Save logic for all providers here
					m.saveSuccess = false
					m.err = nil

					switch m.selectedProvider {
					case ProviderAIHorde:
						m.apiConfig.AIHorde.APIKey = m.apiKeyInput.Value()
						m.apiConfig.AIHorde.BaseURL = m.baseURLInput.Value()
						m.apiConfig.AIHorde.Enabled = m.apiKeyInput.Value() != "" // Enable if key is present
					case ProviderPollinations:
						m.apiConfig.Pollinations.APIKey = m.apiKeyInput.Value()
						m.apiConfig.Pollinations.BaseURL = m.baseURLInput.Value()
						m.apiConfig.Pollinations.Enabled = m.apiKeyInput.Value() != ""
					case ProviderImageRouter:
						m.apiConfig.ImageRouter.APIKey = m.apiKeyInput.Value()
						m.apiConfig.ImageRouter.BaseURL = m.baseURLInput.Value()
						m.apiConfig.ImageRouter.Enabled = m.apiKeyInput.Value() != ""
					case ProviderMistral:
						m.apiConfig.Mistral.APIKey = m.apiKeyInput.Value()
						m.apiConfig.Mistral.BaseURL = m.baseURLInput.Value()
						m.apiConfig.Mistral.Enabled = m.apiKeyInput.Value() != ""
					case ProviderGroq:
						m.apiConfig.Groq.APIKey = m.apiKeyInput.Value()
						m.apiConfig.Groq.BaseURL = m.baseURLInput.Value()
						m.apiConfig.Groq.Enabled = m.apiKeyInput.Value() != ""
					case ProviderHuggingFace:
						m.apiConfig.HuggingFace.APIKey = m.apiKeyInput.Value()
						m.apiConfig.HuggingFace.BaseURL = m.baseURLInput.Value()
						m.apiConfig.HuggingFace.Enabled = m.apiKeyInput.Value() != ""
					case ProviderOpenAI:
						m.apiConfig.OpenAI.APIKey = m.apiKeyInput.Value()
						m.apiConfig.OpenAI.BaseURL = m.baseURLInput.Value()
						m.apiConfig.OpenAI.Enabled = m.apiKeyInput.Value() != ""
					case ProviderClaude:
						m.apiConfig.Claude.APIKey = m.apiKeyInput.Value()
						m.apiConfig.Claude.BaseURL = m.baseURLInput.Value()
						m.apiConfig.Claude.Enabled = m.apiKeyInput.Value() != ""
					case ProviderCustomLLM, ProviderCustomImage:
						name := strings.TrimSpace(m.customNameInput.Value())
						endpoint := strings.TrimSpace(m.baseURLInput.Value())
						apiKey := m.apiKeyInput.Value()
						apiType := strings.ToLower(strings.TrimSpace(m.customTypeInput.Value()))

						if name == "" || endpoint == "" || (apiType != "llm" && apiType != "image") {
							m.err = fmt.Errorf("custom API requires a name, endpoint, and type ('llm' or 'image')")
							return m, nil
						}

						newCustomAPI := CustomAPIConfig{
							CommonAPIConfig: CommonAPIConfig{
								Enabled: true, // Custom APIs are enabled upon creation
								APIKey:  apiKey,
								BaseURL: endpoint,
							},
							Name:    name,
							Type:    apiType,
						}

						// Check if updating an existing custom API or adding a new one
						found := false
						for i, api := range m.apiConfig.CustomAPIs {
							if api.Name == name && api.Type == apiType {
								m.apiConfig.CustomAPIs[i] = newCustomAPI
								found = true
								break
							}
						}
						if !found {
							m.apiConfig.CustomAPIs = append(m.apiConfig.CustomAPIs, newCustomAPI)
						}
					}

					if m.err == nil {
						err := SaveAPIConfig(m.user, m.apiConfig)
						if err != nil {
							m.err = fmt.Errorf("failed to save API config: %w", err)
						} else {
							m.saveSuccess = true
							// Optionally, return to provider list after save
							m.currentView = providerListView
							m.apiKeyInput.Reset()
							m.baseURLInput.Reset()
							m.customNameInput.Reset()
							m.customTypeInput.Reset()
							m.apiKeyInput.Blur()
							m.baseURLInput.Blur()
							m.customNameInput.Blur()
							m.customTypeInput.Blur()
						}
					}
				}
			case modelSelectionView:
				var selectedModel string
				if m.currentModelListType == "llm" {
					selectedItem := m.llmModelsList.SelectedItem()
					if selectedItem == nil {
						m.err = fmt.Errorf("no LLM model selected")
						return m, nil
					}
					selectedModel = selectedItem.(providerItem).title
				} else { // image
					selectedItem := m.imageModelsList.SelectedItem()
					if selectedItem == nil {
						m.err = fmt.Errorf("no Image model selected")
						return m, nil
					}
					selectedModel = selectedItem.(providerItem).title
				}

				// Save selected model
				switch m.modelSelectionOrigin {
				case ProviderMistral:
					m.apiConfig.Mistral.DefaultLLMModel = LLMModel(selectedModel)
				case ProviderGroq:
					m.apiConfig.Groq.DefaultLLMModel = LLMModel(selectedModel)
				case ProviderHuggingFace:
					if m.currentModelListType == "llm" {
						m.apiConfig.HuggingFace.DefaultLLMModel = LLMModel(selectedModel)
					} else {
						m.apiConfig.HuggingFace.DefaultImageModel = ImageModel(selectedModel)
					}
				case ProviderOpenAI:
					if m.currentModelListType == "llm" {
						m.apiConfig.OpenAI.DefaultLLMModel = LLMModel(selectedModel)
					} else {
						m.apiConfig.OpenAI.DefaultImageModel = ImageModel(selectedModel)
					}
				case ProviderClaude:
					m.apiConfig.Claude.DefaultLLMModel = LLMModel(selectedModel)
				case ProviderAIHorde:
					m.apiConfig.AIHorde.DefaultImageModel = ImageModel(selectedModel)
				case ProviderPollinations:
					m.apiConfig.Pollinations.DefaultImageModel = ImageModel(selectedModel)
				case ProviderImageRouter:
					m.apiConfig.ImageRouter.DefaultImageModel = ImageModel(selectedModel)
				// Handle Custom APIs
				case ProviderCustomLLM:
					for i := range m.apiConfig.CustomAPIs {
						if m.apiConfig.CustomAPIs[i].Name == string(m.modelSelectionOrigin) {
							m.apiConfig.CustomAPIs[i].DefaultLLMModel = LLMModel(selectedModel)
							break
						}
					}
				case ProviderCustomImage:
					for i := range m.apiConfig.CustomAPIs {
						if m.apiConfig.CustomAPIs[i].Name == string(m.modelSelectionOrigin) {
							m.apiConfig.CustomAPIs[i].DefaultImageModel = ImageModel(selectedModel)
							break
						}
					}
				}

				if m.err == nil {
					err := SaveAPIConfig(m.user, m.apiConfig)
					if err != nil {
						m.err = fmt.Errorf("failed to save API config: %w", err)
					} else {
						m.saveSuccess = true
					}
				}
				m.currentView = providerDetailsView // Go back to the details view
				return m, nil
			}
		case "ctrl+s":
			if m.currentView == providerDetailsView {
				m.currentView = setProviderDefaultView
				return m, nil
			}
		case "alt+l":
			if m.currentView == providerDetailsView {
				// Load LLM models for the selected provider
				m.currentModelListType = "llm"
				m.modelSelectionOrigin = m.selectedProvider
				
				// Get available models based on provider
				switch m.selectedProvider {
				case ProviderMistral:
					// Mock models for testing
					models := []list.Item{
						providerItem{provider: ProviderMistral, title: "mistral-tiny", desc: "Fast and efficient"},
						providerItem{provider: ProviderMistral, title: "mistral-small", desc: "Balanced performance"},
						providerItem{provider: ProviderMistral, title: "mistral-medium", desc: "High quality"},
					}
					m.llmModelsList.SetItems(models)
				case ProviderGroq:
					models := []list.Item{
						providerItem{provider: ProviderGroq, title: "llama3-8b-8192", desc: "Fast Llama 3 8B"},
						providerItem{provider: ProviderGroq, title: "llama3-70b-8192", desc: "Powerful Llama 3 70B"},
						providerItem{provider: ProviderGroq, title: "mixtral-8x7b-32768", desc: "Mixture of Experts"},
					}
					m.llmModelsList.SetItems(models)
				case ProviderHuggingFace:
					models := []list.Item{
						providerItem{provider: ProviderHuggingFace, title: "mistralai/Mistral-7B-Instruct-v0.2", desc: "Mistral 7B"},
						providerItem{provider: ProviderHuggingFace, title: "meta-llama/Llama-2-70b-chat-hf", desc: "Llama 2 70B"},
					}
					m.llmModelsList.SetItems(models)
				case ProviderOpenAI:
					models := []list.Item{
						providerItem{provider: ProviderOpenAI, title: "gpt-3.5-turbo", desc: "Fast and affordable"},
						providerItem{provider: ProviderOpenAI, title: "gpt-4", desc: "Most capable"},
						providerItem{provider: ProviderOpenAI, title: "gpt-4-turbo", desc: "Fast and capable"},
					}
					m.llmModelsList.SetItems(models)
				case ProviderClaude:
					models := []list.Item{
						providerItem{provider: ProviderClaude, title: "claude-3-opus-20240229", desc: "Most powerful"},
						providerItem{provider: ProviderClaude, title: "claude-3-sonnet-20240229", desc: "Balanced"},
						providerItem{provider: ProviderClaude, title: "claude-3-haiku-20240307", desc: "Fast and compact"},
					}
					m.llmModelsList.SetItems(models)
				case ProviderCustomLLM:
					// For custom LLM, we'll need to fetch from the API or use a default
					models := []list.Item{
						providerItem{provider: ProviderCustomLLM, title: "custom-model-1", desc: "Custom LLM Model"},
					}
					m.llmModelsList.SetItems(models)
				default:
					// For other providers, use a generic list or fetch from API
					models := []list.Item{
						providerItem{provider: m.selectedProvider, title: "default-model", desc: "Default model"},
					}
					m.llmModelsList.SetItems(models)
				}
				
				m.currentView = modelSelectionView
				return m, nil
			}
		case "1", "2":
			if m.currentView == setProviderDefaultView {
				if msg.String() == "1" {
					m.apiConfig.SelectedLLMProvider = m.selectedProvider
				} else {
					m.apiConfig.SelectedImageProvider = m.selectedProvider
				}
				err := SaveAPIConfig(m.user, m.apiConfig)
				if err != nil {
					m.err = fmt.Errorf("failed to save API config: %w", err)
				} else {
					m.saveSuccess = true
				}
				m.currentView = providerDetailsView // Go back to the details view
				return m, nil
			}
				        case "tab": // For cycling inputs in details view
				            if m.currentView == providerDetailsView || m.currentView == customProviderDetailsView {
				                if m.apiKeyInput.Focused() {
				                    m.apiKeyInput.Blur()
				                    m.baseURLInput.Focus()
				                    return m, nil
				                } else if m.baseURLInput.Focused() && m.selectedProvider != ProviderCustomLLM && m.selectedProvider != ProviderCustomImage {
				                    m.baseURLInput.Blur()
				                    m.apiKeyInput.Focus() // Cycle back to API Key
				                    return m, nil
				                } else if m.baseURLInput.Focused() && (m.selectedProvider == ProviderCustomLLM || m.selectedProvider == ProviderCustomImage) {
				                    m.baseURLInput.Blur()
				                    m.customTypeInput.Focus()
				                    return m, nil
				                } else if m.customTypeInput.Focused() {
				                    m.customTypeInput.Blur()
				                    m.customNameInput.Focus()
				                    return m, nil
				                } else if m.customNameInput.Focused() {
				                    m.customNameInput.Blur()
				                    m.apiKeyInput.Focus()
				                    return m, nil
				                }
				            }
				        }	}

	// Update the focused text input
	if m.currentView == providerDetailsView || m.currentView == customProviderDetailsView {
		if m.apiKeyInput.Focused() {
			m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
			cmds = append(cmds, cmd)
		}
		if m.baseURLInput.Focused() {
			m.baseURLInput, cmd = m.baseURLInput.Update(msg)
			cmds = append(cmds, cmd)
		}
		if m.customNameInput.Focused() {
			m.customNameInput, cmd = m.customNameInput.Update(msg)
			cmds = append(cmds, cmd)
		}
		if m.customTypeInput.Focused() {
			m.customTypeInput, cmd = m.customTypeInput.Update(msg)
			cmds = append(cmds, cmd)
		}
	} else if m.currentView == modelSelectionView {
		if m.currentModelListType == "llm" {
			m.llmModelsList, cmd = m.llmModelsList.Update(msg)
		} else {
			m.imageModelsList, cmd = m.imageModelsList.Update(msg)
		}
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m apiSettingsModel) View() string {
	var s string
	var help string
	var status string

	if m.err != nil {
		status = apiSettingsErrorStyle.Render(fmt.Sprintf("⛔ Error: %v", m.err)) + "\n"
	} else if m.saveSuccess {
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render("✅ Settings saved successfully!") + "\n"
	}

	switch m.currentView {
	case providerListView:
		s = apiSettingsTitleStyle.Render("API Settings ⚙️") + "\n\n" + m.providerList.View()
		help = "↑/↓ move • enter select • esc back to main • q quit"
	case providerDetailsView:
		s = apiSettingsTitleStyle.Render(fmt.Sprintf("%s Settings", m.selectedProvider)) + "\n\n" +
			apiSettingsInputPromptStyle.Render("API Key:") + "\n" +
			m.apiKeyInput.View() + "\n\n" +
			apiSettingsInputPromptStyle.Render("Base URL (optional):") + "\n" +
			m.baseURLInput.View() + "\n\n"
		help = "tab cycle fields • enter save • ctrl+s set as default • esc back to list • q quit"
	case customProviderDetailsView:
		s = apiSettingsTitleStyle.Render(fmt.Sprintf("Custom API Settings (%s)", m.selectedProvider)) + "\n\n" +
			apiSettingsInputPromptStyle.Render("Custom API Name:") + "\n" +
			m.customNameInput.View() + "\n\n" +
			apiSettingsInputPromptStyle.Render("API Type ('llm' or 'image'):") + "\n" +
			m.customTypeInput.View() + "\n\n" +
			apiSettingsInputPromptStyle.Render("API Key (optional):") + "\n" +
			m.apiKeyInput.View() + "\n\n" +
			apiSettingsInputPromptStyle.Render("Endpoint URL:") + "\n" +
			m.baseURLInput.View() + "\n\n"
		help = "tab cycle fields • enter save • esc back to list • q quit"
	case setProviderDefaultView:
		s = apiSettingsTitleStyle.Render(fmt.Sprintf("Set %s as default for:", m.selectedProvider)) + "\n\n" +
			"1. LLM\n" +
			"2. Image\n"
		help = "1 or 2 to select • esc to cancel"
	case modelSelectionView:
		title := fmt.Sprintf("Select %s Model for %s", strings.ToUpper(m.currentModelListType), m.modelSelectionOrigin)
		s = apiSettingsTitleStyle.Render(title) + "\n\n"
		if m.currentModelListType == "llm" {
			s += m.llmModelsList.View()
		} else {
			s += m.imageModelsList.View()
		}
		help = "↑/↓ move • enter select • esc back to provider details"
	}

	return apiSettingsBoxStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left,
			s,
			status,
			apiSettingsHelpStyle.Render(help),
		),
	)
}