import { UnitType } from '@coze-data/knowledge-resource-processor-core';
import { IconCozDocument } from '@coze-arch/coze-design/icons';

import { SourceRadio } from '../../../components/source-radio';

export const TableAPI = () => (
  <SourceRadio
    title="API"
    description="获取 JSON 格式 API 内容"
    icon={<IconCozDocument className="w-4 h-4" />}
    key={UnitType.TABLE_API}
    value={UnitType.TABLE_API}
  />
);
