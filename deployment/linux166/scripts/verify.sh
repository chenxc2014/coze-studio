set -eu
printf '运行状态\n'
/home/coze/ops/compose.sh ps
printf '实际数据路径\n'
docker info --format 'DockerRootDir={{.DockerRootDir}} LoggingDriver={{.LoggingDriver}}'
containerd config dump | head -n 8
docker inspect $(/home/coze/ops/compose.sh ps -q) --format '{{.Name}} {{range .Mounts}}{{.Source}} => {{.Destination}}; {{end}} 日志={{.HostConfig.LogConfig.Type}} {{json .HostConfig.LogConfig.Config}}'
python3 - <<'PY'
import json,subprocess
ids=subprocess.check_output(['/home/coze/ops/compose.sh','ps','-aq']).decode().split()
items=json.loads(subprocess.check_output(['docker','inspect',*ids]))
assert len(items)==11, f'容器数量异常：{len(items)}'
for item in items:
    state=item['State']
    assert state['Running'], item['Name']+' 未运行'
    if 'Health' in state:
        assert state['Health']['Status']=='healthy', item['Name']+' 未健康'
    for mount in item['Mounts']:
        assert mount['Source'].startswith('/home/coze/'), mount['Source']
print('十一项服务均在运行；已配置健康检查的组件全部通过；所有挂载位于 /home/coze。')
import secrets
key='localminio/opencoze/deployment-check/'+secrets.token_hex(12)+'.txt'
content=b'Coze Studio storage verification\n'
subprocess.run(['docker','exec','-i','coze-minio','/usr/bin/mc','pipe',key],input=content,stdout=subprocess.DEVNULL,check=True)
try:
    assert subprocess.check_output(['docker','exec','coze-minio','/usr/bin/mc','cat',key])==content
finally:
    subprocess.run(['docker','exec','coze-minio','/usr/bin/mc','rm',key],stdout=subprocess.DEVNULL,check=True)
print('对象存储文件写入、读取及测试对象清理通过。')
PY
python3 /home/coze/ops/capacity.py
printf '应用 HTTP 检查\n'
python3 - <<'PY'
import json, urllib.request, urllib.error
for path in ['/sign','/admin/','/api/admin/config/basic/get','/v1/conversation/create']:
    request=urllib.request.Request('http://192.168.1.166:8888'+path)
    if path=='/v1/conversation/create':
        request=urllib.request.Request(request.full_url,data=b'{}',headers={'Content-Type':'application/json'})
    try:
        with urllib.request.urlopen(request, timeout=15) as r:
            if path=='/v1/conversation/create':
                body=json.load(r)
                assert body.get('code')==700012006, body
                print(path,r.status,'业务鉴权拒绝，code='+str(body['code']))
                continue
            assert path in ['/sign','/admin/'] and r.status==200, (path,r.status)
            print(path, r.status, r.headers.get('Content-Type'))
    except urllib.error.HTTPError as e:
        assert path in ['/api/admin/config/basic/get','/v1/conversation/create'] and e.code in [401,403], (path,e.code)
        print(path,e.code,'未认证请求已拒绝')
PY
printf '容量巡检定时器\n'
systemctl list-timers coze-capacity.timer --no-pager
printf '存储占用\n'
du -sh /home/coze/data/* /home/coze/runtime/* /home/coze/src /home/coze/backups
docker system df
