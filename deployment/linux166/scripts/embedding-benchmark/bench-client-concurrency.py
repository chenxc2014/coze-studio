"""补测真实客户端并发，只控制隔离测试实例。"""
import json
import math
from pathlib import Path
import subprocess
import time
from concurrent.futures import ThreadPoolExecutor
import urllib.request

ROOT=Path('/home/coze/build/embedding-bench')
URL='http://127.0.0.1:11435'
TEXTS=[x['text'] for x in json.loads((ROOT/'sample.json').read_text())['records'][:64]]
def post(path,data):
    opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
    req=urllib.request.Request(URL+path,json.dumps(data,ensure_ascii=False).encode(),headers={'Content-Type':'application/json'})
    return json.load(opener.open(req,timeout=180))

def main():
    override=Path('/run/systemd/system/coze-embedding-bench.service.d/parameters.conf')
    override.parent.mkdir(exist_ok=True)
    override.write_text('[Service]\nEnvironment=OLLAMA_NUM_PARALLEL=2\nCPUQuota=800%\n')
    try:
        subprocess.run(['systemctl','daemon-reload'],check=True)
        subprocess.run(['systemctl','start','coze-embedding-bench'],check=True)
        for _ in range(30):
            try:post('/api/create',{'model':'bench-m3','from':'bge-m3:latest','parameters':{'num_thread':8},'stream':False});break
            except (OSError,urllib.error.URLError):time.sleep(1)
        else:raise RuntimeError('隔离服务未就绪')
        post('/api/embed',{'model':'bench-m3','input':TEXTS[-2:],'truncate':False})
        output=[];reference=None
        for workers in [1,2]:
            started=time.monotonic()
            def encode(chunk):return post('/api/embed',{'model':'bench-m3','input':chunk,'truncate':False})['embeddings']
            with ThreadPoolExecutor(max_workers=workers) as pool:
                chunks=list(pool.map(encode,[TEXTS[i:i+8] for i in range(0,64,8)]))
            vectors=[v for chunk in chunks for v in chunk]
            assert len(vectors)==64 and all(len(v)==1024 and all(math.isfinite(x) for x in v) for v in vectors)
            elapsed=time.monotonic()-started
            item={'client_workers':workers,'requested_model_parallel':2,'effective_model_parallel':1,'threads':8,'cpu_limit':8,'batch':8,'rows':64,'seconds':round(elapsed,3),'rows_per_minute':round(64*60/elapsed,2)}
            if reference is None:reference=vectors
            else:item['max_absolute_component_difference']=max(abs(x-y) for a,b in zip(reference,vectors) for x,y in zip(a,b))
            output.append(item);print(json.dumps(item),flush=True)
        report={'runs':output,'two_client_speedup':round(output[0]['seconds']/output[1]['seconds'],4),'shared_production_load':True,'single_round_only':True,'scope':'验证同一隔离模型的真实客户端并发请求；引擎仅 1 个实际 slot，不是两个模型副本。'}
        (ROOT/'results/client-concurrency.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    finally:
        subprocess.run(['systemctl','stop','coze-embedding-bench'],check=True)
        if override.exists():override.unlink()
        subprocess.run(['systemctl','daemon-reload'],check=True)

if __name__=='__main__':main()
