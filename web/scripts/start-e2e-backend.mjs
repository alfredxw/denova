import { spawn, spawnSync } from 'node:child_process'
import { existsSync, mkdirSync, rmSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const repositoryRoot = path.resolve(webRoot, '..')
const testResultsRoot = path.join(webRoot, 'test-results')
const runtimeRoot = path.join(testResultsRoot, 'runtime')
const relativeRuntime = path.relative(testResultsRoot, runtimeRoot)

if (relativeRuntime.startsWith('..') || path.isAbsolute(relativeRuntime)) {
  throw new Error(`Refusing to prepare E2E runtime outside ${testResultsRoot}`)
}

if (existsSync(runtimeRoot)) rmSync(runtimeRoot, { recursive: true, force: true })
const denovaDir = path.join(runtimeRoot, 'denova')
const binaryDir = path.join(runtimeRoot, 'bin')
mkdirSync(denovaDir, { recursive: true })
mkdirSync(binaryDir, { recursive: true })

const backendPort = process.env.DENOVA_E2E_BACKEND_PORT || '18080'
const modelPort = process.env.DENOVA_E2E_MODEL_PORT || '18081'
const binaryPath = path.join(binaryDir, process.platform === 'win32' ? 'denova-e2e.exe' : 'denova-e2e')
const config = `language = "zh-CN"
update_check_enabled = false
model_max_retries = 1

[[model_endpoints]]
id = "e2e"
name = "E2E deterministic model"
provider = "openai-compatible"
protocol = "openai-chat-completions"
api_key = "e2e-test-key"
base_url = "http://127.0.0.1:${modelPort}/v1"

[[model_profiles]]
id = "e2e"
name = "E2E deterministic model"
endpoint_id = "e2e"
model = "denova-e2e"
context_window_tokens = 100000

[agent_models.default]
profile_id = "e2e"
thinking_level = "off"

[agent_models.ide]
profile_id = "e2e"
thinking_level = "off"

[agent_models.interactive_story]
profile_id = "e2e"
thinking_level = "off"

[agent_models.interactive_director]
profile_id = "e2e"
thinking_level = "off"
`
writeFileSync(path.join(denovaDir, 'config.toml'), config, 'utf8')

const legacyWorkspace = path.join(denovaDir, 'projects', 'Legacy E2E Book')
const legacyLorePath = path.join(legacyWorkspace, '.nova', 'lore', 'items.json')
const legacyStoryDir = path.join(legacyWorkspace, 'interactive', 'story')
const legacyStoryID = 'st_legacy_v033_e2e'
const legacyStoryTimestamp = '2026-01-01T00:00:00Z'
const legacyDirectorDir = path.join(legacyWorkspace, 'interactive', 'stories', legacyStoryID, 'director', 'main')
mkdirSync(path.join(legacyWorkspace, 'chapters'), { recursive: true })
mkdirSync(path.dirname(legacyLorePath), { recursive: true })
mkdirSync(legacyStoryDir, { recursive: true })
mkdirSync(legacyDirectorDir, { recursive: true })
writeFileSync(path.join(legacyWorkspace, 'book.json'), JSON.stringify({
  title: 'Legacy E2E Book',
  author: 'Denova v0.3.3',
  description: 'Seeded released-version compatibility fixture.',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}, null, 2), 'utf8')
writeFileSync(path.join(legacyWorkspace, 'chapters', 'legacy-chapter.md'), '# 旧章节\n\n这是 v0.3.3 保留的正文。\n', 'utf8')
writeFileSync(legacyLorePath, JSON.stringify({
  version: 1,
  items: [{
    id: 'hero',
    enabled: true,
    type: 'character',
    name: '林川',
    importance: 'major',
    content: '旧资料库正文',
  }],
}, null, 2), 'utf8')
writeFileSync(path.join(legacyStoryDir, 'index.json'), JSON.stringify({
  current_story_id: legacyStoryID,
  stories: [{
    id: legacyStoryID,
    title: 'Legacy v0.3.3 Story',
    origin: '旧车站仍在等待下一位访客。',
    story_teller_id: 'classic',
    story_director_id: 'default',
    director_run_policy: { mode: 'manual' },
    reply_target_chars: 2000,
    choice_count: 2,
    opening: { mode: 'custom', custom_text: '旧车站仍在等待下一位访客。' },
    image_settings: { mode: 'manual', interval_turns: 3, preset_id: 'game-cg' },
    state_schema_policy: { mode: 'fixed_template' },
    created_at: legacyStoryTimestamp,
    updated_at: legacyStoryTimestamp,
    branches: 1,
    events: 1,
  }],
}, null, 2), 'utf8')
const legacyStoryRows = [{
  v: 1,
  type: 'meta',
  story_id: legacyStoryID,
  title: 'Legacy v0.3.3 Story',
  origin: '旧车站仍在等待下一位访客。',
  story_teller_id: 'classic',
  story_director_id: 'default',
  director_run_policy: { mode: 'manual' },
  reply_target_chars: 2000,
  choice_count: 2,
  opening: { mode: 'custom', custom_text: '旧车站仍在等待下一位访客。' },
  image_settings: { mode: 'manual', interval_turns: 3, preset_id: 'game-cg' },
  state_schema_policy: { mode: 'fixed_template' },
  current_branch: 'main',
  branches: {
    main: { head: 'turn_legacy_v033_e2e', created_at: legacyStoryTimestamp, title: '主线' },
  },
  created_at: legacyStoryTimestamp,
  updated_at: legacyStoryTimestamp,
}, {
  v: 1,
  type: 'turn',
  id: 'turn_legacy_v033_e2e',
  parent_id: null,
  branch_id: 'main',
  ts: legacyStoryTimestamp,
  user: '查看旧车站',
  narrative: '这是 v0.3.3 保存的游戏正文。',
  state_status: 'ready',
}]
writeFileSync(
  path.join(legacyStoryDir, `story-${legacyStoryID}.jsonl`),
  `${legacyStoryRows.map((row) => JSON.stringify(row)).join('\n')}\n`,
  'utf8',
)
writeFileSync(path.join(legacyDirectorDir, 'director.md'), `# 导演私密规划

${[
  '阶段目标与隐藏钩子', '资料库锚点', '选角覆盖', '核心角色与关系张力',
  '重要势力与阶段阻力', '当前场景幕后信息', '信息揭示与线索密度', '遭遇、检定与代价',
  '爽点、危机与反转', '状态连续性', '最近分支安排', '伏笔与回收',
].map((heading) => `## ${heading}\n保留 v0.3.3 的导演规划。`).join('\n\n')}
`, 'utf8')
writeFileSync(path.join(legacyDirectorDir, 'agent-brief.md'), `# 正文 Agent 简报

${[
  '当前目标与可见钩子', '当前场景与行动空间', '当前角色与可见关系', '已公开信息与可发现线索',
  '遭遇、检定与可见代价', '状态连续性', '最近分支承接',
].map((heading) => `## ${heading}\n承接 v0.3.3 保存的旧车站剧情。`).join('\n\n')}
`, 'utf8')
writeFileSync(path.join(legacyDirectorDir, 'lore-context.md'), `# 分支资料工作集

## 当前

[[林川]] 仍在旧车站。

## 候场

暂无。

## 暂离场

暂无。
`, 'utf8')
writeFileSync(path.join(legacyDirectorDir, 'metadata.json'), JSON.stringify({
  version: 1,
  story_id: legacyStoryID,
  branch_id: 'main',
  revision: 'legacy-v033',
  branch_planning_turns: 5,
  updated_at: legacyStoryTimestamp,
  source: 'interactive_director',
  source_turn_id: 'turn_legacy_v033_e2e',
  last_run: {
    status: 'ready',
    summary: 'v0.3.3 保存的导演规划。',
    source_turn_id: 'turn_legacy_v033_e2e',
    updated_at: legacyStoryTimestamp,
    planned_docs: 3,
    completed_docs: 3,
    start_ready: true,
    blocking: false,
  },
}, null, 2), 'utf8')
writeFileSync(path.join(denovaDir, 'books.json'), JSON.stringify({
  current: legacyWorkspace,
  books: [{
    name: 'Legacy E2E Book',
    path: legacyWorkspace,
    last_opened_at: '2026-01-01T00:00:00Z',
  }],
  sort_mode: 'recent',
  order: [legacyWorkspace],
  hidden: [],
}, null, 2), 'utf8')

const build = spawnSync('go', ['build', '-o', binaryPath, './cmd/denova'], {
  cwd: repositoryRoot,
  env: process.env,
  stdio: 'inherit',
})
if (build.error) throw build.error
if (build.status !== 0) process.exit(build.status ?? 1)

const backend = spawn(binaryPath, ['--no-open', `--port=${backendPort}`], {
  cwd: runtimeRoot,
  env: {
    ...process.env,
    DENOVA_DIR: denovaDir,
    DENOVA_SKILLS_DIR: path.join(repositoryRoot, 'skills'),
  },
  stdio: 'inherit',
})

function forwardSignal(signal) {
  if (!backend.killed) backend.kill(signal)
}
process.once('SIGINT', () => forwardSignal('SIGINT'))
process.once('SIGTERM', () => forwardSignal('SIGTERM'))
backend.once('error', (error) => {
  console.error('[e2e-backend] failed to start', error)
  process.exitCode = 1
})
backend.once('exit', (code) => process.exit(code ?? 0))
