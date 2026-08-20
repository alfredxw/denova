import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  SETTINGS_SECTION_SCROLL_DURATION_SECONDS,
  scrollSettingsSectionIntoView,
} from './settings-section-scroll'

const { animateMock, stopMock } = vi.hoisted(() => ({
  animateMock: vi.fn(),
  stopMock: vi.fn(),
}))

vi.mock('motion/react', () => ({
  animate: animateMock,
}))

describe('scrollSettingsSectionIntoView', () => {
  beforeEach(() => {
    animateMock.mockReset()
    stopMock.mockReset()
    animateMock.mockReturnValue({ stop: stopMock })
  })

  it('uses the same short duration for nearby and distant sections', () => {
    const container = createScrollContainer()
    const nearSection = createSection(250)
    const distantSection = createSection(1_250)

    scrollSettingsSectionIntoView(container, nearSection, { reducedMotion: false })
    scrollSettingsSectionIntoView(container, distantSection, { reducedMotion: false })

    expect(animateMock).toHaveBeenCalledTimes(2)
    expect(animateMock.mock.calls[0][2]).toMatchObject({
      type: 'tween',
      duration: SETTINGS_SECTION_SCROLL_DURATION_SECONDS,
    })
    expect(animateMock.mock.calls[1][2]).toMatchObject({
      type: 'tween',
      duration: SETTINGS_SECTION_SCROLL_DURATION_SECONDS,
    })
    expect(animateMock.mock.calls[0][1]).toBe(284)
    expect(animateMock.mock.calls[1][1]).toBe(1_284)
  })

  it('jumps immediately when motion is reduced', () => {
    const container = createScrollContainer()
    const onComplete = vi.fn()

    const animation = scrollSettingsSectionIntoView(container, createSection(1_250), {
      reducedMotion: true,
      onComplete,
    })

    expect(animation).toBeNull()
    expect(animateMock).not.toHaveBeenCalled()
    expect(container.scrollTop).toBe(1_284)
    expect(onComplete).toHaveBeenCalledOnce()
  })
})

function createScrollContainer() {
  const container = document.createElement('div')
  Object.defineProperties(container, {
    scrollTop: { configurable: true, writable: true, value: 100 },
    scrollHeight: { configurable: true, value: 2_000 },
    clientHeight: { configurable: true, value: 500 },
  })
  vi.spyOn(container, 'getBoundingClientRect').mockReturnValue(rectAt(50))
  return container
}

function createSection(top: number) {
  const section = document.createElement('section')
  section.style.scrollMarginTop = '16px'
  vi.spyOn(section, 'getBoundingClientRect').mockReturnValue(rectAt(top))
  return section
}

function rectAt(top: number): DOMRect {
  return {
    x: 0,
    y: top,
    top,
    right: 0,
    bottom: top,
    left: 0,
    width: 0,
    height: 0,
    toJSON: () => ({}),
  }
}
