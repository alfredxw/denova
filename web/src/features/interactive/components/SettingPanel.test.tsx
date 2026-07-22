import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast, type Action } from 'sonner'
import { APIError, deleteLoreItem, generateLoreItemImage, getLoreItems, readFile, saveFile, streamLoreImagesGenerate, updateLoreItem, type LoreItem } from '@/lib/api'
import { preserveAutosaveConflict } from '@/lib/api-client/autosave-conflicts'
import { createActorState, createImagePreset, createInteractiveTeller, createStoryDirector, deleteActorState, deleteEventPackage, deleteImagePreset, deleteInteractiveTeller, deleteStoryDirector, getActorStates, getEventPackages, getImagePresets, getInteractiveTellers, getRuleSystems, getStoryDirectors, getStyleReferences, updateActorState, updateEventPackage, updateImagePreset, updateInteractiveTeller, updateRuleSystem, updateStoryDirector } from '../api'
import type { EventPackageModule, ImagePreset, RuleSystemModule, StoryDirector, Teller } from '../types'
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

vi.mock('sonner', () => ({
  toast: {
    dismiss: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
    success: vi.fn(),
    warning: vi.fn(),
  },
}))

vi.mock('@/lib/api-client/autosave-conflicts', () => ({
  preserveAutosaveConflict: vi.fn(),
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

vi.mock('@/components/Editor/MarkdownRichEditor', () => ({
  MarkdownRichEditor: (props: {
    value: string
    onChange: (value: string) => void
    highlightQuery?: string
    className?: string
    'aria-label'?: string
  }) => (
    <div data-testid="lore-rich-editor-root" className={props.className}>
      <textarea
        data-testid="lore-rich-editor"
        aria-label={props['aria-label']}
        data-highlight-query={props.highlightQuery ?? ''}
        value={props.value}
        onChange={(event) => props.onChange(event.target.value)}
      />
    </div>
  ),
}))

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    abortLoreImagesGenerate: vi.fn(),
    clearLoreItemImage: vi.fn(),
    createLoreItem: vi.fn(),
    deleteLoreItem: vi.fn(),
    generateLoreItemImage: vi.fn(),
    getLoreItems: vi.fn().mockResolvedValue([]),
    readFile: vi.fn().mockResolvedValue({ workspace: '/workspace', path: '', content: '' }),
    saveFile: vi.fn(),
    streamLoreImagesGenerate: vi.fn(),
    updateLoreItem: vi.fn(),
    workspaceAssetURL: (path: string) => `/api/workspace/asset?path=${encodeURIComponent(path)}`,
  }
})

vi.mock('../api', () => ({
  createActorState: vi.fn(),
  createEventPackage: vi.fn(),
  createImagePreset: vi.fn(),
  createInteractiveTeller: vi.fn(),
  createRuleSystem: vi.fn(),
  createStoryDirector: vi.fn(),
  deleteActorState: vi.fn(),
  deleteEventPackage: vi.fn(),
  deleteImagePreset: vi.fn(),
  deleteInteractiveTeller: vi.fn(),
  deleteRuleSystem: vi.fn(),
  deleteStoryDirector: vi.fn(),
  getActorStates: vi.fn(),
  getEventPackages: vi.fn(),
  getImagePresets: vi.fn(),
  getInteractiveTellers: vi.fn(),
  getRuleSystems: vi.fn(),
  getStoryDirectors: vi.fn(),
  getStyleReferences: vi.fn(),
  saveStyleReference: vi.fn(),
  updateEventPackage: vi.fn(),
  updateImagePreset: vi.fn(),
  updateInteractiveTeller: vi.fn(),
  updateActorState: vi.fn(),
  updateRuleSystem: vi.fn(),
  updateStoryDirector: vi.fn(),
}))

describe('SettingPanel', () => {
  beforeEach(() => {
    window.localStorage.clear()
    configManagerChatProps.length = 0
    monacoEditorActions.length = 0
    vi.mocked(getLoreItems).mockReset()
    vi.mocked(preserveAutosaveConflict).mockReset()
    vi.mocked(preserveAutosaveConflict).mockResolvedValue({
      id: 'conflict-record',
      path: 'conflicts/conflict-record.json',
      storage: 'server',
    })
    vi.mocked(updateLoreItem).mockReset()
    vi.mocked(deleteLoreItem).mockReset()
    vi.mocked(generateLoreItemImage).mockReset()
    vi.mocked(streamLoreImagesGenerate).mockReset()
    vi.mocked(readFile).mockReset()
    vi.mocked(readFile).mockResolvedValue({ workspace: '/workspace', path: '', content: '' })
    vi.mocked(saveFile).mockReset()
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
    vi.mocked(deleteEventPackage).mockReset()
    vi.mocked(updateEventPackage).mockReset()
    vi.mocked(getRuleSystems).mockReset()
    vi.mocked(updateRuleSystem).mockReset()
    vi.mocked(getStyleReferences).mockReset()
    vi.mocked(updateImagePreset).mockReset()
    vi.mocked(deleteImagePreset).mockReset()
    vi.mocked(toast.dismiss).mockReset()
    vi.mocked(toast.error).mockReset()
    vi.mocked(toast.success).mockReset()
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
    vi.mocked(deleteEventPackage).mockResolvedValue(undefined)
    vi.mocked(updateEventPackage).mockImplementation(async (id, input) => ({ ...eventPackage(id, input.name || id), ...input, id, custom: id !== 'default', builtin_overridden: id === 'default', updated_at: '2026-01-01T00:00:01Z' }) as EventPackageModule)
    vi.mocked(getRuleSystems).mockResolvedValue([
      ruleSystem('default', '均衡 DM 检定'),
      ruleSystem('dm-fail-forward', '推进型 DM：失败也前进'),
      ruleSystem('dm-osr-player-skill', 'OSR 型 DM：玩家技巧优先'),
    ])
    vi.mocked(updateRuleSystem).mockImplementation(async (id, input) => ({ ...ruleSystem(id, input.name || id), ...input, id, custom: !isBuiltinRuleSystemID(id), builtin_overridden: isBuiltinRuleSystemID(id), updated_at: '2026-01-01T00:00:01Z' }) as RuleSystemModule)
    vi.mocked(getStyleReferences).mockResolvedValue([])
  })

  it('opens the presets config Agent by default', () => {
    render(<PresetPanelHarness />)

    expect(screen.getByTestId('config-manager-chat')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '配置管理 Agent' })).toHaveAttribute('aria-current', 'true')
    expect(screen.queryByRole('heading', { name: '经典叙事' })).not.toBeInTheDocument()
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

  it('does not create a missing CREATOR.md until the user edits it', async () => {
    vi.mocked(readFile).mockRejectedValueOnce(new APIError('not found', { status: 404 }))
    vi.mocked(saveFile).mockResolvedValue({ path: 'CREATOR.md', message: 'ok', revision: 'creator-rev-1' })

    render(<SettingPanel mode="creator" workspace="/workspace" />)

    await waitFor(() => expect(readFile).toHaveBeenCalledWith('CREATOR.md'))
    flushSettingPanelAutosave()

    expect(saveFile).not.toHaveBeenCalled()
    fireEvent.change(screen.getByPlaceholderText('写下本书最高优先级的创作规则...'), { target: { value: '只在编辑后保存' } })
    flushSettingPanelAutosave()

    await waitFor(() => expect(saveFile).toHaveBeenCalledWith('CREATOR.md', '只在编辑后保存', 'missing', '/workspace'))
  })

  it('loads an external CREATOR.md update without writing when the editor is clean', async () => {
    vi.mocked(readFile)
      .mockResolvedValueOnce({ workspace: '/workspace', path: 'CREATOR.md', content: 'Initial', revision: 'r1' })
      .mockResolvedValueOnce({ workspace: '/workspace', path: 'CREATOR.md', content: 'Updated externally', revision: 'r2' })

    render(<SettingPanel mode="creator" workspace="/workspace" />)

    const editor = await screen.findByDisplayValue('Initial')
    act(() => {
      window.dispatchEvent(new CustomEvent('nova:workspace-change', {
        detail: { workspace: '/workspace', paths: ['CREATOR.md'] },
      }))
    })

    await waitFor(() => expect(editor).toHaveValue('Updated externally'))
    expect(saveFile).not.toHaveBeenCalled()
  })

  it('shows the rebased CREATOR.md after transparently retrying a conflict', async () => {
    vi.mocked(readFile)
      .mockResolvedValueOnce({
        workspace: '/workspace',
        path: 'CREATOR.md',
        content: 'Title\n\nDetail\n',
        revision: 'r1',
      })
      .mockResolvedValueOnce({
        workspace: '/workspace',
        path: 'CREATOR.md',
        content: 'Title\n\nExternal detail\n',
        revision: 'r2',
      })
    vi.mocked(saveFile)
      .mockRejectedValueOnce(new APIError('revision conflict', { status: 409 }))
      .mockResolvedValueOnce({ path: 'CREATOR.md', message: 'ok', revision: 'r3' })

    render(<SettingPanel mode="creator" workspace="/workspace" />)

    const editor = await screen.findByPlaceholderText('写下本书最高优先级的创作规则...')
    await waitFor(() => expect(editor).toHaveValue('Title\n\nDetail\n'))
    fireEvent.change(editor, { target: { value: 'Local title\n\nDetail\n' } })
    flushSettingPanelAutosave()

    await waitFor(() => expect(saveFile).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(editor).toHaveValue('Local title\n\nExternal detail\n'))
    expect(screen.getByRole('status')).toHaveAccessibleName('所有更改均已保存')
  })

  it('keeps the config Agent above search without repeating the lore directory heading', async () => {
    render(<SettingPanel mode="lore" workspace="/workspace" imagePresets={[]} />)

    const configAgent = await screen.findByRole('button', { name: '配置管理 Agent' })
    const search = screen.getByRole('textbox', { name: '搜索资料' })

    expect(screen.queryByText('在目录中选择条目，右侧打开编辑。')).not.toBeInTheDocument()
    expect(configAgent.compareDocumentPosition(search) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('keeps the workspace close action in the active preset toolbar', async () => {
    const user = userEvent.setup()
    const onClose = vi.fn()

    render(<SettingPanel mode="teller" workspace="/workspace" onClose={onClose} />)

    await user.click(screen.getByRole('button', { name: '关闭' }))
    expect(onClose).toHaveBeenCalledOnce()
  })

  it('loads a legacy opening preset without writing and saves it after an edit with the missing revision', async () => {
    const user = userEvent.setup()
    vi.mocked(readFile)
      .mockRejectedValueOnce(new APIError('not found', { status: 404 }))
      .mockResolvedValueOnce({
        workspace: '/workspace',
        path: 'setting/interactive-opening.md',
        content: '旧版开场白',
        revision: 'legacy-revision',
      })
    vi.mocked(saveFile).mockResolvedValue({ path: 'setting/interactive-openings.json', message: 'ok', revision: 'opening-rev-1' })

    render(<SettingPanel mode="lore" workspace="/workspace" imagePresets={[]} />)

    await user.click(await screen.findByRole('button', { name: '书籍预设开场白' }))
    await waitFor(() => expect(readFile).toHaveBeenCalledTimes(2))
    flushSettingPanelAutosave()
    expect(saveFile).not.toHaveBeenCalled()

    const editor = await screen.findByPlaceholderText('写下本书预设开场白，例如主角初始处境、场景钩子、关键目标和第一轮可行动空间...')
    expect(editor).toHaveValue('旧版开场白')
    fireEvent.change(editor, { target: { value: '编辑后的旧版开场白' } })
    flushSettingPanelAutosave()

    await waitFor(() => expect(saveFile).toHaveBeenCalledWith(
      'setting/interactive-openings.json',
      expect.any(String),
      'missing',
      '/workspace',
    ))
  })

  it('overrides a built-in narrative style in place instead of copying it', async () => {
    const user = userEvent.setup()
    render(<PresetPanelHarness />)

    await user.click(screen.getByRole('button', { name: /经典叙事/ }))
    fireEvent.change(screen.getByDisplayValue('经典叙事'), { target: { value: '覆盖后的经典叙事' } })
    flushSettingPanelAutosave()

    await waitFor(() => expect(updateInteractiveTeller).toHaveBeenCalled())
    expect(updateInteractiveTeller).toHaveBeenCalledWith(
      'classic',
      expect.objectContaining({
        id: 'classic',
        name: '覆盖后的经典叙事',
        custom: false,
      }),
      undefined,
      '/workspace',
    )
    expect(createInteractiveTeller).not.toHaveBeenCalled()
  })

  it('flushes a pending preset autosave before switching resource types', async () => {
    const user = userEvent.setup()
    render(<PresetPanelHarness />)

    await user.click(screen.getByRole('button', { name: /经典叙事/ }))
    fireEvent.change(screen.getByDisplayValue('经典叙事'), { target: { value: '切换前自动保存' } })
    expandSection('图像方案')
    await user.click(screen.getByRole('button', { name: /游戏 CG/ }))

    await waitFor(() => expect(updateInteractiveTeller).toHaveBeenCalled())
    expect(updateInteractiveTeller).toHaveBeenCalledWith(
      'classic',
      expect.objectContaining({
        id: 'classic',
        name: '切换前自动保存',
      }),
      undefined,
      '/workspace',
    )
    expect(screen.getByRole('heading', { name: '游戏 CG' })).toBeInTheDocument()
  })

  it('retries a failed preset autosave before allowing a resource switch', async () => {
    const user = userEvent.setup()
    vi.mocked(updateInteractiveTeller)
      .mockRejectedValueOnce(new Error('temporary save failure'))
      .mockImplementation(async (id, input) => ({
        ...teller(id, input.name || id),
        ...input,
        id,
        updated_at: '2026-01-01T00:00:02Z',
      }) as Teller)
    render(<PresetPanelHarness />)

    await user.click(screen.getByRole('button', { name: /经典叙事/ }))
    fireEvent.change(screen.getByDisplayValue('经典叙事'), { target: { value: '失败后重试' } })
    flushSettingPanelAutosave()
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('自动保存失败'))

    expandSection('图像方案')
    await user.click(screen.getByRole('button', { name: /游戏 CG/ }))

    await waitFor(() => expect(updateInteractiveTeller).toHaveBeenCalledTimes(2))
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

      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: /自定义 A/ }))
        await Promise.resolve()
      })
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

  it('loads an external preset update without writing when the draft is clean', async () => {
    const initial = { ...teller('custom-a', '自定义 A'), updated_at: 'a-r1' }
    const external = { ...initial, name: '外部更新 A', updated_at: 'a-r2' }
    const onTellersChange = vi.fn()
    const view = render(
      <SettingPanel
        mode="teller"
        workspace="/workspace"
        tellers={[initial]}
        storyDirectors={[storyDirector('default', '默认导演')]}
        imagePresets={[imagePreset('game-cg', '游戏 CG')]}
        onTellersChange={onTellersChange}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /自定义 A/ }))
    await screen.findByDisplayValue('自定义 A')
    view.rerender(
      <SettingPanel
        mode="teller"
        workspace="/workspace"
        tellers={[external]}
        storyDirectors={[storyDirector('default', '默认导演')]}
        imagePresets={[imagePreset('game-cg', '游戏 CG')]}
        onTellersChange={onTellersChange}
      />,
    )

    expect(await screen.findByDisplayValue('外部更新 A')).toBeInTheDocument()
    expect(updateInteractiveTeller).not.toHaveBeenCalled()
  })

  it('rebases a dirty preset draft over an external list update', async () => {
    const initial = { ...teller('custom-a', '自定义 A'), updated_at: 'a-r1' }
    const external = { ...initial, description: '外部更新的描述', updated_at: 'a-r2' }
    const onTellersChange = vi.fn()
    const view = render(
      <SettingPanel
        mode="teller"
        workspace="/workspace"
        tellers={[initial]}
        storyDirectors={[storyDirector('default', '默认导演')]}
        imagePresets={[imagePreset('game-cg', '游戏 CG')]}
        onTellersChange={onTellersChange}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /自定义 A/ }))
    const name = await screen.findByDisplayValue('自定义 A')
    fireEvent.change(name, { target: { value: '本地改名 A' } })
    view.rerender(
      <SettingPanel
        mode="teller"
        workspace="/workspace"
        tellers={[external]}
        storyDirectors={[storyDirector('default', '默认导演')]}
        imagePresets={[imagePreset('game-cg', '游戏 CG')]}
        onTellersChange={onTellersChange}
      />,
    )

    await waitFor(() => expect(name).toHaveValue('本地改名 A'))
    expect(screen.getByRole('textbox', { name: '描述' })).toHaveValue('外部更新的描述')
    flushSettingPanelAutosave()

    await waitFor(() => expect(updateInteractiveTeller).toHaveBeenCalledWith(
      'custom-a',
      expect.objectContaining({ name: '本地改名 A', description: '外部更新的描述' }),
      'a-r2',
      '/workspace',
    ))
  })

  it('keeps a newer preset edit made while an overlapping reload is being archived', async () => {
    const initial = { ...teller('custom-a', '自定义 A'), updated_at: 'a-r1' }
    const external = { ...initial, name: '外部改名 A', updated_at: 'a-r2' }
    let finishArchive!: (value: Awaited<ReturnType<typeof preserveAutosaveConflict>>) => void
    const archivePending = new Promise<Awaited<ReturnType<typeof preserveAutosaveConflict>>>((resolve) => {
      finishArchive = resolve
    })
    vi.mocked(preserveAutosaveConflict).mockReturnValueOnce(archivePending)
    const props = {
      mode: 'teller' as const,
      workspace: '/workspace',
      storyDirectors: [storyDirector('default', '默认导演')],
      imagePresets: [imagePreset('game-cg', '游戏 CG')],
      onTellersChange: vi.fn(),
    }
    const view = render(<SettingPanel {...props} tellers={[initial]} />)

    fireEvent.click(screen.getByRole('button', { name: /自定义 A/ }))
    const name = await screen.findByDisplayValue('自定义 A')
    fireEvent.change(name, { target: { value: '归档前的本地改名' } })
    view.rerender(<SettingPanel {...props} tellers={[external]} />)
    await waitFor(() => expect(preserveAutosaveConflict).toHaveBeenCalledOnce())

    fireEvent.change(name, { target: { value: '归档等待期间的最新改名' } })
    await act(async () => {
      finishArchive({ id: 'conflict-record', path: 'conflicts/conflict-record.json', storage: 'server' })
      await archivePending
      await Promise.resolve()
    })

    expect(name).toHaveValue('归档等待期间的最新改名')
  })

  it('transparently rebases and retries a preset revision conflict', async () => {
    const initial = { ...teller('custom-a', '自定义 A'), updated_at: 'a-r1' }
    const external = { ...initial, description: '服务端新描述', updated_at: 'a-r2' }
    vi.mocked(getInteractiveTellers).mockResolvedValue([external])
    vi.mocked(updateInteractiveTeller)
      .mockRejectedValueOnce(new APIError('revision conflict', { status: 409 }))
      .mockImplementationOnce(async (id, input) => ({
        ...external,
        ...input,
        id,
        updated_at: 'a-r3',
      }) as Teller)
    render(
      <SettingPanel
        mode="teller"
        workspace="/workspace"
        tellers={[initial]}
        storyDirectors={[storyDirector('default', '默认导演')]}
        imagePresets={[imagePreset('game-cg', '游戏 CG')]}
        onTellersChange={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /自定义 A/ }))
    const name = await screen.findByDisplayValue('自定义 A')
    fireEvent.change(name, { target: { value: '本地改名 A' } })
    flushSettingPanelAutosave()

    await waitFor(() => expect(updateInteractiveTeller).toHaveBeenCalledTimes(2))
    expect(updateInteractiveTeller).toHaveBeenNthCalledWith(
      2,
      'custom-a',
      expect.objectContaining({ name: '本地改名 A', description: '服务端新描述' }),
      'a-r2',
      '/workspace',
    )
    expect(screen.getByRole('status')).toHaveAccessibleName('所有更改均已保存')
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

    await user.click(screen.getByRole('button', { name: /覆盖后的经典叙事/ }))
    await waitFor(() => expect(screen.getAllByText('内置覆盖').length).toBeGreaterThan(0))
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

    expandSection('图像方案')
    await user.click(screen.getByRole('button', { name: /游戏 CG/ }))
    fireEvent.change(screen.getByDisplayValue('游戏 CG'), { target: { value: '覆盖后的图像方案' } })
    flushSettingPanelAutosave()

    await waitFor(() => expect(updateImagePreset).toHaveBeenCalled())
    expect(updateImagePreset).toHaveBeenCalledWith(
      'game-cg',
      expect.objectContaining({
        id: 'game-cg',
        name: '覆盖后的图像方案',
        custom: false,
      }),
      undefined,
      '/workspace',
    )
    expect(createImagePreset).not.toHaveBeenCalled()
    await waitFor(() => expect(screen.getAllByText('内置覆盖').length).toBeGreaterThan(0))

    await user.click(screen.getByRole('button', { name: '恢复内置' }))

    await waitFor(() => {
      expect(deleteImagePreset).toHaveBeenCalledWith('game-cg')
    })
    expect(getImagePresets).toHaveBeenCalled()
  })

  it('opens the presets config Agent without leaving the expanded image preset group', async () => {
    const user = userEvent.setup()
    render(<PresetPanelHarness />)

    expandSection('图像方案')
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
    const directory = document.querySelector('.preset-directory')
    expect(directory).not.toBeNull()
    expect(directory).toHaveClass('nova-sidebar', 'overflow-hidden')
    expect(screen.queryByText('在目录中选择条目，右侧打开编辑。')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '配置管理 Agent' })).toBeInTheDocument()
    expect(sectionToggle('叙事风格')).toBeInTheDocument()
    expect(sectionToggle('图像方案')).toBeInTheDocument()
    expect(sectionToggle('故事导演')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /默认导演/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /经典叙事/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '新建故事导演' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '新建叙事风格' })).toBeInTheDocument()
    expect(await screen.findByRole('button', { name: /默认事件包/ })).toBeInTheDocument()
    expect(await screen.findByRole('button', { name: /均衡 DM 检定/ })).toBeInTheDocument()
    const expectedSectionOrder = ['故事导演', '叙事风格', '状态系统', 'TRPG 检定', '图像方案', '事件包']
    for (const [index, name] of expectedSectionOrder.entries()) {
      const nextName = expectedSectionOrder[index + 1]
      if (nextName) {
        expect(sectionHeader(name).compareDocumentPosition(sectionHeader(nextName)) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
      }
    }

    await user.click(screen.getByRole('button', { name: '收起全部' }))
    expect(screen.queryByRole('button', { name: /默认导演/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /经典叙事/ })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '展开全部' }))
    expect(screen.getByRole('button', { name: /默认导演/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /默认事件包/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /均衡 DM 检定/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /推进型 DM：失败也前进/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /OSR 型 DM：玩家技巧优先/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '收起全部' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '收起全部' }))
    expect(screen.queryByRole('button', { name: /默认导演/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /经典叙事/ })).not.toBeInTheDocument()
    expandSection('故事导演')

    await selectDefaultDirector(user)
    expect(screen.queryByRole('tablist', { name: '导演资源' })).not.toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: /事件引用/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: /TRPG 检定/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: /开局选择/ })).not.toBeInTheDocument()
    expect(screen.queryByTestId('preset-config-visual-editor')).not.toBeInTheDocument()

    expandSection('事件包')

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

    expect(sectionToggle('叙事风格')).toBeInTheDocument()
    expect(sectionToggle('图像方案')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /(展开|折叠)故事导演$/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /(展开|折叠)事件包$/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /(展开|折叠)TRPG 检定$/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /(展开|折叠)状态系统$/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: '默认事件包' })).not.toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '经典叙事' })).toBeInTheDocument()
  })

  it('restores the remembered preset selection per usage mode when switching modes', async () => {
    render(<PresetModeRoundTripHarness />)

    // 游戏模式下选中事件包
    expandSection('事件包')
    fireEvent.click(await screen.findByRole('button', { name: /默认事件包/ }))
    expect(await screen.findByRole('heading', { name: '默认事件包' })).toBeInTheDocument()

    // 首次切到写作模式：无记忆，按兜底顺序选中第一个叙事风格
    fireEvent.click(screen.getByRole('button', { name: '写作模式' }))
    expect(await screen.findByRole('heading', { name: '经典叙事' })).toBeInTheDocument()

    // 写作模式下改选图像方案
    expandSection('图像方案')
    fireEvent.click(screen.getByRole('button', { name: /游戏 CG/ }))
    expect(await screen.findByRole('heading', { name: '游戏 CG' })).toBeInTheDocument()

    // 切回游戏模式：恢复游戏模式记忆（默认事件包）
    fireEvent.click(screen.getByRole('button', { name: '游戏模式' }))
    expect(await screen.findByRole('heading', { name: '默认事件包' })).toBeInTheDocument()

    // 再切回写作模式：恢复写作模式记忆（游戏 CG）
    fireEvent.click(screen.getByRole('button', { name: '写作模式' }))
    expect(await screen.findByRole('heading', { name: '游戏 CG' })).toBeInTheDocument()
  })

  it('saves visual edits from an event package card', async () => {
    const user = userEvent.setup()
    render(<PresetModeHarness />)

    expandSection('事件包')
    await user.click(await screen.findByRole('button', { name: /默认事件包/ }))
    await user.click(await screen.findByRole('button', { name: '新增事件卡' }))
    expect(screen.getByTestId('event-package-card-editor')).toHaveClass('grid')
    expect(screen.getByTestId('preset-config-visual-editor')).toHaveClass('preset-config-visual-container')
    expect(screen.getByTestId('event-package-card-editor')).toHaveClass('preset-visual-editor-shell', 'overflow-hidden')
    expect(screen.getByTestId('event-package-card-detail-scroll')).toHaveClass('p-3')
    expect(screen.getByTestId('event-package-card-detail-scroll')).not.toHaveClass('overflow-y-auto')
    await user.type(screen.getByLabelText('事件类型名'), '伏笔回收')
    flushSettingPanelAutosave()

    await waitFor(() => expect(updateEventPackage).toHaveBeenCalled())
    expect(updateEventPackage).toHaveBeenCalledWith('default', expect.objectContaining({ id: 'default', custom: false }), undefined, '/workspace')
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
    flushSettingPanelAutosave()

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
    flushSettingPanelAutosave()

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
    flushSettingPanelAutosave()

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

    const modeField = screen.getByText('新故事默认运行方式').closest('label') as HTMLElement
    await user.click(within(modeField).getByRole('combobox'))
    await user.click(screen.getByRole('option', { name: /每回合/ }))

    const visibilityField = screen.getByText('规则可见性').closest('label') as HTMLElement
    await user.click(within(visibilityField).getByRole('combobox'))
    await user.click(screen.getByRole('option', { name: /公开掷骰/ }))

    flushSettingPanelAutosave()
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
      '## 阶段目标与隐藏钩子',
      '钩子',
      '## 资料库锚点',
      '资料库角色与势力',
      '## 选角覆盖',
      '标准场景',
      '## 核心角色与关系张力',
      '核心角色',
      '## 重要势力与阶段阻力',
      '势力阻力',
      '## 当前场景幕后信息',
      '行动空间',
      '## 信息揭示与线索密度',
      '线索密度',
      '## 遭遇、检定与代价',
      '检定代价',
      '## 爽点、危机与反转',
      '爽点反转',
      '## 状态连续性',
      '状态',
      '## 最近分支安排',
      '最近分支',
      '## 伏笔与回收',
      '伏笔',
    ].join('\n')
    const agentBriefTemplate = [
      '# 自定义正文 Agent 简报',
      '## 当前目标与可见钩子',
      '目标',
      '## 当前场景与行动空间',
      '场景',
      '## 当前角色与可见关系',
      '角色',
      '## 已公开信息与可发现线索',
      '线索',
      '## 遭遇、检定与可见代价',
      '代价',
      '## 状态连续性',
      '状态',
      '## 最近分支承接',
      '承接',
    ].join('\n')
    const planTemplateField = screen.getByRole('textbox', { name: /director\.md 模板/ })
    expect(planTemplateField).toHaveClass('min-h-[calc(20*1.25rem+1rem)]')
    fireEvent.change(planTemplateField, { target: { value: template } })
    fireEvent.change(screen.getByRole('textbox', { name: /agent-brief\.md 模板/ }), { target: { value: agentBriefTemplate } })

    flushSettingPanelAutosave()
    await waitFor(() => expect(updateStoryDirector).toHaveBeenCalled())
    const payload = vi.mocked(updateStoryDirector).mock.calls.at(-1)?.[1] as Partial<StoryDirector>
    expect(payload.strategy?.planning_templates?.plan).toBe(template)
    expect(payload.strategy?.planning_templates?.agent_brief).toBe(agentBriefTemplate)
  })

  it('saves advanced Markdown strategy prompt for story directors', async () => {
    const user = userEvent.setup()
    render(<PresetModeHarness />)

    await selectDefaultDirector(user)
    await user.click(screen.getByRole('button', { name: /高级 Markdown 策略提示/ }))
    const prompt = '- 避免连续两回合使用同类型突发事件。\n- 伏笔回收前至少给一次可感知征兆。'
    fireEvent.change(screen.getByPlaceholderText(/优先制造可逆但有代价的选择/), { target: { value: prompt } })
    expect(screen.getAllByText('已启用自定义策略提示').length).toBeGreaterThan(0)

    flushSettingPanelAutosave()

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

    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('修复配置后将自动保存'))
    expect(screen.getByText('策略提示已超过 65536 bytes（当前 65537 bytes）；缩短后将自动保存。')).toBeInTheDocument()
    expect(updateStoryDirector).not.toHaveBeenCalled()
  })

  it('blocks saving and preset navigation while JSON view is invalid', async () => {
    const user = userEvent.setup()
    render(<PresetModeHarness />)

    expandSection('事件包')
    await user.click(await screen.findByRole('button', { name: /默认事件包/ }))
    await user.click(screen.getByRole('button', { name: 'JSON' }))
    fireEvent.change(screen.getByTestId('monaco-json-editor'), { target: { value: '{' } })

    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('修复配置后将自动保存'))
    expect(screen.getByText('请先修复 JSON，再切回可视化视图。')).toBeInTheDocument()

    expandSection('TRPG 检定')
    await user.click(await screen.findByRole('button', { name: /均衡 DM 检定/ }))
    expect(screen.getByRole('heading', { name: '默认事件包' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: '均衡 DM 检定' })).not.toBeInTheDocument()
    expect(updateEventPackage).not.toHaveBeenCalled()
    expect(toast.error).toHaveBeenCalledWith(
      '当前配置包含无效 JSON',
      expect.objectContaining({
        description: '请先修复 JSON；修复后将自动保存，并可继续切换配置。',
        action: undefined,
      }),
    )
  })

  it('offers a manual built-in restore for invalid JSON from an old override', async () => {
    const user = userEvent.setup()
    const overridden = {
      ...eventPackage('default', '默认事件包'),
      builtin_overridden: true,
    }
    vi.mocked(getEventPackages)
      .mockResolvedValueOnce([overridden])
      .mockResolvedValue([eventPackage('default', '默认事件包')])
    render(<PresetModeHarness />)

    expandSection('事件包')
    await user.click(await screen.findByRole('button', { name: /默认事件包/ }))
    await user.click(screen.getByRole('button', { name: 'JSON' }))
    fireEvent.change(screen.getByTestId('monaco-json-editor'), { target: { value: '{' } })

    expandSection('TRPG 检定')
    await user.click(await screen.findByRole('button', { name: /均衡 DM 检定/ }))

    expect(screen.getByRole('heading', { name: '默认事件包' })).toBeInTheDocument()
    expect(deleteEventPackage).not.toHaveBeenCalled()
    expect(toast.error).toHaveBeenCalledWith(
      '当前配置包含无效 JSON',
      expect.objectContaining({
        description: '当前配置可能是旧版本留下的内置覆盖数据。你可以修复 JSON，或手动恢复内置版本；系统不会自动修改现有数据。',
        action: expect.objectContaining({ label: '恢复内置' }),
      }),
    )

    const toastOptions = vi.mocked(toast.error).mock.calls.at(-1)?.[1]
    const restoreAction = toastOptions?.action as Action
    await act(async () => {
      restoreAction.onClick({} as never)
      await Promise.resolve()
    })

    await waitFor(() => expect(deleteEventPackage).toHaveBeenCalledWith('default'))
  })

  it('edits TRPG checks through the focused DM-style visual workflow', async () => {
    const user = userEvent.setup()
    render(<PresetModeHarness />)

    expandSection('TRPG 检定')
    await user.click(await screen.findByRole('button', { name: /均衡 DM 检定/ }))

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

    flushSettingPanelAutosave()
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
    const emptyImage = screen.getByText('暂无图片')
    const emptyImageRow = emptyImage.parentElement
    expect(emptyImageRow).not.toBeNull()
    expect(screen.getByText('当前图片').parentElement).toBe(emptyImageRow)
    expect(screen.getByRole('button', { name: '打开图片生成' }).parentElement).toBe(emptyImageRow)
    const metadataGroup = screen.getByRole('group', { name: '资料元数据' })
    expect(metadataGroup).toContainElement(emptyImage)
    expect(screen.getByRole('region', { name: '资料编辑区' })).toContainElement(metadataGroup)

    const secondaryFields = within(metadataGroup).getByRole('textbox', { name: '标签' }).closest('[data-slot="lore-secondary-fields"]')
    expect(secondaryFields).not.toBeNull()
    expect(secondaryFields).toContainElement(within(metadataGroup).getByRole('textbox', { name: '简介' }))
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
    const primaryFieldsWithImage = within(metadataGroup).getByRole('textbox', { name: '名称' }).closest('[data-slot="lore-primary-fields"]')
    expect(primaryFieldsWithImage).toHaveClass('2xl:grid-cols-[minmax(12rem,2fr)_repeat(4,minmax(7rem,1fr))]')
    expect(primaryFieldsWithImage).not.toHaveClass('xl:grid-cols-[minmax(12rem,2fr)_repeat(4,minmax(7rem,1fr))]')
    expect(screen.queryByText('已有图片')).not.toBeInTheDocument()
    expect(screen.queryByText('assets/lore/images/lin-chuan/20260101000000/image.png')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '放大查看资料图片' }))

    const previewDialog = screen.getByRole('dialog', { name: '林川' })
    expect(within(previewDialog).getByTestId('image-preview-viewport')).toBeInTheDocument()
  })

  it('keeps the lore body editable in the unified scroller and feeds the directory query to the editor', async () => {
    const item = {
      ...loreItem('long-lore', '长正文资料'),
      content: '正文段落\n'.repeat(80),
    }
    vi.mocked(getLoreItems).mockResolvedValue([item])

    render(<SettingPanel mode="lore" workspace="/workspace" imagePresets={[imagePreset('game-cg', '游戏 CG')]} />)

    fireEvent.click(await screen.findByRole('button', { name: /长正文资料/ }))
    const editor = screen.getByRole('region', { name: '资料编辑区' })
    const content = within(editor).getByRole('textbox', { name: '正文' }) as HTMLTextAreaElement

    // 渲染视图本身即可编辑：不再有 预览/Raw 切换，也不引入嵌套滚动容器
    expect(content).toHaveAttribute('data-testid', 'lore-rich-editor')
    expect(within(editor).queryByText('正文')).not.toBeInTheDocument()
    expect(within(editor).queryByRole('button', { name: '预览' })).not.toBeInTheDocument()
    expect(within(editor).queryByRole('button', { name: 'Raw' })).not.toBeInTheDocument()
    expect(editor.querySelector('.overflow-y-auto')).toBeNull()
    const editorRoot = within(editor).getByTestId('lore-rich-editor-root')
    expect(editorRoot).toHaveClass('flex', 'min-h-[420px]', 'flex-1')
    expect(editorRoot.parentElement).toHaveClass('flex', 'min-h-full', 'min-w-0', 'flex-col')
    expect(within(editor).getByRole('textbox', { name: '名称' }).closest('[data-slot="lore-primary-fields"]')).toHaveClass(
      'grid-cols-2',
      'md:grid-cols-3',
      'xl:grid-cols-[minmax(12rem,2fr)_repeat(4,minmax(7rem,1fr))]',
    )

    fireEvent.change(content, { target: { value: `${item.content}新增段落` } })
    expect(content.value).toBe(`${item.content}新增段落`)

    // 目录搜索词直接传给正文编辑器做高亮，搜索时依然可以编辑
    fireEvent.change(screen.getByRole('textbox', { name: '搜索资料' }), { target: { value: '正文段落' } })
    expect(content).toHaveAttribute('data-highlight-query', '正文段落')
    expect(content).not.toBeDisabled()
  })

  it('loads an external Lore update without writing when the editor is clean', async () => {
    const initial = loreItem('lin-chuan', '林川')
    const external = {
      ...initial,
      name: '外部更新后的林川',
      content: '## 外部正文',
      updated_at: '2026-01-01T00:00:01Z',
    }
    vi.mocked(getLoreItems)
      .mockResolvedValueOnce([initial])
      .mockResolvedValueOnce([external])

    render(<SettingPanel mode="lore" workspace="/workspace" imagePresets={[]} />)

    const name = await screen.findByRole('textbox', { name: '名称' })
    expect(name).toHaveValue('林川')
    act(() => window.dispatchEvent(new CustomEvent('nova:lore-updated', { detail: { item_ids: ['lin-chuan'] } })))

    await waitFor(() => expect(name).toHaveValue('外部更新后的林川'))
    expect(screen.getByRole('textbox', { name: '正文' })).toHaveValue('## 外部正文\n')
    expect(updateLoreItem).not.toHaveBeenCalled()
  })

  it('rebases a dirty Lore draft over an external update before saving', async () => {
    const initial = loreItem('lin-chuan', '林川')
    const external = {
      ...initial,
      content: '## 外部正文',
      updated_at: '2026-01-01T00:00:01Z',
    }
    vi.mocked(getLoreItems)
      .mockResolvedValueOnce([initial])
      .mockResolvedValueOnce([external])
    vi.mocked(updateLoreItem).mockImplementation(async (id, input) => ({
      ...external,
      ...input,
      id,
      updated_at: '2026-01-01T00:00:02Z',
    }) as LoreItem)

    render(<SettingPanel mode="lore" workspace="/workspace" imagePresets={[]} />)

    const name = await screen.findByRole('textbox', { name: '名称' })
    fireEvent.change(name, { target: { value: '本地改名' } })
    act(() => window.dispatchEvent(new CustomEvent('nova:lore-updated', { detail: { item_ids: ['lin-chuan'] } })))

    await waitFor(() => expect(screen.getByRole('textbox', { name: '正文' })).toHaveValue('## 外部正文\n'))
    expect(name).toHaveValue('本地改名')
    flushSettingPanelAutosave()

    await waitFor(() => expect(updateLoreItem).toHaveBeenCalledWith(
      'lin-chuan',
      expect.objectContaining({ name: '本地改名', content: '## 外部正文' }),
      '2026-01-01T00:00:01Z',
    ))
  })

  it('keeps a newer Lore edit made while an overlapping reload is being archived', async () => {
    const initial = loreItem('lin-chuan', '林川')
    const external = {
      ...initial,
      name: '外部改名',
      updated_at: '2026-01-01T00:00:01Z',
    }
    let finishArchive!: (value: Awaited<ReturnType<typeof preserveAutosaveConflict>>) => void
    const archivePending = new Promise<Awaited<ReturnType<typeof preserveAutosaveConflict>>>((resolve) => {
      finishArchive = resolve
    })
    vi.mocked(preserveAutosaveConflict).mockReturnValueOnce(archivePending)
    vi.mocked(getLoreItems)
      .mockResolvedValueOnce([initial])
      .mockResolvedValueOnce([external])

    render(<SettingPanel mode="lore" workspace="/workspace" imagePresets={[]} />)

    const name = await screen.findByRole('textbox', { name: '名称' })
    fireEvent.change(name, { target: { value: '归档前的本地改名' } })
    act(() => window.dispatchEvent(new CustomEvent('nova:lore-updated', { detail: { item_ids: ['lin-chuan'] } })))
    await waitFor(() => expect(preserveAutosaveConflict).toHaveBeenCalledOnce())

    fireEvent.change(name, { target: { value: '归档等待期间的最新改名' } })
    await act(async () => {
      finishArchive({ id: 'conflict-record', path: 'conflicts/conflict-record.json', storage: 'server' })
      await archivePending
      await Promise.resolve()
    })

    expect(name).toHaveValue('归档等待期间的最新改名')
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

    flushSettingPanelAutosave()

    await waitFor(() => expect(updateLoreItem).toHaveBeenCalled())
    expect(updateLoreItem).toHaveBeenCalledWith(
      'lin-chuan',
      expect.objectContaining({ enabled: false }),
      '2026-01-01T00:00:00Z',
    )
  })

  it('warns without blocking when resident lore exceeds 32 KB', async () => {
    const user = userEvent.setup()
    const item = { ...loreItem('resident-rules', '常驻规则', 'rule'), load_mode: 'resident' as const, content: 'x'.repeat(33 * 1024) }
    vi.mocked(getLoreItems).mockResolvedValue([item])

    render(<SettingPanel mode="lore" workspace="/workspace" imagePresets={[imagePreset('game-cg', '游戏 CG')]} />)

    await user.click(await screen.findByRole('button', { name: /常驻规则/ }))
    expect(screen.getByText('当前常驻资料约 33 KB，超过 32 KB 建议值；不会阻止保存或使用。')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '保存' })).not.toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent('所有更改均已保存')
  })

  it('filters lore by load strategy and labels each directory item', async () => {
    const user = userEvent.setup()
    const resident = { ...loreItem('resident', '常驻人物'), load_mode: 'resident' as const }
    const automatic = loreItem('automatic', '自动人物')
    const manual = { ...loreItem('manual', '手动人物'), load_mode: 'manual' as const }
    vi.mocked(getLoreItems).mockResolvedValue([resident, automatic, manual])

    render(<SettingPanel mode="lore" workspace="/workspace" imagePresets={[imagePreset('game-cg', '游戏 CG')]} />)

    const residentButton = await screen.findByRole('button', { name: /常驻人物/ })
    expect(within(residentButton).getByText('常驻')).toBeInTheDocument()
    expect(within(screen.getByRole('button', { name: /自动人物/ })).getByText('按需')).toHaveAttribute('title', '简介自动匹配')
    expect(within(screen.getByRole('button', { name: /手动人物/ })).getByText('按需')).toHaveAttribute('title', '手动引用')

    const loadModeFilter = screen.getByRole('combobox', { name: /按加载策略筛选/ })
    const searchGroup = loadModeFilter.closest('[data-slot="input-group"]')
    expect(searchGroup).not.toBeNull()
    expect(within(searchGroup as HTMLElement).getByPlaceholderText('搜索资料')).toBeInTheDocument()

    await user.click(loadModeFilter)
    await user.click(screen.getByRole('option', { name: '常驻' }))

    expect(screen.getByRole('button', { name: /常驻人物/ })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /自动人物/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /手动人物/ })).not.toBeInTheDocument()

    await user.click(screen.getByRole('combobox', { name: /按加载策略筛选/ }))
    await user.click(screen.getByRole('option', { name: '按需' }))

    expect(screen.queryByRole('button', { name: /常驻人物/ })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /自动人物/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /手动人物/ })).toBeInTheDocument()
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

    await user.click(screen.getByRole('button', { name: /黑色幽默/ }))
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

function flushSettingPanelAutosave() {
  const headings = screen.getAllByRole('heading', { level: 2 })
  const heading = headings[headings.length - 1]
  if (!heading) throw new Error('SettingPanel feature heading not found')
  fireEvent.keyDown(heading, { key: 's', ctrlKey: true })
}

function sectionHeader(name: string) {
  return sectionToggle(name)
}

function sectionToggle(name: string) {
  return screen.getByRole('button', { name: new RegExp(`^(展开|折叠)${name}$`) })
}

function expandSection(name: string) {
  const toggle = sectionToggle(name)
  if (toggle.getAttribute('aria-expanded') === 'false') fireEvent.click(toggle)
}

async function selectDefaultDirector(user: ReturnType<typeof userEvent.setup>) {
  if (!screen.queryByRole('button', { name: /默认导演/ })) {
    await user.click(sectionToggle('故事导演'))
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

function PresetModeRoundTripHarness() {
  const [presetUsageMode, setPresetUsageMode] = useState<'writing' | 'game'>('game')
  return (
    <>
      <button type="button" onClick={() => setPresetUsageMode('writing')}>写作模式</button>
      <button type="button" onClick={() => setPresetUsageMode('game')}>游戏模式</button>
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

function loreItem(id: string, name: string, type: LoreItem['type'] = 'character'): LoreItem {
  return {
    id,
    enabled: true,
    type,
    type_source: 'manual',
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
