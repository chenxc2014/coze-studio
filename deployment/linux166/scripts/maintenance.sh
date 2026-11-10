set -eu
install -m 0750 /home/coze/tmp/backup.py /home/coze/ops/backup.py
install -d /etc/systemd/journald.conf.d
cat > /etc/systemd/journald.conf.d/coze-capacity.conf <<'CONF'
[Journal]
SystemMaxUse=512M
SystemKeepFree=10G
RuntimeMaxUse=128M
CONF
systemctl restart systemd-journald
cat > /etc/systemd/system/coze-backup.service <<'UNIT'
[Unit]
Description=Coze Studio 元数据备份（最多七份）
RequiresMountsFor=/home/coze
After=docker.service
[Service]
Type=oneshot
ExecStart=/usr/bin/python3 /home/coze/ops/backup.py
UNIT
cat > /etc/systemd/system/coze-backup.timer <<'UNIT'
[Unit]
Description=每日备份 Coze Studio 元数据
[Timer]
OnCalendar=*-*-* 03:30:00
Persistent=true
[Install]
WantedBy=timers.target
UNIT
systemctl daemon-reload
systemctl enable --now coze-backup.timer
systemctl start coze-backup.service
systemctl list-timers 'coze-*' --no-pager
