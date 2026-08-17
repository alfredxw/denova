import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import {
  createSettingsMergePatch,
  fetchProjectSettings,
  fetchSettings,
  patchProjectSettings,
  patchSettings,
  refreshProjectSettings,
  refreshSettings,
} from '@/features/settings/api'
import type { LayeredSettings, Settings } from '@/features/settings/types'
import { setConfiguredLocale } from '@/i18n'
import { APIError } from '@/lib/api-client'
import { ImageGenerationSettingsMenu } from './ImageGenerationSettingsMenu'
import { WritingImagePresetMenu } from './WritingComposerSettingsMenu'

vi.mock('@/features/settings/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/settings/api')>()
  return {
    ...actual,
    fetchSettings: vi.fn(),
    fetchProjectSettings: vi.fn(),
    patchSettings: vi.fn(),
    patchProjectSettings: vi.fn(),
    refreshSettings: vi.fn(),
    refreshProjectSettings: vi.fn(),
  }
})

let latestSettings: LayeredSettings

describe('ImageAgentModelSettingsMenu', () => {
  beforeEach(() => {
    setConfiguredLocale('zh-CN')
    latestSettings = settingsSnapshot({
      user: {
        agent_models: { image: { profile_id: 'reasoning' } },
        default_image_api_profile_id: 'illustrator',
      },
      effective: {
        model_profiles: [
          { id: 'default', name: 'GPT 默认', model: 'gpt-default' },
          { id: 'reasoning', name: 'Reasoning', model: 'reasoning-v1' },
          { id: 'ds-flash', name: 'DS Flash', model: 'ds-flash' },
        ],
        image_api_profiles: [
          { id: 'illustrator', name: 'Illustrator', openai_model: 'image-v1' },
          { id: 'flux', name: 'Flux', openai_model: 'flux-pro' },
        ],
        agent_models: { image: { profile_id: 'reasoning' } },
        default_image_api_profile_id: 'illustrator',
      },
      revisions: { user: 'user-r1' },
    })
    vi.mocked(fetchSettings).mockReset().mockImplementation(async () => latestSettings)
    vi.mocked(fetchProjectSettings).mockReset().mockImplementation(async () => latestSettings)
    vi.mocked(refreshSettings).mockReset().mockImplementation(async () => latestSettings)
    vi.mocked(refreshProjectSettings).mockReset().mockImplementation(async () => latestSettings)
    vi.mocked(patchSettings).mockReset().mockImplementation(async (_layer, changes) => {
      const user = applySettingsPatch(latestSettings.user, changes as Settings)
      latestSettings = settingsSnapshot({
        ...latestSettings,
        user,
        effective: { ...latestSettings.effective, ...user },
        revisions: { user: `user-r${Number(latestSettings.revisions?.user?.split('r')[1] || 1) + 1}` },
      })
      return latestSettings
    })
    vi.mocked(patchProjectSettings).mockReset().mockImplementation(async (_projectId, layer, changes) => {
      const baseline = layer === 'workspace' ? latestSettings.workspace : latestSettings.user
      const updated = applySettingsPatch(baseline, changes as Settings)
      latestSettings = settingsSnapshot({
        ...latestSettings,
        [layer]: updated,
        effective: { ...latestSettings.effective, ...updated },
        revisions: { ...latestSettings.revisions, [layer]: `${layer}-r2` },
      })
      return latestSettings
    })
  })

  afterEach(() => setConfiguredLocale('zh-CN'))

  it('groups the language model, image model, and image preset under image generation options', async () => {
    const user = userEvent.setup()
    renderMenu()

    await user.click(screen.getByRole('button', { name: 'open actions' }))
    expect(screen.getByRole('menuitem', { name: '图像生成选项' })).toBeInTheDocument()
    expect(screen.queryByRole('separator')).not.toBeInTheDocument()
    expect(screen.queryByText('Image Agent')).not.toBeInTheDocument()
    await user.hover(screen.getByRole('menuitem', { name: '图像生成选项' }))

    expect(await screen.findByRole('menuitem', { name: '语言模型' })).toHaveTextContent('Reasoning')
    expect(screen.getByRole('menuitem', { name: '图像模型' })).toHaveTextContent('Illustrator')
    expect(screen.getByRole('menuitem', { name: '图像方案' })).toHaveTextContent('游戏 CG')
  })

  it('persists the language model to the same user settings used by the Agents page', async () => {
    const user = userEvent.setup()
    renderMenu()

    await user.click(screen.getByRole('button', { name: 'open actions' }))
    await user.hover(screen.getByRole('menuitem', { name: '图像生成选项' }))
    fireEvent.keyDown(await screen.findByRole('menuitem', { name: '语言模型' }), { key: 'ArrowRight' })
    fireEvent.click(await screen.findByRole('menuitem', { name: /DS Flash/ }))

    await waitFor(() => expect(patchSettings).toHaveBeenCalledWith(
      'user',
      { agent_models: { image: { profile_id: 'ds-flash' } } },
      'user-r1',
    ))
  })

  it('persists the output image model to the same user settings used by the Agents page', async () => {
    const user = userEvent.setup()
    renderMenu()

    await user.click(screen.getByRole('button', { name: 'open actions' }))
    await user.hover(screen.getByRole('menuitem', { name: '图像生成选项' }))
    fireEvent.keyDown(await screen.findByRole('menuitem', { name: '图像模型' }), { key: 'ArrowRight' })
    fireEvent.click(await screen.findByRole('menuitem', { name: /Flux/ }))

    await waitFor(() => expect(patchSettings).toHaveBeenCalledWith(
      'user',
      { default_image_api_profile_id: 'flux' },
      'user-r1',
    ))
  })

  it('updates the Project layer when it currently overrides the output image model', async () => {
    const user = userEvent.setup()
    latestSettings = settingsSnapshot({
      ...latestSettings,
      workspace: { default_image_api_profile_id: 'illustrator' },
      effective: { ...latestSettings.effective, default_image_api_profile_id: 'illustrator' },
      revisions: { user: 'user-r1', workspace: 'workspace-r1' },
    })
    renderMenu('project-book')

    await user.click(screen.getByRole('button', { name: 'open actions' }))
    await user.hover(screen.getByRole('menuitem', { name: '图像生成选项' }))
    fireEvent.keyDown(await screen.findByRole('menuitem', { name: '图像模型' }), { key: 'ArrowRight' })
    fireEvent.click(await screen.findByRole('menuitem', { name: /Flux/ }))

    await waitFor(() => expect(patchProjectSettings).toHaveBeenCalledWith(
      'project-book',
      'workspace',
      { default_image_api_profile_id: 'flux' },
      'workspace-r1',
    ))
    expect(patchSettings).not.toHaveBeenCalled()
  })

  it('replays a quick selection over the latest user settings after a revision conflict', async () => {
    const user = userEvent.setup()
    const concurrentSettings = settingsSnapshot({
      ...latestSettings,
      user: { ...latestSettings.user, theme: 'light' },
      effective: { ...latestSettings.effective, theme: 'light' },
      revisions: { user: 'user-r2' },
    })
    const savedSettings = settingsSnapshot({
      ...concurrentSettings,
      user: {
        ...concurrentSettings.user,
        agent_models: { image: { profile_id: 'ds-flash' } },
      },
      effective: {
        ...concurrentSettings.effective,
        agent_models: { image: { profile_id: 'ds-flash' } },
      },
      revisions: { user: 'user-r3' },
    })
    vi.mocked(patchSettings)
      .mockReset()
      .mockRejectedValueOnce(new APIError('revision conflict', { status: 409, code: 'revision_conflict' }))
      .mockResolvedValueOnce(savedSettings)
    vi.mocked(refreshSettings).mockReset().mockResolvedValue(concurrentSettings)
    renderMenu()

    await user.click(screen.getByRole('button', { name: 'open actions' }))
    await user.hover(screen.getByRole('menuitem', { name: '图像生成选项' }))
    fireEvent.keyDown(await screen.findByRole('menuitem', { name: '语言模型' }), { key: 'ArrowRight' })
    fireEvent.click(await screen.findByRole('menuitem', { name: /DS Flash/ }))

    await waitFor(() => expect(patchSettings).toHaveBeenNthCalledWith(
      2,
      'user',
      { agent_models: { image: { profile_id: 'ds-flash' } } },
      'user-r2',
    ))
    expect(savedSettings.user.theme).toBe('light')
  })

  it('renders the same image generation hierarchy in English', async () => {
    setConfiguredLocale('en-US')
    const user = userEvent.setup()
    renderMenu()

    await user.click(screen.getByRole('button', { name: 'open actions' }))
    expect(screen.getByRole('menuitem', { name: 'Image Generation Options' })).toBeInTheDocument()
    await user.hover(screen.getByRole('menuitem', { name: 'Image Generation Options' }))

    expect(await screen.findByRole('menuitem', { name: 'Language Model' })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: 'Image Model' })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: 'Image Preset' })).toBeInTheDocument()
  })
})

function renderMenu(projectId = '') {
  return render(
    <DropdownMenu>
      <DropdownMenuTrigger>open actions</DropdownMenuTrigger>
      <DropdownMenuContent>
        <ImageGenerationSettingsMenu projectId={projectId}>
          <WritingImagePresetMenu
            enabled
            imagePresets={[{ id: 'game-cg', name: '游戏 CG', description: '', prompt: '', custom: false, version: 1 }]}
            imagePresetID="game-cg"
            onChange={() => {}}
          />
        </ImageGenerationSettingsMenu>
      </DropdownMenuContent>
    </DropdownMenu>,
  )
}

function settingsSnapshot(patch: Partial<LayeredSettings>): LayeredSettings {
  return {
    default: {},
    global: {},
    user: {},
    workspace: {},
    effective: {},
    paths: {
      denova_dir: '/denova',
      nova_dir: '/nova',
      user_config: '/nova/config.toml',
      workspace_config: '/book/.nova/config.toml',
    },
    resolved_agent_tool_manifests: {},
    resolved_agent_contexts: {},
    ...patch,
  }
}

function applySettingsPatch(current: Settings, patch: Settings): Settings {
  const draft = structuredClone(current)
  if (patch.default_image_api_profile_id) draft.default_image_api_profile_id = patch.default_image_api_profile_id
  if (patch.agent_models?.image) {
    draft.agent_models = {
      ...draft.agent_models,
      image: { ...draft.agent_models?.image, ...patch.agent_models.image },
    }
  }
  expect(createSettingsMergePatch(current, draft)).toEqual(patch)
  return draft
}
