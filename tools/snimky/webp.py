"""Prevedie snímky PNG v docs/obrazky a site/obrazky na WebP (menšie, rovnako čitateľné)."""
import glob
import os

from PIL import Image

root = os.path.join(os.path.dirname(__file__), '..', '..')
for d in ('docs/obrazky', 'site/obrazky'):
    for f in glob.glob(os.path.join(root, d, '*.png')):
        Image.open(f).save(f[:-4] + '.webp', 'WEBP', quality=88, method=6)
        os.remove(f)
