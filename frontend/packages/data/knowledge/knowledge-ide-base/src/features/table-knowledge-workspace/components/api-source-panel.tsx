import { useEffect, useState } from 'react';

import { Button, Select } from '@coze-arch/bot-semi';

interface Source {
  url: string;
  interval_days: number;
  last_success_at: number;
  next_run_at: number;
  last_error?: string;
}

export const APISourcePanel = ({ documentID, canEdit, onUpdated }: {
  documentID: string;
  canEdit: boolean;
  onUpdated?: () => void;
}) => {
  const [source, setSource] = useState<Source>();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const api = async (action: string, extra: object = {}) => {
    const base = /^\/agent(?:\/|$)/.test(window.location.pathname) ? '/agent' : '';
    const res = await fetch(`${base}/api/knowledge/table-api/${action}`, {
      method: 'POST', credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ document_id: documentID, ...extra }),
    });
    const result = await res.json();
    if (!res.ok || result.code !== 0) throw new Error(result.message || 'API 来源操作失败');
    return result.data;
  };
  useEffect(() => {
    let active = true;
    let previous: number | undefined;
    const load = async () => {
      try {
        const next: Source = await api('source');
        if (!active) return;
        setSource(next);
        if (previous !== undefined && next.last_success_at > previous) onUpdated?.();
        previous = next.last_success_at;
      } catch (e) {
        if (active) setMessage(e instanceof Error ? e.message : '读取 API 来源失败');
      }
    };
    void load();
    const timer = window.setInterval(load, 15000);
    return () => { active = false; window.clearInterval(timer); };
  }, [documentID]);

  const change = async (action: string, extra: object = {}) => {
    setBusy(true);
    try {
      await api(action, extra);
      setSource(await api('source'));
      setMessage(action === 'refresh' ? '更新已受理；有数据变化时将重新处理，完成后自动显示。' : '更新频率已保存');
      if (action === 'refresh') onUpdated?.();
    } catch (e) {
      setMessage(e instanceof Error ? e.message : '操作失败，请重试');
    } finally { setBusy(false); }
  };
  const time = (value: number) => value ? new Date(value).toLocaleString() : '暂无';
  return <section aria-label="API 来源配置" style={{ flex: '0 0 auto', padding: '12px 24px', borderBottom: '1px solid var(--semi-color-border)' }}>
    <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 12 }}>
      <strong>API 来源</strong>
      <span style={{ flex: 1, minWidth: 180, overflowWrap: 'anywhere' }}>{source?.url || '正在读取…'}</span>
      <Select aria-label="自动更新频率" value={source?.interval_days ?? 0} disabled={!canEdit || busy || !source}
        optionList={[0, 1, 3, 7, 30].map(value => ({ value, label: value ? `每 ${value} 天更新` : '不自动更新' }))}
        onChange={value => void change('schedule', { interval_days: Number(value) })} />
      {canEdit ? <Button loading={busy} onClick={() => void change('refresh')}>立即更新</Button> : null}
    </div>
    <p style={{ marginTop: 8, color: 'var(--semi-color-text-2)', fontSize: 12 }}>最近成功：{time(source?.last_success_at || 0)}　下次检查：{source?.interval_days ? time(source.next_run_at) : '不自动更新'}。更新会以接口全量数据替换此表。</p>
    {source?.last_error ? <p role="alert" style={{ color: 'var(--semi-color-danger)' }}>{source.last_error}</p> : null}
    {message ? <p role="status">{message}</p> : null}
  </section>;
};
