import { act, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { speechPlayer } from '@/features/speech/player'
import type { InteractiveTurnPersistedEvent, Snapshot, StorySummary, TurnEvent } from './types'
import { useStorySpeech } from './use-story-speech'

vi.mock('@/features/speech/hooks', () => ({ useSpeechSettings: () => ({ settings: { endpoint: 'http://localhost/speech', model: 'm', voice: 'v', api_key: '' } }) }))

const turn = (id: string, narrative = '她说：“你好。”') => ({ id, narrative }) as TurnEvent
const story = (auto_read = true) => ({ id: 'story', speech_settings: { auto_read, mode: 'all', ignore_asterisks: false } }) as StorySummary
const snapshot = (turns: TurnEvent[]) => ({ story_id: 'story', branch_id: 'main', turns }) as Snapshot
const event = (id: string) => ({ story_id: 'story', branch_id: 'main', turn: turn(id) }) as InteractiveTurnPersistedEvent
afterEach(() => vi.restoreAllMocks())

describe('game speech lifecycle', () => {
  it('reads a new commit once but never hydration or checkpoint replay', () => {
    const read = vi.spyOn(speechPlayer, 'read').mockImplementation(() => {})
    const { result, rerender } = renderHook(props => useStorySpeech(props), { initialProps: { owner: 'game', story: story(), snapshot: snapshot([turn('history')]), active: true } })
    expect(read).not.toHaveBeenCalled()
    act(() => result.current.onPersisted(event('history')))
    act(() => result.current.onPersisted(event('replayed'), { replayed: true }))
    expect(read).not.toHaveBeenCalled()
    act(() => { result.current.onPersisted(event('new')); result.current.onPersisted(event('new')) })
    expect(read).toHaveBeenCalledTimes(1)
    expect(read.mock.calls[0][0]).toMatchObject({ turnId: 'new', enqueue: true })
    rerender({ owner: 'game', story: story(), snapshot: snapshot([turn('new'), turn('another-version')]), active: true })
    expect(read).toHaveBeenCalledTimes(1)
  })
  it('uses the switch state at completion even through an older stream callback', () => {
    const read = vi.spyOn(speechPlayer, 'read').mockImplementation(() => {})
    const { result, rerender } = renderHook(props => useStorySpeech(props), { initialProps: { owner: 'game', story: story(false), snapshot: snapshot([]), active: true } })
    const commit = result.current.onPersisted
    rerender({ owner: 'game', story: story(true), snapshot: snapshot([]), active: true })
    act(() => commit(event('enabled')))
    expect(read).toHaveBeenCalledTimes(1)
    rerender({ owner: 'game', story: story(false), snapshot: snapshot([]), active: true })
    act(() => commit(event('disabled')))
    expect(read).toHaveBeenCalledTimes(1)
    rerender({ owner: 'game', story: story(true), snapshot: snapshot([]), active: false })
    act(() => commit(event('away')))
    expect(read).toHaveBeenCalledTimes(1)
  })
  it('stops for text edits, filters and leaving the game; unmount cleans up', () => {
    vi.spyOn(speechPlayer, 'read').mockImplementation(() => {})
    const stop = vi.spyOn(speechPlayer, 'stopOwner')
    const { result, rerender, unmount } = renderHook(props => useStorySpeech(props), { initialProps: { owner: 'game', story: story(), snapshot: snapshot([turn('one')]), active: true } })
    act(() => result.current.read(turn('one')))
    stop.mockClear()
    rerender({ owner: 'game', story: story(), snapshot: snapshot([turn('one', 'edited')]), active: true })
    expect(stop).toHaveBeenCalledWith('game')
    stop.mockClear()
    rerender({ owner: 'game', story: story(), snapshot: snapshot([]), active: false })
    expect(stop).toHaveBeenCalledWith('game')
    stop.mockClear()
    unmount()
    expect(stop).toHaveBeenCalledWith('game')
  })
})
