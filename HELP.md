# ⬡ char-gen-cli — Help & Reference

A CLI-first AI character generator, card browser, editor, and multi-AI chat.
Part of the NeXuS stack.

---

## Launching

```bash
./char-gen-cli
```

With a specific cards directory:

```bash
CARDS_DIR=~/01/ai-characters ./char-gen-cli
```

On first run you'll create an account (encrypted local storage — nothing leaves your machine).

---

## Main Menu

| Item | What it does |
|------|--------------|
| **Character Generator** | Create, list, edit, delete, and LLM-generate characters saved in your library |
| **Card Browser** | Browse PNG character cards (chara_card_v2) with portraits, edit, chat, import |
| **Library** | Manage stored files and documents |
| **API Settings** | Configure LLM/image providers and API keys |
| **Logout** | Return to login |

---

## Card Browser

Browse a directory of PNG character cards. Portraits and thumbnails render right in the terminal.

| Key | Action |
|-----|--------|
| `↑ ↓` / `j k` | Move between cards |
| `enter` / `e` | Open the card **editor** |
| `c` | **Chat** with the character (ephemeral — nothing saved) |
| `i` | **Import** the card into your character library |
| `v` | **Fullscreen preview** — crisp, true-color image (high graphics) |
| `d` | Change the cards directory |
| `q` / `esc` | Back to main menu |

### About the images

There are two rendering paths:

- **Inline (list + detail pane)** — uses Unicode half-block symbols with **24-bit truecolor**.
  This composes cleanly with the panel layout and works everywhere, including tmux.
  It looks slightly "blocky" because each character cell is ~2 pixels.

- **Fullscreen preview (`v` key)** — uses **sixel**, true pixel-for-pixel rendering at full
  color. This is the crisp, photographic view. It bypasses the panel layout, which is why
  it's a separate fullscreen mode (sixel and bordered panels don't mix).

Press any key to close the preview.

### Graphics mode

Controlled by the `CHAR_GEN_GRAPHICS` environment variable:

```bash
CHAR_GEN_GRAPHICS=high ./char-gen-cli   # sixel preview enabled (default)
CHAR_GEN_GRAPHICS=low  ./char-gen-cli   # symbols only (best for tmux / no-sixel terminals)
```

- **high** (default): the `v` key shows a true-color sixel preview. Best in **foot** or any
  sixel-capable terminal. If you run inside **tmux**, enable passthrough first:
  `tmux set -g allow-passthrough on`
- **low**: everything stays as truecolor symbols. Use this if sixel garbles your terminal.

---

## Card Editor

Edit every field of a chara_card_v2 card.

| Key | Action |
|-----|--------|
| `tab` / `shift+tab` | Move between fields |
| `ctrl+s` | **Save** changes back into the PNG (image untouched) |
| `ctrl+r` | Export as an **aichat role** (`.md` in your roles dir) |
| `ctrl+j` | Export as **JSON** |
| `ctrl+c` | Jump straight into **chat** with this card |
| `esc` | Back to the browser |

---

## Chat

Talk to a character. Two modes:

- **Card chat** (from the browser `c` key): ephemeral, uses the card's system prompt directly.
  Nothing is saved. `esc` returns to the browser.
- **Library chat** (`/character <name>`): full memory, saved sessions, multi-character.

### Chat commands

| Command | Action |
|---------|--------|
| `/help` | Show all commands |
| `/models` | List available models from the active provider |
| `/provider` | Switch LLM or image provider |
| `/character <name>` | Load a saved character into the chat |
| `/invite <name>` | Add another character to the conversation |
| `/ai2ai <charA> <charB> <topic>` | Watch two characters talk to each other |
| `/image <prompt>` | Generate an image |
| `/system <prompt>` | Set the system prompt |
| `/topic <text>` | Set the conversation topic |
| `/save` | Save the current session |
| `/end` | Clear the conversation |
| `/quit` | Exit chat |

Keys: `enter` sends, `esc` goes back, `ctrl+c` quits the app.

---

## Character Generator

Your saved character library. Imported cards (`i` in the browser) appear here.

| Key | Action |
|-----|--------|
| `↑ ↓` | Move between characters |
| `enter` | Edit the selected character |
| `n` | New character |
| `d` | Delete the selected character |
| `alt+g` | LLM-generate fields from a prompt |
| `tab` / `shift+tab` | Move between form fields (in create/edit) |
| `esc` | Back |

---

## API Settings & `.env`

Configure providers in the API Settings screen, or drop a `.env` file in the project root
(or `~/.config/char-gen-cli/.env`):

```env
# Any OpenAI-compatible endpoint (aichat --serve, Ollama /v1, LM Studio, etc.)
CUSTOM_API_URL=http://localhost:3030
CUSTOM_API_KEY=
CUSTOM_MODEL=default

# Cloud providers
OPENAI_API_KEY=sk-...
MISTRAL_API_KEY=...
ANTHROPIC_API_KEY=sk-ant-...
GROQ_API_KEY=...

# Image generation (OpenAI-compatible or Pollinations)
CUSTOM_IMAGE_API_URL=https://image.pollinations.ai
```

`.env` values overlay whatever is saved in API Settings.

---

## Paths (all configurable via env)

| Variable | Default | Purpose |
|----------|---------|---------|
| `CARDS_DIR` | `./cards` | PNG character cards |
| `CHAR_GEN_DATA_DIR` | `~/.local/share/char-gen-cli` | Encrypted user data |
| `AICHAT_ROLES_DIR` | `~/.config/aichat/roles` | aichat role export target |
| `CHAR_GEN_LOG` | `$DATA_DIR/debug.log` | Debug log |
| `CHAR_GEN_GRAPHICS` | `high` | `high` (sixel preview) or `low` (symbols only) |

---

## Troubleshooting

**"Failed to show notification: … org.freedesktop.Notifications …"**
Harmless. Your terminal is trying to ring the desktop notification daemon, which isn't
running. It does not affect the app.

**Images look blocky inline**
That's the truecolor symbol renderer. Press `v` for the crisp sixel preview, or make sure
you're in a sixel terminal (foot) with `CHAR_GEN_GRAPHICS=high`.

**Sixel preview garbles the screen (especially in tmux)**
Run `CHAR_GEN_GRAPHICS=low`, or enable tmux passthrough: `tmux set -g allow-passthrough on`.

**Chat says "mock" responses / no real replies**
No provider is configured. Set one in API Settings or via `.env` (see above).

---

*Part of the NeXuS stack — Sane · Simple · Secure · Stealthy · Beautiful*
Author: **hackenstacks** — hackenstacks@gmail.com
