import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { LoreItem } from '@/lib/api'
import type { StoryDirector, Teller } from '../types'
import { NewStorySetupPanel } from './NewStorySetupPanel'

vi.mock('../api', () => ({
  getActorStates: vi.fn().mockResolvedValue([{ id: 'state-basic', name: '基础状态' }]),
  getEventPackages: vi.fn().mockResolvedValue([]),
  getRuleSystems: vi.fn().mockResolvedValue([{ id: 'd20', name: 'D20' }]),
}))

vi.mock('@/features/agents/CustomAgentSelect', () => ({
  CustomAgentSelect: () => <div data-testid="custom-agent-select" />,
}))

const director: StoryDirector = {
  version: 1,
  id: 'adventure',
  name: '冒险',
  description: '',
  module_refs: {
    narrative_style_id: 'cinematic',
    rule_system_id: 'd20',
    actor_state_id: 'state-basic',
    image_preset_id: 'game-cg',
  },
  strategy: {},
  trpg_system: {},
  custom: false,
}

const teller: Teller = {
  version: 1,
  id: 'cinematic',
  name: '电影化',
  description: '',
  context_policy: {} as Teller['context_policy'],
  slots: [],
  custom: false,
}

const loreCharacter: LoreItem = {
  id: 'hero',
  enabled: true,
  type: 'character',
  type_source: 'manual',
  name: '林川',
  importance: 'major',
  load_mode: 'auto',
  tags: ['航海士'],
  brief_description: '失忆的领航员',
  keywords: [],
  content: '林川熟悉旧港的每一条水道。',
  created_at: '2026-08-30T00:00:00Z',
  updated_at: '2026-08-31T00:00:00Z',
}

describe('NewStorySetupPanel', () => {
  it('submits a Lore protagonist snapshot source and opening from one start flow', async () => {
    const user = userEvent.setup()
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(
      <NewStorySetupPanel
        projectId="project-1"
        stories={[]}
        tellers={[teller]}
        directors={[director]}
        imagePresets={[{ version: 1, id: 'game-cg', name: 'Game CG', description: '', custom: false }]}
        loreItems={[loreCharacter]}
        bookOpeningPresets={[{ id: 'harbor', title: '雾港来信', content: '港口的灯逐盏熄灭。' }]}
        onCancel={vi.fn()}
        onCreate={onCreate}
      />,
    )

    await user.click(screen.getByRole('radio', { name: '从资料库选择' }))
    await user.click(screen.getByRole('button', { name: '选择资料库角色' }))
    await user.click(await screen.findByRole('option', { name: /林川/ }))
    await user.click(screen.getByRole('tab', { name: /书籍预设/ }))
    await user.click(await screen.findByRole('option', { name: /雾港来信/ }))
    await user.click(screen.getByRole('button', { name: '开始故事' }))

    await waitFor(() => expect(onCreate).toHaveBeenCalledTimes(1))
    expect(onCreate.mock.calls[0]?.[0]).toMatchObject({
      protagonist: { mode: 'lore', source_lore_item_id: 'hero' },
      opening: { mode: 'preset', preset_id: 'harbor', preset_text: '港口的灯逐盏熄灭。' },
      check_settings: { difficulty_shift: 0, roll_modifier: 0 },
      image_settings: { mode: 'manual', interval_turns: 3, preset_id: 'game-cg' },
    })
  })

  it('keeps all story initialization controls behind one progressive disclosure', async () => {
    render(
      <NewStorySetupPanel
        projectId="project-1"
        stories={[]}
        tellers={[teller]}
        directors={[director]}
        imagePresets={[]}
        loreItems={[]}
        bookOpeningPresets={[]}
        onCancel={vi.fn()}
        onCreate={vi.fn()}
      />,
    )

    expect(screen.queryByRole('heading', { name: '回合判定' })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /高级设置/ }))
    expect(await screen.findByRole('heading', { name: 'Game Agent' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '回合判定' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '互动图像' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '状态面板' })).toBeInTheDocument()
    expect(screen.queryByText('主舞台展示')).not.toBeInTheDocument()
  })
})
