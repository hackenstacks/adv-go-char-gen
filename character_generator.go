package main

import (
	"log"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Character Generator Styles
var (
	charGenTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF69B4")). // Hot Pink
			Padding(1, 4).
			Align(lipgloss.Center).
			Bold(true)

	charGenItemStyle = lipgloss.NewStyle().
			PaddingLeft(2).
			Foreground(lipgloss.Color("#FFB6C1")) // Light Pink

	charGenSelectedItemStyle = lipgloss.NewStyle().
					PaddingLeft(2).
					Foreground(lipgloss.Color("#FF1493")). // Deep Pink
					Border(lipgloss.RoundedBorder(), false, false, false, true).
					BorderForeground(lipgloss.Color("#FF1493"))

	charGenPromptStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#FFB6C1"))

	charGenInputTextStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#FFD700")). // Gold
					BorderBottom(true).
					BorderBottomForeground(lipgloss.Color("#FFD700"))

	charGenErrorStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#FF0000")). // Red
					Bold(true)

	charGenHelpStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#757575")). // Grey
					PaddingTop(1)

	charGenBoxStyle = lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(lipgloss.Color("#FF69B4")). // Hot Pink
					Padding(1, 2).
					Width(80)
)

type charGenState int

const (
	charGenListView charGenState = iota
	charGenCreateView
	charGenEditView
)

// characterItem represents a single character in the list.
type characterItem struct {
	characterID string
	name        string
	description string
}

func (i characterItem) FilterValue() string { return i.name }
func (i characterItem) Title() string       { return i.name }
func (i characterItem) Description() string  { return i.description }

// LLMCharacterMsg is sent when the LLM successfully generates character data.
type LLMCharacterMsg struct {
	Name string
	Description string
	Personality string
	Scenario string
	FirstMessage string
	Err error
}

// GenerateCharacterCmd is a command that initiates the LLM character generation.
func GenerateCharacterCmd(user *User, apiConfig APIConfig, llmPrompt string) tea.Cmd {
	return func() tea.Msg {
		if user == nil || user.EncryptionKey == nil {
			return LLMCharacterMsg{Err: fmt.Errorf("user not logged in or encryption key not available")}
		}

		llmService, err := LLMFactory(apiConfig.SelectedLLMProvider, apiConfig)
		if err != nil {
			return LLMCharacterMsg{Err: fmt.Errorf("failed to create LLM service: %w", err)}
		}

		// The prompt for the LLM to generate character details
		// We ask for JSON output to make parsing easier.
		fullPrompt := fmt.Sprintf(`You are a creative character generator. Generate a character with the following details in JSON format:
{
  "name": "",
  "description": "",
  "personality": "",
  "scenario": "",
  "firstMessage": ""
}
The character should be %s.`, llmPrompt)

		// Get the default LLM model for the selected provider
		commonConfig, ok := apiConfig.GetLLMProviderConfig(apiConfig.SelectedLLMProvider)
		if !ok {
			return LLMCharacterMsg{Err: fmt.Errorf("could not get config for selected LLM provider: %s", apiConfig.SelectedLLMProvider)}
		}
		model := commonConfig.DefaultLLMModel
		if model == "" {
			// Fallback to a default if not specified
			switch apiConfig.SelectedLLMProvider {
			case ProviderMistral:
				model = "mistral-tiny" // A common small model for Mistral
			case ProviderMock:
				model = "mock-llm-fast"
			default:
				return LLMCharacterMsg{Err: fmt.Errorf("no default LLM model specified for provider: %s", apiConfig.SelectedLLMProvider)}
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second) // 2-minute timeout
		defer cancel()

		rawResponse, err := llmService.GenerateResponse(ctx, fullPrompt, model, apiConfig)
		if err != nil {
			return LLMCharacterMsg{Err: fmt.Errorf("LLM generation failed: %w", err)}
		}

		// Parse the JSON response from the LLM
		var generatedChar Character
		err = json.Unmarshal([]byte(rawResponse), &generatedChar)
		if err != nil {
			// Attempt to clean the response if it's wrapped in markdown
			if strings.HasPrefix(rawResponse, "```json") && strings.HasSuffix(rawResponse, "```") {
				cleanedResponse := strings.TrimPrefix(rawResponse, "```json")
				cleanedResponse = strings.TrimSuffix(cleanedResponse, "```")
				err = json.Unmarshal([]byte(cleanedResponse), &generatedChar)
			}
			if err != nil {
				return LLMCharacterMsg{Err: fmt.Errorf("failed to parse LLM response: %w\nRaw response: %s", err, rawResponse)}
			}
		}

		return LLMCharacterMsg{
			Name: generatedChar.Name,
			Description: generatedChar.Description,
			Personality: generatedChar.Personality,
			Scenario: generatedChar.Scenario,
			FirstMessage: generatedChar.FirstMessage,
		}
	}
}

type characterGeneratorModel struct {
	user         *User
	state        charGenState
	characterList list.Model
	selectedChar *Character
	// Input fields for creating/editing character
	nameInput        textinput.Model
	descriptionInput textarea.Model
	personalityInput textarea.Model
	scenarioInput    textarea.Model
	firstMessageInput textarea.Model
	focusedInput int // 0: name, 1: desc, 2: personality, 3: scenario, 4: first message
	err          error
	saveSuccess  bool
	generating   bool // New field to indicate LLM generation in progress
}

func initialCharacterGeneratorModel(u *User) characterGeneratorModel {
	// Initialize character list
	characters, err := ListCharacters(u)
	if err != nil {
		log.Printf("Error loading characters: %v\n", err)
	}
	items := make([]list.Item, len(characters))
	for i, charInfo := range characters {
		items[i] = characterItem{
			characterID: charInfo.ID,
			name:        charInfo.Name,
			description: charInfo.Name, // Description is not available in CharacterInfo directly
		}
	}

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = charGenSelectedItemStyle
	delegate.Styles.SelectedDesc = charGenSelectedItemStyle.Copy().Faint(true)
	delegate.Styles.NormalTitle = charGenItemStyle
	delegate.Styles.NormalDesc = charGenItemStyle.Copy().Faint(true)

	l := list.New(items, delegate, 0, 0)
	l.Title = "Your Characters 🎭"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = charGenTitleStyle
	l.SetShowTitle(true) // Ensure title is always shown

	// Initialize text inputs
	nameInput := textinput.New()
	nameInput.Placeholder = "Character Name"
	nameInput.CharLimit = 50
	nameInput.Width = 40
	nameInput.PromptStyle = charGenPromptStyle
	nameInput.TextStyle = charGenInputTextStyle

	descriptionInput := textarea.New()
	descriptionInput.Placeholder = "Description (e.g., A valiant knight, a mischievous mage)"
	descriptionInput.SetWidth(60)
	descriptionInput.SetHeight(5)
	descriptionInput.Prompt = "" // No prompt for textarea
	descriptionInput.FocusedStyle.CursorLine = lipgloss.NewStyle()
	descriptionInput.ShowLineNumbers = false
	descriptionInput.KeyMap.DeleteWordBackward.Unbind()
	descriptionInput.KeyMap.LineStart.Unbind()
	descriptionInput.KeyMap.LineEnd.Unbind()
	// descriptionInput.KeyMap.DeleteBackward.Unbind()

	personalityInput := textarea.New()
	personalityInput.Placeholder = "Personality (e.g., Brave, loyal, sarcastic, kind)"
	personalityInput.SetWidth(60)
	personalityInput.SetHeight(5)
	personalityInput.Prompt = "" // No prompt for textarea
	personalityInput.FocusedStyle.CursorLine = lipgloss.NewStyle()
	personalityInput.ShowLineNumbers = false
	personalityInput.KeyMap.DeleteWordBackward.Unbind()
	personalityInput.KeyMap.LineStart.Unbind()
	personalityInput.KeyMap.LineEnd.Unbind()
	// personalityInput.KeyMap.DeleteBackward.Unbind()

	scenarioInput := textarea.New()
	scenarioInput.Placeholder = "Scenario (e.g., You are on a quest to defeat a dragon)"
	scenarioInput.SetWidth(60)
	scenarioInput.SetHeight(5)
	scenarioInput.Prompt = "" // No prompt for textarea
	scenarioInput.FocusedStyle.CursorLine = lipgloss.NewStyle()
	scenarioInput.ShowLineNumbers = false
	scenarioInput.KeyMap.DeleteWordBackward.Unbind()
	scenarioInput.KeyMap.LineStart.Unbind()
	scenarioInput.KeyMap.LineEnd.Unbind()
	// scenarioInput.KeyMap.DeleteBackward.Unbind()

	firstMessageInput := textarea.New()
	firstMessageInput.Placeholder = "First message (optional, what the character says to start a conversation)"
	firstMessageInput.SetWidth(60)
	firstMessageInput.SetHeight(3)
	firstMessageInput.Prompt = "" // No prompt for textarea
	firstMessageInput.FocusedStyle.CursorLine = lipgloss.NewStyle()
	firstMessageInput.ShowLineNumbers = false
	firstMessageInput.KeyMap.DeleteWordBackward.Unbind()
	firstMessageInput.KeyMap.LineStart.Unbind()
	firstMessageInput.KeyMap.LineEnd.Unbind()
	// firstMessageInput.KeyMap.DeleteBackward.Unbind()


	return characterGeneratorModel{
		user:          u,
		state:         charGenListView,
		characterList: l,
		nameInput:     nameInput,
		descriptionInput: descriptionInput,
		personalityInput: personalityInput,
		scenarioInput: scenarioInput,
		firstMessageInput: firstMessageInput,
		focusedInput: 0,
	}
}

func (m characterGeneratorModel) Init() tea.Cmd {
	return nil
}

func (m characterGeneratorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := charGenBoxStyle.GetHorizontalFrameSize(), charGenBoxStyle.GetVerticalFrameSize()
		m.characterList.SetSize(msg.Width-h, msg.Height-v)
		// Adjust text area widths as well
		m.nameInput.Width = msg.Width - h - 10
		m.descriptionInput.SetWidth(msg.Width - h - 10)
		m.personalityInput.SetWidth(msg.Width - h - 10)
		m.scenarioInput.SetWidth(msg.Width - h - 10)
		m.firstMessageInput.SetWidth(msg.Width - h - 10)


	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			m.err = nil // Clear error on escape
			m.saveSuccess = false // Clear success message
			if m.state == charGenListView {
				return m, func() tea.Msg { return BackToMainAppMsg{} }
			}
			m.state = charGenListView
			m.characterList.SetItems(m.loadCharacterItems()) // Reload list
			m.resetInputs()
			return m, nil

		case "tab", "shift+tab":
			if m.state == charGenCreateView || m.state == charGenEditView {
				s := msg.String()
				if s == "tab" {
					m.focusedInput = (m.focusedInput + 1) % 5 // Cycle through 5 inputs
				} else { // shift+tab
					m.focusedInput = (m.focusedInput - 1 + 5) % 5
				}
				m.updateInputFocus()
				return m, nil
			}

		case "alt+g": // Trigger LLM generation
			if m.state == charGenCreateView || m.state == charGenEditView {
				// Load API config to get selected LLM provider and API key
				apiConfig, err := LoadAPIConfig(m.user)
				if err != nil {
					m.err = fmt.Errorf("failed to load API configuration: %w", err)
					return m, nil
				}
				if apiConfig.SelectedLLMProvider == ProviderMock {
					m.err = fmt.Errorf("please select a real LLM provider in API settings before generating")
					return m, nil
				}
				m.generating = true
				m.err = nil // Clear previous errors
				// Use current input for prompt if available, otherwise a generic one
				llmPrompt := m.nameInput.Value()
				if llmPrompt == "" {
					llmPrompt = "a generic fantasy character"
				}
				return m, GenerateCharacterCmd(m.user, apiConfig, llmPrompt)
			}
		case "enter":
			switch m.state {
			case charGenListView:
				selectedItem := m.characterList.SelectedItem()
				if selectedItem == nil { // Handle case where no item is selected
					m.err = fmt.Errorf("no character selected")
					return m, nil
				}
				selected := selectedItem.(characterItem)
				loadedChar, err := LoadCharacter(m.user, selected.characterID)
				if err != nil {
					m.err = fmt.Errorf("failed to load character for editing: %w", err)
					return m, nil
				}
				m.selectedChar = loadedChar
				m.fillInputsWithCharacter(loadedChar)
				m.state = charGenEditView
				m.nameInput.Focus()
				m.updateInputFocus()
				return m, nil
			case charGenCreateView, charGenEditView:
				if m.focusedInput < 4 { // If not on the last input, move focus
					m.focusedInput = (m.focusedInput + 1) % 5
					m.updateInputFocus()
					return m, nil
				}

				// Otherwise, save character
				m.err = nil
				m.saveSuccess = false
				char := m.selectedChar
				if char == nil {
					char = NewCharacter()
				}
				char.Name = m.nameInput.Value()
				char.Description = m.descriptionInput.Value()
				char.Personality = m.personalityInput.Value()
				char.Scenario = m.scenarioInput.Value()
				char.FirstMessage = m.firstMessageInput.Value()

				if char.Name == "" {
					m.err = fmt.Errorf("character name cannot be empty")
					return m, nil
				}

				if err := SaveCharacter(m.user, char); err != nil {
					m.err = fmt.Errorf("failed to save character: %w", err)
				} else {
					m.saveSuccess = true
					m.state = charGenListView // Go back to list view
					m.characterList.SetItems(m.loadCharacterItems()) // Reload list
					m.resetInputs()
				}
				return m, nil
			}
		case "n": // New character
			if m.state == charGenListView {
				m.state = charGenCreateView
				m.resetInputs()
				m.nameInput.Focus()
				m.updateInputFocus()
				return m, nil
			}
		case "d": // Delete character
			if m.state == charGenListView {
				selectedItem := m.characterList.SelectedItem()
				if selectedItem == nil { // Handle case where no item is selected
					m.err = fmt.Errorf("no character selected to delete")
					return m, nil
				}
				selected := selectedItem.(characterItem)
				if selected.characterID != "" {
					if err := DeleteCharacter(m.user, selected.characterID); err != nil {
						m.err = fmt.Errorf("failed to delete character: %w", err)
					} else {
						m.saveSuccess = true // Reuse saveSuccess for deletion confirmation
						m.characterList.SetItems(m.loadCharacterItems()) // Reload list
					}
				}
				return m, nil
			}
		}
	case LLMCharacterMsg:
		m.generating = false
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		// Populate input fields with LLM generated data
		m.nameInput.SetValue(msg.Name)
		m.descriptionInput.SetValue(msg.Description)
		m.personalityInput.SetValue(msg.Personality)
		m.scenarioInput.SetValue(msg.Scenario)
		m.firstMessageInput.SetValue(msg.FirstMessage)
		m.err = nil
		return m, nil
	}

	// Delegate updates to appropriate sub-components
	switch m.state {
	case charGenListView:
		m.characterList, cmd = m.characterList.Update(msg)
		cmds = append(cmds, cmd)
	case charGenCreateView, charGenEditView:
		switch m.focusedInput {
		case 0:
			m.nameInput, cmd = m.nameInput.Update(msg)
		case 1:
			m.descriptionInput, cmd = m.descriptionInput.Update(msg)
		case 2:
			m.personalityInput, cmd = m.personalityInput.Update(msg)
		case 3:
			m.scenarioInput, cmd = m.scenarioInput.Update(msg)
		case 4:
			m.firstMessageInput, cmd = m.firstMessageInput.Update(msg)
		}
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m characterGeneratorModel) View() string {
	var s string
	var help string
	var status string

	if m.err != nil {
		status = charGenErrorStyle.Render(fmt.Sprintf("⛔ Error: %v", m.err)) + "\n"
	} else if m.saveSuccess {
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render("✅ Operation successful!") + "\n"
	}

	switch m.state {
	case charGenListView:
		s = m.characterList.View()
		
		// Show helpful message when character list is empty
		if len(m.characterList.Items()) == 0 {
			helpMessage := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF69B4")).
				Bold(true).
				Padding(1, 0).
				Render("🎭 No characters yet! Press 'n' to create your first character")
			s += "\n\n" + helpMessage
		}
		
		help = "\n📝 n: new character  👆/👇: move  ✏️ enter: view/edit  🗑️ d: delete  🔙 esc: back  🚪 q: quit"
	case charGenCreateView, charGenEditView:
		title := "Create New Character ✍️"
		if m.state == charGenEditView {
			title = fmt.Sprintf("Edit Character: %s 📝", m.nameInput.Value())
		}
		s = charGenTitleStyle.Render(title) + "\n\n" +
			charGenPromptStyle.Render("Name:") + "\n" +
			m.nameInput.View() + "\n\n" +
			charGenPromptStyle.Render("Description:") + "\n" +
			m.descriptionInput.View() + "\n\n" +
			charGenPromptStyle.Render("Personality:") + "\n" +
			m.personalityInput.View() + "\n\n" +
			charGenPromptStyle.Render("Scenario:") + "\n" +
			m.scenarioInput.View() + "\n\n" +
			charGenPromptStyle.Render("First Message:") + "\n" +
			m.firstMessageInput.View() + "\n\n"
		help = "tab/shift+tab cycle fields • enter to next field/save • alt+g generate • esc back • q quit"

		if m.generating {
			s += lipgloss.NewStyle().Foreground(lipgloss.Color("#A020F0")).Render("Generating character... please wait 🧠") + "\n"
		}
	}

	return charGenBoxStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left,
			s,
			status,
			charGenHelpStyle.Render(help),
		),
	)
}

// Helper to load characters for the list.
func (m *characterGeneratorModel) loadCharacterItems() []list.Item {
	characters, err := ListCharacters(m.user)
	if err != nil {
		m.err = fmt.Errorf("error loading characters for list: %w", err)
		return []list.Item{}
	}
	
	// If no characters exist, return empty list (UI will show instructions)
	if len(characters) == 0 {
		return make([]list.Item, 0)
	}
	
	items := make([]list.Item, len(characters))
	for i, charInfo := range characters {
		items[i] = characterItem{
			characterID: charInfo.ID,
			name:        charInfo.Name,
			description: charInfo.Name, // Temp, full description not in info struct
		}
	}
	return items
}

// Helper to reset input fields.
func (m *characterGeneratorModel) resetInputs() {
	m.nameInput.SetValue("")
	m.descriptionInput.SetValue("")
	m.personalityInput.SetValue("")
	m.scenarioInput.SetValue("")
	m.firstMessageInput.SetValue("")
	m.selectedChar = nil
	m.focusedInput = 0
}

// Helper to fill input fields with character data.
func (m *characterGeneratorModel) fillInputsWithCharacter(char *Character) {
	m.nameInput.SetValue(char.Name)
	m.descriptionInput.SetValue(char.Description)
	m.personalityInput.SetValue(char.Personality)
	m.scenarioInput.SetValue(char.Scenario)
	m.firstMessageInput.SetValue(char.FirstMessage)
}

// Helper to update input focus styles.
func (m *characterGeneratorModel) updateInputFocus() {
	m.nameInput.Blur()
	m.descriptionInput.Blur()
	m.personalityInput.Blur()
	m.scenarioInput.Blur()
	m.firstMessageInput.Blur()

	m.nameInput.PromptStyle = charGenPromptStyle
	// m.descriptionInput.PromptStyle = charGenPromptStyle
	// m.personalityInput.PromptStyle = charGenPromptStyle
	// m.scenarioInput.PromptStyle = charGenPromptStyle
	// m.firstMessageInput.PromptStyle = charGenPromptStyle

	m.nameInput.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#696969"))
	/* m.descriptionInput.SetStyles(textarea.Style{
		Focused: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#FFD700")),
		Blurred: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#696969")),
	})
	m.personalityInput.SetStyles(textarea.Style{
		Focused: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#FFD700")),
		Blurred: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#696969")),
	})
	m.scenarioInput.SetStyles(textarea.Style{
		Focused: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#FFD700")),
		Blurred: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#696969")),
	})
	m.firstMessageInput.SetStyles(textarea.Style{
		Focused: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#FFD700")),
		Blurred: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#696969")),
	}) */

	switch m.focusedInput {
	case 0:
		m.nameInput.Focus()
		m.nameInput.PromptStyle = charGenSelectedItemStyle
		m.nameInput.TextStyle = charGenInputTextStyle
	case 1:
		m.descriptionInput.Focus()
		m.descriptionInput.FocusedStyle.Prompt = charGenSelectedItemStyle
	case 2:
		m.personalityInput.Focus()
		m.personalityInput.FocusedStyle.Prompt = charGenSelectedItemStyle
	case 3:
		m.scenarioInput.Focus()
		m.scenarioInput.FocusedStyle.Prompt = charGenSelectedItemStyle
	case 4:
		m.firstMessageInput.Focus()
		m.firstMessageInput.FocusedStyle.Prompt = charGenSelectedItemStyle
	}
}