package main

import (
	"fmt"
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	mainTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF00")).
			Bold(true).
			Padding(1, 4)

	mainItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF"))

	mainSelectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFF00")).
				Bold(true)
)

type mainAppModel struct {
	user        *User
	choices     []string
	cursor      int
	selectedApp AppState
}

func initialMainAppModel(user *User) mainAppModel {
	return mainAppModel{
		user: user,
		choices: []string{
			"Character Generator",
			"Card Browser",
			"Library",
			"API Settings",
			"Logout",
		},
	}
}

func (m mainAppModel) Init() tea.Cmd {
	return nil
}

func (m mainAppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		log.Printf("mainAppModel received key press: %s", msg.String())
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "enter":
			switch m.cursor {
			case 0:
				return m, func() tea.Msg { return ShowCharacterGeneratorMsg{} }
			case 1:
				return m, func() tea.Msg { return ShowCardBrowserMsg{} }
			case 2:
				return m, func() tea.Msg { return ShowLibraryMsg{} }
			case 3:
				return m, func() tea.Msg { return ShowApiSettingsMsg{} }
			case 4:
				return m, func() tea.Msg { return LogoutMsg{} }
			}
		}
	case BackToMainAppMsg:
		// This message is sent when user presses esc in sub-menus to return to main menu
		// No action needed here as we're already in the main menu
		return m, nil
	}
	return m, nil
}

func (m mainAppModel) View() string {
	s := mainTitleStyle.Render(fmt.Sprintf("Welcome, %s! What would you like to do?", m.user.Username))
	s += "\n\n"

	for i, choice := range m.choices {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
			s += mainSelectedItemStyle.Render(fmt.Sprintf("%s %s", cursor, choice))
		} else {
			s += mainItemStyle.Render(fmt.Sprintf("%s %s", cursor, choice))
		}
		s += "\n"
	}

	s += "\nPress 'q' to quit."
	return s
}