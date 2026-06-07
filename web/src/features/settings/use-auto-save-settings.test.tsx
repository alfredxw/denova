import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, render } from '@testing-library/react'
import type { LayeredSettings, Settings } from './types'
import { useAutoSaveSettings } from './use-auto-save-settings'

describe('useAutoSaveSettings', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('waits for draft to sync before saving user edits', async () => {
    vi.useFakeTimers()
    const save = vi.fn(async (settings: Settings) => layered(settings))
    const onSaved = vi.fn()

    const view = render(
      <HookHarness
        draft={{}}
        saved={{ language: 'zh-CN', reading_font_size: 16 }}
        save={save}
        onSaved={onSaved}
      />,
    )

    await advanceAutoSaveTimer()
    expect(save).not.toHaveBeenCalled()

    view.rerender(
      <HookHarness
        draft={{ language: 'zh-CN', reading_font_size: 16 }}
        saved={{ language: 'zh-CN', reading_font_size: 16 }}
        save={save}
        onSaved={onSaved}
      />,
    )
    await advanceAutoSaveTimer()
    expect(save).not.toHaveBeenCalled()

    view.rerender(
      <HookHarness
        draft={{ language: 'en-US', reading_font_size: 16 }}
        saved={{ language: 'zh-CN', reading_font_size: 16 }}
        save={save}
        onSaved={onSaved}
      />,
    )
    await advanceAutoSaveTimer()
    expect(save).toHaveBeenCalledTimes(1)
    expect(save).toHaveBeenCalledWith({ language: 'en-US', reading_font_size: 16 })
    expect(onSaved).toHaveBeenCalledTimes(1)
  })
})

function HookHarness({
  draft,
  saved,
  save,
  onSaved,
}: {
  draft: Settings
  saved: Settings
  save: (settings: Settings) => Promise<LayeredSettings>
  onSaved: (next: LayeredSettings) => void
}) {
  useAutoSaveSettings({
    draft,
    saved,
    ready: true,
    save,
    onSavingChange: () => undefined,
    onSaved,
    onError: () => undefined,
  })
  return null
}

async function advanceAutoSaveTimer() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(700)
  })
}

function layered(settings: Settings): LayeredSettings {
  return {
    default: {},
    global: {},
    user: settings,
    workspace: {},
    effective: settings,
    paths: {
      nova_dir: '',
      user_config: '',
      workspace_config: '',
    },
  }
}
