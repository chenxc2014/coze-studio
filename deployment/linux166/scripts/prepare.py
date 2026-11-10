"""生成服务器专用配置，所有持久数据均落入 /home 分区。"""
import json
import re
import secrets
from pathlib import Path

base = Path('/home/coze')
repo = base / 'src/coze-studio'
docker = repo / 'docker'
env_path = docker / '.env'
if not env_path.exists():
    text = (docker / '.env.example').read_text()
    values = {
        'MYSQL_ROOT_PASSWORD': secrets.token_hex(20),
        'MYSQL_PASSWORD': secrets.token_hex(20),
        'MINIO_ROOT_PASSWORD': secrets.token_hex(20),
        'PLUGIN_AES_AUTH_SECRET': secrets.token_hex(16),
        'PLUGIN_AES_STATE_SECRET': secrets.token_hex(16),
        'PLUGIN_AES_OAUTH_TOKEN_SECRET': secrets.token_hex(16),
        'LOG_LEVEL': 'info',
        'WEB_LISTEN_ADDR': '127.0.0.1:8888',
        'MAX_REQUEST_BODY_SIZE': '52428800',
        'ES_NUMBER_OF_REPLICAS': '0',
        'CODE_RUNNER_TYPE': 'sandbox',
    }
    for key, value in values.items():
        text, count = re.subn(r'^export '+key+r'=.*$', f'export {key}="{value}"', text, flags=re.M)
        assert count == 1, key
    env_path.write_text(text)
    env_path.chmod(0o600)

config = json.loads((base / 'tmp/compose-base.json').read_text())
limits = {'mysql':'1536m','redis':'512m','elasticsearch':'2g','minio':'1g',
          'etcd':'512m','milvus':'4g','coze-server':'2g','coze-web':'256m',
          'nsqd':'256m','nsqlookupd':'128m','nsqadmin':'128m'}
for name, service in config['services'].items():
    service['restart'] = 'unless-stopped'
    service['logging'] = {'driver':'local','options':{'max-size':'20m','max-file':'3'}}
    service['mem_limit'] = limits[name]
    volumes = []
    for volume in service.get('volumes', []):
        parts = volume.split(':')
        source = parts[0]
        if source.startswith('./data/'):
            source = str(base / 'data' / source.removeprefix('./data/'))
            Path(source).mkdir(parents=True, exist_ok=True)
        else:
            source = str((docker / source).resolve())
        options = parts[2].split(',') if len(parts) > 2 else []
        options = [item for item in options if item not in ('z','Z')]
        options.append('z')
        volumes.append(':'.join([source, parts[1], ','.join(options)]))
    service['volumes'] = volumes
    if name in ['coze-server','coze-web']:
        service['image'] = f'cozedev/coze-studio-{name.removeprefix("coze-")}:0.5.1'

# 队列数据显式持久化，避免只存在容器可写层。
queue = base / 'data/nsq'
queue.mkdir(parents=True, exist_ok=True)
config['services']['nsqd']['volumes'].append(str(queue)+':/data:rw,z')
config['services']['nsqd']['command'] += ' --data-path=/data --max-bytes-per-file=104857600'
mysql = config['services']['mysql']
mysql['entrypoint'][-1] = mysql['entrypoint'][-1].replace(
    'mysqld --character-set-server',
    'mysqld --binlog-expire-logs-seconds=259200 --max-binlog-size=104857600 --character-set-server')
config['services']['elasticsearch']['environment'].append('ES_JAVA_OPTS=-Xms512m -Xmx1024m')
config['services']['elasticsearch']['healthcheck'].update({'start_period':'120s','retries':30})
config['services']['mysql']['healthcheck'].update({'start_period':'60s',
    'test':['CMD-SHELL','MYSQL_PWD="$${MYSQL_PASSWORD}" mysql -h 127.0.0.1 -u "$${MYSQL_USER}" "$${MYSQL_DATABASE}" -e "SELECT 1" >/dev/null']})
manifest = docker / 'docker-compose.deploy.json'
manifest.write_text(json.dumps(config, ensure_ascii=False, indent=2)+'\n')
manifest.chmod(0o600)
print('已生成固定版本、分区落盘及日志轮转配置；凭据未输出。')
