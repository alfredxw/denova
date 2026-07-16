import { describe, expect, it } from 'vitest'
import { Utf8OffsetIndex } from './utf8-offset-index'

describe('Utf8OffsetIndex', () => {
  it('maps Chinese, emoji, CRLF and trailing lines between UTF-8 bytes and Monaco positions', () => {
    const index = new Utf8OffsetIndex('A中😀\r\n尾\n')

    expect(index.byteLength).toBe(14)
    expect(index.lineCount).toBe(3)
    expect(index.positionAtByteOffset(0)).toEqual({ lineNumber: 1, column: 1 })
    expect(index.positionAtByteOffset(1)).toEqual({ lineNumber: 1, column: 2 })
    expect(index.positionAtByteOffset(4)).toEqual({ lineNumber: 1, column: 3 })
    expect(index.positionAtByteOffset(8)).toEqual({ lineNumber: 1, column: 5 })
    expect(index.positionAtByteOffset(9)).toEqual({ lineNumber: 1, column: 5 })
    expect(index.positionAtByteOffset(10)).toEqual({ lineNumber: 2, column: 1 })
    expect(index.positionAtByteOffset(14)).toEqual({ lineNumber: 3, column: 1 })

    expect(index.byteOffsetAtPosition({ lineNumber: 1, column: 2 })).toBe(1)
    expect(index.byteOffsetAtPosition({ lineNumber: 1, column: 3 })).toBe(4)
    expect(index.byteOffsetAtPosition({ lineNumber: 1, column: 4 })).toBe(4)
    expect(index.byteOffsetAtPosition({ lineNumber: 1, column: 5 })).toBe(8)
    expect(index.byteOffsetAtPosition({ lineNumber: 2, column: 1 })).toBe(10)
    expect(index.byteOffsetAtPosition({ lineNumber: 3, column: 1 })).toBe(14)
  })

  it('never splits a multibyte code point and slices authoritative byte ranges', () => {
    const index = new Utf8OffsetIndex('前😀后')

    expect(index.positionAtByteOffset(1)).toEqual({ lineNumber: 1, column: 1 })
    expect(index.positionAtByteOffset(5)).toEqual({ lineNumber: 1, column: 2 })
    expect(index.sliceBytes(0, 3)).toBe('前')
    expect(index.sliceBytes(3, 7)).toBe('😀')
    expect(index.sliceBytes(7, 10)).toBe('后')
    expect(index.byteOffsetAtUtf16Offset(2)).toBe(3)
    expect(index.utf16OffsetAtByteOffset(6)).toBe(1)
  })

  it('clamps empty and out-of-range inputs', () => {
    const index = new Utf8OffsetIndex('')
    expect(index.byteLength).toBe(0)
    expect(index.lineCount).toBe(1)
    expect(index.positionAtByteOffset(100)).toEqual({ lineNumber: 1, column: 1 })
    expect(index.byteOffsetAtPosition({ lineNumber: 99, column: 99 })).toBe(0)
    expect(index.sliceBytes(-10, 100)).toBe('')
  })
})
