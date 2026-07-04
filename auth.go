package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/scrypt"
)

// User represents a user in the system
type User struct {
	Username       string `json:"username"`
	HashedPassword string `json:"hashedPassword"`
	// Salt is stored separately or prepended to the encrypted data, not directly in this marshaled JSON
	EncryptionKey []byte `json:"-"` // This will hold the derived key for session, not marshaled
}

var usersDir = StorageDir()

const (
	userFilePerm  = 0600 // Owner read/write only
	keyLen        = 32   // AES-256 key length
	saltLen       = 16   // 16 bytes for salt
	scryptN       = 32768
	scryptR       = 8
	scryptP       = 1
)

// HashPassword hashes a password using bcrypt
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPasswordHash checks if a hashed password matches a plaintext password
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// GenerateSalt generates a random salt for key derivation
func GenerateSalt(n int) ([]byte, error) {
	bytes := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return nil, err
	}
	return bytes, nil
}

// DeriveKey derives an encryption key from a password and salt using scrypt
func DeriveKey(password string, salt []byte) ([]byte, error) {
	key, err := scrypt.Key([]byte(password), salt, scryptN, scryptR, scryptP, keyLen)
	if err != nil {
		return nil, err
	}
	return key, nil
}

// Encrypt encrypts data using AES-256 GCM
func Encrypt(plaintext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts data using AES-256 GCM
func Decrypt(ciphertext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

// SaveUser saves user data to a file, prepending the salt to the encrypted data
func SaveUser(user *User, password string) error {
	// Generate a random salt
	salt, err := GenerateSalt(saltLen)
	if err != nil {
		return fmt.Errorf("failed to generate salt: %w", err)
	}

	// Derive encryption key from password and salt
	encryptionKey, err := DeriveKey(password, salt)
	if err != nil {
		return fmt.Errorf("failed to derive key: %w", err)
	}

	// Hash the password for storage
	hashedPassword, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	user.HashedPassword = hashedPassword

	// Marshal user data (excluding EncryptionKey which is for session)
	userData, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("failed to marshal user data: %w", err)
	}

	// Encrypt the user data using the derived key
	encryptedUserData, err := Encrypt(userData, encryptionKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt user data: %w", err)
	}

	// Prepend the salt to the encrypted data before writing to file
	finalData := append(salt, encryptedUserData...)

	usernameHash := sha256.Sum256([]byte(user.Username))
	userFile := hex.EncodeToString(usernameHash[:]) + ".json"
	userFilePath := filepath.Join(usersDir, userFile)

	// Ensure the users directory exists
	if _, err := os.Stat(usersDir); os.IsNotExist(err) {
		err = os.Mkdir(usersDir, 0700) // Owner read/write/execute
		if err != nil {
			return fmt.Errorf("failed to create users directory: %w", err)
		}
	}

	err = os.WriteFile(userFilePath, finalData, userFilePerm)
	if err != nil {
		return fmt.Errorf("failed to write user file: %w", err)
	}
	return nil
}

// LoadUser loads user data from a file, extracting the salt and decrypting with the provided password
func LoadUser(username, password string) (*User, error) {
	usernameHash := sha256.Sum256([]byte(username))
	userFile := hex.EncodeToString(usernameHash[:]) + ".json"
	userFilePath := filepath.Join(usersDir, userFile)

	finalData, err := os.ReadFile(userFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("user '%s' not found", username)
		}
		return nil, fmt.Errorf("failed to read user file: %w", err)
	}

	if len(finalData) < saltLen {
		return nil, fmt.Errorf("invalid user data format: too short to contain salt")
	}

	// Extract the salt from the beginning of the data
	salt := finalData[:saltLen]
	encryptedUserData := finalData[saltLen:]

	// Derive the encryption key using the provided password and extracted salt
	encryptionKey, err := DeriveKey(password, salt)
	if err != nil {
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}

	// Decrypt the user data
	decryptedUserData, err := Decrypt(encryptedUserData, encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt user data. Incorrect password or corrupted data: %w", err)
	}

	var loadedUser User
	if err := json.Unmarshal(decryptedUserData, &loadedUser); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user data: %w", err)
	}

	// Check if the provided password matches the stored hashed password
	if !CheckPasswordHash(password, loadedUser.HashedPassword) {
		return nil, fmt.Errorf("invalid username or password")
	}

	// Set the derived encryption key for session use
	loadedUser.EncryptionKey = encryptionKey

	return &loadedUser, nil
}

// UserExists checks if a username already exists by checking for its user file
func UserExists(username string) bool {
	usernameHash := sha256.Sum256([]byte(username))
	userFile := hex.EncodeToString(usernameHash[:]) + ".json"
	userFilePath := filepath.Join(usersDir, userFile)
	_, err := os.Stat(userFilePath)
	return !os.IsNotExist(err)
}