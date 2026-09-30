package engine

import (
	"os"
	"path/filepath"
)

// Paths décrit l'emplacement de tous les composants du moteur de rendu.
type Paths struct {
	Root      string // dossier racine du moteur
	VS        string // VapourSynth portable (+ Python embarqué)
	VSPipe    string
	SevenZip  string
	Plugins   string
	FFmpeg    string
	FFprobe   string
	RifeModel string
	Lib       string // scripts VapourSynth (neiblur.py + paquet blur/)
	Script    string
	Marker    string // fichier écrit quand l'installation est complète
}

// AppDataDir renvoie le dossier de données de l'application.
// Mode portable : si un dossier "NeiBlur-data" existe à côté de l'exe, il est utilisé.
func AppDataDir() string {
	if d := os.Getenv("NEIBLUR_DATA"); d != "" {
		return d
	}
	if exe, err := os.Executable(); err == nil {
		portable := filepath.Join(filepath.Dir(exe), "NeiBlur-data")
		if st, err := os.Stat(portable); err == nil && st.IsDir() {
			return portable
		}
	}
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base, _ = os.UserConfigDir()
	}
	return filepath.Join(base, "NeiBlur")
}

func NewPaths(root string) Paths {
	vs := filepath.Join(root, "vapoursynth")
	ff := filepath.Join(root, "ffmpeg")
	lib := filepath.Join(root, "lib")
	return Paths{
		Root:      root,
		VS:        vs,
		VSPipe:    filepath.Join(vs, "VSPipe.exe"),
		SevenZip:  filepath.Join(vs, "7z.exe"),
		Plugins:   filepath.Join(vs, "vs-plugins"),
		FFmpeg:    filepath.Join(ff, "ffmpeg.exe"),
		FFprobe:   filepath.Join(ff, "ffprobe.exe"),
		RifeModel: filepath.Join(root, "models", RifeModelName),
		Lib:       lib,
		Script:    filepath.Join(lib, "neiblur.py"),
		Marker:    filepath.Join(root, "engine.json"),
	}
}

func DefaultPaths() Paths { return NewPaths(filepath.Join(AppDataDir(), "engine")) }

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// Installed indique si le moteur est complet et à la bonne version.
func (p Paths) Installed() bool {
	b, err := os.ReadFile(p.Marker)
	if err != nil || string(b) != EngineVersion {
		return false
	}
	for _, f := range []string{p.VSPipe, p.FFmpeg, p.FFprobe, filepath.Join(p.RifeModel, "flownet.bin")} {
		if !exists(f) {
			return false
		}
	}
	return true
}
