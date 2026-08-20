import { afterEach, describe, expect, it, vi } from 'vitest'
import { queryLocalFontFamilies } from './local-fonts'

afterEach(() => {
  Reflect.deleteProperty(window, 'queryLocalFonts')
})

describe('local fonts', () => {
  it('reports unsupported browsers without prompting', async () => {
    await expect(queryLocalFontFamilies()).resolves.toEqual({ status: 'unsupported', families: [] })
  })

  it('keeps valid unique families when individual results are malformed', async () => {
    Object.defineProperty(window, 'queryLocalFonts', {
      configurable: true,
      value: vi.fn().mockResolvedValue([
        { family: '霞鹜文楷', fullName: '霞鹜文楷 Regular', postscriptName: 'LXGWWenKai-Regular', style: 'Regular' },
        { family: '霞鹜文楷', fullName: '霞鹜文楷 Bold', postscriptName: 'LXGWWenKai-Bold', style: 'Bold' },
        { family: 'Inter', fullName: 'Inter Regular', postscriptName: 'Inter-Regular', style: 'Regular' },
        { family: 'Broken\nFont', fullName: '', postscriptName: '', style: '' },
        { family: 42 },
      ]),
    })

    const result = await queryLocalFontFamilies()
    expect(result.status).toBe('ready')
    expect(result.families).toHaveLength(2)
    expect(result.families).toEqual(expect.arrayContaining(['Inter', '霞鹜文楷']))
  })

  it('returns a recoverable denied state', async () => {
    Object.defineProperty(window, 'queryLocalFonts', {
      configurable: true,
      value: vi.fn().mockRejectedValue(new DOMException('Denied', 'NotAllowedError')),
    })

    await expect(queryLocalFontFamilies()).resolves.toEqual({ status: 'denied', families: [] })
  })
})
