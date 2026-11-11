/* 同一构建兼容内网根路径与公网 /agent/；仅调整当前站点的应用请求。 */
(() => {
  'use strict';
  const basePath = /^\/agent(?:\/|$)/.test(location.pathname) ? '/agent' : '';
  const appURL = value => {
    if (!basePath || value == null || value === '') return value;
    const input = String(value);
    let url;
    try { url = new URL(input, location.href); } catch { return value; }
    if (!['http:', 'https:', 'ws:', 'wss:'].includes(url.protocol) || url.host !== location.host) return value;
    if (url.pathname === basePath || url.pathname.startsWith(basePath + '/')) return value;
    url.pathname = basePath + url.pathname;
    if (url.protocol === 'http:' || url.protocol === 'https:') url.protocol = location.protocol;
    else url.protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
    return url.href;
  };
  window.CozePath = Object.freeze({basePath, url:appURL});
  const updateAddressEntry = () => {
    const match = location.pathname.match(/\/space\/(\d+)\/knowledge\/(\d+)(?:\/|$)/);
    let entry = document.getElementById('coze-knowledge-address-entry');
    if (!match) { if (entry) entry.remove(); return; }
    if (!entry) {
      entry = document.createElement('a');
      entry.id = 'coze-knowledge-address-entry';
      entry.textContent = '地址查询';
      entry.style.cssText = 'position:fixed;right:28px;bottom:24px;z-index:1000;padding:10px 18px;border-radius:8px;background:#2875b9;color:#fff;text-decoration:none;font:14px system-ui;box-shadow:0 3px 14px #17323f26';
      document.body.append(entry);
    }
    const href = basePath + '/admin/knowledge-address.html?space_id=' + match[1] + '&dataset_id=' + match[2];
    if (entry.getAttribute('href') !== href) entry.setAttribute('href', href);
  };
  const installAddressEntry = () => {
    updateAddressEntry();
    let scheduled = false;
    new MutationObserver(() => {
      if (scheduled) return;
      scheduled = true;
      requestAnimationFrame(() => { scheduled = false; updateAddressEntry(); });
    }).observe(document.body, {childList:true, subtree:true});
  };
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', installAddressEntry, {once:true});
  else installAddressEntry();
  if (!basePath) return;

  const fetch = window.fetch.bind(window);
  window.fetch = (input, init) => {
    if (input instanceof Request) {
      const mapped = appURL(input.url);
      return fetch(mapped === input.url ? input : new Request(mapped, input), init);
    }
    return fetch(appURL(input), init);
  };
  const open = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function(method, url, ...options) {
    return open.call(this, method, appURL(url), ...options);
  };
  for (const name of ['EventSource', 'WebSocket']) {
    const Native = window[name];
    if (!Native) continue;
    const Wrapped = function(url, options) {
      return arguments.length > 1 ? new Native(appURL(url), options) : new Native(appURL(url));
    };
    Wrapped.prototype = Native.prototype;
    Object.setPrototypeOf(Wrapped, Native);
    window[name] = Wrapped;
  }
  for (const name of ['pushState', 'replaceState']) {
    const original = history[name].bind(history);
    history[name] = (state, unused, url) => original(state, unused, appURL(url));
  }
  const openWindow = window.open.bind(window);
  window.open = (url, ...options) => openWindow(appURL(url), ...options);
  document.addEventListener('click', event => {
    const target = event.target instanceof Element ? event.target.closest('a[href]') : null;
    if (target) target.href = appURL(target.href);
  }, true);
})();
