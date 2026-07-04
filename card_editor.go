package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	cc "github.com/hackenstacks/nexus-charcard"
)

// ── Styles ────────────────────────────────────────────────────────────────────

var (
	eHeaderStyle  = lipgloss.NewStyle().Background(lipgloss.Color("#0f1520")).Padding(0, 1)
	eNameStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#00d4ff")).Bold(true).Padding(0, 1)
	eBtnSave      = lipgloss.NewStyle().Background(lipgloss.Color("#00ff88")).Foreground(lipgloss.Color("#000")).Bold(true).Padding(0, 1)
	eBtnMd        = lipgloss.NewStyle().Background(lipgloss.Color("#7c3aed")).Foreground(lipgloss.Color("#fff")).Bold(true).Padding(0, 1)
	eBtnJson      = lipgloss.NewStyle().Background(lipgloss.Color("#ff8c00")).Foreground(lipgloss.Color("#000")).Bold(true).Padding(0, 1)
	eBtnChat      = lipgloss.NewStyle().Background(lipgloss.Color("#00d4ff")).Foreground(lipgloss.Color("#000")).Bold(true).Padding(0, 1)
	eLabelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#00d4ff")).Bold(true)
	eLabelFocus   = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff88")).Bold(true)
	eStatusOk     = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff88"))
	eStatusErr    = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff4444"))
	eHelpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#6e7681"))
	eKeyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff88")).Bold(true)
	eBorderStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#1e3a5f"))
	ePortraitBox  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#1e3a5f")).Padding(0, 1)
)

// ── Model ─────────────────────────────────────────────────────────────────────

type cardField struct {
	label     string
	key       string
	multiline bool
	input     textinput.Model
	area      textarea.Model
}

type cardEditorModel struct {
	user      *User
	card      *cc.Card
	portrait  string
	fields    []cardField
	viewport  viewport.Model
	focus     int
	status    string
	statusOk  bool
	loading   bool
	err       string
	cardPath  string
	width     int
	height    int
}

type cardEditorLoadedMsg struct {
	card     *cc.Card
	portrait string
	err      error
}

type cardSavedMsg struct {
	status string
	ok     bool
}

func initialCardEditorModel(user *User, cardPath string) cardEditorModel {
	vp := viewport.New(80, 20)
	return cardEditorModel{
		user:     user,
		cardPath: cardPath,
		loading:  true,
		viewport: vp,
	}
}

func (m cardEditorModel) Init() tea.Cmd {
	return func() tea.Msg {
		card, err := cc.LoadFromPNG(m.cardPath)
		if err != nil {
			return cardEditorLoadedMsg{err: err}
		}
		// Portrait: 28 wide, height calculated later; use symbols so lipgloss doesn't mangle it
		opts := cc.RenderOptions{Width: 28, Height: 20, Format: cc.Symbols}
		art, _ := cc.RenderPortraitCard(card, opts)
		return cardEditorLoadedMsg{card: card, portrait: art}
	}
}

func makeFields(card *cc.Card) []cardField {
	d := card.Data
	defs := []struct {
		label     string
		key       string
		value     string
		multiline bool
	}{
		{"Name",                     "name",                      d.Name,                    false},
		{"Description",              "description",               d.Description,             true},
		{"Personality",              "personality",               d.Personality,             true},
		{"Scenario",                 "scenario",                  d.Scenario,                true},
		{"System Prompt",            "system_prompt",             d.SystemPrompt,            true},
		{"Post History Instructions","post_history_instructions", d.PostHistoryInstructions, true},
		{"First Message",            "first_mes",                 d.FirstMes,                true},
		{"Example Messages",         "mes_example",               d.MesExample,              true},
		{"Creator",                  "creator",                   d.Creator,                 false},
		{"Creator Notes",            "creator_notes",             d.CreatorNotes,            false},
		{"Version",                  "character_version",         d.CharacterVersion,        false},
		{"Tags (comma separated)",   "tags",                      strings.Join(d.Tags, ", "), false},
	}

	fields := make([]cardField, len(defs))
	for i, def := range defs {
		f := cardField{label: def.label, key: def.key, multiline: def.multiline}
		if def.multiline {
			ta := textarea.New()
			ta.SetValue(def.value)
			ta.SetWidth(60)
			ta.SetHeight(5)
			ta.CharLimit = 0
			ta.ShowLineNumbers = false
			f.area = ta
		} else {
			ti := textinput.New()
			ti.SetValue(def.value)
			ti.Width = 60
			ti.CharLimit = 0
			f.input = ti
		}
		fields[i] = f
	}
	fields[0].input.Focus()
	return fields
}

func (m cardEditorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Update field widths and viewport
		fieldW := m.width - 32 - 6
		if fieldW < 40 { fieldW = 40 }
		for i := range m.fields {
			if m.fields[i].multiline {
				m.fields[i].area.SetWidth(fieldW)
			} else {
				m.fields[i].input.Width = fieldW
			}
		}
		m.viewport.Width = fieldW + 4
		m.viewport.Height = m.height - 5
		m.viewport.SetContent(m.renderFields())

	case cardEditorLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.card = msg.card
		m.portrait = msg.portrait
		m.fields = makeFields(msg.card)
		m.viewport.SetContent(m.renderFields())
		return m, textinput.Blink

	case cardSavedMsg:
		m.status = msg.status
		m.statusOk = msg.ok
		return m, nil

	case imagePreviewClosedMsg:
		m.viewport.SetContent(m.renderFields())
		return m, nil

	case tea.KeyMsg:
		if len(m.fields) == 0 {
			if msg.String() == "esc" {
				return m, func() tea.Msg { return ShowCardBrowserMsg{} }
			}
			return m, nil
		}
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return ShowCardBrowserMsg{} }

		case "tab":
			m.blurCurrent()
			m.focus = (m.focus + 1) % len(m.fields)
			cmds = append(cmds, m.focusCurrent())
			m.viewport.SetContent(m.renderFields())
			// Scroll to show focused field
			m.scrollToFocus()
			return m, tea.Batch(cmds...)

		case "shift+tab":
			m.blurCurrent()
			m.focus = (m.focus - 1 + len(m.fields)) % len(m.fields)
			cmds = append(cmds, m.focusCurrent())
			m.viewport.SetContent(m.renderFields())
			m.scrollToFocus()
			return m, tea.Batch(cmds...)

		case "ctrl+s":
			return m, m.doSave()
		case "ctrl+r":
			return m, m.doExportRole()
		case "ctrl+j":
			return m, m.doExportJSON()
		case "ctrl+t":
			return m, func() tea.Msg { return ChatWithCardMsg{CardPath: m.cardPath} }
		case "ctrl+p":
			name := ""
			if m.card != nil { name = m.card.Name() }
			return m, runImagePreviewCmd(m.cardPath, name)
		}

		// Delegate to focused field
		if m.fields[m.focus].multiline {
			newArea, cmd := m.fields[m.focus].area.Update(msg)
			m.fields[m.focus].area = newArea
			cmds = append(cmds, cmd)
		} else {
			newInput, cmd := m.fields[m.focus].input.Update(msg)
			m.fields[m.focus].input = newInput
			cmds = append(cmds, cmd)
		}
		m.viewport.SetContent(m.renderFields())
	}

	newVP, cmd := m.viewport.Update(msg)
	m.viewport = newVP
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *cardEditorModel) blurCurrent() {
	if len(m.fields) == 0 { return }
	if m.fields[m.focus].multiline {
		m.fields[m.focus].area.Blur()
	} else {
		m.fields[m.focus].input.Blur()
	}
}

func (m *cardEditorModel) focusCurrent() tea.Cmd {
	if len(m.fields) == 0 { return nil }
	if m.fields[m.focus].multiline {
		return m.fields[m.focus].area.Focus()
	}
	m.fields[m.focus].input.Focus()
	return nil
}

func (m *cardEditorModel) scrollToFocus() {
	// Estimate line position of focused field
	line := 0
	for i := 0; i < m.focus && i < len(m.fields); i++ {
		line += 2 // label + blank
		if m.fields[i].multiline {
			line += 7 // textarea height + border
		} else {
			line += 2
		}
	}
	if line > m.viewport.YOffset+m.viewport.Height-6 {
		m.viewport.SetYOffset(line - m.viewport.Height/2)
	} else if line < m.viewport.YOffset {
		m.viewport.SetYOffset(line)
	}
}

func (m cardEditorModel) renderFields() string {
	var b strings.Builder
	for i, f := range m.fields {
		label := f.label
		if i == m.focus {
			b.WriteString(eLabelFocus.Render("▶ "+label) + "\n")
		} else {
			b.WriteString(eLabelStyle.Render("  "+label) + "\n")
		}
		if f.multiline {
			b.WriteString(f.area.View() + "\n\n")
		} else {
			b.WriteString(f.input.View() + "\n\n")
		}
	}
	return b.String()
}

func (m *cardEditorModel) collectFields() {
	if m.card == nil || m.card.Data == nil { return }
	d := m.card.Data
	for _, f := range m.fields {
		var val string
		if f.multiline { val = f.area.Value() } else { val = f.input.Value() }
		switch f.key {
		case "name":                      d.Name = val
		case "description":               d.Description = val
		case "personality":               d.Personality = val
		case "scenario":                  d.Scenario = val
		case "system_prompt":             d.SystemPrompt = val
		case "post_history_instructions": d.PostHistoryInstructions = val
		case "first_mes":                 d.FirstMes = val
		case "mes_example":               d.MesExample = val
		case "creator":                   d.Creator = val
		case "creator_notes":             d.CreatorNotes = val
		case "character_version":         d.CharacterVersion = val
		case "tags":
			tags := []string{}
			for _, t := range strings.Split(val, ",") {
				if s := strings.TrimSpace(t); s != "" { tags = append(tags, s) }
			}
			d.Tags = tags
		}
	}
}

func (m cardEditorModel) doSave() tea.Cmd {
	return func() tea.Msg {
		m.collectFields()
		if err := m.card.SaveToPNG(m.cardPath); err != nil {
			return cardSavedMsg{status: "Save failed: " + err.Error(), ok: false}
		}
		return cardSavedMsg{status: "💾 Saved to PNG", ok: true}
	}
}

func (m cardEditorModel) doExportRole() tea.Cmd {
	return func() tea.Msg {
		m.collectFields()
		os.MkdirAll(Paths.RolesDir, 0755)
		name := strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' { return r }
			if r >= 'A' && r <= 'Z' { return r + 32 }
			return '-'
		}, strings.ReplaceAll(m.card.Name(), " ", "-"))
		name = strings.Trim(name, "-")
		rolePath := filepath.Join(Paths.RolesDir, name+".md")
		if err := os.WriteFile(rolePath, []byte(m.card.ToAichatRole("")+"\n"), 0644); err != nil {
			return cardSavedMsg{status: "Export failed: " + err.Error(), ok: false}
		}
		return cardSavedMsg{status: "⬡ Exported → " + rolePath, ok: true}
	}
}

func (m cardEditorModel) doExportJSON() tea.Cmd {
	return func() tea.Msg {
		m.collectFields()
		jsonBytes, err := m.card.ToJSON()
		if err != nil {
			return cardSavedMsg{status: "JSON error: " + err.Error(), ok: false}
		}
		name := strings.ToLower(strings.ReplaceAll(m.card.Name(), " ", "-"))
		jsonPath := filepath.Join(filepath.Dir(m.cardPath), name+".json")
		if err := os.WriteFile(jsonPath, jsonBytes, 0644); err != nil {
			return cardSavedMsg{status: "Write failed: " + err.Error(), ok: false}
		}
		return cardSavedMsg{status: "{} Saved → " + jsonPath, ok: true}
	}
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m cardEditorModel) View() string {
	if m.loading {
		return eHelpStyle.Render("\n  Loading card...")
	}
	if m.err != "" {
		return eStatusErr.Render("\n  Error: " + m.err + "\n\n  Press esc to go back.")
	}

	name := ""
	if m.card != nil { name = m.card.Name() }

	// ── Header bar ────────────────────────────────────────────────────────
	header := lipgloss.JoinHorizontal(lipgloss.Center,
		eNameStyle.Render("⬡ "+name),
		"  ",
		eBtnSave.Render("💾 Save"),
		" ",
		eBtnMd.Render("⬡ .md"),
		" ",
		eBtnJson.Render("{} JSON"),
		" ",
		eBtnChat.Render("💬 Chat"),
	)

	// ── Left: portrait ────────────────────────────────────────────────────
	portraitContent := m.portrait
	if portraitContent == "" {
		portraitContent = eHelpStyle.Render("\n  No portrait\n")
	}

	// File path below portrait
	shortPath := m.cardPath
	if len(shortPath) > 30 { shortPath = "..." + shortPath[len(shortPath)-27:] }
	metaBlock := ePortraitBox.Render(
		portraitContent + "\n" +
		eHelpStyle.Render(shortPath),
	)

	// ── Right: scrollable fields ──────────────────────────────────────────
	fieldPane := eBorderStyle.Render(m.viewport.View())

	// ── Status bar ────────────────────────────────────────────────────────
	var statusLine string
	if m.status != "" {
		if m.statusOk {
			statusLine = eStatusOk.Render("  " + m.status)
		} else {
			statusLine = eStatusErr.Render("  " + m.status)
		}
	}

	help := eKeyStyle.Render("tab") + eHelpStyle.Render(" next  ") +
		eKeyStyle.Render("ctrl+s") + eHelpStyle.Render(" save  ") +
		eKeyStyle.Render("ctrl+r") + eHelpStyle.Render(" →aichat  ") +
		eKeyStyle.Render("ctrl+j") + eHelpStyle.Render(" →json  ") +
		eKeyStyle.Render("ctrl+t") + eHelpStyle.Render(" chat  ") +
		eKeyStyle.Render("ctrl+p") + eHelpStyle.Render(" preview  ") +
		eKeyStyle.Render("esc") + eHelpStyle.Render(" back")

	body := lipgloss.JoinHorizontal(lipgloss.Top, metaBlock, "  ", fieldPane)

	parts := []string{header, body}
	if statusLine != "" { parts = append(parts, statusLine) }
	parts = append(parts, help)

	return strings.Join(parts, "\n")
}
