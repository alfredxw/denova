import { act, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { preserveAutosaveConflict } from '@/lib/api-client/autosave-conflicts'
import type { ImagePreset } from '../../types'
import { usePresetDraftSync, type PresetResources } from './use-preset-resources'
import { usePresetResourceAutosave } from './usePresetResourceAutosave'
import { presetResourceDraftSignature } from './presetResources'

vi.mock('@/lib/api-client/autosave-conflicts', () => ({
  preserveAutosaveConflict: vi.fn(),
}))

interface HarnessControls {
  rename: (name: string) => void
}

let controls: HarnessControls | null = null

describe('usePresetDraftSync', () => {
  afterEach(() => {
    controls = null
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  it('accepts its own saved baseline without archiving edits made while the save was in flight', async () => {
    vi.useFakeTimers()
    vi.mocked(preserveAutosaveConflict).mockResolvedValue({
      id: 'unexpected-conflict',
      path: 'conflicts/unexpected-conflict.json',
      storage: 'server',
    })
    const firstSave = deferred<ImagePreset>()
    const save = vi.fn()
      .mockImplementationOnce(() => firstSave.promise)
      .mockImplementationOnce(async (_id: string, payload: Partial<ImagePreset>) => ({
        ...imagePreset('newer local edit', 'r3'),
        ...payload,
        updated_at: 'r3',
      }))

    render(<PresetDraftSyncHarness save={save} />)
    await act(async () => { await Promise.resolve() })

    act(() => controls?.rename('submitted edit'))
    await advanceAutosave()
    expect(save).toHaveBeenCalledWith(
      'image-preset',
      expect.objectContaining({ name: 'submitted edit' }),
      'r1',
    )

    act(() => controls?.rename('newer local edit'))
    firstSave.resolve(imagePreset('submitted edit', 'r2'))
    await act(async () => {
      await firstSave.promise
      await Promise.resolve()
    })

    expect(preserveAutosaveConflict).not.toHaveBeenCalled()
    expect(screen.getByTestId('draft')).toHaveTextContent('newer local edit:r2')

    await advanceAutosave()
    expect(save).toHaveBeenLastCalledWith(
      'image-preset',
      expect.objectContaining({ name: 'newer local edit', updated_at: 'r2' }),
      'r2',
    )
  })
})

function PresetDraftSyncHarness({
  save,
}: {
  save: (id: string, payload: Partial<ImagePreset>, baseRevision?: string) => Promise<ImagePreset>
}) {
  const [imagePresets, setImagePresets] = useState([imagePreset('original', 'r1')])
  const [imageDraft, setImageDraft] = useState<ImagePreset | null>(null)
  const imageAutosave = usePresetResourceAutosave<ImagePreset, Partial<ImagePreset>, ImagePreset>({
    draft: imageDraft,
    active: true,
    scopeKey: 'workspace',
    makePayload: (draft) => ({ ...draft }),
    signature: presetResourceDraftSignature,
    baselineFromSaved: (saved) => saved,
    save,
    onSaved: (saved) => {
      setImagePresets((current) => current.map((item) => item.id === saved.id ? saved : item))
    },
  })
  const resources = {
    workspace: 'workspace',
    tellers: [],
    activeTellerId: '',
    tellerDraft: null,
    setTellerDraft: noop,
    setActiveSlotId: noop,
    storyDirectors: [],
    activeStoryDirectorId: '',
    storyDirectorDraft: null,
    setStoryDirectorDraft: noop,
    imagePresets,
    activeImagePresetId: 'image-preset',
    imagePresetDraft: imageDraft,
    setImagePresetDraft: setImageDraft,
    eventPackages: [],
    activeEventPackageId: '',
    eventPackageDraft: null,
    setEventPackageDraft: noop,
    ruleSystems: [],
    activeRuleSystemId: '',
    ruleSystemDraft: null,
    setRuleSystemDraft: noop,
    actorStates: [],
    activeActorStateId: '',
    actorStateDraft: null,
    setActorStateDraft: noop,
  } as unknown as PresetResources
  const inactiveAutosave = { isBaselineAcknowledged: () => false, resetBaseline: noop }

  usePresetDraftSync(resources, {
    teller: inactiveAutosave,
    director: inactiveAutosave,
    image: imageAutosave,
    event: inactiveAutosave,
    rule: inactiveAutosave,
    'actor-state': inactiveAutosave,
  })

  controls = {
    rename: (name) => setImageDraft((current) => current ? { ...current, name } : current),
  }
  return <output data-testid="draft">{imageDraft ? `${imageDraft.name}:${imageDraft.updated_at}` : 'empty'}</output>
}

function imagePreset(name: string, revision: string): ImagePreset {
  return {
    version: 1,
    id: 'image-preset',
    name,
    description: '',
    custom: true,
    updated_at: revision,
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

async function advanceAutosave() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1300)
  })
}

function noop() {}
