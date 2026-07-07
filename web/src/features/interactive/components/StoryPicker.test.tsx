import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { StoryPicker } from './StoryPicker'

const { rollInteractiveOpeningMock } = vi.hoisted(() => ({
  rollInteractiveOpeningMock: vi.fn(),
}))

vi.mock('../api', () => ({
  rollInteractiveOpening: rollInteractiveOpeningMock,
}))

beforeEach(() => {
  rollInteractiveOpeningMock.mockReset()
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
            random_event_rate: 0.15,
            tags: [],
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

    expect(screen.getByText('当前故事导演已关闭开局选择器；新故事不会从导演抽取开局词条。')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '抽取词条' })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: '抽取词条' }))
    expect(rollInteractiveOpeningMock).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: '创建' }))

    expect(onCreate).toHaveBeenCalledWith(expect.objectContaining({
      story_teller_id: 'classic',
      story_director_id: 'detached',
      image_settings: expect.objectContaining({ preset_id: 'game-cg' }),
      initial_state_ops: undefined,
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

  it('rolls opening traits and includes initial state ops in create input', async () => {
    const onCreate = vi.fn()
    rollInteractiveOpeningMock.mockResolvedValue({
      teller_id: 'classic',
      seed: 42,
      traits: [{ pool_id: 'talent', id: 'hidden-bloodline', name: '隐脉', summary: '灵力上限更高' }],
      state_ops: [{ op: 'set', path: 'resources.hp', value: 18 }],
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

    await waitFor(() => expect(rollInteractiveOpeningMock).toHaveBeenCalledWith({ story_director_id: 'default', selected_trait_ids: [] }))
    await waitFor(() => expect(screen.getAllByText('隐脉').length).toBeGreaterThan(0))

    fireEvent.click(screen.getByRole('button', { name: '创建' }))

    expect(onCreate).toHaveBeenCalledWith(expect.objectContaining({
      initial_state_ops: [{ op: 'set', path: 'resources.hp', value: 18 }],
    }))
  })

  it('applies manually selected opening traits', async () => {
    rollInteractiveOpeningMock.mockResolvedValue({
      teller_id: 'classic',
      seed: 42,
      traits: [{ pool_id: 'talent', id: 'hidden-bloodline', name: '隐脉', summary: '灵力上限更高' }],
      state_ops: [],
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

    await waitFor(() => expect(rollInteractiveOpeningMock).toHaveBeenCalledWith({ story_director_id: 'default', selected_trait_ids: ['hidden-bloodline'] }))
  })
})

function classicTeller() {
  return {
    version: 5,
    id: 'classic',
    name: '经典叙事',
    description: '',
    random_event_rate: 0.15,
    tags: [],
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

function classicStoryDirector() {
  return {
    version: 1,
    id: 'default',
    name: '默认导演',
    description: '',
    strategy: { enabled: true },
    event_system: {},
    trpg_system: {},
    opening_selector: {
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
    tags: [],
    custom: false,
  }
}

function detachedStoryDirector() {
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
      opening_selector_id: 'default',
      opening_selector_disabled: true,
      image_preset_id: 'ink-wash',
      image_preset_disabled: true,
    },
    opening_selector: {
      enabled: true,
      trait_pools: [{
        id: 'talent',
        name: '天赋',
        draw_count: 1,
        traits: [{ id: 'hidden-bloodline', name: '隐脉', summary: '灵力上限更高' }],
      }],
      initial_state_ops: [],
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
