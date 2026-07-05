package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
	cc "github.com/hackenstacks/nexus-charcard"
)

// rawCardChat is a NON-curses chat screen. Like rawCardBrowser it implements
// tea.ExecCommand so bubbletea releases the terminal; we then take raw control
// and render a TRUE-COLOR sixel avatar (via renderSixel) with the conversation
// and input placed by absolute cursor moves. This is the only way to show a
// crisp sixel image alongside live text — the whole reason the browser went raw.
type rawCardChat struct {
	user     *User
	cardPath string
	stdin    io.Reader
	stdout   io.Writer

	action string // "back"
}

func (c *rawCardChat) SetStdin(r io.Reader)  { c.stdin = r }
func (c *rawCardChat) SetStdout(w io.Writer) { c.stdout = w }
func (c *rawCardChat) SetStderr(w io.Writer) {}

const ansiClrLine = "\033[2K"

// chatEvent is delivered from background goroutines (LLM / image work) to the
// main input loop so the UI never blocks while waiting on the network.
type chatEvent struct {
	kind      string // "reply" | "error" | "image" | "sys"
	msg       Message
	imgPath   string
	imgPrompt string
}

// spinnerFrames animates the "typing…" indicator.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// sharedAttachment is a Library file the user has shared into the conversation.
// Text-like docs inject their text into the prompt; images are sent to the model
// as real pixels when it supports vision, otherwise referenced by name/caption.
type sharedAttachment struct {
	name  string
	ftype string // library type: text | json | chat_log | image | …
	text  string // populated for text-like docs
	image *ImageAttachment
}

// looksTextual heuristically decides whether raw bytes are safe to inject as text
// (valid-ish UTF-8, few control bytes) — used for docs whose type is unknown.
func looksTextual(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	n := len(b)
	if n > 512 {
		n = 512
	}
	ctrl := 0
	for _, c := range b[:n] {
		if c == 0 {
			return false
		}
		if c < 9 || (c > 13 && c < 32) {
			ctrl++
		}
	}
	return ctrl*20 < n // <5% control bytes
}

// imageMediaType guesses a media type from a filename/library name.
func imageMediaType(name string) string {
	switch {
	case strings.HasSuffix(strings.ToLower(name), ".png"):
		return "image/png"
	case strings.HasSuffix(strings.ToLower(name), ".webp"):
		return "image/webp"
	case strings.HasSuffix(strings.ToLower(name), ".gif"):
		return "image/gif"
	default:
		return "image/jpeg"
	}
}

func (c *rawCardChat) Run() error {
	out := c.stdout
	if out == nil {
		out = os.Stdout
	}

	// ── Load config, services, and the card persona ──────────────────────────
	apiCfg, _ := LoadAPIConfig(c.user)
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

	provider := apiCfg.SelectedLLMProvider
	model := resolveDefaultModel(&apiCfg)
	imgModel := string(apiCfg.Pollinations.DefaultImageModel)
	if imgModel == "" {
		imgModel = "flux"
	}

	// Can this provider+model actually see shared images? If not, images are
	// referenced by title/caption in the prompt instead.
	_, hasVision := llmSvc.(VisionLLMService)
	visionCapable := hasVision && modelSupportsVision(model)

	card, err := cc.LoadFromPNG(c.cardPath)
	if err != nil || card == nil {
		c.action = "back"
		return nil
	}
	cardName := card.Name()
	sysPrompt := card.SystemPrompt(c.user.Username)
	memID := "card:" + cardName

	var messages []Message
	pushSys := func(s string) {
		messages = append(messages, Message{ID: GenerateRandomID(), Sender: "System",
			Content: s, Timestamp: time.Now().Unix(), Type: MessageTypeSystem})
	}
	if provider == ProviderMock || provider == "" {
		pushSys("⚠ No AI provider configured — replies are mock text. Set a provider in .env (e.g. POLLINATIONS_API_KEY or CUSTOM_API_URL).")
	}
	pushSys(fmt.Sprintf("Chatting with %s via %s (%s) — /help for commands, /exit to leave.", cardName, provider, model))
	if card.Data != nil && strings.TrimSpace(card.Data.FirstMes) != "" {
		greeting := strings.ReplaceAll(strings.ReplaceAll(card.Data.FirstMes, "{{char}}", cardName), "{{user}}", c.user.Username)
		messages = append(messages, Message{ID: GenerateRandomID(), Sender: cardName,
			Content: greeting, Timestamp: time.Now().Unix(), Type: MessageTypeCharacter})
	}

	// ── Raw terminal ─────────────────────────────────────────────────────────
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		c.action = "back"
		return nil
	}
	defer term.Restore(fd, oldState)
	fmt.Fprint(out, ansiHideCur)
	defer fmt.Fprint(out, ansiShowCur+cReset+ansiClear)

	// ── Runtime state ────────────────────────────────────────────────────────
	input := ""
	scroll := 0
	userScrolled := false
	busy := false
	spin := 0

	// Generated/shared images accumulate as a filmstrip across the top row, to
	// the RIGHT of the avatar — each on its own rows, positioned absolutely, and
	// retained (the most recent that fit are shown).
	var strip []string // image file paths, oldest→newest

	// Library wiring: conversations and images are saved into the encrypted
	// Library (del/share/export available there). libChatID holds this session's
	// single conversation entry so repeated saves update in place instead of
	// piling up duplicates.
	libChatID := ""
	canLibrary := c.user != nil && c.user.EncryptionKey != nil

	// Files the user has shared into this conversation from the Library, plus the
	// numbered listing shown by /files so /share <n> can resolve a pick.
	var shared []sharedAttachment
	var libPick []LibraryFile
	// saveConvToLibrary snapshots the current conversation into the Library.
	saveConvToLibrary := func() error {
		if !canLibrary || len(messages) == 0 {
			return nil
		}
		data, err := json.MarshalIndent(messages, "", "  ")
		if err != nil {
			return err
		}
		if libChatID == "" {
			name := fmt.Sprintf("Chat · %s · %s", cardName, time.Now().Format("2006-01-02 15:04"))
			id, err := AddDataToLibrary(c.user, name, "chat_log", data, "")
			if err != nil {
				return err
			}
			libChatID = id
			return nil
		}
		return UpdateLibraryFileContent(c.user, libChatID, data)
	}
	// addImageToLibrary files a generated/shared image into the Library.
	addImageToLibrary := func(imgPath, prompt string) {
		if !canLibrary || imgPath == "" {
			return
		}
		name := strings.TrimSpace(strings.TrimPrefix(prompt, "🎬 "))
		if len(name) > 60 {
			name = name[:60]
		}
		if name == "" {
			name = "Image · " + cardName
		}
		AddFileToLibrary(c.user, imgPath, name, "image")
	}

	// Scene-image auto-illustration: every 3 character rounds the AI paints the
	// current scene beside the avatar. Toggle with /autoimage.
	roundCount := 0
	autoImage := true
	isMock := provider == ProviderMock || provider == ""

	// Ensure every conversation with at least one exchange lands in the Library,
	// no matter how the chat is left (/exit, esc, ctrl+c). Best-effort.
	defer func() {
		if roundCount > 0 {
			saveConvToLibrary()
		}
	}()

	// Layout, recomputed on resize.
	var cols, rows, avH, avW, convTop, convBottom, barRow, inputRow int
	layout := func() {
		cols, rows, _ = term.GetSize(fd)
		if cols <= 0 {
			cols = 100
		}
		if rows <= 0 {
			rows = 40
		}
		avH = rows / 3
		if avH < 8 {
			avH = 8
		}
		if avH > 14 {
			avH = 14
		}
		avW = avH * 3 / 2
		if avW > cols-2 {
			avW = cols - 2
		}
		convTop = 3 + avH   // header(1) + avatar(avH) + separator(1), then conv
		inputRow = rows
		barRow = rows - 1
		convBottom = rows - 2
		if convBottom < convTop {
			convBottom = convTop
		}
	}
	layout()

	// drawFilmstrip paints the retained images across the top row, right of the
	// avatar. Each thumbnail is emitted at an absolute cursor position (the only
	// reliable way to place multiple sixels), most-recent last, filling the row.
	drawFilmstrip := func() {
		if len(strip) == 0 {
			return
		}
		startCol := avW + 3
		avail := cols - startCol
		if avail < 14 {
			return // no room beside the avatar
		}
		thumbW := 2 * avH // square images need ~2 cols per row of height
		if thumbW < 12 {
			thumbW = 12
		}
		per := thumbW + 1
		n := avail / per
		if n < 1 {
			n = 1
		}
		items := strip
		if len(items) > n {
			items = items[len(items)-n:] // most recent that fit
		}
		col := startCol
		for _, p := range items {
			fmt.Fprint(out, at(2, col))
			renderSixel(out, p, thumbW, avH)
			col += per
		}
		fmt.Fprint(out, at(1+avH, startCol)+cPurple+"🖼 "+cReset+
			cMuted+fmt.Sprintf("%d image(s) this session", len(strip))+cReset)
	}

	convWidth := func() int {
		w := cols - 2
		if w < 20 {
			w = 20
		}
		return w
	}

	// buildDisplayLines flattens the conversation into wrapped, colored lines.
	buildDisplayLines := func() []string {
		w := convWidth()
		var lines []string
		for _, msg := range messages {
			var label string
			switch msg.Type {
			case MessageTypeUser:
				label = cCyan + cBold + "You" + cReset
			case MessageTypeCharacter:
				label = cGreen + cBold + "⬡ " + msg.Sender + cReset
			case MessageTypeSummary:
				label = cPurple + cBold + "▤ Summary" + cReset
			default:
				label = cMuted + msg.Sender + cReset
			}
			lines = append(lines, label)
			bodyColor := cReset
			if msg.Type == MessageTypeSystem {
				bodyColor = cMuted
			}
			for _, bl := range wrapAll(msg.Content, w) {
				lines = append(lines, bodyColor+bl+cReset)
			}
			lines = append(lines, "")
		}
		return lines
	}

	// redrawConv paints only the conversation region (never the sixel rows), so
	// typing and new messages don't cause the avatar to flicker.
	redrawConv := func() {
		lines := buildDisplayLines()
		convH := convBottom - convTop + 1
		maxScroll := len(lines) - convH
		if maxScroll < 0 {
			maxScroll = 0
		}
		if !userScrolled {
			scroll = maxScroll
		}
		if scroll > maxScroll {
			scroll = maxScroll
		}
		if scroll < 0 {
			scroll = 0
		}
		var sb strings.Builder
		for i := 0; i < convH; i++ {
			row := convTop + i
			sb.WriteString(at(row, 1) + ansiClrLine)
			idx := scroll + i
			if idx < len(lines) {
				sb.WriteString(lines[idx])
			}
		}
		// Scroll hint when more is above/below.
		if maxScroll > 0 {
			pos := fmt.Sprintf("%s↑↓ %d/%d%s", cMuted, scroll, maxScroll, cReset)
			sb.WriteString(at(convTop, cols-visLen(pos)) + pos)
		}
		fmt.Fprint(out, sb.String())
	}

	// redrawInput paints the quick-launch bar + the reply box.
	redrawInput := func() {
		var sb strings.Builder
		// Quick-launch bar (or status while busy).
		sb.WriteString(at(barRow, 1) + ansiClrLine)
		if busy {
			sb.WriteString(fmt.Sprintf("%s%s %s is thinking…%s", cGreen, spinnerFrames[spin%len(spinnerFrames)], cardName, cReset))
		} else {
			qk := func(k, label string) string {
				return cGreen + k + cReset + cMuted + " " + label + cReset
			}
			bar := strings.Join([]string{
				qk("^G", "/image"), qk("^O", "/memory"), qk("^K", "/compact"),
				qk("^E", "/export"), qk("^S", "/save"), qk("^X", "exit"),
			}, cMuted+"  ·  "+cReset)
			sb.WriteString(bar)
		}
		// Reply box.
		sb.WriteString(at(inputRow, 1) + ansiClrLine)
		prompt := cCyan + cBold + ">> " + cReset
		shown := input
		// Keep the tail visible if the line is longer than the terminal.
		maxIn := cols - 5
		if len(shown) > maxIn && maxIn > 0 {
			shown = "…" + shown[len(shown)-maxIn+1:]
		}
		sb.WriteString(prompt + shown + cGreen + "▌" + cReset)
		fmt.Fprint(out, sb.String())
	}

	fullDraw := func() {
		layout()
		var sb strings.Builder
		sb.WriteString(ansiClear)
		// Header line.
		header := fmt.Sprintf("%s%s⬡ %s%s  %s%s · %s%s",
			cCyan, cBold, cardName, cReset, cMuted, provider, model, cReset)
		sb.WriteString(at(1, 1) + header)
		// Separator under the avatar.
		sb.WriteString(at(2+avH, 1) + cMuted + strings.Repeat("─", cols) + cReset)
		fmt.Fprint(out, sb.String())
		// True-color sixel avatar (its own rows — text never shares them).
		fmt.Fprint(out, at(2, 1))
		renderSixel(out, c.cardPath, avW, avH)
		drawFilmstrip()
		redrawConv()
		redrawInput()
	}

	// buildPrompt assembles the LLM prompt from persona + recent history.
	buildPrompt := func() string {
		var b strings.Builder
		b.WriteString(sysPrompt)
		b.WriteString("\n\nYou are ")
		b.WriteString(cardName)
		b.WriteString(". Stay in character. Respond only as ")
		b.WriteString(cardName)
		b.WriteString(".\n\n")
		// Inject any Library files the user shared into this conversation.
		if len(shared) > 0 {
			b.WriteString("--- Shared materials (provided by " + c.user.Username + ") ---\n")
			for _, sh := range shared {
				switch {
				case sh.text != "":
					txt := sh.text
					if len(txt) > 4000 {
						txt = txt[:4000] + "\n…(truncated)"
					}
					b.WriteString(fmt.Sprintf("Document %q:\n%s\n\n", sh.name, txt))
				case sh.image != nil && visionCapable:
					b.WriteString(fmt.Sprintf("[Image %q is attached to this message — look at it directly.]\n", sh.name))
				case sh.image != nil:
					b.WriteString(fmt.Sprintf("[The user shared an image titled %q. You cannot see it directly; use the title as its caption.]\n", sh.name))
				}
			}
			b.WriteString("\n")
		}
		b.WriteString("--- Conversation ---\n")
		start := 0
		if len(messages) > 16 {
			start = len(messages) - 16
		}
		for i := start; i < len(messages); i++ {
			mm := messages[i]
			if mm.Type == MessageTypeSystem || mm.Type == MessageTypeCommand {
				continue
			}
			b.WriteString(mm.Sender + ": " + mm.Content + "\n")
		}
		b.WriteString(cardName + ": ")
		return b.String()
	}

	events := make(chan chatEvent, 4)

	// sendToLLM fires the reply request in the background.
	sendToLLM := func() {
		prompt := buildPrompt()
		// Gather shared images for a vision-capable model (snapshot for the goroutine).
		var imgs []ImageAttachment
		if visionCapable {
			for _, sh := range shared {
				if sh.image != nil {
					imgs = append(imgs, *sh.image)
				}
			}
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			var resp string
			var err error
			if len(imgs) > 0 {
				if vsvc, ok := llmSvc.(VisionLLMService); ok {
					resp, err = vsvc.GenerateResponseWithImages(ctx, prompt, imgs, LLMModel(model), apiCfg)
				} else {
					resp, err = llmSvc.GenerateResponse(ctx, prompt, LLMModel(model), apiCfg)
				}
			} else {
				resp, err = llmSvc.GenerateResponse(ctx, prompt, LLMModel(model), apiCfg)
			}
			if err != nil {
				events <- chatEvent{kind: "error", msg: Message{Content: err.Error()}}
				return
			}
			events <- chatEvent{kind: "reply", msg: Message{ID: GenerateRandomID(), Sender: cardName,
				Content: strings.TrimSpace(resp), Timestamp: time.Now().Unix(), Type: MessageTypeCharacter}}
		}()
	}

	// saveImageData persists a GenerateImage result (URL or base64) to a local
	// file and returns the path.
	saveImageData := func(ctx context.Context, data string) (string, error) {
		dir := filepath.Join(Paths.DataDir, "images")
		os.MkdirAll(dir, 0755)
		path := filepath.Join(dir, fmt.Sprintf("img-%d.jpg", time.Now().UnixNano()))
		if strings.HasPrefix(data, "http") {
			if err := downloadImage(ctx, data, path); err != nil {
				return "", err
			}
		} else {
			raw, err := base64.StdEncoding.DecodeString(data)
			if err != nil {
				return "", err
			}
			if err := os.WriteFile(path, raw, 0644); err != nil {
				return "", err
			}
		}
		return path, nil
	}

	// generateImage fires image generation from an explicit prompt in the background.
	generateImage := func(prompt string) {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			data, err := imgSvc.GenerateImage(ctx, prompt, ImageModel(imgModel))
			if err != nil {
				events <- chatEvent{kind: "error", msg: Message{Content: "image generation failed: " + err.Error()}}
				return
			}
			path, err := saveImageData(ctx, data)
			if err != nil {
				events <- chatEvent{kind: "error", msg: Message{Content: "save image: " + err.Error()}}
				return
			}
			events <- chatEvent{kind: "image", imgPath: path, imgPrompt: prompt}
		}()
	}

	// generateSceneImage asks the LLM to distill the last few messages into a
	// vivid image prompt, then generates a picture of the current scene — this is
	// how the character illustrates the story (via /image with no prompt, and
	// automatically every few rounds).
	generateSceneImage := func() {
		snapshot := make([]Message, len(messages))
		copy(snapshot, messages)
		go func() {
			// Collect the last 3 conversational lines.
			var recent []Message
			for i := len(snapshot) - 1; i >= 0 && len(recent) < 3; i-- {
				switch snapshot[i].Type {
				case MessageTypeSystem, MessageTypeCommand, MessageTypeSummary:
					continue
				}
				recent = append([]Message{snapshot[i]}, recent...)
			}
			var b strings.Builder
			b.WriteString("You are an art director. From the recent roleplay lines below, write ONE vivid image-generation prompt depicting the CURRENT scene — setting, characters, action, mood, lighting, and art style. Output ONLY the prompt on a single line, no preamble, no quotes.\n\n")
			for _, mm := range recent {
				b.WriteString(mm.Sender + ": " + mm.Content + "\n")
			}
			sctx, scancel := context.WithTimeout(context.Background(), 60*time.Second)
			scenePrompt, err := llmSvc.GenerateResponse(sctx, b.String(), LLMModel(model), apiCfg)
			scancel()
			if err != nil {
				events <- chatEvent{kind: "error", msg: Message{Content: "scene summary failed: " + err.Error()}}
				return
			}
			scenePrompt = strings.Trim(strings.TrimSpace(scenePrompt), "\"'`")
			if scenePrompt == "" {
				events <- chatEvent{kind: "error", msg: Message{Content: "scene summary was empty"}}
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			data, err := imgSvc.GenerateImage(ctx, scenePrompt, ImageModel(imgModel))
			if err != nil {
				events <- chatEvent{kind: "error", msg: Message{Content: "scene image failed: " + err.Error()}}
				return
			}
			path, err := saveImageData(ctx, data)
			if err != nil {
				events <- chatEvent{kind: "error", msg: Message{Content: "save image: " + err.Error()}}
				return
			}
			events <- chatEvent{kind: "image", imgPath: path, imgPrompt: "🎬 " + scenePrompt}
		}()
	}

	// handleCommand runs a slash command. Returns true if the chat should exit.
	handleCommand := func(cmd, args string) bool {
		switch cmd {
		case "exit", "quit", "q":
			c.action = "back"
			return true
		case "help":
			pushSys("Commands: /image <prompt> (blank = illustrate current scene) · /autoimage on|off · " +
				"/files · /share <n> · /shared · /unshare [all|n] · " +
				"/memory · /commit <fact> · /compact · /prompt <text> · /save · /export · /models · /exit\n" +
				"Shortcuts: ^G image  ^F files  ^O memory  ^K compact  ^E export  ^S save  ^X exit  ·  ↑/↓ or PgUp/PgDn scroll\n" +
				"Share Library files with the character: /files to list, /share <n> to give one. Docs inject text; " +
				"images are seen directly on vision models (openai/gemini/claude) or referenced by caption otherwise. " +
				"The character auto-illustrates the scene every 3 rounds (toggle /autoimage).")
		case "prompt":
			if strings.TrimSpace(args) == "" {
				pushSys("Usage: /prompt <direction> — steer upcoming replies.")
			} else {
				sysPrompt += "\n\n[Director's note: " + args + "]"
				pushSys("📝 Directive added.")
			}
		case "commit":
			if strings.TrimSpace(args) == "" {
				pushSys("Usage: /commit <fact>")
			} else {
				entry := &MemoryEntry{Content: args, Type: "semantic", Source: "user /commit", Keywords: []string{cardName}}
				if err := mm.AddMemory(c.user, memID, entry, &apiCfg); err != nil {
					pushSys("Memory error: " + err.Error())
				} else {
					pushSys("🧠 Committed: " + args)
				}
			}
		case "memory":
			mems, err := mm.RetrieveMemories(c.user, memID, "", 50, &apiCfg)
			if err != nil {
				pushSys("Memory error: " + err.Error())
			} else if len(mems) == 0 {
				pushSys("No memories for " + cardName + " yet. Use /commit <fact>.")
			} else {
				var b strings.Builder
				b.WriteString(fmt.Sprintf("🧠 %s's memory (%d):", cardName, len(mems)))
				for i, mem := range mems {
					if i >= 20 {
						break
					}
					b.WriteString("\n  • " + mem.Content)
				}
				pushSys(b.String())
			}
		case "files", "library", "lib":
			if !canLibrary {
				pushSys("Library unavailable (not logged in).")
				break
			}
			files, ferr := ListLibraryFiles(c.user)
			if ferr != nil {
				pushSys("Library error: " + ferr.Error())
				break
			}
			if len(files) == 0 {
				pushSys("Your Library is empty. Generate images or /save a chat first.")
				break
			}
			sort.Slice(files, func(i, j int) bool { return files[i].Timestamp.After(files[j].Timestamp) })
			libPick = files
			var b strings.Builder
			b.WriteString(fmt.Sprintf("📚 Library (%d) — /share <n> to give one to %s:", len(files), cardName))
			for i, f := range files {
				if i >= 20 {
					b.WriteString("\n  …")
					break
				}
				b.WriteString(fmt.Sprintf("\n  %d. [%s] %s", i+1, f.Type, f.Name))
			}
			pushSys(b.String())
		case "share":
			if !canLibrary {
				pushSys("Library unavailable (not logged in).")
				break
			}
			idxStr := strings.TrimSpace(args)
			if idxStr == "" {
				pushSys("Usage: /share <n> — run /files first, then share Library file n with " + cardName + ".")
				break
			}
			n, perr := strconv.Atoi(idxStr)
			if perr != nil || n < 1 || n > len(libPick) {
				pushSys(fmt.Sprintf("Pick a number from /files (1–%d).", len(libPick)))
				break
			}
			lf := libPick[n-1]
			_, content, lerr := LoadFileFromLibrary(c.user, lf.ID)
			if lerr != nil {
				pushSys("Load failed: " + lerr.Error())
				break
			}
			att := sharedAttachment{name: lf.Name, ftype: lf.Type}
			isImage := lf.Type == "image" ||
				(len(content) > 3 && ((content[0] == 0xFF && content[1] == 0xD8) || (content[0] == 0x89 && content[1] == 'P')))
			switch {
			case isImage:
				att.image = &ImageAttachment{Data: content, MediaType: imageMediaType(lf.Name)}
				shared = append(shared, att)
				if visionCapable {
					pushSys(fmt.Sprintf("🖼  Shared image %q — %s can see it (vision model %s).", lf.Name, cardName, model))
				} else {
					pushSys(fmt.Sprintf("🖼  Shared image %q — this model can't see images, so %s receives it as a caption. Set a vision model (openai/gemini/claude) in .env for true sight.", lf.Name, cardName))
				}
			case lf.Type == "text" || lf.Type == "json" || lf.Type == "chat_log" || looksTextual(content):
				att.text = string(content)
				shared = append(shared, att)
				pushSys(fmt.Sprintf("📄 Shared document %q (%d chars) — its text is now in %s's context.", lf.Name, len(att.text), cardName))
			default:
				shared = append(shared, att) // referenced by name only
				pushSys(fmt.Sprintf("📎 Shared %q by name (binary %s can't be inlined).", lf.Name, lf.Type))
			}
		case "shared":
			if len(shared) == 0 {
				pushSys("No files shared yet. Use /files then /share <n>.")
				break
			}
			var b strings.Builder
			b.WriteString(fmt.Sprintf("📎 Shared with %s (%d):", cardName, len(shared)))
			for i, sh := range shared {
				kind := sh.ftype
				if sh.image != nil {
					if visionCapable {
						kind = "image→vision"
					} else {
						kind = "image→caption"
					}
				}
				b.WriteString(fmt.Sprintf("\n  %d. [%s] %s", i+1, kind, sh.name))
			}
			pushSys(b.String())
		case "unshare":
			a := strings.TrimSpace(args)
			if a == "" || a == "all" {
				shared = nil
				pushSys("Cleared all shared files.")
				break
			}
			n, perr := strconv.Atoi(a)
			if perr != nil || n < 1 || n > len(shared) {
				pushSys("Usage: /unshare [all|<n>]")
				break
			}
			name := shared[n-1].name
			shared = append(shared[:n-1], shared[n:]...)
			pushSys("Removed shared file: " + name)
		case "save":
			if err := SaveChatSession(c.user, memID, messages); err != nil {
				pushSys("Save failed: " + err.Error())
			} else if err := saveConvToLibrary(); err != nil {
				pushSys("💾 Session saved (Library snapshot failed: " + err.Error() + ")")
			} else {
				pushSys("💾 Conversation saved — also in your Library (del/share/export there).")
			}
		case "export":
			dir := filepath.Join(Paths.DataDir, "exports")
			os.MkdirAll(dir, 0755)
			name := strings.ToLower(strings.ReplaceAll(cardName, " ", "-"))
			if name == "" {
				name = "chat"
			}
			path := filepath.Join(dir, fmt.Sprintf("%s-%d.json", name, time.Now().Unix()))
			data, _ := json.MarshalIndent(messages, "", "  ")
			if err := os.WriteFile(path, data, 0644); err != nil {
				pushSys("Export failed: " + err.Error())
			} else {
				pushSys("📤 Exported → " + path)
			}
		case "models":
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			mdls, err := llmSvc.GetAvailableModels(ctx)
			cancel()
			if err != nil {
				pushSys("Models error: " + err.Error())
			} else {
				names := make([]string, len(mdls))
				for i, md := range mdls {
					names[i] = string(md)
				}
				pushSys("Available models: " + strings.Join(names, ", "))
			}
		case "compact":
			busy = true
			redrawInput()
			snapshot := make([]Message, len(messages))
			copy(snapshot, messages)
			go func() {
				var b strings.Builder
				b.WriteString("Summarize the following roleplay conversation concisely, preserving key events, facts, and the current situation. Write it as a recap.\n\n")
				for _, mm := range snapshot {
					if mm.Type == MessageTypeSystem || mm.Type == MessageTypeCommand {
						continue
					}
					b.WriteString(mm.Sender + ": " + mm.Content + "\n")
				}
				ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
				defer cancel()
				summary, err := llmSvc.GenerateResponse(ctx, b.String(), LLMModel(model), apiCfg)
				if err != nil {
					events <- chatEvent{kind: "error", msg: Message{Content: "compact failed: " + err.Error()}}
					return
				}
				events <- chatEvent{kind: "sys", msg: Message{ID: GenerateRandomID(), Sender: "Summary",
					Content: strings.TrimSpace(summary), Timestamp: time.Now().Unix(), Type: MessageTypeSummary}}
			}()
		case "image":
			if isMock {
				pushSys("Image generation needs a real provider (set POLLINATIONS_API_KEY in .env).")
			} else if strings.TrimSpace(args) == "" {
				// No prompt → illustrate the current scene from the last 3 messages.
				busy = true
				pushSys("🎬 Picturing the current scene…")
				generateSceneImage()
			} else {
				busy = true
				pushSys("🎨 Generating image: " + args)
				generateImage(args)
			}
		case "autoimage", "auto":
			switch strings.ToLower(strings.TrimSpace(args)) {
			case "off", "0", "false", "no":
				autoImage = false
				pushSys("🎬 Auto scene-images OFF.")
			default:
				autoImage = true
				pushSys("🎬 Auto scene-images ON — the character illustrates the scene every 3 rounds.")
			}
		default:
			pushSys("Unknown command /" + cmd + " — try /help.")
	}
		return false
	}

	// ── Background key reader (select-able) ──────────────────────────────────
	keyCh := make(chan []byte, 16)
	go func() {
		b := make([]byte, 16)
		for {
			n, err := c.stdin.Read(b)
			if err != nil || n == 0 {
				close(keyCh)
				return
			}
			cp := make([]byte, n)
			copy(cp, b[:n])
			keyCh <- cp
		}
	}()

	// Resize via SIGWINCH.
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)

	spinner := time.NewTicker(120 * time.Millisecond)
	defer spinner.Stop()

	submit := func() bool {
		line := strings.TrimSpace(input)
		input = ""
		if line == "" {
			return false
		}
		if strings.HasPrefix(line, "/") {
			parts := strings.SplitN(strings.TrimPrefix(line, "/"), " ", 2)
			cmd := strings.ToLower(parts[0])
			args := ""
			if len(parts) > 1 {
				args = parts[1]
			}
			return handleCommand(cmd, args)
		}
		// Ordinary message → user turn + async reply.
		messages = append(messages, Message{ID: GenerateRandomID(), Sender: c.user.Username,
			Content: line, Timestamp: time.Now().Unix(), Type: MessageTypeUser})
		userScrolled = false
		busy = true
		sendToLLM()
		return false
	}

	fullDraw()

	for {
		select {
		case <-winch:
			fullDraw()

		case <-spinner.C:
			if busy {
				spin++
				redrawInput()
			}

		case ev, ok := <-events:
			if !ok {
				c.action = "back"
				return nil
			}
			switch ev.kind {
			case "reply":
				busy = false
				messages = append(messages, ev.msg)
				userScrolled = false
				// The character illustrates the scene every 3rd reply.
				roundCount++
				if autoImage && !isMock && roundCount%3 == 0 {
					busy = true
					pushSys("🎬 " + cardName + " pictures the scene…")
					generateSceneImage()
				}
			case "sys":
				busy = false
				messages = append(messages, ev.msg)
				userScrolled = false
			case "error":
				busy = false
				pushSys("⚠ " + ev.msg.Content)
				userScrolled = false
			case "image":
				busy = false
				strip = append(strip, ev.imgPath)
				addImageToLibrary(ev.imgPath, ev.imgPrompt)
				pushSys("🖼  Image saved to your Library (del/share/export there) — added to the top row.")
				userScrolled = false
				// Full clean redraw so the sixel strip renders reliably.
				fullDraw()
				continue
			}
			redrawConv()
			redrawInput()

		case key, ok := <-keyCh:
			if !ok {
				c.action = "back"
				return nil
			}
			n := len(key)
			// Arrow / page keys.
			if n >= 3 && key[0] == 0x1b && key[1] == '[' {
				switch key[2] {
				case 'A': // up
					if scroll > 0 {
						scroll--
						userScrolled = true
					}
				case 'B': // down
					scroll++
					userScrolled = true
				case '5': // PgUp
					scroll -= (convBottom - convTop)
					if scroll < 0 {
						scroll = 0
					}
					userScrolled = true
				case '6': // PgDn
					scroll += (convBottom - convTop)
					userScrolled = true
				}
				redrawConv()
				continue
			}
			b0 := key[0]
			switch {
			case b0 == 0x1b && n == 1: // Esc → exit
				c.action = "back"
				return nil
			case b0 == 3, b0 == 24: // Ctrl+C / Ctrl+X → exit
				c.action = "back"
				return nil
			case b0 == '\r' || b0 == '\n':
				if submit() {
					return nil
				}
				redrawConv()
				redrawInput()
			case b0 == 0x7f || b0 == 8: // Backspace
				if len(input) > 0 {
					input = input[:len(input)-1]
				}
				redrawInput()
			case b0 == 7: // Ctrl+G → prefill /image
				input = "/image "
				redrawInput()
			case b0 == 6: // Ctrl+F → list Library files to share
				handleCommand("files", "")
				redrawConv()
				redrawInput()
			case b0 == 15: // Ctrl+O → /memory
				handleCommand("memory", "")
				redrawConv()
				redrawInput()
			case b0 == 11: // Ctrl+K → /compact
				handleCommand("compact", "")
				redrawConv()
				redrawInput()
			case b0 == 5: // Ctrl+E → /export
				handleCommand("export", "")
				redrawConv()
				redrawInput()
			case b0 == 19: // Ctrl+S → /save
				handleCommand("save", "")
				redrawConv()
				redrawInput()
			case b0 >= 32 && b0 < 127: // printable (single byte)
				if n == 1 {
					input += string(rune(b0))
				} else {
					// paste / multibyte: append printable bytes
					for _, bb := range key {
						if bb >= 32 && bb < 127 {
							input += string(rune(bb))
						}
					}
				}
				redrawInput()
			}
		}
	}
}

// wrapAll word-wraps text to width, honoring explicit newlines. Unlike wrapLines
// it returns every line (no cap) so the whole conversation can scroll.
func wrapAll(s string, width int) []string {
	if width < 4 {
		width = 4
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if strings.TrimSpace(para) == "" {
			out = append(out, "")
			continue
		}
		line := ""
		for _, w := range strings.Fields(para) {
			// Break a single over-long word.
			for len(w) > width {
				if line != "" {
					out = append(out, line)
					line = ""
				}
				out = append(out, w[:width])
				w = w[width:]
			}
			switch {
			case line == "":
				line = w
			case len(line)+1+len(w) <= width:
				line += " " + w
			default:
				out = append(out, line)
				line = w
			}
		}
		out = append(out, line)
	}
	return out
}

// runRawChatCmd runs the raw sixel chat via tea.Exec, then returns to the browser.
func runRawChatCmd(user *User, cardPath string) tea.Cmd {
	c := &rawCardChat{user: user, cardPath: cardPath}
	return tea.Exec(c, func(err error) tea.Msg {
		return ShowCardBrowserMsg{}
	})
}
