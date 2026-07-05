package main

import (
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestLibraryRoundTrip exercises the new Library wiring end-to-end at the store
// level: add bytes (chat log + image), list, load, update-in-place, export to a
// plain file, and delete. This mirrors what the chat and Library UI now do.
func TestLibraryRoundTrip(t *testing.T) {
	// Isolate all data under a temp dir (usersDir is the package-level store root).
	tmp := t.TempDir()
	oldUsersDir := usersDir
	usersDir = filepath.Join(tmp, "storage")
	defer func() { usersDir = oldUsersDir }()

	// A logged-in user = a username + a 32-byte AES key.
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("keygen: %v", err)
	}
	user := &User{Username: "tester", EncryptionKey: key}

	// 1. Add a conversation snapshot (chat_log) as raw bytes.
	convo := []Message{
		{ID: "m1", Sender: "You", Content: "hello", Type: MessageTypeUser},
		{ID: "m2", Sender: "Nova", Content: "hi there", Type: MessageTypeCharacter},
	}
	convoJSON, _ := json.MarshalIndent(convo, "", "  ")
	chatID, err := AddDataToLibrary(user, "Chat · Nova", "chat_log", convoJSON, "")
	if err != nil {
		t.Fatalf("AddDataToLibrary(chat): %v", err)
	}

	// 2. Add an "image" via a file on disk (AddFileToLibrary path).
	imgPath := filepath.Join(tmp, "img.jpg")
	imgBytes := []byte("\xFF\xD8\xFFfake-jpeg-bytes")
	if err := os.WriteFile(imgPath, imgBytes, 0644); err != nil {
		t.Fatalf("write img: %v", err)
	}
	imgID, err := AddFileToLibrary(user, imgPath, "Scene image", "image")
	if err != nil {
		t.Fatalf("AddFileToLibrary(image): %v", err)
	}

	// 3. List — both entries present.
	files, err := ListLibraryFiles(user)
	if err != nil {
		t.Fatalf("ListLibraryFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 library files, got %d", len(files))
	}

	// 4. Load the chat back and confirm content survived encryption round-trip.
	lf, content, err := LoadFileFromLibrary(user, chatID)
	if err != nil {
		t.Fatalf("LoadFileFromLibrary(chat): %v", err)
	}
	if lf.Type != "chat_log" {
		t.Errorf("expected type chat_log, got %q", lf.Type)
	}
	if string(content) != string(convoJSON) {
		t.Errorf("chat content mismatch after round-trip")
	}

	// 5. Update in place (repeated /save) — same ID, new content.
	convo = append(convo, Message{ID: "m3", Sender: "You", Content: "bye", Type: MessageTypeUser})
	convoJSON2, _ := json.MarshalIndent(convo, "", "  ")
	if err := UpdateLibraryFileContent(user, chatID, convoJSON2); err != nil {
		t.Fatalf("UpdateLibraryFileContent: %v", err)
	}
	if after, _ := ListLibraryFiles(user); len(after) != 2 {
		t.Fatalf("update should not add an entry; got %d files", len(after))
	}
	_, content2, err := LoadFileFromLibrary(user, chatID)
	if err != nil {
		t.Fatalf("reload after update: %v", err)
	}
	if string(content2) != string(convoJSON2) {
		t.Errorf("update did not replace content")
	}

	// 6. Export the image out to a plain file and verify bytes match.
	dest := filepath.Join(tmp, "exports")
	outPath, err := ExportLibraryFile(user, imgID, dest)
	if err != nil {
		t.Fatalf("ExportLibraryFile: %v", err)
	}
	if filepath.Ext(outPath) != ".jpg" {
		t.Errorf("expected .jpg export, got %q", outPath)
	}
	exported, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read exported: %v", err)
	}
	if string(exported) != string(imgBytes) {
		t.Errorf("exported image bytes differ from original")
	}

	// 7. Delete the chat entry; only the image remains.
	if err := DeleteFileFromLibrary(user, chatID); err != nil {
		t.Fatalf("DeleteFileFromLibrary: %v", err)
	}
	files, _ = ListLibraryFiles(user)
	if len(files) != 1 || files[0].ID != imgID {
		t.Fatalf("after delete expected only image; got %d files", len(files))
	}
}
