# 前端长会话性能与卡死风险治理报告

> 状态：核心治理已执行，条件性优化待真实基准触发
>
> 执行日期：2026-07-20
>
> 覆盖范围：写作模式、游戏模式、会话恢复、流式渲染、滚动、轮询、SSE 解析、代码高亮与历史接口

## 1. 执行结论

这次没有发现一个可以解释全部“卡死”的单点。真正的高风险是流式更新沿着以下路径叠加放大：

```text
流式事件
  -> React 状态提交
  -> 历史 normalize / view 派生
  -> Markdown 解析
  -> 虚拟列表布局
  -> 自动滚动
  -> 隐藏页面轮询和宽泛 store 订阅
```

已完成的治理把高频路径收敛为“每个动画帧最多提交一次”，把写作首屏历史限制为最近 100 条，把重复 Markdown 树、全历史滚动指纹、重复滚动调度、重叠轮询和无界高亮缓存移除或收敛。设置请求也不再阻塞应用外壳挂载。

附件中较弱模型的总方向有参考价值，但它把若干未证实或低频问题提升成了 P0，并给出了一些可能破坏正确性或引入新复杂度的修复。最典型的是：生产主聊天流本来已有 `throttle: 60`；所谓“零节流 SSE Hook”没有生产调用方；仅在流结束时 normalize、用字符串长度代替指纹、全面增加 `React.memo`、把 diff 全部搬进 Worker、用 `useReducer` 减少渲染等建议都不应直接采用。

本次没有截断正文、thinking、工具参数或历史，没有增加 LLM 超时，没有改变模式切换、后台运行、模型上下文或持久化历史语义。

## 2. 风险分级说明

- 用户体验风险：问题未治理时对卡顿、白屏、滚动抢夺或长会话退化的影响。
- 过度设计风险：继续采用重型架构、额外状态机、Worker、复杂缓存或大范围 memo 后引入回归和维护成本的风险。
- “部分治理”表示已经消除确定性热路径，但仍保留需要真实性能数据才能决定的架构项。

## 3. 风险清单与当前状态

| ID | 风险 | 用户体验风险 | 过度设计风险 | 状态与最小处置 |
| --- | --- | --- | --- | --- |
| F0 | 设置请求完成前不挂载 React，悬挂时白屏 | 极高 | 低 | **已治理**：立即挂载应用外壳，设置异步应用；pending、失败均有测试。 |
| F1 | 根组件接触聊天状态、访问过的路由保留挂载 | 中高 | 高 | **部分治理**：inactive 路由暂停普通轮询和快照派生；未进行高风险的全路由卸载或聊天架构重写。 |
| F2 | 长历史首屏全量加载，流式期间重复 normalize/指纹 | 高 | 中高 | **已治理主要路径**：首屏最近 100 条、向前分页、不可变消息/part 身份缓存、帧级提交。 |
| F3 | 游戏历史转换与 store 宽泛订阅放大流式更新 | 高 | 中 | **已治理主要路径**：窄 selector、稳定历史身份缓存、工具参数帧级批处理；超长已保存游戏历史仍是条件性风险。 |
| F4 | StreamingContentStage 同时渲染两棵完整 Markdown 树 | 高 | 低 | **已治理**：只保留一棵渲染树，未自研 parser。 |
| F5 | 滚动检测遍历历史并 stringify 工具输入/输出 | 高 | 低 | **已治理**：删除全历史 scroll key 和工具 JSON stringify。 |
| F6 | 同一次内容变化触发立即滚动、多个 rAF、timer、DOM 与 Virtuoso 双写 | 高 | 高 | **已治理**：一个 rAF 调度，以 Virtuoso 为主，删除重复原生监听和补偿 timer。 |
| F7 | UI 流和游戏 tool args 每个 chunk 直接提交 | 高 | 低 | **已治理**：共享小型 rAF batcher；done/error/aborted/stop/cleanup 边界 flush 或 discard。 |
| F8 | 一个虚拟行内的 Trace 详情可能无限增长 | 中 | 高 | **行为保持，部分治理**：写作面板沿用摘要配置；游戏中活动 trace 展开语义未擅自改变。只有真实超大 trace 复现后才考虑详情内虚拟化。 |
| F9 | setInterval 轮询重叠，隐藏页面继续请求 | 中 | 低 | **已治理**：workspace、自动化角标、消息中心、游戏快照改为 visibility-aware single-flight。 |
| F10 | workspace change 多路刷新树、摘要和文件 | 中 | 中高 | **部分治理**：周期刷新已合并并防重入；事件级 affected-path 批处理需单独测量后再做。 |
| F11 | SSE buffer 反复 split，尾部事件处理不完整 | 条件性高 | 中 | **已治理扫描成本**：增量查找 LF/CRLF/CR 分隔符、支持多行和未终止尾事件、取消底层 reader；不截断大事件。单个超大 JSON 的同步 `JSON.parse` 仍是条件项。 |
| F12 | 会话快速切换时旧历史响应覆盖新会话 | 中高 | 低 | **已治理**：history generation guard；流 reader cleanup 明确取消/丢弃待提交更新。 |
| F13 | CodeBlock 缓存键碰撞且容量无界 | 中 | 低 | **已治理**：精确内容键、并发去重、64 项/总 1M 源码字符 LRU；超大单项不缓存。 |
| F14 | 大 diff/merge、巨大 FileTree、初始 bundle | 条件性中高 | 高 | **暂不改**：没有 profile 证据时不引入 Worker、全树 memo 或额外分包边界。生产构建仅有现存的 500KB 提示。 |

## 4. 已执行改动

### 4.1 启动和流式提交

- [x] 应用外壳不再等待设置请求，失败或永久 pending 都可以渲染。
- [x] 主写作流保留 AI SDK 已有的 60ms throttle，不替换协议。
- [x] `useAgentUIMessageStream` 使用模块内 rAF batcher 合并同帧更新。
- [x] 游戏 tool args 增量使用同一批处理边界。
- [x] 删除没有生产调用方的 `useAgentSSEUIMessageStream`，避免维护第二套流状态机。
- [x] stop、完成、错误、取消、组件清理和外部消息替换均有边界处理测试。

### 4.2 历史、身份和分页

- [x] `/api/session/messages` 在提供 `limit` 时支持 newest-first window 和 `before` 游标。
- [x] 不带分页参数时仍返回原数组，保留旧调用兼容；前端也兼容旧后端数组响应。
- [x] 首屏默认加载最近 100 条，提供中英双语“加载更早消息”。
- [x] 加载旧历史使用 Virtuoso `firstItemIndex` 保持锚点。
- [x] 后端分页消息 ID 继续以全局历史位置生成，不因分页窗口改变。
- [x] 不可变 `ChatMessage` 和引用稳定的 part 使用有明确失效条件的 WeakMap 身份缓存。
- [x] 快速切换会话时，旧 generation 的返回值不能覆盖当前会话。
- [x] 分页只影响 UI 展示；transport 仍只发送本轮用户输入，模型完整上下文继续由后端历史装配。

### 4.3 Markdown、列表和滚动

- [x] 删除流式 Markdown reserve/overlay 双树及相关 CSS。
- [x] 删除 `buildMessageListScrollKey` 的全历史遍历和工具 JSON stringify。
- [x] 自动滚动由一次 rAF 合并，不再立即执行后追加两帧和 80ms timer。
- [x] 外层列表以 Virtuoso API 为主，删除重复原生 scroll/resize 监听和直接 DOM 双写路径。
- [x] 计划卡锚定删除多帧重试、timer 和额外 ResizeObserver。
- [x] 修正分页引入的 Virtuoso 绝对索引/相对索引转换，并覆盖回合导航和最后一项判断。
- [x] 保持用户向上滚动后不被抢回；显式返回底部后恢复跟随。

### 4.4 游戏和生命周期

- [x] StoryStage 与 InteractiveLayout 改为窄 Zustand selector。
- [x] 游戏历史转换复用稳定消息身份，避免活动尾部更新时重建已完成消息对象。
- [x] inactive 游戏路由暂停普通快照轮询，但不停止已经开始的后台回合。
- [x] workspace、消息中心和自动化角标轮询改为 single-flight，页面隐藏时暂停并在恢复后继续。
- [x] 重复 focus/visibility 事件不会叠加并发 refresh。

### 4.5 SSE 和代码高亮

- [x] SSE parser 改为带 scan offset 的增量扫描，不再对累积 buffer 反复 `split`。
- [x] 支持 LF、CRLF、CR、多行 data 和最后一个未带空行的事件。
- [x] consumer 取消时取消底层 stream reader。
- [x] 不增加任意事件大小上限，不截断工具或 thinking 数据。
- [x] 代码高亮使用精确键、有界 LRU 和 in-flight promise 去重，补齐碰撞和淘汰测试。

## 5. 对附件建议的逐项判断

| 附件项 | 判断 | 本次决定 | 原因及风险 |
| --- | --- | --- | --- |
| #1、#17 SSE/UI 零节流 | **部分成立，定位不准确** | UI 流增加 rAF batch；删除未使用 SSE Hook | 主聊天生产路径本来已有 `throttle: 60`。给未使用 Hook 再造 throttle 会增加重复机制。 |
| #2 normalize O(n)+全文 hash | **成立** | 展示窗口 100 条、稳定身份缓存、帧级提交 | “只在 done 时 normalize”会让流中去重、工具状态升级和恢复语义延迟或出错，不采用。 |
| #3 Markdown 全量解析和双树 | **核心成立，修复建议一半不成立** | 删除双树，保留成熟 ReactMarkdown | 变化中的全文无法靠普通 `useMemo` 复用 AST；`visibility:hidden` 仍然会渲染和解析完整树。 |
| #4 diff3 同步阻塞 | **条件性成立** | 暂不改 | 只在冲突/重基线低频路径触发。无基准直接引入 Worker 会增加序列化、错误处理和取消协议。 |
| #5 工具 stringify | **成立但范围被夸大** | 删除滚动 key 的 stringify；通过稳定 view 避免重复派生 | 以内容长度做 key 会碰撞；无边界 WeakMap 也不应当作通用补丁。 |
| #6、#18 多次滚动 | **成立** | 收敛到一次 rAF 和单一主机制 | `MutationObserver -> ResizeObserver` 不是自动正确答案；当前消息列表主路径直接删除重复机制更简单。 |
| #7 FileTree O(n² log n) | **证据不足** | 暂不改 | 递归排序不等于必然 O(n² log n)，全面 memo + 自定义比较器容易产生陈旧节点和高维护成本。 |
| #8 StoryStage 全历史 flatMap | **部分成立** | 窄订阅、稳定历史身份缓存 | 原依赖并非每个正文 chunk 都必然变化；用 ref 维护第二份可变时间线更容易产生恢复/分支错位。 |
| #9 大量 useState | **结论不成立** | 不用 useReducer 机械合并 | `useReducer` 不会天然减少组件 render；只收窄真正的订阅和高频提交。 |
| #10 巨型 scroll key | **成立** | 删除 | 仅使用最后消息长度会漏掉同长度编辑、附件、计划卡和工具状态变化，因此没有采用附件方案。 |
| #11 全列表项重建 | **成立** | 100 条窗口 + 稳定消息/view 身份 | 未新增复杂时间线 store 或无边界分组缓存；收益已由更小的数据面和身份稳定获得。 |
| #12 JSON clone preset | **未证明为热路径** | 暂不改 | `structuredClone` 不保证更快，且语义不同；配置编辑低频时收益很小。 |
| #13 unified diff | **条件性成立** | 暂不改 | 只影响 review 流程。应先记录具体文档规模和长任务，再决定 memo 或 Worker。 |
| #14 tool args 重复 parse | **局部可能成立** | 先以提交批处理降频，不增加全局缓存 | args 在流中不断变化，按长度缓存会碰撞；跨工具解析缓存会扩大状态失效面。 |
| #15、#29、#30 正则/cloneElement/Trace filter | **可能有累积成本** | 暂不改 | 尚未成为 profile 热点；逐项 memo 会增加比较成本并可能破坏 Markdown/高亮语义。 |
| #16 overscan 过大 | **没有充分证据** | 保持 | overscan 是滚动连续性取舍，不能仅凭像素值判断错误。 |
| #19 localStorage 无 debounce | **低频且未证明** | 暂不改 | 需要先确认写入是否真的处于 token 热路径；盲目 debounce 会引入退出前丢设置风险。 |
| #20 每个 SSE event JSON.parse | **事实成立，结论不完整** | 优化 framing，不把 parse 搬 Worker | 每个 JSON 协议事件本来就需要解析；只有单事件 parse 超过长任务阈值才值得 Worker 化。 |
| #21 设置草稿 stable stringify | **条件性风险** | 暂不改 | 自动保存正确性优先；先以真实大 preset 测量。 |
| #22 callback 依赖多 | **不构成性能证据** | 不改 | 依赖数量本身不产生昂贵计算，关键是值是否变化及下游是否有 memo 边界。 |
| #23、#24 props/内联 JSX 新引用 | **条件性成立** | 不做机械 memo | 父组件已经 render 时，新引用不必然造成额外工作；只有稳定 memo 子边界才有意义。 |
| #25 flattenFileTree | **可能是普通重复派生** | 暂不改 | 与会话卡死关联弱；应先确认树规模和 render 频率。 |
| #26-#28 设置 sections、reduce、TextEncoder | **低频或局部问题** | 暂不改 | 没有证据进入流式主路径，优化优先级低。 |
| P3 `textFingerprint` 改长度 | **不安全** | 明确拒绝 | 同长度不同内容会碰撞，可能直接破坏消息去重和工具状态。 |
| P3 无消息上限 | **成立** | 写作展示历史分页 | 不删除持久化历史、不影响模型上下文；游戏超长历史另列条件项。 |
| 其余 P3 项 | **可能但未证实** | 暂不改 | runtime log、Intl、正则预检查、motion 包装、review key 都需要 profiler 证据，不能按清单批量改。 |

总体判断：附件更适合作为“候选热点列表”，不能直接作为修复清单。它准确发现了 normalize、双 Markdown 树、滚动 stringify、多次滚动和历史无界增长；但优先级、调用路径和若干复杂度结论存在明显过度推断。

## 6. 功能与兼容性保护

- 一级菜单和模式切换规则未改变。
- 写作和游戏均保持后台运行语义；inactive 只暂停非必要 UI 轮询。
- thinking、正文、工具参数和结果均未截断。
- 历史分页不删除后端消息，旧 API 调用仍可获得完整数组。
- 当前前端可连接未升级的旧后端；收到旧数组时自动按单页处理。
- Trace 默认展开行为保持原样。本次曾验证“统一折叠”会破坏游戏实时 thinking/tool 可见性，因此没有保留该改动。
- 新增“加载更早消息”提供中英文文案，并使用已有 shadcn Button。

## 7. 已完成验证

### 7.1 自动化验证

```bash
cd web
pnpm exec tsc --noEmit
pnpm test
pnpm check:i18n
pnpm build

cd ..
go mod tidy
./scripts/build.sh
go test ./...
```

结果：

- TypeScript 检查通过。
- Vitest **147 个测试文件、842 个测试全部通过**。
- i18n **3031 个 key 中英文对齐**。
- 前端生产构建通过。
- `go mod tidy`、项目构建和全部 Go 测试通过。
- 生产构建主 `index` chunk 约 541.27KB minified / 146.04KB gzip；存在 Vite 500KB 提示，但当前已有广泛 code splitting，不据此盲目增加手工分包。

### 7.2 浏览器验证

使用用户已经运行的 `127.0.0.1:5173`，没有启动、替换或终止前后端进程。

- [x] 宽屏写作真实会话：50 条消息、trace、change summary 正常。
- [x] 宽屏游戏真实故事：9 条回合消息、故事正文和导演台正常。
- [x] 390x844 窄屏写作和游戏：输入区、移动导航、返回底部和长文本无横向溢出。
- [x] dark/light 主题：临时切换检查后恢复原 dark 设置。
- [x] 模式切换仍由用户显式操作，没有点击一级菜单自动切换模式。

控制台出现的错误仅为当前工作区缺少可选的 `setting/interactive-openings.json` 与 `setting/interactive-opening.md` 所产生的 404；页面有既有 fallback，写作与游戏均正常渲染。这不是本次改动引入的异常，但后续可另行把“可选文件不存在”从 error 级网络噪声收敛为可识别的空状态。

## 8. 尚未执行的条件项

以下项目不是遗漏，而是为了避免过度设计而有意保留的测量门禁：

- [ ] 建立 100/500/2000 条写作消息和 50/200 回合游戏的可重复浏览器 benchmark。
- [ ] 记录 React commit、p95 long task、滚动写次数和 heap，不以主观感受代替数据。
- [ ] 如果 App/ModeRouter 的 token 更新仍让无关页面 commit，再把活动消息订阅下沉到独立画布；当前不重写全局路由缓存。
- [ ] 如果游戏恢复 200+ 回合的转换或内存成为实际瓶颈，再设计游戏历史分页；必须同时验证分支、版本、规则账本和图片事件。
- [ ] 如果单次 diff3、unified diff 或单个 SSE JSON parse 的 p95 主线程任务超过 50ms，再评估 Worker；每种任务单独设计取消和错误回退。
- [ ] 如果真实 FileTree 规模使交互 commit 超过 50ms，先把排序移到数据变更边界，再考虑局部 memo。
- [ ] 如果真实首屏指标证明 bundle 是瓶颈，再按依赖图拆分；不要只为消除 Vite warning 手工切 chunk。
- [ ] 如果单个 Trace 详情能够稳定复现滚动/布局长任务，再为详情建立独立虚拟区域，不改变默认可见语义。
- [ ] 合入前按项目要求更新 `CHANGELOG.md`；本次未创建 commit，因此没有提前写入发布记录。

## 9. 后续执行门槛

只有满足以下任一条件才继续进行结构性优化：

1. 可重复真实数据集下出现超过 50ms 的主线程任务；
2. 500 条与 100 条历史相比，活动尾部更新 p95 增长超过 25%；
3. inactive 路由仍随 token 产生可观测 React commit 或请求；
4. 单次用户操作可稳定复现 200ms 以上无响应；
5. heap 在停止流式并完成 GC 后仍随会话切换单调增长。

达到门槛后，一次只处理一个边界，先补最小复现和回归测试，再修改生产代码。不得把 App 拆分、游戏分页、Worker、全树 memo 和新缓存放进同一变更。

## 10. 回滚边界

如果回归出现，可按以下职责独立回退，不需要永久 feature flag：

| 回归类型 | 首选回滚边界 |
| --- | --- |
| 启动主题或 locale 异常 | `web/src/main.tsx` 启动异步化 |
| chunk 丢失、结束态缺尾段 | rAF batcher 与两个流调用点 |
| 历史缺失或锚点跳动 | session messages 分页、`useAgentChat`、Virtuoso `firstItemIndex` |
| Markdown 样式/高度异常 | `StreamingContentStage` 单树改动 |
| 滚动抢夺或无法回底 | bottom scroll scheduler 与 `useVirtuosoBottomLock` |
| 隐藏页后台行为异常 | active/visibility-aware polling 改动 |
| SSE 协议兼容异常 | `parseSSEStream` 增量 scanner |
| 高亮错误或内存增长 | CodeBlock LRU |

出现消息丢失、顺序变化、后台任务被中断、模型上下文变化或历史不可访问时，应直接回退对应边界，不能继续叠加 timer、Observer、镜像状态或兼容分支来掩盖问题。
