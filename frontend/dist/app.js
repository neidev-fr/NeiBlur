"use strict";
// NeiBlur — interface. Créé par neidev.

const $ = (s, r = document) => r.querySelector(s);
const api = () => window.go.main.App;
const on = (name, cb) => window.runtime.EventsOn(name, cb);

const S = { presets: [], defaults: {}, config: null, jobs: [], gpus: null, installed: false, version: "" };

// Réglages « machine / sortie » : toujours conservés quand on change de style.
const MACHINE = ["gpuEncoding", "gpuDecoding", "gpuInterp", "codec", "detailedFilenames", "copyDates", "resolution", "container", "noAudio", "audioBitrate"];
// Colorimétrie : conservée quand on choisit un style intégré (un style perso l'applique).
const COLOR = ["brightness", "contrast", "saturation", "exposure", "temperature", "tint", "shadows", "highlights", "vibrance", "fade", "vignette", "sharpen", "look", "lookAmount"];

// ---------- Utilitaires ----------
const clone = (o) => JSON.parse(JSON.stringify(o));
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
const icon = (id) => `<svg><use href="#i-${id}"/></svg>`;
const same = (a, b) => (typeof a === "number" && typeof b === "number" ? Math.abs(a - b) < 1e-6 : a === b);
function debounce(fn, ms) { let t; return (...a) => { clearTimeout(t); t = setTimeout(() => fn(...a), ms); }; }
function fmtClock(s) { s = Math.max(0, Math.round(s)); const m = Math.floor(s / 60); return `${m}:${String(s % 60).padStart(2, "0")}`; }
function fmtDur(s) {
  if (!(s >= 0) || !isFinite(s)) return "…";
  s = Math.round(s);
  if (s < 60) return `${s} s`;
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), r = s % 60;
  return h ? `${h} h ${String(m).padStart(2, "0")}` : `${m} min ${String(r).padStart(2, "0")}`;
}
function fmtSize(b) { if (!b) return ""; const u = ["o", "Ko", "Mo", "Go"]; let i = 0; while (b >= 1024 && i < 3) { b /= 1024; i++; } return `${b.toFixed(i > 1 ? 1 : 0)} ${u[i]}`; }
function fmtFps(f) { return f ? (Math.abs(f - Math.round(f)) < 0.01 ? Math.round(f) : f.toFixed(2)) : "?"; }

// Remplit la piste d'un curseur entre son origine (0 ou la valeur neutre) et la valeur actuelle.
function paintRange(inp, origin) {
  const min = +inp.min, max = +inp.max, span = max - min || 1;
  const o = origin === undefined ? min : Math.min(max, Math.max(min, origin));
  const a = ((Math.min(o, +inp.value) - min) / span) * 100, b = ((Math.max(o, +inp.value) - min) / span) * 100;
  inp.style.setProperty("--a", `${a}%`);
  inp.style.setProperty("--b", `${b}%`);
}

function toast(msg, kind = "ok", ms = 3500) {
  const el = document.createElement("div");
  el.className = `toast ${kind}`;
  el.innerHTML = `${icon(kind === "err" ? "alert" : "check")}<span>${esc(msg)}</span>`;
  $("#toasts").append(el);
  setTimeout(() => el.remove(), ms);
}

// ---------- Dialogues ----------
function openDlg(id) { const d = $(id); d.hidden = false; const f = d.querySelector("input,select,button:not([data-close])"); f?.focus(); }
function closeDlg(d) { if (typeof d === "string") d = $(d); d.hidden = true; if (d.id === "previewDlg") previewState.open = false; }
document.addEventListener("click", (e) => {
  const c = e.target.closest("[data-close]"); if (c) closeDlg(c.closest(".overlay"));
  const a = e.target.closest("a[data-url]"); if (a) { e.preventDefault(); api().OpenURL(a.dataset.url); }
});
document.querySelectorAll(".overlay").forEach((o) => o.addEventListener("mousedown", (e) => { if (e.target === o && o.id !== "setup") closeDlg(o); }));

// ---------- Onglets ----------
function showTab(name) {
  document.querySelectorAll(".tab").forEach((t) => t.setAttribute("aria-selected", String(t.dataset.tab === name)));
  document.querySelectorAll(".panel").forEach((p) => (p.hidden = p.dataset.panel !== name));
  $("#sideScroll").scrollTop = 0;
  try { localStorage.setItem("tab", name); } catch {}
}
document.querySelectorAll(".tab").forEach((t) => t.addEventListener("click", () => showTab(t.dataset.tab)));

// ---------- Styles (préréglages) ----------
const allPresets = () => [...S.presets, ...(S.config.userPresets || [])];
const currentPreset = () => allPresets().find((p) => p.id === S.config.presetId) || S.presets[0];
const ignoredKeys = () => (currentPreset().custom ? MACHINE : [...MACHINE, ...COLOR]);

function isModified() {
  const p = currentPreset().settings, s = S.config.settings, skip = ignoredKeys();
  return Object.keys(p).some((k) => !skip.includes(k) && !same(p[k], s[k]));
}
const colorActive = (s) => COLOR.some((k) => !same(s[k], S.defaults[k]));
const SPEED = { 1: ["lent", "slow"], 2: ["moyen", ""], 3: ["rapide", ""] };

function renderPresets() {
  const box = $("#presets");
  box.innerHTML = "";
  for (const p of allPresets()) {
    const b = document.createElement("div");
    b.className = "preset";
    b.tabIndex = 0;
    b.setAttribute("role", "radio");
    b.setAttribute("aria-checked", String(p.id === S.config.presetId));
    b.title = p.description;
    const [speed, cls] = SPEED[p.speed] || SPEED[2];
    b.innerHTML = `${icon(p.icon || "star")}<span class="preset-name">${esc(p.name)}</span><span class="preset-speed ${cls}" title="Vitesse de rendu">${speed}</span>
      ${p.custom ? `<button class="icon-btn sm del" title="Supprimer" aria-label="Supprimer ${esc(p.name)}">${icon("x")}</button>` : ""}`;
    const pick = () => selectPreset(p);
    b.addEventListener("click", (e) => {
      if (e.target.closest(".del")) { e.stopPropagation(); api().DeletePreset(p.id).then((c) => { S.config = c; syncAll(); }); return; }
      pick();
    });
    b.addEventListener("keydown", (e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); pick(); } });
    box.append(b);
  }
  let tip = $("#presetTip");
  if (!tip) { tip = document.createElement("p"); tip.id = "presetTip"; tip.className = "preset-tip"; box.after(tip); }
  const bad = rifeBroken(S.config.settings);
  tip.classList.toggle("warn", bad);
  tip.textContent = bad ? RIFE_WARN : currentPreset().description;
  document.querySelectorAll(".preset").forEach((el, i) => el.classList.toggle("warn", rifeBroken(allPresets()[i]?.settings || {})));
}

const RIFE_WARN = "RIFE ne fonctionne pas correctement avec ton pilote graphique (images vides). Mets à jour le pilote de ta carte graphique, ou utilise un style basé sur SVP (Gaming, Cinéma…).";
const usesRife = (s) => (s.interpolate && (s.interpMethod === "rife" || (s.interpMethod === "svp" && s.preInterpolate))) || (s.deduplicate && s.dedupMethod === "rife");
const rifeBroken = (s) => S.rifeOk === false && usesRife(s);

function selectPreset(p) {
  const keep = {};
  (p.custom ? MACHINE : [...MACHINE, ...COLOR]).forEach((k) => (keep[k] = S.config.settings[k]));
  S.config.presetId = p.id;
  S.config.settings = { ...clone(p.settings), ...keep };
  syncAll();
  changed();
}

// ---------- Réglages principaux ----------
const intensity = $("#intensity");
intensity.addEventListener("input", () => { S.config.settings.blurAmount = +intensity.value; syncMain(); changed(); });
intensity.addEventListener("dblclick", () => { S.config.settings.blurAmount = currentPreset().settings.blurAmount; syncMain(); changed(); });

$("#fpsSeg").addEventListener("click", (e) => {
  const b = e.target.closest("button"); if (!b) return;
  if (b.dataset.v === "custom") { $("#fpsCustom").hidden = false; $("#fpsCustom").focus(); return; }
  S.config.settings.outputFps = +b.dataset.v; syncMain(); changed();
});
$("#fpsCustom").addEventListener("change", (e) => {
  const v = Math.round(+e.target.value);
  if (v > 0 && v <= 2000) { S.config.settings.outputFps = v; changed(); } else toast("Valeur entre 1 et 2000", "err");
  syncMain();
});

const STD_FPS = [24, 30, 60, 120, 240];
function syncMain() {
  const s = S.config.settings;
  intensity.value = s.blurAmount;
  paintRange(intensity);
  $("#intensityOut").textContent = `${Math.round(s.blurAmount * 100)} % · ${Math.round(s.blurAmount * 360)}°`;
  $("#intensityOut").title = "Intensité · angle d'obturation équivalent";
  const std = STD_FPS.includes(s.outputFps);
  $("#fpsSeg").querySelectorAll("button").forEach((b) => b.setAttribute("aria-checked", String(std ? +b.dataset.v === s.outputFps : b.dataset.v === "custom")));
  $("#fpsCustomBtn").textContent = std ? "Autre" : `${s.outputFps}`;
  if (std) $("#fpsCustom").hidden = true; else $("#fpsCustom").value = s.outputFps;
  $("#modified").hidden = !isModified();
  $("#colorDot").hidden = !colorActive(s);
  document.querySelectorAll(".preset").forEach((el, i) => el.setAttribute("aria-checked", String(allPresets()[i]?.id === S.config.presetId)));
}

// ---------- Réglages détaillés (générés) ----------
const pct = (v) => `${Math.round(v * 100)} %`;
const signed = (v) => { const n = Math.round(v * 100); return n > 0 ? `+${n}` : `${n}`; };
const QUALITY = (v) => `${v} ${v <= 12 ? "· max" : v <= 18 ? "· haute" : v <= 25 ? "· normale" : "· légère"}`;

// panel : blur (sous le réglage d'intensité), blur-adv (repliable), color, output
const ADV = [
  { panel: "blur", rows: [
    { key: "weighting", type: "select", label: "Forme du flou", help: "Comment les images sont mélangées. « Obturateur doux » et « Cloche » évitent les bords durs des traînées.",
      options: [["soft_shutter", "Obturateur doux"], ["hann", "Cloche (fondu)"], ["equal", "Uniforme (Blur)"], ["gaussian_sym", "Gaussienne douce"], ["pyramid", "Pyramide"], ["descending", "Traînée arrière"], ["ascending", "Traînée avant"], ["vegas", "Vegas"], ["gaussian", "Gaussienne"], ["gaussian_reverse", "Gaussienne inversée"]] },
    { key: "gamma", type: "range", label: "Lumière réaliste (gamma)", min: 0.5, max: 4, step: 0.1, origin: 1, fmt: (v) => v.toFixed(1), help: "2.2 mélange les images en lumière linéaire : les zones claires laissent de vraies traînées lumineuses, comme une vraie caméra. 1 = comportement de Blur." },
  ] },
  { panel: "blur-adv", title: "Interpolation", rows: [
    { key: "interpolate", type: "toggle", label: "Interpoler les images", help: "Crée des images intermédiaires pour un flou lisse. Recommandé." },
    { key: "interpMethod", type: "seg", label: "Méthode", dep: (s) => s.interpolate, options: [["svp", "SVP"], ["rife", "RIFE (IA)"]], help: "SVP : très rapide sur GPU. RIFE : moins d'artefacts, beaucoup plus lent." },
    { key: "interpolatedFps", type: "text", label: "Images/s interpolées", dep: (s) => s.interpolate, placeholder: "1200 ou 5x", help: "Un nombre (1200, 2400…) ou un multiple de la source (5x). Plus c'est haut, plus le flou est lisse aux fortes intensités." },
    { key: "preInterpolate", type: "toggle", label: "Pré-interpolation RIFE", dep: (s) => s.interpolate && s.interpMethod === "svp", help: "Passe d'abord par RIFE avant SVP : meilleure qualité, plus lent." },
    { key: "preInterpolatedFps", type: "text", label: "Images/s pré-interpolées", dep: (s) => s.interpolate && s.interpMethod === "svp" && s.preInterpolate, placeholder: "360" },
  ] },
  { panel: "blur-adv", title: "Déduplication", rows: [
    { key: "deduplicate", type: "toggle", label: "Remplacer les images dupliquées", help: "Corrige les saccades des enregistrements qui ont perdu des images." },
    { key: "dedupMethod", type: "seg", label: "Méthode", dep: (s) => s.deduplicate, options: [["svp", "SVP"], ["rife", "RIFE"]] },
    { key: "dedupRange", type: "number", label: "Portée (images)", dep: (s) => s.deduplicate, min: -1, max: 100, help: "Augmente-la si ta vidéo est enregistrée à un fps plus bas que prévu. -1 = illimité." },
    { key: "dedupThreshold", type: "text", label: "Seuil", dep: (s) => s.deduplicate, placeholder: "0.001" },
  ] },
  { panel: "blur-adv", title: "Vitesse", rows: [
    { key: "inputTimescale", type: "number", label: "Vitesse de la source", min: 0.01, max: 100, step: 0.01, help: "Ex. 0.5 si ta vidéo est déjà ralentie ×2." },
    { key: "outputTimescale", type: "number", label: "Vitesse de sortie", min: 0.01, max: 100, step: 0.01, help: "0.5 = ralenti ×2, 2 = accéléré ×2." },
    { key: "audioPitch", type: "toggle", label: "Changer la hauteur du son", dep: (s) => s.outputTimescale !== 1 },
  ] },
  { panel: "blur-adv", title: "SVP (expert)", rows: [
    { key: "svpPreset", type: "select", label: "Préréglage SVP", options: ["weak", "film", "smooth", "animation", "default"].map((v) => [v, v]) },
    { key: "svpAlgorithm", type: "select", label: "Algorithme", options: [["13", "13 (recommandé)"], ["23", "23"], ["1", "1"], ["2", "2"], ["11", "11"], ["21", "21"]] },
    { key: "blockSize", type: "select", label: "Taille de bloc", options: ["4", "8", "16", "32"].map((v) => [v, v]), help: "Petit = plus précis mais plus lent." },
    { key: "maskArea", type: "number", label: "Masque statique", min: 0, max: 2000, help: "Plus haut = les éléments fixes (HUD) sont moins floutés." },
  ] },

  { panel: "color", title: "Look", rows: [
    { key: "look", type: "select", label: "Look", help: "Une ambiance prête à l'emploi, combinée à tes réglages ci-dessous.",
      options: [["", "Aucun"], ["film", "Film"], ["teal_orange", "Teal & orange"], ["warm", "Chaud"], ["cool", "Froid"], ["vivid", "Vif"], ["vintage", "Vintage"], ["night", "Nuit"], ["bw", "Noir et blanc"]] },
    { key: "lookAmount", type: "range", label: "Intensité du look", min: 0.1, max: 2, step: 0.05, origin: 0.1, fmt: pct, dep: (s) => !!s.look },
  ] },
  { panel: "color", title: "Lumière", rows: [
    { key: "exposure", type: "range", label: "Exposition", min: -2, max: 2, step: 0.05, fmt: (v) => `${v > 0 ? "+" : ""}${v.toFixed(2)} IL` },
    { key: "contrast", type: "range", label: "Contraste", min: 0, max: 2, step: 0.01, fmt: pct },
    { key: "brightness", type: "range", label: "Luminosité", min: 0.5, max: 1.5, step: 0.01, fmt: pct },
    { key: "highlights", type: "range", label: "Hautes lumières", min: -1, max: 1, step: 0.01, fmt: signed },
    { key: "shadows", type: "range", label: "Ombres", min: -1, max: 1, step: 0.01, fmt: signed },
    { key: "fade", type: "range", label: "Noirs délavés", min: -0.3, max: 0.3, step: 0.01, fmt: signed, help: "Positif : noirs grisés façon film. Négatif : noirs plus profonds." },
  ] },
  { panel: "color", title: "Couleur", rows: [
    { key: "temperature", type: "range", label: "Température", min: -1, max: 1, step: 0.01, fmt: signed, help: "Négatif = plus froid (bleu), positif = plus chaud (orange)." },
    { key: "tint", type: "range", label: "Teinte", min: -1, max: 1, step: 0.01, fmt: signed, help: "Négatif = vert, positif = magenta." },
    { key: "saturation", type: "range", label: "Saturation", min: 0, max: 3, step: 0.01, fmt: pct },
    { key: "vibrance", type: "range", label: "Vibrance", min: -1, max: 1, step: 0.01, fmt: signed, help: "Renforce surtout les couleurs ternes, sans brûler celles déjà saturées." },
  ] },
  { panel: "color", title: "Effets", rows: [
    { key: "vignette", type: "range", label: "Vignettage", min: -1, max: 1, step: 0.01, fmt: signed, help: "Assombrit les coins (négatif : les éclaircit)." },
    { key: "sharpen", type: "range", label: "Netteté", min: 0, max: 2, step: 0.01, fmt: pct, help: "Redonne du piqué aux zones fixes après le flou." },
  ] },

  { panel: "output", title: "Encodage", rows: [
    { key: "codec", type: "seg", label: "Codec", options: [["h264", "H.264"], ["h265", "H.265"], ["av1", "AV1"]], help: "H.264 : lisible partout. H.265 / AV1 : fichiers plus légers." },
    { key: "quality", type: "range", label: "Qualité", min: 0, max: 51, step: 1, fmt: QUALITY, reverse: true, help: "Plus le nombre est petit, meilleure est la qualité (et plus le fichier est gros)." },
    { key: "gpuEncoding", type: "toggle", label: "Encodage GPU", dep: () => !!gpuName(), help: "Encode avec la carte graphique : plus rapide." },
    { key: "gpuDecoding", type: "toggle", label: "Décodage GPU" },
    { key: "gpuInterp", type: "toggle", label: "Interpolation GPU (SVP)" },
  ] },
  { panel: "output", title: "Image", rows: [
    { key: "resolution", type: "select", num: true, label: "Résolution", help: "Côté le plus court de la vidéo (les vidéos verticales sont gérées). Monter en 1440p ou 4K améliore la qualité sur YouTube.",
      options: [[0, "Originale"], [2160, "2160p (4K)"], [1440, "1440p"], [1080, "1080p"], [720, "720p"], [480, "480p"]] },
    { key: "container", type: "seg", label: "Format", options: [["mp4", "MP4"], ["mkv", "MKV"], ["mov", "MOV"]] },
  ] },
  { panel: "output", title: "Son", rows: [
    { key: "noAudio", type: "toggle", label: "Supprimer le son" },
    { key: "audioBitrate", type: "select", num: true, label: "Débit audio", dep: (s) => !s.noAudio, options: [[128, "128 kb/s"], [192, "192 kb/s"], [256, "256 kb/s"], [320, "320 kb/s"]] },
  ] },
  { panel: "output", title: "Fichiers", rows: [
    { key: "detailedFilenames", type: "toggle", label: "Réglages dans le nom du fichier" },
    { key: "copyDates", type: "toggle", label: "Conserver la date de la source" },
  ] },
];
const PANEL_BODY = { blur: "#blurBody", "blur-adv": "#advBody", color: "#colorBody", output: "#outputBody" };
const advUpdaters = [];

function buildAdvanced() {
  Object.values(PANEL_BODY).forEach((sel) => ($(sel).innerHTML = ""));
  for (const g of ADV) {
    const grp = document.createElement("div");
    if (g.title) { grp.className = "group"; grp.innerHTML = `<div class="group-title">${g.title}</div>`; }
    for (const r of g.rows) grp.append(buildRow(r));
    $(PANEL_BODY[g.panel]).append(grp);
  }
}

// Valeur « neutre » d'un réglage (double-clic) : défaut pour couleur / sortie, sinon valeur du style.
const resetValue = (key) => (COLOR.includes(key) || MACHINE.includes(key) ? S.defaults[key] : currentPreset().settings[key]);

function buildRow(r) {
  const row = document.createElement("div");
  const id = `adv-${r.key}`;
  const set = (v) => { S.config.settings[r.key] = v; changed(); syncAll(); };
  let update;
  if (r.type === "range") {
    row.className = "adv-row col";
    row.innerHTML = `<div class="top"><label for="${id}">${r.label}</label><output></output></div><input type="range" id="${id}" min="${r.min}" max="${r.max}" step="${r.step}">`;
    const inp = $("input", row), out = $("output", row);
    const origin = () => (r.reverse ? +r.max : r.origin ?? (r.min < 0 ? 0 : COLOR.includes(r.key) ? S.defaults[r.key] : r.min));
    const show = (v) => { out.textContent = r.fmt(v); paintRange(inp, origin()); row.classList.toggle("changed", COLOR.includes(r.key) && !same(v, S.defaults[r.key])); };
    inp.addEventListener("input", () => { S.config.settings[r.key] = +inp.value; show(+inp.value); changed(); syncMain(); });
    inp.addEventListener("dblclick", () => set(resetValue(r.key)));
    update = (s) => { inp.value = s[r.key]; show(+s[r.key]); };
  } else if (r.type === "toggle") {
    row.className = "adv-row";
    row.innerHTML = `<label for="${id}">${r.label}</label><input type="checkbox" id="${id}">`;
    const inp = $("input", row);
    inp.addEventListener("change", () => set(inp.checked));
    update = (s) => { inp.checked = !!s[r.key]; };
  } else if (r.type === "seg") {
    row.className = "adv-row";
    row.innerHTML = `<span>${r.label}</span><div class="segmented" role="radiogroup" aria-label="${r.label}">${r.options.map(([v, l]) => `<button data-v="${v}">${l}</button>`).join("")}</div>`;
    row.querySelector(".segmented").addEventListener("click", (e) => { const b = e.target.closest("button"); if (b) set(b.dataset.v); });
    update = (s) => row.querySelectorAll("button").forEach((b) => b.setAttribute("aria-checked", String(b.dataset.v === s[r.key])));
  } else if (r.type === "select") {
    row.className = "adv-row";
    row.innerHTML = `<label for="${id}">${r.label}</label><select class="input" id="${id}">${r.options.map(([v, l]) => `<option value="${v}">${l}</option>`).join("")}</select>`;
    const inp = $("select", row);
    inp.addEventListener("change", () => set(r.num ? +inp.value : inp.value));
    update = (s) => { inp.value = String(s[r.key] ?? ""); };
  } else {
    row.className = "adv-row";
    const num = r.type === "number";
    row.innerHTML = `<label for="${id}">${r.label}</label><input class="input" id="${id}" ${num ? `type="number" min="${r.min}" max="${r.max}" step="${r.step || 1}"` : `type="text" placeholder="${r.placeholder || ""}"`}>`;
    const inp = $("input", row);
    inp.addEventListener("change", () => {
      if (num) { const v = +inp.value; if (inp.value !== "" && v >= r.min && v <= r.max) set(v); else { inp.value = S.config.settings[r.key]; toast(`Valeur entre ${r.min} et ${r.max}`, "err"); } }
      else set(inp.value.trim());
    });
    update = (s) => { if (document.activeElement !== inp) inp.value = s[r.key]; };
  }
  if (r.help) row.title = r.help;
  advUpdaters.push((s) => { update(s); row.classList.toggle("dis", r.dep ? !r.dep(s) : false); });
  return row;
}

function syncAdvanced() { advUpdaters.forEach((u) => u(S.config.settings)); }

$("#btnReset").addEventListener("click", () => selectPreset(currentPreset()));
$("#btnResetColor").addEventListener("click", () => {
  COLOR.forEach((k) => (S.config.settings[k] = S.defaults[k]));
  syncAll();
  changed();
});
$("#btnSavePreset").addEventListener("click", () => {
  $("#promptInput").value = "";
  openDlg("#promptDlg");
});
$("#promptOk").addEventListener("click", savePresetFromPrompt);
$("#promptInput").addEventListener("keydown", (e) => { if (e.key === "Enter") savePresetFromPrompt(); });
async function savePresetFromPrompt() {
  const name = $("#promptInput").value.trim();
  if (!name) return $("#promptInput").focus();
  S.config = await api().SavePreset(name, S.config.settings);
  closeDlg("#promptDlg");
  syncAll();
  toast(`Style « ${name} » enregistré`);
}

// ---------- Sauvegarde ----------
const saveConfig = debounce(() => api().SaveConfig(S.config).catch((e) => toast(String(e), "err")), 300);
function changed() { saveConfig(); if (previewState.open) requestPreview(); }
function syncAll() { renderPresets(); syncMain(); syncAdvanced(); updateStart(); }

// ---------- File d'attente ----------
const jobEls = new Map();

function renderJobs() {
  const q = $("#queue");
  const has = S.jobs.length > 0;
  $("#empty").hidden = has;
  $("#queueHead").hidden = !has;
  $("#drop").classList.toggle("compact", has);
  $("#queueCount").textContent = has ? `${S.jobs.length}` : "";
  const finished = S.jobs.some((j) => ["done", "error", "cancelled"].includes(j.status));
  $("#btnClear").hidden = !finished;

  const ids = new Set(S.jobs.map((j) => j.id));
  for (const [id, el] of jobEls) if (!ids.has(id)) { el.remove(); jobEls.delete(id); }
  S.jobs.forEach((j, i) => {
    let el = jobEls.get(j.id);
    if (!el) { el = createJobEl(j); jobEls.set(j.id, el); }
    if (q.children[i] !== el) q.insertBefore(el, q.children[i] || null);
    updateJobEl(el, j);
  });
  updateStart();
  refreshPreviewJobs();
}

function createJobEl(j) {
  const li = document.createElement("li");
  li.className = "job";
  li.innerHTML = `<div class="thumb">${icon("film")}</div>
    <div class="job-main"><div class="job-name"></div><div class="job-meta"></div><div class="job-status"></div><div class="bar" hidden><div class="bar-fill"></div></div></div>
    <div class="job-actions"></div>`;
  li.addEventListener("click", (e) => {
    const b = e.target.closest("[data-act]"); if (!b) return;
    const job = S.jobs.find((x) => x.id === j.id); if (!job) return;
    const act = b.dataset.act;
    if (act === "pause") api().TogglePause(job.id).catch((err) => toast(String(err), "err"));
    else if (act === "cancel") api().Cancel(job.id);
    else if (act === "remove") api().Remove(job.id);
    else if (act === "folder") api().Reveal(job.output);
    else if (act === "play") api().OpenFile(job.output);
    else if (act === "preview") openPreview(job.id);
    else if (act === "details") showDetails(job);
  });
  return li;
}

function updateJobEl(el, j) {
  const key = JSON.stringify([j.status, j.thumb ? 1 : 0, j.progress, j.error, j.info.duration]);
  if (el._key === key) return;
  el._key = key;
  el.className = `job ${j.status === "rendering" || j.status === "paused" ? "active" : ""} ${j.status}`;
  const th = $(".thumb", el);
  if (j.thumb && !th.style.backgroundImage) { th.style.backgroundImage = `url(${j.thumb})`; th.innerHTML = ""; }
  $(".job-name", el).textContent = j.name;
  $(".job-name", el).title = j.path;
  const i = j.info;
  $(".job-meta", el).textContent = i.duration ? [`${i.width}×${i.height}`, `${fmtFps(i.fps)} i/s`, fmtClock(i.duration), fmtSize(i.size)].filter(Boolean).join("  ·  ") : "";

  const st = $(".job-status", el), bar = $(".bar", el), fill = $(".bar-fill", el), p = j.progress || {};
  st.className = "job-status";
  bar.hidden = !(j.status === "rendering" || j.status === "paused");
  fill.style.width = `${p.percent || 0}%`;
  switch (j.status) {
    case "probing": st.textContent = S.installed ? "Analyse…" : "En attente du moteur"; break;
    case "ready": st.textContent = "Prêt"; break;
    case "queued": st.innerHTML = `<span class="badge">En attente</span>`; break;
    case "rendering":
      st.textContent = p.total ? `${Math.floor(p.percent)} %  ·  ${p.fps || "…"} i/s  ·  reste ${p.eta >= 0 ? fmtDur(p.eta) : "…"}` : "Préparation (indexation de la vidéo)…";
      break;
    case "paused": st.textContent = `En pause · ${Math.floor(p.percent || 0)} %`; break;
    case "done": st.classList.add("ok"); st.innerHTML = `${icon("check")}Terminé en ${fmtDur(j.elapsed)}`; break;
    case "cancelled": st.textContent = "Annulé"; break;
    case "error":
      st.classList.add("err");
      st.innerHTML = `${icon("alert")}<span>${esc(j.error)}</span>${j.details ? ` <button class="link-btn" data-act="details">Détails</button>` : ""}`;
      break;
  }

  const acts = [];
  const btn = (act, ic, label) => `<button class="icon-btn sm" data-act="${act}" title="${label}" aria-label="${label}">${icon(ic)}</button>`;
  if (j.status === "rendering" || j.status === "paused") {
    acts.push(j.status === "paused" ? btn("pause", "play", "Reprendre") : btn("pause", "pause", "Mettre en pause"));
    acts.push(btn("cancel", "x", "Annuler"));
  } else {
    if (j.status === "done") { acts.push(btn("play", "play", "Lire la vidéo")); acts.push(btn("folder", "folder", "Afficher dans le dossier")); }
    else if (j.info.duration) acts.push(btn("preview", "eye", "Aperçu"));
    if (j.status === "queued") acts.push(btn("cancel", "x", "Retirer de la file"));
    else acts.push(btn("remove", "trash", "Retirer"));
  }
  $(".job-actions", el).innerHTML = acts.join("");
}

function startable() { return S.jobs.filter((j) => j.status === "ready" || j.status === "cancelled" || (j.status === "error" && j.info.duration)); }

function updateStart() {
  const n = startable().length;
  const running = S.jobs.some((j) => j.status === "rendering" || j.status === "paused" || j.status === "queued");
  $("#btnStart").disabled = !S.installed || n === 0;
  $("#startLabel").textContent = n ? `Lancer le rendu${n > 1 ? ` (${n})` : ""}` : running ? "Rendu en cours…" : "Lancer le rendu";
  $("#btnPreview").disabled = !S.installed || !S.jobs.some((j) => j.info.duration);
}

$("#btnStart").addEventListener("click", start);
async function start() {
  if ($("#btnStart").disabled) return;
  if (rifeBroken(S.config.settings)) return toast(RIFE_WARN, "err", 7000);
  const p = currentPreset();
  const name = isModified() ? `${p.name} (modifié)` : p.name;
  await api().SaveConfig(S.config);
  const n = await api().Start(name);
  if (n) toast(`${n} vidéo(s) ajoutée(s) au rendu`);
}
$("#btnClear").addEventListener("click", () => api().ClearFinished());
$("#drop").addEventListener("click", () => api().BrowseFiles());
$("#drop").addEventListener("keydown", (e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); api().BrowseFiles(); } });

function showDetails(j) {
  $("#detailsMsg").textContent = j.error;
  $("#detailsLog").textContent = j.details || "(aucun journal)";
  openDlg("#detailsDlg");
}
$("#btnCopyLog").addEventListener("click", () => {
  navigator.clipboard.writeText(`${$("#detailsMsg").textContent}\n\n${$("#detailsLog").textContent}`).then(() => toast("Journal copié"));
});

// ---------- Aperçu ----------
const previewState = { open: false, id: 0, token: 0 };

function refreshPreviewJobs() {
  const sel = $("#previewJob");
  const list = S.jobs.filter((j) => j.info.duration);
  const cur = sel.value;
  sel.innerHTML = list.map((j) => `<option value="${j.id}">${esc(j.name)}</option>`).join("");
  if (list.some((j) => String(j.id) === cur)) sel.value = cur;
}

function openPreview(id) {
  const list = S.jobs.filter((j) => j.info.duration);
  if (!list.length) return;
  refreshPreviewJobs();
  if (!id) id = +$("#previewJob").value || list[0].id;
  $("#previewJob").value = id;
  setPreviewJob(id, true);
  previewState.open = true;
  openDlg("#previewDlg");
}

function setPreviewJob(id, resetTime) {
  previewState.id = +id;
  const j = S.jobs.find((x) => x.id === +id);
  if (!j) return;
  const t = $("#previewTime");
  t.max = Math.max(0.1, j.info.duration - 0.2);
  if (resetTime) t.value = Math.min(j.info.duration * 0.4, +t.max);
  paintRange(t);
  $("#previewTimeOut").textContent = fmtClock(+t.value);
  $("#imgBefore").removeAttribute("src");
  $("#imgAfter").removeAttribute("src");
  requestPreview(true);
}

const requestPreview = debounce(async () => {
  if (!previewState.open) return;
  const token = ++previewState.token;
  if (rifeBroken(S.config.settings)) { $("#previewError").textContent = RIFE_WARN; $("#previewError").hidden = false; return; }
  $("#previewLoading").hidden = false;
  $("#previewError").hidden = true;
  const res = await api().Preview(previewState.id, +$("#previewTime").value, S.config.settings);
  if (token !== previewState.token) return;
  $("#previewLoading").hidden = true;
  if (res.error) {
    if (res.error === "cancelled") return;
    $("#previewError").textContent = res.error;
    $("#previewError").hidden = false;
    return;
  }
  if (res.before) $("#imgBefore").src = res.before;
  $("#imgAfter").src = res.after;
  $("#previewMs").textContent = `calculé en ${(res.ms / 1000).toFixed(1)} s`;
}, 350);

$("#btnPreview").addEventListener("click", () => openPreview());
$("#previewJob").addEventListener("change", (e) => setPreviewJob(e.target.value, true));
$("#previewTime").addEventListener("input", (e) => { paintRange(e.target); $("#previewTimeOut").textContent = fmtClock(+e.target.value); requestPreview(); });
$("#split").addEventListener("input", (e) => $("#compare").style.setProperty("--split", `${e.target.value}%`));

// ---------- Paramètres ----------
function syncSettingsDlg() {
  const c = S.config;
  $("#outDirLabel").textContent = c.outputDir || "À côté de chaque vidéo source";
  $("#optLowPrio").checked = c.lowPriority;
  $("#optAwake").checked = c.keepAwake;
  $("#optNotify").checked = c.notify;
  $("#optReveal").checked = c.revealOnDone;
}
$("#btnSettings").addEventListener("click", () => { syncSettingsDlg(); openDlg("#settingsDlg"); });
$("#btnAbout").addEventListener("click", () => openDlg("#aboutDlg"));
[["#optLowPrio", "lowPriority"], ["#optAwake", "keepAwake"], ["#optNotify", "notify"], ["#optReveal", "revealOnDone"]].forEach(([sel, k]) =>
  $(sel).addEventListener("change", (e) => { S.config[k] = e.target.checked; saveConfig(); }));
$("#btnOutSame").addEventListener("click", () => { S.config.outputDir = ""; saveConfig(); syncSettingsDlg(); });
$("#btnOutPick").addEventListener("click", async () => { const d = await api().BrowseOutputDir(); if (d) { S.config.outputDir = d; saveConfig(); syncSettingsDlg(); } });
$("#btnEngineFolder").addEventListener("click", () => api().OpenEngineFolder());
$("#btnReinstall").addEventListener("click", () => { closeDlg("#settingsDlg"); showSetup(true); });

function gpuName() {
  const g = S.gpus; if (!g) return "";
  return g.nvidia ? "NVIDIA" : g.amd ? "AMD" : g.intel ? "Intel" : "";
}
function syncGpu() {
  const pill = $("#gpuPill");
  if (!S.installed || !S.gpus) { pill.hidden = true; return; }
  const n = gpuName();
  pill.hidden = false;
  pill.className = `status-chip ${n ? "ok" : ""}`;
  pill.textContent = n ? `GPU ${n}` : "Encodage CPU";
  pill.title = n ? `Encodeur matériel ${n} détecté (activable dans l'onglet Sortie)` : "Aucun encodeur matériel utilisable détecté";
  syncAdvanced();
}

// ---------- Installation du moteur ----------
function showSetup(reinstall) {
  $("#setupSize").textContent = `Téléchargement unique d'environ ${Math.round(S.downloadSize / 1e6)} Mo · dossier : ${S.enginePath}`;
  $("#installProgress").hidden = true;
  $("#installError").hidden = true;
  $("#btnInstall").hidden = false;
  $("#btnInstall").disabled = false;
  $("#setupTitle").textContent = reinstall ? "Réinstaller le moteur" : "Bienvenue dans NeiBlur";
  $("#setup").hidden = false;
}
$("#btnInstall").addEventListener("click", () => {
  $("#btnInstall").disabled = true;
  $("#btnInstall").hidden = true;
  $("#installProgress").hidden = false;
  $("#installError").hidden = true;
  api().InstallEngine();
});
on("install", (p) => {
  $("#installBar").style.width = `${p.percent}%`;
  $("#installStep").textContent = `${p.stepIndex}/${p.stepCount} · ${p.step}`;
  $("#installPct").textContent = `${Math.floor(p.percent)} %`;
});
on("install-done", (err) => {
  if (err) {
    $("#installError").textContent = `Installation interrompue : ${err}. Vérifie ta connexion puis réessaie.`;
    $("#installError").hidden = false;
    $("#btnInstall").hidden = false;
    $("#btnInstall").disabled = false;
    $("#btnInstall").lastChild.textContent = "Réessayer";
    return;
  }
  S.installed = true;
  $("#setup").hidden = true;
  toast("Moteur installé, tout est prêt !");
  api().ProbePending();
  syncAll();
  renderJobs();
});

// ---------- Événements du moteur ----------
on("jobs", (jobs) => { S.jobs = jobs || []; renderJobs(); });
on("gpus", (g) => { S.gpus = g; syncGpu(); });
on("rife", (ok) => { S.rifeOk = ok; renderPresets(); syncMain(); });

// ---------- Clavier ----------
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape") { const o = [...document.querySelectorAll(".overlay:not([hidden])")].pop(); if (o && o.id !== "setup") closeDlg(o); }
  if (e.ctrlKey && e.key.toLowerCase() === "o") { e.preventDefault(); api().BrowseFiles(); }
  if (e.ctrlKey && e.key === "Enter") { e.preventDefault(); start(); }
  if (e.ctrlKey && e.key === ",") { e.preventDefault(); syncSettingsDlg(); openDlg("#settingsDlg"); }
  if (e.ctrlKey && e.key.toLowerCase() === "p" && !$("#btnPreview").disabled) { e.preventDefault(); openPreview(); }
});
document.addEventListener("contextmenu", (e) => { if (!e.target.closest("input,textarea,pre")) e.preventDefault(); });

// ---------- Démarrage ----------
async function boot() {
  if (!window.go?.main?.App || !window.runtime) return setTimeout(boot, 30);
  const st = await api().Init();
  Object.assign(S, { presets: st.presets, defaults: st.defaults, config: st.config, gpus: st.gpus, rifeOk: st.rifeOk, jobs: st.jobs || [],
    installed: st.installed, version: st.version, downloadSize: st.downloadSize, enginePath: st.enginePath });
  S.config.userPresets ||= [];
  $("#version").textContent = `v${st.version}`;
  $("#aboutVersion").textContent = `v${st.version}`;
  $("#engineLabel").textContent = st.enginePath;
  buildAdvanced();
  let tab = "blur";
  try { tab = localStorage.getItem("tab") || "blur"; } catch {}
  showTab(["blur", "color", "output"].includes(tab) ? tab : "blur");
  syncAll();
  renderJobs();
  syncGpu();
  if (!st.installed) showSetup(false);
}
boot();
