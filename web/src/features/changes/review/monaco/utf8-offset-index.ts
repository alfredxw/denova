export interface MonacoTextPosition {
  lineNumber: number
  column: number
}

interface IndexedLine {
  utf16Start: number
  utf16End: number
  utf16AfterEOL: number
  byteStart: number
  byteEnd: number
  byteAfterEOL: number
}

/**
 * Maps the UTF-8 byte offsets persisted by the workspace change ledger to the
 * UTF-16 line/column coordinates used by Monaco. Lines are indexed once while
 * conversion within the selected line stays allocation-free.
 */
export class Utf8OffsetIndex {
  readonly text: string
  readonly byteLength: number
  readonly lineCount: number
  private readonly lines: IndexedLine[]

  constructor(text: string) {
    this.text = text
    this.lines = indexLines(text)
    const lastLine = this.lines[this.lines.length - 1]
    this.byteLength = lastLine.byteAfterEOL
    this.lineCount = this.lines.length
  }

  positionAtByteOffset(offset: number): MonacoTextPosition {
    const target = clampInteger(offset, 0, this.byteLength)
    const lineIndex = findLineByByteOffset(this.lines, target)
    const line = this.lines[lineIndex]
    const contentTarget = Math.min(target, line.byteEnd)
    return {
      lineNumber: lineIndex + 1,
      column: utf16UnitsForBytes(this.text, line.utf16Start, line.utf16End, contentTarget - line.byteStart) + 1,
    }
  }

  byteOffsetAtPosition(position: MonacoTextPosition): number {
    const lineIndex = clampInteger(position.lineNumber, 1, this.lines.length) - 1
    const line = this.lines[lineIndex]
    const requestedUnits = clampInteger(position.column, 1, line.utf16End - line.utf16Start + 1) - 1
    return line.byteStart + utf8BytesForUtf16Units(this.text, line.utf16Start, line.utf16End, requestedUnits)
  }

  byteOffsetAtUtf16Offset(offset: number): number {
    const target = clampInteger(offset, 0, this.text.length)
    const lineIndex = findLineByUtf16Offset(this.lines, target)
    const line = this.lines[lineIndex]
    if (target > line.utf16End) {
      return line.byteEnd + Math.min(target - line.utf16End, line.byteAfterEOL - line.byteEnd)
    }
    return line.byteStart + utf8BytesForUtf16Units(this.text, line.utf16Start, line.utf16End, target - line.utf16Start)
  }

  utf16OffsetAtByteOffset(offset: number): number {
    const target = clampInteger(offset, 0, this.byteLength)
    const lineIndex = findLineByByteOffset(this.lines, target)
    const line = this.lines[lineIndex]
    if (target > line.byteEnd) {
      return line.utf16End + Math.min(target - line.byteEnd, line.utf16AfterEOL - line.utf16End)
    }
    return line.utf16Start + utf16UnitsForBytes(this.text, line.utf16Start, line.utf16End, target - line.byteStart)
  }

  sliceBytes(start: number, end: number): string {
    const normalizedStart = clampInteger(start, 0, this.byteLength)
    const normalizedEnd = clampInteger(end, normalizedStart, this.byteLength)
    return this.text.slice(
      this.utf16OffsetAtByteOffset(normalizedStart),
      this.utf16OffsetAtByteOffset(normalizedEnd),
    )
  }
}

function indexLines(text: string): IndexedLine[] {
  const result: IndexedLine[] = []
  let lineUtf16Start = 0
  let lineByteStart = 0
  let utf16Offset = 0
  let byteOffset = 0

  while (utf16Offset < text.length) {
    const codeUnit = text.charCodeAt(utf16Offset)
    if (codeUnit === 10 || codeUnit === 13) {
      const contentUtf16End = utf16Offset
      const contentByteEnd = byteOffset
      if (codeUnit === 13 && text.charCodeAt(utf16Offset + 1) === 10) {
        utf16Offset += 2
        byteOffset += 2
      } else {
        utf16Offset += 1
        byteOffset += 1
      }
      result.push({
        utf16Start: lineUtf16Start,
        utf16End: contentUtf16End,
        utf16AfterEOL: utf16Offset,
        byteStart: lineByteStart,
        byteEnd: contentByteEnd,
        byteAfterEOL: byteOffset,
      })
      lineUtf16Start = utf16Offset
      lineByteStart = byteOffset
      continue
    }
    const codePoint = text.codePointAt(utf16Offset) ?? codeUnit
    utf16Offset += codePoint > 0xffff ? 2 : 1
    byteOffset += utf8Width(codePoint)
  }

  result.push({
    utf16Start: lineUtf16Start,
    utf16End: text.length,
    utf16AfterEOL: text.length,
    byteStart: lineByteStart,
    byteEnd: byteOffset,
    byteAfterEOL: byteOffset,
  })
  return result
}

function utf16UnitsForBytes(text: string, start: number, end: number, requestedBytes: number): number {
  let utf16Offset = start
  let bytes = 0
  while (utf16Offset < end) {
    const codePoint = text.codePointAt(utf16Offset) ?? text.charCodeAt(utf16Offset)
    const width = utf8Width(codePoint)
    if (bytes + width > requestedBytes) break
    bytes += width
    utf16Offset += codePoint > 0xffff ? 2 : 1
  }
  return utf16Offset - start
}

function utf8BytesForUtf16Units(text: string, start: number, end: number, requestedUnits: number): number {
  let utf16Offset = start
  let bytes = 0
  const target = start + requestedUnits
  while (utf16Offset < end && utf16Offset < target) {
    const codePoint = text.codePointAt(utf16Offset) ?? text.charCodeAt(utf16Offset)
    const units = codePoint > 0xffff ? 2 : 1
    if (utf16Offset + units > target) break
    bytes += utf8Width(codePoint)
    utf16Offset += units
  }
  return bytes
}

function utf8Width(codePoint: number): number {
  if (codePoint <= 0x7f) return 1
  if (codePoint <= 0x7ff) return 2
  if (codePoint <= 0xffff) return 3
  return 4
}

function findLineByByteOffset(lines: IndexedLine[], offset: number): number {
  let low = 0
  let high = lines.length - 1
  while (low < high) {
    const middle = Math.ceil((low + high) / 2)
    if (lines[middle].byteStart <= offset) low = middle
    else high = middle - 1
  }
  return low
}

function findLineByUtf16Offset(lines: IndexedLine[], offset: number): number {
  let low = 0
  let high = lines.length - 1
  while (low < high) {
    const middle = Math.ceil((low + high) / 2)
    if (lines[middle].utf16Start <= offset) low = middle
    else high = middle - 1
  }
  return low
}

function clampInteger(value: number, minimum: number, maximum: number): number {
  if (!Number.isFinite(value)) return minimum
  return Math.min(maximum, Math.max(minimum, Math.trunc(value)))
}
