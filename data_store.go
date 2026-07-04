package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

const (
	userDataDirPerm = 0700 // Owner read/write/execute
)

// GetUserDataPath returns the base path for a user's encrypted data
func GetUserDataPath(username string) string {
	usernameHash := sha256.Sum256([]byte(username))
	return filepath.Join(usersDir, hex.EncodeToString(usernameHash[:]), "data")
}

// SaveEncryptedData encrypts and saves generic data associated with the user.
// The dataType helps organize data into subdirectories (e.g., "characters", "chats").
// The filename is derived from the dataName.
func SaveEncryptedData(user *User, username string, dataType string, dataName string, data []byte) error {
	if user == nil || user.EncryptionKey == nil {
		return fmt.Errorf("user not logged in or encryption key not available")
	}

	userDataPath := GetUserDataPath(username)
	typedDataPath := filepath.Join(userDataPath, dataType)

	// Ensure the user's data directory exists
	if _, err := os.Stat(typedDataPath); os.IsNotExist(err) {
		err = os.MkdirAll(typedDataPath, userDataDirPerm)
		if err != nil {
			return fmt.Errorf("failed to create user data directory: %w", err)
		}
	}

	encryptedData, err := Encrypt(data, user.EncryptionKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt data: %w", err)
	}

	filePath := filepath.Join(typedDataPath, dataName+".bin") // Using .bin for binary encrypted data
	err = os.WriteFile(filePath, encryptedData, userFilePerm)
	if err != nil {
		return fmt.Errorf("failed to write encrypted data file: %w", err)
	}
	return nil
}

// LoadEncryptedData loads and decrypts generic data associated with the user.
func LoadEncryptedData(user *User, username string, dataType string, dataName string) ([]byte, error) {
	if user == nil || user.EncryptionKey == nil {
		return nil, fmt.Errorf("user not logged in or encryption key not available")
	}

	userDataPath := GetUserDataPath(username)
	typedDataPath := filepath.Join(userDataPath, dataType)
	filePath := filepath.Join(typedDataPath, dataName+".bin")

	encryptedData, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("data '%s/%s' not found for user '%s'", dataType, dataName, user.Username)
		}
		return nil, fmt.Errorf("failed to read encrypted data file: %w", err)
	}

	decryptedData, err := Decrypt(encryptedData, user.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data. Key mismatch or corrupted data: %w", err)
	}

	return decryptedData, nil
}
