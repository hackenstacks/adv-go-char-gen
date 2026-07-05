package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestOpenAIMessageContentPolymorphic verifies the interface{} Content change:
// a plain string still marshals as a JSON string (regression), and a content-part
// array marshals as the OpenAI multimodal shape (text + image_url data URI).
func TestOpenAIMessageContentPolymorphic(t *testing.T) {
	// Plain text message — must serialize content as a bare string.
	txt, _ := json.Marshal(openAIMessage{Role: "user", Content: "hello"})
	if !strings.Contains(string(txt), `"content":"hello"`) {
		t.Errorf("text content should marshal as a string, got %s", txt)
	}

	// Vision message — content is an array with a data URI image.
	att := ImageAttachment{Data: []byte{0xFF, 0xD8, 0xFF, 0x01}, MediaType: "image/jpeg"}
	msg := openAIMessage{Role: "user", Content: []contentPart{
		{Type: "text", Text: "what is this?"},
		{Type: "image_url", ImageURL: &imageURLRef{URL: dataURI(att)}},
	}}
	vis, _ := json.Marshal(msg)
	s := string(vis)
	for _, want := range []string{`"type":"text"`, `"type":"image_url"`, `"image_url"`, "data:image/jpeg;base64,"} {
		if !strings.Contains(s, want) {
			t.Errorf("vision message missing %q in %s", want, s)
		}
	}
	// Text-only parts must not emit an empty image_url.
	if strings.Contains(s, `"image_url":null`) {
		t.Errorf("text part should omit image_url, got %s", s)
	}
}

func TestModelSupportsVision(t *testing.T) {
	vision := []string{"openai", "openai-large", "gpt-5.4", "gemini", "claude", "grok"}
	for _, m := range vision {
		if !modelSupportsVision(m) {
			t.Errorf("%q should be vision-capable", m)
		}
	}
	for _, m := range []string{"deepseek", "llama", ""} {
		if modelSupportsVision(m) {
			t.Errorf("%q should NOT be vision-capable", m)
		}
	}
}

func TestLooksTextualAndMedia(t *testing.T) {
	if !looksTextual([]byte("plain readable document text")) {
		t.Error("plain text should look textual")
	}
	if looksTextual([]byte{0xFF, 0xD8, 0xFF, 0x00, 0x01, 0x02, 0x00, 0x03}) {
		t.Error("binary/jpeg bytes should not look textual")
	}
	if looksTextual(nil) {
		t.Error("empty should not look textual")
	}
	if got := imageMediaType("scene.png"); got != "image/png" {
		t.Errorf("png media type wrong: %s", got)
	}
	if got := imageMediaType("img-123.jpg"); got != "image/jpeg" {
		t.Errorf("jpg media type wrong: %s", got)
	}
}
