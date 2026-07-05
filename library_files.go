package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)


// LibraryFile represents a file stored in the user's library.
type LibraryFile struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`          // Display name of the file
	Type         string    `json:"type"`          // e.g., "image", "chat_log", "character_card", "json", "text"
	OriginalPath string    `json:"originalPath"`  // Original path if uploaded, empty if generated
	StoredPath   string    `json:"storedPath"`    // Path to the encrypted content in data_store
	Timestamp    time.Time `json:"timestamp"`
}

// --- Helper Functions (Assuming these exist elsewhere) ---

// GenerateRandomID generates a unique ID for a file.
func GenerateRandomID() string {
	// Implementation omitted for brevity
	return "random-id-" + fmt.Sprint(time.Now().UnixNano())
}



// --- Core Functions ---

// SharedFile represents a file encrypted for sharing, including its metadata, encrypted content, salt, and nonce.
type SharedFile struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	EncryptedData []byte `json:"encryptedData"`
	Salt         []byte `json:"salt"`
	Nonce        []byte `json:"nonce"`
}

// AddFileToLibrary adds a file on disk to the user's encrypted library.
// It reads the file content and hands off to AddDataToLibrary.
func AddFileToLibrary(user *User, originalFilePath, desiredFileName, fileType string) (string, error) {
	fileContent, err := ioutil.ReadFile(originalFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to read original file '%s': %w", originalFilePath, err)
	}
	return AddDataToLibrary(user, desiredFileName, fileType, fileContent, originalFilePath)
}

// AddDataToLibrary adds raw bytes to the user's encrypted library, encrypting the
// content, saving it, and writing metadata. originalPath is optional provenance
// (empty for content generated in-app, e.g. chat logs). Returns the new file ID.
func AddDataToLibrary(user *User, desiredFileName, fileType string, content []byte, originalPath string) (string, error) {
	if user == nil || user.Username == "" || user.EncryptionKey == nil {
		return "", fmt.Errorf("invalid user for adding to library")
	}

	fileID := GenerateRandomID()
	if err := SaveEncryptedData(user, user.Username, "library_content", fileID, content); err != nil {
		return "", fmt.Errorf("failed to save encrypted file content: %w", err)
	}

	libraryFile := LibraryFile{
		ID:           fileID,
		Name:         desiredFileName,
		Type:         fileType,
		OriginalPath: originalPath,
		StoredPath:   filepath.Join(GetUserDataPath(user.Username), "library_content", fileID+".bin"),
		Timestamp:    time.Now(),
	}

	metadataBytes, err := json.Marshal(libraryFile)
	if err != nil {
		return "", fmt.Errorf("failed to marshal library file metadata: %w", err)
	}
	if err := SaveEncryptedData(user, user.Username, "library_metadata", fileID, metadataBytes); err != nil {
		return "", fmt.Errorf("failed to save encrypted library file metadata: %w", err)
	}
	return fileID, nil
}

// UpdateLibraryFileContent replaces the encrypted content of an existing library
// entry in place (same ID) and bumps its timestamp. Used to keep a single library
// snapshot of a live chat up to date across repeated /save calls.
func UpdateLibraryFileContent(user *User, fileID string, content []byte) error {
	if user == nil || user.Username == "" || user.EncryptionKey == nil {
		return fmt.Errorf("invalid user for updating library file")
	}
	if fileID == "" {
		return fmt.Errorf("file ID cannot be empty")
	}
	if err := SaveEncryptedData(user, user.Username, "library_content", fileID, content); err != nil {
		return fmt.Errorf("failed to update library content: %w", err)
	}
	// Refresh the metadata timestamp so the list reflects the latest save.
	metadataBytes, err := LoadEncryptedData(user, user.Username, "library_metadata", fileID)
	if err != nil {
		return nil // content updated; metadata refresh is best-effort
	}
	var lf LibraryFile
	if json.Unmarshal(metadataBytes, &lf) == nil {
		lf.Timestamp = time.Now()
		if b, err := json.Marshal(lf); err == nil {
			SaveEncryptedData(user, user.Username, "library_metadata", fileID, b)
		}
	}
	return nil
}

// libraryExtForType returns a sensible file extension for exporting a library entry.
func libraryExtForType(fileType string) string {
	switch fileType {
	case "image":
		return ".jpg"
	case "chat_log":
		return ".json"
	case "json":
		return ".json"
	case "character_card":
		return ".png"
	case "text":
		return ".txt"
	default:
		return ".bin"
	}
}

// ExportLibraryFile decrypts a library entry and writes it as a plain file into
// destDir, returning the written path. Used by the "export" action so a saved
// conversation or image can leave the encrypted store as a normal file.
func ExportLibraryFile(user *User, fileID, destDir string) (string, error) {
	file, content, err := LoadFileFromLibrary(user, fileID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create export dir: %w", err)
	}

	base := strings.TrimSpace(file.Name)
	if base == "" {
		base = "library-" + fileID
	}
	// Slugify the name and ensure it carries the right extension.
	base = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		case r >= 'A' && r <= 'Z':
			return r + 32
		default:
			return '-'
		}
	}, strings.ReplaceAll(base, " ", "-"))
	base = strings.Trim(base, "-")
	ext := libraryExtForType(file.Type)
	if !strings.HasSuffix(strings.ToLower(base), ext) {
		base += ext
	}

	outPath := filepath.Join(destDir, base)
	if err := os.WriteFile(outPath, content, 0644); err != nil {
		return "", fmt.Errorf("failed to write export '%s': %w", outPath, err)
	}
	return outPath, nil
}

// EncryptAndMarshalSharedFile encrypts file content with a sharing password and marshals it into a sharable JSON.
func EncryptAndMarshalSharedFile(outputPath string, file *LibraryFile, content []byte, sharingPassword string) error {
	if file == nil || content == nil || sharingPassword == "" {
		return fmt.Errorf("invalid parameters for sharing file")
	}

	// Generate salt for sharing password
	sharingSalt, err := GenerateSalt(saltLen)
	if err != nil {
		return fmt.Errorf("failed to generate salt for sharing: %w", err)
	}

	// Derive key from sharing password and salt
	sharingEncryptionKey, err := DeriveKey(sharingPassword, sharingSalt)
	if err != nil {
		return fmt.Errorf("failed to derive encryption key for sharing: %w", err)
	}

	// Encrypt the content with the sharing key
	block, err := aes.NewCipher(sharingEncryptionKey)
	if err != nil {
		return fmt.Errorf("failed to create cipher for sharing: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM for sharing: %w", err)
	}

	sharingNonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, sharingNonce); err != nil {
		return fmt.Errorf("failed to generate nonce for sharing: %w", err)
	}

	encryptedContent := gcm.Seal(sharingNonce, sharingNonce, content, nil)

	// Create shared file structure
	sharedFile := SharedFile{
		Name:         file.Name,
		Type:         file.Type,
		EncryptedData: encryptedContent,
		Salt:         sharingSalt,
		Nonce:        sharingNonce,
	}

	// Marshal into JSON
	sharedFileJSON, err := json.MarshalIndent(sharedFile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal shared file JSON: %w", err)
	}

	// Write to output path
	if err := ioutil.WriteFile(outputPath, sharedFileJSON, 0644); err != nil {
		return fmt.Errorf("failed to write shared file to output path '%s': %w", outputPath, err)
	}

	return nil
}

// LoadFileFromLibrary loads a file's metadata and content from the user's encrypted library.
func LoadFileFromLibrary(user *User, fileID string) (*LibraryFile, []byte, error) {
	if user == nil || user.Username == "" {
		return nil, nil, fmt.Errorf("invalid user for loading file from library")
	}
	if fileID == "" {
		return nil, nil, fmt.Errorf("file ID cannot be empty")
	}

	// Load metadata
	metadataBytes, err := LoadEncryptedData(user, user.Username, "library_metadata", fileID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load encrypted library file metadata for ID '%s': %w", fileID, err)
	}

	var libraryFile LibraryFile
	if err := json.Unmarshal(metadataBytes, &libraryFile); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal library file metadata for ID '%s': %w", fileID, err)
	}

	// Load content
	contentBytes, err := LoadEncryptedData(user, user.Username, "library_content", fileID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load encrypted file content for ID '%s': %w", fileID, err)
	}

	return &libraryFile, contentBytes, nil
}

// ListLibraryFiles lists all files stored in the user's library.
func ListLibraryFiles(user *User) ([]LibraryFile, error) {
	if user == nil || user.Username == "" {
		return nil, fmt.Errorf("invalid user for listing library files")
	}

	userLibraryMetadataPath := filepath.Join(GetUserDataPath(user.Username), "library_metadata")

	files, err := filepath.Glob(filepath.Join(userLibraryMetadataPath, "*.bin"))
	if err != nil {
		return nil, fmt.Errorf("failed to glob library metadata files: %w", err)
	}

	var libraryFiles []LibraryFile
	for _, file := range files {
		fileName := filepath.Base(file)
		fileID := strings.TrimSuffix(fileName, filepath.Ext(fileName))

		metadataBytes, err := LoadEncryptedData(user, user.Username, "library_metadata", fileID)
		if err != nil {
			fmt.Printf("Warning: Could not load metadata for library file ID %s: %v\n", fileID, err)
			continue
		}

		var libraryFile LibraryFile
		if err := json.Unmarshal(metadataBytes, &libraryFile); err != nil {
			fmt.Printf("Warning: Could not unmarshal metadata for library file ID %s: %v\n", fileID, err)
			continue
		}
		libraryFiles = append(libraryFiles, libraryFile)
	}
	return libraryFiles, nil
}

// DeleteLibraryFile removes a file and its metadata from the user's library.
func DeleteLibraryFile(user *User, fileID string) error {
	if user == nil || user.Username == "" {
		return fmt.Errorf("invalid user for deleting file from library")
	}
	if fileID == "" {
		return fmt.Errorf("file ID cannot be empty")
	}

	// 1. Remove from the library index (by listing and then re-saving without the file)
	library, err := ListLibraryFiles(user)
	if err != nil {
		return fmt.Errorf("failed to list library files for deletion: %w", err)
	}

	var updatedLibrary []LibraryFile
	found := false
	for _, file := range library {
		if file.ID == fileID {
			found = true
			continue
		}
		updatedLibrary = append(updatedLibrary, file)
	}

	if !found {
		return fmt.Errorf("file with ID '%s' not found in library index", fileID)
	}

	// 2. Save the updated index
	indexPath := filepath.Join(GetUserDataPath(user.Username), "library_metadata", "index.json") // Assuming an index file exists
	indexData, err := json.MarshalIndent(updatedLibrary, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal updated library index: %w", err)
	}
	encryptedIndex, err := Encrypt(indexData, user.EncryptionKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt updated library index: %w", err)
	}

	// This part needs to be adapted based on how the index is actually stored.
	// If it's a single index file:
	if err := os.WriteFile(indexPath, encryptedIndex, 0644); err != nil {
		return fmt.Errorf("failed to write updated library index: %w", err)
	}

	// If each file's metadata is stored separately and ListLibraryFiles reads them all,
	// then deleting the individual metadata file is sufficient.
	// The following is for deleting the individual metadata file:
	metadataFilePath := filepath.Join(GetUserDataPath(user.Username), "library_metadata", fileID+".bin")
	if err := os.Remove(metadataFilePath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete library metadata file '%s': %w", metadataFilePath, err)
		}
	}

	// 3. Delete the actual encrypted content file
	encryptedFilePath := filepath.Join(GetUserDataPath(user.Username), "library_content", fileID+".bin")
	if err := os.Remove(encryptedFilePath); err != nil {
		// If the file is already gone, we don't consider it a fatal error
		// as the main goal was to remove it from the index.
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete encrypted file '%s': %w", encryptedFilePath, err)
		}
	}

	return nil
}

// DeleteFileFromLibrary deletes a file and its metadata from the user's library.
// This is a more direct deletion function, assuming metadata is stored per file.
func DeleteFileFromLibrary(user *User, fileID string) error {
	if user == nil || user.Username == "" {
		return fmt.Errorf("invalid user for deleting file from library")
	}
	if fileID == "" {
		return fmt.Errorf("file ID cannot be empty")
	}

	// Delete content file
	contentFilePath := filepath.Join(GetUserDataPath(user.Username), "library_content", fileID+".bin")
	err := os.Remove(contentFilePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete library content file '%s': %w", contentFilePath, err)
	}

	// Delete metadata file
	metadataFilePath := filepath.Join(GetUserDataPath(user.Username), "library_metadata", fileID+".bin")
	err = os.Remove(metadataFilePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete library metadata file '%s': %w", metadataFilePath, err)
	}

	return nil
}
