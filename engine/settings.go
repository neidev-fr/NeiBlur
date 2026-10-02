package engine

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Settings : tous les réglages de rendu. Les valeurs par défaut sont celles de Blur.
type Settings struct {
	BlurAmount float64 `json:"blurAmount"`
	OutputFPS  int     `json:"outputFps"`
	Weighting  string  `json:"weighting"`
	Gamma      float64 `json:"gamma"`

	Interpolate        bool   `json:"interpolate"`
	InterpolatedFPS    string `json:"interpolatedFps"`
	InterpMethod       string `json:"interpMethod"` // svp | rife
	PreInterpolate     bool   `json:"preInterpolate"`
	PreInterpolatedFPS string `json:"preInterpolatedFps"`

	Deduplicate    bool   `json:"deduplicate"`
	DedupMethod    string `json:"dedupMethod"` // svp | rife
	DedupRange     int    `json:"dedupRange"`
	DedupThreshold string `json:"dedupThreshold"`

	InputTimescale  float64 `json:"inputTimescale"`
	OutputTimescale float64 `json:"outputTimescale"`
	AudioPitch      bool    `json:"audioPitch"`

	Brightness float64 `json:"brightness"`
	Saturation float64 `json:"saturation"`
	Contrast   float64 `json:"contrast"`

	// Colorimétrie (0 = neutre, voir scripts/blur/grading.py)
	Exposure    float64 `json:"exposure"` // en IL
	Temperature float64 `json:"temperature"`
	Tint        float64 `json:"tint"`
	Shadows     float64 `json:"shadows"`
	Highlights  float64 `json:"highlights"`
	Vibrance    float64 `json:"vibrance"`
	Fade        float64 `json:"fade"`
	Vignette    float64 `json:"vignette"`
	Sharpen     float64 `json:"sharpen"`
	Look        string  `json:"look"`
	LookAmount  float64 `json:"lookAmount"`

	Codec       string `json:"codec"` // h264 | h265 | av1
	Quality     int    `json:"quality"`
	GPUDecoding bool   `json:"gpuDecoding"`
	GPUInterp   bool   `json:"gpuInterp"`
	GPUEncoding bool   `json:"gpuEncoding"`

	Resolution   int    `json:"resolution"` // côté court en pixels, 0 = original
	Container    string `json:"container"`  // mp4 | mkv | mov
	NoAudio      bool   `json:"noAudio"`
	AudioBitrate int    `json:"audioBitrate"` // kb/s

	SVPPreset    string `json:"svpPreset"`
	SVPAlgorithm string `json:"svpAlgorithm"`
	BlockSize    string `json:"blockSize"`
	MaskArea     int    `json:"maskArea"`

	DetailedFilenames bool `json:"detailedFilenames"`
	CopyDates         bool `json:"copyDates"`
}

func DefaultSettings() Settings {
	return Settings{
		BlurAmount: 1, OutputFPS: 60, Weighting: "equal", Gamma: 1,
		Interpolate: true, InterpolatedFPS: "1200", InterpMethod: "svp", PreInterpolatedFPS: "360",
		Deduplicate: true, DedupMethod: "svp", DedupRange: 2, DedupThreshold: "0.001",
		InputTimescale: 1, OutputTimescale: 1,
		Brightness: 1, Saturation: 1, Contrast: 1, LookAmount: 1,
		Codec: "h264", Quality: 16, GPUDecoding: true, GPUInterp: true, Container: "mp4", AudioBitrate: 320,
		SVPPreset: "weak", SVPAlgorithm: "13", BlockSize: "8", MaskArea: 0,
	}
}

type Preset struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Icon        string   `json:"icon"`
	Speed       int      `json:"speed"` // 1 (lent) à 3 (rapide), indicatif
	Settings    Settings `json:"settings"`
	Custom      bool     `json:"custom"`
}

func BuiltinPresets() []Preset {
	with := func(f func(*Settings)) Settings { s := DefaultSettings(); f(&s); return s }
	return []Preset{
		{ID: "gaming", Name: "Gaming fluide", Icon: "gamepad", Speed: 3,
			Description: "Réglages d'origine de Blur. Flou naturel, idéal pour les clips de jeu en 60 i/s.",
			Settings:    DefaultSettings()},
		{ID: "natural", Name: "Lumière réaliste", Icon: "sun", Speed: 2,
			Description: "Flou mélangé en lumière linéaire avec un obturateur doux : les zones claires laissent de vraies traînées, sans bords durs.",
			Settings: with(func(s *Settings) { s.Weighting = "soft_shutter"; s.Gamma = 2.2 })},
		{ID: "cinema", Name: "Cinéma", Icon: "film", Speed: 2,
			Description: "Obturation à 180°, 30 i/s, obturateur doux et lumière réaliste. Rendu « film ».",
			Settings: with(func(s *Settings) { s.BlurAmount = 0.5; s.OutputFPS = 30; s.Weighting = "soft_shutter"; s.Gamma = 2.2 })},
		{ID: "subtle", Name: "Subtil", Icon: "feather", Speed: 3,
			Description: "Un léger flou qui adoucit les mouvements sans traînées visibles.",
			Settings: with(func(s *Settings) { s.BlurAmount = 0.3; s.Weighting = "hann" })},
		{ID: "intense", Name: "Intense", Icon: "zap", Speed: 3,
			Description: "Traînées marquées, effet très fluide pour montages et edits.",
			Settings: with(func(s *Settings) { s.BlurAmount = 1.8; s.Weighting = "hann" })},
		{ID: "extreme", Name: "Extrême", Icon: "wind", Speed: 2,
			Description: "Traînées très longues (350 %) à fondu progressif, pour des effets stylisés.",
			Settings: with(func(s *Settings) { s.BlurAmount = 3.5; s.Weighting = "hann"; s.Gamma = 1.8 })},
		{ID: "quality", Name: "Qualité max", Icon: "sparkles", Speed: 1,
			Description: "Interpolation IA RIFE : moins d'artefacts, mais nettement plus lent.",
			Settings: with(func(s *Settings) { s.InterpMethod = "rife"; s.InterpolatedFPS = "600"; s.DedupMethod = "rife"; s.Quality = 14 })},
		{ID: "draft", Name: "Brouillon rapide", Icon: "rocket", Speed: 3,
			Description: "Pour tester vite un réglage : moins d'images interpolées, fichier plus léger.",
			Settings: with(func(s *Settings) { s.InterpolatedFPS = "360"; s.Quality = 23; s.Deduplicate = false })},
	}
}

// blurJSON produit le dictionnaire "settings" attendu par blur.py (mêmes clés que Blur).
func (s Settings) blurJSON(p Paths, gpuType string) (string, error) {
	timescale := s.InputTimescale != 1 || s.OutputTimescale != 1
	filters := s.Brightness != 1 || s.Saturation != 1 || s.Contrast != 1
	m := map[string]any{
		"blur": true, "blur_amount": s.BlurAmount, "blur_output_fps": s.OutputFPS,
		"blur_weighting": s.Weighting, "blur_gamma": s.Gamma,
		"interpolate": s.Interpolate, "interpolated_fps": s.InterpolatedFPS, "interpolation_method": s.InterpMethod,
		"pre_interpolate": s.PreInterpolate, "pre_interpolated_fps": s.PreInterpolatedFPS,
		"deduplicate": s.Deduplicate, "deduplicate_method": s.DedupMethod,
		"timescale": timescale, "input_timescale": s.InputTimescale, "output_timescale": s.OutputTimescale,
		"output_timescale_audio_pitch": s.AudioPitch,
		"filters": filters, "brightness": s.Brightness, "saturation": s.Saturation, "contrast": s.Contrast,
		"grading": s.grading(), "exposure": s.Exposure, "temperature": s.Temperature, "tint": s.Tint,
		"shadows": s.Shadows, "highlights": s.Highlights, "vibrance": s.Vibrance, "fade": s.Fade,
		"vignette": s.Vignette, "sharpen": s.Sharpen, "look": s.Look, "look_amount": s.LookAmount,
		"encode preset": s.Codec, "quality": s.Quality, "preview": false, "detailed_filenames": s.DetailedFilenames,
		"gpu_decoding": s.GPUDecoding, "gpu_interpolation": s.GPUInterp, "gpu_encoding": s.GPUEncoding,
		"deduplicate_range": s.DedupRange, "deduplicate_threshold": s.DedupThreshold, "debug": false,
		"blur_weighting_gaussian_std_dev": 1.0, "blur_weighting_gaussian_mean": 2.0, "blur_weighting_gaussian_bound": "[0,2]",
		"svp_interpolation_preset": s.SVPPreset, "svp_interpolation_algorithm": s.SVPAlgorithm,
		"interpolation_blocksize": s.BlockSize, "interpolation_mask_area": s.MaskArea,
		"rife_model": strings.ReplaceAll(p.RifeModel, `\`, "/"),
		"manual_svp": false, "super_string": "", "vectors_string": "", "smooth_string": "",
		"gpu_type": gpuType, "rife_gpu_index": 0,
	}
	b, err := json.Marshal(m)
	return string(b), err
}

// Normalize corrige les valeurs hors limites pour éviter les erreurs incompréhensibles côté moteur.
func (s *Settings) Normalize() {
	d := DefaultSettings()
	if s.OutputFPS <= 0 {
		s.OutputFPS = d.OutputFPS
	}
	if s.BlurAmount < 0 {
		s.BlurAmount = 0
	}
	if s.Gamma <= 0 {
		s.Gamma = 1
	}
	if s.InputTimescale <= 0 {
		s.InputTimescale = 1
	}
	if s.OutputTimescale <= 0 {
		s.OutputTimescale = 1
	}
	if !validFPS(s.InterpolatedFPS) {
		s.InterpolatedFPS = d.InterpolatedFPS
	}
	if !validFPS(s.PreInterpolatedFPS) {
		s.PreInterpolatedFPS = d.PreInterpolatedFPS
	}
	if s.Quality < 0 || s.Quality > 51 {
		s.Quality = d.Quality
	}
	switch s.Codec {
	case "h264", "h265", "av1":
	default:
		s.Codec = "h264"
	}
	if s.InterpMethod != "rife" {
		s.InterpMethod = "svp"
	}
	if s.DedupMethod != "rife" {
		s.DedupMethod = "svp"
	}
	if s.Weighting == "" {
		s.Weighting = "equal"
	}
	if s.BlurAmount > 10 {
		s.BlurAmount = 10
	}
	s.Exposure = clamp(s.Exposure, -3, 3)
	for _, v := range []*float64{&s.Temperature, &s.Tint, &s.Shadows, &s.Highlights, &s.Vibrance, &s.Vignette} {
		*v = clamp(*v, -1, 1)
	}
	s.Fade = clamp(s.Fade, -0.5, 0.5)
	s.Sharpen = clamp(s.Sharpen, 0, 2)
	switch s.Look {
	case "", "warm", "cool", "teal_orange", "film", "vintage", "vivid", "night", "bw":
	default:
		s.Look = ""
	}
	if s.LookAmount <= 0 || s.LookAmount > 2 {
		s.LookAmount = 1
	}
	switch s.Container {
	case "mp4", "mkv", "mov":
	default:
		s.Container = "mp4"
	}
	if s.Resolution < 0 || s.Resolution > 4320 {
		s.Resolution = 0
	}
	if s.AudioBitrate < 64 || s.AudioBitrate > 512 {
		s.AudioBitrate = d.AudioBitrate
	}
}

func clamp(v, lo, hi float64) float64 { return max(lo, min(hi, v)) }

// grading indique si une étape de colorimétrie NeiBlur change l'image.
func (s Settings) grading() bool {
	if s.Look != "" {
		return true
	}
	for _, v := range []float64{s.Exposure, s.Temperature, s.Tint, s.Shadows, s.Highlights, s.Vibrance, s.Fade, s.Vignette, s.Sharpen} {
		if v != 0 {
			return true
		}
	}
	return false
}

func validFPS(v string) bool {
	v = strings.TrimSpace(v)
	if m, ok := strings.CutSuffix(v, "x"); ok {
		f, err := strconv.ParseFloat(strings.TrimSpace(m), 64)
		return err == nil && f > 0
	}
	n, err := strconv.Atoi(v)
	return err == nil && n > 0
}

// encoderArgs : mêmes préréglages d'encodage que Blur (AV1 CPU passe par SVT-AV1, bien plus rapide que libaom).
func encoderArgs(codec, gpuType string, gpu bool, q int) []string {
	qs := strconv.Itoa(q)
	if gpu {
		switch gpuType {
		case "nvidia":
			enc := map[string]string{"h264": "h264_nvenc", "h265": "hevc_nvenc", "av1": "av1_nvenc"}[codec]
			return []string{"-c:v", enc, "-preset", "p5", "-rc", "vbr", "-cq", qs}
		case "amd":
			enc := map[string]string{"h264": "h264_amf", "h265": "hevc_amf", "av1": "av1_amf"}[codec]
			return []string{"-c:v", enc, "-rc", "cqp", "-qp_i", qs, "-qp_p", qs, "-qp_b", qs}
		case "intel":
			enc := map[string]string{"h264": "h264_qsv", "h265": "hevc_qsv", "av1": "av1_qsv"}[codec]
			return []string{"-c:v", enc, "-global_quality", qs, "-preset", "veryfast"}
		}
	}
	switch codec {
	case "h265":
		return []string{"-c:v", "libx265", "-preset", "veryfast", "-crf", qs}
	case "av1":
		return []string{"-c:v", "libsvtav1", "-preset", "8", "-crf", qs}
	}
	return []string{"-c:v", "libx264", "-preset", "veryfast", "-crf", qs}
}
