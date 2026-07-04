package main

import (
	"os"
	"path/filepath"
)

// AppPaths holds all configurable directory and file paths.
// Every path is resolved once at startup from env vars, then used throughout.
//
// Environment variables:
//
//	CHAR_GEN_DATA_DIR   — user data / storage root      (default: ~/.local/share/char-gen-cli)
//	CHAR_GEN_LOG        — debug log file                 (default: CHAR_GEN_DATA_DIR/debug.log)
//	CARDS_DIR           — PNG character card directory   (default: ~/ai-characters)
//	AICHAT_ROLES_DIR    — aichat roles output directory  (default: ~/.config/aichat/roles)
type AppPaths struct {
	DataDir   string // root for all app data (storage, logs)
	LogFile   string // debug log path
	CardsDir  string // PNG character cards
	RolesDir  string // aichat .md role files
}

var Paths AppPaths

// GraphicsMode controls image rendering quality.
//   "high" — sixel (true pixels) for the fullscreen preview; best in foot/xterm
//   "low"  — symbols (truecolor half-blocks) everywhere; works in tmux and any terminal
// Set via CHAR_GEN_GRAPHICS env var. Default: high (foot supports sixel).
var GraphicsMode = getEnv("CHAR_GEN_GRAPHICS", "high")

func init() {
	home, _ := os.UserHomeDir()

	dataDir := getEnv("CHAR_GEN_DATA_DIR",
		filepath.Join(home, ".local", "share", "char-gen-cli"))

	Paths = AppPaths{
		DataDir:  dataDir,
		LogFile:  getEnv("CHAR_GEN_LOG", filepath.Join(dataDir, "debug.log")),
		CardsDir: getEnv("CARDS_DIR", "cards"),
		RolesDir: getEnv("AICHAT_ROLES_DIR", filepath.Join(home, ".config", "aichat", "roles")),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// StorageDir returns the path used for encrypted user data.
// This replaces the old hardcoded "storage" relative path.
func StorageDir() string {
	return filepath.Join(Paths.DataDir, "storage")
}
