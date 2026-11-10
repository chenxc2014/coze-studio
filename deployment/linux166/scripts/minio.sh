set -eu
cd /home/coze/src/minio-runtime
docker image inspect debian:bookworm-slim >/dev/null 2>&1 || docker pull debian:bookworm-slim
base=$(docker image inspect debian:bookworm-slim --format '{{index .RepoDigests 0}}')
printf 'FROM %s\nCOPY minio mc /usr/bin/\nEXPOSE 9000 9001\nCMD ["/usr/bin/minio", "server", "/data", "--console-address", ":9001"]\n' "$base" > Dockerfile
HTTP_PROXY=http://127.0.0.1:18080 HTTPS_PROXY=http://127.0.0.1:18080 NO_PROXY=localhost,127.0.0.1 docker build --network=none --pull=false -t coze-local/minio:2025-06-13 .
docker run --rm --network none coze-local/minio:2025-06-13 /usr/bin/minio --version
docker run --rm --network none coze-local/minio:2025-06-13 /usr/bin/mc --version
python3 - <<'PY'
from pathlib import Path
import json
p=Path('/home/coze/src/coze-studio/docker/docker-compose.deploy.json')
data=json.loads(p.read_text())
data['services']['minio']['image']='coze-local/minio:2025-06-13'
data['services']['minio']['pull_policy']='never'
p.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
PY
/home/coze/ops/compose.sh config --quiet
