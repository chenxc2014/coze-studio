// 浏览器地址决定挂载路径，内网根路径仍使用原来的路由。
export const getAppBasePath = () =>
  typeof window !== 'undefined' && /^\/agent(?:\/|$)/.test(window.location.pathname)
    ? '/agent'
    : '';

export const withAppBasePath = (value: string) => {
  const base = getAppBasePath();
  if (!base || !value) return value;
  try {
    const url = new URL(value, window.location.href);
    if (!['http:', 'https:'].includes(url.protocol) || url.host !== window.location.host) return value;
    if (url.pathname === base || url.pathname.startsWith(`${base}/`)) return value;
    url.pathname = base + url.pathname;
    url.protocol = window.location.protocol;
    return url.href;
  } catch {
    return value;
  }
};
