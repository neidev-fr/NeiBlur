# NeiBlur : autotest de RIFE. Certains pilotes GPU (Intel anciens notamment) renvoient des images NaN,
# ce qui donne une vidéo verte sans aucune erreur. On interpole deux images unies et on vérifie le résultat.
import math
import sys

import vapoursynth as vs

core = vs.core
model_path = vars().get("model_path", "")

a = core.std.BlankClip(width=128, height=128, format=vs.RGBS, length=1, color=[0.1, 0.2, 0.3], fpsnum=24)
b = core.std.BlankClip(width=128, height=128, format=vs.RGBS, length=1, color=[0.9, 0.8, 0.7], fpsnum=24)
r = core.rife.RIFE(a + b + b, fps_num=48, fps_den=1, model_path=model_path, gpu_id=0)
avg = core.std.PlaneStats(r[1], plane=0).get_frame(0).props["PlaneStatsAverage"]
ok = not math.isnan(avg) and 0.05 < avg < 0.95
print("RIFE_OK" if ok else f"RIFE_BAD {avg}", file=sys.stderr)
a.set_output()
