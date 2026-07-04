package main

import (
	"fmt"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// AppModel is the top-level model that manages different application states
type AppModel struct {
	state                   AppState
	loginModel              loginModel
	createAccountModel      createAccountModel
	mainAppModel            mainAppModel
	apiSettingsModel        apiSettingsModel
	characterGeneratorModel characterGeneratorModel
	libraryModel            libraryModel
	cardBrowserModel        cardBrowserModel
	cardEditorModel         cardEditorModel
	chatModel               chatModel
	user                    *User
	width                   int
	height                  int
}

func initialAppModel() AppModel {
	// Start with login screen for proper end-to-end flow
	return AppModel{
		state:           LoginState,
		loginModel:      initialLoginModel(),
		createAccountModel: initialCreateAccountModel(),
	}
}

// Init initializes the top-level application model.
func (m AppModel) Init() tea.Cmd {
	return m.mainAppModel.Init()
}

// Update handles messages and updates the application state.
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Propagate window size to the currently active sub-model
		switch m.state {
		case LoginState:
			newModel, newCmd := m.loginModel.Update(msg)
			m.loginModel = newModel.(loginModel)
			cmds = append(cmds, newCmd)
		case CreateAccountState:
			newModel, newCmd := m.createAccountModel.Update(msg)
			m.createAccountModel = newModel.(createAccountModel)
			cmds = append(cmds, newCmd)
		case MainAppState:
			newModel, newCmd := m.mainAppModel.Update(msg)
			m.mainAppModel = newModel.(mainAppModel)
			cmds = append(cmds, newCmd)
		case ApiSettingsState:
			newModel, newCmd := m.apiSettingsModel.Update(msg)
			m.apiSettingsModel = newModel.(apiSettingsModel)
			cmds = append(cmds, newCmd)
		case CharGenState:
			newModel, newCmd := m.characterGeneratorModel.Update(msg)
			m.characterGeneratorModel = newModel.(characterGeneratorModel)
			cmds = append(cmds, newCmd)
		case LibraryState:
			newModel, newCmd := m.libraryModel.Update(msg)
			m.libraryModel = newModel.(libraryModel)
			cmds = append(cmds, newCmd)
		case CardBrowserState:
			newModel, newCmd := m.cardBrowserModel.Update(msg)
			m.cardBrowserModel = newModel.(cardBrowserModel)
			cmds = append(cmds, newCmd)
		case CardEditorState:
			newModel, newCmd := m.cardEditorModel.Update(msg)
			m.cardEditorModel = newModel.(cardEditorModel)
			cmds = append(cmds, newCmd)
		case ChatState:
			newModel, newCmd := m.chatModel.Update(msg)
			m.chatModel = newModel.(chatModel)
			cmds = append(cmds, newCmd)
		}

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// For other keyboard messages, delegate to the appropriate sub-model
		switch m.state {
		case LoginState:
			newLoginModel, newCmd := m.loginModel.Update(msg)
			m.loginModel = newLoginModel.(loginModel)
			cmds = append(cmds, newCmd)
		case CreateAccountState:
			newCreateAccountModel, newCmd := m.createAccountModel.Update(msg)
			m.createAccountModel = newCreateAccountModel.(createAccountModel)
			cmds = append(cmds, newCmd)
		case MainAppState:
			newMainAppModel, newCmd := m.mainAppModel.Update(msg)
			m.mainAppModel = newMainAppModel.(mainAppModel)
			cmds = append(cmds, newCmd)
		case ApiSettingsState:
			newApiSettingsModel, newCmd := m.apiSettingsModel.Update(msg)
			m.apiSettingsModel = newApiSettingsModel.(apiSettingsModel)
			cmds = append(cmds, newCmd)
		case CharGenState:
			newCharGenModel, newCmd := m.characterGeneratorModel.Update(msg)
			m.characterGeneratorModel = newCharGenModel.(characterGeneratorModel)
			cmds = append(cmds, newCmd)
		case LibraryState:
			newNameModel, newCmd := m.libraryModel.Update(msg)
			m.libraryModel = newNameModel.(libraryModel)
			cmds = append(cmds, newCmd)
		case CardBrowserState:
			newModel, newCmd := m.cardBrowserModel.Update(msg)
			m.cardBrowserModel = newModel.(cardBrowserModel)
			cmds = append(cmds, newCmd)
		case CardEditorState:
			newModel, newCmd := m.cardEditorModel.Update(msg)
			m.cardEditorModel = newModel.(cardEditorModel)
			cmds = append(cmds, newCmd)
		case ChatState:
			newModel, newCmd := m.chatModel.Update(msg)
			m.chatModel = newModel.(chatModel)
			cmds = append(cmds, newCmd)
		}
		return m, tea.Batch(cmds...)

	case LoginSuccessMsg:
		m.user = msg.User
		m.state = MainAppState
		m.mainAppModel = initialMainAppModel(m.user) // Initialize main app model with logged-in user
		return m, m.mainAppModel.Init()

	case ShowCreateAccountMsg:
		m.state = CreateAccountState
		m.createAccountModel = initialCreateAccountModel() // Initialize create account model
		return m, m.createAccountModel.Init()

	case AccountCreatedMsg:
		// After account creation, go back to login screen
		m.state = LoginState
		m.loginModel = initialLoginModel() // Reset login model
		return m, m.loginModel.Init()

	case LogoutMsg:
		m.user = nil
		m.state = LoginState
		m.loginModel = initialLoginModel() // Reset login model
		return m, m.loginModel.Init()

	case ShowApiSettingsMsg:
		m.state = ApiSettingsState
		m.apiSettingsModel = initialApiSettingsModel(m.user) // Initialize API settings model
		return m, m.apiSettingsModel.Init()

	case ShowCharacterGeneratorMsg: // New message handling
		m.state = CharGenState
		m.characterGeneratorModel = initialCharacterGeneratorModel(m.user) // Initialize character generator model
		return m, m.characterGeneratorModel.Init()

	case ShowLibraryMsg:
		m.state = LibraryState
		m.libraryModel = initialLibraryModel(m.user)
		return m, m.libraryModel.Init()

	case ShowCardBrowserMsg:
		// Raw (non-curses) browser: takes over the terminal for crisp sixel
		// with absolute text placement, then returns via a dispatch message.
		return m, runRawBrowserCmd(m.user)

	case ShowCardEditorMsg:
		m.state = CardEditorState
		m.cardEditorModel = initialCardEditorModel(m.user, msg.CardPath)
		w, h := m.width, m.height
		return m, tea.Batch(
			m.cardEditorModel.Init(),
			func() tea.Msg { return tea.WindowSizeMsg{Width: w, Height: h} },
		)

	case ChatWithCardMsg:
		m.state = ChatState
		m.chatModel = initialChatModelFromCard(m.user, msg.CardPath)
		w, h := m.width, m.height
		return m, tea.Batch(
			m.chatModel.Init(),
			func() tea.Msg { return tea.WindowSizeMsg{Width: w, Height: h} },
		)

	case BackToMainAppMsg:
		// This message can come from any sub-model to return to the main app menu
		// or if we're in login/create account, it can mean going back to main app if already logged in,
		// or back to login if not.
		if m.user != nil {
			m.state = MainAppState
			m.mainAppModel = initialMainAppModel(m.user) // Re-initialize to refresh
			return m, m.mainAppModel.Init()
		} else {
			// If not logged in, 'esc' from create account goes back to login
			m.state = LoginState
			m.loginModel = initialLoginModel()
			return m, m.loginModel.Init()
		}

	default:
		// Delegate update calls to the currently active model
		switch m.state {
		case LoginState:
			newLoginModel, newCmd := m.loginModel.Update(msg)
			m.loginModel = newLoginModel.(loginModel)
			cmds = append(cmds, newCmd)
		case CreateAccountState:
			newCreateAccountModel, newCmd := m.createAccountModel.Update(msg)
			m.createAccountModel = newCreateAccountModel.(createAccountModel)
			cmds = append(cmds, newCmd)
		case MainAppState:
			newMainAppModel, newCmd := m.mainAppModel.Update(msg)
			m.mainAppModel = newMainAppModel.(mainAppModel)
			cmds = append(cmds, newCmd)
		case ApiSettingsState:
			newApiSettingsModel, newCmd := m.apiSettingsModel.Update(msg)
			m.apiSettingsModel = newApiSettingsModel.(apiSettingsModel)
			cmds = append(cmds, newCmd)
		case CharGenState: // Delegate for Character Generator
			newCharGenModel, newCmd := m.characterGeneratorModel.Update(msg)
			m.characterGeneratorModel = newCharGenModel.(characterGeneratorModel)
			cmds = append(cmds, newCmd)
		case LibraryState:
			newNameModel, newCmd := m.libraryModel.Update(msg)
			m.libraryModel = newNameModel.(libraryModel)
			cmds = append(cmds, newCmd)
		case CardBrowserState:
			newModel, newCmd := m.cardBrowserModel.Update(msg)
			m.cardBrowserModel = newModel.(cardBrowserModel)
			cmds = append(cmds, newCmd)
		case CardEditorState:
			newModel, newCmd := m.cardEditorModel.Update(msg)
			m.cardEditorModel = newModel.(cardEditorModel)
			cmds = append(cmds, newCmd)
		case ChatState:
			newModel, newCmd := m.chatModel.Update(msg)
			m.chatModel = newModel.(chatModel)
			cmds = append(cmds, newCmd)
		}
	}
	return m, tea.Batch(cmds...)
}

// View renders the currently active application view.
func (m AppModel) View() string {
	switch m.state {
	case LoginState:
		return m.loginModel.View()
	case CreateAccountState:
		return m.createAccountModel.View()
	case MainAppState:
		return m.mainAppModel.View()
	case ApiSettingsState:
		return m.apiSettingsModel.View()
	case CharGenState: // Render Character Generator view
		return m.characterGeneratorModel.View()
	case LibraryState:
		return m.libraryModel.View()
	case CardBrowserState:
		return m.cardBrowserModel.View()
	case CardEditorState:
		return m.cardEditorModel.View()
	case ChatState:
		return m.chatModel.View()
	case QuitState:
		return "See you next time! 👋\n"
	default:
		return "Unknown state. This should not happen."
	}
}

func main() {
	// Ensure data directories exist
	os.MkdirAll(StorageDir(), 0755)
	os.MkdirAll(Paths.CardsDir, 0755)

	log.Println("Application is running in a terminal, proceeding with TUI initialization.")

	f, err := tea.LogToFile(Paths.LogFile, "debug")
	if err != nil {
		fmt.Println("fatal:", err)
		os.Exit(1)
	}
	defer f.Close()
	log.SetOutput(f)
	log.Println("Bubble Tea program starting...")

	// Try to create the program with alternative terminal handling
	p := tea.NewProgram(initialAppModel(), 
		tea.WithAltScreen(),       // Use alternate screen buffer
		tea.WithMouseCellMotion(), // Enable mouse support
	)

	// Set up error handling for terminal issues
	if _, err := p.Run(); err != nil {
		log.Printf("Alas, there's been an error running the Bubble Tea program: %v", err)
		fmt.Printf("Alas, there's been an error: %v", err)
		
		// If it's a TTY-related error, provide specific guidance
		if err.Error() == "could not open a new TTY: open /dev/tty: no such device or address" {
			fmt.Println("\n💡 Tip: This application requires a proper terminal environment.")
			fmt.Println("Try running it in a different terminal or check your terminal settings.")
			fmt.Println("If you're using Docker or a remote connection, try adding -it flags.")
		}
		
		os.Exit(1)
	}
	log.Println("Bubble Tea program exited cleanly.")
}
