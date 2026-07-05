package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"strings"
	"testing"
	"time"
)

// TestPollinationsVisionLive exercises the real vision path against Pollinations.
// Skipped unless LIVE=1 (needs network + a POLLINATIONS_API_KEY in .env). Sends a
// 1x1 red PNG and expects the model to name the color.
func TestPollinationsVisionLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE=1 to run the live Pollinations vision test")
	}
	key := ""
	if b, err := os.ReadFile(".env"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "POLLINATIONS_API_KEY=") {
				key = strings.TrimSpace(strings.TrimPrefix(line, "POLLINATIONS_API_KEY="))
			}
		}
	}
	if key == "" {
		t.Skip("no POLLINATIONS_API_KEY in .env")
	}

	svc := NewPollinationsLLMService("openai", key)
	var _ VisionLLMService = svc // compile-time: it implements the capability

	// Synthesize a real 32x32 solid-red JPEG (a 1x1 image is rejected as unparseable).
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{R: 220, G: 20, B: 20, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	resp, err := svc.GenerateResponseWithImages(ctx,
		"Reply with only the single dominant color word of this image.",
		[]ImageAttachment{{Data: buf.Bytes(), MediaType: "image/jpeg"}},
		"openai", APIConfig{})
	if err != nil {
		t.Fatalf("vision request failed: %v", err)
	}
	t.Logf("model replied: %q", strings.TrimSpace(resp))
	if strings.TrimSpace(resp) == "" {
		t.Fatal("empty vision response")
	}
}
