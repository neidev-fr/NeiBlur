# NeiBlur — colorimétrie (étalonnage simple, appliqué après le flou, à la fréquence de sortie).
# Tout est calculé en RVB flottant 0..1 ; chaque étape n'est ajoutée que si elle change l'image.

import vapoursynth as vs
from vapoursynth import core

LUMA = (0.2126, 0.7152, 0.0722)

# Looks : décalages ajoutés aux réglages de l'utilisateur, multipliés par l'intensité du look.
# split_shadows / split_highlights : teinte ajoutée aux ombres / hautes lumières (R, V, B).
LOOKS = {
    "warm": {"temperature": 0.35, "vibrance": 0.1},
    "cool": {"temperature": -0.35, "tint": -0.05},
    "teal_orange": {
        "vibrance": 0.15, "curve": 0.15,
        "split_shadows": (-0.06, 0.02, 0.07), "split_highlights": (0.07, 0.02, -0.06),
    },
    "film": {
        "curve": 0.2, "saturation": -0.1, "fade": 0.04,
        "split_shadows": (0.0, 0.01, 0.03), "split_highlights": (0.04, 0.02, -0.02),
    },
    "vintage": {
        "fade": 0.1, "saturation": -0.3, "temperature": 0.2, "curve": -0.05,
        "split_highlights": (0.05, 0.04, -0.04),
    },
    "vivid": {"vibrance": 0.45, "curve": 0.12},
    "night": {
        "temperature": -0.25, "tint": 0.15, "curve": 0.1,
        "split_shadows": (0.02, -0.02, 0.07), "split_highlights": (0.05, -0.02, 0.05),
    },
    "bw": {"saturation": -1.0, "curve": 0.12},
}

MATRICES = {"709", "170m", "470bg", "2020ncl", "240m", "fcc"}


def is_active(s: dict) -> bool:
    keys = ("exposure", "temperature", "tint", "shadows", "highlights", "vibrance", "fade", "vignette", "sharpen")
    return any(abs(float(s.get(k, 0))) > 1e-6 for k in keys) or s.get("look", "") in LOOKS


def _params(s: dict) -> dict:
    p = {k: float(s.get(k, 0)) for k in ("exposure", "temperature", "tint", "shadows", "highlights", "vibrance", "fade", "vignette", "sharpen")}
    p.update(saturation=0.0, curve=0.0, split_shadows=(0.0, 0.0, 0.0), split_highlights=(0.0, 0.0, 0.0))
    look = LOOKS.get(s.get("look", ""))
    if look:
        amount = float(s.get("look_amount", 1))
        for k, v in look.items():
            if isinstance(v, tuple):
                p[k] = tuple(a + b * amount for a, b in zip(p[k], v))
            else:
                p[k] += v * amount
    return p


def _f(v: float) -> str:
    return f"{v:.6f}"


def grade(video: vs.VideoNode, is_full_color_range: bool, matrix: str, s: dict) -> vs.VideoNode:
    if not is_active(s):
        return video

    orig = video.format
    if matrix not in MATRICES:
        matrix = "709" if video.height >= 720 else "170m"

    rgb = video
    if orig.color_family != vs.RGB or orig.sample_type != vs.FLOAT:
        kw = {"format": vs.RGBS}
        if orig.color_family == vs.YUV:
            kw.update(matrix_in_s=matrix, range_in=int(is_full_color_range))
        rgb = core.resize.Bicubic(video, **kw)

    p = _params(s)
    rgb = _apply(rgb, p)

    if rgb.format.id != orig.id:
        kw = {"format": orig.id, "dither_type": "error_diffusion"}
        if orig.color_family == vs.YUV:
            kw.update(matrix_s=matrix, range=int(is_full_color_range))
        rgb = core.resize.Bicubic(rgb, **kw)
    return rgb


def _apply(c: vs.VideoNode, p: dict) -> vs.VideoNode:
    # 1. exposition + balance des blancs (gain par canal, luminance conservée)
    t, m = p["temperature"], p["tint"]
    gains = [1 + 0.2 * t, 1 - 0.15 * m, 1 - 0.2 * t]
    k = sum(g * w for g, w in zip(gains, LUMA))
    ev = 2 ** p["exposure"]
    gains = [g / k * ev for g in gains]
    if any(abs(g - 1) > 1e-6 for g in gains):
        c = core.std.Expr(c, [f"x {_f(g)} *" for g in gains])

    # 2. noirs délavés (fondu)
    fade = max(-0.5, min(0.5, p["fade"]))
    if abs(fade) > 1e-6:
        c = core.std.Expr(c, f"x 0 max 1 min 1 {_f(fade)} - * {_f(fade)} +")

    # 3. ombres / hautes lumières / courbe en S (zéro et blanc restent fixes)
    sh, hi, cv = p["shadows"], p["highlights"], p["curve"]
    if abs(sh) + abs(hi) + abs(cv) > 1e-6:
        e = "x 0 max 1 min v! v@ "
        if abs(sh) > 1e-6:  # bosse centrée vers 1/3
            e += f"v@ 1 v@ - dup * * {_f(0.9 * sh)} * + "
        if abs(hi) > 1e-6:  # bosse centrée vers 2/3
            e += f"v@ dup * 1 v@ - * {_f(0.9 * hi)} * + "
        if abs(cv) > 1e-6:
            e += f"v@ 2 * 1 - v@ * 1 v@ - * {_f(2 * cv)} * + "
        c = core.akarin.Expr(c, e)

    # 4. saturation / vibrance / virage partiel (opérations entre canaux)
    sat = max(0.0, 1 + p["saturation"])
    vib = p["vibrance"]
    so, ho = p["split_shadows"], p["split_highlights"]
    if abs(sat - 1) > 1e-6 or abs(vib) > 1e-6 or any(abs(v) > 1e-6 for v in so + ho):
        r, g, b = (core.std.ShufflePlanes(c, i, vs.GRAY) for i in range(3))
        pre = (f"x {LUMA[0]} * y {LUMA[1]} * + z {LUMA[2]} * + lum! "
               "x y max z max x y min z min - 0 max 1 min chr! ")
        outs = []
        for ch, sname in enumerate("xyz"):
            e = pre + f"lum@ {sname} lum@ - {_f(sat)} 1 {_f(vib)} 1 chr@ - * + 0 max * * + "
            if abs(so[ch]) > 1e-6:
                e += f"1 lum@ 0 max 1 min - dup * {_f(so[ch])} * + "
            if abs(ho[ch]) > 1e-6:
                e += f"lum@ 0 max 1 min dup * {_f(ho[ch])} * + "
            outs.append(core.akarin.Expr([r, g, b], e))
        c = core.std.ShufflePlanes(outs, [0, 0, 0], vs.RGB)

    # 5. vignette (négatif = coins plus clairs)
    vig = p["vignette"]
    if abs(vig) > 1e-6:
        c = core.akarin.Expr(
            c, f"X width / 0.5 - dup * Y height / 0.5 - dup * + 2 * 1.5 pow {_f(vig)} * 1 swap - x *"
        )

    # 6. netteté (masque flou)
    shp = p["sharpen"]
    if abs(shp) > 1e-6:
        soft = core.std.Convolution(c, matrix=[1, 2, 1, 2, 4, 2, 1, 2, 1])
        c = core.akarin.Expr([c, soft], f"x x y - {_f(shp * 1.5)} * +")

    return core.std.Expr(c, "x 0 max 1 min")
