import { useState } from 'react';

import {
  type ContentProps,
  FooterBtnStatus,
  UploadStatus,
} from '@coze-data/knowledge-resource-processor-core';
import { Button, Input, Select, Modal } from '@coze-arch/bot-semi';
import { TableDataType } from '@coze-arch/bot-api/knowledge';

import { UploadFooter } from '@/components';
import { TableStatus } from '@/constants';
import { tableSettingsToString } from '@/utils';
import { useFetchTableSchemaInfo } from '@/features/knowledge-type/table/hooks';
import type { UploadTableAction, UploadTableState } from '@/features/knowledge-type/table/interface';

import { TableLocalAddConfig } from '../../local/add/config';
import { TableLocalStep } from '../../local/constants';

type State = UploadTableState<TableLocalStep> & UploadTableAction<TableLocalStep>;

const platformPath = (path: string) =>
  `${/^\/agent(?:\/|$)/.test(window.location.pathname) ? '/agent' : ''}${path}`;

const TableAPIUpload = ({ useStore, footer }: ContentProps<State>) => {
  const unitList = useStore(state => state.unitList);
  const settings = useStore(state => state.tableSettings);
  const [url, setURL] = useState(unitList[0]?.url || '');
  const [interval, setInterval] = useState(unitList[0]?.updateInterval || 0);
  const [visible, setVisible] = useState(!unitList.length);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const fetchSchema = useFetchTableSchemaInfo(useStore);
  const prepared = unitList[0];

  const prepare = async () => {
    setBusy(true);
    setError('');
    try {
      const response = await fetch(platformPath('/api/knowledge/table-api/prepare'), {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ url: url.trim(), interval_days: interval }),
      });
      const result = await response.json();
      if (!response.ok || result.code !== 0) {
        throw new Error(result.message || '抓取失败，请检查接口地址');
      }
      const data = result.data;
      useStore.getState().setUnitList([{
        uri: data.uri, url: url.trim(), name: `API 数据表（${data.rows} 行）`,
        type: 'csv', status: UploadStatus.SUCCESS, percent: 100,
        size: String(data.size), updateInterval: interval,
      }]);
      setVisible(false);
    } catch (e) {
      setError(e instanceof Error ? e.message : '抓取失败，请稍后重试');
    } finally {
      setBusy(false);
    }
  };

  return <>
    <section style={{ maxWidth: 720, margin: '48px auto', padding: 24, border: '1px solid var(--semi-color-border)', borderRadius: 12 }}>
      <h2 style={{ fontSize: 18, marginBottom: 16 }}>新增 API</h2>
      <p style={{ color: 'var(--semi-color-text-2)', marginBottom: 16 }}>通过 HTTP 或 HTTPS GET 获取 JSON 表格，支持内网地址和自定义端口，再配置字段与语义索引。导入的数据可绑定智能体使用。</p>
      {prepared ? <div style={{ marginBottom: 16, overflowWrap: 'anywhere' }}><strong>{prepared.name}</strong><p>{prepared.url}</p><p>{prepared.updateInterval ? `每 ${prepared.updateInterval} 天自动更新` : '不自动更新'}</p></div> : null}
      <Button onClick={() => setVisible(true)}>{prepared ? '修改 URL 或更新频率' : '添加 URL'}</Button>
      <p style={{ fontSize: 12, color: 'var(--semi-color-text-2)', marginTop: 16 }}>支持对象数组及 data / rows / items / list 包装；最大 8 MB、50000 行、100 列。暂不支持分页接口或自定义鉴权请求头。</p>
    </section>
    <Modal title="添加 URL" visible={visible} onCancel={() => !busy && setVisible(false)}
      onOk={prepare} confirmLoading={busy} okText="抓取并保存" cancelText="取消" maskClosable={!busy}>
      <label style={{ display: 'block', marginBottom: 8 }} htmlFor="table-api-url">网址 URL</label>
      <Input id="table-api-url" value={url} onChange={setURL} placeholder="http://example.com:8000/api" disabled={busy} />
      <label style={{ display: 'block', margin: '20px 0 8px' }} htmlFor="table-api-interval">更新频率</label>
      <Select id="table-api-interval" style={{ width: '100%' }} value={interval} disabled={busy}
        onChange={v => setInterval(Number(v))}
        optionList={[0, 1, 3, 7, 30].map(value => ({ value, label: value ? `每 ${value} 天` : '不自动更新' }))} />
      <Button theme="borderless" style={{ marginTop: 16 }} disabled={busy} onClick={() => setURL(new URL(platformPath('/admin/examples/medical-institutions.json'), window.location.origin).href)}>使用示例 API（虚构测试数据）</Button>
      {error ? <p role="alert" style={{ color: 'var(--semi-color-danger)', marginTop: 12 }}>{error}</p> : null}
    </Modal>
    {footer?.([{
      text: '下一步', type: 'hgltplus', theme: 'solid',
      status: prepared && !busy ? FooterBtnStatus.ENABLE : FooterBtnStatus.DISABLE,
      onClick: () => {
        useStore.getState().setCurrentStep(TableLocalStep.CONFIGURATION);
        useStore.getState().setStatus(TableStatus.LOADING);
        fetchSchema({ source_file: { tos_uri: prepared.uri }, table_data_type: TableDataType.AllData, table_sheet: tableSettingsToString(settings) });
      },
    }])}
  </>;
};

export const TableAPIAddConfig = {
  ...TableLocalAddConfig,
  steps: TableLocalAddConfig.steps.map((step, index) => index ? step : {
    ...step,
    title: '新增 API',
    content: (props: ContentProps<State>) => <TableAPIUpload {...props} footer={controls => <UploadFooter controls={controls} />} />,
  }),
};
