<div align="center">

# ⬡ adv-go-char-gen

### Your characters, in true color — right in the terminal.

**Browse · Edit · Chat · Illustrate · Keep — a single Go binary that renders PNG character cards in real *sixel* true color, chats with them through live AI, and paints the scene as you go.**

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![TUI](https://img.shields.io/badge/TUI-Bubble%20Tea-ff69b4)](https://github.com/charmbracelet/bubbletea)
[![Graphics](https://img.shields.io/badge/graphics-true--color%20sixel-7c3aed)](https://en.wikipedia.org/wiki/Sixel)
[![Encrypted](https://img.shields.io/badge/library-AES--256--GCM-00ff88)](#-the-library)
[![Part of NeXuS](https://img.shields.io/badge/stack-NeXuS-DAA520)](#-the-nexus-stack)

**Author: [hackenstacks](https://github.com/hackenstacks)** · [hackenstacks@gmail.com](mailto:hackenstacks@gmail.com)

</div>

---

## ✨ Why this exists

Most character-card tools are web apps. This one lives where you already are — the terminal — and
refuses to compromise on image quality. The card browser and the chat are **raw-terminal screens** that
emit genuine **sixel** graphics, so a portrait looks like a *photo*, not a mosaic of blocks. You talk to
a character with a real LLM, and every few turns it **illustrates the current scene** beside its avatar.
Everything you make is quietly filed into a **local, encrypted library** you fully own.

> **Sane · Simple · Secure · Stealthy · Beautiful** — part of the [NeXuS](#-the-nexus-stack) stack.

---

## 🎬 What it does

| | |
|---|---|
| 🖼️ **True-color sixel browser** | Scroll a wall of PNG character cards rendered in full-quality sixel — not ASCII, not half-blocks. |
| ✏️ **PNG card editor** | Edit all 12 card fields (name, description, personality, scenario, system prompt, first message…) and save straight back into the PNG. Opens with a crisp true-color portrait. |
| 💬 **Live AI chat** | Talk to any card through a real LLM. The avatar is drawn in sixel; the conversation scrolls beside it, flicker-free. |
| 🎨 **Scene illustration** | The character paints the current scene every 3 rounds — or on demand with `/image`. Images retain as a filmstrip across the top row. |
| 📚 **Encrypted library** | Conversations and images auto-save into an AES-256-GCM store. **Delete · Share · Export** any entry. 100% local — no cloud, no IPFS, no telemetry. |
| 🧠 **Per-character memory** | Commit facts a character should remember; conversations are summarized to hold context. |
| 🔌 **Multi-provider** | Pollinations, OpenAI, Mistral, Gemini, Groq, Anthropic, AI Horde, or any custom OpenAI-compatible endpoint — chosen by a simple `.env`. |

---

## 🚀 Quick start

### Requirements

- **Go 1.26+**
- **[chafa](https://hpjansson.org/chafa/)** — does the image→sixel conversion (`apk add chafa` / `apt install chafa` / `brew install chafa`)
- A **sixel-capable terminal**, run **outside tmux** for best results:
  - ✅ [`foot`](https://codeberg.org/dnkl/foot) (recommended), `xterm -ti vt340`, WezTerm, Kitty*
  - ⚠️ tmux degrades sixel — run in a bare terminal, or pass it through explicitly.

### Build & run

```bash
git clone https://github.com/hackenstacks/adv-go-char-gen.git
cd adv-go-char-gen
go build -o adv-go-char-gen .

# point it at your cards and go
env CARDS_DIR=~/my-cards ./adv-go-char-gen
```

Cards default to `./cards`. On first run you'll create an account — this derives the key that encrypts
your library.

---

## 🕹️ The flow

```
  Login ──▶ Main menu ──▶ 📇 Card Browser ──▶ ✏️ Edit  (ctrl+t) ──▶ 💬 Chat
                    │                                                  │
                    └────────────▶ 📚 Library ◀── conversations & images auto-file here
```

### 📇 Browser (raw sixel)
Arrow keys to move, `Enter` to open a card in the editor, chat shortcut to talk. Every card is a
real sixel thumbnail.

### ✏️ Editor
| Key | Action |
|---|---|
| `tab` / `shift+tab` | Move between fields |
| `ctrl+s` | 💾 Save back into the PNG |
| `ctrl+r` | Export as an [aichat](https://github.com/sigoden/aichat) role (`.md`) |
| `ctrl+j` | Export as JSON |
| `ctrl+t` | 💬 Chat with this character |
| `ctrl+p` | 🖼️ True-color sixel portrait preview |
| `esc` | Back to browser |

The inline portrait uses truecolor symbols (a curses limitation); `ctrl+p` and the on-open greeting give
you the real sixel.

### 💬 Chat
The avatar is drawn once in sixel at top-left; the conversation and reply box repaint per keystroke, so
the image never flickers. Generated images build a **filmstrip** across the top row.

**Slash commands**

| Command | What it does |
|---|---|
| `/image <prompt>` | Generate an image beside the avatar |
| `/image` | *(no prompt)* The character illustrates the **current scene** from the last 3 messages |
| `/autoimage on\|off` | Toggle auto scene-illustration every 3 rounds (on by default) |
| `/memory` · `/commit <fact>` | View / add to this character's memory |
| `/compact` | Summarize the conversation to save context |
| `/prompt <text>` | Steer the character's upcoming replies |
| `/save` · `/export` | Save the session (→ Library) · export conversation as JSON |
| `/models` · `/help` · `/exit` | List models · help · leave |

**Reply-bar shortcuts:** `^G` /image · `^O` /memory · `^K` /compact · `^E` /export · `^S` /save ·
`^X` exit · `↑/↓` `PgUp/PgDn` scroll.

---

## 📚 The Library

Everything you create is filed into a **local, encrypted store** — no network, no content-addressing,
no IPFS. Each entry is AES-256-GCM encrypted at rest with a key derived from your password.

- **Auto-saved:** every generated/scene image the moment it's created, and each conversation on `/save`
  and on exit (one entry per session, updated in place — no duplicates).
- **In the Library screen:**

| Key | Action |
|---|---|
| `enter` | View details |
| `e` | **Export** — decrypt an entry out to a plain file (`…/exports/`) |
| `s` | **Share** — re-encrypt with a separate password into a self-contained shareable JSON |
| `d` | **Delete** |

Entries are sorted newest-first. Sharing never exposes your library key — the recipient needs the
sharing password you set.

---

## 🔌 Providers & configuration

Configure everything with a `.env` file in the project root (it is **git-ignored** — your keys never
leave your machine):

```ini
# Pollinations — primary provider (text + images). Needs an sk_ key.
POLLINATIONS_API_KEY=sk_xxxxxxxxxxxxxxxxxxxxxxxx
POLLINATIONS_TEXT_MODEL=openai      # openai · mistral · gemini · claude · deepseek · llama …
POLLINATIONS_IMAGE_MODEL=flux       # flux · kontext · seedream · gptimage · qwen-image …

# …or bring your own
# MISTRAL_API_KEY=...
# OPENAI_API_KEY=...
# GEMINI_API_KEY=...
# GROQ_API_KEY=...
# ANTHROPIC_API_KEY=...
# CUSTOM_API_URL=...     CUSTOM_MODEL=...     # any OpenAI-compatible endpoint

CHAR_GEN_GRAPHICS=high              # high = sixel, low = symbols
```

| Provider | Text | Images |
|---|:---:|:---:|
| **Pollinations** (default) | ✅ | ✅ |
| OpenAI / Groq / Mistral / Gemini / Anthropic | ✅ | – |
| AI Horde | ✅ | ✅ |
| Custom (OpenAI-compatible) | ✅ | ✅ |

> 🔐 **Never commit your `.env` or paste a key anywhere public.** Rotate any key that leaves your machine.

---

## 🏗️ Architecture — the one hard problem

Bubble Tea (like all curses TUIs) **cannot render sixel**: it line-diffs the screen and can't measure a
pixel blob, so images get clipped or misplaced. The fix that makes this whole app possible:

> **The screens that need images are raw-terminal programs**, launched via `tea.Exec` (which releases the
> terminal). They use `golang.org/x/term` raw mode + absolute cursor positioning + `chafa` sixel to place
> pixel-perfect true-color images alongside text.

| File | Role |
|---|---|
| `rawbrowser.go` | Raw sixel card browser + fullscreen image preview |
| `rawchat.go` | Raw sixel chat — avatar, live conversation, image filmstrip, scene engine |
| `card_editor.go` | Bubble Tea PNG editor (12 fields, save-to-PNG, exports) |
| `library.go` / `library_files.go` | Encrypted library + delete / share / export |
| `pollinations.go` · `openai_compat.go` · `services.go` | Providers + factories |
| `data_store.go` · `auth.go` | AES-256-GCM store + account/key derivation |

Card I/O and rendering come from the companion package
[**nexus-charcard**](https://github.com/hackenstacks/nexus-charcard) (`v0.1.0`), pulled automatically by
`go build` — no manual setup, just clone and build.

---

## 🧪 Tests

```bash
go test -run TestLibraryRoundTrip .    # library add/list/load/update/export/delete round-trip
```

*(The full `go test ./...` includes a known-failing end-to-end login harness unrelated to features —
use the filtered command above for a clean pass.)*

---

## 🛰️ The NeXuS stack

adv-go-char-gen is one node in **NeXuS** — a sovereign, privacy-first toolset built on five principles:
**Sane · Simple · Secure · Stealthy · Beautiful.** Local-first, encrypted by default, yours to run.

---

## 🤝 Contributing

Issues and PRs welcome. Keep it sane and simple, back up before you change, and never commit secrets or
binaries.

---

<div align="center">

*Part of the NeXuS stack · Made with ⬡ by **hackenstacks***

[hackenstacks@gmail.com](mailto:hackenstacks@gmail.com) · [github.com/hackenstacks](https://github.com/hackenstacks)

</div>
