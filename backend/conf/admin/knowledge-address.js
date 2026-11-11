/* 地址查询仅展示受当前账号授权的知识文档。 */
(() => {
  'use strict';
  const $=id=>document.getElementById(id), params=new URLSearchParams(location.search);
  const base=/^\/agent(?:\/|$)/.test(location.pathname)?'/agent':'';
  const endpoint=path=>base+path;
  let currentPage=1, busy=false, last=null;
  function message(text,error=false){$('message').textContent=text;$('message').classList.toggle('error',error);}
  function node(tag,text){const n=document.createElement(tag);n.textContent=text;return n;}
  async function api(path,body){
    const response=await fetch(endpoint(path),{method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
    const data=await response.json();
    if(response.status===401||data.code===700012006)throw new Error('登录已失效，请返回平台登录后重新打开。');
    if(!response.ok||data.code!==0)throw new Error(data.message||data.msg||'读取失败，请稍后重试');
    return data;
  }
  function controls(){ $('submit').disabled=busy;$('reset').disabled=busy;for(const n of $('search').querySelectorAll('input,select'))n.disabled=busy;$('prev').disabled=busy||!last||currentPage<=1;$('next').disabled=busy||!last||!last.has_more; }
  function render(data){
    last=data; currentPage=data.page;
    $('head').replaceChildren();$('rows').replaceChildren();
    const tr=document.createElement('tr');tr.append(node('th','序号'));
    for(const title of data.columns)tr.append(node('th',title));$('head').append(tr);
    data.rows.forEach((row,i)=>{const tr=document.createElement('tr');tr.append(node('td',String((data.page-1)*data.page_size+i+1)));for(const title of data.columns){const text=row.values[title]||'';const td=node('td',text);td.title=text;tr.append(td);}$('rows').append(tr);});
    $('summary').textContent='共匹配 '+data.total+' 条记录，本页 '+data.rows.length+' 条'+(data.count_only?'（计数查询）':'');
    $('applied').textContent=Object.values(data.filter).filter(Boolean).join(' / ');
    $('warning').hidden=data.import_complete;
    $('empty').hidden=data.rows.length!==0||data.count_only;
    $('page').textContent='第 '+data.page+' / '+Math.max(1,Math.ceil(data.total/data.page_size))+' 页';
    message(data.count_only?'已完成计数，当前问法不返回机构明细。':'查询完成。结果按记录分页，匹配总数不保证机构去重。');controls();
  }
  async function search(page){
    if(busy)return;
    const filter=Object.fromEntries(['province','city','district','address'].map(k=>[k,$(k).value.trim()]));
    const query=$('query').value.trim();
    if(!query&&!Object.values(filter).some(Boolean)){message('请至少填写一个地址条件或地址问法。',true);return;}
    busy=true;controls();message('正在查询…');
    try {render((await api('/api/knowledge/address/query',{document_id:$('document').value,filter,query,page,page_size:Number($('size').value),allow_partial:$('partial').checked})).data);}
    catch(error){last=null;$('head').replaceChildren();$('rows').replaceChildren();$('summary').textContent='查询未完成';$('empty').hidden=true;$('warning').hidden=true;message(error.message,true);}
    finally{busy=false;controls();}
  }
  $('search').addEventListener('submit',event=>{event.preventDefault();search(1);});
  $('prev').addEventListener('click',()=>search(currentPage-1));$('next').addEventListener('click',()=>search(currentPage+1));
  $('reset').addEventListener('click',()=>{for(const key of ['province','city','district','address','query'])$(key).value='';last=null;currentPage=1;$('head').replaceChildren();$('rows').replaceChildren();$('summary').textContent='尚未查询';$('applied').textContent='';$('warning').hidden=true;$('empty').hidden=true;$('page').textContent='第 1 页';message('请设置至少一个地址条件。');controls();});
  for(const key of ['province','city','district','address','query','partial','size','document'])$(key).addEventListener('change',()=>{last=null;$('head').replaceChildren();$('rows').replaceChildren();$('summary').textContent='条件已变更';$('applied').textContent='';$('warning').hidden=true;$('empty').hidden=true;message('条件已变更，请重新查询。');controls();});
  async function initialize(){
    const id=params.get('dataset_id');if(!/^\d+$/.test(id||'')){message('请从知识库页面的“地址查询”入口打开。',true);$('document').replaceChildren(node('option','未选择知识库'));$('submit').disabled=true;return;}
    const space=params.get('space_id');if(/^\d+$/.test(space||''))$('back').href=endpoint('/space/'+space+'/knowledge/'+id);
    try{
      const data=await api('/api/knowledge/document/list',{dataset_id:id,page:0,size:100});
      $('document').replaceChildren();
      for(const item of data.document_infos||[]){if(Number(item.format_type)!==1)continue;const option=node('option',item.name||item.document_name||'知识文档');option.value=String(item.document_id);$('document').append(option);}
      if(!$('document').options.length){message('当前知识库没有可查询的表格文档。',true);$('submit').disabled=true;return;}
      if(params.get('document_id'))$('document').value=params.get('document_id');
      message('已读取知识文档，请填写地址条件。');
    }catch(error){message(error.message,true);$('submit').disabled=true;}
  }
  initialize();
})();
