import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { deleteLoreItem, generateLoreItemImage, getLoreItems, streamLoreImagesGenerate, updateLoreItem, type LoreItem } from '@/lib/api'
import { createActorState, createImagePreset, createInteractiveTeller, createStoryDirector, createStoryMemoryStructure, deleteActorState, deleteImagePreset, deleteInteractiveTeller, deleteStoryDirector, deleteStoryMemoryStructurePreset, getActorStates, getEventPackages, getImagePresets, getInteractiveTellers, getRuleSystems, getStoryDirectors, getStoryMemoryStructures, getStyleReferences, updateActorState, updateEventPackage, updateImagePreset, updateInteractiveTeller, updateRuleSystem, updateStoryDirector, updateStoryMemoryStructure } from '../api'
import type { EventPackageModule, ImagePreset, RuleSystemModule, StoryDirector, StoryMemoryStructureModule, Teller } from '../types'
import { defaultRuleTemplates } from './preset-config/ruleTemplates'
import { newRuleSystemDraft } from './setting-panel/presetResources'
import { SettingPanel } from './SettingPanel'

const { configManagerChatProps, monacoEditorActions } = vi.hoisted(() => ({
  configManagerChatProps: [] as Array<{
    origin?: string
    resourceId?: string
    onMutated?: () => void
  }>,
  monacoEditorActions: [] as string[],
}))

vi.mock('@monaco-editor/react', () => ({
  Editor: ({ value, onChange, onMount, language, theme, options }: {
    value?: string
    onChange?: (value?: string) => void
    onMount?: (
      editor: {
        addCommand: (command: number, callback: () => void) => void
        focus: () => void
        getAction: (id: string) => { run: () => void }
      },
      monaco: { KeyMod: { CtrlCmd: number }; KeyCode: { KeyS: number } }
    ) => void
    language?: string
    theme?: string
    options?: { ariaLabel?: string; wordWrap?: string }
  }) => {
    onMount?.(
      {
        addCommand: () => undefined,
        focus: () => undefined,
        getAction: (id: string) => ({
          run: () => {
            monacoEditorActions.push(id)
          },
        }),
      },
      { KeyMod: { CtrlCmd: 1 }, KeyCode: { KeyS: 2 } },
    )

    return (
      <textarea
        aria-label={options?.ariaLabel}
        data-testid="monaco-json-editor"
        data-language={language}
        data-theme={theme}
        data-word-wrap={options?.wordWrap}
        value={value}
        onChange={(event) => onChange?.(event.target.value)}
      />
    )
  },
}))

vi.mock('next-themes', () => ({
  useTheme: () => ({ resolvedTheme: 'dark' }),
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
  abortLoreImagesGenerate: vi.fn(),
  clearLoreItemImage: vi.fn(),
  createLoreItem: vi.fn(),
  deleteLoreItem: vi.fn(),
  generateLoreItemImage: vi.fn(),
  getLoreItems: vi.fn().mockResolvedValue([]),
  readFile: vi.fn().mockResolvedValue({ content: '' }),
  saveFile: vi.fn(),
  streamLoreImagesGenerate: vi.fn(),
  updateLoreItem: vi.fn(),
  workspaceAssetURL: (path: string) => `/api/workspace/asset?path=${encodeURIComponent(path)}`,
}))

vi.mock('../api', () => ({
  createActorState: vi.fn(),
  createEventPackage: vi.fn(),
  createImagePreset: vi.fn(),
  createInteractiveTeller: vi.fn(),
  createRuleSystem: vi.fn(),
  createStoryDirector: vi.fn(),
  createStoryMemoryStructure: vi.fn(),
  deleteActorState: vi.fn(),
  deleteEventPackage: vi.fn(),
  deleteImagePreset: vi.fn(),
  deleteInteractiveTeller: vi.fn(),
  deleteRuleSystem: vi.fn(),
  deleteStoryDirector: vi.fn(),
  deleteStoryMemoryStructurePreset: vi.fn(),
  getActorStates: vi.fn(),
  getEventPackages: vi.fn(),
  getImagePresets: vi.fn(),
  getInteractiveTellers: vi.fn(),
  getRuleSystems: vi.fn(),
  getStoryDirectors: vi.fn(),
  getStoryMemoryStructures: vi.fn(),
  getStyleReferences: vi.fn(),
  saveStyleReference: vi.fn(),
  updateEventPackage: vi.fn(),
  updateImagePreset: vi.fn(),
  updateInteractiveTeller: vi.fn(),
  updateActorState: vi.fn(),
  updateRuleSystem: vi.fn(),
  updateStoryDirector: vi.fn(),
  updateStoryMemoryStructure: vi.fn(),
}))

describe('SettingPanel', () => {
  beforeEach(() => {
    window.localStorage.clear()
    configManagerChatProps.length = 0
    monacoEditorActions.length = 0
    vi.mocked(getLoreItems).mockReset()
    vi.mocked(updateLoreItem).mockReset()
    vi.mocked(deleteLoreItem).mockReset()
    vi.mocked(generateLoreItemImage).mockReset()
    vi.mocked(streamLoreImagesGenerate).mockReset()
    vi.mocked(getInteractiveTellers).mockReset()
    vi.mocked(createInteractiveTeller).mockReset()
    vi.mocked(updateInteractiveTeller).mockReset()
    vi.mocked(deleteInteractiveTeller).mockReset()
    vi.mocked(getImagePresets).mockReset()
    vi.mocked(createImagePreset).mockReset()
    vi.mocked(getStoryDirectors).mockReset()
    vi.mocked(createStoryDirector).mockReset()
    vi.mocked(updateStoryDirector).mockReset()
    vi.mocked(deleteStoryDirector).mockReset()
    vi.mocked(getStoryMemoryStructures).mockReset()
    vi.mocked(createStoryMemoryStructure).mockReset()
    vi.mocked(updateStoryMemoryStructure).mockReset()
    vi.mocked(deleteStoryMemoryStructurePreset).mockReset()
    vi.mocked(getActorStates).mockReset()
    vi.mocked(createActorState).mockReset()
    vi.mocked(updateActorState).mockReset()
    vi.mocked(deleteActorState).mockReset()
    vi.mocked(getEventPackages).mockReset()
    vi.mocked(updateEventPackage).mockReset()
    vi.mocked(getRuleSystems).mockReset()
    vi.mocked(updateRuleSystem).mockReset()
    vi.mocked(getStyleReferences).mockReset()
    vi.mocked(updateImagePreset).mockReset()
    vi.mocked(deleteImagePreset).mockReset()
    vi.mocked(getLoreItems).mockResolvedValue([])
    vi.mocked(getInteractiveTellers).mockResolvedValue([teller('classic', '经典叙事'), teller('slow-burn', '慢热叙事')])
    vi.mocked(updateInteractiveTeller).mockImplementation(async (id, input) => ({ ...teller(id, input.name || id), ...input, id, custom: id !== 'classic', builtin_overridden: id === 'classic', updated_at: '2026-01-01T00:00:01Z' }) as Teller)
    vi.mocked(deleteInteractiveTeller).mockResolvedValue(undefined)
    vi.mocked(getStoryDirectors).mockResolvedValue([storyDirector('default', '默认导演')])
    vi.mocked(createStoryDirector).mockResolvedValue(storyDirector('default-custom', '默认导演'))
    vi.mocked(updateStoryDirector).mockImplementation(async (id, input) => ({ ...storyDirector(id, input.name || id), ...input, id, custom: id !== 'default', builtin_overridden: id === 'default', updated_at: '2026-01-01T00:00:01Z' }) as StoryDirector)
    vi.mocked(deleteStoryDirector).mockResolvedValue(undefined)
    vi.mocked(getStoryMemoryStructures).mockResolvedValue([memoryStructure('default', '默认记忆结构')])
    vi.mocked(createStoryMemoryStructure).mockResolvedValue(memoryStructure('custom-memory', '自定义记忆结构'))
    vi.mocked(updateStoryMemoryStructure).mockImplementation(async (id, input) => ({ ...memoryStructure(id, input.name || id), ...input, id, custom: id !== 'default', builtin_overridden: id === 'default', updated_at: '2026-01-01T00:00:01Z' }) as StoryMemoryStructureModule)
    vi.mocked(deleteStoryMemoryStructurePreset).mockResolvedValue(undefined)
    vi.mocked(getActorStates).mockResolvedValue([])
    vi.mocked(deleteActorState).mockResolvedValue(undefined)
    vi.mocked(getImagePresets).mockResolvedValue([imagePreset('game-cg', '游戏 CG')])
    vi.mocked(updateImagePreset).mockImplementation(async (id, input) => ({ ...imagePreset(id, input.name || id), ...input, id, custom: id !== 'game-cg', builtin_overridden: id === 'game-cg', updated_at: '2026-01-01T00:00:01Z' }) as ImagePreset)
    vi.mocked(deleteImagePreset).mockResolvedValue(undefined)
    vi.mocked(getEventPackages).mockResolvedValue([eventPackage('default', '默认事件包')])
    vi.mocked(updateEventPackage).mockImplementation(async (id, input) => ({ ...eventPackage(id, input.name || id), ...input, id, custom: id !== 'default', builtin_overridden: id === 'default', updated_at: '2026-01-01T00:00:01Z' }) as EventPackageModule)
    vi.mocked(getRuleSystems).mockResolvedValue([
      ruleSystem('default', '均衡 DM 检定'),
      ruleSystem('dm-fail-forward', '推进型 DM：失败也前进'),
      ruleSystem('dm-osr-player-skill', 'OSR 型 DM：玩家技巧优先'),
    ])
    vi.mocked(updateRuleSystem).mockImplementation(async (id, input) => ({ ...ruleSystem(id, input.name || id), ...input, id, custom: !isBuiltinRuleSystemID(id), builtin_overridden: isBuiltinRuleSystemID(id), updated_at: '2026-01-01T00:00:01Z' }) as RuleSystemModule)
    vi.mocked(getStyleReferences).mockResolvedValue([])
  })

  it('keeps the presets config Agent open after its tools refresh narrative styles', async () => {
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

  it('overrides a built-in narrative style in place instead of copying it', async () => {
    const user = userEvent.setup()
    render(<PresetPanelHarness />)

    await user.click(screen.getByRole('button', { name: /经典叙事/ }))
    fireEvent.change(screen.getByDisplayValue('经典叙事'), { target: { value: '覆盖后的经典叙事' } })
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => expect(updateInteractiveTeller).toHaveBeenCalled())
    expect(updateInteractiveTeller).toHaveBeenCalledWith(
      'classic',
      expect.objectContaining({
        id: 'classic',
        name: '覆盖后的经典叙事',
        custom: false,
      }),
      '',
      '/workspace',
    )
    expect(createInteractiveTeller).not.toHaveBeenCalled()
  })

  it('flushes a pending preset autosave before switching resource types', async () => {
    const user = userEvent.setup()
    render(<PresetPanelHarness />)

    await user.click(screen.getByRole('button', { name: /经典叙事/ }))
    fireEvent.change(screen.getByDisplayValue('经典叙事'), { target: { value: '切换前自动保存' } })
    await user.click(screen.getByRole('button', { name: '图像方案' }))
    await user.click(screen.getByRole('button', { name: /游戏 CG/ }))

    await waitFor(() => expect(updateInteractiveTeller).toHaveBeenCalled())
    expect(updateInteractiveTeller).toHaveBeenCalledWith(
      'classic',
      expect.objectContaining({
        id: 'classic',
        name: '切换前自动保存',
      }),
      '',
      '/workspace',
    )
    expect(screen.getByRole('heading', { name: '游戏 CG' })).toBeInTheDocument()
  })

  it('round-trips a custom preset through autosave with its latest content and revision', async () => {
    vi.useFakeTimers()
    const customA = { ...teller('custom-a', '自定义 A'), updated_at: 'a-r1' }
    const customB = { ...teller('custom-b', '自定义 B'), updated_at: 'b-r1' }
    let resolveFirstSave!: (saved: Teller) => void
    const firstSave = new Promise<Teller>((resolve) => { resolveFirstSave = resolve })
    vi.mocked(updateInteractiveTeller)
      .mockImplementationOnce(async () => firstSave)
      .mockImplementation(async (id, input) => ({
      ...teller(id, input.name || id),
      ...input,
      id,
      custom: true,
      updated_at: id === 'custom-a' ? 'a-r3' : 'b-r2',
    }) as Teller)

    try {
      render(<CustomTellerRoundTripHarness initialTellers={[customA, customB]} />)

      fireEvent.change(screen.getByDisplayValue('自定义 A'), { target: { value: 'A 首次保存' } })
      await act(async () => { await vi.advanceTimersByTimeAsync(1300) })
      expect(updateInteractiveTeller).toHaveBeenCalledTimes(1)

      fireEvent.change(screen.getByDisplayValue('A 首次保存'), { target: { value: 'A 最新内容' } })
      await act(async () => {
        resolveFirstSave({ ...customA, name: 'A 首次保存', updated_at: 'a-r2' })
        await firstSave
      })
      expect(screen.getByDisplayValue('A 最新内容')).toBeInTheDocument()

      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: /自定义 B/ }))
        await Promise.resolve()
      })
      expect(screen.getByRole('heading', { name: '自定义 B' })).toBeInTheDocument()

      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: /A 最新内容/ }))
        await Promise.resolve()
      })
      expect(screen.getByDisplayValue('A 最新内容')).toBeInTheDocument()

      expect(updateInteractiveTeller).toHaveBeenCalledTimes(2)
      expect(vi.mocked(updateInteractiveTeller).mock.calls[0][2]).toBe('a-r1')
      expect(vi.mocked(updateInteractiveTeller).mock.calls[1][2]).toBe('a-r2')
    } finally {
      vi.useRealTimers()
    }
  })

  it('restores an overridden built-in narrative style from the top-right action', async () => {
    const user = userEvent.setup()
    const overridden = { ...teller('classic', '覆盖后的经典叙事'), builtin_overridden: true }
    vi.mocked(getInteractiveTellers).mockResolvedValue([teller('classic', '经典叙事')])
    render(
      <SettingPanel
        mode="teller"
        workspace="/workspace"
        tellers={[overridden]}
        storyDirectors={[storyDirector('default', '默认导演')]}
        imagePresets={[imagePreset('game-cg', '游戏 CG')]}
      />,
    )

    expect(screen.getAllByText('内置覆盖').length).toBeGreaterThan(0)
    await user.click(screen.getByRole('button', { name: '恢复内置' }))

    await waitFor(() => {
      expect(deleteInteractiveTeller).toHaveBeenCalledWith('classic')
    })
    expect(getInteractiveTellers).toHaveBeenCalled()
  })

  it('overrides and restores a built-in image preset in place', async () => {
    const user = userEvent.setup()
    vi.mocked(getImagePresets).mockResolvedValue([imagePreset('game-cg', '游戏 CG')])
    render(<PresetPanelHarness />)

    await user.click(screen.getByRole('button', { name: '图像方案' }))
    await user.click(screen.getByRole('button', { name: /游戏 CG/ }))
    fireEvent.change(screen.getByDisplayValue('游戏 CG'), { target: { value: '覆盖后的图像方案' } })
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => expect(updateImagePreset).toHaveBeenCalled())
    expect(updateImagePreset).toHaveBeenCalledWith(
      'game-cg',
      expect.objectContaining({
        id: 'game-cg',
        name: '覆盖后的图像方案',
        custom: false,
      }),
      '',
      '/workspace',
    )
    expect(createImagePreset).not.toHaveBeenCalled()
    expect(screen.getAllByText('内置覆盖').length).toBeGreaterThan(0)

    await user.click(screen.getByRole('button', { name: '恢复内置' }))

    await waitFor(() => {
      expect(deleteImagePreset).toHaveBeenCalledWith('game-cg')
    })
    expect(getImagePresets).toHaveBeenCalled()
  })

  it('opens the presets config Agent without leaving the expanded image preset group', async () => {
    const user = userEvent.setup()
    render(<PresetPanelHarness />)

    await user.click(screen.getByRole('button', { name: '图像方案' }))
    await user.click(screen.getByRole('button', { name: /游戏 CG/ }))
    expect(screen.getByRole('heading', { name: '游戏 CG' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '配置管理 Agent' }))

    expect(screen.getByTestId('config-manager-chat')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /游戏 CG/ })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: '经典叙事' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /游戏 CG/ }))

    expect(screen.queryByTestId('config-manager-chat')).not.toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '游戏 CG' })).toBeInTheDocument()
  })

  it('follows the global mode when filtering preset module types', async () => {
    const user = userEvent.setup()
    render(<PresetModeHarness />)

    expect(screen.queryByLabelText('方案预设模式筛选')).not.toBeInTheDocument()
    expect(screen.getByTestId('preset-directory-scroll')).toHaveClass('h-0', 'overflow-hidden', 'overscroll-y-contain')
    expect(screen.queryByText('在目录中选择条目，右侧打开编辑。')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '配置管理 Agent' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '叙事风格' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '图像方案' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '故事导演' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /默认导演/ })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /经典叙事/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '新建故事导演' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '新建叙事风格' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /默认事件包/ })).not.toBeInTheDocument()
    expect(sectionHeader('故事导演').compareDocumentPosition(sectionHeader('叙事风格')) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(sectionHeader('故事导演').compareDocumentPosition(sectionHeader('图像方案')) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()

    await user.click(screen.getByRole('button', { name: '展开全部目录' }))
    expect(screen.getByRole('button', { name: /默认导演/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /默认事件包/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /均衡 DM 检定/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /推进型 DM：失败也前进/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /OSR 型 DM：玩家技巧优先/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '折叠全部目录' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '折叠全部目录' }))
    expect(screen.queryByRole('button', { name: /默认导演/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /经典叙事/ })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '故事导演' }))

    await selectDefaultDirector(user)
    expect(screen.queryByRole('tablist', { name: '导演资源' })).not.toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: /事件引用/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: /TRPG 检定/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: /开局选择/ })).not.toBeInTheDocument()
    expect(screen.queryByTestId('preset-config-visual-editor')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '事件包' }))

    expect(screen.getByRole('button', { name: /默认事件包/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '新建事件包' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /默认事件包/ }))
    expect(screen.getByRole('heading', { name: '默认事件包' })).toBeInTheDocument()
    expect(screen.getByTestId('preset-config-visual-editor')).toBeInTheDocument()
    expect(screen.queryByTestId('monaco-json-editor')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'JSON' }))
    expect(window.localStorage.getItem('nova.settingPanel.presetConfigView.v1')).toContain('event-package.events')
    const jsonEditors = screen.getAllByTestId('story-director-json-editor')
    expect(jsonEditors).toHaveLength(1)
    expect(jsonEditors[0]).toHaveClass('overflow-hidden')
    expect(screen.getByTestId('monaco-json-editor')).toHaveAttribute('data-word-wrap', 'on')
    expect(screen.getByDisplayValue(/events/)).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: '折叠全部' })).toHaveLength(1)
    expect(screen.queryByRole('button', { name: '展开全部' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '折叠全部' }))
    expect(monacoEditorActions).toEqual(['editor.foldAll'])
    expect(screen.getByRole('button', { name: '展开全部' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '展开全部' }))
    expect(monacoEditorActions).toEqual(['editor.foldAll', 'editor.unfoldAll'])
    expect(screen.getAllByRole('button', { name: '折叠全部' })).toHaveLength(1)
    expect(screen.queryByRole('button', { name: '展开全部' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '写作模式' }))

    expect(screen.getByRole('button', { name: '叙事风格' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '图像方案' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '故事导演' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '事件包' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'TRPG 检定' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '开局选择器' })).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: '默认事件包' })).not.toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '经典叙事' })).toBeInTheDocument()
  })

  it('saves visual edits from an event package card', async () => {
    const user = userEvent.setup()
    render(<PresetModeHarness />)

    await user.click(screen.getByRole('button', { name: '事件包' }))
    await user.click(await screen.findByRole('button', { name: /默认事件包/ }))
    await user.click(await screen.findByRole('button', { name: '新增事件卡' }))
    expect(screen.getByTestId('event-package-card-editor')).toHaveClass('grid')
    expect(screen.getByTestId('preset-config-visual-editor')).toHaveClass('preset-config-visual-container')
    expect(screen.getByTestId('event-package-card-editor')).toHaveClass('preset-visual-editor-shell', 'overflow-hidden')
    expect(screen.getByTestId('event-package-card-detail-scroll')).toHaveClass('p-3')
    expect(screen.getByTestId('event-package-card-detail-scroll')).not.toHaveClass('overflow-y-auto')
    await user.type(screen.getByLabelText('事件类型名'), '伏笔回收')
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => expect(updateEventPackage).toHaveBeenCalled())
    expect(updateEventPackage).toHaveBeenCalledWith('default', expect.objectContaining({ id: 'default', custom: false }), '', '/workspace')
    expect(createStoryDirector).not.toHaveBeenCalled()
    const payload = vi.mocked(updateEventPackage).mock.calls.at(-1)?.[1] as Partial<EventPackageModule>
    expect(payload.events?.[0]?.type_name).toBe('伏笔回收')
  })

  it('saves disabled story director module switches without clearing selected refs', async () => {
    const user = userEvent.setup()
    render(<PresetModeHarness />)

    await selectDefaultDirector(user)
    const eventSwitch = screen.getByRole('switch', { name: '停用事件包模块' })
    expect(eventSwitch).toBeChecked()
    await user.click(eventSwitch)
    expect(screen.getByRole('switch', { name: '启用事件包模块' })).not.toBeChecked()
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => expect(updateStoryDirector).toHaveBeenCalled())
    const payload = vi.mocked(updateStoryDirector).mock.calls.at(-1)?.[1] as Partial<StoryDirector>
    expect(payload.module_refs).toMatchObject({
      event_package_ids: ['default'],
      event_packages_disabled: true,
    })
  })

  it('selects a DM check style through the story director TRPG resource ref', async () => {
    const user = userEvent.setup()
    render(<PresetModeHarness />)

    await selectDefaultDirector(user)
    const ruleRow = screen.getAllByText('TRPG 检定')
      .map((node) => node.closest('div') as HTMLElement | null)
      .find((node): node is HTMLElement => Boolean(node && within(node).queryByRole('combobox'))) as HTMLElement
    await user.click(within(ruleRow).getByRole('combobox'))
    await user.click(screen.getByRole('option', { name: /推进型 DM：失败也前进/ }))
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => expect(updateStoryDirector).toHaveBeenCalled())
    const payload = vi.mocked(updateStoryDirector).mock.calls.at(-1)?.[1] as Partial<StoryDirector>
    expect(payload.module_refs).toMatchObject({ rule_system_id: 'dm-fail-forward' })
    expect(payload.module_refs as Record<string, unknown>).not.toHaveProperty('dm_adjudication_style')
  })

  it('uses localized enum controls for story director strategy values', async () => {
    const user = userEvent.setup()
    render(<PresetModeHarness />)

    await selectDefaultDirector(user)
    expect(screen.getByText('平衡牵引')).toBeInTheDocument()
    expect(screen.getByText('在自由行动和长期主线之间保持平衡，适合作为通用默认。')).toBeInTheDocument()
    expect(screen.getByText('可逆失败')).toBeInTheDocument()
		expect(screen.getByText('均衡')).toBeInTheDocument()
    expect(screen.queryByText('balanced')).not.toBeInTheDocument()

    const mainlineField = screen.getByText('主线强度').closest('label') as HTMLElement
    await user.click(within(mainlineField).getByRole('combobox'))
    await user.click(screen.getByRole('option', { name: /强主线/ }))

    const failureField = screen.getByText('失败策略').closest('label') as HTMLElement
    await user.click(within(failureField).getByRole('combobox'))
    await user.click(screen.getByRole('option', { name: /失败前进/ }))

    const pacingField = screen.getByText('节奏曲线').closest('label') as HTMLElement
    await user.click(within(pacingField).getByRole('combobox'))
    await user.click(screen.getByRole('option', { name: /波峰波谷/ }))

		const eventFrequencyField = screen.getByText('事件机会频率').closest('label') as HTMLElement
		await user.click(within(eventFrequencyField).getByRole('combobox'))
		await user.click(screen.getByRole('option', { name: /频繁/ }))
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => expect(updateStoryDirector).toHaveBeenCalled())
    const payload = vi.mocked(updateStoryDirector).mock.calls.at(-1)?.[1] as Partial<StoryDirector>
    expect(payload.strategy).toMatchObject({
      mainline_strength: 'strong_arc',
      failure_policy: 'fail_forward',
      pacing_curve: 'wave',
			event_frequency: 'frequent',
    })
  })

  it('saves background director planning settings', async () => {
    const user = userEvent.setup()
    render(<PresetModeHarness />)

    await selectDefaultDirector(user)
    expect(screen.getByText('分支规划回合')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /高级设置/ })).not.toBeInTheDocument()
    const statusSwitch = screen.getByRole('switch', { name: '停用状态' })
    expect(statusSwitch).toBeChecked()
    await user.click(statusSwitch)
    expect(screen.getByRole('switch', { name: '启用状态' })).not.toBeChecked()

    const branchTurnsField = screen.getByText('分支规划回合').closest('label') as HTMLElement
    const branchTurnsInput = within(branchTurnsField).getByRole('spinbutton')
    expect(branchTurnsInput).toHaveValue(5)
    fireEvent.change(branchTurnsInput, { target: { value: '7' } })

    const modeField = screen.getByText('后台导演运行方式').closest('label') as HTMLElement
    await user.click(within(modeField).getByRole('combobox'))
    await user.click(screen.getByRole('option', { name: /每回合/ }))

    const visibilityField = screen.getByText('规则可见性').closest('label') as HTMLElement
    await user.click(within(visibilityField).getByRole('combobox'))
    await user.click(screen.getByRole('option', { name: /公开掷骰/ }))

    await user.click(screen.getByRole('button', { name: '保存' }))
    await waitFor(() => expect(updateStoryDirector).toHaveBeenCalled())
    const payload = vi.mocked(updateStoryDirector).mock.calls.at(-1)?.[1] as Partial<StoryDirector>
    expect(payload.strategy).toMatchObject({
      enabled: false,
      director_agent_mode: 'every_turn',
      rule_visibility_mode: 'public_roll',
      branch_planning_turns: 7,
    })
  })

  it('saves custom director planning templates', async () => {
    const user = userEvent.setup()
    render(<PresetModeHarness />)

    await selectDefaultDirector(user)
    await user.click(screen.getByText('导演规划模板').closest('button') as HTMLElement)
    expect(screen.queryByRole('tablist', { name: '导演规划模板' })).not.toBeInTheDocument()

    const template = [
      '# 自定义导演规划',
      '',
      '## 正文Agent可读',
      '### 阶段钩子与阅读欲望',
      '钩子',
      '### 资料库锚点',
      '资料库角色与势力',
      '### 核心角色与关系张力',
      '核心角色',
      '### 重要势力与阶段阻力',
      '势力阻力',
      '### 当前场景与行动空间',
      '行动空间',
      '### 信息揭示与线索密度',
      '线索密度',
      '### 遭遇、检定与代价',
      '检定代价',
      '### 爽点、危机与反转',
      '爽点反转',
      '### 状态连续性',
      '状态',
      '### 最近分支安排',
      '最近分支',
      '### 伏笔与回收',
      '伏笔',
      '',
      '## 后台导演私密',
      '隐藏推进安排',
    ].join('\n')
    const planTemplateField = screen.getByRole('textbox', { name: /director\.md 模板/ })
    expect(planTemplateField).toHaveClass('min-h-[calc(20*1.25rem+1rem)]')
    fireEvent.change(planTemplateField, { target: { value: template } })

    await user.click(screen.getByRole('button', { name: '保存' }))
    await waitFor(() => expect(updateStoryDirector).toHaveBeenCalled())
    const payload = vi.mocked(updateStoryDirector).mock.calls.at(-1)?.[1] as Partial<StoryDirector>
    expect(payload.strategy?.planning_templates?.plan).toBe(template)
  })

  it('saves advanced Markdown strategy prompt for story directors', async () => {
    const user = userEvent.setup()
    render(<PresetModeHarness />)

    await selectDefaultDirector(user)
    await user.click(screen.getByRole('button', { name: /高级 Markdown 策略提示/ }))
    const prompt = '- 避免连续两回合使用同类型突发事件。\n- 伏笔回收前至少给一次可感知征兆。'
    fireEvent.change(screen.getByPlaceholderText(/优先制造可逆但有代价的选择/), { target: { value: prompt } })
    expect(screen.getAllByText('已启用自定义策略提示').length).toBeGreaterThan(0)

    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => expect(updateStoryDirector).toHaveBeenCalled())
    const payload = vi.mocked(updateStoryDirector).mock.calls.at(-1)?.[1] as Partial<StoryDirector>
    expect(payload.strategy?.prompt_markdown).toBe(prompt)
  })

  it('blocks saving oversized story director Markdown strategy prompts', async () => {
    const user = userEvent.setup()
    render(<PresetModeHarness />)

    await selectDefaultDirector(user)
    await user.click(screen.getByRole('button', { name: /高级 Markdown 策略提示/ }))
    fireEvent.change(screen.getByPlaceholderText(/优先制造可逆但有代价的选择/), { target: { value: 'a'.repeat((64 * 1024) + 1) } })

    await waitFor(() => expect(screen.getByRole('button', { name: '保存' })).toBeDisabled())
    expect(screen.getByText('策略提示已超过 65536 bytes（当前 65537 bytes），请缩短后再保存。')).toBeInTheDocument()
    expect(updateStoryDirector).not.toHaveBeenCalled()
  })

  it('blocks saving and preset navigation while JSON view is invalid', async () => {
    const user = userEvent.setup()
    render(<PresetModeHarness />)

    await user.click(screen.getByRole('button', { name: '事件包' }))
    await user.click(await screen.findByRole('button', { name: /默认事件包/ }))
    await user.click(screen.getByRole('button', { name: 'JSON' }))
    fireEvent.change(screen.getByTestId('monaco-json-editor'), { target: { value: '{' } })

    await waitFor(() => expect(screen.getByRole('button', { name: '保存' })).toBeDisabled())
    expect(screen.getByText('请先修复 JSON，再切回可视化视图。')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'TRPG 检定' }))
    await user.click(await screen.findByRole('button', { name: /均衡 DM 检定/ }))
    expect(screen.getByRole('heading', { name: '默认事件包' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: '均衡 DM 检定' })).not.toBeInTheDocument()
    expect(updateEventPackage).not.toHaveBeenCalled()
  })

  it('edits memory structure metadata in the shared resource identity panel', async () => {
    const user = userEvent.setup()
    render(<PresetModeHarness />)

    await user.click(screen.getByRole('button', { name: '展开全部目录' }))
    await user.click(screen.getByRole('button', { name: /默认记忆结构/ }))

    const metadata = screen.getByTestId('preset-metadata')
    const nameInput = within(metadata).getByRole('textbox', { name: '名称' })
    const descriptionInput = within(metadata).getByRole('textbox', { name: '描述' })
    expect(nameInput).toHaveValue('默认记忆结构')
    expect(descriptionInput).toHaveValue('默认记忆结构 description')
    expect(screen.getByText('编辑内置资源会保存为同 ID 覆盖；右上角可恢复内置版本。')).toBeInTheDocument()
    expect(screen.getByTestId('preset-config-visual-editor')).toHaveClass('preset-config-visual-container')
    expect(screen.getByTestId('memory-structure-editor')).toHaveClass('preset-visual-editor-shell', 'overflow-hidden')

    await user.clear(nameInput)
    await user.type(nameInput, '长期记忆结构')
    await user.clear(descriptionInput)
    await user.type(descriptionInput, '记录长期承接信息')
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => expect(updateStoryMemoryStructure).toHaveBeenCalled())
    expect(updateStoryMemoryStructure).toHaveBeenCalledWith('default', expect.objectContaining({
      name: '长期记忆结构',
      description: '记录长期承接信息',
    }), '', '/workspace')
  })

  it('edits TRPG checks through the focused DM-style visual workflow', async () => {
    const user = userEvent.setup()
    render(<PresetModeHarness />)

    await user.click(screen.getByRole('button', { name: '展开全部目录' }))
    await user.click(screen.getByRole('button', { name: /均衡 DM 检定/ }))

    expect(screen.getByRole('heading', { name: '均衡 DM 检定' })).toBeInTheDocument()
    expect(screen.getByDisplayValue('均衡 d20 检定')).toBeInTheDocument()
    expect(screen.queryByText('规则 ID')).not.toBeInTheDocument()
    expect(screen.queryByText(/安全表达式/)).not.toBeInTheDocument()
    expect(screen.queryByText('成功 StateOps')).not.toBeInTheDocument()
    expect(screen.queryByText('默认难度')).not.toBeInTheDocument()
    expect(screen.queryByText('掷骰方式')).not.toBeInTheDocument()
    expect(screen.queryByText('状态影响')).not.toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: '必须检定例子' })).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: '不要检定例子' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '新增规则' })).not.toBeInTheDocument()

    const ruleLabelInput = screen.getByRole('textbox', { name: '规则名称' })
    fireEvent.change(ruleLabelInput, { target: { value: '自定义 DM 检定' } })

    const mustCheckExamples = '守卫逼近时强行撬锁\n攻击警戒守卫'
    const skipCheckExamples = '观察空房间\n和友善同伴闲聊'
    fireEvent.change(screen.getByRole('textbox', { name: '必须检定例子' }), { target: { value: mustCheckExamples } })
    fireEvent.change(screen.getByRole('textbox', { name: '不要检定例子' }), { target: { value: skipCheckExamples } })

    await user.click(screen.getByRole('tab', { name: /如何裁定/ }))
    expect(screen.getByRole('textbox', { name: '难度判断标准' })).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: '状态影响指引' })).toBeInTheDocument()
    expect(screen.getAllByRole('combobox')).toHaveLength(1)
    expect(screen.getByDisplayValue('固定 d20')).toBeDisabled()

    const modifierField = screen.getByText('修正值').closest('label') as HTMLElement
    const modifierInput = within(modifierField).getByRole('textbox')
    fireEvent.change(modifierInput, { target: { value: '7' } })

    const failureField = screen.getByText('失败处理').closest('label') as HTMLElement
    await user.click(within(failureField).getByRole('combobox'))
    await user.click(screen.getByRole('option', { name: '明确失败' }))

    const difficultyGuidance = '目标警戒越高难度越高，主角准备充分时降低一档。'
    const stateEffectGuidance = '失败时警戒 +1，代价成功时体力 -1。'
    fireEvent.change(screen.getByRole('textbox', { name: '难度判断标准' }), { target: { value: difficultyGuidance } })
    fireEvent.change(screen.getByRole('textbox', { name: '状态影响指引' }), { target: { value: stateEffectGuidance } })

    await user.click(screen.getByRole('button', { name: '保存' }))
    await waitFor(() => expect(updateRuleSystem).toHaveBeenCalled())
    const visualPayload = vi.mocked(updateRuleSystem).mock.calls.at(-1)?.[1] as Partial<RuleSystemModule>
    expect(visualPayload.trpg_system?.rule_templates).toHaveLength(1)
    expect(visualPayload.trpg_system?.rule_templates?.[0]).toMatchObject({
      label: '自定义 DM 检定',
      dice: '1d20',
      modifier: 7,
      failure_policy: 'hard_failure',
      difficulty_guidance: difficultyGuidance,
      state_effect_guidance: stateEffectGuidance,
      must_check_examples: ['守卫逼近时强行撬锁', '攻击警戒守卫'],
      skip_check_examples: ['观察空房间', '和友善同伴闲聊'],
    })
    expect(visualPayload.trpg_system?.rule_templates?.[0]).not.toHaveProperty('impact')
    expect(visualPayload.trpg_system?.rule_templates?.[0]).not.toHaveProperty('category')
    expect(visualPayload.trpg_system?.rule_templates?.[0]).not.toHaveProperty('default_difficulty')
    expect(visualPayload.trpg_system?.rule_templates?.[0]).not.toHaveProperty('default_roll_mode')
    expect(visualPayload.trpg_system?.rule_templates?.[0]).not.toHaveProperty('success_state_ops')
  })

  it('normalizes legacy TRPG JSON into one supported DM check', async () => {
    const user = userEvent.setup()
    render(<PresetModeHarness />)

    await user.click(screen.getByRole('button', { name: '展开全部目录' }))
    await user.click(screen.getByRole('button', { name: /均衡 DM 检定/ }))

    await user.click(screen.getByRole('button', { name: 'JSON' }))
    fireEvent.change(screen.getByTestId('monaco-json-editor'), {
      target: {
        value: JSON.stringify({
          rule_templates: [{
            id: 'legacy',
            label: '旧规则',
            kind: 'dice',
            mode: 'd100_under',
            dice: '1d100',
            modifier: 3,
            difficulty: 55,
            resource_cost_path: 'resources.hp',
            success_state_ops: [],
            category: 'social',
            default_difficulty: 'hard',
            default_roll_mode: 'advantage',
            failure_policy: 'success_at_cost',
            impact: 'relationship_change',
            difficulty_guidance: '对方越抗拒越难。',
            state_effect_guidance: '成功关系 +1，失败关系 -1。',
            trigger: '谈判触发',
            must_check_examples: ['逼迫敌对 NPC 让步'],
            skip_check_examples: ['普通寒暄'],
          }, {
            id: 'legacy-extra',
            label: '旧规则 2',
            dice: '1d20',
            failure_policy: 'hard_failure',
          }],
        }, null, 2),
      },
    })
    await user.click(screen.getByRole('button', { name: '保存' }))
    await waitFor(() => expect(updateRuleSystem).toHaveBeenCalledTimes(1))
    const jsonPayload = vi.mocked(updateRuleSystem).mock.calls.at(-1)?.[1] as Partial<RuleSystemModule>
    expect(jsonPayload.trpg_system?.rule_templates).toEqual([expect.objectContaining({
      id: 'legacy',
      label: '旧规则',
      dice: '1d20',
      modifier: 3,
      failure_policy: 'success_at_cost',
      difficulty_guidance: '对方越抗拒越难。',
      state_effect_guidance: '成功关系 +1，失败关系 -1。',
      trigger: '谈判触发',
      must_check_examples: ['逼迫敌对 NPC 让步'],
      skip_check_examples: ['普通寒暄'],
    })])
    expect(jsonPayload.trpg_system?.rule_templates).toHaveLength(1)
    expect(jsonPayload.trpg_system?.rule_templates?.[0]).not.toHaveProperty('impact')
    expect(jsonPayload.trpg_system?.rule_templates?.[0]).not.toHaveProperty('kind')
    expect(jsonPayload.trpg_system?.rule_templates?.[0]).not.toHaveProperty('category')
    expect(jsonPayload.trpg_system?.rule_templates?.[0]).not.toHaveProperty('default_difficulty')
    expect(jsonPayload.trpg_system?.rule_templates?.[0]).not.toHaveProperty('default_roll_mode')
    expect(jsonPayload.trpg_system?.rule_templates?.[0]).not.toHaveProperty('resource_cost_path')
    expect(jsonPayload.trpg_system?.rule_templates?.[0]).not.toHaveProperty('success_state_ops')
  })

  it('starts custom TRPG check modules from built-in rule templates', () => {
    const draft = newRuleSystemDraft()

    expect(draft.trpg_system?.rule_templates).toEqual(defaultRuleTemplates())
  })

  it('generates a current image for one lore item from the editor', async () => {
    const user = userEvent.setup()
    const item = loreItem('lin-chuan', '林川')
    const withImage = {
      ...item,
      updated_at: '2026-01-01T00:00:01Z',
      image: loreImage('assets/lore/images/lin-chuan/20260101000000/image.png'),
    }
    vi.mocked(getLoreItems).mockResolvedValue([item])
    vi.mocked(updateLoreItem).mockResolvedValue(item)
    vi.mocked(generateLoreItemImage).mockResolvedValue(withImage)

    render(<SettingPanel mode="lore" workspace="/workspace" imagePresets={[imagePreset('game-cg', '游戏 CG')]} />)

    await user.click(await screen.findByRole('button', { name: /林川/ }))
    expect(screen.getByText('暂无图片')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '打开图片生成' }))

    const generateDialog = await screen.findByRole('dialog', { name: '生成图片' })
    await user.click(within(generateDialog).getByRole('button', { name: '生成图片' }))

    await waitFor(() => {
      expect(generateLoreItemImage).toHaveBeenCalledWith('lin-chuan', expect.objectContaining({ image_preset_id: 'game-cg' }))
    })
    await user.click(within(generateDialog).getByRole('button', { name: '关闭' }))
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })

    expect(await screen.findByRole('img', { name: '林川' })).toHaveAttribute('src', '/api/workspace/asset?path=assets%2Flore%2Fimages%2Flin-chuan%2F20260101000000%2Fimage.png')
    expect(screen.queryByText('已有图片')).not.toBeInTheDocument()
    expect(screen.queryByText('assets/lore/images/lin-chuan/20260101000000/image.png')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '放大查看资料图片' }))

    const previewDialog = screen.getByRole('dialog', { name: '林川' })
    expect(within(previewDialog).getByTestId('image-preview-viewport')).toBeInTheDocument()
  })

  it('saves lore item enabled status from a switch', async () => {
    const user = userEvent.setup()
    const item = loreItem('lin-chuan', '林川')
    vi.mocked(getLoreItems).mockResolvedValue([item])
    vi.mocked(updateLoreItem).mockImplementation(async (id, input) => ({
      ...item,
      ...input,
      id,
      updated_at: '2026-01-01T00:00:01Z',
    }) as LoreItem)

    render(<SettingPanel mode="lore" workspace="/workspace" imagePresets={[imagePreset('game-cg', '游戏 CG')]} />)

    await user.click(await screen.findByRole('button', { name: /林川/ }))
    const statusSwitch = screen.getByRole('switch', { name: '停用状态' })
    expect(statusSwitch).toBeChecked()
    await user.click(statusSwitch)
    expect(screen.getByRole('switch', { name: '启用状态' })).not.toBeChecked()

    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => expect(updateLoreItem).toHaveBeenCalled())
    expect(updateLoreItem).toHaveBeenCalledWith(
      'lin-chuan',
      expect.objectContaining({ enabled: false }),
      '2026-01-01T00:00:00Z',
    )
  })

  it('confirms lore deletion with an in-app dialog', async () => {
    const user = userEvent.setup()
    const item = loreItem('lin-chuan', '林川')
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    vi.mocked(getLoreItems).mockResolvedValueOnce([item]).mockResolvedValue([])
    vi.mocked(deleteLoreItem).mockResolvedValue(undefined)

    render(<SettingPanel mode="lore" workspace="/workspace" imagePresets={[imagePreset('game-cg', '游戏 CG')]} />)

    await user.click(await screen.findByRole('button', { name: /林川/ }))
    await user.click(screen.getByRole('button', { name: '删除资料' }))

    const dialog = await screen.findByRole('alertdialog', { name: '删除资料' })
    expect(within(dialog).getByText('删除资料「林川」？')).toBeInTheDocument()
    expect(confirmSpy).not.toHaveBeenCalled()

    await user.click(within(dialog).getByRole('button', { name: '删除' }))

    await waitFor(() => {
      expect(deleteLoreItem).toHaveBeenCalledWith('lin-chuan')
    })
    confirmSpy.mockRestore()
  })

  it('confirms narrative style deletion with an in-app dialog', async () => {
    const user = userEvent.setup()
    const customTeller = teller('custom-noir', '黑色幽默')
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    vi.mocked(getInteractiveTellers).mockResolvedValue([teller('classic', '经典叙事')])
    vi.mocked(deleteInteractiveTeller).mockResolvedValue(undefined)

    render(
      <SettingPanel
        mode="teller"
        workspace="/workspace"
        tellers={[customTeller]}
        storyDirectors={[storyDirector('default', '默认导演')]}
        imagePresets={[imagePreset('game-cg', '游戏 CG')]}
      />,
    )

    expect(await screen.findByRole('heading', { name: '黑色幽默' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '删除叙事风格' }))

    const dialog = await screen.findByRole('alertdialog', { name: '删除叙事风格' })
    expect(within(dialog).getByText('删除叙事风格「黑色幽默」？')).toBeInTheDocument()
    expect(confirmSpy).not.toHaveBeenCalled()

    await user.click(within(dialog).getByRole('button', { name: '删除' }))

    await waitFor(() => {
      expect(deleteInteractiveTeller).toHaveBeenCalledWith('custom-noir')
    })
    confirmSpy.mockRestore()
  })

  it('requires explicit multi-select before starting lore image batch generation', async () => {
    const user = userEvent.setup()
    const lin = loreItem('lin-chuan', '林川')
    const harbor = loreItem('moon-harbor', '月港', 'location')
    vi.mocked(getLoreItems).mockResolvedValue([lin, harbor])
    vi.mocked(streamLoreImagesGenerate).mockResolvedValue(new ReadableStream({
      start(controller) {
        controller.enqueue({ event: 'done', data: JSON.stringify({ generated: 1, skipped: 0, failed: 0 }) })
        controller.close()
      },
    }))

    render(<SettingPanel mode="lore" workspace="/workspace" imagePresets={[imagePreset('game-cg', '游戏 CG'), imagePreset('ink-wash', '水墨风格')]} />)

    await user.click(await screen.findByRole('button', { name: '批量生成资料图片' }))
    const batchDialog = await screen.findByRole('dialog', { name: '批量生成资料图片' })
    const presetField = within(batchDialog).getByText('图像方案').closest('label') as HTMLElement
    await user.click(within(presetField).getByRole('combobox'))
    await user.click(screen.getByRole('option', { name: '水墨风格' }))
    await user.type(screen.getByPlaceholderText('搜索资料项'), '林川')
    await user.click(screen.getByRole('button', { name: '全选当前结果' }))
    await user.click(screen.getByRole('button', { name: '开始生成' }))

    await waitFor(() => {
      expect(streamLoreImagesGenerate).toHaveBeenCalledWith(expect.objectContaining({
        item_ids: ['lin-chuan'],
        overwrite_existing: false,
        image_preset_id: 'ink-wash',
      }), expect.any(AbortSignal))
    })
  })
})

function sectionHeader(name: string) {
  return screen.getByRole('button', { name })
}

async function selectDefaultDirector(user: ReturnType<typeof userEvent.setup>) {
  if (!screen.queryByRole('button', { name: /默认导演/ })) {
    await user.click(screen.getByRole('button', { name: '故事导演' }))
  }
  await user.click(screen.getByRole('button', { name: /默认导演/ }))
}

function PresetModeHarness() {
  const [presetUsageMode, setPresetUsageMode] = useState<'writing' | 'game'>('game')
  return (
    <>
      <button type="button" onClick={() => setPresetUsageMode('writing')}>写作模式</button>
      <PresetPanelHarness presetUsageMode={presetUsageMode} />
    </>
  )
}

function PresetPanelHarness({ presetUsageMode = 'game' }: { presetUsageMode?: 'writing' | 'game' }) {
  const [tellers, setTellers] = useState([teller('classic', '经典叙事')])
  const [storyDirectors, setStoryDirectors] = useState([storyDirector('default', '默认导演')])
  const [imagePresets, setImagePresets] = useState([imagePreset('game-cg', '游戏 CG')])

  return (
    <SettingPanel
      mode="teller"
      workspace="/workspace"
      tellers={tellers}
      storyDirectors={storyDirectors}
      imagePresets={imagePresets}
      presetUsageMode={presetUsageMode}
      onTellersChange={setTellers}
      onStoryDirectorsChange={setStoryDirectors}
      onImagePresetsChange={setImagePresets}
    />
  )
}

function CustomTellerRoundTripHarness({ initialTellers }: { initialTellers: Teller[] }) {
  const [tellers, setTellers] = useState(initialTellers)
  return (
    <SettingPanel
      mode="teller"
      workspace="/workspace"
      tellers={tellers}
      storyDirectors={[storyDirector('default', '默认导演')]}
      imagePresets={[imagePreset('game-cg', '游戏 CG')]}
      onTellersChange={setTellers}
    />
  )
}

function teller(id: string, name: string): Teller {
  return {
    version: 1,
    id,
    name,
    description: `${name} description`,
    style_refs: [],
    style_rules: [],
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
    custom: id !== 'game-cg',
  }
}

function storyDirector(id: string, name: string): StoryDirector {
  return {
    version: 1,
    id,
    name,
    description: `${name} description`,
    module_refs: {
      narrative_style_id: 'classic',
      event_package_ids: ['default'],
      rule_system_id: 'default',
      actor_state_id: 'default',
      memory_structure_id: 'default',
      image_preset_id: 'game-cg',
    },
    strategy: { enabled: true, mainline_strength: 'balanced' },
    event_packages: [{
      id: 'default',
      name: '默认事件包',
      enabled: true,
      events: [],
    }],
    trpg_system: { rule_templates: [] },
    custom: id !== 'default',
  }
}

function eventPackage(id: string, name: string): EventPackageModule {
  return {
    version: 1,
    id,
    name,
    description: `${name} description`,
    events: [],
    custom: id !== 'default',
  }
}

function ruleSystem(id: string, name: string): RuleSystemModule {
  return {
    version: 1,
    id,
    name,
    description: `${name} description`,
    trpg_system: { rule_templates: defaultRuleTemplates() },
    custom: !isBuiltinRuleSystemID(id),
  }
}

const BUILTIN_RULE_SYSTEM_IDS = new Set(['default', 'dm-fail-forward', 'dm-osr-player-skill'])

function isBuiltinRuleSystemID(id: string) {
  return BUILTIN_RULE_SYSTEM_IDS.has(id)
}

function memoryStructure(id: string, name: string): StoryMemoryStructureModule {
  return {
    version: 1,
    id,
    name,
    description: `${name} description`,
    structures: [{
      id: 'plot',
      name: '剧情纪要',
      description: '',
      generation_instruction: '',
      mode: 'append',
      enabled: true,
      fields: [{ id: 'event', name: '事件', enabled: true, order: 10 }],
      order: 10,
    }],
    custom: id !== 'default',
  }
}

function loreItem(id: string, name: string, type: LoreItem['type'] = 'character'): LoreItem {
  return {
    id,
    enabled: true,
    type,
    name,
    importance: 'important',
    load_mode: 'auto',
    tags: [],
    brief_description: `${name} brief`,
    keywords: [],
    content: `## ${name}`,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  }
}

function loreImage(path: string): NonNullable<LoreItem['image']> {
  return {
    schema: 'lore_item_image.v1',
    image_path: path,
    meta_path: path.replace('/image.png', '/meta.json'),
    alt_text: '林川',
    profile_id: 'default',
    provider: 'openai',
    model: 'gpt-image-1',
    size: '2048x2048',
    output_format: 'png',
    created_at: '2026-01-01T00:00:01Z',
    size_bytes: 12,
  }
}
