"""工作进程启动时预热现有模型，避免首个业务请求承担模型加载。"""
import json
import math
import os
import time
import urllib.error
import urllib.request

address = os.environ['OLLAMA_HOST']
if address not in ('172.18.0.1:11439', '172.18.0.1:11440'):
    raise SystemExit('工作进程监听地址与部署配置不一致')
opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
for attempt in range(30):
    try:
        with opener.open('http://' + address + '/api/version', timeout=2):
            break
    except urllib.error.URLError:
        time.sleep(1)
else:
    raise SystemExit('工作进程启动超时')
request = urllib.request.Request(
    'http://' + address + '/api/embed',
    json.dumps({'model': 'bge-m3-cpu', 'input': ['向量工作进程启动检查']}).encode(),
    headers={'Content-Type': 'application/json'},
)
with opener.open(request, timeout=120) as response:
    vectors = json.load(response)['embeddings']
if len(vectors) != 1 or len(vectors[0]) != 1024 or not all(math.isfinite(v) for v in vectors[0]):
    raise SystemExit('工作进程模型预热校验失败')
print('工作进程模型预热完成，维度为 1024。')
