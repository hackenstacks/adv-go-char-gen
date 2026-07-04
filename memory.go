package main

import (
	"log"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"
)

// MemoryType defines the type of a memory entry.
type MemoryType string

const (
	MemoryTypeEpisodic  MemoryType = "episodic"  // Specific past experiences, session histories
	MemoryTypeSemantic  MemoryType = "semantic"  // Generalized, factual knowledge, user-specific profiles
	MemoryTypeProcedural MemoryType = "procedural" // Rules, skills, functional protocols (often in system prompts)
	MemoryTypeSummary   MemoryType = "summary"   // Summarized segments of conversation
	MemoryTypeSystem    MemoryType = "system"    // System-level notes or instructions
)

// MemoryEntry represents a single piece of stored memory.
type MemoryEntry struct {
	ID        string       `json:"id"`
	CharacterID string       `json:"characterId"` // Character this memory is associated with
	Content   string       `json:"content"`     // The actual text content of the memory
	Type      MemoryType   `json:"type"`        // Type of memory (episodic, semantic, etc.)
	Source    string       `json:"source"`      // Where this memory came from (e.g., "chat", "lorebook", "user_note")
	Timestamp int64        `json:"timestamp"`   // Unix timestamp when the memory was created
	Keywords  []string     `json:"keywords,omitempty"` // Relevant keywords for retrieval
	Embedding []float32    `json:"-"`           // Vector embedding of the content, not stored directly in JSON
	EmbeddingString string `json:"embeddingString,omitempty"` // Base64 encoded embedding for storage
	Score     float64      `json:"-"`           // Score for retrieval, not stored persistently
}

// MemoryManager defines the interface for managing various types of memories.
type MemoryManager interface {
	AddMemory(user *User, characterID string, entry *MemoryEntry, apiConfig *APIConfig) error
	RetrieveMemories(user *User, characterID string, query string, limit int, apiConfig *APIConfig) ([]MemoryEntry, error)
	// UpdateMemory(user *User, entry *MemoryEntry, apiConfig *APIConfig) error
	// DeleteMemory(user *User, memoryID string) error
	GenerateRollingSummary(user *User, characterID string, messages []Message, systemPrompt string, apiConfig *APIConfig) ([]Message, error)
}

// LocalMemoryManager implements MemoryManager for local, file-based storage.
type LocalMemoryManager struct {
	embeddingService EmbeddingService
	summarizer       Summarizer
}

// NewLocalMemoryManager creates a new LocalMemoryManager.
func NewLocalMemoryManager(es EmbeddingService, s Summarizer) *LocalMemoryManager {
	return &LocalMemoryManager{
		embeddingService: es,
		summarizer:       s,
	}
}

// cosineSimilarity calculates the cosine similarity between two vectors.
func cosineSimilarity(vec1, vec2 []float32) float64 {
	if len(vec1) != len(vec2) {
		return 0.0
	}

	var dotProduct, magnitude1, magnitude2 float64
	for i := 0; i < len(vec1); i++ {
		dotProduct += float64(vec1[i] * vec2[i])
		magnitude1 += float64(vec1[i] * vec1[i])
		magnitude2 += float64(vec2[i] * vec2[i])
	}

	if magnitude1 == 0 || magnitude2 == 0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(magnitude1) * math.Sqrt(magnitude2))
}

// AddMemory adds a new memory entry, generates its embedding, and saves it.
func (mm *LocalMemoryManager) AddMemory(user *User, characterID string, entry *MemoryEntry, apiConfig *APIConfig) error {
	if user == nil || characterID == "" || entry == nil {
		return fmt.Errorf("invalid parameters for AddMemory")
	}
	if entry.ID == "" {
		entry.ID = GenerateRandomID()
	}
	entry.CharacterID = characterID
	entry.Timestamp = time.Now().Unix()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Generate embedding
	embedding, err := mm.embeddingService.CreateEmbedding(ctx, entry.Content)
	if err != nil {
		return fmt.Errorf("failed to create embedding for memory entry: %w", err)
	}
	// Encode embedding to base64 string for storage
	embeddingBytes, err := json.Marshal(embedding)
	if err != nil {
		return fmt.Errorf("failed to marshal embedding: %w", err)
	}
	entry.EmbeddingString = fmt.Sprintf("%x", embeddingBytes) // Store as hex string, not base64 directly


	// Save memory entry metadata (including embedding string)
	memoryBytes, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal memory entry: %w", err)
	}

	return SaveEncryptedData(user, characterID, "memories", entry.ID, memoryBytes)
}

// RetrieveMemories retrieves relevant memories based on a query.
func (mm *LocalMemoryManager) RetrieveMemories(user *User, characterID string, query string, limit int, apiConfig *APIConfig) ([]MemoryEntry, error) {
	if user == nil || characterID == "" || query == "" {
		return nil, fmt.Errorf("invalid parameters for RetrieveMemories")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Generate embedding for the query
	queryEmbedding, err := mm.embeddingService.CreateEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedding for query: %w", err)
	}

	// Load all memories for the character
	allMemories, err := mm.loadAllCharacterMemories(user, characterID)
	if err != nil {
		return nil, fmt.Errorf("failed to load all character memories: %w", err)
	}

	// Calculate similarity scores
	for i := range allMemories {
		var storedEmbedding []float32
		if allMemories[i].EmbeddingString != "" {
			decoded, decodeErr := hex.DecodeString(allMemories[i].EmbeddingString)
			if decodeErr != nil {
				log.Printf("Warning: Failed to decode embedding string for memory %s: %v\n", allMemories[i].ID, decodeErr)
				continue
			}
			if err := json.Unmarshal(decoded, &storedEmbedding); err != nil {
				log.Printf("Warning: Failed to unmarshal embedding for memory %s: %v\n", allMemories[i].ID, err)
				continue
			}
		}
		
		if len(storedEmbedding) > 0 {
			allMemories[i].Score = cosineSimilarity(queryEmbedding, storedEmbedding)
		} else {
			allMemories[i].Score = 0.0 // No embedding, no score
		}
	}

	// Sort by score in descending order
	sort.Slice(allMemories, func(i, j int) bool {
		return allMemories[i].Score > allMemories[j].Score
	})

	// Return top 'limit' memories
	if len(allMemories) < limit {
		return allMemories, nil
	}
	return allMemories[:limit], nil
}

// loadAllCharacterMemories is a helper to load all memory entries for a character.
func (mm *LocalMemoryManager) loadAllCharacterMemories(user *User, characterID string) ([]MemoryEntry, error) {
	memoryDirPath := GetUserDataPath(user.Username) // Base user data path
	charMemoryPath := fmt.Sprintf("%s/%s/%s", memoryDirPath, characterID, "memories")

	files, err := ListFilesInDirectory(charMemoryPath, ".bin") // Assuming memories are .bin files
	if err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			return []MemoryEntry{}, nil // No memories yet, return empty slice
		}
		return nil, fmt.Errorf("failed to list memory files: %w", err)
	}

	var memories []MemoryEntry
	for _, file := range files {
		memoryID := strings.TrimSuffix(file, ".bin")
		memoryBytes, err := LoadEncryptedData(user, characterID, "memories", memoryID)
		if err != nil {
			log.Printf("Warning: Could not load encrypted memory data for ID %s: %v\n", memoryID, err)
			continue
		}

		var entry MemoryEntry
		if err := json.Unmarshal(memoryBytes, &entry); err != nil {
			log.Printf("Warning: Could not unmarshal memory entry for ID %s: %v\n", memoryID, err)
			continue
		}
		memories = append(memories, entry)
	}
	return memories, nil
}

// ListFilesInDirectory is a helper function to list files with a specific extension in a directory.
func ListFilesInDirectory(dirPath, ext string) ([]string, error) {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	var fileNames []string
	for _, file := range files {
		if !file.Type().IsDir() && strings.HasSuffix(file.Name(), ext) {
			fileNames = append(fileNames, strings.TrimSuffix(file.Name(), ext))
		}
	}
	return fileNames, nil
}


// GenerateRollingSummary summarizes a segment of messages and adds it as a memory.
func (mm *LocalMemoryManager) GenerateRollingSummary(user *User, characterID string, messages []Message, systemPrompt string, apiConfig *APIConfig) ([]Message, error) {
	if user == nil || characterID == "" || len(messages) == 0 {
		return nil, fmt.Errorf("invalid parameters for GenerateRollingSummary")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second) // Longer timeout for summarization
	defer cancel()

	// Construct a prompt for summarization
	var chatHistoryBuilder strings.Builder
	chatHistoryBuilder.WriteString("Please summarize the following conversation. Focus on key events, decisions, and character developments. Make it concise.\n\n")
	if systemPrompt != "" {
		chatHistoryBuilder.WriteString(fmt.Sprintf("System Instruction: %s\n\n", systemPrompt))
	}
	chatHistoryBuilder.WriteString("---\n")
	for _, msg := range messages {
		chatHistoryBuilder.WriteString(fmt.Sprintf("%s: %s\n", msg.Sender, msg.Content))
	}
	
	summarizationPrompt := chatHistoryBuilder.String()

	// Use LLM to summarize
	summarizedContent, err := mm.summarizer.Summarize(ctx, summarizationPrompt)
	if err != nil {
		return nil, fmt.Errorf("failed to get summarization from LLM: %w", err)
	}

	// Add the summary as a new memory entry
	summaryMemory := &MemoryEntry{
		ID:          GenerateRandomID(),
		CharacterID: characterID,
		Content:     summarizedContent,
		Type:        MemoryTypeSummary,
		Source:      "rolling_summary",
		Timestamp:   time.Now().Unix(),
		Keywords:    []string{"summary", characterID, "conversation_overview"},
	}
	if err := mm.AddMemory(user, characterID, summaryMemory, apiConfig); err != nil {
		return nil, fmt.Errorf("failed to save summary as memory: %w", err)
	}

	// Add summary as a system message to chat history
	summaryMessage := Message{
		ID: GenerateRandomID(), Sender: "System",
		Content:   fmt.Sprintf("✨ Conversation Summary: %s", summarizedContent),
		Timestamp: time.Now().Unix(),
		Type:      MessageTypeSummary,
	}

	// Trim old messages, keeping the summary and recent messages (e.g., last 5-10 messages)
	trimmedMessages := []Message{summaryMessage}
	recentMessagesCount := 10
	if len(messages) > recentMessagesCount {
		trimmedMessages = append(trimmedMessages, messages[len(messages)-recentMessagesCount:]...)
	} else {
		trimmedMessages = append(trimmedMessages, messages...)
	}

	return trimmedMessages, nil
}
