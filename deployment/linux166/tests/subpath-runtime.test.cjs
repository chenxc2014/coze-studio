// 验证路径映射及原请求内容、凭据、信号和第三方地址的保留行为。
const {test} = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const path = require('node:path');
const source = fs.readFileSync(path.resolve(__dirname,'../../../backend/conf/admin/app-base-path.js'),'utf8');

function create(href='https://ai.yaoxiaoyi.com/agent/sign') {
  const calls=[];
  const ctx={URL,Request,Object,Element:class{},location:new URL(href),
    document:{addEventListener:(...args)=>calls.push(['listener',...args])},
    history:{pushState:(...args)=>calls.push(['push',...args]),replaceState:(...args)=>calls.push(['replace',...args])},
    fetch:(...args)=>{calls.push(['fetch',...args]);return Promise.resolve();},
    XMLHttpRequest:class {open(...args){calls.push(['xhr',...args]);}},
    EventSource:class {constructor(...args){calls.push(['sse',...args]);}},
    WebSocket:class {constructor(...args){calls.push(['ws',...args]);}},
    open:(...args)=>calls.push(['window',...args])};
  ctx.window=ctx;vm.createContext(ctx);vm.runInContext(source,ctx);
  return {ctx,calls};
}

test('同站路径加前缀，保留参数和第三方地址',()=>{
  const {ctx}=create();
  for (const [input,expected] of [
    ['/api/chat?q=%2F&a=1#t','https://ai.yaoxiaoyi.com/agent/api/chat?q=%2F&a=1#t'],
    ['/agent/api/chat','/agent/api/chat'],['/agent','/agent'],
    ['http://ai.yaoxiaoyi.com/local_storage/a','https://ai.yaoxiaoyi.com/agent/local_storage/a'],
    ['https://other.example/api/chat','https://other.example/api/chat'],
    ['//other.example/api','//other.example/api'],['blob:x','blob:x'],['about:blank','about:blank'],['',''],
  ]) assert.equal(ctx.CozePath.url(input),expected);
});
test('fetch 的 Request、请求体、认证头、取消信号和覆盖参数保留',async()=>{
  const {ctx,calls}=create();const controller=new AbortController();
  const request=new Request('https://ai.yaoxiaoyi.com/api/test',{method:'POST',headers:{Authorization:'Bearer unit-test-only'},body:'payload',credentials:'include',signal:controller.signal});
  const init={cache:'no-store'};await ctx.fetch(request,init);
  const [,mapped,options]=calls.find(item=>item[0]==='fetch');
  assert.equal(mapped.url,'https://ai.yaoxiaoyi.com/agent/api/test');
  assert.equal(mapped.method,'POST');assert.equal(await mapped.text(),'payload');
  assert.equal(mapped.headers.get('Authorization'),'Bearer unit-test-only');
  assert.equal(mapped.credentials,'include');assert.equal(options,init);
  controller.abort();assert.equal(mapped.signal.aborted,true);
});
test('XHR、SSE、WebSocket 使用前缀并保留选项',()=>{
  const {ctx,calls}=create();
  new ctx.XMLHttpRequest().open('POST','/api/test',false);
  new ctx.EventSource('/v3/chat',{withCredentials:true});
  new ctx.WebSocket('wss://ai.yaoxiaoyi.com/api/ws',['test']);
  assert.equal(calls.find(item=>item[0]==='xhr')[2],'https://ai.yaoxiaoyi.com/agent/api/test');
  assert.equal(calls.find(item=>item[0]==='xhr')[3],false);
  assert.equal(calls.find(item=>item[0]==='sse')[1],'https://ai.yaoxiaoyi.com/agent/v3/chat');
  assert.equal(calls.find(item=>item[0]==='sse')[2].withCredentials,true);
  assert.equal(calls.find(item=>item[0]==='ws')[1],'wss://ai.yaoxiaoyi.com/agent/api/ws');
});
test('页面历史与新窗口不重复添加前缀',()=>{
  const {ctx,calls}=create();const state={key:1};
  ctx.history.pushState(state,'','/space/1');ctx.history.replaceState(state,'','/agent/space/2');ctx.open('/space/3','_blank');
  assert.equal(calls.find(item=>item[0]==='push')[1],state);
  assert.equal(calls.find(item=>item[0]==='push')[3],'https://ai.yaoxiaoyi.com/agent/space/1');
  assert.equal(calls.find(item=>item[0]==='replace')[3],'/agent/space/2');
  assert.equal(calls.find(item=>item[0]==='window')[1],'https://ai.yaoxiaoyi.com/agent/space/3');
});
test('内网根路径和相似前缀保持原行为',()=>{
  for (const href of ['http://192.168.1.166:8888/sign','https://ai.yaoxiaoyi.com/agent-other/']) {
    const {ctx,calls}=create(href);assert.equal(ctx.CozePath.basePath,'');
    ctx.fetch('/api/test');assert.equal(calls.find(item=>item[0]==='fetch')[1],'/api/test');
  }
});
