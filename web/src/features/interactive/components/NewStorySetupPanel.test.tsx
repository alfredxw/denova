import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { NewStorySetupPanel } from './NewStorySetupPanel'

const moduleCatalogMocks = vi.hoisted(() => ({
  getEventPackages: vi.fn(),
  getRuleSystems: vi.fn(),
  getActorStates: vi.fn(),
  getStoryMemoryStructures: vi.fn(),
}))

vi.mock('../api', () => moduleCatalogMocks)

const director = {
  version: 4, id: 'default', name: '默认故事导演', description: '', custom: false,
  strategy: { enabled: true }, trpg_system: {},
  module_refs: { narrative_style_id: 'classic', rule_system_id: 'rules', actor_state_id: 'actors', memory_structure_id: 'memory', image_preset_id: 'game-cg' },
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
    moduleCatalogMocks.getStoryMemoryStructures.mockResolvedValue([
      { version: 6, id: 'memory', name: '默认故事记忆结构', description: '', structures: [], custom: false },
    ])
  })

  it('uses one editable module grid backed by mature popup controls', () => {
    const { container } = render(<NewStorySetupPanel stories={[]} tellers={[{ version: 1, id: 'classic', name: '经典叙事', description: '', context_policy: { creator: 'always', lore: 'relevant', runtime_state: 'always' }, slots: [], custom: false }]} directors={[director]} imagePresets={[{ version: 1, id: 'game-cg', name: '游戏 CG', description: '', custom: false }]} onCancel={vi.fn()} onCreate={vi.fn()} />)

    expect(container.querySelectorAll('select')).toHaveLength(0)
    expect(screen.getAllByRole('combobox')).toHaveLength(6)
    expect(screen.queryByRole('button', { name: '自定义模块' })).not.toBeInTheDocument()
    expect(screen.getByText('事件包')).toBeInTheDocument()
  })

  it('uses preset resource names and options for inherited director modules', async () => {
    const user = userEvent.setup()
    const directorWithEvents = { ...director, module_refs: { ...director.module_refs, event_package_ids: ['default'] } }
    render(<NewStorySetupPanel stories={[]} tellers={[]} directors={[directorWithEvents]} imagePresets={[]} onCancel={vi.fn()} onCreate={vi.fn()} />)

    expect(await screen.findByText('按导演默认 · 均衡 DM 检定')).toBeInTheDocument()
    expect(screen.getByText('按导演默认 · 默认状态系统')).toBeInTheDocument()
    expect(screen.getByText('按导演默认 · 默认故事记忆结构')).toBeInTheDocument()
    expect(screen.getByText('按导演默认 · 默认事件包')).toBeInTheDocument()

    await user.click(screen.getByRole('combobox', { name: '角色状态' }))
    expect(await screen.findByRole('option', { name: '修仙状态系统' })).toBeInTheDocument()
  })

  it('creates only after continuing and sends story-level module refs', async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(<NewStorySetupPanel stories={[]} tellers={[{ version: 1, id: 'classic', name: '经典叙事', description: '', context_policy: { creator: 'always', lore: 'relevant', runtime_state: 'always' }, slots: [], custom: false }]} directors={[director]} imagePresets={[{ version: 1, id: 'game-cg', name: '游戏 CG', description: '', custom: false }]} onCancel={vi.fn()} onCreate={onCreate} />)

    expect(onCreate).not.toHaveBeenCalled()
    fireEvent.change(screen.getByLabelText('每轮目标字数'), { target: { value: '2400' } })
    fireEvent.click(screen.getByRole('button', { name: '继续选择开场方式' }))
    await waitFor(() => expect(onCreate).toHaveBeenCalledWith(expect.objectContaining({
      story_director_id: 'default',
      story_teller_id: 'classic',
      reply_target_chars: 2400,
      module_refs: expect.objectContaining({ actor_state_id: 'actors', memory_structure_id: 'memory' }),
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

  it('prefills an existing empty story when returning from opening', () => {
    render(<NewStorySetupPanel stories={[]} story={{ id: 'st_1', title: '返程故事', origin: '已有简介', story_teller_id: 'classic', story_director_id: 'default', module_refs: { rule_system_id: 'rules' }, reply_target_chars: 1800, opening: { mode: 'ai' }, created_at: '', updated_at: '', branches: 1, events: 0 }} tellers={[]} directors={[director]} imagePresets={[]} onCancel={vi.fn()} onCreate={vi.fn()} />)
    expect(screen.getByRole('heading', { name: '编辑故事线配置' })).toBeInTheDocument()
    expect(screen.getByLabelText('故事线名称')).toHaveValue('返程故事')
    expect(screen.getByPlaceholderText('开端描述')).toHaveValue('已有简介')
    expect(screen.getByLabelText('每轮目标字数')).toHaveValue(1800)
  })
})
