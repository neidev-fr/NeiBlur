# NeiBlur

Ajoute du motion blur à tes vidéos, simplement. **Créé par neidev.**

NeiBlur est une refonte de [Blur](https://github.com/f0e/blur) (f0e) avec une interface simplifiée, des préréglages prêts à l'emploi et un **seul fichier à lancer** : `NeiBlur.exe` (~12 Mo).

## Utilisation

1. Lance `NeiBlur.exe`. Au premier démarrage, clique **Installer le moteur** (téléchargement unique d'environ 118 Mo, sans droits administrateur, fichiers vérifiés par SHA-256).
2. Glisse tes vidéos (ou un dossier) dans la fenêtre.
3. Choisis un style, ajuste l'intensité si besoin, puis clique **Lancer le rendu**.

La vidéo est créée à côté de l'originale : `ma vidéo - blur.mp4`.

| Préréglage | Pour quoi |
|---|---|
| Gaming fluide | Réglages d'origine de Blur (1200 i/s SVP, flou 100 %, 60 i/s) |
| Cinéma | Obturation 180°, 30 i/s, pondération gaussienne |
| Subtil | Flou léger (30 %) |
| Intense | Traînées marquées (160 %, pyramide) |
| Qualité max | Interpolation IA RIFE (plus lent, moins d'artefacts) |
| Brouillon rapide | Tests rapides (360 i/s, fichier léger) |

Tous les réglages de Blur restent disponibles dans **Réglages avancés**, et tu peux enregistrer les tiens comme préréglage.

Raccourcis : `Ctrl+O` ajouter · `Ctrl+Entrée` lancer · `Ctrl+P` aperçu · `Ctrl+,` paramètres · `Échap` fermer.

**Mode portable** : crée un dossier `NeiBlur-data` à côté de l'exe, et le moteur ainsi que la configuration y seront stockés au lieu de `%LOCALAPPDATA%\NeiBlur`.

## Performances

Le moteur de rendu est **exactement celui de Blur** : mêmes composants aux mêmes versions (VapourSynth R70, SVPflow 4.2, RIFE ncnn Vulkan r9_mod_v33 avec le modèle 4.26, Akarin, L-SMASH Works, FFmpeg), mêmes scripts VapourSynth et même chaîne `VSPipe → FFmpeg`. Les performances sont donc identiques. Quelques ajouts :

- un tube de 4 Mo entre VSPipe et FFmpeg, au lieu du tampon par défaut ;
- les processus de rendu en priorité basse (optionnel), pour que le PC reste utilisable ;
- la mise en veille bloquée pendant un rendu ;
- l'aperçu avant/après ne calcule qu'**une seule image** (`vspipe -s N -e N`), grâce à l'évaluation paresseuse de VapourSynth ;
- l'encodeur GPU réellement testé au démarrage, et pas seulement listé ;
- l'AV1 sur CPU passe par SVT-AV1 au lieu de libaom, ce qui est beaucoup plus rapide ;
- le runtime OpenMP (nécessaire à RIFE) est déployé à côté du moteur, donc pas besoin d'installer VC++ Redist en administrateur ;
- un autotest RIFE (moins d'une seconde) au démarrage : certains pilotes GPU, notamment les anciens pilotes Intel, font produire des images vides (vidéo verte) à RIFE sans aucune erreur. NeiBlur le détecte, prévient l'utilisateur et bloque ces réglages au lieu de gâcher un rendu.

## Compiler

Prérequis : Go 1.23+ et Wails v2 (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`).

```powershell
go run ./cmd/genicon            # génère build/appicon.png
wails build -clean -trimpath -ldflags "-s -w" -skipbindings
# → build/bin/NeiBlur.exe
```

Tester le moteur sans l'interface :

```powershell
go run ./cmd/enginetest install
go run ./cmd/enginetest render "ma vidéo.mp4" gaming
go run ./cmd/enginetest preview "ma vidéo.mp4" 3.5
```

## Structure

```
main.go / app.go         fenêtre Wails, file d'attente, API exposée à l'interface
engine/install.go        installation du moteur (URLs épinglées + SHA-256)
engine/render.go         analyse ffprobe, rendu VSPipe | FFmpeg, aperçu, pause/reprise
engine/settings.go       réglages, préréglages, encodeurs
engine/win.go            priorité, suspension de processus, anti-veille (API Windows)
engine/scripts/          scripts VapourSynth de Blur (GPL-3.0)
frontend/dist/           interface (HTML/CSS/JS sans framework)
```

## Crédits et licence

- **neidev** : conception et développement de NeiBlur.
- [Blur](https://github.com/f0e/blur) par f0e : moteur et scripts VapourSynth d'origine (GPL-3.0).
- [VapourSynth](https://www.vapoursynth.com/), [SVPflow](https://www.svp-team.com/wiki/Manual:SVPflow), [RIFE ncnn Vulkan](https://github.com/styler00dollar/VapourSynth-RIFE-ncnn-Vulkan), [FFmpeg](https://ffmpeg.org/), [Wails](https://wails.io/).

NeiBlur reprend les scripts VapourSynth de Blur. Il est donc distribué sous **GPL-3.0** (voir `LICENSE`), et son code source doit rester disponible.
