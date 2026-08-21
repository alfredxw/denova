import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState, type ComponentProps } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast, type Action } from 'sonner'
import { APIError, deleteProjectLoreItem, generateLoreItemImage, getProjectLoreItems, readOptionalProjectFile, readProjectFile, saveProjectFile, updateProjectLoreItem, uploadLoreItemImage, type LoreItem } from '@/lib/api'
import { preserveAutosaveConflict } from '@/lib/api-client/autosave-conflicts'
import { createActorState, createImagePreset, createInteractiveTeller, createStoryDirector, deleteActorState, deleteEventPackage, deleteImagePreset, deleteInteractiveTeller, deleteStoryDirector, getActorStates, getEventPackages, getImagePresets, getInteractiveTellers, getRuleSystems, getStoryDirectors, getStyleReferences, updateActorState, updateEventPackage, updateImagePreset, updateInteractiveTeller, updateRuleSystem, updateStoryDirector } from '../api'
import { serializeBookOpeningPresets } from '../opening'
import type { EventPackageModule, ImagePreset, RuleSystemModule, StoryDirector, Teller } from '../types'
import { defaultRuleTemplates } from './preset-config/ruleTemplates'
import { SettingPanel as ProjectSettingPanel } from './SettingPanel'
import type { MarkdownRichEditorReview } from '@/components/Editor/MarkdownRichEditor'
import type { DocumentReviewController } from '@/features/document-review/controller'
import type { CreateDocumentCommentRequest, DocumentReviewComment } from '@/features/document-review/types'

const TEST_PROJECT_ID = 'project-workspace'

function projectFileDocument(path: string, content: string, revision: string) {
  return {
    project_id: 'project-workspace',
    path,
    content,
    revision,
    kind: 'text' as const,
    mime_type: 'text/markdown',
    size: content.length,
    editable: true,
  }
}

function projectFileSave(path: string, revision: string) {
  return { project_id: 'project-workspace', path, revision, changed: true, message: 'ok' }
}

function SettingPanel(props: Omit<ComponentProps<typeof ProjectSettingPanel>, 'projectId'> & { projectId?: string }) {
  return <ProjectSettingPanel {...props} projectId={props.projectId || TEST_PROJECT_ID} />
}

const { configManagerChatProps, markdownRichEditorProps, monacoEditorActions } = vi.hoisted(() => ({
  configManagerChatProps: [] as Array<{
    origin?: string
    resourceId?: string
    initialInstruction?: string
    initialInstructionKey?: string
    onMutated?: () => void
  }>,
  markdownRichEditorProps: [] as Array<{
    projectId: string
    review?: MarkdownRichEditorReview
  }>,
  monacoEditorActions: [] as string[],
}))

vi.mock('@monaco-editor/react', () => ({
  DiffEditor: () => null,
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
    initialInstruction?: string
    initialInstructionKey?: string
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
    projectId: string
    value: string
    onChange: (value: string) => void
    highlightQuery?: string
    review?: MarkdownRichEditorReview
    className?: string
    'aria-label'?: string
  }) => {
    markdownRichEditorProps.push(props)
    return (
      <div data-testid="lore-rich-editor-root" className={props.className}>
        <textarea
          data-testid="lore-rich-editor"
          aria-label={props['aria-label']}
          data-highlight-query={props.highlightQuery ?? ''}
          value={props.value}
          onChange={(event) => props.onChange(event.target.value)}
        />
      </div>
    )
  },
}))

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    clearLoreItemImage: vi.fn(),
    createProjectLoreItem: vi.fn(),
    deleteProjectLoreItem: vi.fn(),
    generateLoreItemImage: vi.fn(),
    getProjectLoreItems: vi.fn().mockResolvedValue([]),
    readOptionalProjectFile: vi.fn().mockResolvedValue(null),
    readProjectFile: vi.fn().mockResolvedValue(projectFileDocument('', '', '')),
    saveProjectFile: vi.fn(),
    updateProjectLoreItem: vi.fn(),
    uploadLoreItemImage: vi.fn(),
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
    markdownRichEditorProps.length = 0
    monacoEditorActions.length = 0
    vi.mocked(getProjectLoreItems).mockReset()
    vi.mocked(preserveAutosaveConflict).mockReset()
    vi.mocked(preserveAutosaveConflict).mockResolvedValue({
      id: 'conflict-record',
      path: 'conflicts/conflict-record.json',
      storage: 'server',
    })
    vi.mocked(updateProjectLoreItem).mockReset()
    vi.mocked(deleteProjectLoreItem).mockReset()
    vi.mocked(generateLoreItemImage).mockReset()
    vi.mocked(uploadLoreItemImage).mockReset()
    vi.mocked(readProjectFile).mockReset()
    vi.mocked(readProjectFile).mockResolvedValue(projectFileDocument('', '', ''))
    vi.mocked(readOptionalProjectFile).mockReset()
    vi.mocked(readOptionalProjectFile).mockResolvedValue(null)
    vi.mocked(saveProjectFile).mockReset()
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
    vi.mocked(getProjectLoreItems).mockResolvedValue([])
    vi.mocked(getInteractiveTellers).mockResolvedValue([teller('classic', '经典叙事'), teller('slow-burn', '慢热叙事')])
    vi.mocked(updateInteractiveTeller).mockImplementation(async (id, input) => ({ ...teller(id, input.name || id), ...input, id, custom: id !== 'classic', builtin_overridden: id === 'classic', revision: 'sha256:saved', updated_at: '2026-01-01T00:00:01Z' }) as Teller)
    vi.mocked(deleteInteractiveTeller).mockResolvedValue(undefined)
    vi.mocked(getStoryDirectors).mockResolvedValue([storyDirector('default', '默认导演')])
    vi.mocked(createStoryDirector).mockResolvedValue(storyDirector('default-custom', '默认导演'))
    vi.mocked(updateStoryDirector).mockImplementation(async (id, input) => ({ ...storyDirector(id, input.name || id), ...input, id, custom: id !== 'default', builtin_overridden: id === 'default', revision: 'sha256:saved', updated_at: '2026-01-01T00:00:01Z' }) as StoryDirector)
    vi.mocked(deleteStoryDirector).mockResolvedValue(undefined)
    vi.mocked(getActorStates).mockResolvedValue([])
    vi.mocked(deleteActorState).mockResolvedValue(undefined)
    vi.mocked(getImagePresets).mockResolvedValue([imagePreset('game-cg', '游戏 CG')])
    vi.mocked(updateImagePreset).mockImplementation(async (id, input) => ({ ...imagePreset(id, input.name || id), ...input, id, custom: id !== 'game-cg', builtin_overridden: id === 'game-cg', revision: 'sha256:saved', updated_at: '2026-01-01T00:00:01Z' }) as ImagePreset)
    vi.mocked(deleteImagePreset).mockResolvedValue(undefined)
    vi.mocked(getEventPackages).mockResolvedValue([eventPackage('default', '默认事件包')])
    vi.mocked(deleteEventPackage).mockResolvedValue(undefined)
    vi.mocked(updateEventPackage).mockImplementation(async (id, input) => ({ ...eventPackage(id, input.name || id), ...input, id, custom: id !== 'default', builtin_overridden: id === 'default', revision: 'sha256:saved', updated_at: '2026-01-01T00:00:01Z' }) as EventPackageModule)
    vi.mocked(getRuleSystems).mockResolvedValue([
      ruleSystem('default', '均衡 DM 检定'),
      ruleSystem('dm-fail-forward', '推进型 DM：失败也前进'),
      ruleSystem('dm-osr-player-skill', 'OSR 型 DM：玩家技巧优先'),
    ])
    vi.mocked(updateRuleSystem).mockImplementation(async (id, input) => ({ ...ruleSystem(id, input.name || id), ...input, id, custom: !isBuiltinRuleSystemID(id), builtin_overridden: isBuiltinRuleSystemID(id), revision: 'sha256:saved', updated_at: '2026-01-01T00:00:01Z' }) as RuleSystemModule)
    vi.mocked(getStyleReferences).mockResolvedValue([])
  })

  it('does not create a missing CREATOR.md until the user edits it', async () => {
    vi.mocked(readProjectFile).mockRejectedValueOnce(new APIError('not found', { status: 404 }))
    vi.mocked(saveProjectFile).mockResolvedValue(projectFileSave('CREATOR.md', 'creator-rev-1'))

    render(<SettingPanel mode="creator" />)

    await waitFor(() => expect(readProjectFile).toHaveBeenCalledWith(TEST_PROJECT_ID, 'CREATOR.md'))
    flushSettingPanelAutosave()

    expect(saveProjectFile).not.toHaveBeenCalled()
    fireEvent.change(screen.getByPlaceholderText('写下本书最高优先级的创作规则...'), { target: { value: '只在编辑后保存' } })
    flushSettingPanelAutosave()

    await waitFor(() => expect(saveProjectFile).toHaveBeenCalledWith(TEST_PROJECT_ID, 'CREATOR.md', '只在编辑后保存', 'missing'))
  })

  it('loads existing opening presets without treating the initial snapshot as a conflict', async () => {
    const user = userEvent.setup()
    const content = serializeBookOpeningPresets([{
      id: 'saved-opening',
      title: '已保存的开场白',
      content: '雨停之后，城门第一次打开。',
    }])
    vi.mocked(readOptionalProjectFile).mockResolvedValueOnce(
      projectFileDocument('setting/interactive-openings.json', content, 'opening-rev-1'),
    )

    render(<SettingPanel mode="lore" imagePresets={[]} />)

    await user.click(await screen.findByRole('button', { name: '书籍预设开场白' }))
    const editor = await screen.findByPlaceholderText('写下本书预设开场白，例如主角初始处境、场景钩子、关键目标和第一轮可行动空间...')
    await waitFor(() => expect(editor).toHaveValue('雨停之后，城门第一次打开。'))
    flushSettingPanelAutosave()

    expect(preserveAutosaveConflict).not.toHaveBeenCalled()
    expect(saveProjectFile).not.toHaveBeenCalled()
  })

  it('loads a legacy opening preset without writing and saves it after an edit with the missing revision', async () => {
    const user = userEvent.setup()
    vi.mocked(readOptionalProjectFile)
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce(projectFileDocument('setting/interactive-opening.md', '旧版开场白', 'legacy-revision'))
    vi.mocked(saveProjectFile).mockResolvedValue(projectFileSave('setting/interactive-openings.json', 'opening-rev-1'))

    render(<SettingPanel mode="lore" imagePresets={[]} />)

    await user.click(await screen.findByRole('button', { name: '书籍预设开场白' }))
    await waitFor(() => expect(readOptionalProjectFile).toHaveBeenCalledTimes(2))
    flushSettingPanelAutosave()
    expect(saveProjectFile).not.toHaveBeenCalled()

    const editor = await screen.findByPlaceholderText('写下本书预设开场白，例如主角初始处境、场景钩子、关键目标和第一轮可行动空间...')
    expect(editor).toHaveValue('旧版开场白')
    fireEvent.change(editor, { target: { value: '编辑后的旧版开场白' } })
    flushSettingPanelAutosave()

    await waitFor(() => expect(saveProjectFile).toHaveBeenCalledWith(
      TEST_PROJECT_ID,
      'setting/interactive-openings.json',
      expect.any(String),
      'missing',
    ))
  })

  it('overrides a built-in narrative style in place instead of copying it', async () => {
    const user = userEvent.setup()
    render(<PresetPanelHarness />)

    await user.click(screen.getByRole('button', { name: /稳健叙事/ }))
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
      'sha256:fixture',
    )
    expect(createInteractiveTeller).not.toHaveBeenCalled()
  })

  it('round-trips a custom preset through autosave with its latest content and revision', async () => {
    vi.useFakeTimers()
    const customA = { ...teller('custom-a', '自定义 A'), revision: 'a-r1', updated_at: 'same-time' }
    const customB = { ...teller('custom-b', '自定义 B'), revision: 'b-r1', updated_at: 'same-time' }
    let resolveFirstSave!: (saved: Teller) => void
    const firstSave = new Promise<Teller>((resolve) => { resolveFirstSave = resolve })
    vi.mocked(updateInteractiveTeller)
      .mockImplementationOnce(async () => firstSave)
      .mockImplementation(async (id, input) => ({
      ...teller(id, input.name || id),
      ...input,
      id,
      custom: true,
      revision: id === 'custom-a' ? 'a-r3' : 'b-r2',
      updated_at: 'same-time',
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
        resolveFirstSave({ ...customA, name: 'A 首次保存', revision: 'a-r2' })
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
      'sha256:fixture',
    )
    expect(createImagePreset).not.toHaveBeenCalled()
    await waitFor(() => expect(screen.getAllByText('内置覆盖').length).toBeGreaterThan(0))

    await user.click(screen.getByRole('button', { name: '恢复内置' }))

    await waitFor(() => {
      expect(deleteImagePreset).toHaveBeenCalledWith('game-cg')
    })
    expect(getImagePresets).toHaveBeenCalled()
  })

  it('shows every preset type with a fixed availability label', async () => {
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
    expect(screen.getAllByText('通用')).toHaveLength(2)
    expect(screen.getAllByText('游戏专用')).toHaveLength(4)
    expect(screen.getByRole('button', { name: /默认导演/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /稳健叙事/ })).toBeInTheDocument()
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
    expect(screen.queryByRole('button', { name: /稳健叙事/ })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '展开全部' }))
    expect(screen.getByRole('button', { name: /默认导演/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /默认事件包/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /均衡 DM 检定/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /推进型 DM：失败也前进/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /OSR 型 DM：玩家技巧优先/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '收起全部' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '收起全部' }))
    expect(screen.queryByRole('button', { name: /默认导演/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /稳健叙事/ })).not.toBeInTheDocument()
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
    expect(updateEventPackage).toHaveBeenCalledWith('default', expect.objectContaining({ id: 'default', custom: false }), 'sha256:fixture')
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

    await waitFor(() => expect(screen.getByRole('status', { name: /修复配置后将自动保存/ })).toHaveTextContent('修复配置后将自动保存'))
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

    await waitFor(() => expect(screen.getByRole('status', { name: /修复配置后将自动保存/ })).toHaveTextContent('修复配置后将自动保存'))
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

  it('saves TRPG check guidance from the focused DM-style visual workflow', async () => {
    const user = userEvent.setup()
    render(<PresetModeHarness />)

    expandSection('TRPG 检定')
    await user.click(await screen.findByRole('button', { name: /均衡 DM 检定/ }))

    fireEvent.change(screen.getByRole('textbox', { name: '规则名称' }), { target: { value: '自定义 DM 检定' } })

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

  it('generates a current image for one lore item from the editor', async () => {
    const user = userEvent.setup()
    const item = loreItem('lin-chuan', '林川')
    const withImage = {
      ...item,
      updated_at: '2026-01-01T00:00:01Z',
      image: loreImage('assets/lore/images/lin-chuan/20260101000000/image.png'),
    }
    vi.mocked(getProjectLoreItems).mockResolvedValue([item])
    vi.mocked(updateProjectLoreItem).mockResolvedValue(item)
    vi.mocked(generateLoreItemImage).mockResolvedValue(withImage)

    render(<SettingPanel mode="lore" imagePresets={[imagePreset('game-cg', '游戏 CG')]} />)

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
      expect(generateLoreItemImage).toHaveBeenCalledWith(TEST_PROJECT_ID, 'lin-chuan', expect.objectContaining({ image_preset_id: 'game-cg' }))
    })
    await user.click(within(generateDialog).getByRole('button', { name: '关闭' }))
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })

    expect(await screen.findByRole('img', { name: '林川' })).toHaveAttribute('src', '/api/projects/project-workspace/files/asset?path=assets%2Flore%2Fimages%2Flin-chuan%2F20260101000000%2Fimage.png')
    const primaryFieldsWithImage = within(metadataGroup).getByRole('textbox', { name: '名称' }).closest('[data-slot="lore-primary-fields"]')
    expect(primaryFieldsWithImage).toHaveClass('2xl:grid-cols-[minmax(12rem,2fr)_repeat(4,minmax(7rem,1fr))]')
    expect(primaryFieldsWithImage).not.toHaveClass('xl:grid-cols-[minmax(12rem,2fr)_repeat(4,minmax(7rem,1fr))]')
    expect(screen.queryByText('已有图片')).not.toBeInTheDocument()
    expect(screen.queryByText('assets/lore/images/lin-chuan/20260101000000/image.png')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '放大查看资料图片' }))

    const previewDialog = screen.getByRole('dialog', { name: '林川' })
    expect(within(previewDialog).getByTestId('image-preview-viewport')).toBeInTheDocument()
  })

  it('uploads a current image for one lore item from the generation dialog', async () => {
    const user = userEvent.setup()
    const item = loreItem('lin-chuan', '林川')
    const withImage = {
      ...item,
      updated_at: '2026-01-01T00:00:01Z',
      image: loreImage('assets/lore/images/lin-chuan/upload/image.png'),
    }
    vi.mocked(getProjectLoreItems).mockResolvedValue([item])
    vi.mocked(updateProjectLoreItem).mockResolvedValue(item)
    vi.mocked(uploadLoreItemImage).mockResolvedValue(withImage)

    render(<SettingPanel mode="lore" imagePresets={[imagePreset('game-cg', '游戏 CG')]} />)

    await user.click(await screen.findByRole('button', { name: /林川/ }))
    await user.click(screen.getByRole('button', { name: '打开图片生成' }))
    const dialog = await screen.findByRole('dialog', { name: '生成图片' })
    const file = new File(['image'], 'portrait.png', { type: 'image/png' })
    await user.upload(within(dialog).getByLabelText('图片文件'), file)

    await waitFor(() => {
      expect(uploadLoreItemImage).toHaveBeenCalledWith(TEST_PROJECT_ID, 'lin-chuan', file)
    })
    expect(toast.success).toHaveBeenCalledWith('资料图片已上传')
    await user.click(within(dialog).getByRole('button', { name: '关闭' }))
    expect(await screen.findByRole('img', { name: '林川' })).toHaveAttribute('src', '/api/projects/project-workspace/files/asset?path=assets%2Flore%2Fimages%2Flin-chuan%2Fupload%2Fimage.png')
  })

  it('switches the lore body between rich text and raw markdown editing', async () => {
    const item = {
      ...loreItem('raw-lore', '源码资料'),
      content: '## 标题\n\n正文内容',
    }
    vi.mocked(getProjectLoreItems).mockResolvedValue([item])

    render(<SettingPanel mode="lore" imagePresets={[]} />)

    fireEvent.click(await screen.findByRole('button', { name: /源码资料/ }))
    const editor = screen.getByRole('region', { name: '资料编辑区' })

    // 默认富文本编辑
    expect(within(editor).getByTestId('lore-rich-editor')).toBeInTheDocument()

    // 切换为 Raw 源码编辑：同一草稿内容（草稿加载时已归一化补结尾换行），等宽 textarea
    fireEvent.click(within(editor).getByRole('button', { name: 'Raw' }))
    expect(within(editor).queryByTestId('lore-rich-editor')).not.toBeInTheDocument()
    const raw = within(editor).getByRole('textbox', { name: '正文' }) as HTMLTextAreaElement
    expect(raw.value).toBe('## 标题\n\n正文内容\n')
    expect(raw).toHaveClass('font-mono')
    expect(within(editor).getByRole('button', { name: 'Raw' })).toHaveAttribute('aria-pressed', 'true')

    fireEvent.change(screen.getByRole('textbox', { name: '搜索资料' }), { target: { value: '正文内容' } })
    expect(raw.parentElement?.querySelector('mark')).toHaveTextContent('正文内容')
    expect(raw).not.toBeDisabled()

    fireEvent.change(raw, { target: { value: '## 标题\n\n正文内容\n\n新增一行' } })

    // 切回富文本，修改后的内容回灌
    fireEvent.click(within(editor).getByRole('button', { name: '富文本' }))
    const rich = within(editor).getByTestId('lore-rich-editor') as HTMLTextAreaElement
    expect(rich.value).toBe('## 标题\n\n正文内容\n\n新增一行')
  })

  it('navigates document review comments to the matching lore item and snapshots that Project resource', async () => {
    const first = loreItem('first-lore', '第一条资料')
    const target = {
      ...loreItem('reviewed-lore', '被评论的资料'),
      content: '## 服务端正文',
      updated_at: 'reviewed-r2',
    }
    const comment = documentReviewComment(target.id)
    const controller = reviewController([comment])
    vi.mocked(getProjectLoreItems).mockResolvedValue([first, target])

    render(
      <SettingPanel
        mode="lore"
        imagePresets={[]}
        documentReview={controller}
        documentReviewNavigationIntent={{ commentID: comment.id, nonce: 7 }}
      />,
    )

    await waitFor(() => {
      const review = markdownRichEditorProps.at(-1)?.review
      expect(review?.target).toEqual({ kind: 'lore_item', id: target.id, field: 'content' })
      expect(review?.resourceLabel).toBe(target.name)
      expect(review?.navigationIntent).toEqual({ commentID: comment.id, nonce: 7 })
    })

    const review = markdownRichEditorProps.at(-1)?.review
    expect(markdownRichEditorProps.at(-1)?.projectId).toBe(TEST_PROJECT_ID)
    await expect(review?.prepareSnapshot()).resolves.toEqual({
      content: target.content,
      revision: target.updated_at,
    })
    expect(getProjectLoreItems).toHaveBeenLastCalledWith(TEST_PROJECT_ID)
  })

  it('saves lore item enabled status from a switch', async () => {
    const user = userEvent.setup()
    const item = loreItem('lin-chuan', '林川')
    vi.mocked(getProjectLoreItems).mockResolvedValue([item])
    vi.mocked(updateProjectLoreItem).mockImplementation(async (_projectId, id, input) => ({
      ...item,
      ...input,
      id,
      updated_at: '2026-01-01T00:00:01Z',
    }) as LoreItem)

    render(<SettingPanel mode="lore" imagePresets={[imagePreset('game-cg', '游戏 CG')]} />)

    await user.click(await screen.findByRole('button', { name: /林川/ }))
    const statusSwitch = screen.getByRole('switch', { name: '停用状态' })
    expect(statusSwitch).toBeChecked()
    await user.click(statusSwitch)
    expect(screen.getByRole('switch', { name: '启用状态' })).not.toBeChecked()

    flushSettingPanelAutosave()

    await waitFor(() => expect(updateProjectLoreItem).toHaveBeenCalled())
    expect(updateProjectLoreItem).toHaveBeenCalledWith(
      TEST_PROJECT_ID,
      'lin-chuan',
      expect.objectContaining({ enabled: false }),
      '2026-01-01T00:00:00Z',
    )
  })

  it('warns without blocking when resident lore exceeds 32 KB', async () => {
    const user = userEvent.setup()
    const item = { ...loreItem('resident-rules', '常驻规则', 'rule'), load_mode: 'resident' as const, content: 'x'.repeat(33 * 1024) }
    vi.mocked(getProjectLoreItems).mockResolvedValue([item])

    render(<SettingPanel mode="lore" imagePresets={[imagePreset('game-cg', '游戏 CG')]} />)

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
    vi.mocked(getProjectLoreItems).mockResolvedValue([resident, automatic, manual])

    render(<SettingPanel mode="lore" imagePresets={[imagePreset('game-cg', '游戏 CG')]} />)

    const residentButton = await screen.findByRole('button', { name: /常驻人物/ })
    expect(within(residentButton).getByText('常驻')).toBeInTheDocument()
    expect(within(screen.getByRole('button', { name: /自动人物/ })).getByText('按需')).not.toHaveAttribute('title')
    expect(within(screen.getByRole('button', { name: /手动人物/ })).getByText('按需')).not.toHaveAttribute('title')

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
    vi.mocked(getProjectLoreItems).mockResolvedValueOnce([item]).mockResolvedValue([])
    vi.mocked(deleteProjectLoreItem).mockResolvedValue(undefined)

    render(<SettingPanel mode="lore" imagePresets={[imagePreset('game-cg', '游戏 CG')]} />)

    await user.click(await screen.findByRole('button', { name: /林川/ }))
    await user.click(screen.getByRole('button', { name: '删除资料' }))

    const dialog = await screen.findByRole('alertdialog', { name: '删除资料' })
    expect(within(dialog).getByText('删除资料「林川」？')).toBeInTheDocument()
    expect(confirmSpy).not.toHaveBeenCalled()

    await user.click(within(dialog).getByRole('button', { name: '删除' }))

    await waitFor(() => {
      expect(deleteProjectLoreItem).toHaveBeenCalledWith(TEST_PROJECT_ID, 'lin-chuan')
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
    vi.mocked(getProjectLoreItems).mockResolvedValue([lin, harbor])
    render(<SettingPanel mode="lore" imagePresets={[imagePreset('game-cg', '游戏 CG'), imagePreset('ink-wash', '水墨风格')]} />)

    await user.click(await screen.findByRole('button', { name: '批量生成资料图片' }))
    const batchDialog = await screen.findByRole('dialog', { name: '批量生成资料图片' })
    const presetField = within(batchDialog).getByText('图像方案').closest('label') as HTMLElement
    await user.click(within(presetField).getByRole('combobox'))
    await user.click(screen.getByRole('option', { name: '水墨风格' }))
    await user.type(screen.getByPlaceholderText('搜索资料项'), '林川')
    await user.click(screen.getByRole('button', { name: '全选当前结果' }))
    await user.click(screen.getByRole('button', { name: '开始生成' }))

    await waitFor(() => {
      expect(configManagerChatProps.at(-1)).toMatchObject({
        origin: 'lore',
        resourceId: '__config_manager_lore__',
        initialInstructionKey: expect.stringMatching(/^lore-images-/),
      })
      expect(configManagerChatProps.at(-1)?.initialInstruction).toContain('Exact lore item IDs: ["lin-chuan"]')
      expect(configManagerChatProps.at(-1)?.initialInstruction).toContain('Image preset ID: "ink-wash"')
      expect(configManagerChatProps.at(-1)?.initialInstruction).toContain('Overwrite existing images: no.')
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
  return <PresetPanelHarness />
}

function PresetPanelHarness() {
  const [tellers, setTellers] = useState([teller('classic', '经典叙事')])
  const [storyDirectors, setStoryDirectors] = useState([storyDirector('default', '默认导演')])
  const [imagePresets, setImagePresets] = useState([imagePreset('game-cg', '游戏 CG')])

  return (
    <SettingPanel
      mode="teller"
      tellers={tellers}
      storyDirectors={storyDirectors}
      imagePresets={imagePresets}
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
    revision: 'sha256:fixture',
    updated_at: '2026-01-01T00:00:00Z',
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
    revision: 'sha256:fixture',
    updated_at: '2026-01-01T00:00:00Z',
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
    revision: 'sha256:fixture',
    updated_at: '2026-01-01T00:00:00Z',
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
    revision: 'sha256:fixture',
    updated_at: '2026-01-01T00:00:00Z',
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
    revision: 'sha256:fixture',
    updated_at: '2026-01-01T00:00:00Z',
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

function documentReviewComment(loreItemID: string): DocumentReviewComment {
  return {
    id: 'comment-lore-review',
    thread_id: 'thread-lore-review',
    target: { kind: 'lore_item', id: loreItemID, field: 'content' },
    body: 'Review this paragraph',
    anchor: {
      kind: 'text-range',
      encoding: 'utf8-bytes-v1',
      revision: 'reviewed-r2',
      start: 3,
      end: 9,
      quote: '服务端',
      display_quote: '服务端',
    },
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  }
}

function reviewController(comments: DocumentReviewComment[]): DocumentReviewController {
  return {
    comments,
    onCreate: vi.fn(async (request: CreateDocumentCommentRequest) => ({
      ...documentReviewComment(request.target.id),
      target: request.target,
      anchor: request.anchor,
      body: request.body,
    })),
    onUpdate: vi.fn(async (comment, body) => ({ ...comment, body })),
    onDelete: vi.fn(async (comment) => ({ ...comment, deleted: true })),
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
