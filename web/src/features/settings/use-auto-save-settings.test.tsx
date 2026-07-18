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

  it('cancels a pending save when the draft scope changes', async () => {
    vi.useFakeTimers()
    const save = vi.fn(async (settings: Settings) => layered(settings))
    const view = render(
      <HookHarness
        draft={{ language: 'zh-CN' }}
        saved={{ language: 'zh-CN' }}
        resetKey="user"
        save={save}
        onSaved={() => undefined}
      />,
    )

    view.rerender(
      <HookHarness
        draft={{ language: 'en-US' }}
        saved={{ language: 'zh-CN' }}
        resetKey="user"
        save={save}
        onSaved={() => undefined}
      />,
    )
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500)
    })

    view.rerender(
      <HookHarness
        draft={{ language: 'en-US' }}
        saved={{ theme: 'dark' }}
        ready={false}
        resetKey="workspace"
        save={save}
        onSaved={() => undefined}
      />,
    )
    view.rerender(
      <HookHarness
        draft={{ theme: 'dark' }}
        saved={{ theme: 'dark' }}
        resetKey="workspace"
        save={save}
        onSaved={() => undefined}
      />,
    )
    await advanceAutoSaveTimer()

    expect(save).not.toHaveBeenCalled()
  })

  it('treats an explicit server snapshot sync as the new baseline', async () => {
    vi.useFakeTimers()
    const save = vi.fn(async (settings: Settings) => layered(settings))
    const view = render(
      <HookHarness
        draft={{ language: 'zh-CN' }}
        saved={{ language: 'zh-CN' }}
        syncKey={1}
        save={save}
        onSaved={() => undefined}
      />,
    )

    view.rerender(
      <HookHarness
        draft={{ language: 'en-US' }}
        saved={{ language: 'en-US' }}
        syncKey={2}
        save={save}
        onSaved={() => undefined}
      />,
    )
    await advanceAutoSaveTimer()

    expect(save).not.toHaveBeenCalled()
  })

  it('reconciles a successful in-flight write after its scope is superseded', async () => {
    vi.useFakeTimers()
    const pending = deferred<LayeredSettings>()
    const save = vi.fn(() => pending.promise)
    const onSaved = vi.fn()
    const onStaleSuccess = vi.fn()
    const view = render(
      <HookHarness
        draft={{ language: 'zh-CN' }}
        saved={{ language: 'zh-CN' }}
        resetKey="user"
        save={save}
        onSaved={onSaved}
        onStaleSuccess={onStaleSuccess}
      />,
    )

    view.rerender(
      <HookHarness
        draft={{ language: 'en-US' }}
        saved={{ language: 'zh-CN' }}
        resetKey="user"
        save={save}
        onSaved={onSaved}
        onStaleSuccess={onStaleSuccess}
      />,
    )
    await advanceAutoSaveTimer()
    expect(save).toHaveBeenCalledOnce()

    view.rerender(
      <HookHarness
        draft={{ theme: 'dark' }}
        saved={{ theme: 'dark' }}
        resetKey="workspace"
        save={save}
        onSaved={onSaved}
        onStaleSuccess={onStaleSuccess}
      />,
    )
    const response = layered({ language: 'en-US' })
    await act(async () => {
      pending.resolve(response)
      await pending.promise
      await Promise.resolve()
    })

    expect(onSaved).not.toHaveBeenCalled()
    expect(onStaleSuccess).toHaveBeenCalledWith(response)
  })

  it('does not publish an in-flight save result after unmount', async () => {
    vi.useFakeTimers()
    const pending = deferred<LayeredSettings>()
    const onSaved = vi.fn()
    const onStaleSuccess = vi.fn()
    const onError = vi.fn()
    const view = render(
      <HookHarness
        draft={{ language: 'zh-CN' }}
        saved={{ language: 'zh-CN' }}
        save={() => pending.promise}
        onSaved={onSaved}
        onStaleSuccess={onStaleSuccess}
        onError={onError}
      />,
    )

    view.rerender(
      <HookHarness
        draft={{ language: 'en-US' }}
        saved={{ language: 'zh-CN' }}
        save={() => pending.promise}
        onSaved={onSaved}
        onStaleSuccess={onStaleSuccess}
        onError={onError}
      />,
    )
    await advanceAutoSaveTimer()
    view.unmount()

    await act(async () => {
      pending.resolve(layered({ language: 'en-US' }))
      await pending.promise
      await Promise.resolve()
    })

    expect(onSaved).not.toHaveBeenCalled()
    expect(onStaleSuccess).not.toHaveBeenCalled()
    expect(onError).not.toHaveBeenCalled()
  })
})

function HookHarness({
  draft,
  saved,
  baseRevision,
  resetKey,
  syncKey,
  ready = true,
  save,
  onSaved,
  onStaleSuccess,
  onSavingChange = () => undefined,
  onError = () => undefined,
}: {
  draft: Settings
  saved: Settings
  baseRevision?: string
  resetKey?: string
  syncKey?: string | number
  ready?: boolean
  save: (settings: Settings, baseRevision?: string) => Promise<LayeredSettings>
  onSaved: (next: LayeredSettings) => void
  onStaleSuccess?: (next: LayeredSettings) => void | Promise<void>
  onSavingChange?: (saving: boolean) => void
  onError?: (message: string) => void
}) {
  useAutoSaveSettings({
    draft,
    saved,
    baseRevision,
    ready,
    resetKey,
    syncKey,
    save,
    onSavingChange,
    onSaved,
    onStaleSuccess,
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
      denova_dir: '',
      nova_dir: '',
      user_config: '',
      workspace_config: '',
    },
  }
}
