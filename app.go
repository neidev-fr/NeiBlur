package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"neiblur/engine"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ---------- Configuration persistante ----------

type Config struct {
	PresetID     string          `json:"presetId"`
	Settings     engine.Settings `json:"settings"`
	OutputDir    string          `json:"outputDir"`
	LowPriority  bool            `json:"lowPriority"`
	KeepAwake    bool            `json:"keepAwake"`
	Notify       bool            `json:"notify"`
	RevealOnDone bool            `json:"revealOnDone"`
	GPUType      string          `json:"gpuType"`
	UserPresets  []engine.Preset `json:"userPresets"`
	SeenWelcome  bool            `json:"seenWelcome"`
}

func defaultConfig() Config {
	return Config{PresetID: "gaming", Settings: engine.DefaultSettings(), LowPriority: true, KeepAwake: true, Notify: true}
}

func configPath() string    { return filepath.Join(engine.AppDataDir(), "config.json") }
func webviewDataDir() string { return filepath.Join(engine.AppDataDir(), "webview") }

func loadConfig() Config {
	c := defaultConfig()
	if b, err := os.ReadFile(configPath()); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	c.Settings.Normalize()
	for i := range c.UserPresets {
		c.UserPresets[i].Settings.Normalize()
	}
	return c
}

func (c Config) save() error {
	if err := os.MkdirAll(filepath.Dir(configPath()), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	tmp := configPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, configPath())
}

// ---------- File d'attente ----------

type JobItem struct {
	ID       int              `json:"id"`
	Path     string           `json:"path"`
	Name     string           `json:"name"`
	Info     engine.VideoInfo `json:"info"`
	Thumb    string           `json:"thumb"`
	Status   string           `json:"status"` // probing | ready | queued | rendering | paused | done | error | cancelled
	Progress engine.Progress  `json:"progress"`
	Output   string           `json:"output"`
	Error    string           `json:"error"`
	Details  string           `json:"details"`
	Preset   string           `json:"preset"`
	Elapsed  float64          `json:"elapsed"`
	settings engine.Settings
	render   *engine.Render
}

type App struct {
	ctx   context.Context
	paths engine.Paths

	mu         sync.Mutex
	cfg        Config
	jobs       []*JobItem
	nextID     int
	running    bool
	gpus       map[string]bool
	rifeOK     *bool
	installing bool
	previewMu  sync.Mutex
	previewCtl context.CancelFunc
	emitTimer  *time.Timer
}

func NewApp() *App {
	return &App{paths: engine.DefaultPaths(), cfg: loadConfig(), nextID: 1}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	runtime.OnFileDrop(ctx, func(_, _ int, paths []string) { a.AddFiles(paths) })
	_ = runtime.InitializeNotifications(ctx)
	if a.paths.Installed() {
		_ = engine.WriteScripts(a.paths)
		go a.detectGPU()
	}
	// fichiers passés en argument (glisser sur l'exe / « Ouvrir avec »)
	if len(os.Args) > 1 {
		go func() {
			time.Sleep(800 * time.Millisecond)
			a.AddFiles(os.Args[1:])
		}()
	}
}

func (a *App) secondInstance(d options.SecondInstanceData) {
	runtime.WindowUnminimise(a.ctx)
	runtime.Show(a.ctx)
	if len(d.Args) > 0 {
		a.AddFiles(d.Args)
	}
}

// beforeClose : demande confirmation si un rendu est en cours.
func (a *App) beforeClose(ctx context.Context) bool {
	a.mu.Lock()
	busy := a.running
	a.mu.Unlock()
	if !busy {
		return false
	}
	res, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "Rendu en cours",
		Message:       "Un rendu est en cours. Quitter l'arrêtera et le fichier sera supprimé. Quitter quand même ?",
		Buttons:       []string{"Quitter", "Annuler"},
		DefaultButton: "Annuler",
		CancelButton:  "Annuler",
	})
	return err != nil || (res != "Quitter" && res != "Yes" && res != "Oui")
}

func (a *App) shutdown(context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, j := range a.jobs {
		if j.render != nil {
			j.render.Stop()
		}
	}
}

func (a *App) detectGPU() {
	g := engine.DetectGPUEncoders(context.Background(), a.paths)
	a.mu.Lock()
	a.gpus = g
	if a.cfg.GPUType == "" || !g[a.cfg.GPUType] {
		a.cfg.GPUType = ""
		for _, k := range []string{"nvidia", "amd", "intel"} {
			if g[k] {
				a.cfg.GPUType = k
				break
			}
		}
	}
	a.mu.Unlock()
	runtime.EventsEmit(a.ctx, "gpus", g)

	ok := engine.CheckRIFE(context.Background(), a.paths)
	a.mu.Lock()
	a.rifeOK = &ok
	a.mu.Unlock()
	runtime.EventsEmit(a.ctx, "rife", ok)
}

// ---------- API exposée à l'interface ----------

type InitState struct {
	Version       string          `json:"version"`
	Installed     bool            `json:"installed"`
	DownloadSize  int64           `json:"downloadSize"`
	EnginePath    string          `json:"enginePath"`
	Presets       []engine.Preset `json:"presets"`
	Defaults      engine.Settings `json:"defaults"`
	Config        Config          `json:"config"`
	GPUs          map[string]bool `json:"gpus"`
	RifeOK        *bool           `json:"rifeOk"`
	Jobs          []*JobItem      `json:"jobs"`
}

func (a *App) Init() InitState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return InitState{
		Version: AppVersion, Installed: a.paths.Installed(), DownloadSize: engine.TotalDownloadSize(),
		EnginePath: a.paths.Root, Presets: engine.BuiltinPresets(), Defaults: engine.DefaultSettings(),
		Config: a.cfg, GPUs: a.gpus, RifeOK: a.rifeOK, Jobs: a.jobs,
	}
}

func (a *App) InstallEngine() {
	a.mu.Lock()
	if a.installing {
		a.mu.Unlock()
		return
	}
	a.installing = true
	a.mu.Unlock()
	go func() {
		err := engine.Install(a.ctx, a.paths, func(p engine.InstallProgress) { runtime.EventsEmit(a.ctx, "install", p) })
		a.mu.Lock()
		a.installing = false
		a.mu.Unlock()
		msg := ""
		if err != nil {
			msg = err.Error()
		} else {
			go a.detectGPU()
		}
		runtime.EventsEmit(a.ctx, "install-done", msg)
	}()
}

func (a *App) SaveConfig(c Config) error {
	c.Settings.Normalize()
	a.mu.Lock()
	c.GPUType = a.cfg.GPUType
	if c.GPUType == "" || (a.gpus != nil && !a.gpus[c.GPUType]) {
		c.Settings.GPUEncoding = false
	}
	a.cfg = c
	a.mu.Unlock()
	return c.save()
}

var videoExts = []string{".mp4", ".mkv", ".mov", ".avi", ".webm", ".m4v", ".flv", ".wmv", ".ts", ".m2ts", ".mts", ".gif"}

func (a *App) BrowseFiles() {
	paths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Choisir des vidéos",
		Filters: []runtime.FileFilter{
			{DisplayName: "Vidéos", Pattern: "*" + strings.Join(videoExts, ";*")},
			{DisplayName: "Tous les fichiers", Pattern: "*.*"},
		},
	})
	if err == nil && len(paths) > 0 {
		a.AddFiles(paths)
	}
}

func (a *App) BrowseOutputDir() string {
	dir, _ := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Dossier de sortie"})
	return dir
}

// AddFiles accepte fichiers et dossiers (les vidéos d'un dossier déposé sont ajoutées).
func (a *App) AddFiles(paths []string) {
	var files []string
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if st.IsDir() {
			entries, _ := os.ReadDir(p)
			for _, e := range entries {
				if !e.IsDir() && slices.Contains(videoExts, strings.ToLower(filepath.Ext(e.Name()))) {
					files = append(files, filepath.Join(p, e.Name()))
				}
			}
			continue
		}
		files = append(files, p)
	}
	a.mu.Lock()
	var added []*JobItem
	for _, f := range files {
		dup := slices.ContainsFunc(a.jobs, func(j *JobItem) bool {
			return strings.EqualFold(j.Path, f) && (j.Status == "ready" || j.Status == "queued" || j.Status == "probing")
		})
		if dup {
			continue
		}
		j := &JobItem{ID: a.nextID, Path: f, Name: filepath.Base(f), Status: "probing"}
		a.nextID++
		a.jobs = append(a.jobs, j)
		added = append(added, j)
	}
	a.mu.Unlock()
	a.emitJobs()
	if !a.paths.Installed() {
		return
	}
	for _, j := range added {
		go a.probe(j)
	}
}

func (a *App) probe(j *JobItem) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	info, err := engine.Probe(ctx, a.paths, j.Path)
	a.mu.Lock()
	j.Info = info
	if err != nil {
		j.Status, j.Error = "error", err.Error()
	} else {
		j.Status = "ready"
	}
	a.mu.Unlock()
	a.emitJobs()
	if err == nil {
		if b, err := engine.Thumbnail(ctx, a.paths, j.Path, info.Duration); err == nil && len(b) > 0 {
			a.mu.Lock()
			j.Thumb = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(b)
			a.mu.Unlock()
			a.emitJobs()
		}
	}
}

// ProbePending analyse les fichiers ajoutés avant l'installation du moteur.
func (a *App) ProbePending() {
	a.mu.Lock()
	var todo []*JobItem
	for _, j := range a.jobs {
		if j.Status == "probing" {
			todo = append(todo, j)
		}
	}
	a.mu.Unlock()
	for _, j := range todo {
		go a.probe(j)
	}
}

// Start met en file toutes les vidéos prêtes avec les réglages actuels.
func (a *App) Start(presetName string) int {
	a.mu.Lock()
	n := 0
	for _, j := range a.jobs {
		if j.Status == "ready" || j.Status == "cancelled" || j.Status == "error" && j.Info.Duration > 0 {
			j.Status, j.Error, j.Details, j.Progress = "queued", "", "", engine.Progress{}
			j.settings, j.Preset = a.cfg.Settings, presetName
			n++
		}
	}
	start := n > 0 && !a.running
	if start {
		a.running = true
	}
	a.mu.Unlock()
	a.emitJobs()
	if start {
		go a.worker()
	}
	return n
}

func (a *App) worker() {
	stopAwake := make(chan struct{})
	a.mu.Lock()
	if a.cfg.KeepAwake {
		engine.KeepAwake(stopAwake)
	}
	a.mu.Unlock()
	defer close(stopAwake)

	doneCount, failCount := 0, 0
	var lastOut string
	for {
		a.mu.Lock()
		var j *JobItem
		for _, x := range a.jobs {
			if x.Status == "queued" {
				j = x
				break
			}
		}
		if j == nil {
			a.running = false
			a.mu.Unlock()
			break
		}
		s := j.settings
		s.Normalize()
		job := engine.Job{Input: j.Path, Info: j.Info, Settings: s, GPUType: a.cfg.GPUType,
			Output: engine.OutputPath(j.Path, a.cfg.OutputDir, s)}
		if !s.GPUEncoding {
			job.GPUType = ""
		}
		r := engine.NewRender(a.paths, job)
		j.render, j.Status, j.Output = r, "rendering", job.Output
		low := a.cfg.LowPriority
		a.mu.Unlock()
		a.emitJobs()

		t0 := time.Now()
		err := r.Run(low, func(p engine.Progress) {
			a.mu.Lock()
			j.Progress = p
			j.Elapsed = time.Since(t0).Seconds()
			a.mu.Unlock()
			a.emitJobsThrottled()
			runtime.WindowSetTitle(a.ctx, fmt.Sprintf("%.0f %% — NeiBlur", p.Percent))
		})

		a.mu.Lock()
		j.render = nil
		j.Elapsed = time.Since(t0).Seconds()
		var re *engine.RenderError
		switch {
		case err == nil:
			j.Status, j.Progress.Percent = "done", 100
			doneCount++
			lastOut = j.Output
		case errors.Is(err, engine.ErrStopped):
			if j.Status != "removed" {
				j.Status = "cancelled"
			}
		case errors.As(err, &re):
			j.Status, j.Error, j.Details = "error", re.Error(), re.Details()
			failCount++
		default:
			j.Status, j.Error = "error", err.Error()
			failCount++
		}
		a.jobs = slices.DeleteFunc(a.jobs, func(x *JobItem) bool { return x.Status == "removed" })
		a.mu.Unlock()
		a.emitJobs()
	}
	runtime.WindowSetTitle(a.ctx, "NeiBlur")

	a.mu.Lock()
	notify, reveal := a.cfg.Notify, a.cfg.RevealOnDone
	a.mu.Unlock()
	if doneCount+failCount == 0 {
		return
	}
	if notify {
		body := fmt.Sprintf("%d vidéo(s) terminée(s).", doneCount)
		if failCount > 0 {
			body += fmt.Sprintf(" %d en échec.", failCount)
		}
		_ = runtime.SendNotification(a.ctx, runtime.NotificationOptions{ID: fmt.Sprint(time.Now().UnixNano()), Title: "NeiBlur — rendu terminé", Body: body})
	}
	if reveal && lastOut != "" {
		_ = engine.RevealInExplorer(lastOut)
	}
}

func (a *App) find(id int) *JobItem {
	for _, j := range a.jobs {
		if j.ID == id {
			return j
		}
	}
	return nil
}

func (a *App) TogglePause(id int) error {
	a.mu.Lock()
	j := a.find(id)
	if j == nil || j.render == nil {
		a.mu.Unlock()
		return nil
	}
	r, paused := j.render, j.Status == "paused"
	a.mu.Unlock()
	var err error
	if paused {
		err = r.Resume()
	} else {
		err = r.Pause()
	}
	if err == nil {
		a.mu.Lock()
		if paused {
			j.Status = "rendering"
		} else {
			j.Status = "paused"
		}
		a.mu.Unlock()
	}
	a.emitJobs()
	return err
}

func (a *App) Cancel(id int) {
	a.mu.Lock()
	j := a.find(id)
	if j != nil {
		switch {
		case j.render != nil:
			j.render.Stop()
		case j.Status == "queued":
			j.Status = "ready"
		}
	}
	a.mu.Unlock()
	a.emitJobs()
}

func (a *App) Remove(id int) {
	a.mu.Lock()
	j := a.find(id)
	if j != nil {
		if j.render != nil {
			j.Status = "removed"
			j.render.Stop()
		} else {
			a.jobs = slices.DeleteFunc(a.jobs, func(x *JobItem) bool { return x.ID == id })
		}
	}
	a.mu.Unlock()
	a.emitJobs()
}

func (a *App) ClearFinished() {
	a.mu.Lock()
	a.jobs = slices.DeleteFunc(a.jobs, func(x *JobItem) bool {
		return x.Status == "done" || x.Status == "cancelled" || x.Status == "error"
	})
	a.mu.Unlock()
	a.emitJobs()
}

func (a *App) CancelAll() {
	a.mu.Lock()
	for _, j := range a.jobs {
		if j.Status == "queued" {
			j.Status = "ready"
		}
		if j.render != nil {
			j.render.Stop()
		}
	}
	a.mu.Unlock()
	a.emitJobs()
}

type PreviewResult struct {
	Before  string `json:"before"`
	After   string `json:"after"`
	Error   string `json:"error"`
	Details string `json:"details"`
	Ms      int64  `json:"ms"`
}

// Preview calcule une image avant/après. Une nouvelle demande annule la précédente.
func (a *App) Preview(id int, t float64, s engine.Settings) PreviewResult {
	a.mu.Lock()
	j := a.find(id)
	if j == nil || j.Info.Duration == 0 {
		a.mu.Unlock()
		return PreviewResult{Error: "Vidéo introuvable."}
	}
	s.Normalize()
	job := engine.Job{Input: j.Path, Info: j.Info, Settings: s}
	a.mu.Unlock()

	a.previewMu.Lock()
	if a.previewCtl != nil {
		a.previewCtl()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	a.previewCtl = cancel
	a.previewMu.Unlock()
	defer cancel()

	t0 := time.Now()
	before, after, err := engine.PreviewFrames(ctx, a.paths, job, t)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return PreviewResult{Error: "cancelled"}
		}
		res := PreviewResult{Error: "L'aperçu a échoué."}
		var re *engine.RenderError
		if errors.As(err, &re) {
			res.Error, res.Details = re.Error(), re.Details()
		}
		return res
	}
	enc := func(b []byte) string {
		if len(b) == 0 {
			return ""
		}
		return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(b)
	}
	return PreviewResult{Before: enc(before), After: enc(after), Ms: time.Since(t0).Milliseconds()}
}

func (a *App) SavePreset(name string, s engine.Settings) Config {
	s.Normalize()
	a.mu.Lock()
	id := fmt.Sprintf("user-%d", time.Now().UnixNano())
	a.cfg.UserPresets = append(a.cfg.UserPresets, engine.Preset{ID: id, Name: name, Icon: "star", Speed: 2,
		Description: "Préréglage personnel", Settings: s, Custom: true})
	a.cfg.PresetID, a.cfg.Settings = id, s
	c := a.cfg
	a.mu.Unlock()
	_ = c.save()
	return c
}

func (a *App) DeletePreset(id string) Config {
	a.mu.Lock()
	a.cfg.UserPresets = slices.DeleteFunc(a.cfg.UserPresets, func(p engine.Preset) bool { return p.ID == id })
	if a.cfg.PresetID == id {
		a.cfg.PresetID = "gaming"
		a.cfg.Settings = engine.DefaultSettings()
	}
	c := a.cfg
	a.mu.Unlock()
	_ = c.save()
	return c
}

func (a *App) Reveal(path string)  { _ = engine.RevealInExplorer(path) }
func (a *App) OpenFile(path string) { _ = engine.OpenPath(path) }
func (a *App) OpenURL(url string) {
	if strings.HasPrefix(url, "https://") {
		runtime.BrowserOpenURL(a.ctx, url)
	}
}
func (a *App) OpenEngineFolder() { _ = engine.OpenPath(a.paths.Root) }

// ---------- Événements ----------

func (a *App) snapshot() []*JobItem {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]*JobItem, len(a.jobs))
	for i, j := range a.jobs {
		c := *j
		out[i] = &c
	}
	return out
}

func (a *App) emitJobs() {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "jobs", a.snapshot())
	}
}

// emitJobsThrottled limite les mises à jour de progression à ~4 par seconde.
func (a *App) emitJobsThrottled() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.emitTimer != nil {
		return
	}
	a.emitTimer = time.AfterFunc(250*time.Millisecond, func() {
		a.mu.Lock()
		a.emitTimer = nil
		a.mu.Unlock()
		a.emitJobs()
	})
}
