package main

// AppState defines the current state of the application.
type AppState int

// All the possible states of the application.
const (
	LoginState AppState = iota
	CreateAccountState
	MainAppState
	ApiSettingsState
	CharGenState
	LibraryState
	CardBrowserState
	CardEditorState
	ChatState
	QuitState
)

// Messages that are used to communicate between different parts of the application.

// BackToMainAppMsg is a message to go back to the main application screen.
type BackToMainAppMsg struct{}

// LoginSuccessMsg is a message to indicate a successful login.
type LoginSuccessMsg struct {
	User *User
}

// ShowCreateAccountMsg is a message to show the create account screen.
type ShowCreateAccountMsg struct{}

// AccountCreatedMsg is a message to indicate that an account has been created.
type AccountCreatedMsg struct{}

// ShowCharacterGeneratorMsg is a message to show the character generator screen.
type ShowCharacterGeneratorMsg struct{}

// ShowApiSettingsMsg is a message to show the API settings screen.
type ShowApiSettingsMsg struct{}

// ShowLibraryMsg is a message to show the library screen.
type ShowLibraryMsg struct{}

// ShowCardBrowserMsg is a message to show the PNG card browser.
type ShowCardBrowserMsg struct{}

// ShowCardEditorMsg opens a specific card in the editor.
type ShowCardEditorMsg struct{ CardPath string }

// ChatWithCardMsg loads a PNG card into an ephemeral chat session.
type ChatWithCardMsg struct{ CardPath string }

// AI2AICompleteMsg carries all messages from a completed AI-to-AI conversation.
type AI2AICompleteMsg struct {
	Intro    Message
	Messages []Message
	Err      error
}

// modelsListMsg carries the list of available models from the active provider.
type modelsListMsg struct{ models []string }

// compactDoneMsg carries a conversation summary from /compact.
type compactDoneMsg struct{ summary string }

// imageGeneratedMsg carries a generated image's local path.
type imageGeneratedMsg struct {
	path   string
	prompt string
}

// LogoutMsg is a message to log out the user.
type LogoutMsg struct{}
