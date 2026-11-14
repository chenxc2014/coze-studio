import { getAppBasePath } from '@coze-arch/web-context';

export interface Field {
  name: string;
  type: string;
  required?: boolean;
  description?: string;
}
export interface Workflow {
  id: string;
  name: string;
  version: string;
  app_id?: string;
  fields: Field[];
  versions?: string[];
}
export interface Run {
  id: string;
  task_id: string;
  status: string;
  total: number;
  succeeded: number;
  failed: number;
  cancelled: number;
  interrupted: number;
  retry_of: string;
  created_at: number;
  started_at: number;
  finished_at: number;
}
export interface Task {
  id: string;
  space_id: string;
  owner_id: string;
  name: string;
  kind: 'batch' | 'async';
  workflow_id: string;
  workflow_name: string;
  version: string;
  concurrency: number;
  timeout_seconds: number;
  latest_run_id: string;
  latest_run?: Run;
  fields?: Field[];
  created_at: number;
  updated_at: number;
}
export interface Item {
  id: string;
  position: number;
  status: string;
  input_json: string;
  output_json: string;
  error: string;
  execution_id: string;
  started_at: number;
  finished_at: number;
}
export interface Detail {
  task: Task;
  runs: Run[];
  runs_total: number;
  run?: Run;
  items: Item[];
  items_total: number;
}
export interface Preview {
  total: number;
  fields: Field[];
  rows: string[][];
}

const prefix = () => `${getAppBasePath()}/api/task-center/`;
export const key = () =>
  Array.from(crypto.getRandomValues(new Uint8Array(16)), value =>
    value.toString(16).padStart(2, '0'),
  ).join('');

export async function api<T>(
  action: string,
  params: Record<string, string> = {},
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const response = await fetch(
    `${prefix()}${action}${body === undefined ? `?${new URLSearchParams(params)}` : ''}`,
    {
      method: body === undefined ? 'GET' : 'POST',
      credentials: 'same-origin',
      signal,
      headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    },
  );
  let value: { code: number; message?: string; data: T };
  try {
    value = await response.json();
  } catch {
    throw new Error(`请求未完成（HTTP ${response.status}），请稍后重试`);
  }
  if (!response.ok || value.code !== 0)
    throw new Error(value.message || '任务中心请求失败');
  return value.data;
}

export async function download(
  action: 'template' | 'export',
  params: Record<string, string>,
) {
  const response = await fetch(
    `${prefix()}${action}?${new URLSearchParams(params)}`,
    { credentials: 'same-origin' },
  );
  if (!response.ok) {
    const value = await response.json();
    throw new Error(value.message || '下载失败');
  }
  const blob = await response.blob();
  const link = document.createElement('a');
  const url = URL.createObjectURL(blob);
  link.href = url;
  link.download =
    action === 'template' ? '工作流参数模板.csv' : '任务执行结果.csv';
  document.body.appendChild(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export const statuses: Record<string, string> = {
  not_started: '未执行',
  queued: '排队中',
  running: '执行中',
  cancelling: '取消中',
  completed: '已完成',
  partial: '部分失败',
  failed: '失败',
  cancelled: '已取消',
  interrupted: '执行中断',
  succeeded: '成功',
};
export const active = (status: string) =>
  ['queued', 'running', 'cancelling'].includes(status);
export const time = (value: number) =>
  value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—';
// JSON 按原文展示，避免浏览器把超过安全整数范围的参数或结果改写。
export const pretty = (text: string) => text || '暂无结果';
const typeNames: Record<string, string> = {
  string: '文本',
  integer: '整数',
  float: '数字',
  number: '数字',
  boolean: '布尔值',
  object: '对象',
  list: '数组',
  array: '数组',
};
export const typeName = (type: string) => typeNames[type] || type;
