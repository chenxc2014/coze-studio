"""在共享负载下完成同样本速度与召回对比，不连接生产向量索引。"""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import threading
import time
import urllib.request
import urllib.error
import numpy as np

ROOT=Path('/home/coze/build/embedding-bench')
RESULTS=ROOT/'results'
BASE='http://127.0.0.1:11435'
OPENER=urllib.request.build_opener(urllib.request.ProxyHandler({}))
DATA=json.loads((ROOT/'sample.json').read_text())
TEXTS=[x['text'] for x in DATA['records']]
QUERIES=[x['text'] for x in DATA['queries']]
REPORT={'started_at':time.strftime('%Y-%m-%d %H:%M:%S %z'),'sample':DATA['summary'],'load_condition':'同一台服务器上的生产 BGE-M3 导入持续运行；测试单元 CPUWeight=10、Nice=10，速度是共享负载结果。12 核请求上限可能无法获得 12 个实际核心。','screens':[],'full_runs':[],'quality':{},'production_model_switched':False}

def save():
    path=RESULTS/'summary.json';temp=path.with_suffix('.tmp')
    temp.write_text(json.dumps(REPORT,ensure_ascii=False,indent=2)+'\n');temp.replace(path)

def get(path):return json.load(OPENER.open(BASE+path,timeout=10))

def post(url,body):
    req=urllib.request.Request(url,json.dumps(body,ensure_ascii=False).encode(),headers={'Content-Type':'application/json'})
    return json.load(OPENER.open(req,timeout=180))

def cpu_snapshot(unit):
    p=Path('/sys/fs/cgroup/system.slice')/unit/'cpu.stat'
    return {k:int(v) for k,v in (line.split() for line in p.read_text().splitlines())} if p.exists() else {}

def configure(cfg):
    directory=Path('/run/systemd/system/coze-embedding-bench.service.d');directory.mkdir(exist_ok=True)
    (directory/'parameters.conf').write_text('[Service]\nEnvironment=OLLAMA_NUM_PARALLEL='+str(cfg['parallel'])+'\nCPUQuota='+str(cfg['cores']*100)+'%\n')
    subprocess.run(['systemctl','daemon-reload'],check=True)
    subprocess.run(['systemctl','restart','coze-embedding-bench'],check=True)
    for attempt in range(30):
        try:get('/api/version');break
        except Exception:time.sleep(1)
    result=post(BASE+'/api/create',{'model':'bench-m3','from':'bge-m3:latest','parameters':{'num_thread':cfg['threads']},'stream':False})
    assert result.get('status')=='success'

def encode_m3(texts,batch):
    chunks=[]
    for i in range(0,len(texts),batch):
        data=post(BASE+'/api/embed',{'model':'bench-m3','input':texts[i:i+batch],'truncate':False})
        chunks.extend(data['embeddings'])
        if len(texts)>=500 and (i//128!=(i+batch)//128 or i+batch>=len(texts)):print('BGE-M3 已编码 '+str(min(i+batch,len(texts)))+'/'+str(len(texts)),flush=True)
    result=np.asarray(chunks,dtype=np.float32)
    assert result.shape==(len(texts),1024) and np.isfinite(result).all()
    return result/np.maximum(np.linalg.norm(result,axis=1,keepdims=True),1e-12)

def run_m3(cfg,texts,label):
    configure(cfg);encode_m3(TEXTS[-2:],2)
    a=cpu_snapshot('coze-embedding-bench.service');started=time.monotonic()
    vectors=encode_m3(texts,cfg['batch']);elapsed=time.monotonic()-started;b=cpu_snapshot('coze-embedding-bench.service')
    item={'label':label,**cfg,'rows':len(texts),'seconds':round(elapsed,3),'rows_per_minute':round(len(texts)*60/elapsed,2),'actual_average_cpu_cores':round((b.get('usage_usec',0)-a.get('usage_usec',0))/1e6/elapsed,2)}
    print(json.dumps(item,ensure_ascii=False),flush=True)
    return item,vectors

def quality(corpus,queries):
    corpus=corpus/np.maximum(np.linalg.norm(corpus,axis=1,keepdims=True),1e-12)
    queries=queries/np.maximum(np.linalg.norm(queries,axis=1,keepdims=True),1e-12)
    ranking=np.argsort(-(queries@corpus.T),axis=1)
    details=[]
    for i,q in enumerate(DATA['queries']):
        relevant=set(q['relevant']);order=ranking[i].tolist()
        rank=next((j+1 for j,x in enumerate(order) if x in relevant),501)
        details.append({'id':q['id'],'kind':q['kind'],'hit_at_1':float(rank<=1),'hit_at_5':float(rank<=5),'hit_at_10':float(rank<=10),'mrr_at_10':1/rank if rank<=10 else 0,'precision_at_5':len(set(order[:5])&relevant)/5,'recall_at_10':len(set(order[:10])&relevant)/len(relevant)})
    result={}
    for kind in ['all','entity','paraphrase','condition']:
        selected=[x for x in details if kind=='all' or x['kind']==kind]
        if selected:result[kind]={'queries':len(selected),**{key:round(float(np.mean([x[key] for x in selected])),4) for key in details[0] if key not in ['id','kind']}}
    return result,details

def main():
    RESULTS.mkdir(exist_ok=True)
    baseline={'cores':8,'threads':8,'batch':8,'parallel':1}
    configs=[baseline,{'cores':12,'threads':12,'batch':16,'parallel':1},{'cores':8,'threads':8,'batch':16,'parallel':2},{'cores':8,'threads':8,'batch':32,'parallel':2}]
    for i,cfg in enumerate(configs):
        try:
            item,_=run_m3(cfg,TEXTS[:64],'screen-'+str(i));REPORT['screens'].append(item)
        except Exception as e:
            REPORT['screens'].append({'label':'screen-'+str(i),**cfg,'failed':True,'error_type':type(e).__name__});print('参数预筛未通过：'+str(i)+' '+type(e).__name__,flush=True)
        save()
    good=[x for x in REPORT['screens'] if not x.get('failed')]
    assert good,'没有成功的 BGE-M3 预筛配置'
    best=max(good,key=lambda x:x['rows_per_minute']);winner={key:best[key] for key in baseline}
    item,vectors=run_m3(baseline,TEXTS,'m3-baseline-500');REPORT['full_runs'].append(item)
    np.save(RESULTS/'m3-corpus.npy',vectors)
    qvectors=encode_m3(QUERIES,8)
    REPORT['quality']['m3'],details=quality(vectors,qvectors)
    (RESULTS/'m3-query-scores.json').write_text(json.dumps(details,indent=2)+'\n');save()
    if winner!=baseline:
        item,tuned=run_m3(winner,TEXTS,'m3-tuned-500');REPORT['full_runs'].append(item)
        REPORT['tuned_vector_cosine_mean']=round(float(np.mean(np.sum(tuned*vectors,axis=1))),6)
        save()
    else:REPORT['tuning_note']='预筛最优仍为当前配置，复用 500 行结果，不重复计算。';save()
    subprocess.run(['systemctl','stop','coze-embedding-bench'],check=True)
    spec=importlib.util.spec_from_file_location('small',str(ROOT/'bench-small-server.py'));small=importlib.util.module_from_spec(spec);spec.loader.exec_module(small)
    encoder=small.Encoder(ROOT/'small-model',threads=8)
    raw=small.Tokenizer.from_file(str(ROOT/'small-model/tokenizer.json'));raw.no_truncation();raw.no_padding()
    lengths=[len(x.ids) for x in raw.encode_batch(TEXTS+QUERIES)]
    REPORT['small_token_lengths']={'max':max(lengths),'mean':round(float(np.mean(lengths)),2),'over_512':sum(x>512 for x in lengths)}
    http=small.server(encoder);threading.Thread(target=http.serve_forever,daemon=True).start()
    assert OPENER.open('http://127.0.0.1:11436/support_status',timeout=5).read()==b'1'
    def encode_small(texts,batch):
        result=[]
        for i in range(0,len(texts),batch):result.extend(post('http://127.0.0.1:11436/embedding',{'texts':texts[i:i+batch],'need_sparse':False})['dense'])
        return np.asarray(result,dtype=np.float32)
    encode_small(TEXTS[-2:],2)
    small_results=[]
    for batch in [8,16,32]:
        started=time.monotonic();out=encode_small(TEXTS,batch);elapsed=time.monotonic()-started
        item={'label':'small-fp32-500','cores':8,'threads':8,'batch':batch,'parallel':1,'rows':500,'seconds':round(elapsed,3),'rows_per_minute':round(500*60/elapsed,2),'dimensions':512}
        REPORT['full_runs'].append(item);small_results.append(item);np.save(RESULTS/'small-corpus.npy',out);save();print(json.dumps(item),flush=True)
    queries=encode_small(QUERIES,16)
    REPORT['quality']['small'],details=quality(out,queries)
    (RESULTS/'small-query-scores.json').write_text(json.dumps(details,indent=2)+'\n')
    # 查询提示词仅作为附加观察；主比较保持现有 Coze 接口语义，不自动增加提示词。
    prompted=encode_small(['为这个句子生成表示以用于检索相关文章：'+q for q in QUERIES],16)
    REPORT['quality']['small_with_query_instruction'],_=quality(out,prompted)
    http.shutdown();http.server_close()
    REPORT['finished_at']=time.strftime('%Y-%m-%d %H:%M:%S %z');REPORT['validation_finished']=True
    REPORT['limitations']=['测试与生产导入共享服务器，不能把结果当作独占硬件基准。','质量评测在 500 条候选记录上进行，问题由记录和模板构造，并非人工标注的真实查询日志。','主质量比较未额外加入查询提示词，符合当前 Coze 通用向量接口行为。','不同模型不能复用现有索引；本次没有切换生产模型。']
    save();print('同样本性能与召回验证完成。',flush=True)

if __name__=='__main__':
    try:main()
    except Exception as e:
        REPORT['validation_finished']=False;REPORT['failure_type']=type(e).__name__;save();raise
