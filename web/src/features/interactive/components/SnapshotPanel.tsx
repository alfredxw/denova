import { Activity, MapPin, Sparkles, UserRoundCheck } from 'lucide-react'
import type { ReactNode } from 'react'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import type { Snapshot } from '../types'

function asArray(value: unknown): unknown[] {
  return Array.isArray(value) ? value : []
}

export function SnapshotPanel({ snapshot }: { snapshot: Snapshot | null }) {
  const onStage = asArray(snapshot?.state?.on_stage)
  const events = asArray(snapshot?.state?.events)
  const state = snapshot?.state || {}
  const characters = snapshot?.state?.characters && typeof snapshot.state.characters === 'object'
    ? Object.entries(snapshot.state.characters as Record<string, unknown>)
    : []
  const location = pickString(state, ['location', 'place', 'scene', '地点'])
  const time = pickString(state, ['time', 'moment', '时间'])
  const pov = pickString(state, ['pov', 'viewpoint', '视角'])

  return (
    <aside className="flex h-full w-[336px] shrink-0 flex-col border-l border-[#2f3540] bg-[#1b1e24] p-4">
      <div className="mb-3 flex h-8 items-center justify-between">
        <div>
          <h2 className="text-sm font-semibold text-[#e0e4ec]">场景记忆</h2>
          <div className="text-[11px] text-[#7f8898]">当前回合的实时上下文</div>
        </div>
        <Badge variant="outline" className="border-[#3a414d] bg-[#252a33] text-[#8d96a7]">{formatBranchName(snapshot?.branch_id)}</Badge>
      </div>
      <ScrollArea className="min-h-0 flex-1 pr-1">
        <section className="mb-3 rounded-lg border border-[#343b47] bg-[#111318] p-3">
          <div className="mb-3 flex items-center gap-2 text-xs font-semibold text-[#7fb7e8]">
            <MapPin className="h-3.5 w-3.5" />
            当前场景
          </div>
          <div className="grid grid-cols-3 gap-2">
            <SnapshotMetric label="地点" value={location || '未记录'} />
            <SnapshotMetric label="时间" value={time || '未记录'} />
            <SnapshotMetric label="视角" value={pov || '未记录'} />
          </div>
        </section>

        <section className="mb-3 rounded-lg border border-[#343b47] bg-[#111318] p-3">
          <div className="mb-3 flex items-center gap-2 text-xs font-semibold text-[#7fb7e8]">
            <UserRoundCheck className="h-3.5 w-3.5" />
            在场角色
          </div>
          <div className="flex flex-wrap gap-1.5 text-sm text-[#a8adb7]">
            {onStage.length ? onStage.map((name) => <Badge key={String(name)} className="bg-[#263646] text-[#d6e9ff]" variant="secondary">{String(name)}</Badge>) : '暂无在场角色'}
          </div>
        </section>

        <section className="mb-3 rounded-lg border border-[#343b47] bg-[#111318] p-3">
          <div className="mb-3 flex items-center gap-2 text-xs font-semibold text-[#7fb7e8]">
            <Activity className="h-3.5 w-3.5" />
            角色状态
          </div>
          <div className="space-y-2 text-xs text-[#a8adb7]">
            {characters.length ? characters.map(([name, state]) => (
              <div key={name} className="rounded-md border border-[#303743] bg-[#191d24] p-2">
                <div className="mb-1 font-medium text-[#d6dbe5]">{name}</div>
                <StateValue value={state} />
              </div>
            )) : '暂无角色状态'}
          </div>
        </section>

        <section className="rounded-lg border border-[#343b47] bg-[#111318] p-3">
          <div className="mb-3 flex items-center gap-2 text-xs font-semibold text-[#7fb7e8]">
            <Sparkles className="h-3.5 w-3.5" />
            关键事件
          </div>
          <div className="space-y-2 text-xs text-[#a8adb7]">
            {events.length ? events.map((event, index) => (
              <EventItem key={index} event={event} index={index} />
            )) : '暂无关键事件'}
          </div>
        </section>
      </ScrollArea>
    </aside>
  )
}

function SnapshotMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-md border border-[#303743] bg-[#191d24] px-2 py-2">
      <div className="text-[10px] text-[#747f91]">{label}</div>
      <div className="truncate text-xs font-medium text-[#c8d0dd]" title={value}>{value}</div>
    </div>
  )
}

function pickString(source: Record<string, unknown>, keys: string[]) {
  for (const key of keys) {
    const value = source[key]
    if (typeof value === 'string' && value.trim()) return value.trim()
  }
  return ''
}

function StateValue({ value }: { value: unknown }) {
  if (isPlainObject(value)) {
    const entries = Object.entries(value)
    if (!entries.length) return <div className="text-[#747f91]">暂无记录</div>
    return (
      <dl className="grid gap-1.5">
        {entries.map(([key, item]) => (
          <div key={key} className="grid grid-cols-[64px_minmax(0,1fr)] gap-2">
            <dt className="truncate text-[#747f91]" title={formatLabel(key)}>{formatLabel(key)}</dt>
            <dd className="min-w-0 text-[#aeb6c4]">{renderReadableValue(item)}</dd>
          </div>
        ))}
      </dl>
    )
  }
  return <div className="whitespace-pre-wrap text-[#aeb6c4]">{formatScalar(value)}</div>
}

function EventItem({ event, index }: { event: unknown; index: number }) {
  if (!isPlainObject(event)) {
    return (
      <div className="rounded-md border border-[#303743] bg-[#191d24] p-2 text-[#aeb6c4]">
        {formatScalar(event)}
      </div>
    )
  }

  const title = pickEventTitle(event, index)
  const description = pickString(event, ['description', 'summary', 'content', 'text', 'event', '事件', '描述'])
  const detailEntries = Object.entries(event).filter(([key]) => !EVENT_PRIMARY_KEYS.has(key))

  return (
    <article className="rounded-md border border-[#303743] bg-[#191d24] p-2">
      <div className="mb-1 flex items-start justify-between gap-2">
        <div className="min-w-0 font-medium text-[#d6dbe5]">{title}</div>
        {typeof event.type === 'string' && event.type.trim() ? (
          <Badge variant="outline" className="h-5 shrink-0 border-[#3a414d] bg-[#222936] px-1.5 text-[10px] text-[#93a4bb]">
            {event.type.trim()}
          </Badge>
        ) : null}
      </div>
      {description ? <div className="mb-2 whitespace-pre-wrap text-[#aeb6c4]">{description}</div> : null}
      {detailEntries.length ? (
        <dl className="grid gap-1.5">
          {detailEntries.map(([key, value]) => (
            <div key={key} className="grid grid-cols-[64px_minmax(0,1fr)] gap-2">
              <dt className="truncate text-[#747f91]" title={formatLabel(key)}>{formatLabel(key)}</dt>
              <dd className="min-w-0 text-[#aeb6c4]">{renderReadableValue(value)}</dd>
            </div>
          ))}
        </dl>
      ) : null}
    </article>
  )
}

function renderReadableValue(value: unknown): ReactNode {
  if (Array.isArray(value)) {
    if (!value.length) return <span className="text-[#747f91]">空</span>
    if (value.every((item) => !isPlainObject(item) && !Array.isArray(item))) {
      return (
        <div className="flex flex-wrap gap-1">
          {value.map((item, index) => (
            <Badge key={index} variant="secondary" className="bg-[#243040] text-[#c8d6e8]">
              {formatScalar(item)}
            </Badge>
          ))}
        </div>
      )
    }
    return (
      <div className="space-y-1">
        {value.map((item, index) => (
          <div key={index} className="rounded border border-[#2b323d] bg-[#151920] px-2 py-1">
            {renderReadableValue(item)}
          </div>
        ))}
      </div>
    )
  }
  if (isPlainObject(value)) {
    const entries = Object.entries(value)
    if (!entries.length) return <span className="text-[#747f91]">空</span>
    return (
      <div className="space-y-1">
        {entries.map(([key, item]) => (
          <div key={key} className="grid grid-cols-[64px_minmax(0,1fr)] gap-1">
            <span className="text-[#747f91]">{formatLabel(key)}</span>
            <span className="min-w-0">{renderReadableValue(item)}</span>
          </div>
        ))}
      </div>
    )
  }
  return <span className="whitespace-pre-wrap">{formatScalar(value)}</span>
}

function pickEventTitle(event: Record<string, unknown>, index: number) {
  const title = pickString(event, ['title', 'name', 'flag', 'event', '事件名'])
  if (title) return title
  return `事件 ${index + 1}`
}

function formatLabel(key: string) {
  const normalized = key.trim()
  const labels: Record<string, string> = {
    id: '编号',
    type: '类型',
    title: '标题',
    name: '名称',
    flag: '标记',
    description: '描述',
    summary: '摘要',
    content: '内容',
    text: '内容',
    event: '事件',
    story_id: '故事',
    branch_id: '剧情线',
    parent_id: '上级节点',
    parent_event_id: '分叉节点',
    from_event: '来源事件',
    created_at: '创建时间',
    updated_at: '更新时间',
    on_stage: '在场角色',
    characters: '角色',
    events: '关键事件',
    hp: '体力',
    mp: '精神',
    health: '健康',
    location: '位置',
    place: '地点',
    scene: '场景',
    mood: '情绪',
    status: '状态',
    state: '状态',
    relation: '关系',
    relations: '关系',
    relationship: '关系',
    relationship_score: '关系值',
    goal: '目标',
    current_goal: '当前目标',
    current_status: '当前状态',
    current_location: '当前位置',
    last_seen_at: '最后出现',
    item: '物品',
    items: '物品',
    inventory: '物品',
    ts: '时间',
    time: '时间',
    moment: '时刻',
    pov: '视角',
    viewpoint: '视角',
  }
  if (labels[normalized]) return labels[normalized]
  if (/^[a-z][a-z0-9_]*$/i.test(normalized) && normalized.includes('_')) {
    return normalized.split('_').map((part) => labels[part] || FIELD_WORDS[part] || part).join('')
  }
  return normalized
}

function formatScalar(value: unknown) {
  if (value === null || value === undefined) return '未记录'
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  return JSON.stringify(value)
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value)
}

function formatBranchName(branchId?: string) {
  if (!branchId || branchId === 'main') return '主线'
  if (/^branch[_-]?\d+$/i.test(branchId)) return `剧情线 ${branchId.replace(/^branch[_-]?/i, '')}`
  return branchId
}

const FIELD_WORDS: Record<string, string> = {
  current: '当前',
  last: '最后',
  seen: '出现',
  at: '时间',
  score: '值',
  level: '等级',
  branch: '剧情线',
  story: '故事',
  parent: '上级',
  from: '来源',
  created: '创建',
  updated: '更新',
  stage: '在场',
}

const EVENT_PRIMARY_KEYS = new Set(['title', 'name', 'flag', 'event', '事件名', 'description', 'summary', 'content', 'text', '事件', '描述', 'type'])
