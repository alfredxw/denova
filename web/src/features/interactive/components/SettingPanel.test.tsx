import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { deleteLoreItem, generateLoreItemImage, getLoreItems, streamLoreImagesGenerate, updateLoreItem, type LoreItem } from '@/lib/api'
import { createActorState, createImagePreset, createInteractiveTeller, createStoryDirector, deleteActorState, deleteImagePreset, deleteInteractiveTeller, deleteStoryDirector, getActorStates, getEventPackages, getImagePresets, getInteractiveTellers, getOpeningSelectors, getRuleSystems, getStoryDirectors, getStyleReferences, updateActorState, updateEventPackage, updateImagePreset, updateInteractiveTeller, updateOpeningSelector, updateRuleSystem, updateStoryDirector } from '../api'
import type { EventPackageModule, ImagePreset, OpeningSelectorModule, RuleSystemModule, StoryDirector, Teller } from '../types'
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
  createOpeningSelector: vi.fn(),
  createRuleSystem: vi.fn(),
  createStoryDirector: vi.fn(),
  deleteActorState: vi.fn(),
  deleteEventPackage: vi.fn(),
  deleteImagePreset: vi.fn(),
  deleteInteractiveTeller: vi.fn(),
  deleteOpeningSelector: vi.fn(),
  deleteRuleSystem: vi.fn(),
  deleteStoryDirector: vi.fn(),
  getActorStates: vi.fn(),
  getEventPackages: vi.fn(),
  getImagePresets: vi.fn(),
  getInteractiveTellers: vi.fn(),
  getOpeningSelectors: vi.fn(),
  getRuleSystems: vi.fn(),
  getStoryDirectors: vi.fn(),
  getStyleReferences: vi.fn(),
  saveStyleReference: vi.fn(),
  updateEventPackage: vi.fn(),
  updateImagePreset: vi.fn(),
  updateInteractiveTeller: vi.fn(),
  updateActorState: vi.fn(),
  updateOpeningSelector: vi.fn(),
  updateRuleSystem: vi.fn(),
  updateStoryDirector: vi.fn(),
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
    vi.mocked(getActorStates).mockReset()
    vi.mocked(createActorState).mockReset()
    vi.mocked(updateActorState).mockReset()
    vi.mocked(deleteActorState).mockReset()
    vi.mocked(getEventPackages).mockReset()
    vi.mocked(updateEventPackage).mockReset()
    vi.mocked(getRuleSystems).mockReset()
    vi.mocked(updateRuleSystem).mockReset()
    vi.mocked(getOpeningSelectors).mockReset()
    vi.mocked(updateOpeningSelector).mockReset()
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
    vi.mocked(getActorStates).mockResolvedValue([])
    vi.mocked(deleteActorState).mockResolvedValue(undefined)
    vi.mocked(getImagePresets).mockResolvedValue([imagePreset('game-cg', '游戏 CG')])
    vi.mocked(updateImagePreset).mockImplementation(async (id, input) => ({ ...imagePreset(id, input.name || id), ...input, id, custom: id !== 'game-cg', builtin_overridden: id === 'game-cg', updated_at: '2026-01-01T00:00:01Z' }) as ImagePreset)
    vi.mocked(deleteImagePreset).mockResolvedValue(undefined)
    vi.mocked(getEventPackages).mockResolvedValue([eventPackage('default', '默认事件包')])
    vi.mocked(updateEventPackage).mockImplementation(async (id, input) => ({ ...eventPackage(id, input.name || id), ...input, id, custom: id !== 'default', builtin_overridden: id === 'default', updated_at: '2026-01-01T00:00:01Z' }) as EventPackageModule)
    vi.mocked(getRuleSystems).mockResolvedValue([ruleSystem('default-rules', '默认数值规则')])
    vi.mocked(updateRuleSystem).mockImplementation(async (id, input) => ({ ...ruleSystem(id, input.name || id), ...input, id, custom: id !== 'default-rules', builtin_overridden: id === 'default-rules', updated_at: '2026-01-01T00:00:01Z' }) as RuleSystemModule)
    vi.mocked(getOpeningSelectors).mockResolvedValue([openingSelector('default-opening', '默认开局选择')])
    vi.mocked(updateOpeningSelector).mockImplementation(async (id, input) => ({ ...openingSelector(id, input.name || id), ...input, id, custom: id !== 'default-opening', builtin_overridden: id === 'default-opening', updated_at: '2026-01-01T00:00:01Z' }) as OpeningSelectorModule)
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
    )
    expect(createInteractiveTeller).not.toHaveBeenCalled()
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
    expect(screen.queryByText('在目录中选择条目，右侧打开编辑。')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '配置管理 Agent' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '叙事风格' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '图像方案' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '故事导演' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /默认导演/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /经典叙事/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '新建故事导演' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '新建叙事风格' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /默认事件包/ })).not.toBeInTheDocument()
    expect(sectionHeader('故事导演').compareDocumentPosition(sectionHeader('叙事风格')) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(sectionHeader('故事导演').compareDocumentPosition(sectionHeader('图像方案')) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()

    await user.click(screen.getByRole('button', { name: '展开全部目录' }))
    expect(screen.getByRole('button', { name: /默认事件包/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /默认数值规则/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '折叠全部目录' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '折叠全部目录' }))
    expect(screen.queryByRole('button', { name: /默认导演/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /经典叙事/ })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '故事导演' }))

    await selectDefaultDirector(user)
    const directorResourceTabs = screen.getByRole('tablist', { name: '导演资源' })
    expect(directorResourceTabs).toBeInTheDocument()
    expect(directorResourceTabs).toHaveAttribute('data-slot', 'tabs-list')
    expect(directorResourceTabs).toHaveClass('grid', 'group-data-horizontal/tabs:h-auto')
    expect(directorResourceTabs.className).toContain('grid-cols-[repeat(auto-fit,minmax(12rem,1fr))]')
    expect(screen.getByRole('tab', { name: /事件引用/ })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /数值系统/ })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /TRPG 检定/ })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /开局选择/ })).toBeInTheDocument()
    expect(screen.getAllByTestId('preset-config-visual-editor')).toHaveLength(1)
    expect(screen.queryByTestId('monaco-json-editor')).not.toBeInTheDocument()
    await user.click(screen.getAllByRole('button', { name: 'JSON' })[0])
    expect(window.localStorage.getItem('nova.settingPanel.presetConfigView.v1')).toContain('story-director.stat-system')
    const jsonEditors = screen.getAllByTestId('story-director-json-editor')
    expect(jsonEditors).toHaveLength(1)
    expect(jsonEditors[0]).toHaveClass('overflow-hidden')
    expect(screen.getByTestId('monaco-json-editor')).toHaveAttribute('data-word-wrap', 'on')
    expect(screen.getByDisplayValue(/attributes/)).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: '折叠全部' })).toHaveLength(1)
    expect(screen.queryByRole('button', { name: '展开全部' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '折叠全部' }))
    expect(monacoEditorActions).toEqual(['editor.foldAll'])
    expect(screen.getByRole('button', { name: '展开全部' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '展开全部' }))
    expect(monacoEditorActions).toEqual(['editor.foldAll', 'editor.unfoldAll'])
    expect(screen.getAllByRole('button', { name: '折叠全部' })).toHaveLength(1)
    expect(screen.queryByRole('button', { name: '展开全部' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '事件包' }))

    expect(screen.getByRole('button', { name: /默认事件包/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '新建事件包' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /默认事件包/ }))
    expect(screen.getByRole('heading', { name: '默认事件包' })).toBeInTheDocument()
    expect(screen.getByTestId('preset-config-visual-editor')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '写作模式' }))

    expect(screen.getByRole('button', { name: '叙事风格' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '图像方案' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '故事导演' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '事件包' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '数值与TRPG系统' })).not.toBeInTheDocument()
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
    expect(screen.getByTestId('event-package-card-editor').className).toContain('h-[clamp(360px,calc(100dvh-15rem),720px)]')
    expect(screen.getByTestId('event-package-card-editor')).toHaveClass('min-h-0', 'overflow-hidden')
    expect(screen.getByTestId('event-package-card-detail-scroll')).toHaveClass('overflow-y-auto')
    expect(screen.getByTestId('event-package-card-detail-scroll').className).toContain('[scrollbar-gutter:stable]')
    await user.type(screen.getByLabelText('事件类型名'), '伏笔回收')
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => expect(updateEventPackage).toHaveBeenCalled())
    expect(updateEventPackage).toHaveBeenCalledWith('default', expect.objectContaining({ id: 'default', custom: false }), '')
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

  it('uses localized enum controls for story director strategy values', async () => {
    const user = userEvent.setup()
    render(<PresetModeHarness />)

    await selectDefaultDirector(user)
    expect(screen.getByText('平衡牵引')).toBeInTheDocument()
    expect(screen.getByText('在自由行动和长期主线之间保持平衡，适合作为通用默认。')).toBeInTheDocument()
    expect(screen.getByText('可逆失败')).toBeInTheDocument()
    expect(screen.getByText('中等扰动')).toBeInTheDocument()
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

    const randomField = screen.getByText('随机事件率').closest('label') as HTMLElement
    await user.click(within(randomField).getByRole('combobox'))
    await user.click(screen.getByRole('option', { name: /高扰动/ }))
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => expect(updateStoryDirector).toHaveBeenCalled())
    const payload = vi.mocked(updateStoryDirector).mock.calls.at(-1)?.[1] as Partial<StoryDirector>
    expect(payload.strategy).toMatchObject({
      mainline_strength: 'strong_arc',
      failure_policy: 'fail_forward',
      pacing_curve: 'wave',
      random_event_rate: 0.3,
    })
  })

  it('saves background director planning settings', async () => {
    const user = userEvent.setup()
    render(<PresetModeHarness />)

    await selectDefaultDirector(user)
    expect(screen.getByText('回合后规划')).toBeInTheDocument()
    const branchTurnsField = screen.getByText('分支规划回合').closest('label') as HTMLElement
    const branchTurnsInput = within(branchTurnsField).getByRole('spinbutton')
    expect(branchTurnsInput).toHaveValue(5)
    fireEvent.change(branchTurnsInput, { target: { value: '7' } })

    const modeField = screen.getByText('后台导演运行方式').closest('label') as HTMLElement
    await user.click(within(modeField).getByRole('combobox'))
    await user.click(screen.getByRole('option', { name: /每回合/ }))

    await user.click(screen.getByRole('button', { name: '保存' }))
    await waitFor(() => expect(updateStoryDirector).toHaveBeenCalled())
    const payload = vi.mocked(updateStoryDirector).mock.calls.at(-1)?.[1] as Partial<StoryDirector>
    expect(payload.strategy).toMatchObject({
      director_agent_mode: 'every_turn',
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

    await selectDefaultDirector(user)
    await user.click(screen.getAllByRole('button', { name: 'JSON' })[0])
    fireEvent.change(screen.getByTestId('monaco-json-editor'), { target: { value: '{' } })

    await waitFor(() => expect(screen.getByRole('button', { name: '保存' })).toBeDisabled())
    expect(screen.getByText('请先修复 JSON，再切回可视化视图。')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '事件包' }))
    expect(screen.getByRole('heading', { name: '默认导演' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: '默认事件包' })).not.toBeInTheDocument()
    expect(updateStoryDirector).not.toHaveBeenCalled()
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

function teller(id: string, name: string): Teller {
  return {
    version: 1,
    id,
    name,
    description: `${name} description`,
    random_event_rate: 0.15,
    style_refs: [],
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

function storyDirector(id: string, name: string): StoryDirector {
  return {
    version: 1,
    id,
    name,
    description: `${name} description`,
    module_refs: {
      narrative_style_id: 'classic',
      event_package_ids: ['default'],
      rule_system_id: 'default-rules',
      opening_selector_id: 'default-opening',
      image_preset_id: 'game-cg',
    },
    strategy: { enabled: true, mainline_strength: 'balanced' },
    event_packages: [{
      id: 'default',
      name: '默认事件包',
      enabled: true,
      events: [],
    }],
    stat_system: { attributes: [] },
    trpg_system: { rule_templates: [] },
    opening_selector: { enabled: true, trait_pools: [], initial_state_ops: [] },
    tags: [],
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
    tags: [],
    custom: id !== 'default',
  }
}

function ruleSystem(id: string, name: string): RuleSystemModule {
  return {
    version: 1,
    id,
    name,
    description: `${name} description`,
    stat_system: { attributes: [] },
    trpg_system: { rule_templates: [] },
    tags: [],
    custom: id !== 'default-rules',
  }
}

function openingSelector(id: string, name: string): OpeningSelectorModule {
  return {
    version: 1,
    id,
    name,
    description: `${name} description`,
    opening_selector: { enabled: true, trait_pools: [], initial_state_ops: [] },
    tags: [],
    custom: id !== 'default-opening',
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
