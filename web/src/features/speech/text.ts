import { fromMarkdown } from 'mdast-util-from-markdown'

export interface SpeechContentSettings {
  mode: 'all' | 'quoted'
  ignore_asterisks: boolean
}

interface MarkdownNode {
  type: string
  value?: string
  children?: MarkdownNode[]
  position?: { start: { offset?: number } }
}

/** Read visible prose, never media, code, or Markdown syntax. Filtering is
 * deterministic and deliberately independent of speaker identification. */
export function speechText(markdown: string, settings: SpeechContentSettings): string {
  // Inline HTML is tokenized as separate opening/text/closing nodes. Remove
  // complete non-prose containers before walking, including fallback text.
  markdown = markdown.replace(/<(audio|video|script|style|pre|code)\b[^>]*>[\s\S]*?<\/\1\s*>/gi, '')
  // Roleplay actions can include padded delimiters. Strip these before parsing:
  // CommonMark otherwise interprets a leading "* " as a list marker.
  if (settings.ignore_asterisks) markdown = markdown.replace(/(?<![\\*])\*(?!\*)[^*]+(?<!\\)\*(?!\*)/g, '')
  function plain(node: MarkdownNode): string {
    switch (node.type) {
      case 'code': case 'inlineCode': case 'image': case 'imageReference': case 'definition': return ''
      case 'html': {
        const document = new DOMParser().parseFromString(node.value || '', 'text/html')
        document.querySelectorAll('audio,video,img,script,style,pre,code').forEach(element => element.remove())
        return document.body.textContent || ''
      }
      case 'emphasis':
        if (settings.ignore_asterisks && markdown[node.position?.start.offset ?? -1] === '*') return ''
        break
      case 'break': return '\n'
      case 'text': return node.value || ''
    }
    const text = (node.children || []).map(plain).join('')
    return text && ['paragraph', 'heading', 'listItem', 'blockquote'].includes(node.type) ? `${text}\n` : text
  }
  const text = plain(fromMarkdown(markdown))
    .replace(/(?:https?:\/\/|file:\/\/|data:|www\.)[^\s<>「」『』“”"]+/gi, '')
    .replace(/(?:[A-Za-z]:[\\/]|(?:\.\.?[\\/]|\/))[\w./\\%-]+\.(?:png|jpe?g|gif|webp|svg|mp3|wav|ogg|mp4|webm)(?:\?\S*)?/gi, '')
  return (settings.mode === 'quoted' ? quotedText(text) : text).replace(/[ \t]+/g, ' ').replace(/\n{3,}/g, '\n\n').trim()
}

function quotedText(text: string): string {
  const pairs: Record<string, string> = { '“': '”', '"': '"', '「': '」', '『': '』' }
  const stack: string[] = []
  const segments: string[] = []
  let current = ''
  for (const char of text) {
    if (stack.length && char === stack.at(-1)) {
      stack.pop()
      if (!stack.length) { segments.push(current); current = '' }
    } else if (pairs[char]) {
      stack.push(pairs[char])
    } else if (stack.length) current += char
  }
  return segments.filter(segment => segment.trim()).join('\n\n')
}

/** Each non-whitespace character appears exactly once; prefer paragraph and
 * sentence boundaries and never split a Unicode surrogate pair. */
export function speechChunks(text: string, limit = 1000): string[] {
  const chars = Array.from(text)
  const chunks: string[] = []
  for (let start = 0; start < chars.length;) {
    let end = Math.min(start + limit, chars.length)
    if (end < chars.length) {
      for (let i = end - 1; i >= start + limit / 2; i--) {
        if (/[\n。！？.!?;；]/u.test(chars[i])) { end = i + 1; break }
      }
    }
    const chunk = chars.slice(start, end).join('').trim()
    if (chunk) chunks.push(chunk)
    start = end
  }
  return chunks
}
