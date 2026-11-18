import { type ReactNode, useRef, useState } from 'react';

import { Button } from '@coze-arch/coze-design';

import {
  getGuideApiBase,
  guideChatBody,
  guideCurl,
  guideGetCurl,
  parseGuideBotId,
} from './examples';

import s from './index.module.less';

async function copyGuideText(value: string) {
  try {
    await navigator.clipboard.writeText(value);
    return;
  } catch {
    const field = document.createElement('textarea');
    field.value = value;
    field.style.position = 'fixed';
    field.style.opacity = '0';
    document.body.append(field);
    field.select();
    const copied = document.execCommand('copy');
    field.remove();
    if (!copied) throw new Error('请手动选择并复制示例');
  }
}

const CodeExample = ({
  title,
  value,
  testId,
}: {
  title: string;
  value: string;
  testId: string;
}) => {
  const [copyStatus, setCopyStatus] = useState('');
  return (
    <div className={s.example} data-testid={testId}>
      <div className={s['example-header']}>
        <span>{title}</span>
        <button
          type="button"
          aria-label={`复制${title}`}
          onClick={async () => {
            try {
              await copyGuideText(value);
              setCopyStatus('已复制');
            } catch {
              setCopyStatus('请手动选择并复制');
            }
          }}
        >
          复制
        </button>
      </div>
      <pre>
        <code>{value}</code>
      </pre>
      <span className={s['copy-status']} role="status">
        {copyStatus}
      </span>
    </div>
  );
};

const Section = ({
  id,
  title,
  children,
}: {
  id: string;
  title: string;
  children: ReactNode;
}) => (
  <section id={`api-guide-${id}`} className={s.section}>
    <h3>{title}</h3>
    {children}
  </section>
);

export const ApiAuthHeader = ({
  onAddToken,
  onOpenGuide,
}: {
  onAddToken: () => void;
  onOpenGuide: () => void;
}) => (
  <div className={s['auth-header']}>
    <div className={s['auth-actions']}>
      <h3>个人访问令牌</h3>
      <Button onClick={onOpenGuide} data-testid="api-auth.open-guide">
        API接入说明
      </Button>
      <Button theme="solid" type="primary" onClick={onAddToken}>
        添加新令牌
      </Button>
    </div>
    <p>
      外部应用使用平台 API
      令牌调用已发布的智能体。首次接入可先阅读“API接入说明”，再创建并妥善保存令牌。
    </p>
  </div>
);

export const ApiIntegrationGuide = () => {
  const guideRef = useRef<HTMLDivElement>(null);
  const [botId, setBotId] = useState('');
  const [question, setQuestion] =
    useState('你好，请介绍一下你能提供哪些帮助。');
  const base = getGuideApiBase(
    window.location.origin,
    window.location.pathname,
  );
  const validId = parseGuideBotId(botId);
  const body = JSON.stringify(guideChatBody(botId, question), null, 2);
  const scrollToSection = (id: string) => {
    const container = guideRef.current;
    const section = container?.querySelector<HTMLElement>(`#api-guide-${id}`);
    if (!container || !section) return;
    const navigationHeight = container.querySelector('nav')?.offsetHeight || 0;
    const top =
      container.scrollTop +
      section.getBoundingClientRect().top -
      container.getBoundingClientRect().top -
      navigationHeight -
      8;
    container.scrollTo({ top: Math.max(0, top) });
  };
  return (
    <div className={s.guide} ref={guideRef} data-testid="api-integration-guide">
      <header className={s.intro}>
        <span className={s.eyebrow}>外部应用接入</span>
        <h2>调用已发布的智能体</h2>
        <p>
          完成 API 发布后，你的业务后端可通过 HTTPS
          提问，获取智能体结合模型、知识库和工具生成的回答。
        </p>
        <div className={s.address}>
          当前接口前缀 <code data-testid="api-guide.base-url">{base}</code>
        </div>
      </header>
      <nav className={s.navigation} aria-label="API 接入说明目录">
        {[
          ['prepare', '接入准备'],
          ['request', '发起提问'],
          ['result', '获取回答'],
          ['stream', '流式与追问'],
          ['faq', '常见问题'],
        ].map(([id, label]) => (
          <button type="button" key={id} onClick={() => scrollToSection(id)}>
            {label}
          </button>
        ))}
      </nav>
      <Section id="prepare" title="1. 发布智能体并准备令牌">
        <ol>
          <li>
            在智能体编辑器配置并调试，点击“发布”，选择 API
            渠道。接口执行的是已发布版本，修改后需重新发布。
          </li>
          <li>
            复制智能体编辑地址中 <code>/bot/</code> 后的数字作为{' '}
            <code>bot_id</code>，不是空间 ID、知识库 ID 或工作流 ID。ID
            应始终作为字符串传递。
          </li>
          <li>
            由智能体所属账号在“API
            授权”中新建令牌，也可使用已保存且有效的令牌。有效期支持“永久有效”；明文仅在创建时展示，遗失后需新建并替换。
          </li>
        </ol>
        <p className={s.note}>
          调用使用平台 API 令牌，不传平台登录密码，也不传模型厂商的 API
          Key。令牌由业务后端保管，不写入公开网页或前端代码。
        </p>
      </Section>
      <Section id="request" title="2. 发起提问">
        <p>以下示例在本地生成，不会自动调用 API，也不会读取或保存你的令牌。</p>
        <div className={s.form}>
          <label htmlFor="api-guide-bot-id">
            智能体 ID 或编辑页面地址
            <input
              id="api-guide-bot-id"
              data-testid="api-guide.bot-id"
              value={botId}
              onChange={event => setBotId(event.target.value)}
              placeholder="粘贴 /bot/ 后的数字，或智能体完整编辑地址"
              maxLength={2048}
              autoComplete="off"
              spellCheck={false}
            />
          </label>
          {botId && !validId ? (
            <p className={s.error} role="alert">
              请输入数字 ID，或包含 /bot/数字ID 的编辑页面地址。
            </p>
          ) : null}
          <label htmlFor="api-guide-question">
            示例问题
            <textarea
              id="api-guide-question"
              data-testid="api-guide.question"
              value={question}
              onChange={event => setQuestion(event.target.value)}
              maxLength={4000}
              rows={2}
            />
          </label>
        </div>
        <p>
          <strong>请求：</strong>
          <code>POST {base}/v3/chat</code>
        </p>
        <CodeExample
          title="请求头"
          testId="api-guide.headers"
          value={
            'Authorization: Bearer YOUR_PLATFORM_API_TOKEN\nContent-Type: application/json'
          }
        />
        <CodeExample
          title="JSON 请求体"
          testId="api-guide.request-body"
          value={body}
        />
        <p>
          <code>user_id</code>{' '}
          填业务系统中的用户标识，如客户编号；它不是平台登录邮箱。
          <code>stream: false</code> 表示先提交任务，再查询结果。
        </p>
        <CodeExample
          title="cURL 提问示例（Bash）"
          testId="api-guide.curl"
          value={guideCurl(base, botId, question)}
        />
        <p>
          执行 cURL 前，在调用方环境配置 <code>AGENT_API_TOKEN</code>。未填写 ID
          时，示例中的 <code>YOUR_BOT_ID</code> 需要替换；Postman
          可直接填写以上地址、请求头与 JSON 请求体。
        </p>
      </Section>
      <Section id="result" title="3. 查询状态并取得最终回答">
        <p>
          非流式提交先返回任务信息，<strong>不会直接返回完整回答</strong>。保存{' '}
          <code>data.id</code> 作为 <code>chat_id</code>，保存{' '}
          <code>data.conversation_id</code> 作为会话 ID。
        </p>
        <CodeExample
          title="提交响应示意"
          testId="api-guide.submit-response"
          value={JSON.stringify(
            {
              code: 0,
              msg: '',
              data: {
                id: 'CHAT_ID',
                conversation_id: 'CONVERSATION_ID',
                status: 'created',
              },
            },
            null,
            2,
          )}
        />
        <p>
          携带相同的 Bearer 令牌调用状态接口，每隔约 1
          秒查询一次，并设置总超时。把示例中的 <code>CONVERSATION_ID</code> 和{' '}
          <code>CHAT_ID</code> 替换为本次响应值。
        </p>
        <CodeExample
          title="查询任务状态"
          testId="api-guide.retrieve"
          value={guideGetCurl(base, '/v3/chat/retrieve')}
        />
        <p>
          <code>data.status = completed</code> 时获取消息；<code>created</code>
          、<code>in_progress</code> 表示尚未完成。遇到 <code>failed</code>、
          <code>expired</code>、<code>cancelled</code> 或{' '}
          <code>required_action</code>，应停止普通轮询并处理对应状态。
        </p>
        <CodeExample
          title="获取本次消息列表"
          testId="api-guide.messages"
          value={guideGetCurl(base, '/v3/chat/message/list')}
        />
        <p className={s.note}>
          当前版本的消息列表字段名是{' '}
          <strong>
            <code>Messages</code>（首字母大写）
          </strong>
          。从中筛选 <code>role = assistant</code> 且 <code>type = answer</code>{' '}
          的消息，<code>content</code>{' '}
          即回答内容；不要把工具调用、工具结果或推理内容当作最终回答。
        </p>
        <CodeExample
          title="消息列表响应示意"
          testId="api-guide.message-response"
          value={JSON.stringify(
            {
              code: 0,
              msg: '',
              Messages: [
                {
                  role: 'assistant',
                  type: 'answer',
                  content_type: 'text',
                  content: '这里是智能体的回答。',
                },
              ],
            },
            null,
            2,
          )}
        />
        <CodeExample
          title="JavaScript 提取回答（业务后端）"
          testId="api-guide.extract-answer"
          value={
            'const answers = (response.Messages ?? [])\n  .filter(message => message.role === "assistant" && message.type === "answer")\n  .map(message => message.content);'
          }
        />
      </Section>
      <Section id="stream" title="4. 流式接收与连续对话">
        <p>
          需要边生成边展示时，将 <code>stream</code> 改为 <code>true</code>，按
          SSE 事件读取响应，不把它作为普通 JSON 一次解析。
        </p>
        <CodeExample
          title="cURL 流式示例（Bash）"
          testId="api-guide.stream"
          value={guideCurl(base, botId, question, true)}
        />
        <ul>
          <li>
            <code>conversation.message.delta</code>：仅拼接{' '}
            <code>type = answer</code> 的回答增量，按消息 ID 区分。
          </li>
          <li>
            <code>conversation.message.completed</code>
            ：也可使用此事件中的完整回答；不要与已拼接增量重复追加。
          </li>
          <li>
            <code>conversation.chat.completed</code> 表示对话任务完成；同时处理{' '}
            <code>conversation.chat.failed</code>、
            <code>conversation.error</code> 等异常事件。
          </li>
        </ul>
        <p>
          继续追问时，在原 POST 请求体中增加 <code>conversation_id</code>
          ，并保持同一业务用户的 <code>user_id</code>。新对话可不传会话
          ID，由平台创建；不同业务用户分别维护会话，不能混用。
        </p>
        <CodeExample
          title="连续对话请求体"
          testId="api-guide.follow-up"
          value={JSON.stringify(
            {
              ...guideChatBody(botId, '请补充说明。'),
              conversation_id: 'CONVERSATION_ID',
            },
            null,
            2,
          )}
        />
      </Section>
      <Section id="faq" title="5. 常见问题">
        <dl>
          <dt>返回 HTTP 200，是否代表成功？</dt>
          <dd>
            还需检查 JSON 业务码 <code>code</code>；非 0
            时处理错误。流式调用同时检查错误事件，不能仅按 HTTP 状态判断。
          </dd>
          <dt>鉴权失败或权限不足</dt>
          <dd>
            检查是否使用平台 API 令牌、Bearer
            格式、有效期和账号状态，并确认令牌所属账号与智能体的归属及访问权限。永久令牌仍可被删除，账号停用也会拦截调用。
          </dd>
          <dt>找不到智能体或调用了旧配置</dt>
          <dd>
            核对 <code>bot_id</code> 和 API 发布渠道。编辑器预览使用草稿；外部
            API 使用发布版本，修改后应重新发布。
          </dd>
          <dt>地址不正确、404 或出现额外登录框</dt>
          <dd>
            使用本页显示的当前接口前缀。当前子路径部署须保留{' '}
            <code>/agent/</code>，不要改用域名根路径、官方云平台 API
            地址或内部预览接口。
          </dd>
          <dt>接口返回了信息，却没有取得回答</dt>
          <dd>
            非流式提交先获取任务 ID，完成后再读消息列表。按当前版本从{' '}
            <code>Messages</code> 提取回答，不是从 <code>data</code>{' '}
            提取消息数组。
          </dd>
        </dl>
      </Section>
    </div>
  );
};
