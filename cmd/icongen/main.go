// cmd/icongen generates placeholder icon assets for PGPulse desktop mode.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

func main() {
	outDir := filepath.Join("internal", "desktop", "icons")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}

	icons := []struct {
		name  string
		color color.RGBA
	}{
		{"pgpulse-tray.png", color.RGBA{0x22, 0xc5, 0x5e, 0xff}},         // green
		{"pgpulse-tray-warning.png", color.RGBA{0xea, 0xb3, 0x08, 0xff}},  // yellow
		{"pgpulse-tray-critical.png", color.RGBA{0xef, 0x44, 0x44, 0xff}}, // red
	}

	for _, ic := range icons {
		img := renderCircleWithP(64, ic.color)
		if err := writePNG(filepath.Join(outDir, ic.name), img); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", ic.name, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s\n", filepath.Join(outDir, ic.name))
	}

	// ICO file (32x32 PNG-in-ICO).
	icoImg := renderCircleWithP(32, color.RGBA{0x22, 0xc5, 0x5e, 0xff})
	if err := writeICO(filepath.Join(outDir, "pgpulse.ico"), icoImg); err != nil {
		fmt.Fprintf(os.Stderr, "write ico: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s\n", filepath.Join(outDir, "pgpulse.ico"))
}

// renderCircleWithP draws a filled circle with a simple "P" letter.
func renderCircleWithP(size int, fillColor color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	// Transparent background.
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)

	cx, cy := float64(size)/2, float64(size)/2
	r := float64(size)/2 - 1

	// Draw filled circle.
	for y := range size {
		for x := range size {
			dx := float64(x) - cx + 0.5
			dy := float64(y) - cy + 0.5
			if dx*dx+dy*dy <= r*r {
				img.SetRGBA(x, y, fillColor)
			}
		}
	}

	// Draw "P" using simple rectangles (white).
	white := color.RGBA{0xff, 0xff, 0xff, 0xff}
	drawP(img, size, white)

	return img
}

// drawP draws a simple "P" letter scaled to the image size.
func drawP(img *image.RGBA, size int, c color.RGBA) {
	s := float64(size)
	// Vertical stroke of P.
	x0 := int(math.Round(s * 0.30))
	x1 := int(math.Round(s * 0.38))
	y0 := int(math.Round(s * 0.20))
	y1 := int(math.Round(s * 0.80))
	fillRect(img, x0, y0, x1, y1, c)

	// Top horizontal bar.
	fillRect(img, x1, y0, int(math.Round(s*0.58)), int(math.Round(s*0.28)), c)

	// Right curve of P (approximated as a vertical bar).
	fillRect(img, int(math.Round(s*0.58)), int(math.Round(s*0.20)), int(math.Round(s*0.66)), int(math.Round(s*0.52)), c)

	// Middle horizontal bar.
	fillRect(img, x1, int(math.Round(s*0.44)), int(math.Round(s*0.58)), int(math.Round(s*0.52)), c)
}

func fillRect(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if x >= 0 && x < img.Bounds().Dx() && y >= 0 && y < img.Bounds().Dy() {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// writeICO writes a minimal ICO file with a single PNG-in-ICO entry.
func writeICO(path string, img *image.RGBA) error {
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		return err
	}
	pngData := pngBuf.Bytes()

	var buf bytes.Buffer
	// ICO header: 6 bytes.
	writes := []any{uint16(0), uint16(1), uint16(1)} // reserved, type: ICO, count: 1
	for _, v := range writes {
		if err := binary.Write(&buf, binary.LittleEndian, v); err != nil {
			return fmt.Errorf("ico header: %w", err)
		}
	}
	// Directory entry: 16 bytes.
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	buf.WriteByte(byte(w % 256)) // width (0 means 256)
	buf.WriteByte(byte(h % 256)) // height
	buf.WriteByte(0)             // color palette
	buf.WriteByte(0)             // reserved
	dirEntries := []any{uint16(1), uint16(32), uint32(len(pngData)), uint32(6 + 16)}
	for _, v := range dirEntries {
		if err := binary.Write(&buf, binary.LittleEndian, v); err != nil {
			return fmt.Errorf("ico entry: %w", err)
		}
	}
	// PNG data.
	buf.Write(pngData)

	return os.WriteFile(path, buf.Bytes(), 0o644)
}
