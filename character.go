package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// Character represents a character in the generator.
type Character struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Personality  string            `json:"personality"`
	Scenario     string            `json:"scenario"`
	FirstMessage string            `json:"firstMessage,omitempty"`
	Lorebook     map[string]string `json:"lorebook,omitempty"`
	// Add more fields for compatibility with character card types later
	// Example: AvatarURL string `json:"avatarUrl,omitempty"`
}

// CharacterInfo is a lighter version of Character for listing purposes.
type CharacterInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// NewCharacter creates a new character with a generated ID.
func NewCharacter() *Character {
	return &Character{
		ID:       uuid.New().String(),
		Lorebook: make(map[string]string),
	}
}

// SaveCharacter saves a character to the user's encrypted data store.
func SaveCharacter(user *User, char *Character) error {
	if user == nil || user.Username == "" {
		return fmt.Errorf("invalid user for saving character")
	}
	if char == nil || char.ID == "" || char.Name == "" {
		return fmt.Errorf("invalid character data for saving")
	}

	charBytes, err := json.Marshal(char)
	if err != nil {
		return fmt.Errorf("failed to marshal character data: %w", err)
	}

	// Use character ID as dataName for unique storage
	return SaveEncryptedData(user, user.Username, "characters", char.ID, charBytes)
}

// LoadCharacter loads a character from the user's encrypted data store by ID.
func LoadCharacter(user *User, characterID string) (*Character, error) {
	if user == nil || user.Username == "" {
		return nil, fmt.Errorf("invalid user for loading character")
	}
	if characterID == "" {
		return nil, fmt.Errorf("character ID cannot be empty")
	}

	charBytes, err := LoadEncryptedData(user, user.Username, "characters", characterID)
	if err != nil {
		return nil, fmt.Errorf("failed to load encrypted character data for ID '%s': %w", characterID, err)
	}

	var char Character
	if err := json.Unmarshal(charBytes, &char); err != nil {
		return nil, fmt.Errorf("failed to unmarshal character data for ID '%s': %w", characterID, err)
	}
	return &char, nil
}

// ListCharacters lists all characters for a given user.
func ListCharacters(user *User) ([]CharacterInfo, error) {
	if user == nil || user.Username == "" {
		return nil, fmt.Errorf("invalid user for listing characters")
	}

	userCharPath := GetUserDataPath(user.Username)
	charDirPath := filepath.Join(userCharPath, "characters")

	files, err := filepath.Glob(filepath.Join(charDirPath, "*.bin"))
	if err != nil {
		return nil, fmt.Errorf("failed to glob character files: %w", err)
	}

	var charInfos []CharacterInfo
	for _, file := range files {
		// Extract character ID from filename (e.g., "uuid.bin")
		fileName := filepath.Base(file)
		characterID := strings.TrimSuffix(fileName, filepath.Ext(fileName))

		char, err := LoadCharacter(user, characterID)
		if err != nil {
			// Log error but continue to list other characters
			fmt.Printf("Warning: Could not load character with ID %s: %v\n", characterID, err)
			// Add more specific error handling for encryption issues
			if strings.Contains(err.Error(), "failed to decrypt") || strings.Contains(err.Error(), "key mismatch") {
				fmt.Printf("This character may have been created with a different encryption key or the data may be corrupted.\n")
			}
			continue
		}
		charInfos = append(charInfos, CharacterInfo{
			ID:   char.ID,
			Name: char.Name,
		})
	}
	return charInfos, nil
}

// DeleteCharacter deletes a character from the user's encrypted data store.
func DeleteCharacter(user *User, characterID string) error {
	if user == nil || user.Username == "" {
		return fmt.Errorf("invalid user for deleting character")
	}
	if characterID == "" {
		return fmt.Errorf("character ID cannot be empty")
	}

	userCharPath := GetUserDataPath(user.Username)
	charFilePath := filepath.Join(userCharPath, "characters", characterID+".bin")

	// The `os.Remove` function will handle deleting the file
	err := os.Remove(charFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("character with ID '%s' not found", characterID)
		}
		return fmt.Errorf("failed to delete character file for ID '%s': %w", characterID, err)
	}
	return nil
}