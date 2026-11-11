"""在原私有候选集上补测地址查询，不修改生产 Embedding 或索引。"""
import importlib.util
import json
import math
from pathlib import Path
import random
import re
import subprocess
import time
import unicodedata
import urllib.request
import numpy as np

ROOT=Path('/home/coze/build/embedding-bench')
RESULTS=ROOT/'results'
DATA=json.loads((ROOT/'sample.json').read_text())
RECORDS=DATA['records']

def norm(text):
    return re.sub(r'[\s，,。；;：:、·（）()\-—]+','',unicodedata.normalize('NFKC',str(text or ''))).lower()

def field(row,key):return str(row['fields'].get(key) or '').strip()

def short_address(row):
    address=field(row,'地址')
    for key in ['省','市','区县']:
        prefix=field(row,key)
        if prefix and address.startswith(prefix):address=address[len(prefix):]
    return address

def admin(row):return ''.join(field(row,k) for k in ['省','市','区县'])

def same_region(a,b):return all(norm(field(a,k))==norm(field(b,k)) for k in ['市','区县'])

def prepare():
    rng=random.Random(20261112)
    eligible=[r for r in RECORDS if len(norm(field(r,'地址')))>=6 and field(r,'区县') and re.search(r'路|街|巷|弄|大道|镇|乡|村|\d+号',field(r,'地址'))]
    rng.shuffle(eligible)
    full=[];seen=set()
    for row in eligible:
        key=(norm(field(row,'市')),norm(field(row,'区县')),norm(field(row,'地址')))
        if key in seen:continue
        seen.add(key);full.append(row)
        if len(full)==20:break
    assert len(full)==20,'有效完整地址不足 20 个'
    queries=[]
    for i,row in enumerate(full):
        address=field(row,'地址');short=short_address(row)
        relevant=[j for j,r in enumerate(RECORDS) if same_region(row,r) and norm(field(r,'地址'))==norm(address)]
        queries.append({'id':'full-'+str(i),'kind':'full_address','text':'查一下'+admin(row)+short+'这个地址有哪些医疗机构。','relevant':relevant})
        # 省略省和区县，但保留城市，所有同市匹配地址的记录都作为相关项。
        relevant=[j for j,r in enumerate(RECORDS) if norm(field(row,'市'))==norm(field(r,'市')) and norm(short) in norm(field(r,'地址'))]
        queries.append({'id':'spoken-'+str(i),'kind':'spoken_address','text':'我想找'+field(row,'市')+short+'这边的医疗机构，有哪些？','relevant':relevant})
    streets=[];seen=set()
    for row in eligible:
        match=re.search(r'[\u4e00-\u9fff0-9]{2,24}?(?:大道|街道|胡同|大街|公路|路|街|巷|弄|镇|乡|村)',short_address(row))
        if not match:continue
        term=match.group();key=(field(row,'市'),field(row,'区县'),norm(term))
        if key in seen:continue
        seen.add(key);streets.append((row,term))
        if len(streets)==20:break
    assert len(streets)==20,'有效街道路段不足 20 个'
    for i,(row,term) in enumerate(streets):
        relevant=[j for j,r in enumerate(RECORDS) if same_region(row,r) and norm(term) in norm(field(r,'地址'))]
        queries.append({'id':'street-'+str(i),'kind':'street','text':'请列出'+admin(row)+term+'的医疗机构。','relevant':relevant})
    groups={}
    for j,row in enumerate(RECORDS):
        city=field(row,'市');district=field(row,'区县')
        if district and district not in ['无','未知','市辖区','其他']:groups.setdefault((city,district),[]).append(j)
    groups=[(key,ids) for key,ids in groups.items() if len(ids)>=2]
    rng.shuffle(groups)
    assert len(groups)>=20,'具有多条候选的区县不足 20 个'
    for i,((city,district),ids) in enumerate(groups[:20]):
        queries.append({'id':'district-'+str(i),'kind':'district','text':'帮我查一下'+city+district+'范围内的医疗机构有哪些。','relevant':ids})
    assert len(queries)==80 and all(q['relevant'] for q in queries)
    (ROOT/'address-queries.json').write_text(json.dumps(queries,ensure_ascii=False,indent=2)+'\n')
    (ROOT/'address-queries.json').chmod(0o600)
    protocol={'recorded_at':time.strftime('%Y-%m-%d %H:%M:%S %z'),'business_intent':'查指定地址、街道或区县内的医疗机构，不是地理距离搜索','seed':20261112,'candidate_records':500,'queries':80,'query_kinds':{'full_address':20,'spoken_address':20,'street':20,'district':20},'labels':'完整地址按地址与行政字段匹配；口语地址按同市地址子串匹配；街道按同市同区县地址子串匹配；区县按行政字段匹配','human_labeled_queries':False,'primary_query_instruction':False,'ranking':'纯向量精确余弦排序，不经过生产 Milvus 或混合检索','gates':{'minimum_all_hit_at_5_delta':-.05,'minimum_full_address_hit_at_1_delta':-.05,'minimum_spoken_address_hit_at_5_delta':-.05,'minimum_street_precision_at_5_delta':-.05,'minimum_district_precision_at_5_delta':-.05},'decision_scope':'仅作为地址任务上的下一阶段筛查，不允许直接切换生产'}
    (RESULTS/'address-protocol.json').write_text(json.dumps(protocol,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps({'prepared_queries':len(queries),'valid_address_records':len(eligible),'region_groups':len(groups)},ensure_ascii=False),flush=True)
    return queries,protocol

def metrics(corpus,qvectors,queries):
    corpus=corpus/np.maximum(np.linalg.norm(corpus,axis=1,keepdims=True),1e-12)
    qvectors=qvectors/np.maximum(np.linalg.norm(qvectors,axis=1,keepdims=True),1e-12)
    order=np.argsort(-(qvectors@corpus.T),axis=1)
    details=[]
    for q,ranked in zip(queries,order):
        relevant=set(q['relevant']);ranked=ranked.tolist()
        first=next((i+1 for i,j in enumerate(ranked) if j in relevant),501)
        hits=[int(j in relevant) for j in ranked[:10]]
        ideal=sum(1/math.log2(i+2) for i in range(min(10,len(relevant))))
        details.append({'id':q['id'],'kind':q['kind'],'relevant_records':len(relevant),'hit_at_1':float(first<=1),'hit_at_5':float(first<=5),'hit_at_10':float(first<=10),'mrr_at_10':1/first if first<=10 else 0,'precision_at_5':sum(hits[:5])/5,'precision_at_10':sum(hits)/10,'recall_at_10':sum(hits)/len(relevant),'ndcg_at_10':sum(hit/math.log2(i+2) for i,hit in enumerate(hits))/ideal})
    output={}
    keys=['hit_at_1','hit_at_5','hit_at_10','mrr_at_10','precision_at_5','precision_at_10','recall_at_10','ndcg_at_10']
    for kind in ['all','full_address','spoken_address','street','district']:
        rows=[r for r in details if kind=='all' or r['kind']==kind]
        output[kind]={'queries':len(rows),'relevant_records_min':min(r['relevant_records'] for r in rows),'relevant_records_max':max(r['relevant_records'] for r in rows),**{key:round(float(np.mean([r[key] for r in rows])),4) for key in keys}}
        output[kind]['max_possible_precision_at_5']=round(sum(min(5,r['relevant_records'])/5 for r in rows)/len(rows),4)
        output[kind]['max_possible_precision_at_10']=round(sum(min(10,r['relevant_records'])/10 for r in rows)/len(rows),4)
    return output,details

def main():
    queries,protocol=prepare();texts=[q['text'] for q in queries]
    override=Path('/run/systemd/system/coze-embedding-bench.service.d/parameters.conf')
    override.parent.mkdir(exist_ok=True)
    override.write_text('[Service]\nEnvironment=OLLAMA_NUM_PARALLEL=1\nCPUQuota=800%\n')
    opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
    def post(path,data):
        req=urllib.request.Request('http://127.0.0.1:11435'+path,json.dumps(data,ensure_ascii=False).encode(),headers={'Content-Type':'application/json'})
        return json.load(opener.open(req,timeout=180))
    try:
        subprocess.run(['systemctl','daemon-reload'],check=True)
        subprocess.run(['systemctl','start','coze-embedding-bench'],check=True)
        for _ in range(30):
            try:post('/api/create',{'model':'bench-m3','from':'bge-m3:latest','parameters':{'num_thread':8},'stream':False});break
            except (OSError,urllib.error.URLError):time.sleep(1)
        else:raise RuntimeError('隔离服务未就绪')
        post('/api/embed',{'model':'bench-m3','input':[texts[0]],'truncate':False})
        vectors=[]
        for i in range(0,len(texts),8):vectors.extend(post('/api/embed',{'model':'bench-m3','input':texts[i:i+8],'truncate':False})['embeddings'])
    finally:
        subprocess.run(['systemctl','stop','coze-embedding-bench'],check=True)
        if override.exists():override.unlink()
        subprocess.run(['systemctl','daemon-reload'],check=True)
    spec=importlib.util.spec_from_file_location('small',str(ROOT/'bench-small-server.py'));small=importlib.util.module_from_spec(spec);spec.loader.exec_module(small)
    encoder=small.Encoder(ROOT/'small-model',threads=8)
    smallvectors=np.concatenate([encoder.encode(texts[i:i+16]) for i in range(0,len(texts),16)])
    raw=small.Tokenizer.from_file(str(ROOT/'small-model/tokenizer.json'));raw.no_truncation();raw.no_padding()
    lengths=[len(x.ids) for x in raw.encode_batch(texts)]
    m3,md=metrics(np.load(RESULTS/'m3-corpus.npy'),np.asarray(vectors,dtype=np.float32),queries)
    tiny,td=metrics(np.load(RESULTS/'small-corpus.npy'),smallvectors,queries)
    checks=[('all','hit_at_5'),('full_address','hit_at_1'),('spoken_address','hit_at_5'),('street','precision_at_5'),('district','precision_at_5')]
    gates=[{'kind':kind,'metric':metric,'delta':round(tiny[kind][metric]-m3[kind][metric],4),'minimum':-.05,'passed':tiny[kind][metric]-m3[kind][metric]>=-.05000001} for kind,metric in checks]
    summary={'finished_at':time.strftime('%Y-%m-%d %H:%M:%S %z'),'validation_finished':True,'production_model_switched':False,'protocol':protocol,'quality':{'m3':m3,'small':tiny},'query_tokens_max':max(lengths),'query_tokens_over_512':sum(n>512 for n in lengths),'gate_results':gates,'all_gates_passed':all(g['passed'] for g in gates),'label_limitations':['用户确认了问法类别，问题文本与相关项仍由字段和模板构造，没有人工逐题标注。','完整地址和口语地址存在配对，80 条问题不是 80 个相互独立的用户查询。','只在既有 500 条候选中评价，原语料向量复用，不代表全量两万条检索质量。','区县和街道题有多个相关项，Precision 与 NDCG 比仅看 Hit 更有解释力。']}
    (RESULTS/'address-details-private.json').write_text(json.dumps({'m3':md,'small':td},indent=2)+'\n')
    (RESULTS/'address-summary.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps(summary,ensure_ascii=False),flush=True)

if __name__=='__main__':main()
