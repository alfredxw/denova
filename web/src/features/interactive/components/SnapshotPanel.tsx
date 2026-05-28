import { Activity, Compass, Copy, Flag, MapPin, Package, Plus, Sparkles, Tag, UserRoundCheck } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import type { Snapshot } from '../types'

function asArray(value: unknown): unknown[] {
  return Array.isArray(value) ? value : []
}

export function SnapshotPanel({ snapshot }: { snapshot: Snapshot | null }) {
  const state = snapshot?.state || {}
  const onStage = asArray(state.on_stage)
  const events = asArray(state.events)
  const characters = state.characters && typeof state.characters === 'object'
    ? Object.entries(state.characters as Record<string, unknown>)
    : []
  const scene = isPlainObject(state.scene) ? state.scene : {}
  const inventory = isPlainObject(state.inventory) ? state.inventory : null
  const resources = isPlainObject(state.resources) ? state.resources : null
  const worldFlags = asArray(state.world_flags)
  const rules = asArray(state.rules)
  const threads = asArray(state.threads)
  const actionSpace = asArray(state.action_space)
  const location = pickString(state, ['location', 'place', 'scene', '地点'])
    || pickString(scene, ['location', 'place', 'name', '地点', '场景'])
  const time = pickString(state, ['time', 'moment', '时间'])
  const pov = pickString(state, ['pov', 'viewpoint', '视角'])
  const sceneEntries = Object.entries(scene).filter(([key]) => !SCENE_METRIC_KEYS.has(key))
  const stateStatus = snapshot?.current_turn?.state_status

  return (
    <aside className="nova-sidebar flex h-full min-w-0 flex-col border-l p-4">
      <div className="mb-3 flex h-8 items-center justify-between">
        <div>
          <h2 className="text-sm font-semibold text-[var(--nova-text)]">场景记忆</h2>
          <div className="text-[11px] text-[var(--nova-text-faint)]">当前回合的实时上下文</div>
        </div>
        <div className="flex items-center gap-1.5">
          {stateStatus === 'pending' ? <Badge variant="outline" className="border-[var(--nova-accent)]/40 bg-[var(--nova-accent)]/10 text-[var(--nova-accent)]">同步中</Badge> : null}
          {stateStatus === 'failed' ? <Badge variant="outline" className="border-red-500/35 bg-red-500/10 text-red-300">同步失败</Badge> : null}
          <Badge variant="outline" className="border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]">{formatBranchName(snapshot?.branch_id)}</Badge>
        </div>
      </div>
      {stateStatus === 'failed' && snapshot?.current_turn?.state_error ? (
        <div className="mb-3 rounded-[var(--nova-radius)] border border-red-500/35 bg-red-500/10 px-3 py-2 text-xs text-red-300">
          {snapshot.current_turn.state_error}
        </div>
      ) : null}
      <ScrollArea className="min-h-0 flex-1 pr-1">
        <section className={panelSectionClass}>
          <div className={sectionTitleClass}>
            <MapPin className="h-3.5 w-3.5" />
            当前场景
          </div>
          <div className="grid grid-cols-[repeat(auto-fit,minmax(72px,1fr))] gap-2">
            <SnapshotMetric label="地点" value={location || '未记录'} />
            <SnapshotMetric label="时间" value={time || '未记录'} />
            <SnapshotMetric label="视角" value={pov || '未记录'} />
          </div>
          {sceneEntries.length ? (
            <div className="mt-3 border-t border-[var(--nova-border)] pt-3">
              <StateValue value={Object.fromEntries(sceneEntries)} />
            </div>
          ) : null}
        </section>

        <section className={panelSectionClass}>
          <div className="mb-2 text-xs font-semibold text-[var(--nova-text-muted)]">场景笔记</div>
          <div className={`${panelCardClass} px-3 py-2 text-xs leading-5 text-[var(--nova-text-faint)]`}>
            记录场景的设计意图或待完成事项...
          </div>
        </section>

        <section className={panelSectionClass}>
          <div className={sectionTitleClass}>
            <UserRoundCheck className="h-3.5 w-3.5" />
            在场角色
          </div>
          <div className="flex flex-wrap gap-1.5 text-sm text-[var(--nova-text-muted)]">
            {onStage.length ? onStage.map((name) => <Badge key={String(name)} className="border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text)]" variant="secondary">{String(name)}</Badge>) : '暂无在场角色'}
          </div>
        </section>

        <section className={panelSectionClass}>
          <div className={sectionTitleClass}>
            <Compass className="h-3.5 w-3.5" />
            可选择
          </div>
          <CompactList items={actionSpace} empty="暂无可选择" />
        </section>

        <section className={panelSectionClass}>
          <div className={sectionTitleClass}>
            <Activity className="h-3.5 w-3.5" />
            角色状态
          </div>
          <div className="space-y-2 text-xs text-[var(--nova-text-muted)]">
            {characters.length ? characters.map(([name, state]) => (
              <div key={name} className={`${panelCardClass} p-2`}>
                <div className="mb-1 font-medium text-[var(--nova-text)]">{name}</div>
                <StateValue value={state} />
              </div>
            )) : '暂无角色状态'}
          </div>
        </section>

        <section className={panelSectionClass}>
          <div className={sectionTitleClass}>
            <Package className="h-3.5 w-3.5" />
            物品与资源
          </div>
          <div className="space-y-2 text-xs text-[var(--nova-text-muted)]">
            {inventory ? (
              <div className={`${panelCardClass} p-2`}>
                <div className="mb-1 font-medium text-[var(--nova-text)]">物品</div>
                <StateValue value={inventory} />
              </div>
            ) : null}
            {resources ? (
              <div className={`${panelCardClass} p-2`}>
                <div className="mb-1 font-medium text-[var(--nova-text)]">资源</div>
                <StateValue value={resources} />
              </div>
            ) : null}
            {!inventory && !resources ? '暂无物品或资源变化' : null}
          </div>
        </section>

        <section className={panelSectionClass}>
          <div className={sectionTitleClass}>
            <Flag className="h-3.5 w-3.5" />
            规则与暗线
          </div>
          <div className="space-y-3 text-xs text-[var(--nova-text-muted)]">
            <LabeledList label="世界规则" items={[...worldFlags, ...rules]} empty="暂无已激活规则" />
            <LabeledList label="未解决线索" items={threads} empty="暂无未解决线索" />
          </div>
        </section>

        <section className={panelSectionClass}>
          <div className={sectionTitleClass}>
            <Sparkles className="h-3.5 w-3.5" />
            关键事件
          </div>
          <div className="space-y-2 text-xs text-[var(--nova-text-muted)]">
            {events.length ? events.map((event, index) => (
              <EventItem key={index} event={event} index={index} />
            )) : '暂无关键事件'}
          </div>
        </section>

        <section className={`${panelSectionClass} mb-0`}>
          <div className="mb-3 text-xs font-semibold text-[var(--nova-text-muted)]">快捷操作</div>
          <div className="grid grid-cols-2 gap-2">
            <InspectorAction icon={Plus} label="新建场景" />
            <InspectorAction icon={Copy} label="复制链接" />
            <InspectorAction icon={Compass} label="添加分支" />
            <InspectorAction icon={Tag} label="设置标签" />
          </div>
        </section>
      </ScrollArea>
    </aside>
  )
}

function InspectorAction({ icon: Icon, label }: { icon: LucideIcon; label: string }) {
  return (
    <button type="button" className="nova-nav-item flex h-9 items-center justify-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-xs text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]">
      <Icon className="h-3.5 w-3.5" />
      <span>{label}</span>
    </button>
  )
}

function SnapshotMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-2">
      <div className="text-[10px] text-[var(--nova-text-faint)]">{label}</div>
      <div className="truncate text-xs font-medium text-[var(--nova-text)]" title={value}>{value}</div>
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
    if (!entries.length) return <div className="text-[var(--nova-text-faint)]">暂无记录</div>
    return (
      <dl className="grid gap-1.5">
        {entries.map(([key, item]) => (
          <div key={key} className="grid grid-cols-[64px_minmax(0,1fr)] gap-2">
            <dt className="truncate text-[var(--nova-text-faint)]" title={formatLabel(key)}>{formatLabel(key)}</dt>
            <dd className="min-w-0 text-[var(--nova-text-muted)]">{renderReadableValue(item)}</dd>
          </div>
        ))}
      </dl>
    )
  }
  return <div className="whitespace-pre-wrap text-[var(--nova-text-muted)]">{formatScalar(value)}</div>
}

function EventItem({ event, index }: { event: unknown; index: number }) {
  if (!isPlainObject(event)) {
    return (
      <div className={`${panelCardClass} p-2 text-[var(--nova-text-muted)]`}>
        {formatScalar(event)}
      </div>
    )
  }

  const title = pickEventTitle(event, index)
  const description = pickString(event, ['description', 'summary', 'content', 'text', 'event', '事件', '描述'])
  const detailEntries = Object.entries(event).filter(([key]) => !EVENT_PRIMARY_KEYS.has(key))

  return (
    <article className={`${panelCardClass} p-2`}>
      <div className="mb-1 flex items-start justify-between gap-2">
        <div className="min-w-0 font-medium text-[var(--nova-text)]">{title}</div>
        {typeof event.type === 'string' && event.type.trim() ? (
          <Badge variant="outline" className="h-5 shrink-0 border-[var(--nova-border)] bg-[var(--nova-surface)] px-1.5 text-[10px] text-[var(--nova-text-muted)]">
            {event.type.trim()}
          </Badge>
        ) : null}
      </div>
      {description ? <div className="mb-2 whitespace-pre-wrap text-[var(--nova-text-muted)]">{description}</div> : null}
      {detailEntries.length ? (
        <dl className="grid gap-1.5">
          {detailEntries.map(([key, value]) => (
            <div key={key} className="grid grid-cols-[64px_minmax(0,1fr)] gap-2">
              <dt className="truncate text-[var(--nova-text-faint)]" title={formatLabel(key)}>{formatLabel(key)}</dt>
              <dd className="min-w-0 text-[var(--nova-text-muted)]">{renderReadableValue(value)}</dd>
            </div>
          ))}
        </dl>
      ) : null}
    </article>
  )
}

function CompactList({ items, empty }: { items: unknown[]; empty: string }) {
  if (!items.length) return <div className="text-xs text-[var(--nova-text-muted)]">{empty}</div>
  return (
    <div className="space-y-1.5 text-xs text-[var(--nova-text-muted)]">
      {items.map((item, index) => (
        <div key={index} className={`${panelCardClass} px-2 py-1.5`}>
          {renderReadableValue(item)}
        </div>
      ))}
    </div>
  )
}

function LabeledList({ label, items, empty }: { label: string; items: unknown[]; empty: string }) {
  return (
    <div>
      <div className="mb-1 text-[10px] font-medium text-[var(--nova-text-faint)]">{label}</div>
      <CompactList items={items} empty={empty} />
    </div>
  )
}

function renderReadableValue(value: unknown): ReactNode {
  if (Array.isArray(value)) {
    if (!value.length) return <span className="text-[var(--nova-text-faint)]">空</span>
    if (value.every((item) => !isPlainObject(item) && !Array.isArray(item))) {
      return (
        <div className="flex flex-wrap gap-1">
          {value.map((item, index) => (
            <Badge key={index} variant="secondary" className="border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)]">
              {formatScalar(item)}
            </Badge>
          ))}
        </div>
      )
    }
    return (
      <div className="space-y-1">
        {value.map((item, index) => (
          <div key={index} className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1">
            {renderReadableValue(item)}
          </div>
        ))}
      </div>
    )
  }
  if (isPlainObject(value)) {
    const entries = Object.entries(value)
    if (!entries.length) return <span className="text-[var(--nova-text-faint)]">空</span>
    return (
      <div className="space-y-1">
        {entries.map(([key, item]) => (
          <div key={key} className="grid grid-cols-[64px_minmax(0,1fr)] gap-1">
            <span className="text-[var(--nova-text-faint)]">{formatLabel(key)}</span>
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
    resources: '资源',
    resource: '资源',
    danger: '危险',
    danger_level: '危险度',
    tension: '紧张度',
    atmosphere: '氛围',
    obstacle: '阻碍',
    obstacles: '阻碍',
    exits: '出口',
    interactive_objects: '可交互物',
    action_space: '可选择',
    world_flags: '世界标记',
    rules: '世界规则',
    threads: '未解决线索',
    hook: '钩子',
    hooks: '钩子',
    clue: '线索',
    clues: '线索',
    cost: '代价',
    consequence: '后果',
    known_info: '已知信息',
    stance: '立场',
    injury: '伤势',
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
  flags: '标记',
  space: '空间',
  world: '世界',
  danger: '危险',
  action: '行动',
}

const EVENT_PRIMARY_KEYS = new Set(['title', 'name', 'flag', 'event', '事件名', 'description', 'summary', 'content', 'text', '事件', '描述', 'type'])
const SCENE_METRIC_KEYS = new Set(['location', 'place', 'name', '地点', '场景'])

const panelSectionClass = 'mb-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3'
const panelCardClass = 'rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]'
const sectionTitleClass = 'mb-3 flex items-center gap-2 text-xs font-semibold text-[var(--nova-text-muted)]'
