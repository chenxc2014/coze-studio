set -eu
test "$(id -u)" = 0
mountpoint -q /home
test ! -d /var/lib/docker || test -z "$(ls -A /var/lib/docker)"
test ! -d /var/lib/containerd || test -z "$(ls -A /var/lib/containerd)"
python3 - <<'PY'
import shutil
assert shutil.disk_usage('/home').free > 60*1024**3, '/home 可用空间不足 60 GiB'
assert shutil.disk_usage('/').free > 15*1024**3, '系统分区可用空间不足 15 GiB'
PY
install -d -m 0750 /home/coze/{src,data,runtime,backups,logs,tmp,ops}
install -d -m 0711 /home/coze/runtime/{docker,containerd}
dnf -y install dnf-plugins-core policycoreutils-python-utils python3 curl tar git
if curl -fsSL --connect-timeout 10 --max-time 20 https://download.docker.com/linux/centos/docker-ce.repo -o /home/coze/tmp/docker-ce.repo; then
  install -m 0644 /home/coze/tmp/docker-ce.repo /etc/yum.repos.d/docker-ce.repo
else
  # 官方站点在此网络被重置时，采用保留 RPM 签名校验的阿里云镜像。
  curl -fsSL --connect-timeout 10 --max-time 30 https://mirrors.aliyun.com/docker-ce/linux/centos/docker-ce.repo -o /home/coze/tmp/docker-ce.repo
  install -m 0644 /home/coze/tmp/docker-ce.repo /etc/yum.repos.d/docker-ce.repo
fi
dnf -y install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
install -d /etc/docker /etc/systemd/system/docker.service.d /etc/systemd/system/containerd.service.d
python3 - <<'PY'
import json
from pathlib import Path
p=Path('/etc/docker/daemon.json')
assert not p.exists(), '检测到已有 Docker 配置，需先合并'
p.write_text(json.dumps({'data-root':'/home/coze/runtime/docker','log-driver':'local',
    'log-opts':{'max-size':'20m','max-file':'3'},'live-restore':True,
    'max-concurrent-downloads':2}, indent=2)+'\n')
p=Path('/etc/containerd/config.toml')
text=p.read_text()
p.with_suffix('.toml.coze-backup').write_text(text)
import re
text,count=re.subn(r'^root\s*=.*$', 'root = "/home/coze/runtime/containerd"', text, count=1, flags=re.M)
if not count:
    text='root = "/home/coze/runtime/containerd"\n'+text
p.write_text(text)
for name in ['docker','containerd']:
    Path(f'/etc/systemd/system/{name}.service.d/coze-storage.conf').write_text(
        '[Unit]\nRequiresMountsFor=/home/coze/runtime\n')
PY
semanage fcontext -a -e /var/lib/docker /home/coze/runtime/docker
semanage fcontext -a -e /var/lib/containerd /home/coze/runtime/containerd
restorecon -RF /home/coze/runtime/docker /home/coze/runtime/containerd
printf 'vm.max_map_count=262144\n' > /etc/sysctl.d/90-coze.conf
sysctl -p /etc/sysctl.d/90-coze.conf
systemctl daemon-reload
systemctl enable --now containerd docker
docker info --format 'Docker 数据目录={{.DockerRootDir}} 存储驱动={{.Driver}} 日志驱动={{.LoggingDriver}}'
containerd config dump | head -n 12
docker compose version
df -hT / /home
