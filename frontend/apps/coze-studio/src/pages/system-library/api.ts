import { getAppBasePath } from '@coze-arch/web-context';

export interface Field {
  name: string;
  type: string;
  description?: string;
  required?: boolean;
  children?: Field[];
}
export interface Resource {
  kind: string;
  id: string;
  name: string;
  description: string;
  icon?: string;
  version: string;
  origin: string;
  status: string;
  revision: number;
  published_at: number;
  read_only: boolean;
  usage: string;
  notice?: string;
  fields?: Field[];
  tools?: { id: string; name: string; description: string; fields?: Field[] }[];
  content?: string;
}
export interface Listing {
  resources: Resource[];
  total: number;
  counts: Record<string, number>;
  publishing_ready: boolean;
}
export const kinds: Record<string, string> = {
  all: '全部资源',
  plugin: '插件',
  workflow: '工作流',
  prompt: '提示词',
  knowledge: '知识库',
  database: '数据库',
};
export async function request<T>(
  action: string,
  params: Record<string, string>,
  signal?: AbortSignal,
): Promise<T> {
  const response = await fetch(
    `${getAppBasePath()}/api/system-library/${action}?${new URLSearchParams(params)}`,
    { credentials: 'same-origin', signal },
  );
  if (!response.headers.get('content-type')?.includes('application/json'))
    throw new Error('系统资源服务暂不可用，请稍后刷新');
  const data = await response.json();
  if (!response.ok || data.code !== 0)
    throw new Error(data.message || '读取系统资源失败');
  return data.data;
}
