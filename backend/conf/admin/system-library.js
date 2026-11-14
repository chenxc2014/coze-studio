/* 系统资源发布仅管理员可用，不读取或回显源资源凭据。 */
(function () {
  'use strict';
  const labels = {
    plugin: '插件',
    workflow: '工作流',
    prompt: '提示词',
    knowledge: '知识库',
    database: '数据库',
  };
  const escape = value =>
    String(value ?? '').replace(
      /[&<>"']/g,
      ch =>
        ({
          '&': '&amp;',
          '<': '&lt;',
          '>': '&gt;',
          '"': '&quot;',
          "'": '&#39;',
        })[ch],
    );
  async function call(action, params, body) {
    const path =
      '/api/admin/system-library/' +
      action +
      (body ? '' : '?' + new URLSearchParams(params || {}));
    const response = await fetch(window.CozePath.url(path), {
      method: body ? 'POST' : 'GET',
      credentials: 'same-origin',
      headers: body ? { 'Content-Type': 'application/json' } : {},
      body: body ? JSON.stringify(body) : undefined,
    });
    const data = await response.json();
    if (!response.ok || data.code !== 0)
      throw new Error(data.message || '系统资源操作失败');
    return data.data;
  }
  window.renderSystemLibrary = function (container) {
    let preview = null;
    let page = 1;
    let busy = false;
    container.innerHTML = `<section class="system-publisher"><header><div><h2>系统资源发布</h2><p>将维护完成的资源发布为全平台共享资源，所有用户的所有项目均可引用。</p></div><button id="sr-refresh">刷新</button></header>
    <div class="sr-notice">仅发布平台提供、已验证可直接使用且无需厂商额外授权的资源。搜索工具中的第三方插件保持独立，不自动进入系统资源库。知识库和数据库的数据会向全平台用户开放查询。</div>
    <section class="sr-publish-form"><h3>发布或更新资源</h3><div class="sr-form-row"><label>资源类型<select id="sr-kind">${Object.entries(
      labels,
    )
      .map(([key, label]) => `<option value="${key}">${label}</option>`)
      .join(
        '',
      )}</select></label><label class="sr-id-label">资源 ID 或详情地址<input id="sr-id" placeholder="粘贴资源 ID 或资源详情页地址" autocomplete="off"></label><button id="sr-preview">读取并预览</button></div><p class="sr-help">从当前管理员自己的资源库中开发、维护资源，再粘贴资源详情地址或 ID。不会复制资源，发布后保留原资源 ID。</p><div id="sr-preview-result"></div></section>
    <div class="sr-toolbar"><select id="sr-filter"><option value="all">全部类型</option>${Object.entries(
      labels,
    )
      .map(([key, label]) => `<option value="${key}">${label}</option>`)
      .join(
        '',
      )}</select><input id="sr-search" type="search" placeholder="搜索系统资源" maxlength="100"><span id="sr-total"></span></div><p id="sr-message" role="status"></p><div id="sr-list"></div><footer><button id="sr-prev">上一页</button><span id="sr-page"></span><button id="sr-next">下一页</button></footer></section>`;
    const node = id => container.querySelector('#' + id);
    const message = (text, error = false) => {
      node('sr-message').textContent = text;
      node('sr-message').className = error ? 'sr-error' : '';
    };
    const resourceID = raw => {
      if (/^\d+$/.test(raw.trim())) return raw.trim();
      try {
        const url = new URL(raw);
        const query = url.searchParams;
        for (const name of [
          'workflow_id',
          'plugin_id',
          'dataset_id',
          'database_id',
          'prompt_id',
        ])
          if (/^\d+$/.test(query.get(name) || '')) return query.get(name);
        const match = url.pathname.match(
          /(?:knowledge|database|plugin|prompt)\/(\d+)(?:\/|$)/,
        );
        return match?.[1] || '';
      } catch {
        return '';
      }
    };
    const reset = () => {
      preview = null;
      node('sr-preview-result').replaceChildren();
    };
    node('sr-kind').onchange = reset;
    node('sr-id').oninput = reset;
    function showPreview(item) {
      preview = item;
      node('sr-preview-result').innerHTML =
        `<div class="sr-preview-card"><h4>${escape(item.name)}</h4><p>${escape(item.description)}</p><dl><dt>资源类型</dt><dd>${escape(labels[item.kind])}</dd><dt>资源 ID</dt><dd>${escape(item.id)}</dd><dt>发布版本</dt><dd>${escape(item.version || '当前版本')}</dd><dt>共享方式</dt><dd>共享引用，系统原件只读</dd></dl><p>${escape(item.notice || item.usage)}</p><label class="sr-confirm"><input type="checkbox" id="sr-confirm">我已确认该资源及其数据可以向平台所有用户公开共享，并已验证更新对已有项目的影响。</label><label class="sr-confirm"><input type="checkbox" id="sr-ready-confirm">我已完成预览调试，确认资源及其全部依赖可直接使用，且无需厂商额外授权。</label><button id="sr-publish" class="sr-primary" disabled>${item.revision ? '更新发布' : '发布为系统资源'}</button></div>`;
      node('sr-confirm').onchange = () =>
        (node('sr-publish').disabled =
          !node('sr-confirm').checked ||
          !node('sr-ready-confirm').checked ||
          busy);
      node('sr-ready-confirm').onchange = node('sr-confirm').onchange;
      node('sr-publish').onclick = async () => {
        if (
          busy ||
          !preview ||
          !node('sr-confirm').checked ||
          !node('sr-ready-confirm').checked
        )
          return;
        busy = true;
        node('sr-publish').disabled = true;
        try {
          const result = await call('publish', null, {
            kind: preview.kind,
            id: preview.id,
            revision: preview.revision || 0,
            confirmed: true,
            ready_confirmed: true,
          });
          showPreview(result);
          await load();
          message(
            '发布成功，所有用户均可在“资源库-系统”及项目资源选择窗口中使用。',
          );
        } catch (e) {
          message(e.message, true);
        } finally {
          busy = false;
          if (node('sr-publish'))
            node('sr-publish').disabled =
              !node('sr-confirm').checked || !node('sr-ready-confirm').checked;
        }
      };
    }
    node('sr-preview').onclick = async () => {
      const id = resourceID(node('sr-id').value);
      if (!id) {
        message('请输入正确的资源 ID 或详情地址。', true);
        return;
      }
      node('sr-preview').disabled = true;
      try {
        showPreview(await call('preview', { kind: node('sr-kind').value, id }));
        message('请核对源资源及共享范围，再确认发布。');
      } catch (e) {
        reset();
        message(e.message, true);
      } finally {
        node('sr-preview').disabled = false;
      }
    };
    async function load() {
      try {
        const result = await call('list', {
          kind: node('sr-filter').value,
          q: node('sr-search').value,
          page: String(page),
          page_size: '15',
        });
        if (!container.isConnected) return;
        node('sr-total').textContent = '共 ' + result.total + ' 个资源';
        node('sr-page').textContent =
          page + ' / ' + Math.max(1, Math.ceil(result.total / 15));
        node('sr-prev').disabled = page <= 1;
        node('sr-next').disabled = page * 15 >= result.total;
        node('sr-list').innerHTML = result.resources.length
          ? `<div class="sr-table-wrap"><table><thead><tr><th>资源</th><th>类型</th><th>来源</th><th>版本</th><th>状态</th><th>操作</th></tr></thead><tbody>${result.resources.map((item, index) => `<tr><td><strong>${escape(item.name)}</strong><small>${escape(item.id)}</small></td><td>${escape(labels[item.kind])}</td><td>${item.origin === 'builtin' ? '平台内置' : '管理员发布'}</td><td>${escape(item.version || '当前版本')}</td><td><span class="sr-status ${item.status === 'published' ? 'sr-active' : ''}">${item.status === 'published' ? '已发布' : '已下架'}</span></td><td><button data-index="${index}" data-action="preview">查看 / 更新</button>${item.status === 'published' ? `<button class="sr-danger" data-index="${index}" data-action="withdraw">下架</button>` : ''}</td></tr>`).join('')}</tbody></table></div>`
          : '<div class="sr-empty">暂无匹配的资源</div>';
        node('sr-list')
          .querySelectorAll('button[data-index]')
          .forEach(
            button =>
              (button.onclick = async () => {
                const item = result.resources[Number(button.dataset.index)];
                if (button.dataset.action === 'preview') {
                  node('sr-kind').value = item.kind;
                  node('sr-id').value = item.id;
                  try {
                    const value = await call('preview', {
                      kind: item.kind,
                      id: item.id,
                    });
                    showPreview(value);
                    node('sr-preview-result').scrollIntoView({
                      block: 'center',
                      behavior: 'smooth',
                    });
                  } catch (e) {
                    message(e.message, true);
                  }
                  return;
                }
                if (
                  !window.confirm(
                    '下架“' +
                      item.name +
                      '”？下架后不再对普通用户开放，已有共享引用运行时也会提示资源不可用。',
                  )
                )
                  return;
                button.disabled = true;
                try {
                  await call('withdraw', null, {
                    kind: item.kind,
                    id: item.id,
                    revision: item.revision || 0,
                  });
                  await load();
                  message('资源已下架，原资源和历史版本记录保留。');
                } catch (e) {
                  message(e.message, true);
                  button.disabled = false;
                }
              }),
          );
      } catch (e) {
        message(e.message, true);
      }
    }
    node('sr-refresh').onclick = load;
    node('sr-filter').onchange = () => {
      page = 1;
      load();
    };
    let timer;
    node('sr-search').oninput = () => {
      clearTimeout(timer);
      timer = setTimeout(() => {
        page = 1;
        load();
      }, 250);
    };
    node('sr-prev').onclick = () => {
      page--;
      load();
    };
    node('sr-next').onclick = () => {
      page++;
      load();
    };
    load();
  };
})();
