import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { useParams, useSearchParams } from 'react-router-dom';
import { getAppBasePath } from '@coze-arch/web-context';

import {
  active,
  api,
  download,
  key,
  pretty,
  statuses,
  time,
  typeName,
  type Detail,
  type Field,
  type Preview,
  type Run,
  type Task,
  type Workflow,
} from './api';
import './task-center.less';

type Kind = 'batch' | 'async';
const errorText = (error: unknown) =>
  error instanceof Error ? error.message : '操作未完成，请稍后重试';

function Icon({ name = 'clock' }: { name?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {name === 'clock' ? (
        <>
          <circle cx="12" cy="12" r="8.5" />
          <path d="M12 7v5l3 2M6 2 3 5m15-3 3 3" />
        </>
      ) : name === 'upload' ? (
        <>
          <path d="M12 16V3m-4 4 4-4 4 4M4 15v5h16v-5" />
        </>
      ) : name === 'run' ? (
        <>
          <path d="m9 6 10 6-10 6z" />
          <path d="M4 5v14" />
        </>
      ) : name === 'result' ? (
        <>
          <rect x="4" y="3" width="16" height="18" rx="3" />
          <path d="M8 8h8M8 12h8m-8 4h4" />
        </>
      ) : (
        <>
          <rect x="3" y="3" width="7" height="6" rx="1.5" />
          <rect x="14" y="15" width="7" height="6" rx="1.5" />
          <path d="M6 9v8h8m-4-11h8v9" />
        </>
      )}
    </svg>
  );
}

function Dialog({
  title,
  children,
  footer,
  onClose,
  wide = false,
  busy = false,
}: {
  title: string;
  children: ReactNode;
  footer?: ReactNode;
  onClose: () => void;
  wide?: boolean;
  busy?: boolean;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const close = useRef(onClose);
  close.current = onClose;
  useEffect(() => {
    const before = document.activeElement as HTMLElement | null;
    const previous = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    ref.current?.focus();
    const handle = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !busy) close.current();
      if (event.key === 'Tab') {
        const nodes = Array.from(
          ref.current?.querySelectorAll<HTMLElement>(
            'button:not(:disabled),input:not(:disabled),select:not(:disabled),textarea:not(:disabled),a[href],[tabindex="0"]',
          ) || [],
        ).filter(node => node.offsetParent !== null);
        if (!nodes.length) {
          event.preventDefault();
          return;
        }
        if (
          event.shiftKey &&
          (document.activeElement === nodes[0] ||
            document.activeElement === ref.current)
        ) {
          event.preventDefault();
          nodes.at(-1)?.focus();
        }
        if (!event.shiftKey && document.activeElement === nodes.at(-1)) {
          event.preventDefault();
          nodes[0]?.focus();
        }
      }
    };
    document.addEventListener('keydown', handle);
    return () => {
      document.body.style.overflow = previous;
      document.removeEventListener('keydown', handle);
      if (before?.isConnected) before.focus();
    };
  }, [busy]);
  return (
    <div className="tc-overlay">
      <div
        ref={ref}
        className={`tc-dialog ${wide ? 'tc-dialog-wide' : ''}`}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        tabIndex={-1}
      >
        <header>
          <h2>{title}</h2>
          <button
            className="tc-close"
            aria-label="关闭窗口"
            onClick={onClose}
            disabled={busy}
          >
            ×
          </button>
        </header>
        <div className="tc-dialog-body">{children}</div>
        {footer && <footer>{footer}</footer>}
      </div>
    </div>
  );
}

function Badge({ status }: { status: string }) {
  return (
    <span className={`tc-badge tc-status-${status}`}>
      {statuses[status] || status}
    </span>
  );
}
function ErrorBox({ message }: { message: string }) {
  return message ? (
    <div className="tc-error" role="alert">
      {message}
    </div>
  ) : null;
}
function Fields({ fields }: { fields: Field[] }) {
  return fields.length ? (
    <div className="tc-table-wrap">
      <table className="tc-table tc-field-table">
        <thead>
          <tr>
            <th>参数名称</th>
            <th>类型</th>
            <th>必填</th>
            <th>说明</th>
          </tr>
        </thead>
        <tbody>
          {fields.map(field => (
            <tr key={field.name}>
              <td>{field.name}</td>
              <td>{typeName(field.type)}</td>
              <td>{field.required ? '是' : '否'}</td>
              <td>{field.description || '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  ) : (
    <p className="tc-muted">这个工作流无需输入参数，可直接创建异步任务。</p>
  );
}

function CreateDialog({
  space,
  kind,
  onClose,
  onCreated,
}: {
  space: string;
  kind: Kind;
  onClose: () => void;
  onCreated: (task: Task) => void;
}) {
  const [step, setStep] = useState(0);
  const [name, setName] = useState('');
  const [workflows, setWorkflows] = useState<Workflow[]>([]);
  const [workflowID, setWorkflowID] = useState('');
  const [version, setVersion] = useState('');
  const [schema, setSchema] = useState<Workflow>();
  const [workflowQuery, setWorkflowQuery] = useState('');
  const [concurrency, setConcurrency] = useState(2);
  const [timeout, setTimeoutValue] = useState(3600);
  const [loading, setLoading] = useState(false);
  const [schemaLoading, setSchemaLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [fileName, setFileName] = useState('');
  const [csv, setCSV] = useState('');
  const [preview, setPreview] = useState<Preview>();
  const [previewBusy, setPreviewBusy] = useState(false);
  const requestKey = useRef(key());
  useEffect(() => {
    const controller = new AbortController();
    const timer = setTimeout(() => {
      setLoading(true);
      api<{ workflows: Workflow[] }>(
        'workflows',
        { space_id: space, q: workflowQuery },
        undefined,
        controller.signal,
      )
        .then(value => setWorkflows(value.workflows))
        .catch(e => {
          if (!controller.signal.aborted) setError(errorText(e));
        })
        .finally(() => {
          if (!controller.signal.aborted) setLoading(false);
        });
    }, 200);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [space, workflowQuery]);
  useEffect(() => {
    setCSV('');
    setFileName('');
    setPreview(undefined);
    setSchema(undefined);
    if (!workflowID) return;
    const controller = new AbortController();
    setSchemaLoading(true);
    setError('');
    api<Workflow>(
      'schema',
      { space_id: space, workflow_id: workflowID, version },
      undefined,
      controller.signal,
    )
      .then(value => setSchema(value))
      .catch(e => {
        if (!controller.signal.aborted) setError(errorText(e));
      })
      .finally(() => {
        if (!controller.signal.aborted) setSchemaLoading(false);
      });
    return () => controller.abort();
  }, [space, workflowID, version]);
  const chooseWorkflow = (id: string) => {
    setWorkflowID(id);
    setVersion('');
  };
  const upload = async (file?: File) => {
    setError('');
    setPreview(undefined);
    setCSV('');
    setFileName('');
    if (!file || !schema) return;
    if (!/\.csv$/i.test(file.name) || file.size > 2 * 1024 * 1024) {
      setError('请选择 2 MiB 以内的 CSV 文件。');
      return;
    }
    setPreviewBusy(true);
    try {
      const bytes = new Uint8Array(await file.arrayBuffer());
      let binary = '';
      for (let i = 0; i < bytes.length; i += 0x8000)
        binary += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
      const encoded = btoa(binary);
      const value = await api<Preview>(
        'preview',
        {},
        {
          space_id: space,
          workflow_id: workflowID,
          version: schema.version,
          csv_base64: encoded,
        },
      );
      setCSV(encoded);
      setPreview(value);
      setFileName(file.name);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setPreviewBusy(false);
    }
  };
  const submit = async () => {
    if (!schema) return;
    setBusy(true);
    setError('');
    try {
      const task = await api<Task>(
        'create',
        {},
        {
          space_id: space,
          name,
          kind,
          workflow_id: workflowID,
          version: schema.version,
          concurrency: kind === 'async' ? 1 : concurrency,
          timeout_seconds: timeout,
          csv_base64: csv,
          request_key: requestKey.current,
        },
      );
      onCreated(task);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  };
  const next = () => {
    setError('');
    if (!name.trim() || !schema) {
      setError('请填写任务名称并选择已发布的工作流。');
      return;
    }
    if (kind === 'batch' && !schema.fields?.length) {
      setError('这个工作流没有输入参数，请改用异步任务。');
      return;
    }
    setStep(step + 1);
  };
  const versions = schema
    ? Array.from(new Set([schema.version, ...(schema.versions || [])]))
    : [];
  return (
    <Dialog
      title={kind === 'batch' ? '创建批量任务' : '创建异步任务'}
      onClose={onClose}
      busy={busy || previewBusy}
      wide
      footer={
        <>
          <button
            className="tc-button"
            onClick={step ? () => setStep(step - 1) : onClose}
            disabled={busy}
          >
            {step ? '上一步' : '取消'}
          </button>
          {step < 2 ? (
            <button
              className="tc-button tc-primary"
              onClick={next}
              disabled={
                loading ||
                schemaLoading ||
                previewBusy ||
                !schema ||
                (step === 1 && kind === 'batch' && !preview)
              }
            >
              下一步
            </button>
          ) : (
            <button
              className="tc-button tc-primary"
              onClick={submit}
              disabled={busy}
            >
              {busy
                ? '正在提交…'
                : kind === 'batch'
                  ? '创建并执行'
                  : '创建异步任务'}
            </button>
          )}
        </>
      }
    >
      <ol className="tc-steps">
        {[
          '选择任务对象',
          kind === 'batch' ? '上传任务数据' : '配置任务',
          '确认创建',
        ].map((text, i) => (
          <li key={text} className={i <= step ? 'is-active' : ''}>
            <span>{i + 1}</span>
            {text}
          </li>
        ))}
      </ol>
      <ErrorBox message={error} />
      {step === 0 && (
        <div className="tc-form">
          <label>
            任务名称 <em>*</em>
            <input
              value={name}
              onChange={event => setName(event.target.value)}
              maxLength={100}
              placeholder="给任务起一个便于识别的名称"
            />
          </label>
          <label>
            查找工作流
            <input
              type="search"
              value={workflowQuery}
              onChange={event => setWorkflowQuery(event.target.value)}
              placeholder="按名称搜索当前空间中已发布的工作流"
            />
          </label>
          <label>
            任务对象 <em>*</em>
            <select
              value={workflowID}
              onChange={event => chooseWorkflow(event.target.value)}
              disabled={loading}
            >
              <option value="">
                {loading ? '正在加载…' : '请选择已发布的工作流'}
              </option>
              {[
                ['资源库工作流', false],
                ['应用工作流', true],
              ].map(([label, app]) => (
                <optgroup key={String(label)} label={String(label)}>
                  {workflows
                    .filter(workflow => Boolean(workflow.app_id) === app)
                    .map(workflow => (
                      <option key={workflow.id} value={workflow.id}>
                        {workflow.name}
                      </option>
                    ))}
                </optgroup>
              ))}
            </select>
          </label>
          {!loading && !workflows.length && (
            <p className="tc-hint">
              当前没有可用的已发布工作流。请先在资源库或应用中创建并发布普通工作流，再回来选择。
            </p>
          )}
          {schema && (
            <label>
              固定执行版本
              <select
                value={version || schema.version}
                onChange={event => setVersion(event.target.value)}
              >
                {versions.map(value => (
                  <option key={value} value={value}>
                    {value}
                  </option>
                ))}
              </select>
              <small>
                任务固定使用该版本，后续发布新版本不会改变已创建任务。
              </small>
            </label>
          )}
          {schemaLoading && <p className="tc-muted">正在读取参数结构…</p>}
        </div>
      )}
      {step === 1 && schema && (
        <>
          <h3>工作流参数</h3>
          <Fields fields={schema.fields || []} />
          {kind === 'batch' ? (
            <>
              <div className="tc-upload-heading">
                <h3>导入参数配置</h3>
                <button
                  className="tc-button"
                  onClick={() =>
                    download('template', {
                      space_id: space,
                      workflow_id: workflowID,
                      version: schema.version,
                    }).catch(e => setError(errorText(e)))
                  }
                >
                  下载 CSV 模板
                </button>
              </div>
              <label className="tc-upload">
                <Icon name="upload" />
                <strong>
                  {previewBusy
                    ? '正在校验文件…'
                    : fileName || '选择 CSV 参数文件'}
                </strong>
                <span>支持 UTF-8 / GB18030，最多 1000 行、2 MiB</span>
                <input
                  type="file"
                  accept=".csv,text/csv"
                  disabled={previewBusy}
                  onChange={event => upload(event.target.files?.[0])}
                />
              </label>
              {preview && (
                <div className="tc-preview">
                  <p>
                    <strong>校验通过，共 {preview.total} 行</strong>
                    <span className="tc-muted">
                      {' '}
                      · 预览前 {preview.rows.length} 行
                    </span>
                  </p>
                  <div className="tc-table-wrap">
                    <table className="tc-table">
                      <thead>
                        <tr>
                          {preview.fields.map(field => (
                            <th key={field.name}>{field.name}</th>
                          ))}
                        </tr>
                      </thead>
                      <tbody>
                        {preview.rows.map((row, i) => (
                          <tr key={i}>
                            {row.map((value, j) => (
                              <td key={j}>
                                <span className="tc-cell" title={value}>
                                  {value}
                                </span>
                              </td>
                            ))}
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>
              )}
            </>
          ) : (
            <div className="tc-note">
              创建后，你可以多次填写参数并提交后台执行。每次提交都会生成独立的执行记录；关闭页面后任务继续运行。
            </div>
          )}
          <div className="tc-form tc-form-columns">
            {kind === 'batch' && (
              <label>
                并行执行数
                <select
                  value={concurrency}
                  onChange={event => setConcurrency(Number(event.target.value))}
                >
                  {[1, 2, 3, 4].map(value => (
                    <option value={value} key={value}>
                      {value} 条
                    </option>
                  ))}
                </select>
              </label>
            )}
            <label>
              单条最长运行时间
              <select
                value={timeout}
                onChange={event => setTimeoutValue(Number(event.target.value))}
              >
                {[
                  [600, '10 分钟'],
                  [3600, '1 小时'],
                  [21600, '6 小时'],
                  [86400, '24 小时'],
                ].map(([value, label]) => (
                  <option value={value} key={value}>
                    {label}
                  </option>
                ))}
              </select>
              <small>也受工作流节点自身的超时限制。</small>
            </label>
          </div>
        </>
      )}
      {step === 2 && schema && (
        <>
          <div className="tc-review">
            <Icon name="workflow" />
            <h3>{name}</h3>
            <dl>
              <dt>工作流</dt>
              <dd>{schema.name}</dd>
              <dt>版本</dt>
              <dd>{schema.version}</dd>
              <dt>任务类型</dt>
              <dd>{kind === 'batch' ? '批量任务' : '异步任务'}</dd>
              {kind === 'batch' && (
                <>
                  <dt>执行条数</dt>
                  <dd>{preview?.total} 条，每行执行一次</dd>
                  <dt>并行执行数</dt>
                  <dd>{concurrency}</dd>
                </>
              )}
              <dt>单条超时</dt>
              <dd>{timeout / 60} 分钟</dd>
            </dl>
          </div>
          <p className="tc-hint">
            {kind === 'batch'
              ? '提交后开始后台执行，可能产生所选模型和插件的调用费用。'
              : '本次只创建任务配置；填写参数并执行后，才会运行工作流。'}
          </p>
        </>
      )}
    </Dialog>
  );
}

function ExecuteDialog({
  space,
  task,
  onClose,
  onExecuted,
}: {
  space: string;
  task: Task;
  onClose: () => void;
  onExecuted: (run: Run) => void;
}) {
  const [fields, setFields] = useState<Field[]>(task.fields || []);
  const [loading, setLoading] = useState(!task.fields);
  const [values, setValues] = useState<Record<string, string>>({});
  const [jsonMode, setJSONMode] = useState(false);
  const [raw, setRaw] = useState('{}');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const requestKey = useRef(key());
  useEffect(() => {
    if (task.fields) return;
    const controller = new AbortController();
    api<Detail>(
      'detail',
      { space_id: space, task_id: task.id },
      undefined,
      controller.signal,
    )
      .then(data => setFields(data.task.fields || []))
      .catch(e => {
        if (!controller.signal.aborted) setError(errorText(e));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [space, task]);
  const submit = async () => {
    setBusy(true);
    setError('');
    try {
      const input = jsonMode
        ? raw
        : JSON.stringify(
            Object.fromEntries(
              Object.entries(values).filter(([, value]) => value !== ''),
            ),
          );
      const run = await api<Run>(
        'execute',
        {},
        {
          space_id: space,
          task_id: task.id,
          input_json: input,
          request_key: requestKey.current,
        },
      );
      onExecuted(run);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Dialog
      title="执行异步任务"
      onClose={onClose}
      busy={busy}
      footer={
        <>
          <button className="tc-button" onClick={onClose} disabled={busy}>
            取消
          </button>
          <button
            className="tc-button tc-primary"
            onClick={submit}
            disabled={busy || loading}
          >
            {busy ? '正在提交…' : '提交后台执行'}
          </button>
        </>
      }
    >
      <div className="tc-note">
        <strong>{task.name}</strong>
        <br />
        {task.workflow_name} · {task.version}
      </div>
      <ErrorBox message={error} />
      {loading ? (
        <p className="tc-muted">正在读取参数…</p>
      ) : (
        <>
          <label className="tc-check">
            <input
              type="checkbox"
              checked={jsonMode}
              onChange={event => setJSONMode(event.target.checked)}
            />
            使用 JSON 参数
          </label>
          {jsonMode ? (
            <label className="tc-json-label">
              输入参数（JSON 对象）
              <textarea
                value={raw}
                onChange={event => setRaw(event.target.value)}
                rows={10}
                spellCheck={false}
              />
            </label>
          ) : (
            <div className="tc-form">
              {fields.map(field => (
                <label key={field.name}>
                  {field.name} {field.required && <em>*</em>}
                  <span className="tc-field-type">{typeName(field.type)}</span>
                  {field.type === 'boolean' ? (
                    <select
                      value={values[field.name] || ''}
                      onChange={event =>
                        setValues({
                          ...values,
                          [field.name]: event.target.value,
                        })
                      }
                    >
                      <option value="">未设置</option>
                      <option value="true">true</option>
                      <option value="false">false</option>
                    </select>
                  ) : ['object', 'array', 'list'].includes(field.type) ? (
                    <textarea
                      rows={3}
                      value={values[field.name] || ''}
                      onChange={event =>
                        setValues({
                          ...values,
                          [field.name]: event.target.value,
                        })
                      }
                      placeholder={
                        field.type === 'object'
                          ? '{"属性":"值"}'
                          : '["值1","值2"]'
                      }
                      spellCheck={false}
                    />
                  ) : (
                    <input
                      value={values[field.name] || ''}
                      onChange={event =>
                        setValues({
                          ...values,
                          [field.name]: event.target.value,
                        })
                      }
                      placeholder={`请输入${typeName(field.type)}`}
                    />
                  )}
                  {field.description && <small>{field.description}</small>}
                </label>
              ))}
              {!fields.length && (
                <p className="tc-muted">无需填写参数，可直接执行。</p>
              )}
            </div>
          )}
        </>
      )}
      <p className="tc-hint">提交成功后可关闭页面，在执行记录中查看结果。</p>
    </Dialog>
  );
}

function DetailDialog({
  space,
  id,
  runID,
  actor,
  onClose,
  onSelectRun,
  onExecute,
  onChanged,
}: {
  space: string;
  id: string;
  runID: string;
  actor: string;
  onClose: () => void;
  onSelectRun: (id: string) => void;
  onExecute: (task: Task) => void;
  onChanged: () => void;
}) {
  const [historyPage, setHistoryPage] = useState(1);
  const [detail, setDetail] = useState<Detail>();
  const [error, setError] = useState('');
  const [page, setPage] = useState(1);
  const [filter, setFilter] = useState('');
  const [expanded, setExpanded] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [revision, setRevision] = useState(0);
  const [confirm, setConfirm] = useState<'cancel' | 'retry'>();
  const mutationKey = useRef(key());
  useEffect(() => {
    setPage(1);
    setExpanded(undefined);
  }, [runID, id]);
  useEffect(() => {
    let stopped = false;
    const controller = new AbortController();
    const load = () =>
      api<Detail>(
        'detail',
        {
          space_id: space,
          task_id: id,
          run_id: runID,
          page: String(page),
          page_size: '20',
          status: filter,
          history_page: String(historyPage),
        },
        undefined,
        controller.signal,
      )
        .then(data => {
          if (!stopped) {
            setDetail(data);
            setError('');
          }
        })
        .catch(e => {
          if (!stopped) setError(errorText(e));
        });
    load();
    const timer = setInterval(load, 4000);
    return () => {
      stopped = true;
      controller.abort();
      clearInterval(timer);
    };
  }, [space, id, runID, page, filter, revision, historyPage]);
  const task = detail?.task;
  const run = detail?.run;
  const canManage = task?.owner_id === actor;
  const completed = run
    ? run.succeeded + run.failed + run.cancelled + run.interrupted
    : 0;
  const mutate = async (action: 'cancel' | 'retry') => {
    if (!run) return;
    setBusy(true);
    setError('');
    try {
      const value = await api<Run>(
        action,
        {},
        {
          space_id: space,
          task_id: id,
          run_id: run.id,
          request_key: mutationKey.current,
        },
      );
      setConfirm(undefined);
      if (action === 'retry') onSelectRun(value.id);
      setRevision(value => value + 1);
      onChanged();
      mutationKey.current = key();
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Dialog
      title={task?.name || '任务详情'}
      onClose={onClose}
      wide
      busy={busy}
      footer={
        <>
          <button className="tc-button" onClick={onClose}>
            关闭
          </button>
          {canManage && task?.kind === 'async' && (
            <button
              className="tc-button tc-primary"
              onClick={() => onExecute(task)}
            >
              执行任务
            </button>
          )}
          {canManage && run && active(run.status) && (
            <button
              className="tc-button tc-danger"
              disabled={busy || run.status === 'cancelling'}
              onClick={() => setConfirm('cancel')}
            >
              取消执行
            </button>
          )}
          {canManage &&
            run &&
            !active(run.status) &&
            run.failed + run.interrupted + run.cancelled > 0 && (
              <button
                className="tc-button"
                disabled={busy}
                onClick={() => setConfirm('retry')}
              >
                重试未成功项
              </button>
            )}
          {run && !active(run.status) && (
            <button
              className="tc-button"
              onClick={() =>
                download('export', {
                  space_id: space,
                  task_id: id,
                  run_id: run.id,
                }).catch(e => setError(errorText(e)))
              }
            >
              导出 CSV
            </button>
          )}
        </>
      }
    >
      <ErrorBox message={error} />
      {!task ? (
        <p className="tc-muted">正在加载任务…</p>
      ) : (
        <>
          <div className="tc-detail-meta">
            <span>{task.kind === 'batch' ? '批量任务' : '异步任务'}</span>
            <span>{task.workflow_name}</span>
            <span>版本 {task.version}</span>
            <span>创建于 {time(task.created_at)}</span>
          </div>
          {confirm && (
            <div className="tc-confirm" role="alert">
              <strong>
                {confirm === 'cancel'
                  ? '确认取消本次执行？'
                  : '确认重新执行未成功的行？'}
              </strong>
              <p>
                {confirm === 'cancel'
                  ? '停止后续行并取消正在运行的工作流。已经产生的业务操作不会回滚。'
                  : '成功行不会重复执行。中断行的结果未确认，如工作流含写入操作，请先核对。'}
              </p>
              <button
                className="tc-button"
                disabled={busy}
                onClick={() => setConfirm(undefined)}
              >
                返回
              </button>
              <button
                className="tc-button tc-primary"
                disabled={busy}
                onClick={() => mutate(confirm)}
              >
                {busy ? '处理中…' : '确认'}
              </button>
            </div>
          )}
          {!run ? (
            <div className="tc-no-runs">
              <Icon />
              <h3>还没有执行记录</h3>
              <p>填写工作流参数，提交第一次后台执行。</p>
              <Fields fields={task.fields || []} />
            </div>
          ) : (
            <>
              <div className="tc-run-selector">
                <label>
                  执行记录
                  <select
                    value={run.id}
                    onChange={event => {
                      onSelectRun(event.target.value);
                      setConfirm(undefined);
                    }}
                  >
                    {[
                      ...(!detail.runs.some(value => value.id === run.id)
                        ? [run]
                        : []),
                      ...detail.runs,
                    ].map(value => (
                      <option key={value.id} value={value.id}>
                        {time(value.created_at)} · {statuses[value.status]} ·{' '}
                        {value.total} 条
                        {value.retry_of !== '0' ? ' · 重试' : ''}
                      </option>
                    ))}
                  </select>
                </label>
                <span className="tc-muted">共 {detail.runs_total} 次</span>
                <button
                  className="tc-link"
                  disabled={historyPage <= 1}
                  onClick={() => setHistoryPage(value => value - 1)}
                >
                  上一组
                </button>
                <button
                  className="tc-link"
                  disabled={historyPage * 20 >= detail.runs_total}
                  onClick={() => setHistoryPage(value => value + 1)}
                >
                  下一组
                </button>
                <Badge status={run.status} />
              </div>
              <div className="tc-metrics">
                <div>
                  <span>总数</span>
                  <strong>{run.total}</strong>
                </div>
                <div>
                  <span>已完成</span>
                  <strong>{completed}</strong>
                </div>
                <div>
                  <span>成功</span>
                  <strong className="tc-success">{run.succeeded}</strong>
                </div>
                <div>
                  <span>失败 / 中断</span>
                  <strong className="tc-failed">
                    {run.failed + run.interrupted}
                  </strong>
                </div>
              </div>
              <div
                className="tc-progress"
                role="progressbar"
                aria-label="任务完成进度"
                aria-valuemin={0}
                aria-valuemax={run.total}
                aria-valuenow={completed}
              >
                <span
                  style={{
                    width: `${run.total ? (completed / run.total) * 100 : 0}%`,
                  }}
                />
              </div>
              {run.status === 'interrupted' && (
                <div className="tc-note">
                  部分行执行被中断，结果未确认。尚未开始的行会继续处理；中断行不会自动重复执行。
                </div>
              )}
              <div className="tc-results-toolbar">
                <h3>执行结果</h3>
                <select
                  aria-label="筛选执行行状态"
                  value={filter}
                  onChange={event => {
                    setFilter(event.target.value);
                    setPage(1);
                  }}
                >
                  <option value="">所有状态</option>
                  {[
                    'queued',
                    'running',
                    'succeeded',
                    'failed',
                    'cancelled',
                    'interrupted',
                  ].map(status => (
                    <option key={status} value={status}>
                      {statuses[status]}
                    </option>
                  ))}
                </select>
              </div>
              <div className="tc-table-wrap">
                <table className="tc-table tc-results-table">
                  <thead>
                    <tr>
                      <th>行号</th>
                      <th>状态</th>
                      <th>输入参数</th>
                      <th>结果 / 错误</th>
                      <th>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {detail.items.map(item => (
                      <tr key={item.id}>
                        <td>{item.position}</td>
                        <td>
                          <Badge status={item.status} />
                        </td>
                        <td>
                          <span className="tc-cell" title={item.input_json}>
                            {item.input_json}
                          </span>
                        </td>
                        <td>
                          <span
                            className={`tc-cell ${item.error ? 'tc-failed' : ''}`}
                            title={item.error || item.output_json}
                          >
                            {item.error || item.output_json || '—'}
                          </span>
                        </td>
                        <td>
                          <button
                            className="tc-link"
                            onClick={() =>
                              setExpanded(
                                expanded === item.id ? undefined : item.id,
                              )
                            }
                          >
                            查看详情
                          </button>
                          {item.execution_id !== '0' && (
                            <a
                              className="tc-link"
                              href={`${getAppBasePath()}/work_flow?space_id=${encodeURIComponent(space)}&workflow_id=${encodeURIComponent(task.workflow_id)}&execute_id=${encodeURIComponent(item.execution_id)}&execute_mode=2`}
                              target="_blank"
                              rel="noopener noreferrer"
                            >
                              工作流日志
                            </a>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                {!detail.items.length && (
                  <p className="tc-table-empty">没有符合条件的执行行。</p>
                )}
              </div>
              {expanded &&
                detail.items
                  .filter(item => item.id === expanded)
                  .map(item => (
                    <section className="tc-item-detail" key={item.id}>
                      <div>
                        <h4>第 {item.position} 行 · 输入参数</h4>
                        <button
                          className="tc-link"
                          onClick={() => setExpanded(undefined)}
                        >
                          收起
                        </button>
                      </div>
                      <pre>{pretty(item.input_json)}</pre>
                      <h4>执行结果</h4>
                      <pre>{pretty(item.output_json)}</pre>
                      {item.error && <ErrorBox message={item.error} />}
                    </section>
                  ))}
              <Pagination
                page={page}
                size={20}
                total={detail.items_total}
                onChange={setPage}
              />
            </>
          )}
        </>
      )}
    </Dialog>
  );
}

function Pagination({
  page,
  size,
  total,
  onChange,
}: {
  page: number;
  size: number;
  total: number;
  onChange: (page: number) => void;
}) {
  return (
    <div className="tc-pagination">
      <span>共 {total} 条</span>
      <button
        className="tc-button"
        disabled={page <= 1}
        onClick={() => onChange(page - 1)}
      >
        上一页
      </button>
      <span>
        {page} / {Math.max(1, Math.ceil(total / size))}
      </span>
      <button
        className="tc-button"
        disabled={page * size >= total}
        onClick={() => onChange(page + 1)}
      >
        下一页
      </button>
    </div>
  );
}

export default function TaskCenterPage() {
  const { space_id: space = '' } = useParams();
  const [params, setParams] = useSearchParams();
  const kind: Kind = params.get('kind') === 'async' ? 'async' : 'batch';
  const taskID = params.get('task_id') || '';
  const runID = params.get('run_id') || '';
  const [tasks, setTasks] = useState<Task[]>([]);
  const [total, setTotal] = useState(0);
  const [actor, setActor] = useState('');
  const [loading, setLoading] = useState(true);
  const [listError, setListError] = useState('');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [page, setPage] = useState(1);
  const [query, setQuery] = useState('');
  const [search, setSearch] = useState('');
  const [mine, setMine] = useState('');
  const [status, setStatus] = useState('');
  const [revision, setRevision] = useState(0);
  const [create, setCreate] = useState<Kind>();
  const [execute, setExecute] = useState<Task>();
  const [remove, setRemove] = useState<Task>();
  const [help, setHelp] = useState(false);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    const timer = setTimeout(() => {
      setSearch(query);
      setPage(1);
    }, 250);
    return () => clearTimeout(timer);
  }, [query]);
  useEffect(() => {
    setPage(1);
    setStatus('');
    setMine('');
    setQuery('');
    setSearch('');
    setError('');
  }, [kind, space]);
  useEffect(() => {
    const controller = new AbortController();
    let stopped = false;
    setLoading(true);
    const load = () =>
      api<{ tasks: Task[]; total: number; current_user_id: string }>(
        'list',
        {
          space_id: space,
          kind,
          q: search,
          mine,
          status,
          page: String(page),
          page_size: '20',
        },
        undefined,
        controller.signal,
      )
        .then(value => {
          if (!stopped) {
            setTasks(value.tasks);
            setTotal(value.total);
            setActor(value.current_user_id);
            setListError('');
          }
        })
        .catch(e => {
          if (!stopped) setListError(errorText(e));
        })
        .finally(() => {
          if (!stopped) setLoading(false);
        });
    load();
    const timer = setInterval(load, 5000);
    return () => {
      stopped = true;
      controller.abort();
      clearInterval(timer);
    };
  }, [space, kind, search, mine, status, page, revision]);
  const changed = useCallback(() => setRevision(value => value + 1), []);
  const navigateTask = (id?: string, run?: string) => {
    const next = new URLSearchParams({ kind });
    if (id) next.set('task_id', id);
    if (run && run !== '0') next.set('run_id', run);
    setParams(next);
  };
  const stepCards =
    kind === 'batch'
      ? [
          [
            'workflow',
            '选择任务对象',
            '选择来自资源库或应用的已发布工作流，并固定执行版本',
          ],
          [
            'upload',
            '上传任务数据',
            '下载参数模板，上传 CSV，每一行对应一次工作流执行',
          ],
          ['run', '批量任务执行', '提交后在后台有序处理，关闭页面后继续运行'],
          [
            'result',
            '查看任务结果',
            '查看逐行结果与错误，重试未成功项，导出结果文件',
          ],
        ]
      : [
          [
            'workflow',
            '选择工作流',
            '创建可重复使用的异步任务，固定工作流与发布版本',
          ],
          [
            'upload',
            '填写运行参数',
            '每次执行前填写参数，支持文本、数字、对象和数组',
          ],
          ['run', '提交后台执行', '提交后立即返回，随时查看任务状态或取消执行'],
          [
            'result',
            '查看执行记录',
            '每次运行单独保存结果，可查看日志并导出文件',
          ],
        ];
  const deleteTask = async () => {
    if (!remove) return;
    setBusy(true);
    setError('');
    try {
      await api('delete', {}, { space_id: space, task_id: remove.id });
      setRemove(undefined);
      setNotice('任务及其执行记录已删除');
      changed();
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="tc-page" data-testid="task-center-page">
      <header className="tc-page-header">
        <div>
          <h1>任务中心</h1>
          <p>让工作流在后台持续完成任务</p>
        </div>
        <button
          className="tc-button tc-primary"
          onClick={() => {
            setCreate(kind);
            setError('');
          }}
        >
          ＋ 创建{kind === 'batch' ? '批量' : '异步'}任务
        </button>
      </header>
      <nav className="tc-tabs" aria-label="任务类型">
        {(['batch', 'async'] as const).map(value => (
          <button
            key={value}
            className={value === kind ? 'is-active' : ''}
            aria-current={value === kind ? 'page' : undefined}
            onClick={() => setParams({ kind: value })}
          >
            {value === 'batch' ? '批量任务' : '异步任务'}
          </button>
        ))}
      </nav>
      <div className="tc-toolbar">
        <select
          aria-label="筛选创建者"
          value={mine}
          onChange={event => {
            setMine(event.target.value);
            setPage(1);
          }}
        >
          <option value="">所有创建者</option>
          <option value="true">我创建的</option>
        </select>
        <select
          aria-label="筛选任务状态"
          value={status}
          onChange={event => {
            setStatus(event.target.value);
            setPage(1);
          }}
        >
          <option value="">所有状态</option>
          {[
            'not_started',
            'queued',
            'running',
            'completed',
            'partial',
            'failed',
            'cancelled',
            'interrupted',
          ].map(value => (
            <option value={value} key={value}>
              {statuses[value]}
            </option>
          ))}
        </select>
        <input
          type="search"
          aria-label="搜索任务"
          placeholder="搜索任务名称"
          value={query}
          onChange={event => setQuery(event.target.value)}
        />
        <button className="tc-link" onClick={() => setHelp(true)}>
          使用说明
        </button>
        <button className="tc-button" onClick={changed}>
          刷新
        </button>
      </div>
      <ErrorBox message={listError || error} />
      {notice && (
        <div className="tc-notice" role="status">
          {notice}
          <button
            className="tc-close"
            onClick={() => setNotice('')}
            aria-label="关闭提示"
          >
            ×
          </button>
        </div>
      )}
      {loading && !tasks.length ? (
        <div className="tc-loading">正在加载任务…</div>
      ) : !tasks.length && !listError ? (
        <section className="tc-empty">
          <div className="tc-empty-symbol">
            <Icon />
          </div>
          <h2>
            {search || mine || status
              ? '没有符合条件的任务'
              : kind === 'batch'
                ? '批量执行，让重复工作自动完成'
                : '异步执行，让工作流在后台运行'}
          </h2>
          <p>
            {kind === 'batch'
              ? '通过 CSV 配置多行工作流参数，统一执行并收集结果'
              : '保存工作流配置，每次提交生成一条独立的执行记录'}
          </p>
          <div className="tc-guide">
            {stepCards.map(([icon, title, text], index) => (
              <div className="tc-guide-card" key={title}>
                <div className="tc-guide-visual">
                  <Icon name={icon} />
                  <span>0{index + 1}</span>
                </div>
                <h3>{title}</h3>
                <p>{text}</p>
              </div>
            ))}
          </div>
          <button
            className="tc-button tc-primary"
            onClick={() => setCreate(kind)}
          >
            ＋ 创建{kind === 'batch' ? '批量' : '异步'}任务
          </button>
        </section>
      ) : tasks.length > 0 ? (
        <>
          <div className="tc-table-wrap tc-task-list">
            <table className="tc-table">
              <thead>
                <tr>
                  <th>任务名称</th>
                  <th>工作流 / 版本</th>
                  <th>状态</th>
                  <th>最近执行进度</th>
                  <th>创建时间</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {tasks.map(task => {
                  const run = task.latest_run;
                  const done = run
                    ? run.succeeded +
                      run.failed +
                      run.cancelled +
                      run.interrupted
                    : 0;
                  return (
                    <tr key={task.id}>
                      <td>
                        <button
                          className="tc-task-name"
                          onClick={() => navigateTask(task.id)}
                        >
                          {task.name}
                        </button>
                        <small className="tc-muted">
                          {task.owner_id === actor
                            ? '我创建的'
                            : `创建者 ${task.owner_id.slice(-6)}`}
                        </small>
                      </td>
                      <td>
                        <span className="tc-cell">{task.workflow_name}</span>
                        <small className="tc-muted">{task.version}</small>
                      </td>
                      <td>
                        <Badge status={run?.status || 'not_started'} />
                      </td>
                      <td>
                        {run ? (
                          <>
                            <span>
                              {done} / {run.total} 条
                            </span>
                            <div className="tc-progress tc-progress-small">
                              <span
                                style={{
                                  width: `${(done / run.total) * 100}%`,
                                }}
                              />
                            </div>
                            <small className="tc-muted">
                              成功 {run.succeeded} · 失败/中断{' '}
                              {run.failed + run.interrupted}
                            </small>
                          </>
                        ) : (
                          '—'
                        )}
                      </td>
                      <td>{time(task.created_at)}</td>
                      <td>
                        <div className="tc-actions">
                          <button
                            className="tc-link"
                            onClick={() => navigateTask(task.id)}
                          >
                            详情
                          </button>
                          {task.kind === 'async' && task.owner_id === actor && (
                            <button
                              className="tc-link"
                              onClick={() => setExecute(task)}
                            >
                              执行
                            </button>
                          )}
                          {task.owner_id === actor &&
                            (!run || !active(run.status)) && (
                              <button
                                className="tc-link tc-danger-text"
                                onClick={() => setRemove(task)}
                              >
                                删除
                              </button>
                            )}
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
          <Pagination page={page} size={20} total={total} onChange={setPage} />
        </>
      ) : null}
      {execute ? (
        <ExecuteDialog
          space={space}
          task={execute}
          onClose={() => setExecute(undefined)}
          onExecuted={run => {
            const id = execute.id;
            setExecute(undefined);
            navigateTask(id, run.id);
            changed();
            setNotice('已提交后台执行');
          }}
        />
      ) : create ? (
        <CreateDialog
          key={create}
          space={space}
          kind={create}
          onClose={() => setCreate(undefined)}
          onCreated={task => {
            setCreate(undefined);
            navigateTask(task.id);
            changed();
            setNotice(
              task.kind === 'batch'
                ? '批量任务已提交后台执行'
                : '异步任务已创建，可以填写参数并执行',
            );
          }}
        />
      ) : taskID ? (
        <DetailDialog
          space={space}
          id={taskID}
          runID={runID}
          actor={actor}
          onClose={() => navigateTask()}
          onSelectRun={id => navigateTask(taskID, id)}
          onExecute={setExecute}
          onChanged={changed}
        />
      ) : remove ? (
        <Dialog
          title="删除任务"
          onClose={() => setRemove(undefined)}
          busy={busy}
          footer={
            <>
              <button
                className="tc-button"
                onClick={() => setRemove(undefined)}
              >
                取消
              </button>
              <button
                className="tc-button tc-danger"
                disabled={busy}
                onClick={deleteTask}
              >
                确认删除
              </button>
            </>
          }
        >
          <p>
            删除“{remove.name}”及其全部执行记录和结果？此操作不会删除原工作流。
          </p>
          <ErrorBox message={error} />
        </Dialog>
      ) : help ? (
        <Dialog
          title="任务中心使用说明"
          onClose={() => setHelp(false)}
          footer={
            <button
              className="tc-button tc-primary"
              onClick={() => setHelp(false)}
            >
              知道了
            </button>
          }
        >
          <h3>批量任务</h3>
          <p>
            选择已发布的工作流和版本，下载 CSV
            模板，按每行一次执行填写参数，校验后提交。每个文件最多 1000 行、2
            MiB；对象和数组使用 JSON 填写。
          </p>
          <h3>异步任务</h3>
          <p>
            先创建固定工作流版本的任务配置，再填写参数并执行。关闭页面后继续运行，每次执行都有独立记录。
          </p>
          <h3>执行与结果</h3>
          <p>
            空间成员可查看本空间任务，只有创建者可执行、取消、重试和删除。结果可逐行查看或导出
            CSV。模型及插件调用仍按各自配置计费。
          </p>
          <p>
            结果未确认的中断行不会自动重跑。单条输入上限 16 KiB、结果上限 32
            KiB；较大的结果应由工作流保存为文件并返回链接。
          </p>
        </Dialog>
      ) : null}
    </div>
  );
}
