package engine

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// EngineVersion change quand la composition du moteur change (force une réinstallation).
const EngineVersion = "neiblur-engine-2"

const RifeModelName = "rife-v4.26_ensembleFalse"

//go:embed scripts
var scriptsFS embed.FS

// Mêmes composants (et mêmes versions) que l'installeur officiel de Blur :
// on garde exactement le même moteur, donc les mêmes performances.
type component struct {
	Name   string
	URL    string
	SHA256 string
	Size   int64             // taille approximative, pour la barre de progression
	Kind   string            // "zip", "7z" ou "file"
	Files  map[string]string // chemin dans l'archive -> destination (relative à la racine du moteur)
}

const ffmpegBuild = "ffmpeg-2025-08-14-git-cdbb5f1b93-full_build"

var components = []component{
	{
		Name: "Python 3.12 (embarqué)", Kind: "zip", Size: 11133606, SHA256: "4acbed6dd1c744b0376e3b1cf57ce906f9dc9e95e68824584c8099a63025a3c3",
		URL: "https://www.python.org/ftp/python/3.12.10/python-3.12.10-embed-amd64.zip",
	},
	{
		Name: "VapourSynth R70", Kind: "zip", Size: 9008084, SHA256: "5930dc57f27173bc54fb1ceb743c3841c072ab21044dff3aabb667aeee679540",
		URL: "https://github.com/vapoursynth/vapoursynth/releases/download/R70/VapourSynth64-Portable-R70.zip",
	},
	{
		// OpenMP (vcomp140.dll) requis par RIFE. Blur installe VC_redist (droits admin) ;
		// ici on le déploie « app-local » depuis le paquet officiel msvc-runtime de PyPI.
		Name: "Runtime Visual C++ (OpenMP)", Kind: "zip", Size: 1921899, SHA256: "32f9c706009e16ccc319d6947ce3bffe20e5192bee52b18cf48313f9e7bedfbe",
		URL: "https://files.pythonhosted.org/packages/21/3b/134d04268ab8e35853cd007582076429b45d60d6abb1036d159be9c50342/msvc_runtime-14.44.35112-cp312-cp312-win_amd64.whl",
		Files: map[string]string{
			"msvc_runtime-14.44.35112.data/data/vcomp140.dll": "vapoursynth/vcomp140.dll",
		},
	},
	{
		Name: "FFmpeg", Kind: "7z", Size: 59645245, SHA256: "bb165e0e9103d2c0102fdba02a14339e715199f790a102168a31d4655cee637e",
		URL: "https://github.com/GyanD/codexffmpeg/releases/download/2025-08-14-git-cdbb5f1b93/" + ffmpegBuild + ".7z",
		Files: map[string]string{
			ffmpegBuild + "/bin/ffmpeg.exe":  "ffmpeg/ffmpeg.exe",
			ffmpegBuild + "/bin/ffprobe.exe": "ffmpeg/ffprobe.exe",
		},
	},
	{
		Name: "SVPflow (interpolation GPU)", Kind: "zip", Size: 3303344, SHA256: "02e89dc04b0c29e15a9db7cd23801477a2f672d317a13cb7f2f8577192694c63",
		URL: "https://web.archive.org/web/20190322064557if_/http://www.svp-team.com/files/gpl/svpflow-4.2.0.142.zip",
		Files: map[string]string{
			"svpflow-4.2.0.142/lib-windows/vapoursynth/x64/svpflow1_vs64.dll": "vapoursynth/vs-plugins/svpflow1_vs64.dll",
			"svpflow-4.2.0.142/lib-windows/vapoursynth/x64/svpflow2_vs64.dll": "vapoursynth/vs-plugins/svpflow2_vs64.dll",
		},
	},
	{
		Name: "RIFE (interpolation IA)", Kind: "file", Size: 5244416, SHA256: "36a25b471be88e6f915320c818022dc8657dd9beac22a8c3158bd7f4260cc410",
		URL:   "https://github.com/styler00dollar/VapourSynth-RIFE-ncnn-Vulkan/releases/download/r9_mod_v33/librife_windows_x86-64.dll",
		Files: map[string]string{"": "vapoursynth/vs-plugins/librife.dll"},
	},
	{
		Name: "Modèle RIFE 4.26 (1/2)", Kind: "file", Size: 11360684, SHA256: "94d58e30b75d7c7609cfa6f3bdad524deddd14f5f75e85c36d2f827ef5c64731",
		URL:   "https://raw.githubusercontent.com/styler00dollar/VapourSynth-RIFE-ncnn-Vulkan/c3ec6aabc07c8fa37a4f58d7fed9e2ad1fc1b13f/models/" + RifeModelName + "/flownet.bin",
		Files: map[string]string{"": "models/" + RifeModelName + "/flownet.bin"},
	},
	{
		Name: "Modèle RIFE 4.26 (2/2)", Kind: "file", Size: 67885, SHA256: "79f16c28903f93f8308f0c4c947f8c7e0c17d99a57b85473e5f298dd578d137b",
		URL:   "https://raw.githubusercontent.com/styler00dollar/VapourSynth-RIFE-ncnn-Vulkan/c3ec6aabc07c8fa37a4f58d7fed9e2ad1fc1b13f/models/" + RifeModelName + "/flownet.param",
		Files: map[string]string{"": "models/" + RifeModelName + "/flownet.param"},
	},
	{
		Name: "Akarin (mélange d'images)", Kind: "7z", Size: 5565383, SHA256: "1b28b448b4b5a5d0cc2a715433ea4940ef4dcb24b1244f43bdae5ec48a83db60",
		URL:   "https://github.com/AkarinVS/vapoursynth-plugin/releases/download/v0.96/akarin-release-lexpr-amd64-v0.96g3.7z",
		Files: map[string]string{"akarin.dll": "vapoursynth/vs-plugins/akarin.dll"},
	},
	{
		Name: "L-SMASH Works (décodage)", Kind: "7z", Size: 10562462, SHA256: "30569c059b2be3b573b81c958516c76319233efc81fa77ba58d03fe3d420e2de",
		URL:   "https://github.com/HomeOfAviSynthPlusEvolution/L-SMASH-Works/releases/download/1194.0.0.0/L-SMASH-Works-r1194.0.0.0.7z",
		Files: map[string]string{"x64/LSMASHSource.dll": "vapoursynth/vs-plugins/LSMASHSource.dll"},
	},
	{
		Name: "Adjust (filtres couleur)", Kind: "file", Size: 29184, SHA256: "e303847d434200672da3e5df15d8ad63b7b92d8f6c65ec799a2eb16cb564cca0",
		URL:   "https://github.com/f0e/Vapoursynth-adjust/releases/download/v1/adjust.dll",
		Files: map[string]string{"": "vapoursynth/vs-plugins/adjust.dll"},
	},
}

type InstallProgress struct {
	Step       string  `json:"step"`
	StepIndex  int     `json:"stepIndex"`
	StepCount  int     `json:"stepCount"`
	Percent    float64 `json:"percent"`
	Downloaded int64   `json:"downloaded"`
	Total      int64   `json:"total"`
}

func TotalDownloadSize() int64 {
	var t int64
	for _, c := range components {
		t += c.Size
	}
	return t
}

// Install télécharge et assemble le moteur. Réentrant : une installation partielle est reprise proprement.
func Install(ctx context.Context, p Paths, onProgress func(InstallProgress)) error {
	if err := os.MkdirAll(p.Root, 0o755); err != nil {
		return err
	}
	_ = os.Remove(p.Marker)
	tmp := filepath.Join(p.Root, "_download")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	total := TotalDownloadSize()
	var done int64
	for i, c := range components {
		report := func(cur int64) {
			pct := float64(done+min(cur, c.Size)) / float64(total) * 100
			onProgress(InstallProgress{Step: c.Name, StepIndex: i + 1, StepCount: len(components), Percent: pct, Downloaded: done + cur, Total: total})
		}
		report(0)
		file := filepath.Join(tmp, fmt.Sprintf("%02d%s", i, filepath.Ext(c.URL)))
		if err := download(ctx, c.URL, file, c.SHA256, report); err != nil {
			return fmt.Errorf("%s : %w", c.Name, err)
		}
		if err := installComponent(ctx, p, i, c, file, tmp); err != nil {
			return fmt.Errorf("%s : %w", c.Name, err)
		}
		_ = os.Remove(file)
		done += c.Size
	}
	if err := WriteScripts(p); err != nil {
		return err
	}
	onProgress(InstallProgress{Step: "Terminé", StepIndex: len(components), StepCount: len(components), Percent: 100, Downloaded: total, Total: total})
	return os.WriteFile(p.Marker, []byte(EngineVersion), 0o644)
}

func installComponent(ctx context.Context, p Paths, idx int, c component, file, tmp string) error {
	switch {
	case idx == 0: // Python embarqué : tout dans le dossier VapourSynth
		if err := unzipAll(file, p.VS, nil); err != nil {
			return err
		}
		pth := filepath.Join(p.VS, "python312._pth")
		b, err := os.ReadFile(pth)
		if err != nil {
			return err
		}
		if !strings.Contains(string(b), "vs-scripts") {
			b = append(b, []byte("\r\nvs-scripts\r\nLib\\site-packages\r\n")...)
		}
		return os.WriteFile(pth, b, 0o644)

	case idx == 1: // VapourSynth portable + installation manuelle du wheel (équivalent de pip install)
		skip := func(name string) bool {
			return strings.HasPrefix(name, "doc/") || strings.HasPrefix(name, "sdk/") || name == "VSScriptPython38.dll" || strings.Contains(name, "cp38")
		}
		if err := unzipAll(file, p.VS, skip); err != nil {
			return err
		}
		for _, d := range []string{"vs-plugins", "vs-scripts", "Lib/site-packages"} {
			_ = os.MkdirAll(filepath.Join(p.VS, d), 0o755)
		}
		wheel := filepath.Join(p.VS, "wheel", "VapourSynth-70-cp312-cp312-win_amd64.whl")
		site := filepath.Join(p.VS, "Lib", "site-packages")
		err := unzipMap(wheel, func(name string) string {
			if rest, ok := strings.CutPrefix(name, "VapourSynth-70.data/data/"); ok {
				return filepath.Join(p.VS, filepath.FromSlash(rest))
			}
			return filepath.Join(site, filepath.FromSlash(name))
		})
		if err != nil {
			return err
		}
		return os.RemoveAll(filepath.Join(p.VS, "wheel"))

	case c.Kind == "file":
		return moveFile(file, filepath.Join(p.Root, filepath.FromSlash(c.Files[""])))

	case c.Kind == "zip":
		return unzipMap(file, func(name string) string {
			if dst, ok := c.Files[name]; ok {
				return filepath.Join(p.Root, filepath.FromSlash(dst))
			}
			return ""
		})

	case c.Kind == "7z":
		out := filepath.Join(tmp, "x7z")
		_ = os.RemoveAll(out)
		defer os.RemoveAll(out)
		args := []string{"x", file, "-o" + out, "-y"}
		for src := range c.Files {
			args = append(args, filepath.FromSlash(src))
		}
		cmd := exec.CommandContext(ctx, p.SevenZip, args...)
		cmd.SysProcAttr = hiddenProc(false)
		if b, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("extraction 7z : %v\n%s", err, b)
		}
		for src, dst := range c.Files {
			if err := moveFile(filepath.Join(out, filepath.FromSlash(src)), filepath.Join(p.Root, filepath.FromSlash(dst))); err != nil {
				return err
			}
		}
		return nil
	}
	return errors.New("type de composant inconnu")
}

// WriteScripts (ré)écrit les scripts VapourSynth embarqués dans l'exe.
func WriteScripts(p Paths) error {
	return fs.WalkDir(scriptsFS, "scripts", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := scriptsFS.ReadFile(path)
		if err != nil {
			return err
		}
		dst := filepath.Join(p.Lib, filepath.FromSlash(strings.TrimPrefix(path, "scripts/")))
		if old, err := os.ReadFile(dst); err == nil && string(old) == string(b) {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
}

var httpClient = &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, ResponseHeaderTimeout: 60 * time.Second}}

func download(ctx context.Context, url, dst, wantSHA string, report func(int64)) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt*2) * time.Second):
			}
		}
		lastErr = downloadOnce(ctx, url, dst, wantSHA, report)
		if lastErr == nil || ctx.Err() != nil {
			return lastErr
		}
	}
	return lastErr
}

func downloadOnce(ctx context.Context, url, dst, wantSHA string, report func(int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "NeiBlur")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("téléchargement impossible (HTTP %d)", resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	h := sha256.New()
	var n int64
	buf := make([]byte, 256*1024)
	last := time.Now()
	for {
		k, rerr := resp.Body.Read(buf)
		if k > 0 {
			if _, werr := f.Write(buf[:k]); werr != nil {
				f.Close()
				return werr
			}
			h.Write(buf[:k])
			n += int64(k)
			if time.Since(last) > 100*time.Millisecond {
				report(n)
				last = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return rerr
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	if wantSHA != "" {
		if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, wantSHA) {
			return fmt.Errorf("fichier corrompu (SHA-256 %s)", got)
		}
	}
	report(n)
	return nil
}

func unzipAll(file, dstDir string, skip func(string) bool) error {
	return unzipMap(file, func(name string) string {
		if skip != nil && skip(name) {
			return ""
		}
		return filepath.Join(dstDir, filepath.FromSlash(name))
	})
}

// unzipMap extrait chaque entrée vers le chemin renvoyé par dest ("" = ignorer).
func unzipMap(file string, dest func(name string) string) error {
	r, err := zip.OpenReader(file)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		dst := dest(f.Name)
		if dst == "" {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(dst)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func moveFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	_ = os.Remove(dst)
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
