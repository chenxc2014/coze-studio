set -eu
python3 - <<'PY'
import gzip,json,subprocess
from pathlib import Path
daemon=json.loads(Path('/etc/docker/daemon.json').read_text())
assert daemon['data-root']=='/home/coze/runtime/docker'
assert 'proxies' not in daemon, '临时代理配置未移除'
print('临时下载代理已移除。')
assert (Path('/home/coze/ops/admin-initial.json').stat().st_mode & 0o077)==0
print('管理员初始凭据权限检查通过。')
backups=sorted(Path('/home/coze/backups').glob('metadata-*.sql.gz'))
assert backups
with gzip.open(backups[-1],'rb') as stream:
    data=stream.read()
assert b'CREATE TABLE' in data and b'Dump completed' in data
print(f'备份压缩完整性检查通过：{backups[-1].name}，解压后 {len(data)} 字节。')
info=json.loads(subprocess.check_output(['docker','inspect','coze-server','coze-web']))
print('应用重启次数：'+str({x['Name']:x['RestartCount'] for x in info}))
PY
systemctl is-active docker containerd coze-studio coze-capacity.timer coze-backup.timer
systemctl is-enabled docker containerd coze-studio coze-capacity.timer coze-backup.timer
systemctl show coze-backup.service --property=Result,ExecMainStatus
getenforce
df -hT / /home
