// Génère build/appicon.png (logo NeiBlur) : go run ./cmd/genicon
package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

func main() {
	const size, ss = 1024, 4 // suréchantillonnage pour l'anticrénelage
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	accent := [3]float64{124, 108, 255}
	circles := []struct{ cx, a float64 }{{9, .2}, {13.5, .45}, {19, 1}}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					// coordonnées dans le repère 32×32 du SVG
					px := (float64(x) + (float64(sx)+.5)/ss) / size * 32
					py := (float64(y) + (float64(sy)+.5)/ss) / size * 32
					if !inRoundRect(px, py, 2, 2, 28, 28, 8) {
						continue
					}
					c := accent
					for _, ci := range circles {
						if math.Hypot(px-ci.cx, py-16) <= 6 {
							for k := range c {
								c[k] = c[k]*(1-ci.a) + 255*ci.a
							}
						}
					}
					r, g, b, a = r+c[0], g+c[1], b+c[2], a+1
				}
			}
			if a > 0 {
				n := float64(ss * ss)
				img.SetNRGBA(x, y, color.NRGBA{uint8(r / a), uint8(g / a), uint8(b / a), uint8(a / n * 255)})
			}
		}
	}
	_ = os.MkdirAll("build", 0o755)
	f, err := os.Create("build/appicon.png")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
}

func inRoundRect(px, py, x, y, w, h, r float64) bool {
	if px < x || py < y || px > x+w || py > y+h {
		return false
	}
	cx := math.Max(x+r, math.Min(px, x+w-r))
	cy := math.Max(y+r, math.Min(py, y+h-r))
	return math.Hypot(px-cx, py-cy) <= r
}
