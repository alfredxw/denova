const TEXT_DECODER_LABELS = ['utf-8', 'gb18030', 'gbk', 'utf-16le', 'utf-16be'] as const

export async function readTextFile(file: File): Promise<string> {
  return decodeTextBytes(new Uint8Array(await file.arrayBuffer()))
}

export function decodeTextBytes(bytes: Uint8Array): string {
  if (bytes.length >= 3 && bytes[0] === 0xef && bytes[1] === 0xbb && bytes[2] === 0xbf) {
    return new TextDecoder('utf-8', { fatal: true }).decode(bytes.subarray(3))
  }
  if (bytes.length >= 2 && bytes[0] === 0xff && bytes[1] === 0xfe) {
    return stripBom(new TextDecoder('utf-16le', { fatal: true }).decode(bytes))
  }
  if (bytes.length >= 2 && bytes[0] === 0xfe && bytes[1] === 0xff) {
    return stripBom(new TextDecoder('utf-16be', { fatal: true }).decode(bytes))
  }

  const candidates: Array<{ text: string; score: number }> = []
  for (const label of TEXT_DECODER_LABELS) {
    try {
      const text = stripBom(new TextDecoder(label, { fatal: true }).decode(bytes))
      candidates.push({ text, score: decodedTextPenalty(text) })
      if (label === 'utf-8' && decodedTextPenalty(text) === 0) return text
    } catch {
      // Try the next common text encoding.
    }
  }
  if (candidates.length === 0) {
    throw new Error('Unsupported text encoding')
  }
  candidates.sort((a, b) => a.score - b.score)
  return candidates[0].text
}

function stripBom(text: string) {
  return text.charCodeAt(0) === 0xfeff ? text.slice(1) : text
}

function decodedTextPenalty(text: string) {
  let score = 0
  for (const char of text) {
    const code = char.charCodeAt(0)
    if (char === '\uFFFD') score += 100
    else if (char !== '\n' && char !== '\r' && char !== '\t' && code < 0x20) score += 20
  }
  return score
}
