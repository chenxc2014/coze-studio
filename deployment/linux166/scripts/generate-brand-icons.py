"""从已提供的 PNG 原图生成 ICO 尺寸，保持图形比例和透明背景。"""
from pathlib import Path
import shutil
from PIL import Image, ImageOps

root = Path(__file__).resolve().parents[3]
assets = root / 'frontend/apps/coze-studio/assets'
source = Image.open(assets / 'favicon.png').convert('RGBA')
canvas = Image.new('RGBA', (256, 256), (0, 0, 0, 0))
scaled = ImageOps.contain(source, canvas.size, Image.Resampling.LANCZOS)
canvas.alpha_composite(scaled, ((256 - scaled.width) // 2, (256 - scaled.height) // 2))
canvas.save(assets / 'favicon.ico', format='ICO', sizes=[(16,16),(32,32),(48,48),(64,64),(128,128),(256,256)])
shutil.copyfile(assets / 'favicon.ico', root / 'backend/conf/admin/favicon.ico')
shutil.copyfile(assets / 'favicon.png', root / 'backend/conf/admin/platform-logo.png')
print('ICO generated from provided PNG; aspect ratio preserved.')
