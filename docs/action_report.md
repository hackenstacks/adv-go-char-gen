# char-gen-cli-go - Action Report

**Project:** /home/user/Projects/char-gen-cli-go
**Created:** 2026-07-03

---


## char-gen-cli: raw sixel browser + Pollinations text/image + chat commands (2026-07-03 02:10)

**Status:** ✅ TESTED

**What:** Go character-generator TUI now browses cards in true-color sixel (raw/non-curses), chats with real AI (Pollinations), generates flux images, and has a full command set.

**Why:** Bubbletea can't render sixel (line-diffing can't measure pixel blobs). Needed true color + a real AI backend + image gen.

**How:**
- Raw browser (rawbrowser.go): tea.Exec releases the terminal, x/term raw mode, chafa sixel + absolute cursor positioning. rawImagePreview reused for editor ctrl+p and /image fullscreen.
- Pollinations unified API (gen.pollinations.ai): text POST /v1/chat/completions (Bearer key), image GET /image/{prompt}?key= (query param). Both need sk_ key now (anonymous=401).
- Chat commands: /image /exit /save /export /commit /memory /compact /prompt /models /help. Avatars beside responses (symbols).
- .env loader: POLLINATIONS_API_KEY auto-selects text+image; spots for MISTRAL/GEMINI/GROQ/OPENAI/ANTHROPIC/CUSTOM.
- Fixed: q-quits-chat bug, stdout fmt.Printf TUI corruption, editor ctrl+c→ctrl+t, grid width-0, card-chat mock, /image for card chat.

**Files:** rawbrowser.go, pollinations.go, openai_compat.go, services.go, chat.go, card_editor.go, card_browser.go, paths.go, main.go, messages.go, .env, .gitignore. Package: ~/git/nexus-charcard/.

**Testing:** Pollinations text (KEY OK), flux image (real 345KB JPEG of mouse conducting cats) confirmed. Browser sixel renders beautifully. Builds clean.

**Dependencies:** chafa, golang.org/x/term, Pollinations sk_ key in .env (gitignored).

---

