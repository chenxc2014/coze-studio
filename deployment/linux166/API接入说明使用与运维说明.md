# API接入说明使用与运维说明

更新日期：2026-11-18。适用于药销易-智能体平台当前定制版本。平台入口：`https://ai.yaoxiaoyi.com/agent/`。

## 三个进入位置

1. 点击左下角个人图标，选择“API接入说明”，直接打开说明页面。
2. 在同一账号设置窗口的左侧菜单选择“API接入说明”。
3. 打开“API 授权”，在个人访问令牌管理页点击“API接入说明”快捷按钮。

三个入口共用同一个页面。原账号设置、API 授权、添加新令牌和退出登录功能继续保留；创建令牌仍走原有对话框、权限和有效期逻辑。本次未改变令牌的认证规则。

## 页面内容与示例生成

说明包括 API 发布前提、获取智能体 ID、获取平台令牌、POST 提问、非流式状态轮询、获取最终回答、SSE 流式事件、连续对话和常见问题。页面按当前部署地址生成接口前缀：正式域名下保留 `/agent/`，根路径部署不额外添加该前缀。

用户可输入智能体 ID，或粘贴含 `/bot/数字ID` 的编辑地址，页面仅在本地提取 ID，不访问粘贴的网址。长整数作为字符串处理，避免精度丢失。示例问题可编辑，JSON、Bash cURL 及连续对话示例同步更新，并提供复制按钮。无效 ID 显示提示，示例保留 `YOUR_BOT_ID` 待替换。

说明页不读取、回显或保存真实令牌，不使用某个真实账号的智能体作为全平台默认示例，不自动发起问答。示例使用 `YOUR_PLATFORM_API_TOKEN` 或调用方环境变量 `AGENT_API_TOKEN`。复制功能优先使用浏览器剪贴板，受限时尝试本地兼容复制；仍失败则提示手动选择文本。

## 当前版本的调用流程

外部应用使用已发布 API 渠道的智能体；预览使用的草稿配置并不自动成为外部 API 配置。后续修改需要重新发布。调用令牌应由具备该智能体访问资格的所属账号创建，外部请求不传网页登录密码或模型厂商密钥。

| 步骤 | 接口与结果 |
| --- | --- |
| 提问 | `POST /agent/v3/chat`；请求头 `Authorization: Bearer 平台令牌` 与 `Content-Type: application/json`；请求体包含字符串 `bot_id`、业务 `user_id`、`additional_messages` 与明确的 `stream` |
| 非流式提交 | 使用 `stream: false`；响应 `data.id` 是 `chat_id`，`data.conversation_id` 是会话 ID，先返回任务信息，并非完整回答 |
| 状态查询 | `GET /agent/v3/chat/retrieve?conversation_id=会话ID&chat_id=任务ID`；检查 `data.status`，完成前合理间隔轮询并设置总超时 |
| 获取回答 | `GET /agent/v3/chat/message/list?conversation_id=会话ID&chat_id=任务ID`；当前消息数组是 **`Messages`**，筛选 `role=assistant` 且 `type=answer` 的消息，读取 `content` |
| 连续对话 | 之后的 POST 增加原 `conversation_id`；同一业务用户保持其 `user_id`，不同用户分别维护会话 |
| 流式接收 | 使用 `stream: true`，按 SSE 读取回答增量或完成消息，不重复拼接，不将工具结果或推理消息当作最终回答 |

`Messages` 的首字母大写是当前源码中的真实 JSON 字段，不能按照官方云平台的其他版本或旧聊天说明把消息数组写作 `data`。状态查询和提交响应仍使用 `data` 保存任务信息。说明中的消息结构为删减字段后的示意，不冒充真实调用结果。

HTTP 200 不一定表示业务成功，还需检查 JSON 业务码和 SSE 错误事件。`completed` 后获取消息；`created` 和 `in_progress` 尚未完成。`failed`、`expired`、`cancelled`、`required_action` 不应无限轮询，需要分别处理。

## 文件与维护位置

| 文件 | 作用 |
| --- | --- |
| `frontend/packages/foundation/global-adapter/src/components/account-dropdown/index.tsx` | 个人图标菜单入口 |
| `frontend/packages/foundation/global-adapter/src/components/account-dropdown/account-settings/index.tsx` | 左侧说明菜单及 API 授权页快捷入口 |
| `frontend/packages/foundation/global-adapter/src/components/account-dropdown/account-settings/api-guide/index.tsx` | 说明页面、步骤导航、示例表单、复制按钮和令牌页说明头部 |
| 同目录 `index.module.less` | 窗口内滚动阅读、代码块和输入样式 |
| 同目录 `examples.ts` | 部署前缀、编辑地址 ID 提取、JSON 与 cURL 生成 |
| 同目录 `examples.test.ts` | 根路径和子路径、长 ID、特殊字符及流式参数测试 |

API 行为核对位置为 `backend/api/handler/coze/agent_run_service.go`、`backend/api/handler/coze/message_service.go`、`backend/application/conversation/openapi_agent_run.go` 和对应消息模型。以后调整响应字段或执行方式时，应同时更新说明，不能只复制官方云平台文档。

本次复用 `PatBody.renderTopBodySlot` 扩展授权页，不修改共享令牌组件的认证和创建逻辑，也不增加后端接口、数据库迁移或依赖。

## 构建、发布与回退

前端在 `/home/coze/build/subpath-source` 独立构建，产物发布到 `/home/coze/releases/web-agent-api-guide-*`。部署前执行 `/home/coze/ops/capacity.py --guard`，沿用根分区至少余 15 GiB、`/home` 至少余 25 GiB 及既有使用率限制。所有新增产物、缓存和备份使用 `/home/coze`。

仅调整 Compose 中 `coze-web` 的静态目录挂载，再执行 `/home/coze/ops/compose.sh up -d --no-deps coze-web`。部署前保存 Compose 私有备份，检查线上前端基线；发布后核对 `/agent/` 返回新页面、无 Basic Auth 挑战，且 `coze-server` 的容器 ID 和镜像保持不变。发布记录在 `/home/coze/build/api-guide/release.json`。

回退时先确认后续没有其他发布，再恢复该记录 `backup_path` 下的 `compose.json` 并重建 `coze-web`。不恢复数据库、不改令牌、不删除知识数据。后端服务及其插件凭据只读挂载、Embedding 配置、联网查询和已有 API 运行逻辑保持原状。

## 验证范围

四项示例生成单元测试和定向类型检查通过，完整前端构建成功。线上验收记录见同目录 `API接入说明验收.json`，发布及服务检查见 `API接入说明运行核对.json`。

页面验收复用有效登录态，仅检查菜单、说明页、示例生成、复制入口、当前路径、消息字段和滚动阅读。复制行为通过浏览器内的测试替身核对，避免读取或覆盖用户系统剪贴板；cURL 参数使用离线替身函数验证，不发起网络调用。粘贴的编辑网址也不访问。

2026-11-18 已通过 18 项只读页面及参数检查。章节跳转只滚动说明正文，导航保持可见，较小窗口下标题和关闭按钮仍在视野内。当前前端目录为 `/home/coze/releases/web-agent-api-guide-8daedea2aa5f`，Compose 备份为 `/home/coze/backups/api-guide-20261118-191420/compose.json`；首次发布说明功能前的备份为 `/home/coze/backups/api-guide-20261118-190624/compose.json`。后端仍为原容器和镜像，11 个容器正常，两份原知识文档分别保持 19,946 行与 9,000 行且无失败切片，容量守卫通过。精确检查时间及容量见运行核对 JSON。

本次不创建或修改真实令牌，不自动调用已发布智能体或模型，不使用厂商 API，不保存真实凭据或会话。不能将说明页验收表述为真实业务智能体 API 联调通过。账号与令牌生命周期仍按原平台功能维护。
