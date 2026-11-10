"""检查实际挂载与预留容量；只记录告警，不自动删除业务数据。"""
import argparse
import json
import os
import shutil
import sys
from datetime import datetime, timezone
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument('--guard', action='store_true')
args = parser.parse_args()
home = shutil.disk_usage('/home')
root = shutil.disk_usage('/')
warnings = []
if os.stat('/home').st_dev == os.stat('/').st_dev:
    warnings.append('/home 不是独立分区，拒绝继续部署')
for path, space, reserve in [('/home',home,25),('/',root,15)]:
    percent = space.used / space.total * 100
    if percent >= 80 or space.free < reserve*1024**3:
        warnings.append(f'{path} 已用 {percent:.1f}%，剩余 {space.free/1024**3:.1f} GiB')
    filesystem=os.statvfs(path)
    if filesystem.f_files and (1-filesystem.f_favail/filesystem.f_files)>=0.8:
        warnings.append(f'{path} inode 使用率达到 80%')
for path in ['/home/coze/data','/home/coze/runtime/docker','/home/coze/runtime/containerd']:
    if not Path(path).is_dir() or os.stat(path).st_dev != os.stat('/home').st_dev:
        warnings.append(f'数据目录挂载异常：{path}')
result = {'time':datetime.now(timezone.utc).isoformat(),
          'filesystems':{p:{'total_GiB':round(s.total/1024**3,2),'free_GiB':round(s.free/1024**3,2),
                             'used_percent':round(s.used/s.total*100,1)} for p,s in [('/',root),('/home',home)]},
          'warnings':warnings}
if not args.guard:
    Path('/home/coze/ops/capacity-latest.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
print(json.dumps(result,ensure_ascii=False))
sys.exit(1 if warnings else 0)
