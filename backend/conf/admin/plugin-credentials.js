/* 插件授权仅保留在本次输入中，不回显、不写入浏览器存储。 */
(() => {
  'use strict';
  const endpoint = window.CozePath ? window.CozePath.url('/api/admin/plugins') : '/api/admin/plugins';
  const escape = value => String(value ?? '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
  async function request(path, body) {
    const options = {credentials:'same-origin',cache:'no-store'};
    if (body !== undefined) Object.assign(options,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
    const response = await fetch(endpoint + path, options);
    const result = await response.json().catch(() => ({}));
    if (!response.ok || result.code !== 0) throw new Error(result.message || (response.status===401?'登录已失效，请重新登录':response.status===403?'只有平台管理员可以维护插件授权':'操作失败，请稍后重试'));
    return result.data;
  }
  window.loadPluginCredentialsPage = () => {
    const container = document.getElementById('pageContent');
    container.innerHTML = `<section class="plugin-credentials-page">
      <div class="plugin-heading"><div><h2>插件授权配置</h2><p>统一维护常用插件的供应商授权，保存后对后续调用即时生效。</p></div><button type="button" id="plugin-refresh">刷新状态</button></div>
      <div class="plugin-notice">这些凭据由平台内使用相应插件的智能体共用，调用量合并计入供应商额度。密钥不会回显，输入留空会保留原值；配置状态不代表供应商已经验证通过。</div>
      <div id="plugin-global-message" role="status" aria-live="polite">正在读取授权状态…</div>
      <div class="plugin-cards" id="plugin-cards"></div>
    </section>`;
    const root = container.querySelector('.plugin-credentials-page');
    const cards = root.querySelector('#plugin-cards');
    const message = root.querySelector('#plugin-global-message');
    const refresh = root.querySelector('#plugin-refresh');
    let ready = false, busy = false;
    const providers = new Map();
    function notice(text,error=false) {message.textContent=text;message.className=error?'plugin-message error':'plugin-message';}
    function buttons() {
      refresh.disabled=busy;
      cards.querySelectorAll('.plugin-card').forEach(card => {
        const provider=providers.get(card.dataset.provider);
        const input=card.querySelector('input');
        input.disabled=busy||!ready;
        card.querySelector('[data-action="save"]').disabled=busy||!ready||!input.value;
        card.querySelector('[data-action="test"]').disabled=busy||!provider.configured||!!input.value;
        card.querySelector('[data-action="clear"]').disabled=busy||!ready||!provider.configured;
      });
    }
    function status(card,provider) {
      const element=card.querySelector('.plugin-status');
      element.textContent=provider.configured?'已配置':'未配置';element.className='plugin-status '+(provider.configured?'configured':'missing');
      card.querySelector('.plugin-source').textContent=provider.source==='admin'?'来源：后台配置':provider.source==='file'?'来源：服务器配置文件':'尚未设置供应商授权';
      card.querySelector('.plugin-updated').textContent=provider.updated_at?'上次更新：'+new Date(provider.updated_at).toLocaleString('zh-CN',{hour12:false}):'';
    }
    function result(card,text,error=false) {
      const element=card.querySelector('.plugin-result');element.textContent=text;element.className='plugin-result'+(error?' error':'');
    }
    async function load() {
      if(busy)return;
      if([...cards.querySelectorAll('input')].some(input=>input.value)&&!confirm('刷新将清空尚未保存的密钥输入，是否继续？'))return;
      busy=true;buttons();
      try {
        const data=await request('');if(!root.isConnected)return;
        ready=data.encryption_ready;providers.clear();data.providers.forEach(provider=>providers.set(provider.id,provider));
        cards.innerHTML=data.providers.map(provider=>`<article class="plugin-card" data-provider="${escape(provider.id)}">
          <div class="plugin-card-heading"><h3>${escape(provider.name)}</h3><span class="plugin-status"></span></div>
          <p class="plugin-description">${escape(provider.description)}</p><p class="plugin-source"></p>
          <form autocomplete="off"><label for="plugin-key-${escape(provider.id)}">${escape(provider.credential_label)}</label>
            <input id="plugin-key-${escape(provider.id)}" name="credential" type="password" autocomplete="new-password" maxlength="4096" spellcheck="false" placeholder="填写新密钥；留空保留原值">
            <p class="plugin-input-note">输入后请先保存，再测试已保存的配置。测试会发起一次真实请求，可能计入供应商调用额度。</p>
            <div class="plugin-actions"><button type="submit" data-action="save" class="plugin-primary">保存授权</button><button type="button" data-action="test">测试已保存配置</button><button type="button" data-action="clear" class="plugin-danger">清除授权</button></div>
          </form><div class="plugin-result" role="status" aria-live="polite"></div><p class="plugin-updated"></p>
          <a href="${escape(provider.apply_url)}" target="_blank" rel="noopener noreferrer">前往官方平台申请授权</a>
        </article>`).join('');
        cards.querySelectorAll('.plugin-card').forEach(card=>status(card,providers.get(card.dataset.provider)));
        notice(ready?'密钥加密保存，页面仅显示配置状态。':'服务端尚未准备好凭据加密密钥，请联系运维人员完成初始化。',!ready);
      } catch(error) {if(root.isConnected)notice(error.message,true);}
      finally {busy=false;if(root.isConnected)buttons();}
    }
    async function act(card,action) {
      if(busy)return;
      const id=card.dataset.provider,input=card.querySelector('input');
      if(action==='save'&&!input.value)return;
      if(action==='clear'&&!confirm(`确认清除${providers.get(id).name}的平台共用授权？使用该插件的智能体后续调用将失去这份授权。`))return;
      busy=true;buttons();result(card,action==='test'?'正在请求供应商，请稍候…':'正在保存…');
      try {
        if(action==='test') {
          const data=await request('/test',{provider:id});
          if(root.isConnected)result(card,(data.success?'测试通过：':'测试未通过：')+data.message+'（状态码：'+data.code+'）',!data.success);
        } else {
          const payload={provider:id,clear:action==='clear'};
          if(action==='save')payload.credential=input.value;
          const data=await request('/save',payload);
          if(root.isConnected) {providers.set(id,data);input.value='';status(card,data);result(card,action==='clear'?'授权已清除。':'授权已保存，对后续调用即时生效；请点击测试确认供应商权限。');}
        }
      } catch(error) {if(root.isConnected)result(card,error.message,true);}
      finally {busy=false;if(root.isConnected)buttons();}
    }
    cards.addEventListener('submit',event=>{event.preventDefault();act(event.target.closest('.plugin-card'),'save');});
    cards.addEventListener('click',event=>{
      const button=event.target.closest('button[data-action]');if(!button||button.disabled||button.type==='submit')return;
      act(button.closest('.plugin-card'),button.dataset.action);
    });
    cards.addEventListener('input',event=>{const card=event.target.closest('.plugin-card');if(card){result(card,event.target.value?'有未保存的输入，请保存后再测试。':'');buttons();}});
    refresh.addEventListener('click',load);load();
  };
})();
