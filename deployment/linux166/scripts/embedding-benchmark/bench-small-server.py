"""中文小模型的独立 CPU 编码器与兼容 Coze 的测试接口。"""
import argparse
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
import json
from pathlib import Path
import threading
import time
import numpy as np
import onnxruntime as ort
from tokenizers import Tokenizer

ort.disable_telemetry_events()

class Encoder:
    def __init__(self,root,threads=8):
        root=Path(root)
        self.tokenizer=Tokenizer.from_file(str(root/'tokenizer.json'))
        self.tokenizer.enable_padding(pad_id=self.tokenizer.token_to_id('[PAD]'),pad_token='[PAD]')
        self.tokenizer.enable_truncation(max_length=512)
        options=ort.SessionOptions();options.intra_op_num_threads=threads;options.inter_op_num_threads=1
        options.execution_mode=ort.ExecutionMode.ORT_SEQUENTIAL
        options.graph_optimization_level=ort.GraphOptimizationLevel.ORT_ENABLE_ALL
        options.add_session_config_entry('session.intra_op.allow_spinning','0')
        self.session=ort.InferenceSession(str(root/'model.onnx'),sess_options=options,providers=['CPUExecutionProvider'])
        self.inputs={x.name for x in self.session.get_inputs()};self.lock=threading.Lock()

    def encode(self,texts):
        with self.lock:
            encoded=self.tokenizer.encode_batch(texts)
            ids=np.array([x.ids for x in encoded],dtype=np.int64)
            mask=np.array([x.attention_mask for x in encoded],dtype=np.int64)
            feed={'input_ids':ids,'attention_mask':mask,'token_type_ids':np.array([x.type_ids for x in encoded],dtype=np.int64)}
            output=self.session.run(None,{k:v for k,v in feed.items() if k in self.inputs})[0]
            vectors=output[:,0,:] if output.ndim==3 else output
            vectors=vectors.astype(np.float32)
            vectors/=np.maximum(np.linalg.norm(vectors,axis=1,keepdims=True),1e-12)
            if vectors.shape!=(len(texts),512) or not np.isfinite(vectors).all():raise ValueError('向量形状或数值不合法')
            return vectors

def server(encoder,port=11436):
    class Handler(BaseHTTPRequestHandler):
        def log_message(self,*args):pass
        def do_GET(self):
            if self.path!='/support_status':self.send_error(404);return
            body=b'1';self.send_response(200);self.send_header('Content-Length',str(len(body)));self.end_headers();self.wfile.write(body)
        def do_POST(self):
            if self.path!='/embedding':self.send_error(404);return
            length=int(self.headers.get('Content-Length','0'))
            if length<=0 or length>4*1024*1024:self.send_error(400);return
            try:
                data=json.loads(self.rfile.read(length));texts=data['texts']
                if not isinstance(texts,list) or not 1<=len(texts)<=256 or any(not isinstance(t,str) for t in texts):raise ValueError()
                started=time.monotonic();vectors=encoder.encode(texts)
                body=json.dumps({'dense':vectors.tolist(),'inference_seconds':time.monotonic()-started},separators=(',',':')).encode()
                self.send_response(200);self.send_header('Content-Type','application/json');self.send_header('Content-Length',str(len(body)));self.end_headers();self.wfile.write(body)
            except Exception:self.send_error(400,'Invalid embedding request')
    return ThreadingHTTPServer(('127.0.0.1',port),Handler)

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('--model-dir',required=True);p.add_argument('--threads',type=int,default=8);p.add_argument('--port',type=int,default=11436);args=p.parse_args()
    server(Encoder(args.model_dir,args.threads),args.port).serve_forever()
