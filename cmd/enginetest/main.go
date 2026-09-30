// Outil de test en ligne de commande du moteur (hors interface).
//
//	go run ./cmd/enginetest install
//	go run ./cmd/enginetest render <video> [preset]
//	go run ./cmd/enginetest preview <video> <t>
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"neiblur/engine"
)

func main() {
	p := engine.DefaultPaths()
	if root := os.Getenv("NEIBLUR_ENGINE"); root != "" {
		p = engine.NewPaths(root)
	}
	ctx := context.Background()
	switch os.Args[1] {
	case "install":
		t0 := time.Now()
		err := engine.Install(ctx, p, func(pr engine.InstallProgress) {
			fmt.Printf("\r[%d/%d] %-32s %5.1f%%", pr.StepIndex, pr.StepCount, pr.Step, pr.Percent)
		})
		fmt.Println()
		check(err)
		fmt.Println("installé en", time.Since(t0).Round(time.Second))
	case "render", "preview":
		check(engine.WriteScripts(p))
		info, err := engine.Probe(ctx, p, os.Args[2])
		check(err)
		fmt.Printf("%+v\n", info)
		s := engine.DefaultSettings()
		if len(os.Args) > 3 && os.Args[1] == "render" {
			for _, pr := range engine.BuiltinPresets() {
				if pr.ID == os.Args[3] {
					s = pr.Settings
				}
			}
		}
		j := engine.Job{Input: os.Args[2], Info: info, Settings: s, Output: engine.OutputPath(os.Args[2], "", s)}
		if os.Args[1] == "preview" {
			t, _ := strconv.ParseFloat(os.Args[3], 64)
			t0 := time.Now()
			b, a, err := engine.PreviewFrames(ctx, p, j, t)
			if re, ok := err.(*engine.RenderError); ok {
				fmt.Println(re.Details())
			}
			check(err)
			check(os.WriteFile("before.jpg", b, 0o644))
			check(os.WriteFile("after.jpg", a, 0o644))
			fmt.Println("aperçu en", time.Since(t0).Round(time.Millisecond))
			return
		}
		t0 := time.Now()
		err = engine.NewRender(p, j).Run(false, func(pr engine.Progress) {
			fmt.Printf("\r%5.1f%% %d/%d %.1f fps eta %.0fs   ", pr.Percent, pr.Frame, pr.Total, pr.FPS, pr.ETA)
		})
		fmt.Println()
		if re, ok := err.(*engine.RenderError); ok {
			fmt.Println(re.Details())
		}
		check(err)
		fmt.Println("rendu en", time.Since(t0).Round(time.Millisecond), "->", j.Output)
	case "gpu":
		fmt.Println(engine.DetectGPUEncoders(ctx, p))
	}
}

func check(err error) {
	if err != nil {
		fmt.Println("ERREUR:", err)
		os.Exit(1)
	}
}
