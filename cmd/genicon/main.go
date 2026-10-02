// Génère build/appicon.png (logo NeiBlur, identique au SVG de l'interface) : go run ./cmd/genicon
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
	bg := [3]float64{38, 38, 43}       // #26262b
	accent := [3]float64{240, 138, 60} // #f08a3c
	// traînées : x, y, largeur, opacité (repère 32×32 du SVG, hauteur 2.4)
	bars := []struct{ x, y, w, a float64 }{{5, 11, 9, .35}, {3, 15, 12, .6}, {5, 19, 9, .35}}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					px := (float64(x) + (float64(sx)+.5)/ss) / size * 32
					py := (float64(y) + (float64(sy)+.5)/ss) / size * 32
					if !inRoundRect(px, py, 1, 1, 30, 30, 5) {
						continue
					}
					c := bg
					for _, br := range bars {
						if inRoundRect(px, py, br.x, br.y, br.w, 2.4, 1.2) {
							for k := range c {
								c[k] = c[k]*(1-br.a) + accent[k]*br.a
							}
						}
					}
					if math.Hypot(px-20.5, py-16.2) <= 6.5 {
						c = accent
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
