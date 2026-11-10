/* 账号管理页使用独立脚本，沿用管理后台的登录会话。 */
(() => {
  'use strict';
  const api = '/api/admin/accounts';
  const escape = value => String(value ?? '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
  async function request(path, payload) {
    const options = {credentials:'same-origin',cache:'no-store'};
    if (payload !== undefined) Object.assign(options,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)});
    const response = await fetch(api + path,options);
    const result = await response.json().catch(() => ({}));
    if (!response.ok || result.code !== 0) throw new Error(result.message || (response.status === 401 ? '登录已失效，请重新登录' : '操作失败，请确认账号权限并重试'));
    return result.data;
  }
  window.loadAccountManagementPage = () => {
    const container = document.getElementById('pageContent');
    container.innerHTML = `<section class="accounts-page">
      <div class="accounts-heading"><div><h2>账号管理</h2><p>统一维护平台账号、登录状态及管理员身份。</p></div><button class="account-primary" id="account-create">新增账号</button></div>
      <div class="accounts-note">停用会阻止登录、终止网页会话并暂停该账号的 API Token。重置密码后需重新登录；已创建的智能体与知识库会保留。</div>
      <form class="accounts-filters" id="account-search-form">
        <input id="account-search" aria-label="搜索账号" placeholder="搜索邮箱、姓名或用户名" maxlength="100">
        <select id="account-status" aria-label="账号状态"><option value="">全部状态</option><option value="enabled">已启用</option><option value="disabled">已停用</option></select>
        <select id="account-role" aria-label="账号角色"><option value="">全部角色</option><option value="admin">管理员</option><option value="user">普通用户</option></select>
        <button type="submit">查询</button><button type="button" id="account-refresh">刷新</button>
      </form>
      <div id="account-message" role="status" aria-live="polite"></div>
      <div class="accounts-table-wrap"><table class="accounts-table"><thead><tr><th>账号</th><th>角色</th><th>状态</th><th>创建时间</th><th>操作</th></tr></thead><tbody id="account-rows"></tbody></table></div>
      <div class="accounts-pagination"><span id="account-total"></span><div><button id="account-prev">上一页</button><span id="account-page"></span><button id="account-next">下一页</button></div></div>
    </section>`;
    const root = container.querySelector('.accounts-page');
    const $ = id => root.querySelector('#' + id);
    let page = 1, currentUser = '', rows = [], total = 0, busy = false, sequence = 0;
    const message = (text,error = false) => { $('account-message').textContent=text; $('account-message').className=error?'account-error':'account-success'; };
    async function load() {
      const ticket = ++sequence;
      $('account-rows').innerHTML='<tr><td colspan="5" class="account-empty">正在加载…</td></tr>';
      const query = new URLSearchParams({q:$('account-search').value,status:$('account-status').value,role:$('account-role').value,page:String(page),page_size:'20'});
      try {
        const data = await request('?' + query);
        if (ticket!==sequence || !root.isConnected) return;
        rows=data.accounts; total=data.total; currentUser=data.current_user_id;
        $('account-rows').innerHTML=rows.length ? rows.map(account => {
          const self=account.id===currentUser;
          return `<tr><td><strong>${escape(account.name)}${self?' <span class="account-self">当前账号</span>':''}</strong><span class="account-email">${escape(account.email)}</span><small>${escape(account.unique_name)}</small></td>
            <td>${account.is_admin?'管理员':'普通用户'}</td><td><span class="account-badge ${account.disabled?'disabled':''}">${account.disabled?'已停用':'已启用'}</span></td>
            <td>${escape(new Date(account.created_at).toLocaleString('zh-CN',{hour12:false}))}</td>
            <td class="account-actions"><button data-action="edit" data-id="${escape(account.id)}">编辑</button><button data-action="password" data-id="${escape(account.id)}">重置密码</button><button data-action="role" data-id="${escape(account.id)}" ${self || (account.disabled&&!account.is_admin)?'disabled':''}>${account.is_admin?'取消管理员':'设为管理员'}</button><button class="${account.disabled?'':'account-danger'}" data-action="status" data-id="${escape(account.id)}" ${self?'disabled':''}>${account.disabled?'启用':'停用'}</button></td></tr>`;
        }).join('') : '<tr><td colspan="5" class="account-empty">没有符合条件的账号</td></tr>';
        $('account-total').textContent=`共 ${total} 个账号`;
        $('account-page').textContent=`第 ${page} / ${Math.max(1,Math.ceil(total/20))} 页`;
        $('account-prev').disabled=page===1; $('account-next').disabled=page*20>=total;
      } catch(error) { if(ticket===sequence && root.isConnected) { message(error.message,true);$('account-rows').innerHTML='<tr><td colspan="5" class="account-empty">加载失败，请刷新重试</td></tr>'; } }
    }
    async function perform(path,payload,success) {
      if(busy) return false;
      busy=true;
      try { await request(path,payload);message(success);await load();return true; }
      catch(error) { message(error.message,true);return false; }
      finally { busy=false; }
    }
    function dialog(title,fields,onSubmit) {
      const element=document.createElement('dialog');element.className='account-dialog';
      element.innerHTML=`<form><h3>${escape(title)}</h3>${fields}<div class="account-dialog-error" role="alert"></div><div class="account-dialog-buttons"><button type="button" data-cancel>取消</button><button type="submit" class="account-primary">保存</button></div></form>`;
      document.body.append(element);
      element.querySelector('[data-cancel]').onclick=()=>element.close();
      element.addEventListener('close',()=>element.remove());
      element.querySelector('form').onsubmit=async event=>{
        event.preventDefault();const button=element.querySelector('[type=submit]');button.disabled=true;
        try { if(await onSubmit(new FormData(event.target))) element.close();else element.querySelector('.account-dialog-error').textContent=$('account-message').textContent; }
        catch(error) { element.querySelector('.account-dialog-error').textContent=error.message; }
        finally { button.disabled=false; }
      };
      element.showModal();
    }
    $('account-create').onclick=()=>dialog('新增账号',`
      <label>邮箱<input name="email" type="email" required maxlength="128" autocomplete="off"></label>
      <label>姓名<input name="name" required maxlength="64" autocomplete="off"></label>
      <label>初始密码<input name="password" type="password" required minlength="12" maxlength="128" autocomplete="new-password"></label><p>密码需要 12 至 128 个字符，请通过安全渠道交付给使用人。</p>
      <label>角色<select name="role"><option value="user">普通用户</option><option value="admin">管理员</option></select></label>`,data=>perform('/create',{email:data.get('email'),name:data.get('name'),password:data.get('password'),is_admin:data.get('role')==='admin'},'账号已创建，可使用邮箱和初始密码登录。'));
    $('account-rows').onclick=event=>{
      const button=event.target.closest('button[data-action]');if(!button||button.disabled||busy)return;
      const account=rows.find(item=>item.id===button.dataset.id);if(!account)return;
      switch(button.dataset.action) {
        case 'edit':dialog('编辑账号',`<p>${escape(account.email)}</p><label>姓名<input name="name" value="${escape(account.name)}" required maxlength="64"></label><p>邮箱是登录标识，暂不支持修改。</p>`,data=>perform('/update',{id:account.id,name:data.get('name')},'账号资料已更新。'));break;
        case 'password':dialog('重置密码',`<p>${escape(account.email)}</p><label>新密码<input name="password" type="password" required minlength="12" maxlength="128" autocomplete="new-password"></label><p>保存后，该账号的网页会话将失效。API Token 不因重置密码而撤销。</p>`,async data=>{
          const ok=await perform('/reset-password',{id:account.id,password:data.get('password')},'密码已重置，该账号需要重新登录。');
          if(ok&&account.id===currentUser) location.href='/sign?redirect=%2Fadmin%2F%23account-management';return ok;
        });break;
        case 'role':if(confirm(`确认${account.is_admin?'取消':'授予'} ${account.email} 的管理员身份？管理员可以管理平台配置及所有账号。`)) perform('/role',{id:account.id,is_admin:!account.is_admin},'管理员身份已更新。');break;
        case 'status':if(confirm(`确认${account.disabled?'启用':'停用'} ${account.email}？${account.disabled?'启用后需要重新登录。':'该账号将无法登录，已有网页会话及 API Token 将无法继续使用。'}`)) perform('/status',{id:account.id,disabled:!account.disabled},'账号状态已更新。');break;
      }
    };
    $('account-search-form').onsubmit=event=>{event.preventDefault();page=1;message('');load();};
    $('account-refresh').onclick=()=>{message('');load();};
    $('account-prev').onclick=()=>{if(page>1){page--;load();}};
    $('account-next').onclick=()=>{if(page*20<total){page++;load();}};
    load();
  };
})();
