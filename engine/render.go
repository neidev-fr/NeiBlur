package engine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------- Analyse de la vidéo ----------

type VideoInfo struct {
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	FPSNum         int     `json:"fpsNum"`
	FPSDen         int     `json:"fpsDen"`
	FPS            float64 `json:"fps"`
	Duration       float64 `json:"duration"`
	Codec          string  `json:"codec"`
	PixFmt         string  `json:"pixFmt"`
	ColorRange     string  `json:"colorRange"`
	ColorSpace     string  `json:"colorSpace"`
	ColorTransfer  string  `json:"colorTransfer"`
	ColorPrimaries string  `json:"colorPrimaries"`
	SampleRate     int     `json:"sampleRate"`
	Size           int64   `json:"size"`
}

// vsEnv prépare l'environnement de VSPipe :
//   - un PYTHONHOME/PYTHONPATH système casserait le Python embarqué ;
//   - le dossier VapourSynth (qui contient le runtime Visual C++) est mis en tête du PATH, pour que
//     les plugins (RIFE…) trouvent msvcp140.dll sans installer VC_redist (déploiement « app-local »).
func vsEnv(p Paths) []string {
	out := make([]string, 0, len(os.Environ())+1)
	path := p.VS
	for _, e := range os.Environ() {
		u := strings.ToUpper(e)
		switch {
		case strings.HasPrefix(u, "PYTHONHOME="), strings.HasPrefix(u, "PYTHONPATH="):
			continue
		case strings.HasPrefix(u, "PATH="):
			path += ";" + e[5:]
			continue
		}
		out = append(out, e)
	}
	return append(out, "PATH="+path)
}

func Probe(ctx context.Context, p Paths, path string) (VideoInfo, error) {
	cmd := exec.CommandContext(ctx, p.FFprobe, "-v", "error", "-print_format", "json",
		"-show_entries", "stream=codec_type,codec_name,width,height,r_frame_rate,pix_fmt,color_range,color_space,color_transfer,color_primaries,sample_rate,duration",
		"-show_entries", "format=duration,size", path)
	cmd.SysProcAttr = hiddenProc(false)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return VideoInfo{}, fmt.Errorf("fichier illisible : %s", strings.TrimSpace(stderr.String()))
	}
	var r struct {
		Streams []struct {
			CodecType      string `json:"codec_type"`
			CodecName      string `json:"codec_name"`
			RFrameRate     string `json:"r_frame_rate"`
			PixFmt         string `json:"pix_fmt"`
			Duration       string `json:"duration"`
			ColorRange     string `json:"color_range"`
			ColorSpace     string `json:"color_space"`
			ColorTransfer  string `json:"color_transfer"`
			ColorPrimaries string `json:"color_primaries"`
			SampleRate     string `json:"sample_rate"`
			Width          int    `json:"width"`
			Height         int    `json:"height"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
			Size     string `json:"size"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return VideoInfo{}, err
	}
	var info VideoInfo
	found := false
	for _, s := range r.Streams {
		switch s.CodecType {
		case "video":
			if found {
				continue
			}
			found = true
			info.Codec, info.PixFmt, info.Width, info.Height = s.CodecName, s.PixFmt, s.Width, s.Height
			info.ColorRange, info.ColorSpace, info.ColorTransfer, info.ColorPrimaries = s.ColorRange, s.ColorSpace, s.ColorTransfer, s.ColorPrimaries
			if n, d, ok := strings.Cut(s.RFrameRate, "/"); ok {
				info.FPSNum, _ = strconv.Atoi(n)
				info.FPSDen, _ = strconv.Atoi(d)
			}
			if info.FPSDen > 0 {
				info.FPS = float64(info.FPSNum) / float64(info.FPSDen)
			}
			info.Duration, _ = strconv.ParseFloat(s.Duration, 64)
		case "audio":
			if info.SampleRate == 0 {
				info.SampleRate, _ = strconv.Atoi(s.SampleRate)
			}
		}
	}
	if !found {
		return info, errors.New("aucune piste vidéo dans ce fichier")
	}
	if d, err := strconv.ParseFloat(r.Format.Duration, 64); err == nil && d > 0 {
		info.Duration = d
	}
	info.Size, _ = strconv.ParseInt(r.Format.Size, 10, 64)
	if info.Duration < 0.1 {
		return info, errors.New("ce fichier n'est pas une vidéo (durée nulle)")
	}
	return info, nil
}

// ---------- Construction des commandes (identiques à Blur) ----------

type Job struct {
	Input    string
	Output   string
	Info     VideoInfo
	Settings Settings
	GPUType  string
}

func (j Job) vspipeArgs(p Paths, extra ...string) ([]string, error) {
	settings, err := j.Settings.blurJSON(p, j.GPUType)
	if err != nil {
		return nil, err
	}
	colorRange := j.Info.ColorRange
	if colorRange == "" {
		colorRange = "undefined"
	}
	args := []string{"-p", "-c", "y4m"}
	args = append(args, extra...)
	args = append(args,
		"-a", "video_path="+strings.ReplaceAll(j.Input, `\`, "/"),
		"-a", fmt.Sprintf("fps_num=%d", j.Info.FPSNum),
		"-a", fmt.Sprintf("fps_den=%d", j.Info.FPSDen),
		"-a", "color_range="+colorRange,
		"-a", "settings="+settings,
		"-a", "enable_lsmash=true",
		p.Script, "-")
	return args, nil
}

func (j Job) ffmpegArgs() []string {
	s := j.Settings
	args := []string{"-loglevel", "error", "-hide_banner", "-nostats", "-y",
		"-i", "-", "-fflags", "+genpts", "-i", j.Input, "-map", "0:v", "-map", "1:a?"}

	// vspipe perd les métadonnées couleur : on les rétablit (comme Blur)
	var params []string
	if j.Info.ColorRange != "" && j.Info.ColorRange != "unknown" {
		r := "limited"
		if j.Info.ColorRange == "pc" {
			r = "full"
		}
		params = append(params, "range="+r)
	}
	for k, v := range map[string]string{"colorspace": j.Info.ColorSpace, "color_trc": j.Info.ColorTransfer, "color_primaries": j.Info.ColorPrimaries} {
		if v != "" && v != "unknown" {
			params = append(params, k+"="+v)
		}
	}
	if len(params) > 0 {
		args = append(args, "-vf", "setparams="+strings.Join(params, ":"))
	}
	if j.Info.PixFmt != "" {
		args = append(args, "-pix_fmt", j.Info.PixFmt)
	}

	sr := j.Info.SampleRate
	if sr <= 0 {
		sr = 48000
	}
	var af []string
	if s.InputTimescale != 1 {
		af = append(af, fmt.Sprintf("asetrate=%d*%g", sr, 1/s.InputTimescale), "aresample=48000")
	}
	if s.OutputTimescale != 1 {
		if s.AudioPitch {
			af = append(af, fmt.Sprintf("asetrate=%d*%g", sr, s.OutputTimescale), "aresample=48000")
		} else {
			af = append(af, fmt.Sprintf("atempo=%g", s.OutputTimescale))
		}
	}
	if len(af) > 0 {
		args = append(args, "-af", strings.Join(af, ","))
	}
	args = append(args, encoderArgs(s.Codec, j.GPUType, s.GPUEncoding, s.Quality)...)
	args = append(args, "-c:a", "aac", "-b:a", "320k", "-movflags", "+faststart", j.Output)
	return args
}

// ---------- Rendu ----------

type Progress struct {
	Frame   int     `json:"frame"`
	Total   int     `json:"total"`
	FPS     float64 `json:"fps"`
	ETA     float64 `json:"eta"` // secondes
	Percent float64 `json:"percent"`
}

type Render struct {
	job      Job
	paths    Paths
	mu       sync.Mutex
	vs, ff   *exec.Cmd
	paused   bool
	stopped  bool
	pausedAt time.Time
	pausedD  time.Duration
}

func NewRender(p Paths, j Job) *Render { return &Render{job: j, paths: p} }

var frameRe = regexp.MustCompile(`Frame: (\d+)/(\d+)`)

// ErrStopped est renvoyée quand l'utilisateur annule le rendu.
var ErrStopped = errors.New("rendu annulé")

func (r *Render) Run(lowPriority bool, onProgress func(Progress)) error {
	vsArgs, err := r.job.vspipeArgs(r.paths)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(r.job.Output), 0o755); err != nil {
		return err
	}
	pr, pw, err := bigPipe()
	if err != nil {
		return err
	}

	vs := exec.Command(r.paths.VSPipe, vsArgs...)
	vs.Dir = r.paths.Lib
	vs.Env = vsEnv(r.paths)
	vs.SysProcAttr = hiddenProc(lowPriority)
	vs.Stdout = pw
	vsErr, _ := vs.StderrPipe()

	ff := exec.Command(r.paths.FFmpeg, r.job.ffmpegArgs()...)
	ff.SysProcAttr = hiddenProc(lowPriority)
	ff.Stdin = pr
	var ffLog limitedBuffer
	ff.Stderr = &ffLog

	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		pr.Close()
		pw.Close()
		return ErrStopped
	}
	if err := vs.Start(); err != nil {
		r.mu.Unlock()
		pr.Close()
		pw.Close()
		return fmt.Errorf("impossible de lancer VSPipe : %w", err)
	}
	if err := ff.Start(); err != nil {
		r.mu.Unlock()
		_ = vs.Process.Kill()
		pr.Close()
		pw.Close()
		return fmt.Errorf("impossible de lancer FFmpeg : %w", err)
	}
	r.vs, r.ff = vs, ff
	r.mu.Unlock()
	pr.Close() // les processus enfants ont leurs propres copies
	pw.Close()

	// VSPipe écrit « Frame: n/total (x fps) » terminé par \r
	var vsLog limitedBuffer
	start := time.Now()
	startFrame := -1
	var smoothed float64
	var last time.Time
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		br := bufio.NewReaderSize(vsErr, 4096)
		var line []byte
		for {
			b, err := br.ReadByte()
			if err != nil {
				vsLog.Write(line)
				return
			}
			if b != '\r' && b != '\n' {
				line = append(line, b)
				continue
			}
			if m := frameRe.FindSubmatch(line); m != nil {
				cur, _ := strconv.Atoi(string(m[1]))
				total, _ := strconv.Atoi(string(m[2]))
				if startFrame < 0 {
					startFrame, start = cur, time.Now()
				}
				if time.Since(last) >= 250*time.Millisecond || cur == total {
					last = time.Now()
					elapsed := time.Since(start) - r.pausedDuration()
					var fps float64
					if elapsed > 0 && cur > startFrame {
						fps = float64(cur-startFrame) / elapsed.Seconds()
					}
					// moyenne mobile exponentielle : un temps restant stable
					if smoothed == 0 {
						smoothed = fps
					} else if fps > 0 {
						smoothed = smoothed*0.8 + fps*0.2
					}
					eta := -1.0
					if smoothed > 0 {
						eta = float64(total-cur) / smoothed
					}
					pct := 0.0
					if total > 0 {
						pct = float64(cur) / float64(total) * 100
					}
					onProgress(Progress{Frame: cur, Total: total, FPS: math.Round(fps*10) / 10, ETA: eta, Percent: pct})
				}
			} else if len(line) > 0 {
				vsLog.Write(line)
				vsLog.Write([]byte{'\n'})
			}
			line = line[:0]
		}
	}()

	vsWaitErr := vs.Wait()
	<-readDone
	ffWaitErr := ff.Wait()

	r.mu.Lock()
	stopped := r.stopped
	r.mu.Unlock()
	if stopped {
		_ = os.Remove(r.job.Output)
		return ErrStopped
	}
	if vsWaitErr != nil || ffWaitErr != nil {
		_ = os.Remove(r.job.Output)
		return &RenderError{VSPipe: vsLog.String(), FFmpeg: ffLog.String()}
	}
	if r.job.Settings.CopyDates {
		if st, err := os.Stat(r.job.Input); err == nil {
			_ = os.Chtimes(r.job.Output, st.ModTime(), st.ModTime())
		}
	}
	return nil
}

func (r *Render) pausedDuration() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	d := r.pausedD
	if r.paused {
		d += time.Since(r.pausedAt)
	}
	return d
}

// Pause suspend FFmpeg ; VSPipe se bloque de lui-même sur le tube plein (comme Blur).
func (r *Render) Pause() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.paused || r.ff == nil {
		return nil
	}
	if err := setSuspended(r.ff.Process, true); err != nil {
		return err
	}
	_ = setSuspended(r.vs.Process, true)
	r.paused, r.pausedAt = true, time.Now()
	return nil
}

func (r *Render) Resume() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.paused {
		return nil
	}
	_ = setSuspended(r.vs.Process, false)
	if err := setSuspended(r.ff.Process, false); err != nil {
		return err
	}
	r.paused = false
	r.pausedD += time.Since(r.pausedAt)
	return nil
}

func (r *Render) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopped = true
	for _, c := range []*exec.Cmd{r.vs, r.ff} {
		if c != nil && c.Process != nil {
			if r.paused {
				_ = setSuspended(c.Process, false)
			}
			_ = c.Process.Kill()
		}
	}
}

type RenderError struct{ VSPipe, FFmpeg string }

func (e *RenderError) Error() string {
	return FriendlyError(e.VSPipe + "\n" + e.FFmpeg)
}

func (e *RenderError) Details() string {
	return "--- VSPipe ---\n" + strings.TrimSpace(e.VSPipe) + "\n\n--- FFmpeg ---\n" + strings.TrimSpace(e.FFmpeg)
}

// FriendlyError traduit les erreurs techniques les plus courantes en message compréhensible.
func FriendlyError(log string) string {
	l := strings.ToLower(log)
	switch {
	case strings.Contains(l, "nvenc") || strings.Contains(l, "amf") || strings.Contains(l, "qsv") && strings.Contains(l, "error"):
		return "L'encodage GPU a échoué. Désactive « Encodage GPU » dans les réglages avancés ou mets à jour ton pilote graphique."
	case strings.Contains(l, "rife") && (strings.Contains(l, "vulkan") || strings.Contains(l, "gpu")):
		return "RIFE n'a pas pu utiliser la carte graphique (Vulkan). Essaie le préréglage « Gaming fluide » (SVP)."
	case strings.Contains(l, "no space left") || strings.Contains(l, "not enough space"):
		return "Plus assez d'espace disque pour écrire la vidéo."
	case strings.Contains(l, "permission denied"):
		return "Accès refusé au dossier de sortie. Choisis un autre dossier dans les réglages."
	case strings.Contains(l, "invalid fps") || strings.Contains(l, "fps multiplier"):
		return "Valeur d'images/s invalide dans les réglages d'interpolation (ex. : 1200 ou 5x)."
	case strings.Contains(l, "no such file") || strings.Contains(l, "failed to open"):
		return "La vidéo source est introuvable ou illisible."
	}
	return "Le rendu a échoué. Ouvre les détails pour voir le journal technique."
}

// ---------- Aperçu (une seule image, calculée à la demande par VapourSynth) ----------

func (j Job) outputFrameAt(t float64) int {
	s := j.Settings
	n := int(t * s.InputTimescale / s.OutputTimescale * float64(s.OutputFPS))
	if n < 1 {
		n = 1
	}
	return n
}

// PreviewFrames renvoie (avant, après) en JPEG pour l'instant t (secondes dans la source).
func PreviewFrames(ctx context.Context, p Paths, j Job, t float64) (before, after []byte, err error) {
	scale := "scale='min(1280,iw)':-2"
	var wg sync.WaitGroup
	var errB error
	wg.Add(1)
	go func() {
		defer wg.Done()
		cmd := exec.CommandContext(ctx, p.FFmpeg, "-loglevel", "error", "-ss", fmt.Sprintf("%.3f", t), "-i", j.Input,
			"-frames:v", "1", "-vf", scale, "-q:v", "3", "-f", "image2pipe", "-c:v", "mjpeg", "-")
		cmd.SysProcAttr = hiddenProc(false)
		before, errB = cmd.Output()
	}()

	n := j.outputFrameAt(t)
	vsArgs, err := j.vspipeArgs(p, "-s", strconv.Itoa(n), "-e", strconv.Itoa(n))
	if err != nil {
		return nil, nil, err
	}
	pr, pw, err := bigPipe()
	if err != nil {
		return nil, nil, err
	}
	vs := exec.CommandContext(ctx, p.VSPipe, vsArgs...)
	vs.Dir, vs.Env, vs.SysProcAttr, vs.Stdout = p.Lib, vsEnv(p), hiddenProc(false), pw
	var vsLog limitedBuffer
	vs.Stderr = &vsLog
	ff := exec.CommandContext(ctx, p.FFmpeg, "-loglevel", "error", "-i", "-", "-frames:v", "1", "-vf", scale,
		"-q:v", "3", "-f", "image2pipe", "-c:v", "mjpeg", "-")
	ff.SysProcAttr, ff.Stdin = hiddenProc(false), pr
	var out bytes.Buffer
	ff.Stdout = &out
	if err := vs.Start(); err != nil {
		pr.Close()
		pw.Close()
		return nil, nil, err
	}
	if err := ff.Start(); err != nil {
		_ = vs.Process.Kill()
		pr.Close()
		pw.Close()
		return nil, nil, err
	}
	pr.Close()
	pw.Close()
	vsE := vs.Wait()
	ffE := ff.Wait()
	wg.Wait()
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}
	if vsE != nil || ffE != nil || out.Len() == 0 {
		return nil, nil, &RenderError{VSPipe: vsLog.String()}
	}
	if errB != nil {
		return nil, out.Bytes(), nil
	}
	return before, out.Bytes(), nil
}

// Thumbnail : petite vignette pour la file d'attente.
func Thumbnail(ctx context.Context, p Paths, path string, dur float64) ([]byte, error) {
	cmd := exec.CommandContext(ctx, p.FFmpeg, "-loglevel", "error", "-ss", fmt.Sprintf("%.2f", dur*0.3), "-i", path,
		"-frames:v", "1", "-vf", "scale=160:-2", "-q:v", "5", "-f", "image2pipe", "-c:v", "mjpeg", "-")
	cmd.SysProcAttr = hiddenProc(true)
	return cmd.Output()
}

// ---------- Détection du GPU pour l'encodage ----------

// DetectGPUEncoders teste réellement chaque encodeur matériel (la simple liste de FFmpeg ne suffit pas).
func DetectGPUEncoders(ctx context.Context, p Paths) map[string]bool {
	res := map[string]bool{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for gpu, enc := range map[string]string{"nvidia": "h264_nvenc", "amd": "h264_amf", "intel": "h264_qsv"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(cctx, p.FFmpeg, "-loglevel", "error", "-f", "lavfi", "-i", "color=black:s=256x256:d=0.2",
				"-c:v", enc, "-f", "null", "-")
			cmd.SysProcAttr = hiddenProc(false)
			ok := cmd.Run() == nil
			mu.Lock()
			res[gpu] = ok
			mu.Unlock()
		}()
	}
	wg.Wait()
	return res
}

// CheckRIFE vérifie que RIFE produit des images valides sur ce GPU (quelques secondes).
func CheckRIFE(ctx context.Context, p Paths) bool {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, p.VSPipe, "-i", "-a", "model_path="+strings.ReplaceAll(p.RifeModel, `\`, "/"),
		filepath.Join(p.Lib, "rifecheck.py"))
	cmd.Dir, cmd.Env, cmd.SysProcAttr = p.Lib, vsEnv(p), hiddenProc(true)
	out, _ := cmd.CombinedOutput()
	return bytes.Contains(out, []byte("RIFE_OK"))
}

// ---------- Utilitaires ----------

// limitedBuffer conserve seulement la fin du journal (évite une mémoire illimitée sur de longs rendus).
type limitedBuffer struct {
	mu  sync.Mutex
	buf []byte
}

const logLimit = 64 * 1024

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > logLimit {
		b.buf = append([]byte(nil), b.buf[len(b.buf)-logLimit:]...)
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

var _ io.Writer = (*limitedBuffer)(nil)

// OutputPath construit un nom de sortie libre : « nom - blur.mp4 », « nom - blur (2).mp4 »…
func OutputPath(input, outDir string, s Settings) string {
	if outDir == "" {
		outDir = filepath.Dir(input)
	}
	base := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input)) + " - blur"
	if s.DetailedFilenames {
		if s.Interpolate {
			base += fmt.Sprintf(" ~ %dfps (%s, %g)", s.OutputFPS, s.InterpolatedFPS, s.BlurAmount)
		} else {
			base += fmt.Sprintf(" ~ %dfps (%g)", s.OutputFPS, s.BlurAmount)
		}
	}
	for i := 1; ; i++ {
		name := base
		if i > 1 {
			name += fmt.Sprintf(" (%d)", i)
		}
		out := filepath.Join(outDir, name+".mp4")
		if !exists(out) {
			return out
		}
	}
}
