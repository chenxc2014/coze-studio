set -eu
python3 - <<'PY'
import json,re,subprocess
from pathlib import Path
docker=Path('/home/coze/src/coze-studio/docker')
p=docker/'docker-compose.deploy.json'
data=json.loads(p.read_text())
data['services']['elasticsearch']['healthcheck'].update({'start_period':'120s','retries':30})
data['services']['mysql']['healthcheck'].update({'start_period':'60s',
    'test':['CMD-SHELL','MYSQL_PWD="$${MYSQL_PASSWORD}" mysql -h 127.0.0.1 -u "$${MYSQL_USER}" "$${MYSQL_DATABASE}" -e "SELECT 1" >/dev/null']})
lock={}
for service in data['services'].values():
    image=service['image']
    info=json.loads(subprocess.check_output(['docker','image','inspect',image]))[0]
    if not image.startswith('coze-local/'):
        digests=info['RepoDigests']
        assert digests, image
        service['image']=digests[0]
    service['pull_policy']='never'
    lock[image]={'id':info['Id'],'digest':service['image']}
p.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
Path('/home/coze/ops/image-lock.json').write_text(json.dumps(lock,indent=2)+'\n')
p=docker/'.env'
p.write_text(re.sub(r'^export LOG_LEVEL=.*$','export LOG_LEVEL="warn"',p.read_text(),flags=re.M))
PY
python3 /home/coze/ops/capacity.py --guard
sed -i 's|ExecStart=/home/coze/ops/compose.sh|ExecStart=/usr/bin/bash /home/coze/ops/compose.sh|; s|ExecStop=/home/coze/ops/compose.sh|ExecStop=/usr/bin/bash /home/coze/ops/compose.sh|' /etc/systemd/system/coze-studio.service
systemctl daemon-reload
systemctl enable --now coze-studio
/home/coze/ops/compose.sh ps
