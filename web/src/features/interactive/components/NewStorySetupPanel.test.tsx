import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { NewStorySetupPanel } from './NewStorySetupPanel'

const moduleCatalogMocks = vi.hoisted(() => ({
  getEventPackages: vi.fn(),
  getRuleSystems: vi.fn(),
  getActorStates: vi.fn(),
}))

vi.mock('../api', () => moduleCatalogMocks)

const director = {
  version: 4, id: 'default', name: '默认故事导演', description: '', custom: false,
  strategy: { enabled: true }, trpg_system: {},
  module_refs: { narrative_style_id: 'classic', rule_system_id: 'rules', actor_state_id: 'actors', image_preset_id: 'game-cg' },
}

describe('NewStorySetupPanel', () => {
  beforeEach(() => {
    moduleCatalogMocks.getEventPackages.mockResolvedValue([
      { version: 6, id: 'default', name: '默认事件包', description: '', custom: false },
      { version: 6, id: 'urban-core', name: '都市核心事件包', description: '', custom: false },
    ])
    moduleCatalogMocks.getRuleSystems.mockResolvedValue([
      { version: 6, id: 'rules', name: '均衡 DM 检定', description: '', trpg_system: {}, custom: false },
    ])
    moduleCatalogMocks.getActorStates.mockResolvedValue([
      { version: 6, id: 'actors', name: '默认状态系统', description: '', actor_state: {}, custom: false },
      { version: 6, id: 'xiuxian-state', name: '修仙状态系统', description: '', actor_state: {}, custom: false },
    ])
  })

  it('uses one editable module grid backed by mature popup controls', () => {
    const { container } = render(<NewStorySetupPanel stories={[]} tellers={[{ version: 1, id: 'classic', name: '经典叙事', description: '', context_policy: { creator: 'always', lore: 'relevant', runtime_state: 'always' }, slots: [], custom: false }]} directors={[director]} imagePresets={[{ version: 1, id: 'game-cg', name: '游戏 CG', description: '', custom: false }]} onCancel={vi.fn()} onCreate={vi.fn()} />)

    expect(container.querySelectorAll('select')).toHaveLength(0)
    expect(screen.getAllByRole('combobox')).toHaveLength(5)
    expect(screen.queryByRole('button', { name: '自定义模块' })).not.toBeInTheDocument()
    expect(screen.getByText('事件包')).toBeInTheDocument()
  })

  it('uses preset resource names and options for inherited director modules', async () => {
    const user = userEvent.setup()
    const directorWithEvents = { ...director, module_refs: { ...director.module_refs, event_package_ids: ['default'] } }
    render(<NewStorySetupPanel stories={[]} tellers={[]} directors={[directorWithEvents]} imagePresets={[]} onCancel={vi.fn()} onCreate={vi.fn()} />)

    expect(await screen.findByText('按导演默认 · 均衡 DM 检定')).toBeInTheDocument()
    expect(screen.getByText('默认状态系统')).toBeInTheDocument()
    expect(screen.getByText('按导演默认 · 默认事件包')).toBeInTheDocument()

    await user.click(screen.getByRole('combobox', { name: '基础状态模板' }))
    expect(await screen.findByRole('option', { name: '修仙状态系统' })).toBeInTheDocument()
  })

  it('creates only after continuing and sends story-level module refs', async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(<NewStorySetupPanel stories={[]} tellers={[{ version: 1, id: 'classic', name: '经典叙事', description: '', context_policy: { creator: 'always', lore: 'relevant', runtime_state: 'always' }, slots: [], custom: false }]} directors={[director]} imagePresets={[{ version: 1, id: 'game-cg', name: '游戏 CG', description: '', custom: false }]} onCancel={vi.fn()} onCreate={onCreate} />)

    expect(onCreate).not.toHaveBeenCalled()
    fireEvent.change(screen.getByLabelText('每轮目标字数'), { target: { value: '2400' } })
    fireEvent.change(screen.getByLabelText(/\u884c\u52a8\u5efa\u8bae\u6570\u91cf/), { target: { value: '7' } })
    fireEvent.click(screen.getByRole('button', { name: '继续选择开场方式' }))
    await waitFor(() => expect(onCreate).toHaveBeenCalledWith(expect.objectContaining({
      story_director_id: 'default',
      story_teller_id: 'classic',
      choice_count: 7,
      reply_target_chars: 2400,
      module_refs: expect.objectContaining({ actor_state_id: 'actors' }),
      state_schema_policy: { mode: 'adapt_template' },
      director_run_policy: { mode: 'on_demand' },
    })))
  })

  it('offers three director run modes and submits the configured interval', async () => {
    const user = userEvent.setup()
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(<NewStorySetupPanel stories={[]} tellers={[]} directors={[director]} imagePresets={[]} onCancel={vi.fn()} onCreate={onCreate} />)

    expect(screen.getByRole('radio', { name: /按需自动/ })).toBeChecked()
    expect(screen.getByRole('radio', { name: /仅手动/ })).toBeInTheDocument()
    await user.click(screen.getByRole('radio', { name: /每 X 回合自动/ }))
    await user.clear(screen.getByLabelText('导演自动规划间隔'))
    await user.type(screen.getByLabelText('导演自动规划间隔'), '4')
    await user.click(screen.getByRole('button', { name: '继续选择开场方式' }))

    await waitFor(() => expect(onCreate).toHaveBeenCalledWith(expect.objectContaining({
      director_run_policy: { mode: 'interval', interval_turns: 4 },
    })))
  })

  it('switches to generated schema without submitting a state preset', async () => {
    const user = userEvent.setup()
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(<NewStorySetupPanel stories={[]} tellers={[]} directors={[director]} imagePresets={[]} onCancel={vi.fn()} onCreate={onCreate} />)

    await user.click(screen.getByRole('radio', { name: /为故事动态生成/ }))
    expect(screen.queryByRole('combobox', { name: '基础状态模板' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '继续选择开场方式' }))

    await waitFor(() => expect(onCreate).toHaveBeenCalledWith(expect.objectContaining({
      state_schema_policy: { mode: 'generate' },
      module_refs: expect.objectContaining({ actor_state_disabled: true }),
    })))
  })

  it('cancels without creating a placeholder story', () => {
    const onCancel = vi.fn()
    const onCreate = vi.fn()
    render(<NewStorySetupPanel stories={[]} tellers={[]} directors={[director]} imagePresets={[]} onCancel={onCancel} onCreate={onCreate} />)
    fireEvent.click(screen.getByRole('button', { name: '取消' }))
    expect(onCancel).toHaveBeenCalledOnce()
    expect(onCreate).not.toHaveBeenCalled()
  })

  it('rejects an invalid choices-per-turn value', async () => {
    const onCreate = vi.fn()
    render(<NewStorySetupPanel stories={[]} tellers={[]} directors={[director]} imagePresets={[]} onCancel={vi.fn()} onCreate={onCreate} />)
    fireEvent.change(screen.getByLabelText(/行动建议数量/), { target: { value: '11' } })
    fireEvent.click(screen.getByRole('button', { name: '继续选择开场方式' }))
    expect(await screen.findByText('行动建议数量必须是 2–10 之间的整数。')).toBeInTheDocument()
    expect(onCreate).not.toHaveBeenCalled()
  })

  it('prefills an existing empty story when returning from opening', () => {
    render(<NewStorySetupPanel stories={[]} story={{ id: 'st_1', title: '返程故事', origin: '已有简介', story_teller_id: 'classic', story_director_id: 'default', module_refs: { rule_system_id: 'rules' }, state_schema_policy: { mode: 'fixed_template' }, choice_count: 6, reply_target_chars: 1800, opening: { mode: 'ai' }, created_at: '', updated_at: '', branches: 1, events: 0 }} tellers={[]} directors={[director]} imagePresets={[]} onCancel={vi.fn()} onCreate={vi.fn()} />)
    expect(screen.getByRole('heading', { name: '编辑故事线配置' })).toBeInTheDocument()
    expect(screen.getByLabelText('故事线名称')).toHaveValue('返程故事')
    expect(screen.getByPlaceholderText('开端描述')).toHaveValue('已有简介')
    expect(screen.getByLabelText('每轮目标字数')).toHaveValue(1800)
    expect(screen.getByLabelText(/\u884c\u52a8\u5efa\u8bae\u6570\u91cf/)).toHaveValue(6)
    expect(screen.getByRole('radio', { name: /固定使用模板/ })).toBeChecked()
  })
})
