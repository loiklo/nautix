// Command genicons draws the application icons into web/icons.
// Run from the project root: go run ./tools/genicons
//
// The logo is an air tank inside a progress ring filled to 75%. It is drawn on
// a 100x100 grid, the same one as the SVG proposal it comes from.
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
	blue    = color.RGBA{0x4f, 0x9b, 0xc0, 0xff}
	white   = color.RGBA{0xff, 0xff, 0xff, 0xff}
	ring    = color.RGBA{0x6b, 0xc2, 0x8a, 0xff}
	band1   = color.RGBA{0x4f, 0xae, 0x74, 0xff}
	band2   = color.RGBA{0xa9, 0xd9, 0x9a, 0xff}
	fullArc = 0.75 // share of the ring that is filled
)

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

// roundRect reports whether (x, y) is inside the rounded rectangle.
func roundRect(x, y, x0, y0, w, h, r float64) bool {
	cx := clamp(x, x0+r, x0+w-r)
	cy := clamp(y, y0+r, y0+h-r)
	return math.Hypot(x-cx, y-cy) <= r
}

func circle(x, y, cx, cy, r float64) bool { return math.Hypot(x-cx, y-cy) <= r }

// logoAt returns the color of the logo at (x, y) on the 0..100 grid.
func logoAt(x, y float64) color.RGBA {
	c := blue

	// Progress ring: white track, green filled part starting at the top, clockwise.
	const ringR, ringW = 36.0, 9.0
	dx, dy := x-50, y-50
	if d := math.Hypot(dx, dy); math.Abs(d-ringR) <= ringW/2 {
		c = white
		theta := math.Atan2(dx, -dy)
		if theta < 0 {
			theta += 2 * math.Pi
		}
		if theta <= fullArc*2*math.Pi {
			c = ring
		}
	}
	// Round caps of the filled part.
	end := fullArc * 2 * math.Pi
	if circle(x, y, 50, 50-ringR, ringW/2) || circle(x, y, 50+ringR*math.Sin(end), 50-ringR*math.Cos(end), ringW/2) {
		c = ring
	}

	// Tank: neck, valve, body, two bands, and two bubbles. Spans y 26..74, centered on the ring.
	if roundRect(x, y, 46, 28, 8, 8, 2.5) || roundRect(x, y, 48, 26, 9, 5, 2.5) {
		c = white
	}
	if roundRect(x, y, 41, 34, 18, 40, 9) {
		c = white
		switch {
		case y >= 49 && y < 55:
			c = band1
		case y >= 57 && y < 63:
			c = band2
		}
	}
	if circle(x, y, 67, 45, 2.6) || circle(x, y, 63.5, 37.5, 1.7) {
		c = white
	}
	return c
}

// draw renders the logo at the given size. scale < 1 shrinks the drawing
// around the center (used for the maskable icon, whose edges may be cropped).
func draw(size int, scale float64) image.Image {
	const samples = 4 // supersampling for smooth edges
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var r, g, b float64
			for sy := 0; sy < samples; sy++ {
				for sx := 0; sx < samples; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/samples) / float64(size) * 100
					y := (float64(py) + (float64(sy)+0.5)/samples) / float64(size) * 100
					c := logoAt(50+(x-50)/scale, 50+(y-50)/scale)
					r += float64(c.R)
					g += float64(c.G)
					b += float64(c.B)
				}
			}
			n := float64(samples * samples)
			img.SetRGBA(px, py, color.RGBA{uint8(math.Round(r / n)), uint8(math.Round(g / n)), uint8(math.Round(b / n)), 0xff})
		}
	}
	return img
}

func main() {
	dir := filepath.Join("web", "icons")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatal(err)
	}
	icons := []struct {
		name  string
		size  int
		scale float64
	}{
		{"icon-192.png", 192, 1},
		{"icon-512.png", 512, 1},
		{"apple-touch-icon.png", 180, 1},
		{"icon-maskable-512.png", 512, 0.82}, // everything inside the 80% safe zone
	}
	for _, ic := range icons {
		f, err := os.Create(filepath.Join(dir, ic.name))
		if err != nil {
			log.Fatal(err)
		}
		if err := png.Encode(f, draw(ic.size, ic.scale)); err != nil {
			log.Fatal(err)
		}
		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
	}
}
