import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, render } from '@testing-library/react'
import { StrictMode } from 'react'
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

  it('debounces rapid edits and saves only the latest draft', async () => {
    vi.useFakeTimers()
    const save = vi.fn(async (settings: Settings) => layered(settings))
    const onSaved = vi.fn()
    const view = render(
      <HookHarness
        draft={{ language: 'zh-CN' }}
        saved={{ language: 'zh-CN' }}
        save={save}
        onSaved={onSaved}
      />,
    )

    view.rerender(
      <HookHarness
        draft={{ language: 'en-US' }}
        saved={{ language: 'zh-CN' }}
        save={save}
        onSaved={onSaved}
      />,
    )
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500)
    })
    view.rerender(
      <HookHarness
        draft={{ language: 'auto' }}
        saved={{ language: 'zh-CN' }}
        save={save}
        onSaved={onSaved}
      />,
    )

    await advanceAutoSaveTimer()
    expect(save).toHaveBeenCalledTimes(1)
    expect(save).toHaveBeenLastCalledWith({ language: 'auto' })
  })

  it('saves the newest draft after an in-flight save completes', async () => {
    vi.useFakeTimers()
    const firstSave = deferred<LayeredSettings>()
    const save = vi.fn((settings: Settings) => settings.language === 'en-US' ? firstSave.promise : Promise.resolve(layered(settings)))
    const onSaved = vi.fn()
    const onSavingChange = vi.fn()
    const view = render(
      <HookHarness
        draft={{ language: 'zh-CN' }}
        saved={{ language: 'zh-CN' }}
        save={save}
        onSaved={onSaved}
        onSavingChange={onSavingChange}
      />,
    )

    view.rerender(
      <HookHarness
        draft={{ language: 'en-US' }}
        saved={{ language: 'zh-CN' }}
        save={save}
        onSaved={onSaved}
        onSavingChange={onSavingChange}
      />,
    )
    await advanceAutoSaveTimer()
    expect(save).toHaveBeenCalledTimes(1)
    expect(onSavingChange).toHaveBeenLastCalledWith(true)

    view.rerender(
      <HookHarness
        draft={{ language: 'auto' }}
        saved={{ language: 'zh-CN' }}
        save={save}
        onSaved={onSaved}
        onSavingChange={onSavingChange}
      />,
    )
    await advanceAutoSaveTimer()
    expect(save).toHaveBeenCalledTimes(1)

    await act(async () => {
      firstSave.resolve(layered({ language: 'en-US' }))
      await firstSave.promise
      await Promise.resolve()
    })
    await advanceAutoSaveTimer()

    expect(save).toHaveBeenCalledTimes(2)
    expect(save).toHaveBeenLastCalledWith({ language: 'auto' })
    expect(onSavingChange).toHaveBeenLastCalledWith(false)
  })

  it('resets saving state after StrictMode remount checks', async () => {
    vi.useFakeTimers()
    const save = vi.fn(async (settings: Settings) => layered(settings))
    const onSaved = vi.fn()
    const onSavingChange = vi.fn()
    const view = render(
      <StrictMode>
        <HookHarness
          draft={{ language: 'zh-CN' }}
          saved={{ language: 'zh-CN' }}
          save={save}
          onSaved={onSaved}
          onSavingChange={onSavingChange}
        />
      </StrictMode>,
    )

    view.rerender(
      <StrictMode>
        <HookHarness
          draft={{ language: 'en-US' }}
          saved={{ language: 'zh-CN' }}
          save={save}
          onSaved={onSaved}
          onSavingChange={onSavingChange}
        />
      </StrictMode>,
    )
    await advanceAutoSaveTimer()

    expect(save).toHaveBeenCalledTimes(1)
    expect(onSavingChange).toHaveBeenLastCalledWith(false)
  })

  it('passes base revision and does not retry the same failed draft', async () => {
    vi.useFakeTimers()
    const save = vi.fn(async () => {
      throw new Error('conflict')
    })
    const onError = vi.fn()
    const view = render(
      <HookHarness
        draft={{ language: 'zh-CN' }}
        saved={{ language: 'zh-CN' }}
        baseRevision="r1"
        save={save}
        onSaved={() => undefined}
        onError={onError}
      />,
    )

    view.rerender(
      <HookHarness
        draft={{ language: 'en-US' }}
        saved={{ language: 'zh-CN' }}
        baseRevision="r1"
        save={save}
        onSaved={() => undefined}
        onError={onError}
      />,
    )
    await advanceAutoSaveTimer()
    expect(save).toHaveBeenCalledTimes(1)
    expect(save).toHaveBeenLastCalledWith({ language: 'en-US' }, 'r1')
    expect(onError).toHaveBeenCalledWith('conflict')

    await advanceAutoSaveTimer()
    expect(save).toHaveBeenCalledTimes(1)

    view.rerender(
      <HookHarness
        draft={{ language: 'auto' }}
        saved={{ language: 'zh-CN' }}
        baseRevision="r1"
        save={save}
        onSaved={() => undefined}
        onError={onError}
      />,
    )
    await advanceAutoSaveTimer()
    expect(save).toHaveBeenCalledTimes(2)
  })
})

function HookHarness({
  draft,
  saved,
  baseRevision,
  save,
  onSaved,
  onSavingChange = () => undefined,
  onError = () => undefined,
}: {
  draft: Settings
  saved: Settings
  baseRevision?: string
  save: (settings: Settings, baseRevision?: string) => Promise<LayeredSettings>
  onSaved: (next: LayeredSettings) => void
  onSavingChange?: (saving: boolean) => void
  onError?: (message: string) => void
}) {
  useAutoSaveSettings({
    draft,
    saved,
    baseRevision,
    ready: true,
    save,
    onSavingChange,
    onSaved,
    onError,
  })
  return null
}

async function advanceAutoSaveTimer() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1100)
  })
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
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
