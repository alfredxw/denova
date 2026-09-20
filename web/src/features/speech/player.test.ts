import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { SpeechPlayer } from './player'

const settings = { endpoint: 'http://localhost/speech', api_key: '', model: 'custom', voice: 'voice' }
class MockAudio {
  static current: MockAudio
  src = ''
  playbackRate = 1
  preservesPitch = true
  volume = 1
  onended: (() => void) | null = null
  onerror: (() => void) | null = null
  play = vi.fn().mockResolvedValue(undefined)
  pause = vi.fn()
  load = vi.fn()
  constructor() { MockAudio.current = this }
  removeAttribute() { this.src = '' }
  getAttribute() { return this.src }
}
// Response uses Node's Fetch implementation; a jsdom Blob is not a compatible
// body on every supported Node version. Let Response construct its own Blob.
const audioResponse = () => new Response('ID3audio', { headers: { 'Content-Type': 'audio/mpeg' } })
let player: SpeechPlayer
let fetchMock: ReturnType<typeof vi.fn>
const flush = async () => { await vi.waitFor(() => expect(player.getSnapshot().status).not.toBe('loading')) }

beforeEach(() => {
  localStorage.clear()
  fetchMock = vi.fn().mockImplementation(async () => audioResponse())
  vi.stubGlobal('fetch', fetchMock)
  vi.stubGlobal('Audio', MockAudio)
  vi.stubGlobal('URL', class extends URL { static createObjectURL = vi.fn(() => 'blob:audio'); static revokeObjectURL = vi.fn() })
  player = new SpeechPlayer()
})
afterEach(() => { player.clear(); vi.unstubAllGlobals() })
const read = (text = '你好。', extra = {}) => player.read({ owner: 'story', turnId: 'one', text, settings, ...extra })

describe('single speech playback queue', () => {
  it('pauses, changes rate/volume and replays cached audio without another request', async () => {
    read()
    await flush()
    expect(player.getSnapshot().status).toBe('playing')
    player.pause()
    player.setRate(1.5)
    player.setVolume(0.25)
    player.resume()
    await flush()
    expect(MockAudio.current.playbackRate).toBe(1.5)
    expect(MockAudio.current.volume).toBe(0.25)
    expect(MockAudio.current.preservesPitch).toBe(true)
    MockAudio.current.onended?.()
    expect(player.getSnapshot().status).toBe('ended')
    player.replay()
    await flush()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
  it('aborts pending work and discards a late response after stop', async () => {
    let resolve!: (response: Response) => void
    fetchMock.mockImplementation(() => new Promise<Response>(done => { resolve = done }))
    read()
    const signal = fetchMock.mock.calls[0][1].signal as AbortSignal
    const finishSpeech = resolve
    player.stop()
    expect(signal.aborted).toBe(true)
    expect(fetchMock.mock.calls[1][1].method).toBe('DELETE')
    finishSpeech(audioResponse())
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(player.getSnapshot().status).toBe('stopped')
    expect(URL.createObjectURL).not.toHaveBeenCalled()
  })
  it('queues automatic turns without unpausing, and manual preview replaces everything', async () => {
    read()
    await flush()
    player.pause()
    read('下一回合', { turnId: 'two', enqueue: true })
    expect(player.getSnapshot()).toMatchObject({ status: 'paused', queued: 1, turnId: 'one' })
    expect(fetchMock).toHaveBeenCalledTimes(1)
    read('preview', { owner: 'preview', preview: true })
    await flush()
    expect(player.getSnapshot()).toMatchObject({ status: 'playing', queued: 0, owner: 'preview' })
    expect(JSON.parse(fetchMock.mock.calls[1][1].body).preview).toEqual(settings)
  })
  it('retries the failed segment without replaying successful segments', async () => {
    fetchMock.mockResolvedValueOnce(audioResponse()).mockResolvedValueOnce(new Response(JSON.stringify({ code: 'speech.error.rateLimit' }), { status: 429 }))
    read('甲'.repeat(1000) + '乙'.repeat(1000) + '丙')
    await flush()
    MockAudio.current.onended?.()
    await flush()
    expect(player.getSnapshot()).toMatchObject({ status: 'error', segment: 2, error: 'speech.error.rateLimit' })
    player.resume()
    await flush()
    expect(player.getSnapshot()).toMatchObject({ status: 'playing', segment: 2 })
    MockAudio.current.onended?.()
    await flush()
    const texts = fetchMock.mock.calls.map(call => JSON.parse(call[1].body).input)
    expect(texts).toEqual(['甲'.repeat(1000), '乙'.repeat(1000), '乙'.repeat(1000), '丙'])
  })
  it('does not request empty text or incomplete settings', () => {
    read('')
    expect(player.getSnapshot().status).toBe('empty')
    read('text', { settings: undefined })
    expect(player.getSnapshot().error).toBe('speech.error.unconfigured')
    expect(fetchMock).not.toHaveBeenCalled()
  })
  it('resumes browser-blocked audio from its cached blob', async () => {
    read()
    await flush()
    MockAudio.current.play.mockRejectedValueOnce(new DOMException('blocked', 'NotAllowedError'))
    player.replay()
    await flush()
    expect(player.getSnapshot().status).toBe('blocked')
    player.resume()
    await vi.waitFor(() => expect(player.getSnapshot().status).toBe('playing'))
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
  it('does not start playing when paused during synthesis', async () => {
    let resolve!: (response: Response) => void
    fetchMock.mockImplementation(() => new Promise<Response>(done => { resolve = done }))
    read()
    player.pause()
    resolve(audioResponse())
    await vi.waitFor(() => expect(URL.createObjectURL).toHaveBeenCalled())
    expect(MockAudio.current.play).not.toHaveBeenCalled()
    player.resume()
    await vi.waitFor(() => expect(player.getSnapshot().status).toBe('playing'))
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
})
