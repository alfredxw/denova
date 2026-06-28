import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getImagePresets, getInteractiveTellers } from '../api'
import type { ImagePreset, Teller } from '../types'
import { SettingPanel } from './SettingPanel'

const { configManagerChatProps } = vi.hoisted(() => ({
  configManagerChatProps: [] as Array<{
    origin?: string
    resourceId?: string
    onMutated?: () => void
  }>,
}))

vi.mock('@/components/Chat/ConfigManagerChat', () => ({
  ConfigManagerChat: (props: {
    origin?: string
    resourceId?: string
    onMutated?: () => void
  }) => {
    configManagerChatProps.push(props)
    return (
      <div data-testid="config-manager-chat">
        <button type="button" onClick={() => props.onMutated?.()}>mock mutation</button>
      </div>
    )
  },
}))

vi.mock('@/lib/api', () => ({
  createLoreItem: vi.fn(),
  deleteLoreItem: vi.fn(),
  getLoreItems: vi.fn().mockResolvedValue([]),
  readFile: vi.fn().mockResolvedValue({ content: '' }),
  saveFile: vi.fn(),
  updateLoreItem: vi.fn(),
}))

vi.mock('../api', () => ({
  createImagePreset: vi.fn(),
  createInteractiveTeller: vi.fn(),
  deleteImagePreset: vi.fn(),
  deleteInteractiveTeller: vi.fn(),
  getImagePresets: vi.fn(),
  getInteractiveTellers: vi.fn(),
  updateImagePreset: vi.fn(),
  updateInteractiveTeller: vi.fn(),
}))

describe('SettingPanel', () => {
  beforeEach(() => {
    configManagerChatProps.length = 0
    vi.mocked(getInteractiveTellers).mockReset()
    vi.mocked(getImagePresets).mockReset()
    vi.mocked(getInteractiveTellers).mockResolvedValue([teller('classic', '经典叙事'), teller('slow-burn', '慢热叙事')])
    vi.mocked(getImagePresets).mockResolvedValue([imagePreset('game-cg', '游戏 CG')])
  })

  it('keeps the presets config Agent open after its tools refresh narrative plans', async () => {
    const user = userEvent.setup()
    render(<PresetPanelHarness />)

    await user.click(screen.getByRole('button', { name: '配置管理 Agent' }))
    expect(screen.getByTestId('config-manager-chat')).toBeInTheDocument()
    expect(configManagerChatProps.at(-1)).toMatchObject({
      origin: 'teller',
      resourceId: '__config_manager_teller__',
    })

    await user.click(screen.getByRole('button', { name: 'mock mutation' }))

    await waitFor(() => {
      expect(getInteractiveTellers).toHaveBeenCalled()
      expect(screen.getByTestId('config-manager-chat')).toBeInTheDocument()
    })
    expect(screen.getAllByText('配置管理 Agent').length).toBeGreaterThan(0)
  })

  it('opens the presets config Agent without leaving the image presets tab', async () => {
    const user = userEvent.setup()
    render(<PresetPanelHarness />)

    const imageTab = screen.getByRole('button', { name: '图像方案' })
    await user.click(imageTab)
    expect(imageTab).toHaveClass('bg-[var(--nova-active)]')

    await user.click(screen.getByRole('button', { name: '配置管理 Agent' }))

    expect(screen.getByTestId('config-manager-chat')).toBeInTheDocument()
    expect(imageTab).toHaveClass('bg-[var(--nova-active)]')
    expect(screen.queryByRole('heading', { name: '经典叙事' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /游戏 CG/ }))

    expect(screen.queryByTestId('config-manager-chat')).not.toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '游戏 CG' })).toBeInTheDocument()
    expect(imageTab).toHaveClass('bg-[var(--nova-active)]')
  })
})

function PresetPanelHarness() {
  const [tellers, setTellers] = useState([teller('classic', '经典叙事')])
  const [imagePresets, setImagePresets] = useState([imagePreset('game-cg', '游戏 CG')])

  return (
    <SettingPanel
      mode="teller"
      workspace="/workspace"
      tellers={tellers}
      imagePresets={imagePresets}
      onTellersChange={setTellers}
      onImagePresetsChange={setImagePresets}
    />
  )
}

function teller(id: string, name: string): Teller {
  return {
    version: 1,
    id,
    name,
    description: `${name} description`,
    random_event_rate: 0.15,
    style_rules: [],
    tags: [],
    context_policy: { creator: 'always', lore: 'relevant', runtime_state: 'always' },
    slots: [{ id: 'identity', name: '系统提示', target: 'system', enabled: true, content: 'rules' }],
    custom: id !== 'classic',
  }
}

function imagePreset(id: string, name: string): ImagePreset {
  return {
    version: 2,
    id,
    name,
    description: `${name} description`,
    prompt: '## 图像请求 Prompt（tool_request）\n\nvisual prompt',
    slots: [{ id: 'tool_request', name: '图像请求 Prompt', target: 'tool_request', enabled: true, content: 'visual prompt' }],
    tags: [],
    custom: id !== 'game-cg',
  }
}
