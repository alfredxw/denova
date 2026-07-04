import { describe, expect, it } from 'vitest'
import { decodeTextBytes } from './text-file'

describe('decodeTextBytes', () => {
  it('decodes GB18030 Chinese text', () => {
    const gb18030 = new Uint8Array([
      0xb5, 0xda, 0xd2, 0xbb, 0xd5, 0xc2, 0x20, 0xc6, 0xf0, 0xb7, 0xc9,
      0x0a,
      0xc4, 0xda, 0xc8, 0xdd,
    ])

    expect(decodeTextBytes(gb18030)).toBe('第一章 起飞\n内容')
  })

  it('decodes UTF-16LE text with BOM', () => {
    const utf16le = new Uint8Array([0xff, 0xfe, 0x2d, 0x4e, 0x87, 0x65])

    expect(decodeTextBytes(utf16le)).toBe('中文')
  })
})
