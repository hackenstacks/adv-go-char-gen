# ⬡ adv-go-char-gen

> Advanced AI character generator, true-color sixel card browser, and multi-AI chat — all in one terminal app. Part of the NeXuS stack.

**Author: hackenstacks** — [@hackenstacks](https://github.com/hackenstacks) — [hackenstacks@gmail.com](mailto:hackenstacks@gmail.com)

True-color everywhere: the card browser and the chat are raw-terminal screens that render crisp **sixel** images (best in `foot`, outside tmux). The character illustrates the scene beside its avatar as you talk.

## ✨ Features

*   **🔒 Secure & Private:** All data is encrypted locally using military-grade encryption. No data is stored remotely by default.
*   **🤖 Multi-API Support:** Connect to a variety of LLM and image generation APIs, including:
    *   Free, public APIs (default)
    *   AI Horde
    *   Pollinations.ai
    *   ImageRouter
    *   Mistral
    *   Groq
    *   Hugging Face
    *   OpenAI
    *   Claude
    *   Custom user-defined APIs
*   **🎭 Character Card Compatibility:** Import and export character cards from popular formats like Silly Tavern and Open Character AI.
*   **📚 Local Library:** Store and manage your generated characters and files in a local, encrypted library.
*   **🖼️ Inline Image Generation:** Generate images within your chats based on the conversation.
*   **🧠 Advanced Memory System:**
    *   **Working/Short-Term Memory (STM):** For immediate task execution.
    *   **Episodic Memory:** Stores past experiences and session histories.
    *   **Semantic Memory:** A repository for factual knowledge and user profiles.
    *   **Procedural Memory:** Encodes rules, skills, and protocols.
*   **💬 Chat Summarization:** Conversations are automatically summarized to maintain context and relevance.
*   **💅 Beautiful TUI:** A beautiful and intuitive text-based user interface built with Bubble Tea and Lip Gloss.
*   **⚔️ Powerful Commands:** A rich set of slash commands to control the application.

## 🚀 Getting Started

1.  **Build the application:**
    ```bash
    go build -o adv-go-char-gen .
    ```
2.  **Run the application (best in foot, outside tmux, for sixel):**
    ```bash
    ./adv-go-char-gen
    ```
    Card directory defaults to `./cards` — override with `CARDS_DIR=~/my-cards ./adv-go-char-gen`.

## 📝 Chat Commands

*   `/help`: Show available commands and shortcuts.
*   `/image {prompt}`: Generate an image (shown beside the avatar).
*   `/image`: With no prompt, the character illustrates the **current scene** from the last 3 messages.
*   `/autoimage on|off`: Toggle auto scene-illustration every 3 rounds (on by default).
*   `/memory`: View this character's memory. `/commit {fact}`: Save a fact to memory.
*   `/compact`: Summarize the conversation so far. `/prompt {text}`: Steer upcoming replies.
*   `/save`: Save the chat session. `/export`: Export the conversation as JSON.
*   `/models`: List available models. `/exit`: Leave the chat.

**Reply-bar shortcuts:** `^G` /image · `^O` /memory · `^K` /compact · `^E` /export · `^S` /save · `^X` exit · `↑/↓` `PgUp/PgDn` scroll.

---

*Part of the NeXuS stack — © hackenstacks · [hackenstacks@gmail.com](mailto:hackenstacks@gmail.com)*
