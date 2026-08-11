package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strconv"
	"testing"
)

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

// testPNG builds a real PNG so the upload handler's content sniffing sees a
// genuine image.
func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{G: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}
