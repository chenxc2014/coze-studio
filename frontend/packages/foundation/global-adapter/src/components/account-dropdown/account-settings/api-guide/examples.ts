export const getGuideApiBase = (origin: string, pathname: string) =>
  origin.replace(/\/$/, '') +
  (/^\/agent(?:\/|$)/.test(pathname) ? '/agent' : '');

// ID 始终作为字符串处理，避免长整数精度丢失；粘贴地址只在本地提取 ID。
export const parseGuideBotId = (value: string) => {
  const text = value.trim();
  if (/^\d+$/.test(text)) return text;
  return text.match(/\/bot\/(\d+)(?:[/?#]|$)/)?.[1] || '';
};

export const guideChatBody = (
  botId: string,
  question: string,
  stream = false,
) => ({
  bot_id: parseGuideBotId(botId) || 'YOUR_BOT_ID',
  user_id: 'customer_001',
  stream,
  additional_messages: [
    { role: 'user', content: question, content_type: 'text' },
  ],
});

const shellQuote = (value: string) => `'${value.replace(/'/g, "'\\''")}'`;

export const guideCurl = (
  base: string,
  botId: string,
  question: string,
  stream = false,
) =>
  [
    `curl ${stream ? '-N ' : ''}-sS -X POST ${shellQuote(`${base}/v3/chat`)} \\`,
    '  -H "Authorization: Bearer $AGENT_API_TOKEN" \\',
    "  -H 'Content-Type: application/json' \\",
    `  --data-raw ${shellQuote(JSON.stringify(guideChatBody(botId, question, stream), null, 2))}`,
  ].join('\n');

export const guideGetCurl = (base: string, endpoint: string) =>
  `curl -sS ${shellQuote(`${base}${endpoint}?conversation_id=CONVERSATION_ID&chat_id=CHAT_ID`)} \\\n  -H "Authorization: Bearer $AGENT_API_TOKEN"`;
