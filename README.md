# NeiBlur


Ajoute du motion blur à tes vidéos, simplement. **Créé par neidev.**

NeiBlur est une refonte de [Blur](https://github.com/f0e/blur) (f0e) avec une interface simplifiée, des préréglages prêts à l'emploi et un **seul fichier à lancer** : `NeiBlur.exe` (~12 Mo).

<p align="center">
  <img src="http://www.image-heberg.fr/files/1790979578577192846.png" alt="Interface de NeiBlur" width="80%">
</p>

## Utilisation

1. Télécharge `NeiBlur.exe` depuis la page [Releases](https://github.com/neidev-fr/NeiBlur/releases) et lance-le. Au premier démarrage, clique **Installer le moteur** (téléchargement unique d'environ 118 Mo, sans droits administrateur, fichiers vérifiés par SHA-256).
2. Glisse tes vidéos (ou un dossier) dans la fenêtre.
3. Choisis un style, ajuste l'intensité si besoin, puis clique **Lancer le rendu**.

La vidéo est créée à côté de l'originale : `ma vidéo - blur.mp4`.

| Style | Pour quoi |
|---|---|
| Gaming fluide | Réglages d'origine de Blur (1200 i/s SVP, flou 100 %, 60 i/s) |
| Lumière réaliste | Flou mélangé en lumière linéaire (gamma 2.2) avec obturateur doux : vraies traînées lumineuses |
| Cinéma | Obturation 180°, 30 i/s, obturateur doux et lumière réaliste |
| Subtil | Flou léger (30 %) à fondu en cloche |
| Intense | Traînées marquées (180 %) |
| Extrême | Traînées très longues (350 %) pour les effets stylisés |
| Qualité max | Interpolation IA RIFE (plus lent, moins d'artefacts) |
| Brouillon rapide | Tests rapides (360 i/s, fichier léger) |

Les réglages sont rangés en trois onglets :

- **Flou** : style, intensité (jusqu'à 500 %), images/s de sortie (jusqu'à 2000), forme du flou (obturateur doux, cloche, uniforme…), lumière réaliste, et tous les réglages experts de Blur (interpolation, déduplication, vitesse, SVP).
- **Couleur** : looks prêts à l'emploi (Film, Teal & orange, Chaud, Froid, Vif, Vintage, Nuit, Noir et blanc), exposition, contraste, hautes lumières, ombres, noirs délavés, température, teinte, saturation, vibrance, vignettage et netteté. La colorimétrie est appliquée après le flou et visible dans l'aperçu.
- **Sortie** : codec, qualité, encodage GPU, résolution (480p à 4K, vidéos verticales comprises), format MP4 / MKV / MOV, son (suppression, débit).

Tu peux enregistrer tes réglages comme style personnel. Double-clic sur un curseur pour le remettre à sa valeur d'origine.

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

**Sans rien installer** : chaque envoi sur `main` est compilé automatiquement par GitHub Actions (onglet *Actions* du dépôt, l'exe est dans les *Artifacts*). Pour publier une version, onglet *Actions* → *Build* → *Run workflow* : une Release `vX.Y.Z` est créée avec l'exe (la version est lue dans `main.go`).

**Sur ton PC** — prérequis : Go 1.23+ et Wails v2 (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`).

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
engine/scripts/          scripts VapourSynth de Blur (GPL-3.0) + blur/grading.py (colorimétrie NeiBlur)
frontend/dist/           interface (HTML/CSS/JS sans framework)
```

## Crédits et licence

- **neidev** : conception et développement de NeiBlur.
- [Blur](https://github.com/f0e/blur) par f0e : moteur et scripts VapourSynth d'origine (GPL-3.0).
- [VapourSynth](https://www.vapoursynth.com/), [SVPflow](https://www.svp-team.com/wiki/Manual:SVPflow), [RIFE ncnn Vulkan](https://github.com/styler00dollar/VapourSynth-RIFE-ncnn-Vulkan), [FFmpeg](https://ffmpeg.org/), [Wails](https://wails.io/).

NeiBlur reprend les scripts VapourSynth de Blur. Il est donc distribué sous **GPL-3.0** (voir `LICENSE`), et son code source doit rester disponible.
