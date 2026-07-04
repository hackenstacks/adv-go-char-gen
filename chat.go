package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Chat styles
var (
	chatTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#ADD8E6")). // Light Blue
				Padding(1, 4).
				Align(lipgloss.Center).
				Bold(true)

	chatItemStyle = lipgloss.NewStyle().
			PaddingLeft(2).
			Foreground(lipgloss.Color("#B0C4DE")) // Light Steel Blue

	chatSelectedItemStyle = lipgloss.NewStyle().
					PaddingLeft(2).
					Foreground(lipgloss.Color("#00FFFF")). // Cyan
					Border(lipgloss.RoundedBorder(), false, false, false, true).
					BorderForeground(lipgloss.Color("#00FFFF"))

	chatPromptStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#ADD8E6")) // Light Blue

	chatFocusedInputStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#FFFF00")). // Yellow for focused input
					BorderBottom(true).
					BorderBottomForeground(lipgloss.Color("#FFFF00"))

	chatBlurredInputStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#696969")). // Dim Gray for unfocused input
					BorderBottom(true).
					BorderBottomForeground(lipgloss.Color("#696969"))

	chatErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF0000")). // Red for errors
				Bold(true)

	chatHelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#757575")). // Grey
			PaddingTop(1)

	chatBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#ADD8E6")). // Light Blue
			Padding(1, 2).
			Width(80) // Default width, will be adjusted by WindowSizeMsg

	chatTimestampStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240")) // Grey for timestamps
	chatScrollbarThumb = lipgloss.NewStyle().Background(lipgloss.Color("6"))
	chatScrollbarTrack = lipgloss.NewStyle().Background(lipgloss.Color("237"))
)

type MessageType string

const (
	MessageTypeUser      MessageType = "user"
	MessageTypeCharacter MessageType = "character"
	MessageTypeSystem    MessageType = "system"
	MessageTypeImage     MessageType = "image"
	MessageTypeCommand   MessageType = "command" // For commands that are echoed back
	MessageTypeSummary   MessageType = "summary"
)

type APIProvider = Provider

type chatState int

const (
	chatting chatState = iota
	selectingLLMProvider
	selectingLLMModel // New state for selecting LLM model in chat
	selectingImageProvider
	selectingImageModel // New state for selecting Image model in chat
)

// Message represents a single chat turn.
type Message struct {
	ID        string            `json:"id"`
	Sender    string            `json:"sender"`
	Content   string            `json:"content"`
	Timestamp int64             `json:"timestamp"`
	Recipient string            `json:"recipient,omitempty"` // For DMs
	Type      MessageType       `json:"type"`       // Type of message (user, character, system, image, etc.)
	Metadata  map[string]string `json:"metadata,omitempty"` // Additional data, e.g., image URL for image messages
}

// ChatMsg is a message that represents a single chat turn.
type ChatMsg struct {
	Message Message
	Error   error
}

type chatModel struct {
	user         *User
	apiConfig    *APIConfig
	memoryManager MemoryManager
	llmService   LLMService
	imageService ImageService

	messages   []Message
	textInput  textinput.Model
	viewport   viewport.Model
	err        error

	state                chatState
	providerChoices      []string
	providerCursor       int
	selectedLLMProvider  APIProvider
	selectedImageProvider APIProvider
	selectedLLMModel     string // The chosen LLM model for chat
	selectedImageModel   string // The chosen Image model for chat
	llmModels            []string // Available LLM models for the selected provider in chat
	imageModels          []string // Available Image models for the selected provider in chat

	// Current chat participants (characters + user)
	participants []string
	activeCharacterID string // ID of the currently active character in conversation
	selectedCharacterID string // ID of the character the user is currently talking to
	systemPrompt string // Stores the active system prompt for LLM interaction

	// UI specific
	chatHeight int
	chatWidth  int
	turn       int // 0 for user, 1 for first char, 2 for second, etc.
	chatSessionID string // Unique ID for the current chat session

	// Card chat mode: ephemeral chat with a PNG card persona (no library)
	cardChatMode bool
	cardName     string
	cardAvatar   string // large header image of the character
	cardAvatarSm string // compact avatar shown beside each response
	busy         bool
}

func initialChatModel(loggedInUser *User, apiCfg *APIConfig, mm MemoryManager, llmSvc LLMService, imgSvc ImageService) chatModel {

ti := textinput.New()
ti.Placeholder = "Type your message or command..."
ti.Focus()
ti.Width = 80
ti.Prompt = ">> "

vp := viewport.New(80, 20)


	return chatModel{
		user:         loggedInUser,
		apiConfig:    apiCfg,
		memoryManager: mm,
		llmService:   llmSvc,
		imageService: imgSvc,
		messages:     []Message{},
		textInput:    ti,
		viewport: vp,
		participants: []string{loggedInUser.Username}, // User is always a participant
		systemPrompt: "", // Initialize system prompt to empty
		state: chatting,
		selectedLLMProvider: ProviderMock, // Default to mock, or first available from config
		selectedImageProvider: ProviderMock, // Default to mock, or first available from config
		selectedLLMModel: "",
		selectedImageModel: "",
		llmModels:            []string{},
		imageModels:          []string{},
		turn:                 0, // User starts
		chatSessionID:        "", // Initialize chat session ID
	}
}

// SaveChatSession saves the current chat messages for a character.
func SaveChatSession(user *User, characterID string, messages []Message) error {
	if user == nil || characterID == "" {
		return fmt.Errorf("invalid user or character ID for saving chat session")
	}

	messagesBytes, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("failed to marshal chat messages: %w", err)
	}

	// Use characterID as the dataName for the chat session
	return SaveEncryptedData(user, user.Username, "chat_sessions", characterID, messagesBytes)
}

// LoadChatSession loads previous chat messages for a character.
func LoadChatSession(user *User, characterID string) ([]Message, error) {
	if user == nil || characterID == "" {
		return nil, fmt.Errorf("invalid user or character ID for loading chat session")
	}

	messagesBytes, err := LoadEncryptedData(user, user.Username, "chat_sessions", characterID)
	if err != nil {
		// If chat session not found, return empty messages rather than error
		if strings.Contains(err.Error(), "not found") {
			return []Message{}, nil
		}
		return nil, fmt.Errorf("failed to load encrypted chat messages: %w", err)
	}

	var messages []Message
	if err := json.Unmarshal(messagesBytes, &messages); err != nil {
		return nil, fmt.Errorf("failed to unmarshal chat messages: %w", err)
	}
	return messages, nil
}


func (m chatModel) Init() tea.Cmd {
	return nil
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.chatHeight = msg.Height - lipgloss.Height(m.textInput.View()) - 5 // Adjust for input and padding
		m.chatWidth = msg.Width
		if m.cardChatMode {
			// Two-column: avatar (~30) on the left, conversation on the right
			m.viewport.Width = msg.Width - 34
			if m.viewport.Width < 30 { m.viewport.Width = 30 }
		} else {
			m.viewport.Width = msg.Width
		}
		m.viewport.Height = m.chatHeight
		m.viewport.SetContent(m.renderMessages())

	case SetChatCharacterMsg:
		char, err := LoadCharacter(m.user, msg.CharacterID)
		if err != nil {
			m.err = fmt.Errorf("failed to load character for chat: %w", err)
			return m, nil
		}
		m.activeCharacterID = char.ID
		m.selectedCharacterID = char.Name
		m.chatSessionID = GenerateRandomID() // Generate a new session ID for this chat

		// Load previous chat history
		history, err := LoadChatSession(m.user, m.activeCharacterID)
		if err != nil {
			m.err = fmt.Errorf("failed to load chat history: %w", err)
			m.messages = []Message{} // Start with empty if load fails
		} else {
			m.messages = history
		}

		if len(m.messages) == 0 && char.FirstMessage != "" {
			firstMsg := Message{
				ID:        GenerateRandomID(),
				Sender:    char.Name,
				Content:   char.FirstMessage,
				Timestamp: time.Now().Unix(),
				Type:      MessageTypeCharacter,
			}
			m.messages = append(m.messages, firstMsg)
		}
		systemMsg := Message{
			ID:        GenerateRandomID(),
			Sender:    "System",
			Content:   fmt.Sprintf("Started chat with %s.", char.Name),
			Timestamp: time.Now().Unix(),
			Type:      MessageTypeSystem,
		}
		m.messages = append(m.messages, systemMsg)
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

		return m, nil
	case tea.KeyMsg:
		if m.state == selectingLLMProvider || m.state == selectingImageProvider || m.state == selectingLLMModel || m.state == selectingImageModel {
			switch msg.String() {
			case "up", "k":
				if m.providerCursor > 0 {
					m.providerCursor--
				}
			case "down", "j":
				if m.providerCursor < len(m.providerChoices)-1 {
					m.providerCursor++
				}
			case "enter":
				if m.state == selectingLLMProvider {
					m.selectedLLMProvider = Provider(m.providerChoices[m.providerCursor])
					m.state = selectingLLMModel
					llmSvc, err := LLMFactory(m.selectedLLMProvider, *m.apiConfig)
					if err != nil {
						m.err = fmt.Errorf("failed to initialize LLM service for model listing: %w", err)
						m.providerChoices = []string{}
					} else {
						models, err := llmSvc.GetAvailableModels(context.Background())
						if err != nil {
							m.err = fmt.Errorf("failed to get LLM models from %s: %w", m.selectedLLMProvider, err)
							m.providerChoices = []string{}
						} else {
							llmModels := make([]string, len(models))
							for i, v := range models {
								llmModels[i] = string(v)
							}
							m.llmModels = llmModels
							m.providerChoices = providersToChoices(m.llmModels)
						}
					}
					m.providerCursor = 0
				} else if m.state == selectingImageProvider {
					m.selectedImageProvider = Provider(m.providerChoices[m.providerCursor])
					m.state = selectingImageModel
					imgSvc, err := ImageFactory(m.selectedImageProvider, *m.apiConfig)
					if err != nil {
						m.err = fmt.Errorf("failed to initialize Image service for model listing: %w", err)
						m.providerChoices = []string{}
					} else {
						models, err := imgSvc.GetAvailableModels(context.Background())
						if err != nil {
							m.err = fmt.Errorf("failed to get Image models from %s: %w", m.selectedImageProvider, err)
							m.providerChoices = []string{}
						} else {
							imageModels := make([]string, len(models))
							for i, v := range models {
								imageModels[i] = string(v)
							}
							m.imageModels = imageModels
							m.providerChoices = providersToChoices(m.imageModels)
						}
					}
					m.providerCursor = 0
				} else if m.state == selectingLLMModel {
					m.selectedLLMModel = m.providerChoices[m.providerCursor]
					m.state = selectingImageProvider
					m.providerChoices = providersToChoices(GetImageProviders())
					m.providerCursor = 0
				} else if m.state == selectingImageModel {
					m.selectedImageModel = m.providerChoices[m.providerCursor]
					llmService, err := LLMFactory(m.selectedLLMProvider, *m.apiConfig)
					if err != nil {
						m.err = fmt.Errorf("failed to create LLM service: %w", err)
					} else {
						m.llmService = llmService
					}

					imageService, err := ImageFactory(m.selectedImageProvider, *m.apiConfig)
					if err != nil {
						m.err = fmt.Errorf("failed to create Image service: %w", err)
					} else {
						m.imageService = imageService
					}
					m.state = chatting
				}
				return m, nil
			case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
				selectedNum := int(msg.String()[0] - '0')
				if selectedNum >= 0 && selectedNum < len(m.providerChoices) {
					m.providerCursor = selectedNum
					return m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Simulate enter press
				}
			}
			return m, nil
		}


		switch msg.String() {
		case "ctrl+c":
			// Save chat session before quitting (library chats only)
			if m.activeCharacterID != "" && len(m.messages) > 0 {
				if err := SaveChatSession(m.user, m.activeCharacterID, m.messages); err != nil {
					fmt.Fprintf(os.Stderr, "Error saving chat session: %v\n", err)
				}
			}
			return m, tea.Quit
		case "esc":
			switch m.state {
			case chatting:
				// Save library chat session before going back
				if m.activeCharacterID != "" && len(m.messages) > 0 {
					if err := SaveChatSession(m.user, m.activeCharacterID, m.messages); err != nil {
						fmt.Fprintf(os.Stderr, "Error saving chat session: %v\n", err)
					}
				}
				// Card chats return to the browser; library chats to main menu
				if m.cardChatMode {
					return m, func() tea.Msg { return ShowCardBrowserMsg{} }
				}
				return m, func() tea.Msg { return BackToMainAppMsg{} }
			case selectingLLMProvider: // From LLM provider selection, go back to main chat if no LLM chosen
				m.state = chatting
				return m, nil
			case selectingLLMModel: // From LLM model selection, go back to LLM provider selection
				m.state = selectingLLMProvider
				m.providerChoices = providersToChoices(GetLLMProviders()) // Reload provider choices
				m.providerCursor = 0
				return m, nil
			case selectingImageProvider: // From Image provider selection, go back to LLM model selection
				m.state = selectingLLMModel
				if models, err := m.llmService.GetAvailableModels(context.Background()); err != nil {
					m.err = fmt.Errorf("failed to get LLM models from %s: %w", m.selectedLLMProvider, err)
					m.providerChoices = []string{}
				} else {
					llmModels := make([]string, len(models))
					for i, v := range models {
						llmModels[i] = string(v)
					}
					m.providerChoices = llmModels
				}
				m.providerCursor = 0
				return m, nil
			case selectingImageModel: // From Image model selection, go back to Image provider selection
				m.state = selectingImageProvider
				m.providerChoices = providersToChoices(GetImageProviders()) // Reload provider choices
				m.providerCursor = 0
				return m, nil
			}
			return m, nil

		case "enter":
			input := m.textInput.Value()
			m.textInput.SetValue("") // Clear input field

			if strings.TrimSpace(input) == "" {
				return m, nil
			}

			// Check if the input is a command
			command, args, isCommand := parseCommand(input)
			if isCommand {
				// Echo command to chat
				commandMessage := Message{
					ID: GenerateRandomID(),
					Sender: m.user.Username,
					Content: input,
					Timestamp: time.Now().Unix(),
					Type: MessageTypeCommand,
				}
			m.messages = append(m.messages, commandMessage)
				return m, m.handleCommand(command, args)
			}

			// ── Card chat mode: direct LLM call using the card's system prompt ──
			if m.cardChatMode {
				userMsg := Message{
					ID: GenerateRandomID(), Sender: m.user.Username,
					Content: input, Timestamp: time.Now().Unix(), Type: MessageTypeUser,
				}
				m.messages = append(m.messages, userMsg)
				m.busy = true
				m.viewport.SetContent(m.renderMessages())
				m.viewport.GotoBottom()
				return m, m.cardChatSend()
			}

			userMessage := Message{
				ID: GenerateRandomID(),
				Sender: m.user.Username,
				Content: input,
				Timestamp: time.Now().Unix(),
				Type: MessageTypeUser, // Set message type
			}
			m.messages = append(m.messages, userMessage)

			// Store user message as episodic memory
			memEntry := &MemoryEntry{
				Content: userMessage.Content,
				Type: "episodic",
				Source: fmt.Sprintf("Chat with %s", m.selectedCharacterID),
				Keywords: []string{m.user.Username, m.selectedCharacterID},
			}
			if m.selectedCharacterID != "" { // Only save if a character is selected
				if err := m.memoryManager.AddMemory(m.user, m.selectedCharacterID, memEntry, m.apiConfig); err != nil {
					m.err = fmt.Errorf("failed to save user message to memory: %w", err)
				}
			}


			if m.selectedCharacterID == "" {
				systemMessage := Message{
					ID: GenerateRandomID(),
					Sender: "System",
					Content: "Please select a character to chat with using `/character <name>` or invite one with `/invite <name>`.",
					Timestamp: time.Now().Unix(),
					Type: MessageTypeSystem, // Set message type
				}
			m.messages = append(m.messages, systemMessage)
			m.viewport.SetContent(m.renderMessages())
			m.viewport.GotoBottom()
				return m, nil
			}

			            // Determine next character to respond
			            m.turn = (m.turn + 1) % len(m.participants)
			            if m.participants[m.turn] == m.user.Username {
			                // It's the user's turn again, so we just wait for input.
			                // This logic will be more relevant when AI characters can talk to each other.
			                // For now, we cycle back to the first character if it's the user's turn.
			                if len(m.participants) > 1 {
			                    m.turn = 1
			                } else {
			                    m.turn = 0 // Stay on user if no characters
			                }
			            }
			
			            if m.turn > 0 && len(m.participants) > 1 {
			                nextCharName := m.participants[m.turn]
			                charInfo, err := findCharacterInfoByName(m.user, nextCharName)
			                if err != nil {
			                    m.err = fmt.Errorf("failed to find next character '%s' in participant list", nextCharName)
			                    return m, nil
			                }
			                m.activeCharacterID = charInfo.ID // Set the active character for the response
			
			                				// LLM response generation for the next character
			                				go func(characterID string, currentInput string) tea.Msg {
												var promptBuilder strings.Builder
			                					char, err := LoadCharacter(m.user, characterID)
			                					if err != nil {
			                						return ChatMsg{Error: fmt.Errorf("failed to load active character '%s': %w", characterID, err)}
			                					}
			                					retrievedMemories, _ := m.memoryManager.RetrieveMemories(m.user, char.ID, currentInput, 5, m.apiConfig)
			                										// Use the default model from the retrieved config, or the chat model's selected model if it overrides it.
			                										// For now, we prioritize m.selectedLLMModel from the chat state.
			                										usedLLMModel := LLMModel(m.selectedLLMModel)
			                										if usedLLMModel == "" {
			                											// Fallback if chat model hasn't explicitly selected one, use the config's default.
			                											// This requires DefaultLLMModel to be part of CommonAPIConfig or a more specific struct
			                											// For now, let's assume a reasonable default or error if none.
			                											return ChatMsg{Error: fmt.Errorf("no LLM model selected for provider '%s'", m.selectedLLMProvider)}
			                										}
			                					
			                										// Construct LLM prompt
			                										promptBuilder.WriteString(fmt.Sprintf("You are %s. Your goal is to roleplay as %s based on the following details. Always respond in character.\n", char.Name, char.Name))
			                										promptBuilder.WriteString(fmt.Sprintf("DESCRIPTION: %s\n", char.Description))
			                										promptBuilder.WriteString(fmt.Sprintf("PERSONALITY: %s\n", char.Personality))
			                										promptBuilder.WriteString(fmt.Sprintf("SCENARIO: %s\n", char.Scenario))
			                										if char.FirstMessage != "" {
			                											promptBuilder.WriteString(fmt.Sprintf("Your first message was: %s\n", char.FirstMessage))
			                										}
			                										if len(char.Lorebook) > 0 {
			                											promptBuilder.WriteString("\n--- LOREBOOK ENTRIES ---\n")
			                											for key, value := range char.Lorebook {
			                												promptBuilder.WriteString(fmt.Sprintf("%s: %s\n", key, value))
			                											}
			                										}
			                										promptBuilder.WriteString("\n--- Retrieved Memories ---\n")
			                										if len(retrievedMemories) > 0 {
			                											for _, mem := range retrievedMemories {
			                												promptBuilder.WriteString(fmt.Sprintf("- %s (Type: %s, Source: %s): %s\n", mem.ID, mem.Type, mem.Source, mem.Content))
			                											}
			                										} else {
			                											promptBuilder.WriteString("No particularly relevant memories retrieved for this turn.\n")
			                										}
			                										promptBuilder.WriteString("\n--- Conversation History (Recent) ---\n")
			                										historyStart := 0
			                										if len(m.messages) > 10 { // Consider last 10 messages for immediate context
			                											historyStart = len(m.messages) - 10
			                										}
			                										for i := historyStart; i < len(m.messages); i++ {
			                											msg := m.messages[i]
			                											promptBuilder.WriteString(fmt.Sprintf("%s: %s\n", msg.Sender, msg.Content))
			                										}
			                										promptBuilder.WriteString(fmt.Sprintf("CURRENT CONVERSATION TURN - Your response as %s: ", char.Name))			                
			                					fullPrompt := promptBuilder.String()
			                
			                					llmResponseContent, err := m.llmService.GenerateResponse(context.Background(), fullPrompt, usedLLMModel, *m.apiConfig)
			                					if err != nil {
			                						return ChatMsg{Error: fmt.Errorf("failed to get LLM response from '%s': %w", char.Name, err)}
			                					}
			                					responseMsg := Message{
			                						ID:        GenerateRandomID(),
			                						Sender:    char.Name,
			                						Content:   llmResponseContent,
			                						Timestamp: time.Now().Unix(),
			                						Type:      MessageTypeCharacter,
			                					}
			                					memEntry := &MemoryEntry{Content: responseMsg.Content, Type: "episodic", Source: "Chat response"}
			                					m.memoryManager.AddMemory(m.user, char.ID, memEntry, m.apiConfig)
			                					return ChatMsg{Message: responseMsg}
			                				}(m.activeCharacterID, input)
			                			}
			                			// The rest of the original goroutine is now inside the turn-based logic			
			
			// Check for rolling summary
			if len(m.messages) >= 20 { // Summarize every 20 messages
				go func() tea.Msg {
					summaryPrompt := "Summarize the conversation so far, focusing on key themes and character interactions."
					rimmedMsgs, err := m.memoryManager.GenerateRollingSummary(m.user, m.selectedCharacterID, m.messages, summaryPrompt, m.apiConfig)
					if err != nil {
						return ChatMsg{Error: fmt.Errorf("failed to generate rolling summary: %w", err)}
					} else {
						// Replace old messages with trimmed ones (summary + recent messages)
						// The actual summary is added as a system message by GenerateRollingSummary
						m.messages = rimmedMsgs
						return nil // Or a message to update the UI specifically for summary
					}
				}()
			}
		}
	case ChatMsg:
		m.busy = false
		if msg.Error != nil {
			m.err = msg.Error
		} else {
			m.messages = append(m.messages, msg.Message)
			m.viewport.SetContent(m.renderMessages())
			m.viewport.GotoBottom()
		}
		return m, nil

	case AI2AICompleteMsg:
		if msg.Err != nil {
			m.err = msg.Err
		} else {
			m.messages = append(m.messages, msg.Intro)
			for _, aiMsg := range msg.Messages {
				m.messages = append(m.messages, aiMsg)
			}
			m.viewport.SetContent(m.renderMessages())
			m.viewport.GotoBottom()
		}
		return m, nil

	case imageGeneratedMsg:
		m.busy = false
		m.messages = append(m.messages, Message{
			ID: GenerateRandomID(), Sender: "System",
			Content:   "🖼️  Image saved → " + msg.path + "  (opening preview…)",
			Timestamp: time.Now().Unix(), Type: MessageTypeImage,
			Metadata: map[string]string{"image_path": msg.path},
		})
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		// Show it fullscreen in true-color sixel
		return m, runImagePreviewCmd(msg.path, "🎨 "+msg.prompt)

	case imagePreviewClosedMsg:
		m.viewport.SetContent(m.renderMessages())
		return m, nil

	case compactDoneMsg:
		m.busy = false
		// Replace conversation with a system recap, keeping it going
		m.messages = []Message{{ID: GenerateRandomID(), Sender: "System",
			Content: "📜 Conversation compacted:\n" + msg.summary,
			Timestamp: time.Now().Unix(), Type: MessageTypeSummary}}
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil

	case modelsListMsg:
		var content strings.Builder
		content.WriteString("Available models:\n")
		for _, mdl := range msg.models {
			content.WriteString("  " + mdl + "\n")
		}
		sysMsg := Message{ID: GenerateRandomID(), Sender: "System",
			Content: content.String(), Timestamp: time.Now().Unix(), Type: MessageTypeSystem}
		m.messages = append(m.messages, sysMsg)
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)
	m.textInput, cmd = m.textInput.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m chatModel) View() string {
	if m.state == selectingLLMProvider || m.state == selectingImageProvider || m.state == selectingLLMModel || m.state == selectingImageModel {
		return m.renderProviderSelection()
	}
	status := ""
	if m.busy {
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF9800")).Render("  ⏳ " + m.cardName + " is thinking…")
	} else if m.err != nil {
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff4444")).Render("  ⚠ " + m.err.Error())
	}

	// Card chat: editor-style two columns — avatar left, conversation right
	if m.cardChatMode && m.cardAvatar != "" {
		border := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#1e3a5f"))
		nameLbl := lipgloss.NewStyle().Foreground(lipgloss.Color("#00d4ff")).Bold(true).
			Render("⬡ " + m.cardName)
		leftPane := border.Padding(0, 1).Render(m.cardAvatar + "\n" + nameLbl)
		rightPane := border.Render(m.viewport.View())
		body := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, "  ", rightPane)
		return fmt.Sprintf("%s\n%s\n%s", body, m.textInput.View(), status)
	}
	return fmt.Sprintf("%s\n%s\n%s", m.viewport.View(), m.textInput.View(), status)
}


func (m *chatModel) renderMessages() string {
	var sb strings.Builder

	// Display messages
	for _, msg := range m.messages {
		// DM visibility logic
		if msg.Recipient != "" && msg.Sender != m.user.Username && msg.Recipient != m.user.Username {
			continue // Don't show DMs not involving the user
		}

		var senderStyle lipgloss.Style
		var contentStyle lipgloss.Style
		var privateIndicator string

		if msg.Recipient != "" {
			privateIndicator = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(" (private) ")
		}

		timestamp := chatTimestampStyle.Render(time.Unix(msg.Timestamp, 0).Format("15:04")) + " "

		// Editor-style label formatting for card chat: colored name header, text below
		if m.cardChatMode && (msg.Type == MessageTypeUser || msg.Type == MessageTypeCharacter) {
			w := m.viewport.Width - 2
			if w < 20 { w = 20 }
			var lbl lipgloss.Style
			if msg.Type == MessageTypeUser {
				lbl = lipgloss.NewStyle().Foreground(lipgloss.Color("#00d4ff")).Bold(true) // cyan (You)
			} else {
				lbl = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff88")).Bold(true) // green (char)
			}
			body := lipgloss.NewStyle().Foreground(lipgloss.Color("#C5C5C5")).Width(w).Render(msg.Content)
			sb.WriteString(lbl.Render(msg.Sender) + "\n" + body + "\n\n")
			continue
		}

		switch msg.Type {
		case MessageTypeUser:
			senderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#87D7FF")).Bold(true) // Light Blue
			contentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#C5C5C5")) // Light Gray
			sb.WriteString(timestamp + senderStyle.Render(msg.Sender + ":") + privateIndicator + " " + contentStyle.Render(msg.Content) + "\n")
		case MessageTypeCharacter:
			senderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#87FF87")).Bold(true) // Light Green
			contentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#C5C5C5")) // Light Gray
			sb.WriteString(timestamp + senderStyle.Render(msg.Sender + ":") + privateIndicator + " " + contentStyle.Render(msg.Content) + "\n")
		case MessageTypeSystem:
			senderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700")).Bold(true) // Gold
			contentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF87")) // Light Gold
			sb.WriteString(timestamp + senderStyle.Render("SYSTEM:") + " " + contentStyle.Render(msg.Content) + "\n")
		case MessageTypeCommand:
			senderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#87AFFF")).Bold(true) // Light Purple
			contentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#AFAFAF")) // Gray
			sb.WriteString(timestamp + senderStyle.Render("CMD:") + " " + contentStyle.Render(msg.Content) + "\n")
		case MessageTypeImage:
			senderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF87D7")).Bold(true) // Magenta
			contentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFC0CB")) // Pink
			if imgPath, ok := msg.Metadata["image_path"]; ok && imgPath != "" {
				sb.WriteString(timestamp + senderStyle.Render("IMAGE:") + privateIndicator + " " + contentStyle.Render(fmt.Sprintf("Generated Image (Path: %s)", imgPath)) + "\n")
			} else {
				sb.WriteString(timestamp + senderStyle.Render("IMAGE:") + privateIndicator + " " + contentStyle.Render("Generated Image (URL not available)") + "\n")
			}
		case MessageTypeSummary:
			senderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#AFFF87")).Bold(true) // Pale Green
			contentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#E0FFE0")) // Very Pale Green
			sb.WriteString(timestamp + senderStyle.Render("SUMMARY:") + " " + contentStyle.Render(msg.Content) + "\n")
		default:
			senderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true) // White
			contentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#EFEFEF")) // Off-white
			sb.WriteString(timestamp + senderStyle.Render(msg.Sender + ":") + privateIndicator + " " + contentStyle.Render(msg.Content) + "\n")
		}
	}
	return sb.String()
}


// parseCommand checks if the input is a command (starts with '/')
// and extracts the command and its arguments.
func parseCommand(input string) (command string, args string, isCommand bool) {
	if !strings.HasPrefix(input, "/") {
		return "", "", false
	}
	parts := strings.Fields(input[1:]) // Remove '/' and split by space
	if len(parts) == 0 {
		return "", "", false // Just a "/"
	}
	command = parts[0]
	if len(parts) > 1 {
		args = strings.Join(parts[1:], " ")
	}
	return command, args, true
}

// cardChatSend builds a prompt from the card system prompt + recent history and
// calls the configured LLM service directly (no library, no memory required).
func (m *chatModel) cardChatSend() tea.Cmd {
	svc := m.llmService
	sysPrompt := m.systemPrompt
	cardName := m.cardName
	userName := m.user.Username
	model := LLMModel(m.selectedLLMModel)
	apiCfg := *m.apiConfig

	// Snapshot recent history
	history := make([]Message, len(m.messages))
	copy(history, m.messages)

	return func() tea.Msg {
		var b strings.Builder
		b.WriteString(sysPrompt)
		b.WriteString("\n\nYou are ")
		b.WriteString(cardName)
		b.WriteString(". Stay in character. Respond only as ")
		b.WriteString(cardName)
		b.WriteString(".\n\n--- Conversation ---\n")
		start := 0
		if len(history) > 16 {
			start = len(history) - 16
		}
		for i := start; i < len(history); i++ {
			msg := history[i]
			if msg.Type == MessageTypeSystem || msg.Type == MessageTypeCommand {
				continue
			}
			b.WriteString(msg.Sender + ": " + msg.Content + "\n")
		}
		b.WriteString(cardName + ": ")

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		resp, err := svc.GenerateResponse(ctx, b.String(), model, apiCfg)
		if err != nil {
			return ChatMsg{Error: fmt.Errorf("%s (%s): %w", cardName, userName, err)}
		}
		return ChatMsg{Message: Message{
			ID: GenerateRandomID(), Sender: cardName,
			Content: strings.TrimSpace(resp), Timestamp: time.Now().Unix(),
			Type: MessageTypeCharacter,
		}}
	}
}

// handleCommand processes the given chat command.
func (m *chatModel) handleCommand(command, args string) tea.Cmd {
	sysMsg := func(s string) tea.Cmd {
		return func() tea.Msg {
			return ChatMsg{Message: Message{ID: GenerateRandomID(), Sender: "System",
				Content: s, Timestamp: time.Now().Unix(), Type: MessageTypeSystem}}
		}
	}
	memID := "card:" + m.cardName

	switch command {
	case "exit":
		return func() tea.Msg {
			if m.cardChatMode {
				return ShowCardBrowserMsg{}
			}
			return BackToMainAppMsg{}
		}

	case "prompt":
		if strings.TrimSpace(args) == "" {
			return sysMsg("Usage: /prompt <direction> — steer the conversation with a temporary directive.")
		}
		m.systemPrompt += "\n\n[Director's note: " + args + "]"
		return sysMsg("📝 Directive added — it will guide upcoming replies.")

	case "commit":
		if strings.TrimSpace(args) == "" {
			return sysMsg("Usage: /commit <fact> — save a fact to this character's memory.")
		}
		entry := &MemoryEntry{Content: args, Type: "semantic", Source: "user /commit",
			Keywords: []string{m.cardName}}
		if err := m.memoryManager.AddMemory(m.user, memID, entry, m.apiConfig); err != nil {
			return sysMsg("Memory error: " + err.Error())
		}
		return sysMsg("🧠 Committed to memory: " + args)

	case "memory":
		mems, err := m.memoryManager.RetrieveMemories(m.user, memID, "", 50, m.apiConfig)
		if err != nil {
			return sysMsg("Memory error: " + err.Error())
		}
		if len(mems) == 0 {
			return sysMsg("No memories stored for " + m.cardName + " yet. Use /commit <fact>.")
		}
		var b strings.Builder
		b.WriteString("🧠 " + m.cardName + "'s memory (" + fmt.Sprintf("%d", len(mems)) + "):\n")
		for i, mem := range mems {
			if i >= 20 { break }
			b.WriteString("  • " + mem.Content + "\n")
		}
		return sysMsg(b.String())

	case "export":
		return func() tea.Msg {
			dir := filepath.Join(Paths.DataDir, "exports")
			os.MkdirAll(dir, 0755)
			name := strings.ToLower(strings.ReplaceAll(m.cardName, " ", "-"))
			if name == "" { name = "chat" }
			path := filepath.Join(dir, fmt.Sprintf("%s-%d.json", name, time.Now().Unix()))
			data, _ := json.MarshalIndent(m.messages, "", "  ")
			if err := os.WriteFile(path, data, 0644); err != nil {
				return ChatMsg{Message: Message{ID: GenerateRandomID(), Sender: "System",
					Content: "Export failed: " + err.Error(), Timestamp: time.Now().Unix(), Type: MessageTypeSystem}}
			}
			return ChatMsg{Message: Message{ID: GenerateRandomID(), Sender: "System",
				Content: "📤 Exported → " + path, Timestamp: time.Now().Unix(), Type: MessageTypeSystem}}
		}

	case "compact":
		m.busy = true
		svc := m.llmService
		model := LLMModel(m.selectedLLMModel)
		apiCfg := *m.apiConfig
		hist := make([]Message, len(m.messages))
		copy(hist, m.messages)
		return func() tea.Msg {
			var b strings.Builder
			b.WriteString("Summarize the following roleplay conversation concisely, preserving key events, facts, and the current situation. Write it as a recap.\n\n")
			for _, msg := range hist {
				if msg.Type == MessageTypeSystem || msg.Type == MessageTypeCommand { continue }
				b.WriteString(msg.Sender + ": " + msg.Content + "\n")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			summary, err := svc.GenerateResponse(ctx, b.String(), model, apiCfg)
			if err != nil {
				return ChatMsg{Error: fmt.Errorf("compact failed: %w", err)}
			}
			return compactDoneMsg{summary: strings.TrimSpace(summary)}
		}

	case "help":
		helpMessage := "Available commands:\n" +
			"/exit - leave the chat\n" +
			"/save - save this conversation\n" +
			"/export - export conversation as JSON\n" +
			"/commit <fact> - commit a fact to memory\n" +
			"/memory - view this character's memory\n" +
			"/compact - summarize the conversation so far\n" +
			"/prompt <text> - steer the conversation\n" +
			"/image <prompt> - generate an image\n" +
			"/models - list available models\n" +
			"/help - Show this help message\n" +
			"/provider - Select LLM or Image provider\n" +
			"/summarize - Generate a summary of the current conversation\n" +
			"/character {name} - Select a character to chat with\n" +
			"/image {prompt} - Generate an image based on the prompt\n" +
			"/save - Save current chat session\n" + // Added
			"/narrator {key word prompt} - Inject a narrator's message\n" + // Added
			"/ai2ai {topic} - Start an AI-to-AI conversation\n" + // Added
			"/quit - Exit the chat and return to the main menu\n" +
			"/end - End the current conversation and clear context\n" + // Added
			"/review {chat back into last summary} - Review and edit last summary\n" + // Added
			"/system prompt {used to steer conversation} - Set a system-level prompt\n" + // Added
			"/DM character {private message to character} - Send a private message\n" + // Added
			"/invite {character} - Invite another character to conversation\n" + // Added
			"/upload {path to file} - Upload a file to current context\n" + // Added
			"/topic {message} - Set or change the current topic\n" + // Added
			"/boot {character} - Remove character from conversation\n" + // Added
			"/lore {lore entry} - Add a lore entry\n" + // Added
			"/special note {message entry} - Add a special note to memory" // Added


		sysMsg := Message{
			ID:        GenerateRandomID(),
			Sender:    "System",
			Content:   helpMessage,
			Timestamp: time.Now().Unix(),
			Type:      MessageTypeSystem, // Set message type
		}
		return func() tea.Msg { return ChatMsg{Message: sysMsg} }
	case "models":
		svc := m.llmService
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			mdls, err := svc.GetAvailableModels(ctx)
			if err != nil {
				return modelsListMsg{models: []string{"Error: " + err.Error()}}
			}
			names := make([]string, len(mdls))
			for i, m := range mdls {
				names[i] = string(m)
			}
			return modelsListMsg{models: names}
		}

	case "provider":
		m.state = selectingLLMProvider
		m.providerChoices = providersToChoices(GetLLMProviders())
		m.providerCursor = 0
		return nil
	case "summarize":
		// Trigger immediate summarization
		sysMsg := Message{
			ID:        GenerateRandomID(),
			Sender:    "System",
			Content:   "Generating summary... please wait.",
			Timestamp: time.Now().Unix(),
			Type:      MessageTypeSystem, // Set message type
		}
		// Return a command that performs the summarization asynchronously
		return tea.Batch(
			func() tea.Msg { return ChatMsg{Message: sysMsg} },
			func() tea.Msg {
				summaryPrompt := "Summarize the conversation so far, focusing on key themes and character interactions."
				rimmedMsgs, err := m.memoryManager.GenerateRollingSummary(m.user, m.selectedCharacterID, m.messages, summaryPrompt, m.apiConfig)
				if err != nil {
					return ChatMsg{Error: fmt.Errorf("failed to generate summary: %w", err)}
				}
				// The actual summary message is added within GenerateRollingSummary and returned as part of trimmedMsgs.
				// We need to update m.messages with the result.
				m.messages = rimmedMsgs // This updates the chat history
				return ChatMsg{Message: Message{ // Provide feedback that summary was processed
					ID: GenerateRandomID(),
					Sender: "System",
					Content: "Conversation summarized and context trimmed.",
					Timestamp: time.Now().Unix(),
					Type: MessageTypeSystem,
				}}
			},
		)
	case "character":
		if args == "" {
			// List available characters
			chars, err := ListCharacters(m.user)
			if err != nil {
				sysMsg := Message{
					ID: GenerateRandomID(), Sender: "System",
					Content: fmt.Sprintf("Error listing characters: %v", err),
					Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
				}
				return func() tea.Msg { return ChatMsg{Message: sysMsg} }
			}
			if len(chars) == 0 {
				sysMsg := Message{
					ID: GenerateRandomID(), Sender: "System",
					Content: "No characters found. Generate or import one first! 😅",
					Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
				}
				return func() tea.Msg { return ChatMsg{Message: sysMsg} }
			}

			var charList strings.Builder
			charList.WriteString("Available characters:\n")
			for _, charInfo := range chars {
				charList.WriteString(fmt.Sprintf("- %s (ID: %s)\n", charInfo.Name, charInfo.ID))
			}
			charList.WriteString("\nUse /character <name> to select one.")
			sysMsg := Message{
				ID: GenerateRandomID(), Sender: "System",
				Content: charList.String(),
				Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
			}
			return func() tea.Msg { return ChatMsg{Message: sysMsg} }
		} else {
			// Select character by name
			characterName := args
			chars, err := ListCharacters(m.user)
			if err != nil {
				sysMsg := Message{
					ID: GenerateRandomID(), Sender: "System",
					Content: fmt.Sprintf("Error loading characters: %v", err),
					Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
				}
				return func() tea.Msg { return ChatMsg{Message: sysMsg} }
			}

			var foundChar *Character
			for _, charInfo := range chars {
				// We need to load full character to get the name, but ListCharacters gives name
				// so we just compare charInfo.Name
				if strings.EqualFold(charInfo.Name, characterName) {
					// Load the full character to ensure it exists and we have its details
					loadedChar, loadErr := LoadCharacter(m.user, charInfo.ID)
					if loadErr != nil {
						sysMsg := Message{
							ID: GenerateRandomID(), Sender: "System",
							Content: fmt.Sprintf("Error loading character '%s': %v", characterName, loadErr),
							Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
						}
						return func() tea.Msg { return ChatMsg{Message: sysMsg} }
					}
					foundChar = loadedChar
					break
				}
			}

			if foundChar != nil {
				m.selectedCharacterID = foundChar.Name // Use name as ID for chat display
				m.activeCharacterID = foundChar.ID     // Use actual ID for memory management etc.
				sysMsg := Message{
					ID: GenerateRandomID(), Sender: "System",
					Content: fmt.Sprintf("Character '%s' selected! Ready to chat. 💬", foundChar.Name),
					Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
				}
				// Clear existing messages, possibly add first message of character
				m.messages = []Message{}
				if foundChar.FirstMessage != "" {
					m.messages = append(m.messages, Message{
						ID: GenerateRandomID(), Sender: foundChar.Name, Content: foundChar.FirstMessage,
						Timestamp: time.Now().Unix(), Type: MessageTypeCharacter,
					})
				}
				return func() tea.Msg { return ChatMsg{Message: sysMsg} }
			} else {
				sysMsg := Message{
					ID: GenerateRandomID(), Sender: "System",
					Content: fmt.Sprintf("Character '%s' not found. Use /character to see available characters.", characterName),
					Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
				}
				return func() tea.Msg { return ChatMsg{Message: sysMsg} }
			}
		}
	case "image":
		if strings.TrimSpace(args) == "" {
			return sysMsg("Usage: /image <prompt>  (e.g. /image a mouse conducting a symphony of cats)")
		}
		m.busy = true
		svc := m.imageService
		imgModel := ImageModel(m.selectedImageModel)
		if imgModel == "" {
			imgModel = ImageModel(m.apiConfig.Pollinations.DefaultImageModel)
		}
		if imgModel == "" {
			imgModel = "flux"
		}
		prompt := args
		return tea.Batch(
			sysMsg("🎨 Generating image: "+prompt),
			func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
				defer cancel()
				data, err := svc.GenerateImage(ctx, prompt, imgModel)
				if err != nil {
					return ChatMsg{Error: fmt.Errorf("image generation failed: %w", err)}
				}
				dir := filepath.Join(Paths.DataDir, "images")
				os.MkdirAll(dir, 0755)
				path := filepath.Join(dir, fmt.Sprintf("img-%d.jpg", time.Now().Unix()))

				if strings.HasPrefix(data, "http") {
					// URL (Pollinations etc.) — download it
					if err := downloadImage(ctx, data, path); err != nil {
						return ChatMsg{Error: fmt.Errorf("image download failed: %w", err)}
					}
				} else {
					// base64 payload
					raw, err := base64.StdEncoding.DecodeString(data)
					if err != nil {
						return ChatMsg{Error: fmt.Errorf("decode image: %w", err)}
					}
					if err := os.WriteFile(path, raw, 0644); err != nil {
						return ChatMsg{Error: fmt.Errorf("save image: %w", err)}
					}
				}
				return imageGeneratedMsg{path: path, prompt: prompt}
			},
		)
	case "save":
		// Card chat: save under the card-derived memory ID
		if m.cardChatMode {
			if err := SaveChatSession(m.user, "card:"+m.cardName, m.messages); err != nil {
				return sysMsg("Save failed: " + err.Error())
			}
			return sysMsg("💾 Conversation saved.")
		}
		// Check if a character is selected
		if m.activeCharacterID == "" {
			sysMsg := Message{ID: GenerateRandomID(), Sender: "System", Content: "Please select a character first using `/character <name>` to save the chat.", Timestamp: time.Now().Unix(), Type: MessageTypeSystem}
			return func() tea.Msg { return ChatMsg{Message: sysMsg} }
		}

		// Create a temporary file to hold the chat log
		tempFile, err := os.CreateTemp("", "chatchar-*.json")
		if err != nil {
			return func() tea.Msg { return ChatMsg{Error: fmt.Errorf("failed to create temp file for chat log: %w", err)} }
		}
		defer os.Remove(tempFile.Name()) // Clean up the temp file

		// Serialize current messages to JSON
		messagesJSON, err := json.MarshalIndent(m.messages, "", "  ")
		if err != nil {
			return func() tea.Msg { return ChatMsg{Error: fmt.Errorf("failed to serialize messages for saving: %w", err)} }
		}
		if _, err := tempFile.Write(messagesJSON); err != nil {
			return func() tea.Msg { return ChatMsg{Error: fmt.Errorf("failed to write chat log to temp file: %w", err)} }
		}
		tempFile.Close() // Close the file before adding it to library

		// Add the chat log to the library
		filename := fmt.Sprintf("chat_with_%s_%s.json", m.selectedCharacterID, time.Now().Format("2006-01-02_15-04"))
		_, err = AddFileToLibrary(m.user, tempFile.Name(), filename, "chat_log")
		if err != nil {
			return func() tea.Msg { return ChatMsg{Error: fmt.Errorf("failed to add chat log to library: %w", err)} }
		}

		sysMsg := Message{
			ID:        GenerateRandomID(),
			Sender:    "System",
			Content:   fmt.Sprintf("Chat session with '%s' saved to library! 💾", m.selectedCharacterID),
			Timestamp: time.Now().Unix(),
			Type:      MessageTypeSystem,
		}
		return func() tea.Msg { return ChatMsg{Message: sysMsg} }

	case "narrator":
		if args == "" {
			sysMsg := Message{
				ID: GenerateRandomID(), Sender: "System",
				Content: "Please provide a message for the narrator. Usage: `/narrator <message>`",
				Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
			}
			return func() tea.Msg { return ChatMsg{Message: sysMsg} }
		}
		narratorMessage := Message{
			ID: GenerateRandomID(),
			Sender: "Narrator",
			Content: args,
			Timestamp: time.Now().Unix(),
			Type: MessageTypeSystem, // Narrator messages are system messages
		}
		return func() tea.Msg { return ChatMsg{Message: narratorMessage} }

	case "ai2ai":
		return m.handleAI2AICommand(args)
	case "end":
		return func() tea.Msg {
			m.messages = []Message{}
			m.selectedCharacterID = ""
			m.activeCharacterID = ""
			sysMsg := Message{
				ID:        GenerateRandomID(),
				Sender:    "System",
				Content:   "Conversation ended and context cleared. Select a new character to start.",
				Timestamp: time.Now().Unix(),
				Type:      MessageTypeSystem,
			}
			return ChatMsg{Message: sysMsg}
		}
	case "review":
		var lastSummaryIndex = -1
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].Type == MessageTypeSummary {
				lastSummaryIndex = i
				break
			}
		}

		if lastSummaryIndex != -1 {
			var reviewContent strings.Builder
			reviewContent.WriteString("--- Reviewing since last summary ---\n")
			for i := lastSummaryIndex; i < len(m.messages); i++ {
				msg := m.messages[i]
				reviewContent.WriteString(fmt.Sprintf("%s: %s\n", msg.Sender, msg.Content))
			}

			sysMsg := Message{
				ID: GenerateRandomID(), Sender: "System",
				Content: reviewContent.String(),
				Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
			}
			return func() tea.Msg { return ChatMsg{Message: sysMsg} }
		} else {
			sysMsg := Message{
				ID: GenerateRandomID(), Sender: "System",
				Content: "No summary found in current conversation. Generate one with /summarize.",
				Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
			}
			return func() tea.Msg { return ChatMsg{Message: sysMsg} }
		}
	case "system":
		if strings.TrimSpace(args) == "" {
			var currentPrompt string
			if m.systemPrompt == "" {
				currentPrompt = "No system prompt set."
			} else {
				currentPrompt = fmt.Sprintf("Current system prompt: %s", m.systemPrompt)
			}
			sysMsg := Message{
				ID: GenerateRandomID(), Sender: "System",
				Content: currentPrompt,
				Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
			}
			return func() tea.Msg { return ChatMsg{Message: sysMsg} }
		}
		m.systemPrompt = args
		sysMsg := Message{
			ID: GenerateRandomID(), Sender: "System",
			Content: fmt.Sprintf("System prompt updated: %s", args),
			Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
		}
		return func() tea.Msg { return ChatMsg{Message: sysMsg} }
	case "DM":
		parts := strings.Fields(args)
		if len(parts) < 2 {
			sysMsg := Message{
				ID: GenerateRandomID(), Sender: "System",
				Content: "Usage: /dm <character> <message>",
				Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
			}
			return func() tea.Msg { return ChatMsg{Message: sysMsg} }
		}
		charName := parts[0]
		dmContent := strings.Join(parts[1:], " ")

		charInfo, err := findCharacterInfoByName(m.user, charName)
		if err != nil {
			sysMsg := Message{
				ID: GenerateRandomID(), Sender: "System",
				Content: fmt.Sprintf("Error: Character '%s' not found.", charName),
				Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
			}
			return func() tea.Msg { return ChatMsg{Message: sysMsg} }
		}

		dmMessage := Message{
			ID:        GenerateRandomID(),
			Sender:    m.user.Username,
			Content:   dmContent,
			Timestamp: time.Now().Unix(),
			Type:      MessageTypeUser,
			Recipient: charInfo.Name,
		}
		m.messages = append(m.messages, dmMessage)
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

		// Trigger the recipient character's response
		go func() tea.Msg {
			char, err := LoadCharacter(m.user, charInfo.ID)
			if err != nil {
				return ChatMsg{Error: fmt.Errorf("failed to load DM recipient character '%s': %w", charInfo.Name, err)}
			}
			llmProviderConfig, ok := m.apiConfig.GetLLMProviderConfig(m.selectedLLMProvider)
			if !ok {
				return ChatMsg{Error: fmt.Errorf("LLM provider '%s' not configured for DM", m.selectedLLMProvider)}
			}
			// Use the chat model's selected LLMModel, or default if not set.
			usedLLMModel := LLMModel(m.selectedLLMModel)
			if usedLLMModel == "" {
				return ChatMsg{Error: fmt.Errorf("no LLM model selected for DM provider '%s'", m.selectedLLMProvider)}
			}

			// Construct LLM prompt for DM response
			var promptBuilder strings.Builder
			promptBuilder.WriteString(fmt.Sprintf("You are %s. Your goal is to roleplay as %s. You have just received a private message. Respond naturally in character.\n", char.Name, char.Name))
			promptBuilder.WriteString(fmt.Sprintf("DESCRIPTION: %s\n", char.Description))
			promptBuilder.WriteString(fmt.Sprintf("PERSONALITY: %s\n", char.Personality))
			promptBuilder.WriteString(fmt.Sprintf("SCENARIO: %s\n", char.Scenario))
			if len(char.Lorebook) > 0 {
				promptBuilder.WriteString("\n--- LOREBOOK ENTRIES ---\n")
				for key, value := range char.Lorebook {
					promptBuilder.WriteString(fmt.Sprintf("%s: %s\n", key, value))
				}
			}
			promptBuilder.WriteString(fmt.Sprintf("\nYou received a DM from %s: \"%s\"\n", m.user.Username, dmMessage.Content))
			promptBuilder.WriteString(fmt.Sprintf("Your private response as %s: ", char.Name))

			llmResponseContent, err := m.llmService.GenerateResponse(context.Background(), promptBuilder.String(), llmProviderConfig.DefaultLLMModel, *m.apiConfig)
			if err != nil {
				return ChatMsg{Error: fmt.Errorf("failed to get DM response from '%s': %w", char.Name, err)}
			}
			responseMsg := Message{
				ID:        GenerateRandomID(),
				Sender:    char.Name,
				Content:   llmResponseContent,
				Timestamp: time.Now().Unix(),
				Type:      MessageTypeCharacter,
				Recipient: m.user.Username, // Respond back to the user who sent the DM
			}
			// Add character's DM response to their own memory
			memEntry := &MemoryEntry{Content: responseMsg.Content, Type: "episodic", Source: "DM response"}
			m.memoryManager.AddMemory(m.user, char.ID, memEntry, m.apiConfig)
			return ChatMsg{Message: responseMsg}
		}()

		return nil
	case "invite":
		charName := args
		if charName == "" {
			sysMsg := Message{
				ID: GenerateRandomID(), Sender: "System",
				Content: "Usage: /invite <character>",
				Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
			}
			return func() tea.Msg { return ChatMsg{Message: sysMsg} }
		}

		charInfo, err := findCharacterInfoByName(m.user, charName)
		if err != nil {
			sysMsg := Message{
				ID: GenerateRandomID(), Sender: "System",
				Content: fmt.Sprintf("Error: Character '%s' not found.", charName),
				Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
			}
			return func() tea.Msg { return ChatMsg{Message: sysMsg} }
		}

		// Check if character is already a participant
		for _, p := range m.participants {
			if p == charInfo.Name {
				sysMsg := Message{
					ID: GenerateRandomID(), Sender: "System",
					Content: fmt.Sprintf("Character '%s' is already in the conversation.", charName),
					Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
				}
				return func() tea.Msg { return ChatMsg{Message: sysMsg} }
			}
		}

		m.participants = append(m.participants, charInfo.Name)
		sysMsg := Message{
			ID: GenerateRandomID(), Sender: "System",
			Content: fmt.Sprintf("Character '%s' has been invited to the conversation.", charInfo.Name),
			Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
		}
		return func() tea.Msg { return ChatMsg{Message: sysMsg} }
	case "upload":
		filePath := args
		if filePath == "" {
			sysMsg := Message{ID: GenerateRandomID(), Sender: "System", Content: "Please provide a file path. Usage: `/upload <path to file>`", Timestamp: time.Now().Unix(), Type: MessageTypeSystem}
			return func() tea.Msg { return ChatMsg{Message: sysMsg} }
		}

		// Add the file to the library
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			return func() tea.Msg { return ChatMsg{Error: fmt.Errorf("failed to get file info: %w", err)} }
		}

		fileType := "file" // generic file type
		// you can add more specific file type detection here if needed
		if strings.HasSuffix(strings.ToLower(fileInfo.Name()), ".json") {
			fileType = "json"
		} else if strings.HasSuffix(strings.ToLower(fileInfo.Name()), ".png") || strings.HasSuffix(strings.ToLower(fileInfo.Name()), ".jpg") {
			fileType = "image"
		}


		_, err = AddFileToLibrary(m.user, filePath, fileInfo.Name(), fileType)
		if err != nil {
			return func() tea.Msg { return ChatMsg{Error: fmt.Errorf("failed to add file to library: %w", err)} }
		}

		sysMsg := Message{
			ID:        GenerateRandomID(),
			Sender:    "System",
			Content:   fmt.Sprintf("File '%s' added to library successfully! 📚", fileInfo.Name()),
			Timestamp: time.Now().Unix(),
			Type:      MessageTypeSystem,
		}
		return func() tea.Msg { return ChatMsg{Message: sysMsg} }
	case "topic": // Placeholder for /topic command
		if args == "" {
			sysMsg := Message{
				ID: GenerateRandomID(), Sender: "System",
				Content: "Please provide a topic. Usage: `/topic <message>`",
				Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
			}
			return func() tea.Msg { return ChatMsg{Message: sysMsg} }
		}
		m.systemPrompt = fmt.Sprintf("The current topic of conversation is: %s", args)
		sysMsg := Message{
			ID: GenerateRandomID(), Sender: "System",
			Content: fmt.Sprintf("Topic set to: %s", args),
			Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
		}
		return func() tea.Msg { return ChatMsg{Message: sysMsg} }
	case "boot": // Placeholder for /boot command
		charName := args
		if charName == "" {
			sysMsg := Message{
				ID: GenerateRandomID(), Sender: "System",
				Content: "Usage: /boot <character>",
				Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
			}
			return func() tea.Msg { return ChatMsg{Message: sysMsg} }
		}

		var newParticipants []string
		found := false
		for _, p := range m.participants {
			if p == charName {
				found = true
				continue
			}
			newParticipants = append(newParticipants, p)
		}

		if !found {
			sysMsg := Message{
				ID: GenerateRandomID(), Sender: "System",
				Content: fmt.Sprintf("Character '%s' is not in the conversation.", charName),
				Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
			}
			return func() tea.Msg { return ChatMsg{Message: sysMsg} }
		}

		m.participants = newParticipants
		sysMsg := Message{
			ID: GenerateRandomID(), Sender: "System",
			Content: fmt.Sprintf("Character '%s' has been removed from the conversation.", charName),
			Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
		}
		return func() tea.Msg { return ChatMsg{Message: sysMsg} }
	case "lore": // Placeholder for /lore command
		if m.activeCharacterID == "" {
			sysMsg := Message{
				ID: GenerateRandomID(), Sender: "System",
				Content: "Please select a character first using `/character <name>` to add a lore entry.",
				Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
			}
			return func() tea.Msg { return ChatMsg{Message: sysMsg} }
		}
		if args == "" {
			sysMsg := Message{
				ID: GenerateRandomID(), Sender: "System",
				Content: "Please provide a lore entry. Usage: `/lore <key>=<value>`",
				Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
			}
			return func() tea.Msg { return ChatMsg{Message: sysMsg} }
		}

		parts := strings.SplitN(args, "=", 2)
		if len(parts) != 2 {
			sysMsg := Message{
				ID: GenerateRandomID(), Sender: "System",
				Content: "Invalid lore entry format. Usage: `/lore <key>=<value>`",
				Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
			}
			return func() tea.Msg { return ChatMsg{Message: sysMsg} }
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		char, err := LoadCharacter(m.user, m.activeCharacterID)
		if err != nil {
			sysMsg := Message{
				ID: GenerateRandomID(), Sender: "System",
				Content: fmt.Sprintf("Error loading character '%s': %v", m.selectedCharacterID, err),
				Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
			}
			return func() tea.Msg { return ChatMsg{Message: sysMsg} }
		}

		if char.Lorebook == nil {
			char.Lorebook = make(map[string]string)
		}
		char.Lorebook[key] = value

		if err := SaveCharacter(m.user, char); err != nil {
			sysMsg := Message{
				ID: GenerateRandomID(), Sender: "System",
				Content: fmt.Sprintf("Error saving character '%s': %v", m.selectedCharacterID, err),
				Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
			}
			return func() tea.Msg { return ChatMsg{Message: sysMsg} }
		}

		sysMsg := Message{
			ID: GenerateRandomID(), Sender: "System",
			Content: fmt.Sprintf("Lore entry added to '%s': %s = %s", m.selectedCharacterID, key, value),
			Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
		}
		return func() tea.Msg { return ChatMsg{Message: sysMsg} }
	case "special": // Placeholder for /special note command
		if !strings.HasPrefix(args, "note ") {
			sysMsg := Message{
				ID: GenerateRandomID(), Sender: "System",
				Content: "Invalid command. Usage: `/special note <message>`",
				Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
			}
			return func() tea.Msg { return ChatMsg{Message: sysMsg} }
		}
		note := strings.TrimPrefix(args, "note ")

		if m.activeCharacterID == "" {
			sysMsg := Message{
				ID: GenerateRandomID(), Sender: "System",
				Content: "Please select a character first using `/character <name>` to add a special note.",
				Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
			}
			return func() tea.Msg { return ChatMsg{Message: sysMsg} }
		}

		memEntry := &MemoryEntry{
			Content: note,
			Type:    "special",
			Source:  fmt.Sprintf("Special note from user for %s", m.selectedCharacterID),
		}
		if err := m.memoryManager.AddMemory(m.user, m.activeCharacterID, memEntry, m.apiConfig); err != nil {
			sysMsg := Message{
				ID: GenerateRandomID(), Sender: "System",
				Content: fmt.Sprintf("Error adding special note: %v", err),
				Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
			}
			return func() tea.Msg { return ChatMsg{Message: sysMsg} }
		}

		sysMsg := Message{
			ID: GenerateRandomID(), Sender: "System",
			Content: fmt.Sprintf("Special note added to '%s'.", m.selectedCharacterID),
			Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
		}
		return func() tea.Msg { return ChatMsg{Message: sysMsg} }
	case "quit":
		// Clear chat context before quitting to main menu
		return func() tea.Msg {
			m.messages = []Message{}
			m.selectedCharacterID = ""
			m.activeCharacterID = ""
			return nil
		}
	default:
		sysMsg := Message{
			ID:        GenerateRandomID(),
			Sender:    "System",
			Content:   fmt.Sprintf("Unknown command: /%s. Type /help for a list of commands.", command),
			Timestamp: time.Now().Unix(),
			Type:      MessageTypeSystem, // Set message type
		}
		return func() tea.Msg { return ChatMsg{Message: sysMsg} }
	}
}

func (m *chatModel) renderProviderSelection() string {
	var s strings.Builder
	if m.state == selectingLLMProvider {
		s.WriteString("Select an LLM Provider 🧠:\n\n")
	} else if m.state == selectingImageProvider {
		s.WriteString("Select an Image Provider 🖼️:\n\n")
	} else if m.state == selectingLLMModel {
		s.WriteString(fmt.Sprintf("Select a Model for %s 🧠:\n\n", m.selectedLLMProvider))
	} else if m.state == selectingImageModel {
		s.WriteString(fmt.Sprintf("Select a Model for %s 🖼️:\n\n", m.selectedImageProvider))
	} else {
		s.WriteString("Select a Provider:\n\n")
	}

	for i, choice := range m.providerChoices {
		prefix := fmt.Sprintf("%d. ", i)
		if m.providerCursor == i {
			s.WriteString(chatSelectedItemStyle.Render(prefix + "👉 " + choice) + "\n")
		} else {
			s.WriteString(chatItemStyle.Render(prefix + "  " + choice) + "\n")
		}
	}
	s.WriteString(chatHelpStyle.Render(fmt.Sprintf("\nPress 'enter' to select, 'esc' to cancel.")))
	return chatBoxStyle.Render(s.String())
}

// providersToChoices converts a slice of Providers or Models to a slice of strings for TUI display.
func providersToChoices[T ~string](items []T) []string {
	choices := make([]string, len(items))
	for i, item := range items {
		choices[i] = string(item)
	}
	return choices
}

// GetLLMProviders returns a list of supported LLM Providers.
func GetLLMProviders() []Provider {
	return []Provider{
		ProviderMock,
		ProviderMistral,
		ProviderGroq,
		ProviderHuggingFace,
		ProviderOpenAI,
		ProviderClaude,
		ProviderCustomLLM,
	}
}

// GetImageProviders returns a list of supported Image Providers.
func GetImageProviders() []Provider {
	return []Provider{
		ProviderMock,
		ProviderAIHorde,
		ProviderPollinations,
		ProviderImageRouter,
		ProviderHuggingFace,
		ProviderOpenAI, // For DALL-E, etc.
		ProviderCustomImage,
	}
}



// handleAI2AICommand handles the AI2AI conversation initiation.
func (m *chatModel) handleAI2AICommand(args string) tea.Cmd {
	parts := strings.Fields(args)
	if len(parts) < 3 {
		sysMsg := Message{
			ID: GenerateRandomID(), Sender: "System",
			Content: "Usage: /ai2ai <Character1 Name> <Character2 Name> <topic>",
			Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
		}
		return func() tea.Msg { return ChatMsg{Message: sysMsg} }
	}

	char1Name := parts[0]
	char2Name := parts[1]
	topic := strings.Join(parts[2:], " ")

	// Load Character 1
	char1Info, err := findCharacterInfoByName(m.user, char1Name)
	if err != nil {
		sysMsg := Message{
			ID: GenerateRandomID(), Sender: "System",
			Content: fmt.Sprintf("Error: Character '%s' not found. %v", char1Name, err),
			Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
		}
		return func() tea.Msg { return ChatMsg{Message: sysMsg} }
	}
	char1, err := LoadCharacter(m.user, char1Info.ID)
	if err != nil {
		sysMsg := Message{
			ID: GenerateRandomID(), Sender: "System",
			Content: fmt.Sprintf("Error loading character '%s': %v", char1Name, err),
			Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
		}
		return func() tea.Msg { return ChatMsg{Message: sysMsg} }
	}

	// Load Character 2
	char2Info, err := findCharacterInfoByName(m.user, char2Name)
	if err != nil {
		sysMsg := Message{
			ID: GenerateRandomID(), Sender: "System",
			Content: fmt.Sprintf("Error: Character '%s' not found. %v", char2Name, err),
			Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
		}
		return func() tea.Msg { return ChatMsg{Message: sysMsg} }
	}
	char2, err := LoadCharacter(m.user, char2Info.ID)
	if err != nil {
		sysMsg := Message{
			ID: GenerateRandomID(), Sender: "System",
			Content: fmt.Sprintf("Error loading character '%s': %v", char2Name, err),
			Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
		}
		return func() tea.Msg { return ChatMsg{Message: sysMsg} }
	}

	sysMsg := Message{
		ID: GenerateRandomID(), Sender: "System",
		Content: fmt.Sprintf("Initiating AI-to-AI conversation between '%s' and '%s' on the topic of '%s'...", char1Name, char2Name, topic),
		Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
	}

	return func() tea.Msg {
		msgs := startAI2AIConversation(m.user, char1, char2, topic, m.memoryManager, m.llmService, m.apiConfig)
		return AI2AICompleteMsg{Intro: sysMsg, Messages: msgs}
	}
}


// startAI2AIConversation simulates an AI-to-AI conversation for a fixed number of turns.
func startAI2AIConversation(user *User, char1, char2 *Character, topic string, mm MemoryManager, llmService LLMService, apiConfig *APIConfig) []Message {
	const maxTurns = 5
	var chatHistory []Message
	var result []Message
	lastMessageContent := fmt.Sprintf("Hello %s, let's talk about %s.", char2.Name, topic)


	for turn := 0; turn < maxTurns*2; turn++ {
		var activeChar, otherChar *Character
		if turn%2 == 0 {
			activeChar = char1
			otherChar = char2
		} else {
			activeChar = char2
			otherChar = char1
		}

		// Build prompt
		var promptBuilder strings.Builder
		promptBuilder.WriteString(fmt.Sprintf("You are %s. Roleplay as %s. Always respond in character.\n", activeChar.Name, activeChar.Name))
		promptBuilder.WriteString(fmt.Sprintf("DESCRIPTION: %s\nPERSONALITY: %s\n", activeChar.Description, activeChar.Personality))
		promptBuilder.WriteString(fmt.Sprintf("You are talking with %s about: %s\n", otherChar.Name, topic))

		retrievedMemories, _ := mm.RetrieveMemories(user, activeChar.ID, lastMessageContent, 3, apiConfig)
		if len(retrievedMemories) > 0 {
			promptBuilder.WriteString("\n--- Memories ---\n")
			for _, mem := range retrievedMemories {
				promptBuilder.WriteString("- " + mem.Content + "\n")
			}
		}
		promptBuilder.WriteString("\n--- Conversation ---\n")
		for _, msg := range chatHistory {
			promptBuilder.WriteString(fmt.Sprintf("%s: %s\n", msg.Sender, msg.Content))
		}
		promptBuilder.WriteString(fmt.Sprintf("\n%s said: \"%s\"\n%s: ", otherChar.Name, lastMessageContent, activeChar.Name))

		// Use the shared LLM service — whatever provider is active
		model := LLMModel(apiConfig.SelectedLLMProvider)
		responseContent, err := llmService.GenerateResponse(context.Background(), promptBuilder.String(), model, *apiConfig)
		if err != nil {
			errMsg := Message{ID: GenerateRandomID(), Sender: "System",
				Content: fmt.Sprintf("AI-to-AI error for %s: %v", activeChar.Name, err),
				Timestamp: time.Now().Unix(), Type: MessageTypeSystem}
			result = append(result, errMsg)
			break
		}
		lastMessageContent = responseContent

		aiMessage := Message{
			ID: GenerateRandomID(), Sender: activeChar.Name,
			Content: responseContent, Timestamp: time.Now().Unix(),
			Type: MessageTypeCharacter,
		}
		chatHistory = append(chatHistory, aiMessage)
		result = append(result, aiMessage)

		memEntry := &MemoryEntry{
			Content:  responseContent,
			Type:     "episodic",
			Source:   fmt.Sprintf("AI-to-AI with %s", otherChar.Name),
			Keywords: []string{otherChar.Name, topic},
		}
		mm.AddMemory(user, activeChar.ID, memEntry, apiConfig)
	}

	result = append(result, Message{
		ID: GenerateRandomID(), Sender: "System",
		Content:   fmt.Sprintf("AI-to-AI conversation between %s and %s concluded.", char1.Name, char2.Name),
		Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
	})
	return result
}

// findCharacterInfoByName is a helper to find a CharacterInfo by name.
func findCharacterInfoByName(user *User, name string) (*CharacterInfo, error) {
	chars, err := ListCharacters(user)
	if err != nil {
		return nil, err
	}
	for _, charInfo := range chars {
		if strings.EqualFold(charInfo.Name, name) {
			return &charInfo, nil
		}
	}
	return nil, fmt.Errorf("character '%s' not found", name)
}


// Message to explicitly set the active character for chat.
type SetChatCharacterMsg struct {
	CharacterID string
}