import { describe, expect, it, vi, afterEach } from 'vitest';

import {
  ExpirationDate,
  getExpirationOptions,
  getExpirationTime,
  getStatus,
} from '../src/utils/time';

vi.mock('@coze-arch/i18n', () => ({
  I18n: { t: (key: string) => key },
}));

afterEach(() => vi.useRealTimers());

describe('令牌有效期', () => {
  it('永久选项使用永久文案，不计算日期', () => {
    const options = getExpirationOptions();
    expect(options.find(item => item.value === ExpirationDate.PERMANENT)).toEqual({
      value: 'permanent',
      label: 'api_status_permanent_1',
    });
    expect(getExpirationTime(-1)).toBe('api_status_permanent_1');
  });

  it('未来时间仍显示永久有效，普通令牌按到期时间失效', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2099-01-01T00:00:00Z'));
    const now = Math.floor(Date.now() / 1000);
    expect(getStatus(-1)).toBe(true);
    expect(getStatus(0)).toBe(false);
    expect(getStatus(-2)).toBe(false);
    expect(getStatus(now - 1)).toBe(false);
    expect(getStatus(now)).toBe(false);
    expect(getStatus(now + 60)).toBe(true);
  });
});
