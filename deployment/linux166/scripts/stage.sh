set -eu
mountpoint -q /home
test ! -e /home/coze/src/coze-studio
install -d -m 0750 /home/coze/src/coze-studio
tar -xzf /home/coze/tmp/coze-v0.5.1.tar.gz -C /home/coze/src/coze-studio
python3 /home/coze/tmp/prepare.py
install -m 0750 /home/coze/tmp/capacity.py /home/coze/ops/capacity.py
python3 /home/coze/ops/capacity.py --guard
cd /home/coze/src/coze-studio/docker
docker compose -f docker-compose.deploy.json config --quiet
cat > /home/coze/ops/compose.sh <<'SH'
#!/bin/bash
set -euo pipefail
cd /home/coze/src/coze-studio/docker
exec docker compose -f docker-compose.deploy.json "$@"
SH
chmod 750 /home/coze/ops/compose.sh
cat > /etc/systemd/system/coze-capacity.service <<'UNIT'
[Unit]
Description=Coze Studio 磁盘容量检查
RequiresMountsFor=/home/coze
[Service]
Type=oneshot
ExecStart=/usr/bin/python3 /home/coze/ops/capacity.py
UNIT
cat > /etc/systemd/system/coze-capacity.timer <<'UNIT'
[Unit]
Description=每十五分钟检查 Coze Studio 磁盘容量
[Timer]
OnBootSec=2min
OnUnitActiveSec=15min
[Install]
WantedBy=timers.target
UNIT
cat > /etc/systemd/system/coze-studio.service <<'UNIT'
[Unit]
Description=Coze Studio 开发验证平台
Requires=docker.service
After=docker.service network-online.target
RequiresMountsFor=/home/coze
[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=/home/coze/src/coze-studio/docker
ExecStartPre=/usr/bin/python3 /home/coze/ops/capacity.py --guard
ExecStart=/usr/bin/bash /home/coze/ops/compose.sh up -d
ExecStop=/usr/bin/bash /home/coze/ops/compose.sh stop
TimeoutStartSec=900
[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable --now coze-capacity.timer
printf '镜像清单\n'
docker compose -f docker-compose.deploy.json config --images
