import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { VirtuosoMockContext } from 'react-virtuoso'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { StoryStage } from './StoryStage'
import type { StorySummary } from '../types'

const { generateInteractiveImageMock } = vi.hoisted(() => ({
  generateInteractiveImageMock: vi.fn(),
}))

vi.mock('@/features/settings/api', () => ({
  fetchSettings: vi.fn().mockResolvedValue({ effective: {} }),
}))

vi.mock('@/hooks/useSkillCommands', () => ({
  useSkillCommands: () => [],
}))

vi.mock('../api', () => ({
  abortInteractiveChat: vi.fn(),
  analyzeInteractiveContext: vi.fn(),
  compactInteractiveContext: vi.fn(),
  generateInteractiveHotChoices: vi.fn(),
  generateInteractiveImage: generateInteractiveImageMock,
  removeInteractiveContextCompaction: vi.fn(),
  sendInteractiveMessage: vi.fn(),
  switchInteractiveTurnVersion: vi.fn(),
}))

beforeEach(() => {
  generateInteractiveImageMock.mockReset()
})

describe('StoryStage interactive image settings', () => {
  it('sets interactive image mode from the input actions submenu', async () => {
    const user = userEvent.setup()
    const handleImageSettingsChange = vi.fn().mockResolvedValue(undefined)
    render(
      <StoryStage
        workspace="/tmp/book"
        stories={[story()]}
        story={story()}
        tellers={[]}
        storyId="story-1"
        branchId="main"
        snapshot={{ story_id: 'story-1', branch_id: 'main', turns: [], state: {} }}
        onDone={() => {}}
        onImageSettingsChange={handleImageSettingsChange}
      />,
    )

    fireEvent.pointerDown(screen.getByRole('button', { name: '输入动作' }))
    await waitFor(() => expect(screen.getByText('互动图像')).toBeInTheDocument())
    await user.hover(screen.getByRole('menuitem', { name: /互动图像/ }))
    await waitFor(() => expect(screen.getByRole('menuitem', { name: /每 3 轮生成/ })).toBeInTheDocument())
    fireEvent.click(screen.getByRole('menuitem', { name: /每 3 轮生成/ }))

    await waitFor(() => {
      expect(handleImageSettingsChange).toHaveBeenCalledWith({ mode: 'interval', interval_turns: 3, preset_id: 'game-cg' })
    })
  })
})

describe('StoryStage interactive image rendering', () => {
  it('手动生成互动图像成功后不等刷新快照也会立即渲染到对应回合', async () => {
    const user = userEvent.setup()
    const handleDone = vi.fn().mockResolvedValue(undefined)
    generateInteractiveImageMock.mockResolvedValue({
      enabled: true,
      image: {
        schema: 'interactive_image.v1',
        story_id: 'story-1',
        branch_id: 'main',
        turn_id: 'turn-1',
        image_path: 'assets/interactive/images/story-1/main/turn-1/run-a/image.png',
        meta_path: 'assets/interactive/images/story-1/main/turn-1/run-a/meta.json',
        alt_text: '即时互动图像',
      },
    })

    render(
      <VirtuosoMockContext.Provider value={{ viewportHeight: 1200, itemHeight: 120 }}>
        <StoryStage
          workspace="/tmp/book"
          stories={[story()]}
          story={story()}
          tellers={[]}
          storyId="story-1"
          branchId="main"
          snapshot={{
            story_id: 'story-1',
            branch_id: 'main',
            state: {},
            turns: [{
              id: 'turn-1',
              parent_id: null,
              branch_id: 'main',
              ts: '2026-06-28T00:00:00Z',
              user: '继续前进',
              narrative: '玄璃抬头，看见雾气里有一道微光。',
            }],
          }}
          onDone={handleDone}
        />
      </VirtuosoMockContext.Provider>,
    )

    await user.click(screen.getByRole('button', { name: '生成互动图像' }))

    await waitFor(() => {
      expect(screen.getByRole('img', { name: '即时互动图像' })).toHaveAttribute('src', '/api/workspace/asset?path=assets%2Finteractive%2Fimages%2Fstory-1%2Fmain%2Fturn-1%2Frun-a%2Fimage.png')
    })
    expect(handleDone).toHaveBeenCalled()
  })
})

describe('StoryStage opening panel', () => {
  it('fills custom opening text from the book preset button', () => {
    render(
      <StoryStage
        workspace="/tmp/book"
        stories={[story()]}
        story={story()}
        tellers={[]}
        storyId="story-1"
        branchId="main"
        snapshot={{ story_id: 'story-1', branch_id: 'main', turns: [], state: {} }}
        bookOpeningPresets={[{ id: 'preset-1', title: '默认开场', content: '青石镇的雨刚刚停。' }]}
        onDone={() => {}}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '使用书籍预设' }))

    expect(screen.getByPlaceholderText('写下你想使用的开局。生成时会作为有界来源传给游戏 Agent。')).toHaveValue('青石镇的雨刚刚停。')
  })
})

function story(): StorySummary {
  return {
    id: 'story-1',
    title: '故事',
    origin: '',
    story_teller_id: 'classic',
    reply_target_chars: 2000,
    image_settings: { mode: 'manual', interval_turns: 3 },
    opening: { mode: 'ai' },
    created_at: '2026-06-27T00:00:00Z',
    updated_at: '2026-06-27T00:00:00Z',
    branches: 1,
    events: 0,
  }
}
