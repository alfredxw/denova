import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { MaterialAudio } from './LoreMaterialDialog'
import { speechPlayer } from '@/features/speech/player'

afterEach(() => vi.restoreAllMocks())

it('coordinates previews with each other and speech, and stops when the view unmounts', () => {
  const pause = vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
  const stopSpeech = vi.spyOn(speechPlayer, 'stop').mockImplementation(() => {})
  const listeners = new Set<() => void>()
  const unsubscribe = vi.fn()
  vi.spyOn(speechPlayer, 'subscribe').mockImplementation((listener) => {
    listeners.add(listener)
    return () => {
      listeners.delete(listener)
      unsubscribe()
    }
  })
  const { unmount } = render(
    <>
      <MaterialAudio src="/one.wav" name="One" />
      <MaterialAudio src="/two.wav" name="Two" />
    </>,
  )
  const one = screen.getByLabelText('One')
  const two = screen.getByLabelText('Two')
  fireEvent.play(one)
  expect(stopSpeech).toHaveBeenCalledTimes(1)
  fireEvent.play(two)
  expect(pause.mock.contexts).toEqual([one])
  expect(stopSpeech).toHaveBeenCalledTimes(2)
  vi.spyOn(speechPlayer, 'getSnapshot').mockReturnValue({
    ...speechPlayer.getSnapshot(),
    status: 'loading',
  })
  act(() => listeners.forEach((listener) => listener()))
  expect(pause.mock.contexts).toContain(two)
  pause.mockClear()
  unmount()
  expect(pause.mock.contexts).toEqual([one, two])
  expect(unsubscribe).toHaveBeenCalledTimes(2)
})
