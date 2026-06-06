import { describe, expect, it } from 'vitest'
import zhCN from './locales/zh-CN'
import enUS from './locales/en-US'
import { resolveLocale } from './index'

describe('i18n', () => {
  it('resolves auto from browser language', () => {
    expect(resolveLocale('auto', 'zh-Hans')).toBe('zh-CN')
    expect(resolveLocale('auto', 'en-GB')).toBe('en-US')
  })

  it('keeps locale resource keys aligned', () => {
    expect(Object.keys(enUS).sort()).toEqual(Object.keys(zhCN).sort())
  })
})
