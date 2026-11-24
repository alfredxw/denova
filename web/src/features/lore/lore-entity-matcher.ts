import { createContext, useContext } from 'react'
import type { LoreItem } from '@/lib/api'

export const LORE_LENS_ATTR = 'data-lore-lens-id'
export const LORE_LENS_CLASS = 'nova-lore-lens-mention'

export interface LoreEntityMatch {
  from: number
  to: number
  itemId: string
  matchedTerm: string
}

export interface LoreEntityMatcher {
  readonly signature: string
  readonly hasTerms: boolean
  readonly itemById: ReadonlyMap<string, LoreItem>
  findMatches(text: string, seenItemIds?: Set<string>): LoreEntityMatch[]
}

interface TrieTerminal {
  itemId: string
  priority: number
}

interface TrieNode {
  children: Map<number, TrieNode>
  terminal?: TrieTerminal
}

const ASCII_WORD_ONLY = /^[A-Za-z0-9_]+$/
const IMPORTANCE_RANK: Record<string, number> = {
  major: 0,
  important: 1,
  minor: 2,
}

export const LoreLensContext = createContext<LoreEntityMatcher | null>(null)

export function useLoreLensMatcher(): LoreEntityMatcher | null {
  return useContext(LoreLensContext)
}

/**
 * Returns whether a candidate Lore name or keyword is specific enough to
 * decorate in body prose without causing single-character or stopword noise.
 */
export function isMatchableLoreTerm(raw: string): boolean {
  const term = raw.trim()
  if (!term || /[\r\n]/.test(term) || term.includes('[[') || term.includes(']]')) return false
  if (ASCII_WORD_ONLY.test(term)) return term.length >= 3
  return Array.from(term).length >= 2
}

/**
 * Checks whether a writing selection is a clean candidate for "Save as Lore".
 */
export function canQuickCreateLoreName(raw: string, items: readonly LoreItem[]): boolean {
  const term = raw.trim()
  if (!term || /[\r\n]/.test(term) || term.includes('[[') || term.includes(']]')) return false
  const length = Array.from(term).length
  if (length < 2 || length > 24) return false
  if (!/[\p{L}\p{Nd}]/u.test(term)) return false
  const lower = term.toLowerCase()
  return !items.some((item) => item.name.trim().toLowerCase() === lower)
}

/**
 * Builds a single-pass, longest-match-first entity matcher over enabled Lore
 * items. The matcher never mutates source prose and keeps UTF-16 offsets exact.
 */
export function createLoreEntityMatcher(
  items: readonly LoreItem[],
  options: { enabled: boolean; matchKeywords: boolean },
): LoreEntityMatcher | null {
  if (!options.enabled || items.length === 0) return null

  const root: TrieNode = { children: new Map() }
  const itemById = new Map<string, LoreItem>()
  const signatureParts: string[] = [options.matchKeywords ? 'kw1' : 'kw0']
  let termCount = 0

  items.forEach((item, index) => {
    if (!item.enabled) return
    itemById.set(item.id, item)
    const importance = IMPORTANCE_RANK[item.importance] ?? 3

    const insertTerm = (rawTerm: string, isKeyword: boolean) => {
      if (!isMatchableLoreTerm(rawTerm)) return
      const term = rawTerm.trim()
      // Exact priority order: explicit name before keyword, then importance, then order.
      const priority = (isKeyword ? 1_000_000 : 0) + importance * 100_000 + index
      let node = root
      for (let i = 0; i < term.length; i += 1) {
        const code = foldCharCode(term.charCodeAt(i))
        let next = node.children.get(code)
        if (!next) {
          next = { children: new Map() }
          node.children.set(code, next)
        }
        node = next
      }
      if (!node.terminal || priority < node.terminal.priority) {
        node.terminal = { itemId: item.id, priority }
      }
      termCount += 1
      signatureParts.push(`${item.id}:${isKeyword ? 'k' : 'n'}:${term}`)
    }

    insertTerm(item.name, false)
    if (options.matchKeywords && Array.isArray(item.keywords)) {
      for (const keyword of item.keywords) {
        insertTerm(keyword, true)
      }
    }
  })

  if (termCount === 0) return null

  const signature = signatureParts.join('\u0001')

  return {
    signature,
    hasTerms: true,
    itemById,
    findMatches(text: string, seenItemIds?: Set<string>): LoreEntityMatch[] {
      if (!text) return []
      const matches: LoreEntityMatch[] = []
      const length = text.length
      let index = 0

      while (index < length) {
        let node = root.children.get(foldCharCode(text.charCodeAt(index)))
        if (!node) {
          index += 1
          continue
        }

        let bestEnd = -1
        let bestItemId = ''
        let cursor = index + 1

        while (node) {
          if (node.terminal && hasValidBoundary(text, index, cursor)) {
            bestEnd = cursor
            bestItemId = node.terminal.itemId
          }
          if (cursor >= length) break
          node = node.children.get(foldCharCode(text.charCodeAt(cursor)))
          cursor += 1
        }

        if (bestEnd > index) {
          // Consume the longest match even if already seen in this block so a
          // shorter prefix Lore entry cannot falsely match inside it.
          if (!seenItemIds || !seenItemIds.has(bestItemId)) {
            seenItemIds?.add(bestItemId)
            matches.push({
              from: index,
              to: bestEnd,
              itemId: bestItemId,
              matchedTerm: text.slice(index, bestEnd),
            })
          }
          index = bestEnd
        } else {
          index += 1
        }
      }

      return matches
    },
  }
}

/**
 * Returns a clean plain-text preview for the hover card: brief description if
 * available, otherwise the first ~140 characters of stripped Markdown body.
 */
export function loreLensExcerpt(item: LoreItem): string {
  const brief = item.brief_description.trim()
  if (brief) return brief
  const raw = item.content.trim()
  if (!raw) return ''
  const plain = raw
    .replace(/```[\s\S]*?```/g, ' ')
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
    .replace(/\[\[([^\]]+)\]\]/g, '$1')
    .replace(/^#{1,6}\s+/gm, '')
    .replace(/^>\s?/gm, '')
    .replace(/[*_`~]/g, '')
    .replace(/\s+/g, ' ')
    .trim()
  if (plain.length <= 140) return plain
  return `${plain.slice(0, 140).trimEnd()}…`
}

function foldCharCode(code: number): number {
  return code >= 65 && code <= 90 ? code + 32 : code
}

function isAsciiWordCode(code: number): boolean {
  return (
    (code >= 48 && code <= 57)
    || (code >= 65 && code <= 90)
    || (code >= 97 && code <= 122)
    || code === 95
  )
}

function hasValidBoundary(text: string, from: number, to: number): boolean {
  if (isAsciiWordCode(text.charCodeAt(from)) && from > 0 && isAsciiWordCode(text.charCodeAt(from - 1))) {
    return false
  }
  if (isAsciiWordCode(text.charCodeAt(to - 1)) && to < text.length && isAsciiWordCode(text.charCodeAt(to))) {
    return false
  }
  return true
}
