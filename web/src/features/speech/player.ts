import { APIError, fetchAPI, responseAPIError } from '@/lib/api-client'
import i18n from '@/i18n'
import type { SpeechSettings } from '@/features/settings/types'
import { speechChunks } from './text'

export type SpeechStatus = 'idle' | 'loading' | 'playing' | 'paused' | 'stopped' | 'ended' | 'error' | 'blocked' | 'empty'
export interface SpeechState {
  status: SpeechStatus
  owner: string
  turnId: string
  segment: number
  total: number
  queued: number
  error: string
  rate: number
  volume: number
}
interface SpeechTask {
  owner: string
  turnId: string
  text: string
  preview?: SpeechSettings
  chunks: string[]
  urls: Map<number, string>
}

export function speechConfigError(settings?: SpeechSettings): string {
  if (!settings?.endpoint.trim() || !settings.model.trim() || !settings.voice.trim()) return 'speech.error.unconfigured'
  try {
    const url = new URL(settings.endpoint.trim())
    if (!['http:', 'https:'].includes(url.protocol) || !url.hostname || url.username || url.password || url.hash) return 'speech.error.url'
  } catch { return 'speech.error.url' }
  return ''
}

function preference(key: string, fallback: number, min: number, max: number): number {
  try {
    const raw = localStorage.getItem(key)
    const value = raw === null ? fallback : Number(raw)
    return Number.isFinite(value) ? Math.min(max, Math.max(min, value)) : fallback
  } catch { return fallback }
}

/** One player per page, shared by Game and the settings preview. Only the
 * current task retains audio; replacement/invalidation releases every URL. */
export class SpeechPlayer {
  private state: SpeechState = { status: 'idle', owner: '', turnId: '', segment: 0, total: 0, queued: 0, error: '', rate: preference('denova:speech-rate', 1, 0.5, 2), volume: preference('denova:speech-volume', 1, 0, 1) }
  private listeners = new Set<() => void>()
  private audio?: HTMLAudioElement
  private task?: SpeechTask
  private queue: SpeechTask[] = []
  private abort?: AbortController
  private requestID?: string
  private epoch = 0
  private index = 0
  private paused = false

  getSnapshot = () => this.state
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener) } }
  private publish(change: Partial<SpeechState>) {
    this.state = { ...this.state, ...change, queued: this.queue.length }
    this.listeners.forEach(listener => listener())
  }
  private release(task?: SpeechTask) { task?.urls.forEach(url => URL.revokeObjectURL(url)) }
  private halt() {
    this.epoch++
    this.abort?.abort()
    this.abort = undefined
    if (this.requestID) {
      void fetchAPI(`/api/speech/${this.requestID}`, { method: 'DELETE', keepalive: true, suppressBackendUnavailableToast: true }).catch(() => {
        console.warn('[speech] Could not deliver synthesis cancellation')
      })
      this.requestID = undefined
    }
    if (this.audio) { this.audio.onended = null; this.audio.onerror = null; this.audio.pause(); this.audio.removeAttribute('src'); this.audio.load() }
  }
  clear = () => {
    this.halt()
    this.release(this.task)
    this.queue.forEach(task => this.release(task))
    this.task = undefined
    this.queue = []
    this.paused = false
    this.publish({ status: 'idle', owner: '', turnId: '', segment: 0, total: 0, error: '' })
  }
  stop = () => {
    this.halt()
    this.queue.forEach(task => this.release(task))
    this.queue = []
    this.paused = false
    this.publish({ status: 'stopped', error: '' })
  }
  stopOwner(owner: string) { if (this.state.owner === owner) this.clear() }

  read(input: { owner: string; turnId: string; text: string; settings?: SpeechSettings; preview?: boolean; enqueue?: boolean }) {
    // Empty automatic output must not interrupt an already playing turn.
    if (!input.text.trim() && input.enqueue && this.task) return
    if (!input.enqueue && this.task?.owner === input.owner && this.task.turnId === input.turnId && this.task.text === input.text) { this.replay(); return }
    if (!input.enqueue) this.clear()
    const error = speechConfigError(input.settings)
    if (!input.text.trim() || error) {
      if (!this.task) this.publish({ owner: input.owner, turnId: input.turnId, status: input.text.trim() ? 'error' : 'empty', error })
      return
    }
    const task: SpeechTask = { ...input, preview: input.preview ? input.settings : undefined, chunks: speechChunks(input.text), urls: new Map() }
    if (this.task && !['idle', 'ended', 'stopped', 'empty'].includes(this.state.status)) {
      this.queue.push(task)
      this.publish({})
      return
    }
    this.begin(task)
  }
  private begin(task: SpeechTask) {
    this.halt()
    this.release(this.task)
    this.task = task
    this.index = 0
    this.paused = false
    this.publish({ owner: task.owner, turnId: task.turnId, segment: 1, total: task.chunks.length, error: '' })
    void this.loadSegment()
  }
  private async loadSegment() {
    const task = this.task
    if (!task) return
    const epoch = ++this.epoch
    const index = this.index
    const abort = new AbortController()
    this.abort = abort
    this.publish({ status: this.paused ? 'paused' : 'loading', segment: index + 1, error: '' })
    try {
      let url = task.urls.get(index)
      if (!url) {
        this.requestID = crypto.randomUUID()
        const response = await fetchAPI('/api/speech', { method: 'POST', signal: abort.signal, suppressBackendUnavailableToast: true,
          headers: { 'Content-Type': 'application/json', 'X-Denova-Locale': i18n.language },
          body: JSON.stringify({ id: this.requestID, input: task.chunks[index], ...(task.preview ? { preview: task.preview } : {}) }) })
        if (!response.ok) throw await responseAPIError(response)
        const blob = await response.blob()
        if (epoch !== this.epoch) return
        if (!blob.size) throw new Error('speech.error.audio')
        url = URL.createObjectURL(blob)
        task.urls.set(index, url)
      }
      if (epoch !== this.epoch) return
      this.abort = undefined
      const audio = this.audio ||= new Audio()
      audio.src = url
      audio.playbackRate = this.state.rate
      audio.preservesPitch = true
      audio.volume = this.state.volume
      audio.onended = () => {
        if (epoch !== this.epoch) return
        if (this.index + 1 < task.chunks.length) { this.index++; void this.loadSegment() }
        else if (this.queue.length) this.begin(this.queue.shift()!)
        else this.publish({ status: 'ended' })
      }
      audio.onerror = () => {
        if (epoch !== this.epoch) return
        const badURL = task.urls.get(index)
        if (badURL) URL.revokeObjectURL(badURL)
        task.urls.delete(index)
        this.publish({ status: 'error', error: 'speech.error.audio' })
      }
      if (!this.paused) await this.play(epoch)
    } catch (error) {
      if (epoch !== this.epoch || abort.signal.aborted) return
      const code = error instanceof APIError ? error.code : error instanceof Error ? error.message : ''
      const key = code?.startsWith('speech.error.') ? code : 'speech.error.network'
      console.warn('[speech] Synthesis failed', { reason: key, segment: index + 1 })
      this.publish({ status: 'error', error: key })
    } finally {
      if (epoch === this.epoch) { this.abort = undefined; this.requestID = undefined }
    }
  }
  private async play(epoch = this.epoch) {
    try {
      await this.audio?.play()
      if (epoch === this.epoch) {
        if (this.paused) this.audio?.pause()
        else this.publish({ status: 'playing', error: '' })
      }
    } catch (error) {
      if (epoch !== this.epoch) return
      const blocked = (error instanceof Error || error instanceof DOMException) && error.name === 'NotAllowedError'
      this.publish({ status: blocked ? 'blocked' : 'error', error: blocked ? '' : 'speech.error.audio' })
    }
  }
  pause = () => { this.paused = true; this.audio?.pause(); this.publish({ status: 'paused' }) }
  resume = () => {
    this.paused = false
    if (this.abort) this.publish({ status: 'loading' })
    else if (this.state.status === 'error' || !this.audio?.getAttribute('src')) void this.loadSegment()
    else void this.play()
  }
  replay = () => {
    if (!this.task) return
    this.halt()
    this.queue.forEach(task => this.release(task))
    this.queue = []
    this.paused = false
    this.index = 0
    void this.loadSegment()
  }
  setRate = (rate: number) => this.setPreference('rate', Math.min(2, Math.max(0.5, rate)))
  setVolume = (volume: number) => this.setPreference('volume', Math.min(1, Math.max(0, volume)))
  private setPreference(key: 'rate' | 'volume', value: number) {
    if (this.audio) { if (key === 'rate') this.audio.playbackRate = value; else this.audio.volume = value }
    try { localStorage.setItem(`denova:speech-${key}`, String(value)) } catch { /* Playback works without browser storage. */ }
    this.publish({ [key]: value })
  }
}

export const speechPlayer = new SpeechPlayer()
if (typeof window !== 'undefined') window.addEventListener('pagehide', () => speechPlayer.clear())
