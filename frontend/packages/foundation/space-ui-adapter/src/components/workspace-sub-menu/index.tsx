/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import { WorkspaceSubMenu as BaseWorkspaceSubMenu } from '@coze-foundation/space-ui-base';
import { useSpaceStore } from '@coze-foundation/space-store';
import { I18n } from '@coze-arch/i18n';
import { useRouteConfig } from '@coze-arch/bot-hooks';
import {
  IconCozBot,
  IconCozBotFill,
  IconCozKnowledge,
  IconCozKnowledgeFill,
} from '@coze-arch/coze-design/icons';
import { Space, Avatar, Typography } from '@coze-arch/coze-design';

import { SpaceSubModuleEnum } from '@/const';

export const WorkspaceSubMenu = () => {
  const { subMenuKey } = useRouteConfig();

  const currentSpace = useSpaceStore(state => state.space);

  const subMenu = [
    {
      icon: <IconCozBot />,
      activeIcon: <IconCozBotFill />,
      title: () => I18n.t('navigation_workspace_develop', {}, 'Develop'),
      path: SpaceSubModuleEnum.DEVELOP,
      dataTestId: 'navigation_workspace_develop',
    },
    {
      icon: <IconCozKnowledge />,
      activeIcon: <IconCozKnowledgeFill />,
      title: () => I18n.t('navigation_workspace_library', {}, 'Library'),
      path: SpaceSubModuleEnum.LIBRARY,
      dataTestId: 'navigation_workspace_library',
    },
    {
      icon: <IconCozKnowledge />,
      activeIcon: <IconCozKnowledgeFill />,
      title: () => '资源库-系统',
      path: SpaceSubModuleEnum.SYSTEM_LIBRARY,
      dataTestId: 'navigation_workspace_system_library',
    },
    {
      icon: (
        <svg
          viewBox="0 0 24 24"
          width="20"
          height="20"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.7"
        >
          <circle cx="12" cy="12" r="8" />
          <path d="M12 7v5l3 2M6 2 3 5m15-3 3 3" />
        </svg>
      ),
      activeIcon: (
        <svg
          viewBox="0 0 24 24"
          width="20"
          height="20"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
        >
          <circle cx="12" cy="12" r="8" />
          <path d="M12 7v5l3 2M6 2 3 5m15-3 3 3" />
        </svg>
      ),
      title: () => I18n.t('coze_sidebar_task_center', {}, '任务中心'),
      path: SpaceSubModuleEnum.TASKS,
      dataTestId: 'navigation_workspace_tasks',
    },
  ];

  const headerNode = (
    <div className="cursor-pointer w-full">
      <Space
        className="h-[48px] px-[8px] w-full hover:coz-mg-secondary-hovered rounded-[8px]"
        spacing={8}
      >
        <Avatar
          className="w-[24px] h-[24px] rounded-[6px] shrink-0"
          src={currentSpace?.icon_url}
        />
        <Typography.Text
          ellipsis={{ showTooltip: true, rows: 1 }}
          className="flex-1 coz-fg-primary text-[14px] font-[500]"
        >
          {currentSpace?.name || ''}
        </Typography.Text>
      </Space>
    </div>
  );

  return (
    <BaseWorkspaceSubMenu
      header={headerNode}
      menus={subMenu}
      currentSubMenu={subMenuKey}
    />
  );
};
