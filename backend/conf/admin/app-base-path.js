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
