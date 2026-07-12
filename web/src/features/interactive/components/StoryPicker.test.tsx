import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { StoryPicker } from './StoryPicker'
import type { StoryDirector } from '../types'

const { rollInteractiveActorTraitsMock } = vi.hoisted(() => ({
  rollInteractiveActorTraitsMock: vi.fn(),
}))

vi.mock('../api', () => ({
  rollInteractiveActorTraits: rollInteractiveActorTraitsMock,
}))

beforeEach(() => {
  rollInteractiveActorTraitsMock.mockReset()
})

describe('StoryPicker', () => {
  it('shows every story option immediately when opened', () => {
    const stories = Array.from({ length: 12 }, (_, index) => story(`st_${index + 1}`, `故事线 ${index + 1}`))

    render(
      <StoryPicker
        stories={stories}
        currentStoryId="st_1"
        tellers={[]}
        onSelect={vi.fn()}
        onCreate={vi.fn()}
        onDelete={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '选择故事线' }))

    expect(screen.getAllByRole('option')).toHaveLength(12)
    expect(screen.getByRole('option', { name: '故事线 12' })).toBeInTheDocument()
  })

  it('selects a story option and closes the panel', () => {
    const onSelect = vi.fn()

    render(
      <StoryPicker
        stories={[story('st_1', '主线'), story('st_2', '支线')]}
        currentStoryId="st_1"
        tellers={[]}
        onSelect={onSelect}
        onCreate={vi.fn()}
        onDelete={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '选择故事线' }))
    fireEvent.click(screen.getByRole('option', { name: '支线' }))

    expect(onSelect).toHaveBeenCalledWith('st_2')
    expect(screen.queryByRole('option', { name: '支线' })).not.toBeInTheDocument()
  })

  it('keeps delete action inside the story selector panel', () => {
    render(
      <StoryPicker
        stories={[story('st_1', '主线')]}
        currentStoryId="st_1"
        tellers={[]}
        onSelect={vi.fn()}
        onCreate={vi.fn()}
        onDelete={vi.fn()}
      />,
    )

    expect(screen.queryByRole('button', { name: '删除故事线' })).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '选择故事线' }))

    expect(screen.getByRole('button', { name: '删除故事线' })).toBeInTheDocument()
  })

  it('passes reply target chars when creating a story', () => {
    const onCreate = vi.fn()

    render(
      <StoryPicker
        stories={[]}
        currentStoryId=""
        tellers={[
          {
            version: 3,
            id: 'classic',
            name: '经典叙事',
            description: '',
            context_policy: {
              creator: 'always',
              lore: 'relevant',
              runtime_state: 'always',
            },
            slots: [],
            custom: false,
          },
        ]}
        onSelect={vi.fn()}
        onCreate={onCreate}
        onDelete={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '新建' }))
    fireEvent.change(screen.getByText('每轮目标字数').parentElement?.querySelector('input') as HTMLInputElement, { target: { value: '650' } })
    fireEvent.click(screen.getByRole('button', { name: '创建' }))

    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({
        story_teller_id: 'classic',
        story_director_id: 'default',
        reply_target_chars: 650,
      }),
    )
  })

  it('does not inherit disabled director modules when creating a story', () => {
    const onCreate = vi.fn()

    render(
      <StoryPicker
        stories={[]}
        currentStoryId=""
        tellers={[classicTeller(), { ...classicTeller(), id: 'alt-style', name: '强风格' }]}
        storyDirectors={[detachedStoryDirector()]}
        onSelect={vi.fn()}
        onCreate={onCreate}
        onDelete={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '新建' }))

    expect(screen.getByText('当前主角模板没有绑定可用词条池；故事仍会正常创建。')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '抽取词条' })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: '抽取词条' }))
    expect(rollInteractiveActorTraitsMock).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: '创建' }))

    expect(onCreate).toHaveBeenCalledWith(expect.objectContaining({
      story_teller_id: 'classic',
      story_director_id: 'detached',
      image_settings: expect.objectContaining({ preset_id: 'game-cg' }),
      initial_trait_rolls: undefined,
    }))
  })

  it('uses the selected director narrative style when creating a story', () => {
    const onCreate = vi.fn()

    render(
      <StoryPicker
        stories={[]}
        currentStoryId=""
        tellers={[classicTeller(), { ...classicTeller(), id: 'alt-style', name: '强风格' }]}
        storyDirectors={[classicStoryDirector(), { ...classicStoryDirector(), id: 'alt-director', name: '强导演', module_refs: { narrative_style_id: 'alt-style' } }]}
        onSelect={vi.fn()}
        onCreate={onCreate}
        onDelete={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '新建' }))
    fireEvent.change(screen.getByText('故事导演').parentElement?.querySelector('select') as HTMLSelectElement, { target: { value: 'alt-director' } })

    expect(screen.getByText('叙事风格跟随导演：')).toBeInTheDocument()
    expect(screen.getByText('强风格')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '创建' }))

    expect(onCreate).toHaveBeenCalledWith(expect.objectContaining({
      story_teller_id: 'alt-style',
      story_director_id: 'alt-director',
    }))
  })

  it('previews protagonist traits and sends a validated initial trait roll', async () => {
    const onCreate = vi.fn()
    rollInteractiveActorTraitsMock.mockResolvedValue({
      story_director_id: 'default',
      actor_id: 'protagonist',
      template_id: 'protagonist',
      seed: 42,
      traits: [{ pool_id: 'talent', trait_id: 'hidden-bloodline', name: '隐脉', summary: '灵力上限更高' }],
    })

    render(
      <StoryPicker
        stories={[]}
        currentStoryId=""
        tellers={[classicTeller()]}
        storyDirectors={[classicStoryDirector()]}
        onSelect={vi.fn()}
        onCreate={onCreate}
        onDelete={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '新建' }))
    fireEvent.click(screen.getByRole('button', { name: '抽取词条' }))

    await waitFor(() => expect(rollInteractiveActorTraitsMock).toHaveBeenCalledWith({
      story_director_id: 'default',
      actor_id: 'protagonist',
      template_id: 'protagonist',
      selections: [],
    }))
    await waitFor(() => expect(screen.getAllByText('隐脉').length).toBeGreaterThan(0))

    fireEvent.click(screen.getByRole('button', { name: '创建' }))

    expect(onCreate).toHaveBeenCalledWith(expect.objectContaining({
      initial_trait_rolls: [{ actor_id: 'protagonist', seed: 42, selections: [] }],
    }))
  })

  it('applies manually selected protagonist traits by pool', async () => {
    rollInteractiveActorTraitsMock.mockResolvedValue({
      story_director_id: 'default',
      actor_id: 'protagonist',
      template_id: 'protagonist',
      seed: 42,
      traits: [{ pool_id: 'talent', trait_id: 'hidden-bloodline', name: '隐脉', summary: '灵力上限更高' }],
    })

    render(
      <StoryPicker
        stories={[]}
        currentStoryId=""
        tellers={[classicTeller()]}
        storyDirectors={[classicStoryDirector()]}
        onSelect={vi.fn()}
        onCreate={vi.fn()}
        onDelete={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '新建' }))
    fireEvent.click(screen.getByRole('button', { name: '隐脉' }))
    expect(screen.getByText('已选择 1 个词条')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '应用选择' }))

    await waitFor(() => expect(rollInteractiveActorTraitsMock).toHaveBeenCalledWith({
      story_director_id: 'default',
      actor_id: 'protagonist',
      template_id: 'protagonist',
      selections: [{ pool_id: 'talent', trait_ids: ['hidden-bloodline'] }],
    }))
  })

  it('persists fixed protagonist selections even when creation skips preview', () => {
    const onCreate = vi.fn()
    render(
      <StoryPicker
        stories={[]}
        currentStoryId=""
        tellers={[classicTeller()]}
        storyDirectors={[classicStoryDirector()]}
        onSelect={vi.fn()}
        onCreate={onCreate}
        onDelete={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '新建' }))
    fireEvent.click(screen.getByRole('button', { name: '隐脉' }))
    fireEvent.click(screen.getByRole('button', { name: '创建' }))

    expect(rollInteractiveActorTraitsMock).not.toHaveBeenCalled()
    expect(onCreate).toHaveBeenCalledWith(expect.objectContaining({
      initial_trait_rolls: [{
        actor_id: 'protagonist',
        seed: 0,
        selections: [{ pool_id: 'talent', trait_ids: ['hidden-bloodline'] }],
      }],
    }))
  })

  it('shows schema adaptation progress and keeps creation errors in the form', async () => {
    let rejectCreate: ((error: Error) => void) | undefined
    const onCreate = vi.fn(() => new Promise<void>((_resolve, reject) => {
      rejectCreate = reject
    }))
    render(
      <StoryPicker
        stories={[]}
        currentStoryId=""
        tellers={[classicTeller()]}
        storyDirectors={[classicStoryDirector()]}
        onSelect={vi.fn()}
        onCreate={onCreate}
        onDelete={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '新建' }))
    fireEvent.click(screen.getByRole('button', { name: '创建' }))

    expect(await screen.findByRole('button', { name: '正在适配状态结构' })).toBeDisabled()
    await act(async () => rejectCreate?.(new Error('状态结构校验失败')))
    expect(await screen.findByText('状态结构校验失败')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '创建' })).toBeEnabled()
  })
})

function classicTeller() {
  return {
    version: 5,
    id: 'classic',
    name: '经典叙事',
    description: '',
    context_policy: {
      creator: 'always',
      lore: 'relevant',
      runtime_state: 'always',
    },
    orchestration: {
      enabled: true,
      opening: {
        enabled: true,
        trait_pools: [{
          id: 'talent',
          name: '天赋',
          draw_count: 1,
          traits: [
            { id: 'hidden-bloodline', name: '隐脉', summary: '灵力上限更高' },
            { id: 'poor-family', name: '寒门', summary: '初始资源更少' },
          ],
        }],
        initial_state_ops: [],
      },
    },
    slots: [],
    custom: false,
  }
}

function classicStoryDirector(): StoryDirector {
  return {
    version: 1,
    id: 'default',
    name: '默认导演',
    description: '',
    strategy: { enabled: true },
    trpg_system: {},
    actor_state: {
      templates: [{ id: 'protagonist', name: '主角', trait_rules: [{ pool_id: 'talent', draw_count: 1 }] }],
      initial_actors: [{ id: 'protagonist', name: '主角', template_id: 'protagonist' }],
      trait_pools: [{
        id: 'talent',
        name: '天赋',
        traits: [
          { id: 'hidden-bloodline', name: '隐脉', summary: '灵力上限更高', weight: 1, visibility: 'visible' },
          { id: 'poor-family', name: '寒门', summary: '初始资源更少', weight: 1, visibility: 'visible' },
        ],
      }],
    },
    custom: false,
  }
}

function detachedStoryDirector(): StoryDirector {
  return {
    ...classicStoryDirector(),
    id: 'detached',
    name: '关闭模块导演',
    module_refs: {
      narrative_style_id: 'alt-style',
      narrative_style_disabled: true,
      event_system_id: 'default',
      event_system_disabled: true,
      rule_system_id: 'default',
      rule_system_disabled: true,
      actor_state_id: 'default',
      actor_state_disabled: true,
      image_preset_id: 'ink-wash',
      image_preset_disabled: true,
    },
    actor_state: {
      templates: [{ id: 'protagonist', name: '主角', trait_rules: [{ pool_id: 'talent', draw_count: 1 }] }],
      trait_pools: [{
        id: 'talent',
        name: '天赋',
        traits: [{ id: 'hidden-bloodline', name: '隐脉', summary: '灵力上限更高', weight: 1, visibility: 'visible' }],
      }],
      initial_actors: [],
    },
  }
}

function story(id: string, title: string) {
  return {
    id,
    title,
    origin: '',
    story_teller_id: 'classic',
    story_director_id: 'default',
    reply_target_chars: 900,
    opening: { mode: 'ai' as const },
    created_at: '',
    updated_at: '',
    branches: 1,
    events: 1,
  }
}
