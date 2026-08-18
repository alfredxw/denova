import type { ComponentProps } from 'react'
import { render } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it } from 'vitest'
import { setConfiguredLocale } from '@/i18n'
import { MessageItem as ProjectMessageItem } from './MessageItem'

function MessageItem(props: ComponentProps<typeof ProjectMessageItem>) {
  return <ProjectMessageItem {...props} projectId={props.projectId || 'tool-adapter-project'} />
}

async function renderDetail(name: string, args: unknown, result: string) {
  const rendered = render(
    <MessageItem message={{ role: 'tool_call', name, content: name, args: JSON.stringify(args), status: 'success', result }} />,
  )
  await userEvent.setup().click(rendered.container.querySelector('[data-nova-tool-header]') as HTMLElement)
  const detail = rendered.container.querySelector(`[data-nova-tool-detail="${name}"]`) as HTMLElement
  return {
    ...rendered,
    detail,
    input: detail.querySelector('[data-nova-tool-detail-input]') as HTMLElement,
    output: detail.querySelector('[data-nova-tool-detail-output]') as HTMLElement,
  }
}

function expectCompactText(element: HTMLElement, text: string) {
  const pattern = text.trim().split(/\s+/).map(token => token.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('[\\s\\S]*')
  expect(element.textContent || '').toMatch(new RegExp(pattern))
}

describe('extended tool detail adapters', () => {
  beforeEach(() => setConfiguredLocale('zh-CN'))

  it.each([
    {
      name: 'web_search',
      args: { query: 'Denova', time_range: 'week' },
      result: { schema: 'web_search.v1', status: 'success', results: [{ title: 'Denova Docs', url: 'https://example.com/docs', provider: 'search', published_at: '2026-08-18', summary: 'Documentation summary' }] },
      input: 'Denova', output: 'Denova Docs search 2026-08-18 Documentation summary',
    },
    {
      name: 'web_fetch',
      args: { url: 'https://example.com/docs', start_index: 20, max_chars: 1000 },
      result: { schema: 'web_fetch.v1', status: 'success', url: 'https://example.com/docs', final_url: 'https://example.com/final', title: 'Fetched page', fetch_method: 'direct_http', attempts: [], content: 'Fetched body' },
      input: 'https://example.com/docs', output: 'Fetched page direct_http Fetched body',
    },
    {
      name: 'browser',
      args: { action: 'run', tab: 'docs', command: 'observe', selector: 'main' },
      result: { schema: 'browser.result.v1', status: 'completed', action: 'run', tab: 'docs', command: 'observe', observation: { url: 'https://example.com', title: 'Example', text: 'Visible page', elements: [{ ref: 'e1', role: 'button', name: 'Continue', selector: '#continue' }] }, receipt: {} },
      input: 'run · docs · observe main', output: 'Example Visible page e1 button · Continue · #continue',
    },
    {
      name: 'skill',
      args: { action: 'read', refs: [{ source: 'workspace', id: 'drafting' }] },
      result: { results: [{ ref: { source: 'workspace', id: 'drafting' }, content: { name: 'Drafting', instructions: '# Drafting\nComplete instructions' } }] },
      input: 'workspace · drafting', output: 'Drafting # Drafting Complete instructions',
    },
    {
      name: 'task',
      args: { action: 'start', starts: [{ agent: 'researcher', prompt: 'Research the topic', detached: true }] },
      result: { results: [{ index: 0, task: { ref: { agent: 'researcher', session: 'session-1', run: 'run-1' }, status: 'completed', output: 'Research complete' } }] },
      input: 'researcher Research the topic 后台运行', output: '#0 researcher session=session-1 run=run-1 completed Research complete',
    },
    {
      name: 'script',
      args: { source: 'return input.value + 1', input: { value: 1 } },
      result: { value: 2, logs: ['computed value'] },
      input: '源代码 return input.value + 1 脚本输入', output: '日志 computed value',
    },
    {
      name: 'goal',
      args: { action: 'complete', expected_id: 'goal-1', expected_revision: 3, report: 'All checks passed' },
      result: { status: 'completed', revision: 4, state_hash: 'hidden' },
      input: 'All checks passed complete id=goal-1 revision=3', output: 'completed revision=4',
    },
    {
      name: 'config_read',
      args: { operation: 'list', resource: 'image_preset', scope: 'workspace' },
      result: { schema: 'config.catalog.v1', status: 'success', resource: 'image_preset', scope: 'workspace', items: [{ id: 'cinematic', name: 'Cinematic' }], returned: 1, total: 1, truncated: false },
      input: 'list · image_preset scope=workspace', output: 'cinematic "name": "Cinematic"',
    },
    {
      name: 'config_apply',
      args: { operation: 'update', resource: 'image_preset', id: 'cinematic', scope: 'workspace', revision: 'rev-1', value: { name: 'Cinematic 2' } },
      result: { schema: 'config.mutation_receipt.v1', status: 'applied', resource: 'image_preset', operation: 'update', id: 'cinematic', revision: 'rev-2' },
      input: '"name": "Cinematic 2" update image_preset cinematic scope=workspace revision=rev-1', output: 'applied update cinematic revision=rev-2',
    },
    {
      name: 'list_lore_items',
      args: { keywords: ['钟楼'], types: ['location'], detail: 'index' },
      result: '# Lore Index\n\n- id: clock-tower\n  name: 钟楼\n  brief: 城市中心的旧钟楼',
      input: '钟楼 · types=location detail=index', output: '钟楼 brief: 城市中心的旧钟楼',
    },
    {
      name: 'read_lore_items',
      args: { ids: ['clock-tower'] },
      result: '# Lore Items\n\n## 钟楼 (location / important / auto)\nID: clock-tower\nTags: 城市\n\n```markdown\n钟楼的完整设定。\n```',
      input: 'clock-tower', output: '钟楼 location / important / auto Tags: 城市 钟楼的完整设定。',
    },
    {
      name: 'write_lore_items',
      args: { message: '更新钟楼', items: [{ id: 'clock-tower', content: '新设定' }, { name: '新角色', type: 'character' }], delete_ids: ['obsolete'] },
      result: 'Lore updated（created 1，updated 1，deleted 1）\nitem_ids: ["clock-tower","new-character","obsolete"]\ndeleted_ids: ["obsolete"]',
      input: '更新钟楼 新增 新角色 更新 clock-tower 删除 obsolete', output: 'Lore updated clock-tower new-character 已删除 1 项',
    },
    {
      name: 'search_story_history',
      args: { keywords: ['钟楼'], match: 'all', limit: 3 },
      result: { story_id: 'story', branch_id: 'main', hits: [{ turn_id: 'turn-8', timestamp: '2026-08-18', user_action: '登上钟楼', narrative: '钟声响起。', state_changes: ['位置：钟楼'] }], truncated: false },
      input: '钟楼 match=all limit=3', output: 'turn-8 2026-08-18 登上钟楼 钟声响起。 位置：钟楼',
    },
    {
      name: 'prepare_interactive_turn',
      args: { action: '撬开门锁', challenge: '巡逻即将抵达', cost: '暴露行踪', difficulty: 'normal', rule: { roll_mode: 'advantage' }, bonuses: [{ reason: '合适的工具', value: 2 }] },
      result: { resolution_id: 'r1', dice: '1d20', roll_mode: 'advantage', rolls: [8, 16], kept_roll: 16, bonus_total: 2, total: 18, target: 12, outcome: 'success', result: '门锁被打开。', state_changes: [{ actor_id: '主角', field_id: '体力', change: -1, reason: '消耗体力' }] },
      input: '撬开门锁 normal advantage 1 项加值/减值 +2 合适的工具 代价 暴露行踪', output: 'success 1d20 [8, 16] kept=16 bonus=+2 total=18 target=12 门锁被打开。 状态变化 主角.体力 = -1',
    },
    {
      name: 'submit_interactive_turn',
      args: { state_changes: [{ op: 'replace', actor_id: 'story', field_id: '当前事件', value: '门锁已打开' }], choices: ['进入房间', '等待观察'], director_update: { needed: true, reason: '关键地点已开放' } },
      result: { ready: true, module_status: { state_changes: 'accepted', choices: 'accepted' } },
      input: '状态变化 replace story.当前事件 = 门锁已打开 行动选项 进入房间 等待观察 导演更新 关键地点已开放', output: '回合已就绪 state_changes=accepted choices=accepted',
    },
  ])('$name renders its compact input and semantic output', async ({ name, args, result, input, output }) => {
    const view = await renderDetail(name, args, typeof result === 'string' ? result : JSON.stringify(result))
    expect(view.detail).toBeInTheDocument()
    expectCompactText(view.input, input)
    expectCompactText(view.output, output)
  })

  it('shows empty and partial/error outcomes without exposing transport envelopes', async () => {
    const empty = await renderDetail('web_search', { query: 'nothing' }, JSON.stringify({
      schema: 'web_search.v1', status: 'no_results', message: 'No matches', results: [], warnings: ['Try another phrase'],
    }))
    expectCompactText(empty.output, 'No matches Try another phrase')
    expect(empty.output).not.toHaveTextContent('web_search.v1')
    empty.unmount()

    const partial = await renderDetail('config_read', { operation: 'get', resource: 'skill', ids: ['one', 'missing', 'bad'] }, JSON.stringify({
      schema: 'config.read.v1', status: 'partial', resource: 'skill', items: [{ id: 'one', content: 'ok' }],
      missing_ids: ['missing'], failures: [{ id: 'bad', message: 'Could not read' }], processed: 3, total: 3, truncated: true, next_cursor: 'next-page',
    }))
    expectCompactText(partial.output, 'one 未找到 missing bad Could not read 部分结果 next_cursor=next-page')
    expect(partial.output).not.toHaveTextContent('config.read.v1')
    partial.unmount()

    const rejected = await renderDetail('submit_interactive_turn', { choices: ['same'] }, JSON.stringify({
      ready: false, module_status: { state_changes: 'accepted', choices: 'rejected' },
      diagnostics: [{ module: 'choices', code: 'duplicate_choice', path: '/choices/0', message: 'Choices must be distinct.' }], retry_modules: ['choices'],
    }))
    expectCompactText(rejected.output, '回合尚未就绪 choices · /choices/0 · duplicate_choice Choices must be distinct. 需要重试: choices')
  })

  it('keeps long input-heavy content within the existing single max-height detail', async () => {
    const view = await renderDetail('script', { source: 'return 1\n'.repeat(200), input: {} }, JSON.stringify({ result: 1 }))
    expect(view.detail).toHaveClass('max-h-48')
    expect(view.input).toHaveClass('flex-1', 'overflow-y-auto')
    expect(view.output).toHaveClass('max-h-20')
  })
})
