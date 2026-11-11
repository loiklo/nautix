// Command genicons draws the application icons into web/icons.
// Run from the project root: go run ./tools/genicons
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

var (
	background = color.RGBA{0x8f, 0xc7, 0xdf, 0xff} // pastel blue
	foreground = color.RGBA{0xff, 0xff, 0xff, 0xff}
)

// inShape reports whether the point (x, y), in 0..1 coordinates, is part of
// the white drawing: a letter N and two air bubbles. Everything stays inside
// the central safe zone so the icon can also be used as a maskable icon.
func inShape(x, y float64) bool {
	const top, bottom, left, right, bar = 0.30, 0.70, 0.30, 0.70, 0.095
	if y >= top && y <= bottom {
		if (x >= left && x <= left+bar) || (x >= right-bar && x <= right) {
			return true
		}
		diag := left + (y-top)/(bottom-top)*(right-left-bar)
		if x >= diag && x <= diag+bar {
			return true
		}
	}
	circle := func(cx, cy, r float64) bool { return math.Hypot(x-cx, y-cy) <= r }
	return circle(0.775, 0.235, 0.036) || circle(0.715, 0.185, 0.022)
}

func draw(size int) image.Image {
	const samples = 4 // supersampling for smooth edges
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			hits := 0
			for sy := 0; sy < samples; sy++ {
				for sx := 0; sx < samples; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/samples) / float64(size)
					y := (float64(py) + (float64(sy)+0.5)/samples) / float64(size)
					if inShape(x, y) {
						hits++
					}
				}
			}
			t := float64(hits) / (samples * samples)
			mix := func(a, b uint8) uint8 { return uint8(math.Round(float64(a)*(1-t) + float64(b)*t)) }
			img.SetRGBA(px, py, color.RGBA{
				mix(background.R, foreground.R), mix(background.G, foreground.G), mix(background.B, foreground.B), 0xff,
			})
		}
	}
	return img
}

func main() {
	dir := filepath.Join("web", "icons")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatal(err)
	}
	for name, size := range map[string]int{"icon-192.png": 192, "icon-512.png": 512, "apple-touch-icon.png": 180} {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			log.Fatal(err)
		}
		if err := png.Encode(f, draw(size)); err != nil {
			log.Fatal(err)
		}
		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
	}
}
