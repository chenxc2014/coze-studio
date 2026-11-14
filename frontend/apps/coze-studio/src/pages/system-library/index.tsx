import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { Modal, Button } from '@coze-arch/coze-design';
import { kinds, request, type Resource, type Listing, type Field } from './api';
import './system-library.less';

function Fields({ fields }: { fields: Field[] }) {
  return fields.length ? (
    <div className="sl-table-wrap">
      <table>
        <thead>
          <tr>
            <th>参数</th>
            <th>类型</th>
            <th>必填</th>
            <th>说明</th>
          </tr>
        </thead>
        <tbody>
          {fields.map(field => (
            <tr key={field.name}>
              <td>{field.name}</td>
              <td>{field.type}</td>
              <td>{field.required ? '是' : '否'}</td>
              <td>{field.description || '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  ) : (
    <p className="sl-muted">无需输入参数。</p>
  );
}

function ResourceDetail({
  kind,
  id,
  onClose,
}: {
  kind: string;
  id: string;
  onClose: () => void;
}) {
  const [item, setItem] = useState<Resource>();
  const [error, setError] = useState('');
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    request<Resource>('detail', { kind, id }, controller.signal)
      .then(setItem)
      .catch(e => {
        if (!controller.signal.aborted)
          setError(e instanceof Error ? e.message : '读取失败');
      });
    return () => controller.abort();
  }, [kind, id]);
  return (
    <Modal
      visible
      title={item?.name || '系统资源详情'}
      onCancel={onClose}
      width={850}
      footer={<Button onClick={onClose}>关闭</Button>}
      className="sl-detail"
      data-testid="system-resource-detail"
    >
      {error ? (
        <p role="alert" className="sl-error">
          {error}
        </p>
      ) : !item ? (
        <p className="sl-muted">正在读取资源…</p>
      ) : (
        <>
          <div className="sl-meta">
            <span>{kinds[item.kind]}</span>
            <span>{item.origin === 'builtin' ? '平台内置' : '管理员发布'}</span>
            <span>全平台共享 · 只读</span>
            <span>{item.version || '当前版本'}</span>
          </div>
          <p className="sl-description">{item.description || '暂无描述'}</p>
          <section className="sl-usage">
            <h3>如何使用</h3>
            <p>{item.usage}</p>
            {item.notice && <p className="sl-muted">{item.notice}</p>}
          </section>
          {item.fields && (
            <>
              <h3>参数结构</h3>
              <Fields fields={item.fields} />
            </>
          )}
          {item.tools && (
            <>
              <h3>可用工具（{item.tools.length}）</h3>
              {item.tools.map(tool => (
                <details key={tool.id} className="sl-tool">
                  <summary>
                    {tool.name}
                    <span>{tool.description}</span>
                  </summary>
                  <Fields fields={tool.fields || []} />
                </details>
              ))}
            </>
          )}
          {item.content && (
            <>
              <div className="sl-content-heading">
                <h3>提示词内容</h3>
                <Button
                  onClick={async () => {
                    try {
                      await navigator.clipboard.writeText(item.content || '');
                      setCopied(true);
                    } catch {
                      setError('复制失败，请在下方选中文本后复制');
                    }
                  }}
                >
                  {copied ? '已复制' : '复制内容'}
                </Button>
              </div>
              <pre>{item.content}</pre>
            </>
          )}
          <p className="sl-muted">
            资源 ID：{item.id}
            {item.published_at
              ? ` · 最近发布：${new Date(item.published_at).toLocaleString('zh-CN', { hour12: false })}`
              : ''}
          </p>
        </>
      )}
    </Modal>
  );
}

export default function SystemLibraryPage() {
  const [params, setParams] = useSearchParams();
  const kind = Object.hasOwn(kinds, params.get('kind') || 'all')
    ? params.get('kind') || 'all'
    : 'all';
  const [search, setSearch] = useState('');
  const [query, setQuery] = useState('');
  const [page, setPage] = useState(1);
  const [revision, setRevision] = useState(0);
  const [data, setData] = useState<Listing>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [selected, setSelected] = useState<Resource>();
  useEffect(() => {
    const timer = setTimeout(() => {
      setQuery(search);
      setPage(1);
    }, 250);
    return () => clearTimeout(timer);
  }, [search]);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    request<Listing>(
      'list',
      { kind, q: query, page: String(page), page_size: '15' },
      controller.signal,
    )
      .then(value => {
        setData(value);
        setError('');
      })
      .catch(e => {
        if (!controller.signal.aborted)
          setError(e instanceof Error ? e.message : '读取失败');
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [kind, query, page, revision]);
  const choose = (value: string) => {
    setPage(1);
    setParams({ kind: value });
  };
  return (
    <div className="sl-page" data-testid="system-library-page">
      <header className="sl-heading">
        <div>
          <h1>资源库-系统</h1>
          <p>
            平台提供、可直接使用且无需厂商额外授权的资源，所有用户、所有项目均可引用
          </p>
        </div>
        <span className="sl-readonly">全平台共享 · 只读</span>
      </header>
      <nav className="sl-types" aria-label="资源类型">
        {Object.entries(kinds).map(([value, label]) => (
          <button
            key={value}
            className={kind === value ? 'is-active' : ''}
            aria-pressed={kind === value}
            onClick={() => choose(value)}
          >
            {label}
            <span>{data?.counts[value] ?? 0}</span>
          </button>
        ))}
      </nav>
      <div className="sl-toolbar">
        <label>
          <span>搜索系统资源</span>
          <input
            value={search}
            onChange={e => setSearch(e.target.value)}
            placeholder="搜索资源名称或描述"
            maxLength={100}
            type="search"
          />
        </label>
        <Button
          onClick={() => setRevision(value => value + 1)}
          disabled={loading}
        >
          刷新
        </Button>
        <span className="sl-muted">共 {data?.total ?? 0} 个资源</span>
      </div>
      <div className="sl-banner">
        在智能体、工作流的资源选择窗口中选择“资源库-系统”即可引用。第三方工具仍从独立的“搜索工具”入口选择；此处仅展示平台发布维护、无需厂商额外授权的资源。
      </div>
      {error && (
        <p role="alert" className="sl-error">
          {error}
        </p>
      )}
      {loading ? (
        <div className="sl-empty">正在读取系统资源…</div>
      ) : !data?.resources.length ? (
        <div className="sl-empty">
          <div className="sl-empty-symbol">▧</div>
          <h2>{query ? '未找到匹配的系统资源' : '暂无这类系统资源'}</h2>
          <p>
            {query
              ? '请调整搜索内容或切换资源类型。'
              : '管理员发布后会在这里展示，所有用户都可以选择使用。'}
          </p>
        </div>
      ) : (
        <div className="sl-grid">
          {data.resources.map(item => (
            <button
              className="sl-card"
              key={`${item.kind}:${item.id}`}
              onClick={() => setSelected(item)}
            >
              <div className="sl-card-top">
                <span className={`sl-icon sl-icon-${item.kind}`}>
                  {item.name.slice(0, 1)}
                </span>
                <span className="sl-origin">
                  {item.origin === 'builtin' ? '平台内置' : '管理员发布'}
                </span>
              </div>
              <h2>{item.name}</h2>
              <p>{item.description || '暂无描述'}</p>
              <footer>
                <span>{kinds[item.kind]} · 共享引用</span>
                <span>查看详情 →</span>
              </footer>
            </button>
          ))}
        </div>
      )}
      <div className="sl-pagination">
        <span>
          第 {page} / {Math.max(1, Math.ceil((data?.total || 0) / 15))} 页
        </span>
        <Button
          disabled={page === 1 || loading}
          onClick={() => setPage(value => value - 1)}
        >
          上一页
        </Button>
        <Button
          disabled={loading || page * 15 >= (data?.total || 0)}
          onClick={() => setPage(value => value + 1)}
        >
          下一页
        </Button>
      </div>
      {selected && (
        <ResourceDetail
          kind={selected.kind}
          id={selected.id}
          onClose={() => setSelected(undefined)}
        />
      )}
    </div>
  );
}
