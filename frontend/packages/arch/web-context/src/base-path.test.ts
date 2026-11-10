import { afterEach, describe, expect, it, vi } from 'vitest';
import { getAppBasePath, withAppBasePath } from './base-path';

const setLocation = (href: string) => vi.stubGlobal('window', {location: new URL(href)});
afterEach(() => vi.unstubAllGlobals());

describe('应用挂载路径', () => {
  it('只识别完整的 /agent 路径段', () => {
    for (const value of ['/agent', '/agent/', '/agent/space/1']) {
      setLocation(`https://ai.yaoxiaoyi.com${value}`);
      expect(getAppBasePath()).toBe('/agent');
    }
    setLocation('https://ai.yaoxiaoyi.com/agent-other/');
    expect(getAppBasePath()).toBe('');
  });
  it('登录跳转加前缀且保留查询及锚点', () => {
    setLocation('https://ai.yaoxiaoyi.com/agent/space');
    expect(withAppBasePath('/sign?redirect=%2Fadmin#test')).toBe('https://ai.yaoxiaoyi.com/agent/sign?redirect=%2Fadmin#test');
    expect(withAppBasePath('/agent/sign')).toBe('/agent/sign');
  });
  it('外部链接和内网根路径保持不变', () => {
    setLocation('https://ai.yaoxiaoyi.com/agent/');
    expect(withAppBasePath('https://example.com/sign')).toBe('https://example.com/sign');
    setLocation('http://192.168.1.166:8888/');
    expect(withAppBasePath('/space')).toBe('/space');
  });
});
