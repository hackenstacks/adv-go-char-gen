package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
	cc "github.com/hackenstacks/nexus-charcard"
)

// rawCardBrowser is a NON-curses card browser. It implements tea.ExecCommand so
// bubbletea releases the terminal; we then take full raw control — emitting
// sixel and positioning text with absolute ANSI cursor moves. This is the only
// way to get a crisp true-color image with text reliably placed below it.
type rawCardBrowser struct {
	user   *User
	stdin  io.Reader
	stdout io.Writer

	// result — read by the bubbletea callback after Run returns
	action string // "edit" | "chat" | "import" | "back"
	path   string
}

func (b *rawCardBrowser) SetStdin(r io.Reader)  { b.stdin = r }
func (b *rawCardBrowser) SetStdout(w io.Writer) { b.stdout = w }
func (b *rawCardBrowser) SetStderr(w io.Writer) {}

const (
	ansiClear   = "\033[2J\033[H"
	ansiHome    = "\033[H"
	ansiHideCur = "\033[?25l"
	ansiShowCur = "\033[?25h"
	cReset      = "\033[0m"
	cCyan       = "\033[38;2;0;212;255m"
	cGreen      = "\033[38;2;0;255;136m"
	cPurple     = "\033[38;2;124;58;237m"
	cMuted      = "\033[38;2;110;118;129m"
	cBold       = "\033[1m"
)

func at(row, col int) string { return fmt.Sprintf("\033[%d;%dH", row, col) }

func (b *rawCardBrowser) Run() error {
	out := b.stdout
	if out == nil { out = os.Stdout }

	// Load cards
	home, _ := os.UserHomeDir()
	dir := Paths.CardsDir
	cards, err := cc.ListCards(dir)
	if err != nil || len(cards) == 0 {
		// Try the common fallback
		alt := home + "/01/ai-characters"
		if c2, e2 := cc.ListCards(alt); e2 == nil && len(c2) > 0 {
			cards, dir = c2, alt
		}
	}
	if len(cards) == 0 {
		b.action = "back"
		return nil
	}

	// Raw mode
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		b.action = "back"
		return nil
	}
	defer term.Restore(fd, oldState)
	fmt.Fprint(out, ansiHideCur)
	defer fmt.Fprint(out, ansiShowCur+cReset+ansiClear)

	filtered := cards
	cursor := 0
	search := ""
	searching := false
	buf := make([]byte, 8)

	render := func() {
		cols, rows, _ := term.GetSize(fd)
		if cols <= 0 { cols = 100 }
		if rows <= 0 { rows = 40 }

		var sb strings.Builder
		sb.WriteString(ansiClear)

		// Header: title left, search right
		title := fmt.Sprintf("%s%s⬡ NeXuS PNG² Browser%s  %s%d/%d%s",
			cCyan, cBold, cReset, cMuted, len(filtered), len(cards), cReset)
		sright := cMuted + "🔍 / to search" + cReset
		if searching {
			sright = cGreen + "🔍 " + cReset + search + "▌"
		}
		gap := cols - visLen(title) - visLen(sright) - 2
		if gap < 1 { gap = 1 }
		sb.WriteString(at(1, 1) + title + strings.Repeat(" ", gap) + sright)

		if len(filtered) == 0 {
			sb.WriteString(at(3, 1) + cMuted + "  No cards match your search." + cReset)
			fmt.Fprint(out, sb.String())
			return
		}
		if cursor >= len(filtered) { cursor = len(filtered) - 1 }
		card := filtered[cursor]

		// Image sizing — portrait aspect, leaving room for text below
		ph := rows - 9
		if ph < 10 { ph = 10 }
		if ph > 40 { ph = 40 }
		pw := ph * 3 / 2
		if pw > cols-2 { pw = cols - 2 }

		fmt.Fprint(out, sb.String())

		// Sixel image at row 2
		fmt.Fprint(out, at(2, 1))
		renderSixel(out, card.SourcePath, pw, ph)

		// Text ALWAYS at an absolute row below the image
		textRow := ph + 3
		var tb strings.Builder
		tb.WriteString(at(textRow, 1))
		tb.WriteString(cCyan + cBold + card.Name() + cReset)
		tb.WriteString(fmt.Sprintf("%s  [%d/%d]%s", cMuted, cursor+1, len(filtered), cReset))
		if card.Data != nil && len(card.Data.Tags) > 0 {
			tb.WriteString("  " + cPurple + strings.Join(card.Data.Tags, " ") + cReset)
		}
		if card.Data != nil {
			desc := oneLine(card.Data.Description)
			desc = strings.ReplaceAll(strings.ReplaceAll(desc, "{{char}}", card.Name()), "{{user}}", "User")
			w := cols - 2
			lines := wrapLines(desc, w, 3)
			for i, ln := range lines {
				tb.WriteString(at(textRow+1+i, 1) + cMuted + ln + cReset)
			}
		}
		// Help bar at bottom
		help := fmt.Sprintf("%s←→↑↓%s flip  %senter%s open  %sc%s chat  %si%s import  %s/%s search  %sq%s back",
			cGreen, cReset, cGreen, cReset, cGreen, cReset, cGreen, cReset, cGreen, cReset, cGreen, cReset)
		tb.WriteString(at(rows, 1) + help)
		fmt.Fprint(out, tb.String())
	}

	applyFilter := func() {
		q := strings.ToLower(strings.TrimSpace(search))
		if q == "" {
			filtered = cards
		} else {
			var o []*cc.Card
			for _, c := range cards {
				hay := strings.ToLower(c.Name())
				if c.Data != nil {
					hay += " " + strings.ToLower(c.Data.Description) + " " + strings.ToLower(strings.Join(c.Data.Tags, " "))
				}
				if strings.Contains(hay, q) { o = append(o, c) }
			}
			filtered = o
		}
		cursor = 0
	}

	render()
	for {
		n, err := b.stdin.Read(buf)
		if err != nil || n == 0 { b.action = "back"; return nil }
		key := buf[:n]

		if searching {
			switch {
			case key[0] == 0x1b && n == 1: // esc
				searching = false; search = ""; applyFilter()
			case key[0] == '\r' || key[0] == '\n':
				searching = false
			case key[0] == 0x7f || key[0] == 8: // backspace
				if len(search) > 0 { search = search[:len(search)-1]; applyFilter() }
			case key[0] >= 32 && key[0] < 127:
				search += string(key[0]); applyFilter()
			}
			render()
			continue
		}

		// Arrow escape sequences
		if n >= 3 && key[0] == 0x1b && key[1] == '[' {
			switch key[2] {
			case 'C', 'B': // right/down → next
				if cursor < len(filtered)-1 { cursor++ }
			case 'D', 'A': // left/up → prev
				if cursor > 0 { cursor-- }
			}
			render()
			continue
		}

		switch key[0] {
		case 'q', 0x1b, 3: // q / esc / ctrl-c
			b.action = "back"; return nil
		case 'l', 'j':
			if cursor < len(filtered)-1 { cursor++ }
		case 'h', 'k':
			if cursor > 0 { cursor-- }
		case '/':
			searching = true; search = ""
		case '\r', '\n', 'e':
			if len(filtered) > 0 { b.action = "edit"; b.path = filtered[cursor].SourcePath; return nil }
		case 'c':
			if len(filtered) > 0 { b.action = "chat"; b.path = filtered[cursor].SourcePath; return nil }
		case 'i':
			if len(filtered) > 0 {
				_ = importCardToLibrary(b.user, filtered[cursor])
			}
		}
		render()
	}
}

// renderSixel runs chafa to emit a true-color sixel image at the cursor.
func renderSixel(out io.Writer, path string, w, h int) {
	format := "sixel"
	if GraphicsMode == "low" { format = "symbols" }
	args := []string{
		"--format", format,
		"--colors", "full",
		"--color-space", "rgb",
		"--size", fmt.Sprintf("%dx%d", w, h),
	}
	if format == "sixel" {
		args = append(args, "--dither", "ordered")
	}
	if os.Getenv("TMUX") != "" && format == "sixel" {
		args = append(args, "--passthrough", "tmux")
	}
	args = append(args, path)
	cmd := exec.Command("chafa", args...)
	cmd.Stdout = out
	cmd.Run()
}

// ── small text helpers (ANSI-aware-ish) ──────────────────────────────────────

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}

func visLen(s string) int {
	// rough visible length ignoring ANSI escapes
	n, inEsc := 0, false
	for _, r := range s {
		if r == 0x1b { inEsc = true; continue }
		if inEsc { if r == 'm' || r == 'H' { inEsc = false }; continue }
		n++
	}
	return n
}

func wrapLines(s string, width, maxLines int) []string {
	if width < 10 { width = 10 }
	var lines []string
	for len(s) > 0 && len(lines) < maxLines {
		if len(s) <= width { lines = append(lines, s); break }
		cut := width
		for cut > 0 && s[cut] != ' ' { cut-- }
		if cut == 0 { cut = width }
		lines = append(lines, s[:cut])
		s = strings.TrimLeft(s[cut:], " ")
	}
	if len(s) > 0 && len(lines) == maxLines {
		last := lines[maxLines-1]
		if len(last) > 1 { lines[maxLines-1] = last[:len(last)-1] + "…" }
	}
	return lines
}

// rawImagePreview shows one card image fullscreen in true-color sixel and waits
// for a keypress. Implements tea.ExecCommand so bubbletea releases the terminal.
type rawImagePreview struct {
	path   string
	name   string
	stdin  io.Reader
	stdout io.Writer
}

func (p *rawImagePreview) SetStdin(r io.Reader)  { p.stdin = r }
func (p *rawImagePreview) SetStdout(w io.Writer) { p.stdout = w }
func (p *rawImagePreview) SetStderr(w io.Writer) {}

func (p *rawImagePreview) Run() error {
	out := p.stdout
	if out == nil { out = os.Stdout }
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil { return nil }
	defer term.Restore(fd, oldState)
	fmt.Fprint(out, ansiHideCur)
	defer fmt.Fprint(out, ansiShowCur+cReset+ansiClear)

	cols, rows, _ := term.GetSize(fd)
	if cols <= 0 { cols = 100 }
	if rows <= 0 { rows = 40 }
	ph := rows - 3
	if ph < 10 { ph = 10 }
	pw := ph * 3 / 2
	if pw > cols-2 { pw = cols - 2 }

	fmt.Fprint(out, ansiClear+at(1, 1)+cCyan+cBold+"⬡ "+p.name+cReset)
	fmt.Fprint(out, at(2, 1))
	renderSixel(out, p.path, pw, ph)
	fmt.Fprint(out, at(rows, 1)+cMuted+"  press any key to return (auto-closes in 8s)"+cReset)

	// Read keys in the background so we can drain the buffered Enter, then
	// wait for a deliberate keypress OR an auto-close timeout.
	keyCh := make(chan struct{}, 4)
	go func() {
		b := make([]byte, 8)
		for {
			if _, err := p.stdin.Read(b); err != nil {
				return
			}
			keyCh <- struct{}{}
		}
	}()

	// Drain input buffered from the command submission (~400ms).
	drain := time.NewTimer(400 * time.Millisecond)
	draining := true
	for draining {
		select {
		case <-keyCh:
			// discard buffered keystroke
		case <-drain.C:
			draining = false
		}
	}
	// Now wait for a real keypress, or auto-close after 8s.
	select {
	case <-keyCh:
	case <-time.After(8 * time.Second):
	}
	return nil
}

// runImagePreviewCmd shows the fullscreen image, then returns to the same editor.
func runImagePreviewCmd(path, name string) tea.Cmd {
	p := &rawImagePreview{path: path, name: name}
	return tea.Exec(p, func(err error) tea.Msg { return imagePreviewClosedMsg{} })
}

type imagePreviewClosedMsg struct{}

// runRawBrowserCmd returns a tea.Cmd that runs the raw browser via tea.Exec,
// then dispatches based on the chosen action.
func runRawBrowserCmd(user *User) tea.Cmd {
	b := &rawCardBrowser{user: user}
	return tea.Exec(b, func(err error) tea.Msg {
		switch b.action {
		case "edit":
			return ShowCardEditorMsg{CardPath: b.path}
		case "chat":
			return ChatWithCardMsg{CardPath: b.path}
		default:
			return BackToMainAppMsg{}
		}
	})
}
