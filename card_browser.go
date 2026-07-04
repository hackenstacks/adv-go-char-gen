package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	cc "github.com/hackenstacks/nexus-charcard"
)

// ── Styles ────────────────────────────────────────────────────────────────────

var (
	cbTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00d4ff")).Bold(true).Padding(0, 1)
	cbSelectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00d4ff")).Bold(true)
	cbNormalStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#c9d1d9"))
	cbMutedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#6e7681"))
	cbTagStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#7c3aed"))
	cbBorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#1e3a5f"))
	cbKeyStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff88")).Bold(true)
	cbErrorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff4444"))
	cbSelBoxStyle   = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#00d4ff")).
				Foreground(lipgloss.Color("#00d4ff")).
				Bold(true)
	cbNormalBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#1e2a3f"))
)

// ── Card browser model ────────────────────────────────────────────────────────

type cardBrowserModel struct {
	user        *User
	cards       []*cc.Card
	thumbs      map[string]string
	cursor      int
	portrait    string
	err         string
	cardsDir    string
	width       int
	height      int
	loading     bool
	promptDir   bool
	dirInput    textinput.Model
	leftVP      viewport.Model
	rightVP     viewport.Model
	previewMode bool   // fullscreen sixel preview
	previewArt  string
	showHelp    bool
	filtered    []*cc.Card // active (searched) view
	search      textinput.Model
	searching   bool
}

type previewRenderedMsg struct{ art string }

type thumbLoadedMsg struct {
	path string
	art  string
}

type cardsLoadedMsg struct {
	cards []*cc.Card
	err   error
}

type portraitRenderedMsg struct {
	art string
	err error
	idx int
}

func initialCardBrowserModel(user *User) cardBrowserModel {
	ti := textinput.New()
	ti.Placeholder = "e.g. /home/user/ai-characters"
	ti.Width = 60
	ti.CharLimit = 256

	dir := Paths.CardsDir
	_, statErr := os.Stat(dir)
	prompt := statErr != nil // prompt if directory doesn't exist

	si := textinput.New()
	si.Placeholder = "search…"
	si.Width = 24
	si.CharLimit = 64

	m := cardBrowserModel{
		user:      user,
		cardsDir:  dir,
		dirInput:  ti,
		search:    si,
		promptDir: prompt,
		loading:   !prompt,
		leftVP:    viewport.New(36, 40),
		rightVP:   viewport.New(80, 40),
	}
	if prompt {
		m.dirInput.SetValue(dir)
		m.dirInput.Focus()
	}
	return m
}

func (m cardBrowserModel) Init() tea.Cmd {
	if m.promptDir {
		return textinput.Blink
	}
	return func() tea.Msg {
		cards, err := cc.ListCards(m.cardsDir)
		return cardsLoadedMsg{cards: cards, err: err}
	}
}

// renderPortrait renders the selected card as a large true-color image.
// High graphics → sixel (perfect pixels); low → truecolor symbols.
func (m cardBrowserModel) renderPortrait() tea.Cmd {
	if len(m.filtered) == 0 || m.cursor >= len(m.filtered) {
		return nil
	}
	card := m.filtered[m.cursor]
	// Image occupies the left ~half; height ~ terminal height minus chrome.
	// Portrait-sized image; text is placed below via row-padding in View.
	ph := m.portraitRows()
	pw := ph * 3 / 2 // ~portrait aspect (cells are ~2:1, so w≈h*3/2 looks ~2:3)
	if pw > m.width-2 { pw = m.width - 2 }
	if pw < 24 { pw = 24 }
	format := cc.Sixel
	if GraphicsMode == "low" {
		format = cc.Symbols
	}
	idx := m.cursor
	return func() tea.Msg {
		opts := cc.RenderOptions{Width: pw, Height: ph, Format: format}
		art, err := cc.RenderPortraitCard(card, opts)
		if err != nil && format == cc.Sixel {
			opts.Format = cc.Symbols
			art, err = cc.RenderPortraitCard(card, opts)
		}
		return portraitRenderedMsg{art: art, err: err, idx: idx}
	}
}

func (m cardBrowserModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// ── Directory prompt mode ─────────────────────────────────────────────
	if m.promptDir {
		// Check for special keys before passing to textinput
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.Type {
			case tea.KeyEsc:
				return m, func() tea.Msg { return BackToMainAppMsg{} }
			case tea.KeyEnter:
				dir := strings.TrimSpace(m.dirInput.Value())
				if dir == "" {
					return m, nil
				}
				if strings.HasPrefix(dir, "~/") {
					if home, err := os.UserHomeDir(); err == nil {
						dir = home + dir[1:]
					}
				}
				os.MkdirAll(dir, 0755)
				Paths.CardsDir = dir
				m.cardsDir = dir
				m.promptDir = false
				m.err = ""
				m.loading = true
				loadDir := dir
				return m, func() tea.Msg {
					cards, err := cc.ListCards(loadDir)
					return cardsLoadedMsg{cards: cards, err: err}
				}
			}
		}
		// All other messages (including regular key presses) go to the textinput
		var cmd tea.Cmd
		m.dirInput, cmd = m.dirInput.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.portrait = ""
		return m, m.renderPortrait()

	case cardsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		// If directory exists but is empty, prompt for a new one
		if len(msg.cards) == 0 && msg.err == nil {
			m.dirInput.SetValue(m.cardsDir)
			m.dirInput.Focus()
			m.promptDir = true
			m.err = "No PNG cards found in: " + m.cardsDir
			return m, textinput.Blink
		}
		m.cards = msg.cards
		m.filtered = msg.cards
		m.cursor = 0
		return m, m.renderPortrait()

	case portraitRenderedMsg:
		// Ignore stale renders from previous selections
		if msg.err == nil && msg.idx == m.cursor {
			m.portrait = msg.art
		}

	case tea.MouseMsg:
		var cmd tea.Cmd
		m.leftVP, cmd = m.leftVP.Update(msg)
		return m, cmd

	case previewRenderedMsg:
		m.previewArt = msg.art
		m.previewMode = true

	case tea.KeyMsg:
		// In fullscreen preview, any key exits
		if m.previewMode {
			m.previewMode = false
			m.previewArt = ""
			return m, nil
		}
		// In help overlay, any key closes it
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		// Search mode: keystrokes filter the grid
		if m.searching {
			switch msg.Type {
			case tea.KeyEsc:
				m.searching = false
				m.search.Blur()
				m.search.SetValue("")
				m.applyFilter()
				return m, nil
			case tea.KeyEnter:
				m.searching = false
				m.search.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			m.applyFilter()
			return m, tea.Batch(cmd, m.renderPortrait())
		}
		switch msg.String() {
		case "/":
			m.searching = true
			m.search.Focus()
			return m, textinput.Blink
		case "?":
			m.showHelp = true
			return m, nil
		case "pgdown":
			m.leftVP.ViewDown()
			return m, nil
		case "pgup":
			m.leftVP.ViewUp()
			return m, nil
		case "esc", "q":
			return m, func() tea.Msg { return BackToMainAppMsg{} }
		case "v":
			// Fullscreen high-res sixel preview of the selected card
			if len(m.filtered) > 0 && GraphicsMode == "high" {
				card := m.filtered[m.cursor]
				return m, func() tea.Msg {
					opts := cc.RenderOptions{Width: 80, Height: 46, Format: cc.Sixel}
					art, err := cc.RenderPortraitCard(card, opts)
					if err != nil {
						return previewRenderedMsg{art: "Preview unavailable: " + err.Error()}
					}
					return previewRenderedMsg{art: art}
				}
			}
		case "left", "h", "up", "k":
			if m.cursor > 0 {
				m.cursor--
				return m, m.onSelect()
			}
		case "right", "l", "down", "j":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
				return m, m.onSelect()
			}
		case "enter", "e":
			if len(m.filtered) > 0 {
				path := m.filtered[m.cursor].SourcePath
				return m, func() tea.Msg { return ShowCardEditorMsg{CardPath: path} }
			}
		case "c":
			if len(m.filtered) > 0 {
				path := m.filtered[m.cursor].SourcePath
				return m, func() tea.Msg { return ChatWithCardMsg{CardPath: path} }
			}
		case "i":
			// Import selected card into the encrypted character library
			if len(m.filtered) > 0 {
				card := m.filtered[m.cursor]
				if err := importCardToLibrary(m.user, card); err != nil {
					m.err = "Import failed: " + err.Error()
				} else {
					m.err = "✓ Imported " + card.Name() + " to your character library"
				}
			}
			return m, nil
		case "d":
			// Change cards directory
			m.dirInput.SetValue(m.cardsDir)
			m.dirInput.Focus()
			m.promptDir = true
			return m, textinput.Blink
		}
	}
	return m, nil
}

func (m cardBrowserModel) View() string {
	if m.showHelp {
		return m.helpOverlay()
	}
	// Fullscreen sixel preview — raw output, no lipgloss
	if m.previewMode {
		name := ""
		if m.cursor < len(m.filtered) {
			name = m.filtered[m.cursor].Name()
		}
		return cbSelectedStyle.Render("⬡ "+name) + "\n" +
			m.previewArt + "\n" +
			cbMutedStyle.Render("  press any key to close")
	}
	if m.promptDir {
		return cbTitleStyle.Render("⬡ Card Browser — Set Cards Directory") + "\n\n" +
			cbMutedStyle.Render("  Enter the path to your PNG character card directory:\n\n") +
			"  " + m.dirInput.View() + "\n\n" +
			cbMutedStyle.Render("  This can also be set via the ") +
			cbKeyStyle.Render("CARDS_DIR") +
			cbMutedStyle.Render(" environment variable.\n\n") +
			cbKeyStyle.Render("  enter") + cbMutedStyle.Render(" confirm  ") +
			cbKeyStyle.Render("esc") + cbMutedStyle.Render(" back")
	}
	if m.loading {
		return cbTitleStyle.Render("⬡ Card Browser") + "\n\n" + cbMutedStyle.Render("  Loading cards...")
	}
	if m.err != "" {
		return cbTitleStyle.Render("⬡ Card Browser") + "\n\n" + cbErrorStyle.Render("  Error: "+m.err)
	}
	if len(m.cards) == 0 {
		return cbTitleStyle.Render("⬡ Card Browser") + "\n\n" +
			cbMutedStyle.Render(fmt.Sprintf("  No PNG cards found in %s", m.cardsDir))
	}

	// Header with title left, search box right
	title := cbTitleStyle.Render("⬡ NeXuS PNG² Browser") +
		cbMutedStyle.Render(fmt.Sprintf("  %d/%d", len(m.filtered), len(m.cards)))
	searchBox := ""
	if m.searching {
		searchBox = cbKeyStyle.Render("🔍 ") + m.search.View()
	} else {
		searchBox = cbMutedStyle.Render("🔍 press / to search")
	}
	gap := m.width - lipgloss.Width(title) - lipgloss.Width(searchBox) - 2
	if gap < 1 { gap = 1 }
	header := title + strings.Repeat(" ", gap) + searchBox

	if len(m.filtered) == 0 {
		return header + "\n\n" + cbMutedStyle.Render("  No cards match your search.")
	}

	// ── Details line (name + position + tags + short desc) ────────────────
	card := m.filtered[m.cursor]
	var detail strings.Builder
	detail.WriteString(cbSelectedStyle.Render(card.Name()))
	detail.WriteString(cbMutedStyle.Render(fmt.Sprintf("   [%d/%d]", m.cursor+1, len(m.filtered))))
	if card.Data != nil && len(card.Data.Tags) > 0 {
		detail.WriteString("   " + cbTagStyle.Render(strings.Join(card.Data.Tags, " ")))
	}
	detail.WriteString("\n")
	if card.Data != nil {
		desc := strings.ReplaceAll(strings.ReplaceAll(card.Data.Description, "\n", " "), "\r", "")
		desc = strings.ReplaceAll(strings.ReplaceAll(desc, "{{char}}", card.Name()), "{{user}}", "User")
		w := m.width - 4
		if w < 20 { w = 20 }
		// two lines max on the header strip
		wrapped := wrapText(desc, w)
		dl := strings.Split(wrapped, "\n")
		if len(dl) > 2 { dl = dl[:2]; dl[1] += "…" }
		detail.WriteString(cbMutedStyle.Render(strings.Join(dl, "\n")))
	}

	help := cbKeyStyle.Render("←→/↑↓") + cbMutedStyle.Render(" flip  ") +
		cbKeyStyle.Render("enter") + cbMutedStyle.Render(" open  ") +
		cbKeyStyle.Render("c") + cbMutedStyle.Render(" chat  ") +
		cbKeyStyle.Render("i") + cbMutedStyle.Render(" import  ") +
		cbKeyStyle.Render("/") + cbMutedStyle.Render(" search  ") +
		cbKeyStyle.Render("?") + cbMutedStyle.Render(" help  ") +
		cbKeyStyle.Render("q") + cbMutedStyle.Render(" back")

	// ── True-color sixel image; foot advances the cursor below it, so text
	//    placed right after (like the working v-preview) lands underneath. ──
	image := m.portrait
	if image == "" {
		return header + "\n\n  rendering…\n\n" + detail.String() + "\n" + help
	}
	return header + "\n" + image + "\n" + detail.String() + "\n" + help
}

// portraitRows returns the row height used for the sixel image, matching renderPortrait.
// Leaves room for the header (1) and details+help (~6) below the image.
func (m cardBrowserModel) portraitRows() int {
	ph := m.height - 8
	if ph < 10 { ph = 10 }
	if ph > 40 { ph = 40 }
	return ph
}

// helpOverlay renders an in-app help/keys screen.
func (m cardBrowserModel) helpOverlay() string {
	title := cbTitleStyle.Render("⬡ Card Browser — Help")
	k := func(key, desc string) string {
		return "  " + cbKeyStyle.Render(fmt.Sprintf("%-10s", key)) + cbMutedStyle.Render(desc)
	}
	sec := func(s string) string { return cbSelectedStyle.Render(s) }

	lines := []string{
		title, "",
		sec("Navigation"),
		k("↑ ↓ / j k", "move between cards"),
		k("q / esc", "back to main menu"),
		"",
		sec("Actions"),
		k("enter / e", "open the card editor"),
		k("c", "chat with the character (ephemeral)"),
		k("i", "import card into your character library"),
		k("d", "change the cards directory"),
		"",
		sec("Images"),
		k("v", "fullscreen true-color (sixel) preview"),
		cbMutedStyle.Render("  Inline images use 24-bit truecolor symbols (blocky but"),
		cbMutedStyle.Render("  accurate). The v preview is crisp sixel — best in foot."),
		"",
		sec("Graphics mode  (env: CHAR_GEN_GRAPHICS)"),
		cbMutedStyle.Render("  high  = sixel preview enabled (default)"),
		cbMutedStyle.Render("  low   = symbols only (tmux / no-sixel terminals)"),
		cbMutedStyle.Render("  tmux users: tmux set -g allow-passthrough on"),
		"",
		cbMutedStyle.Render("  Current mode: ") + cbKeyStyle.Render(GraphicsMode),
		"",
		sec("Editor keys (inside a card)"),
		k("tab", "next field"),
		k("ctrl+s", "save to PNG"),
		k("ctrl+r", "export as aichat role (.md)"),
		k("ctrl+j", "export as JSON"),
		k("ctrl+c", "chat with this card"),
		"",
		cbMutedStyle.Render("  Full docs: HELP.md in the project root"),
		"",
		cbKeyStyle.Render("  press any key to close"),
	}
	return strings.Join(lines, "\n")
}

// buildLeftContent renders all card entries for the left viewport.
// cellInnerW is the content width inside each grid card (excluding the border).
const cellInnerW = 22
const cellTotalW = cellInnerW + 2 // + border

// applyFilter rebuilds m.filtered from the search query (name/tags/description).
func (m *cardBrowserModel) applyFilter() {
	q := strings.ToLower(strings.TrimSpace(m.search.Value()))
	if q == "" {
		m.filtered = m.cards
	} else {
		var out []*cc.Card
		for _, c := range m.cards {
			hay := strings.ToLower(c.Name())
			if c.Data != nil {
				hay += " " + strings.ToLower(c.Data.Description)
				hay += " " + strings.ToLower(strings.Join(c.Data.Tags, " "))
			}
			if strings.Contains(hay, q) {
				out = append(out, c)
			}
		}
		m.filtered = out
	}
	m.cursor = 0
}

// gridCols returns how many card columns fit in the current width.
func (m cardBrowserModel) gridCols() int {
	c := m.width / (cellTotalW + 1)
	if c < 1 { c = 1 }
	return c
}

// gridRowHeight is the number of terminal rows a card cell occupies
// (thumbnail 11 + name 1 + desc 2 + tags 1 + border 2 ≈ 17).
const gridRowHeight = 17

// onSelect re-renders the large true-color image for the newly selected card.
func (m *cardBrowserModel) onSelect() tea.Cmd {
	m.portrait = ""
	return m.renderPortrait()
}

// renderCell renders a single card as a bordered gallery cell.
func (m cardBrowserModel) renderCell(card *cc.Card, selected bool) string {
	name := card.Name()
	if len(name) > cellInnerW { name = name[:cellInnerW-1] + "…" }

	desc := ""
	tags := ""
	if card.Data != nil {
		desc = strings.ReplaceAll(strings.ReplaceAll(card.Data.Description, "\n", " "), "\r", "")
		desc = strings.ReplaceAll(strings.ReplaceAll(desc, "{{char}}", card.Name()), "{{user}}", "User")
		if len(card.Data.Tags) > 0 {
			n := len(card.Data.Tags)
			if n > 2 { n = 2 }
			tags = strings.Join(card.Data.Tags[:n], " ")
			if len(tags) > cellInnerW { tags = tags[:cellInnerW] }
		}
	}
	// two-line description clamp
	descWrap := wrapText(desc, cellInnerW)
	dlines := strings.Split(descWrap, "\n")
	if len(dlines) > 2 { dlines = dlines[:2]; dlines[1] += "…" }
	desc = strings.Join(dlines, "\n")

	thumb := m.thumbs[card.SourcePath]
	if thumb == "" {
		thumb = strings.Repeat("\n", 5) // placeholder space while loading
	}

	var c strings.Builder
	c.WriteString(thumb)
	c.WriteString("\n")
	if selected {
		c.WriteString(cbSelectedStyle.Render(name))
	} else {
		c.WriteString(cbNormalStyle.Bold(true).Render(name))
	}
	c.WriteString("\n")
	c.WriteString(cbMutedStyle.Render(desc))
	if tags != "" {
		c.WriteString("\n" + cbTagStyle.Render(tags))
	}

	style := cbNormalBoxStyle
	if selected { style = cbSelBoxStyle }
	return style.Width(cellInnerW).Height(gridRowHeight - 2).Render(c.String())
}

// buildGrid arranges all cards into a responsive grid of cells.
func (m cardBrowserModel) buildGrid() string {
	cols := m.gridCols()
	var rows []string
	for i := 0; i < len(m.filtered); i += cols {
		var cells []string
		for j := i; j < i+cols && j < len(m.filtered); j++ {
			cells = append(cells, m.renderCell(m.filtered[j], j == m.cursor))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}
	return strings.Join(rows, "\n")
}

// scrollGridToSelected keeps the selected row visible in the grid viewport.
func (m *cardBrowserModel) scrollGridToSelected() {
	cols := m.gridCols()
	row := m.cursor / cols
	lineOffset := row * gridRowHeight
	vpH := m.leftVP.Height
	if lineOffset < m.leftVP.YOffset {
		m.leftVP.SetYOffset(lineOffset)
	} else if lineOffset+gridRowHeight > m.leftVP.YOffset+vpH {
		m.leftVP.SetYOffset(lineOffset + gridRowHeight - vpH)
	}
}

// buildRightContent renders the detail panel for the selected card.
func (m cardBrowserModel) buildRightContent() string {
	if m.cursor >= len(m.filtered) { return "" }
	card := m.filtered[m.cursor]
	if card.Data == nil { return "" }

	d := card.Data
	name := d.Name
	rightW := m.rightVP.Width - 4
	if rightW < 30 { rightW = 30 }

	var b strings.Builder
	if m.portrait != "" { b.WriteString(m.portrait + "\n") }

	b.WriteString(cbSelectedStyle.Render(name) + "\n")
	if len(d.Tags) > 0 {
		b.WriteString(cbTagStyle.Render(strings.Join(d.Tags, "  ")) + "\n")
	}
	b.WriteString("\n")

	printField := func(label, value string) {
		if value == "" { return }
		value = strings.ReplaceAll(value, "{{char}}", name)
		value = strings.ReplaceAll(value, "{{user}}", "User")
		value = strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\n", " ")
		// No truncation — full text; the pane scrolls with the mouse wheel / PgUp-PgDn
		b.WriteString(cbKeyStyle.Render(label) + "\n")
		b.WriteString(cbMutedStyle.Render(wrapText(value, rightW)) + "\n\n")
	}

	printField("Description",  d.Description)
	printField("Personality",  d.Personality)
	printField("Scenario",     d.Scenario)
	printField("System",       d.SystemPrompt)
	printField("First Msg",    d.FirstMes)
	if d.Creator != "" {
		b.WriteString(cbMutedStyle.Render("Creator: "+d.Creator) + "\n")
	}
	return b.String()
}

// wrapText wraps s to at most width chars per line, breaking on spaces.
func wrapText(s string, width int) string {
	if width <= 0 || len(s) <= width {
		return s
	}
	var out strings.Builder
	for len(s) > 0 {
		if len(s) <= width {
			out.WriteString(s)
			break
		}
		cut := width
		for cut > 0 && s[cut] != ' ' {
			cut--
		}
		if cut == 0 {
			cut = width
		}
		out.WriteString(s[:cut] + "\n")
		s = strings.TrimLeft(s[cut:], " ")
	}
	return out.String()
}

// ── Chat from card ────────────────────────────────────────────────────────────

// initialChatModelFromCard loads a PNG card and starts an ephemeral chat with its persona.
func initialChatModelFromCard(user *User, cardPath string) chatModel {
	apiCfg, _ := LoadAPIConfig(user)
	LoadEnvOverlay(&apiCfg)
	mm := NewLocalMemoryManager(nil, nil)
	llmSvc, _ := LLMFactory(apiCfg.SelectedLLMProvider, apiCfg)
	imgSvc, _ := ImageFactory(apiCfg.SelectedImageProvider, apiCfg)
	if llmSvc == nil {
		llmSvc, _ = LLMFactory(ProviderMock, apiCfg)
	}
	if imgSvc == nil {
		imgSvc, _ = ImageFactory(ProviderMock, apiCfg)
	}
	m := initialChatModel(user, &apiCfg, mm, llmSvc, imgSvc)

	// Wire the real provider + model into the chat state
	m.selectedLLMProvider = apiCfg.SelectedLLMProvider
	m.selectedLLMModel = resolveDefaultModel(&apiCfg)
	m.cardChatMode = true

	if apiCfg.SelectedLLMProvider == ProviderMock || apiCfg.SelectedLLMProvider == "" {
		m.messages = append(m.messages, Message{
			ID: GenerateRandomID(), Sender: "System",
			Content:   "⚠ No AI provider configured — replies will be mock text. Set CUSTOM_API_URL in .env (e.g. http://localhost:3030 for aichat) or configure API Settings.",
			Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
		})
	}

	card, err := cc.LoadFromPNG(cardPath)
	if err == nil && card != nil {
		m.cardName = card.Name()
		m.systemPrompt = card.SystemPrompt(user.Username)
		// Render avatars (symbols — compose with chat text layout, editor-style)
		if av, e := cc.RenderPortraitCard(card, cc.RenderOptions{Width: 26, Height: 18, Format: cc.Symbols}); e == nil {
			m.cardAvatar = av
		}
		if av, e := cc.RenderPortraitCard(card, cc.RenderOptions{Width: 14, Height: 7, Format: cc.Symbols}); e == nil {
			m.cardAvatarSm = av
		}
		greeting := ""
		if card.Data != nil {
			greeting = card.Data.FirstMes
			greeting = strings.ReplaceAll(greeting, "{{char}}", card.Name())
			greeting = strings.ReplaceAll(greeting, "{{user}}", user.Username)
		}
		m.messages = append(m.messages, Message{
			ID: GenerateRandomID(), Sender: "System",
			Content:   "Chatting with " + card.Name() + " via " + string(m.selectedLLMProvider) + " (" + m.selectedLLMModel + ") — ephemeral, not saved.",
			Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
		})
		if strings.TrimSpace(greeting) != "" {
			m.messages = append(m.messages, Message{
				ID: GenerateRandomID(), Sender: card.Name(),
				Content: greeting, Timestamp: time.Now().Unix(), Type: MessageTypeCharacter,
			})
		}
	}
	return m
}

// importCardToLibrary converts a PNG card into an app Character and saves it
// to the encrypted library, making it usable in full chat / AI2AI / memory.
func importCardToLibrary(user *User, card *cc.Card) error {
	if card == nil || card.Data == nil {
		return fmt.Errorf("card has no data")
	}
	d := card.Data
	ch := NewCharacter()
	ch.Name = card.Name()
	ch.Description = strings.ReplaceAll(strings.ReplaceAll(d.Description, "{{char}}", ch.Name), "{{user}}", user.Username)
	ch.Personality = d.Personality
	ch.Scenario = d.Scenario
	ch.FirstMessage = strings.ReplaceAll(strings.ReplaceAll(d.FirstMes, "{{char}}", ch.Name), "{{user}}", user.Username)
	// Fold system prompt + post-history into lorebook so nothing is lost
	if d.SystemPrompt != "" {
		ch.Lorebook["system_prompt"] = d.SystemPrompt
	}
	if d.PostHistoryInstructions != "" {
		ch.Lorebook["post_history"] = d.PostHistoryInstructions
	}
	if len(d.Tags) > 0 {
		ch.Lorebook["tags"] = strings.Join(d.Tags, ", ")
	}
	return SaveCharacter(user, ch)
}

// resolveDefaultModel picks a sensible model name for the selected provider.
func resolveDefaultModel(cfg *APIConfig) string {
	switch cfg.SelectedLLMProvider {
	case ProviderMistral:
		if cfg.Mistral.DefaultLLMModel != "" { return string(cfg.Mistral.DefaultLLMModel) }
		return "mistral-small-latest"
	case ProviderOpenAI:
		if cfg.OpenAI.DefaultLLMModel != "" { return string(cfg.OpenAI.DefaultLLMModel) }
		return "gpt-4o-mini"
	case ProviderClaude:
		if cfg.Claude.DefaultLLMModel != "" { return string(cfg.Claude.DefaultLLMModel) }
		return "claude-3-5-haiku-latest"
	case ProviderGroq:
		if cfg.Groq.DefaultLLMModel != "" { return string(cfg.Groq.DefaultLLMModel) }
		return "llama-3.1-8b-instant"
	case ProviderPollinations:
		if cfg.Pollinations.DefaultLLMModel != "" { return string(cfg.Pollinations.DefaultLLMModel) }
		return "openai"
	case ProviderCustomLLM:
		for _, c := range cfg.CustomAPIs {
			if c.Type == "llm" && c.Enabled && c.DefaultLLMModel != "" {
				return string(c.DefaultLLMModel)
			}
		}
		return "default"
	}
	return "mock-model"
}
