import { describe, expect, it } from 'vitest';

import {
  getGuideApiBase,
  guideChatBody,
  guideCurl,
  parseGuideBotId,
} from './examples';

describe('API 接入说明示例', () => {
  it('识别子路径和根路径部署，避免重复或误加 agent 前缀', () => {
    expect(getGuideApiBase('https://example.com', '/agent/space/1')).toBe(
      'https://example.com/agent',
    );
    expect(getGuideApiBase('http://localhost:8888', '/space/1')).toBe(
      'http://localhost:8888',
    );
    expect(getGuideApiBase('https://example.com', '/agent-other')).toBe(
      'https://example.com',
    );
  });

  it('从编辑地址提取完整长整数 ID，不请求粘贴的网址', () => {
    expect(
      parseGuideBotId(
        ' https://example.com/agent/space/1/bot/9999999999999999999?x=1 ',
      ),
    ).toBe('9999999999999999999');
    expect(guideChatBody('9999999999999999999', '提问').bot_id).toBe(
      '9999999999999999999',
    );
    expect(parseGuideBotId('javascript:alert(1)')).toBe('');
  });

  it('保留中文、引号和换行，并对 Bash 单引号进行转义', () => {
    const question = '请查询 O\'Brien "门诊"\n地址';
    const body = guideChatBody('123', question);
    expect(
      JSON.parse(JSON.stringify(body)).additional_messages[0].content,
    ).toBe(question);
    expect(guideCurl('https://example.com/agent', '123', question)).toContain(
      "O'\\''Brien",
    );
    expect(guideCurl('https://example.com/agent', '', question)).toContain(
      'YOUR_BOT_ID',
    );
  });

  it('明确区分非流式提交和 SSE 请求，使用平台令牌变量', () => {
    expect(guideChatBody('123', '你好').stream).toBe(false);
    expect(guideChatBody('123', '你好', true).stream).toBe(true);
    expect(
      guideCurl('https://example.com/agent', '123', '你好', true),
    ).toContain('curl -N -sS');
    expect(guideCurl('https://example.com/agent', '123', '你好')).toContain(
      '$AGENT_API_TOKEN',
    );
  });
});
