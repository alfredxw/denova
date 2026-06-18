import { beforeEach, describe, expect, it } from 'vitest'
import { AUTO_UPDATE_CHECK_INTERVAL_MS, markAutoUpdateChecked, shouldRunAutoUpdateCheck } from './update-check-cache'

describe('update check cache', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('allows the first automatic update check', () => {
    expect(shouldRunAutoUpdateCheck(1000)).toBe(true)
  })

  it('skips automatic update checks inside one hour', () => {
    markAutoUpdateChecked(1000)

    expect(shouldRunAutoUpdateCheck(1000 + AUTO_UPDATE_CHECK_INTERVAL_MS - 1)).toBe(false)
  })

  it('allows automatic update checks after one hour', () => {
    markAutoUpdateChecked(1000)

    expect(shouldRunAutoUpdateCheck(1000 + AUTO_UPDATE_CHECK_INTERVAL_MS)).toBe(true)
  })
})
