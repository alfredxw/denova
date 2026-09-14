# Denova 插件开发手册

更新：2026-09-13。接口以当前代码和运行实例的 OpenAPI 为准。

配套：[扩展能力开发手册](extension-development.md) · [游戏开发手册](game-developer-guide.md) · [系统设计](plugin-platform-design.md) · [HTTP API](plugin-platform-api.md)。

扩展页使用二级目录管理已安装的插件与游戏；工作台统一承接源码开发。插件提供可复用工具、工具集与服务；游戏交付可玩的作品。Skills 与 Agents 继续由各自页面管理，插件清单不分发这两类资源。宿主模型调用 API 和游戏私有角色定义保留。

## 内置示例

完整示例源码供开发参考，不作为创建选项，默认不安装、不启用。创建源码项目、检查打包和准备隔离预览均不会将示例加入已安装扩展，也不会改变默认游戏；安装后才成为可用扩展。

| 示例源码目录 | 示例 | 演示能力 |
| --- | --- | --- |
| `http-tool` | 创作工具箱 | 文本统计与批量整理、独立笔记、资料库、图像模型槽、工具集、设置与权限 |
| `galgame` | 余光来信 | 流式对话、角色立绘与背景、CG、Story 恢复、资料库与图像生成 |

## 在 App 中开发

1. 在「扩展 → 创建扩展」选择「开发插件」，填写名称和可选需求。创建后进入工作台，打开源码项目和普通开发会话；填写的需求会作为第一条开发消息发送。
2. 默认在应用数据目录的 projects 下创建新目录，自动生成稳定包 ID。插件与游戏各自生成一个通用开发骨架，不选择模板；高级选项只用于指定包 ID 或父目录。已有同名目录不会被覆盖。
3. 开发骨架在 Project 根生成 denova.plugin.json、源码和 DEVELOPMENT.md。ProjectID 是稳定身份；受管 location 持久化为规范相对路径。源码草稿只在工作台显示，安装不会创建新 Project；扩展页的「打开源码」可关联多个独立源码副本。
4. 工作台「更多开发操作 → 源码清单」直接编辑并自动保存清单，保留未编辑字段；文件和源码版本沿用项目编辑器与版本历史。含根清单的已有 Project 自动显示开发工具，损坏清单保留修复入口。同一项目可通过显式子目录绑定开发多个扩展。
5. 工作台普通 Agent 会自动关联 extension-development Skill、当前所选源码位置和最近构建/测试反馈。反馈仅附在后续消息中，最多保留 16,384 字符，超出时保留末尾并标记省略；完整构建输出仍在终端中。切换扩展来源不会混用反馈。
6. 工作台主操作为「试运行」和「发布到本机」。发布只确认名称、说明和版本等基本信息，保存源码编辑器后检查并安装，启用声明的必需能力；模型、测试参数和逐项授权不进入发布表单。可选能力在扩展设置中管理。需要构建的项目通过「更多开发操作 → 构建」在可见终端执行；关闭构建窗口停止该终端。检查验证文件、清单、schema 和引用，不执行代码。发布完成后打开已安装扩展，详情页可导出其准确冻结版本；本机设置、测试存档和未发布源码不会进入安装包。对外分发由用户自行处理。
7. 试运行使用开发 Project 及独立测试数据。后续源码修改需要重新检查并开启新预览；同版本号的源码变化也可安装。已安装快照和真实存档不会随源码编辑改变。模型选择属于运行实例，扩展通用设置由扩展页统一管理。

## 从 GitHub 安装与更新

扩展页「安装扩展 → GitHub」接受公开的 `https://github.com/owner/repository` 地址，可填写分支、标签、完整 commit 和包所在子目录；默认使用默认分支和仓库根目录。每个包目录恰有一种 Denova 清单。同仓库的游戏与插件分别定位、检查和安装。

检查先解析并固定 commit，再下载源码归档，按 `distribution.files` 收集安装文件并冻结候选包。安装只使用已检查字节。安装无需系统 Git，也不创建 Project；源码归档不包含 Git 历史。下载与展开各不超过 256 MiB，归档不超过 10000 项、单文件不超过 16 MiB；拒绝不符合跨系统规则的路径、大小写冲突、符号链接和特殊文件。暂不支持私有仓库认证、SSH 或自动拉取子模块、Git LFS 内容。

作者应提交可直接运行的文件或构建产物。需要构建时，选择「导入源码到工作台」创建独立受管 Project，保留整个仓库的共享构建输入和所选子目录。导入不执行代码；依赖准备、编译由作者声明的配方和既有可见终端负责。构建后重新检查安装。导入记录的 commit 表示基础源码，最终产物以内容摘要为准；再次导入会创建独立副本，不覆盖本地修改。

详情页显示当前版本、仓库、ref 和 commit，并提供「检查更新」「更新」。更新按上游提交判断，确认时固定该提交；更新过程中包身份变化或原安装已被替换会拒绝安装。同版本号可以包含新的源码快照，版本号由作者用于表达兼容性。已有游戏存档继续引用原发行；普通 Agent 会话的后续执行直接采用当前已安装插件。新依赖默认采用当前已安装插件并校验版本范围，不在历史发行中自动择优。旧快照仍用于恢复，界面不展示发行历史列表。

源码初始化失败时保留已创建 Project 和文件，进入工作台检查，不删除用户工作。目录显示名和清单中英文名称可分别修改。Node 程序拥有当前系统账户权限；平台范围凭证不等于操作系统沙箱。

扩展页「使用配置与授权」提供独有设置表单、TOML 编辑、恢复默认值和独立权限管理。同一扩展各发行共用一份 settings.toml；保存校验、并发保护、备份、预览隔离及运行快照规则见[扩展设置标准](extension-settings.md)。修改设置在下次启动生效，修改权限会停止受影响实例。卸载保留源码、会话、发行、设置和数据。

## HTTP 工具参考示例

仓库中的 `internal/platform/templates/http-tool` 包含：

```text
denova.plugin.json
server.mjs
runtime.mjs
tools.mjs
tools/*.json
settings.schema.json
defaults.toml
locales/zh-CN.json
locales/en-US.json
```

开发目录还提供通用浏览器连接示例；只有 distribution.files 列出的文件和清单进入发布包。该示例无需下载依赖，构建配方为 node --check server.mjs。

ID 为 example.text-tools 的参考示例公开六个工具：count-characters、clean-text、read-notes、save-note、search-library、illustrate，分别归入 text-utils 和 creative-resources 工具集。笔记追加到宿主按会话或 Story 分支隔离的数据目录，相同 requestId 重试不重复写入；单作用域上限 1 MiB，超限明确报错。批量整理逐项报告错误。资料库、图像和写操作由可选权限控制；illustrate 需要调用方绑定本插件的 illustrator 图像模型槽，没有绑定时返回 NOT_CONFIGURED，普通文本工具仍可使用。工具统计 Unicode 码点，默认包括空白；在扩展设置中启用「忽略空白字符」后，新启动会跳过空白。定义使用 JSON Schema 2020-12，声明英文模型描述、输入、可选结构化输出和 effect（pure、read、propose、write）。宿主在调用前后分别验证输入及结构化输出。Agent 工具适配器使用现有参数修复与执行引擎。

提供器收到 POST /tools/count，HTTP JSON body 直接是工具输入；成功返回：

```json
{ "content": "The text contains 3 Unicode code points.", "data": { "count": 3 } }
```

公共引用是 pluginId/localId。工具集必须引用本包已有工具；跨插件引用需要 requires 中明确列出对应贡献。游戏私有定义使用 local:id，不注册为其他游戏可见能力。

## 清单和发布

清单使用 denova.plugin.json，至少声明 manifestVersion: 1、id、严格语义版本 version、apiMajor: 1、中英文 name、permissions 和 contributes。ID 使用小写字母、数字、点和连字符；builtin、local 保留。一个产物根只能有一种清单。

- distribution.files 是文件或目录白名单；未列出的依赖、入口或定义会使检查失败。
- runtime.backend 支持 protocol: denova-runtime-v1 与 launch.kind: runtime、runtime: node、entry、args。
- modelSlots 支持 text 与 image；titleKey 必须存在于中英 locale 文件。宿主分别提供文本和图像配置选择；扩展只能使用实际绑定的槽位。
- settings 引用分发内的 JSON Schema 与 defaults TOML，可选 uiSchema 提供声明式布局；字段必须有中英文标题。详见[扩展设置标准](extension-settings.md)。
- requires 声明插件 ID、语义版本范围和所需贡献。运行时使用准确发行，不按加载顺序替换。
- 声明 agents.run、tools.invoke、pluginData 权限；消费者直接调用或游戏 NPC 使用 `effect: write/propose` 的工具还需显式授予 `tools.write`。尚未实现的能力会明确拒绝。

## 写作与游戏共用工具

插件管理统一在扩展页中完成。启用并授权后，插件的公开工具自动供写作、工作台和内置游戏 Agent 使用，无需逐会话选择；工具集与成员不重复暴露。新执行读取当前已安装版本及共享设置，现有会话同样采用最新配置。正在执行的任务使用已经装载的工具；暂停期间修改相关版本或设置后，原任务可能无法原地恢复，需要保留历史并按当前配置重新发起。

三个入口和委派子 Agent 使用同一 Toolset 适配器。`pure/read` 工具按只读执行，`write/propose` 按外部变更沿用宿主 Agent 审批；插件结果回到原 Agent 的模型上下文，写作内容与游戏回合仍由原流程接纳。第三方游戏私有 NPC 通过定义中的 `tools/toolsets` 引用相同插件，并受游戏已授予权限限制。公开 Agent 定义、动态 context 和编辑器面板仍不属于本版插件贡献。

工具 schema 的检查不启动插件代码；第一次实际调用才启动后端，本轮结束或取消后清理。数据按 Project、Product Session 或 Story 分支隔离。插件启停与当前版本归 installed.json，参数归 settings.toml；会话 JSONL／Story JSONL 不保存插件选择、版本或参数副本。插件进程路径、凭证和连接信息只存在于运行时。

安装使用已检查字节，内容摘要区分同版本号的不同快照。旧快照保留，禁用只阻止新启动，当前游戏与任务可以继续至正常停止。停止后需重新启用才能启动；缺少可用依赖的游戏不会出现在新建故事线的可选列表中。卸载会停止受影响实例，同时保留数据与冻结快照。

重新安装相同发行可以调整授权；宿主先停止受影响的实例并撤销旧凭证，再保存新的授权快照。源码和发行字节保持不变。

## 后端启动协议

宿主以 Node 启动已冻结入口，通过 stdin 的一行 JSON 发送 type: bootstrap、protocol、context、connection、packageDir、dataDir、tempDir、hostToken。路径仅是当前宿主运行时投影，不应写入业务身份。平台凭证和内部服务凭证分别使用，均不得写入 URL、argv、日志或发行包。

提供器监听独立 loopback 端口，stdout 第一行返回：

```json
{ "type": "ready", "protocol": "denova-runtime-v1", "port": 42000 }
```

宿主再使用 hostToken 验证 GET /__denova/ready 返回 denova-runtime-v1。后台其他入口也必须验证该凭证。启动等待有基础设施超时；Agent 运行没有固定总时长或迭代上限。stderr 和后续 stdout 进入本地日志。

stdin 收到 shutdown 或关闭后停止服务。Windows 通过 Job Object 管理进程树；其他桌面系统使用进程组。当前变更在 Windows 原生验证；macOS 与 Linux/WSL 仍需目标平台验证。

需要组合资料库、资源、图像或现有 Story 时，使用[扩展能力开发手册](extension-development.md)中的授权接口。自定义游戏界面由游戏视图完整实现。
