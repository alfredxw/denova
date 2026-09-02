import {
  DetailBlock,
  DetailPre,
  DetailStack,
  EmptyValue,
  fieldMeta,
  formatMaybeJSON,
  formatValue,
  MetaLine,
  numericMeta,
  parseRecord,
  recordArray,
  stringArray,
  stringValue,
  ToolResourceLink,
  type ToolDetailAdapter,
  type ToolDetailRenderer,
  type ToolDetailRenderProps,
} from './shared'
import { interactiveTurnToolDetailAdapters } from './interactive-turn'

export const domainToolDetailAdapters: Record<string, ToolDetailAdapter> = {
  list_lore_items: outputAdapter(renderListLoreInput, renderLoreOutput),
  read_lore_items: outputAdapter(renderReadLoreInput, renderLoreOutput),
  write_lore_items: inputAdapter(renderWriteLoreInput, renderWriteLoreOutput),
  search_story_history: outputAdapter(renderHistoryInput, renderHistoryOutput),
  ...interactiveTurnToolDetailAdapters,
}

function outputAdapter(renderInput: ToolDetailRenderer, renderOutput: ToolDetailRenderer): ToolDetailAdapter {
  return { layout: 'output', renderInput, renderOutput }
}

function inputAdapter(renderInput: ToolDetailRenderer, renderOutput: ToolDetailRenderer): ToolDetailAdapter {
  return { layout: 'input', renderInput, renderOutput }
}

function renderListLoreInput({ input, t }: ToolDetailRenderProps) {
  const keywords = stringArray(input.keywords)
  const filters = [
    keywords.length ? keywords.join(' · ') : t('chat.tool.detail.allLore'),
    stringArray(input.types).length ? `types=${stringArray(input.types).join(',')}` : '',
    stringArray(input.load_modes).length ? `load_modes=${stringArray(input.load_modes).join(',')}` : '',
  ].filter(Boolean)
  return (
    <DetailStack className="space-y-1.5">
      <DetailPre className="text-[var(--nova-text)]">{filters.join(' · ')}</DetailPre>
      <MetaLine items={[fieldMeta('match', input.match), fieldMeta('detail', input.detail), numericMeta('limit', input.limit), numericMeta('offset', input.offset)]} />
    </DetailStack>
  )
}

function renderReadLoreInput({ input }: ToolDetailRenderProps) {
  const ids = stringArray(input.ids)
  const names = stringArray(input.names)
  return (
    <DetailStack className="space-y-1">
      {ids.map(id => <div key={id}><LoreLink id={id}>{id}</LoreLink></div>)}
      {names.map(name => <div key={name}><LoreLink name={name}>{name}</LoreLink></div>)}
    </DetailStack>
  )
}

function renderLoreOutput({ result, t }: ToolDetailRenderProps) {
  const entries = parseLoreEntries(result)
  if (!entries.length) return result.trim() ? <DetailPre>{result}</DetailPre> : <EmptyValue t={t} />
  return (
    <DetailStack>
      {entries.map((entry, index) => (
        <DetailBlock key={`${entry.id || entry.name}-${index}`}>
          <div className="flex min-w-0 flex-wrap items-baseline gap-x-2">
            <LoreLink id={entry.id} name={entry.id ? undefined : entry.name}>{entry.name}</LoreLink>
            <MetaLine items={[entry.meta]} />
          </div>
          {entry.body ? <DetailPre>{entry.body}</DetailPre> : null}
        </DetailBlock>
      ))}
      <MetaLine items={lorePaginationMeta(result)} />
    </DetailStack>
  )
}

function renderWriteLoreInput({ input, t }: ToolDetailRenderProps) {
  const items = recordArray(input.items)
  const creates = items.filter(item => !stringValue(item.id))
  const updates = items.filter(item => stringValue(item.id))
  const deletes = stringArray(input.delete_ids)
  return (
    <DetailStack>
      {stringValue(input.message) ? <DetailPre className="text-[var(--nova-text)]">{stringValue(input.message)}</DetailPre> : null}
      {creates.length ? <LoreMutationGroup title={t('chat.tool.detail.create')} items={creates} /> : null}
      {updates.length ? <LoreMutationGroup title={t('chat.tool.detail.update')} items={updates} link /> : null}
      {deletes.length ? (
        <DetailBlock title={t('chat.tool.detail.delete')} tone="danger">
          <DetailPre>{deletes.join('\n')}</DetailPre>
        </DetailBlock>
      ) : null}
    </DetailStack>
  )
}

function LoreMutationGroup({ title, items, link = false }: { title: string; items: Record<string, unknown>[]; link?: boolean }) {
  return (
    <DetailBlock title={title}>
      <DetailStack>
        {items.map((item, index) => {
          const id = stringValue(item.id)
          const name = stringValue(item.name) || id || `#${index + 1}`
          return (
            <div key={`${id || name}-${index}`} className="min-w-0 space-y-1">
              <div className="text-[var(--nova-text)]">{link && id ? <LoreLink id={id}>{name}</LoreLink> : name}</div>
              <DetailPre>{formatValue(item)}</DetailPre>
            </div>
          )
        })}
      </DetailStack>
    </DetailBlock>
  )
}

function renderWriteLoreOutput({ result, t }: ToolDetailRenderProps) {
  if (!result.trim()) return <EmptyValue t={t} />
  const createdOrUpdated = parseJSONLine(result, 'item_ids')
  const deleted = new Set(parseJSONLine(result, 'deleted_ids'))
  const activeIDs = createdOrUpdated.filter(id => !deleted.has(id))
  const summary = result.split('\n')[0]
  return (
    <DetailStack className="space-y-1.5">
      <span>{summary}</span>
      {activeIDs.length ? (
        <div className="flex min-w-0 flex-wrap gap-x-2 gap-y-0.5">
          {activeIDs.map(id => <LoreLink key={id} id={id}>{id}</LoreLink>)}
        </div>
      ) : null}
      {deleted.size ? <MetaLine items={[t('chat.tool.detail.deletedCount', { count: deleted.size })]} /> : null}
    </DetailStack>
  )
}

function renderHistoryInput({ input, t }: ToolDetailRenderProps) {
  const keywords = stringArray(input.keywords)
  return (
    <DetailStack className="space-y-1.5">
      <DetailPre className="text-[var(--nova-text)]">{keywords.length ? keywords.join(' · ') : t('chat.tool.detail.recentTurns')}</DetailPre>
      <MetaLine items={[fieldMeta('match', input.match), fieldMeta('before_turn_id', input.before_turn_id), numericMeta('limit', input.limit), fieldMeta('cursor', input.cursor)]} />
    </DetailStack>
  )
}

function renderHistoryOutput({ result, t }: ToolDetailRenderProps) {
  const response = parseRecord(result)
  if (!response) return <DetailPre>{formatMaybeJSON(result)}</DetailPre>
  const hits = recordArray(response.hits)
  if (!hits.length) return <EmptyValue t={t} />
  return (
    <DetailStack>
      {hits.map((hit, index) => (
        <DetailBlock key={`${stringValue(hit.turn_id)}-${index}`}>
          <MetaLine items={[stringValue(hit.turn_id), stringValue(hit.timestamp), hit.score === undefined ? '' : `score=${hit.score}`]} />
          {stringValue(hit.user_action) ? <DetailPre className="text-[var(--nova-text)]">{stringValue(hit.user_action)}</DetailPre> : null}
          {stringValue(hit.narrative) ? <DetailPre>{stringValue(hit.narrative)}</DetailPre> : null}
          {stringArray(hit.state_changes).length ? <DetailPre className="text-[var(--nova-text-muted)]">{stringArray(hit.state_changes).join('\n')}</DetailPre> : null}
        </DetailBlock>
      ))}
      {response.truncated === true ? <MetaLine items={[t('chat.tool.detail.partial'), fieldMeta('next_cursor', response.next_cursor)]} /> : null}
    </DetailStack>
  )
}

function LoreLink({ id, name, children }: { id?: string; name?: string; children: string }) {
  return <ToolResourceLink target={{ kind: 'lore_item', id, name }}>{children}</ToolResourceLink>
}

interface LoreEntry { id?: string; name: string; meta?: string; body?: string }

function parseLoreEntries(result: string): LoreEntry[] {
  const full = parseFullLoreEntries(result)
  if (full.length) return full

  const lines = result.split('\n')
  const indexed: LoreEntry[] = []
  for (let index = 0; index < lines.length; index++) {
    const compact = lines[index].match(/^- id:\s*(.+?)\s*\|\s*name:\s*(.+)$/)
    if (compact) {
      indexed.push({ id: compact[1].trim(), name: compact[2].trim() })
      continue
    }
    const start = lines[index].match(/^- id:\s*(.+)$/)
    if (start) {
      const body: string[] = []
      let name = start[1].trim()
      while (index + 1 < lines.length && /^\s{2}\S/.test(lines[index + 1])) {
        const line = lines[++index].trim()
        const nameMatch = line.match(/^name:\s*(.+)$/)
        if (nameMatch) name = nameMatch[1].trim()
        else body.push(line)
      }
      indexed.push({ id: start[1].trim(), name, body: body.join('\n') })
      continue
    }
    const catalog = lines[index].match(/^- \[([^\]]+)\]\s+(.+)$/)
    if (catalog) indexed.push({ name: catalog[2].trim(), meta: catalog[1].trim() })
  }
  return indexed
}

function parseFullLoreEntries(result: string): LoreEntry[] {
  const starts = [...result.matchAll(/^##\s+(.+?)(?:\s+\(([^\n]+)\))?\s*$/gm)]
  const entries: LoreEntry[] = []
  for (let index = 0; index < starts.length; index++) {
    const start = starts[index]
    const chunkStart = (start.index || 0) + start[0].length + 1
    const chunkEnd = index + 1 < starts.length ? starts[index + 1].index || result.length : result.length
    const chunk = result.slice(chunkStart, chunkEnd).trim()
    const idMatch = chunk.match(/^ID:\s*(.+)$/m)
    if (!idMatch) continue
    const body = chunk
      .replace(/^ID:\s*.+\n?/m, '')
      .replace(/^```markdown\s*$/m, '')
      .replace(/^```\s*$/m, '')
      .trim()
    entries.push({ id: idMatch[1].trim(), name: start[1].trim(), meta: start[2]?.trim(), body })
  }
  return entries
}

function lorePaginationMeta(result: string) {
  const next = result.match(/^next_offset:\s*(.+)$/m)?.[1]
    || result.match(/Use offset=(\d+)/)?.[1]
  const total = result.match(/^total:\s*(\d+)$/m)?.[1]
  return [total ? `total=${total}` : '', next && next !== 'null' ? `next_offset=${next}` : '']
}

function parseJSONLine(result: string, field: string) {
  const match = result.match(new RegExp(`^${field}:\\s*(\\[.*\\])$`, 'm'))
  if (!match) return []
  try {
    return stringArray(JSON.parse(match[1]))
  } catch {
    return []
  }
}
