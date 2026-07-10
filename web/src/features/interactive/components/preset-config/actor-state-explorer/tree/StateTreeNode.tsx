import { ChevronDown, ChevronRight, Database, FileSpreadsheet, Hash, Layers, Plus, Sparkle, Sparkles, User, Users, Zap, type LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import type { TreeNode, TreeNodeKind } from '../types'
import { StateTreeGroupHeader } from './StateTreeGroupHeader'

const KIND_ICONS: Record<TreeNodeKind, LucideIcon> = {
  group: Database,
  template: FileSpreadsheet,
  field: Hash,
  'actor-group': Users,
  actor: User,
  'opening-group': Sparkles,
  'opening-ops': Zap,
  pool: Layers,
  trait: Sparkle,
}

interface StateTreeNodeProps {
  node: TreeNode
  selectedId: string
  expandedIds: Set<string>
  indentLevel: number
  onSelect: (id: string) => void
  onToggleExpanded: (id: string) => void
  onAddTemplate?: () => void
  onAddField?: (templateId: string) => void
  onAddActor?: (templateId: string) => void
  onAddPool?: () => void
  onAddTrait?: (poolId: string) => void
}

export function StateTreeNode({
  node,
  selectedId,
  expandedIds,
  indentLevel,
  onSelect,
  onToggleExpanded,
  onAddTemplate,
  onAddField,
  onAddActor,
  onAddPool,
  onAddTrait,
}: StateTreeNodeProps) {
  const { t } = useTranslation()
  // Group nodes use the group header component
  if (node.kind === 'group' || node.kind === 'actor-group' || node.kind === 'opening-group') {
    const expanded = expandedIds.has(node.id)
    const addHandler = getGroupAddHandler(node, { onAddTemplate, onAddField, onAddActor, onAddPool, onAddTrait })

    return (
      <div className="mt-0.5 min-w-0 max-w-full overflow-hidden">
        <StateTreeGroupHeader
          label={node.label}
          badge={node.badge}
          expanded={expanded}
          onToggle={() => onToggleExpanded(node.id)}
          onAdd={addHandler}
          addLabel={getGroupAddLabel(node.kind, t)}
          indentLevel={indentLevel}
        >
          <div className="mt-0.5 min-w-0 max-w-full overflow-hidden">
            {node.children.map((child) => (
              <StateTreeNode
                key={child.id}
                node={child}
                selectedId={selectedId}
                expandedIds={expandedIds}
                indentLevel={indentLevel + 1}
                onSelect={onSelect}
                onToggleExpanded={onToggleExpanded}
                onAddTemplate={onAddTemplate}
                onAddField={onAddField}
                onAddActor={onAddActor}
                onAddPool={onAddPool}
                onAddTrait={onAddTrait}
              />
            ))}
          </div>
        </StateTreeGroupHeader>
      </div>
    )
  }

  // Selectable item nodes
  return (
    <TreeItem
      node={node}
      selectedId={selectedId}
      expandedIds={expandedIds}
      indentLevel={indentLevel}
      onSelect={onSelect}
      onToggleExpanded={onToggleExpanded}
      onAddField={onAddField}
      onAddActor={onAddActor}
      onAddTrait={onAddTrait}
    />
  )
}

interface TreeItemProps {
  node: TreeNode
  selectedId: string
  expandedIds: Set<string>
  indentLevel: number
  onSelect: (id: string) => void
  onToggleExpanded: (id: string) => void
  onAddField?: (templateId: string) => void
  onAddActor?: (templateId: string) => void
  onAddTrait?: (poolId: string) => void
}

function TreeItem({
  node,
  selectedId,
  expandedIds,
  indentLevel,
  onSelect,
  onToggleExpanded,
  onAddField,
  onAddActor,
  onAddTrait,
}: TreeItemProps) {
  const { t } = useTranslation()
  const isSelected = node.id === selectedId
  const hasChildren = node.children.length > 0
  const expanded = expandedIds.has(node.id)
  const Icon = KIND_ICONS[node.kind] || FileSpreadsheet
  const paddingLeft = 6 + indentLevel * 12
  const isField = node.kind === 'field'

  // Determine add handler for this node type (template can add fields/actors, pool can add traits)
  const addHandler = getNodeAddHandler(node, { onAddField, onAddActor, onAddTrait })

  return (
    <div className="relative min-w-0 max-w-full overflow-hidden">
      <div
        className={cn(
          'group flex min-h-8 w-full min-w-0 max-w-full items-center gap-1 overflow-hidden rounded-[10px] pr-2 transition-colors duration-200',
          isSelected
            ? 'bg-[var(--nova-surface)] text-[var(--nova-text)] shadow-[inset_3px_0_0_var(--nova-accent),inset_0_0_0_1px_var(--nova-border)]'
            : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]',
        )}
        style={{ paddingLeft }}
      >
        {/* Expand/collapse for nodes with children */}
        {hasChildren ? (
          <button
            type="button"
            className="flex h-7 w-4 shrink-0 items-center justify-center text-[var(--nova-text-faint)] transition-colors hover:text-[var(--nova-text)]"
            onClick={(e) => {
              e.stopPropagation()
              onToggleExpanded(node.id)
            }}
            aria-label={expanded ? t('settingPanel.actorState.explorer.collapse') : t('settingPanel.actorState.explorer.expand')}
          >
            <ChevronIcon expanded={expanded} />
          </button>
        ) : null}

        {/* Icon */}
        <Icon className={cn(
          'h-3.5 w-3.5 shrink-0',
          isSelected ? 'text-[var(--nova-accent)]' : 'text-[var(--nova-text-faint)]',
        )} />

        {/* Label + subtitle */}
        <button
          type="button"
          className="flex min-w-0 flex-1 flex-col items-start py-1.5 text-left"
          onClick={() => onSelect(node.id)}
          title={node.subtitle ? `${node.label}\n${node.subtitle}` : node.label}
        >
          <span className="block w-full truncate text-[12px] font-medium leading-tight">
            {node.label}
          </span>
          {node.subtitle ? (
            <span className={cn(
              'mt-0.5 block w-full truncate text-[10px] text-[var(--nova-text-faint)]',
              isField && 'font-mono',
            )}>
              {node.subtitle}
            </span>
          ) : null}
        </button>

        {/* Badge */}
        {node.badge ? (
          <span className="max-w-[4.5rem] shrink-0 truncate rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] leading-none text-[var(--nova-text-faint)]">
            {node.badge}
          </span>
        ) : null}

        {/* Add button for template/pool nodes */}
        {addHandler ? (
          <button
            type="button"
            className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-[var(--nova-text-faint)] opacity-0 transition-opacity duration-200 hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] group-hover:opacity-100"
            onClick={(e) => {
              e.stopPropagation()
              addHandler()
            }}
            aria-label={t('settingPanel.actorState.explorer.addChild')}
            title={t('settingPanel.actorState.explorer.addChild')}
          >
            <PlusIcon />
          </button>
        ) : null}
      </div>

      {/* Children */}
      {hasChildren && expanded ? (
        <div className="mt-0.5 min-w-0 max-w-full overflow-hidden">
          {node.children.map((child) => (
            <StateTreeNode
              key={child.id}
              node={child}
              selectedId={selectedId}
              expandedIds={expandedIds}
              indentLevel={indentLevel + 1}
              onSelect={onSelect}
              onToggleExpanded={onToggleExpanded}
              onAddField={onAddField}
              onAddActor={onAddActor}
              onAddTrait={onAddTrait}
            />
          ))}
        </div>
      ) : null}
    </div>
  )
}

function getGroupAddHandler(
  node: TreeNode,
  handlers: {
    onAddTemplate?: () => void
    onAddField?: (templateId: string) => void
    onAddActor?: (templateId: string) => void
    onAddPool?: () => void
    onAddTrait?: (poolId: string) => void
  },
): (() => void) | undefined {
  if (node.kind === 'group') return handlers.onAddTemplate
  if (node.kind === 'opening-group') return handlers.onAddPool
  return undefined
}

function getGroupAddLabel(kind: TreeNodeKind, t: ReturnType<typeof useTranslation>['t']): string {
  switch (kind) {
    case 'group': return t('settingPanel.actorState.addTemplate')
    case 'opening-group': return t('settingPanel.actorState.explorer.addPool')
    default: return t('settingPanel.actorState.explorer.addChild')
  }
}

function getNodeAddHandler(
  node: TreeNode,
  handlers: {
    onAddField?: (templateId: string) => void
    onAddActor?: (templateId: string) => void
    onAddTrait?: (poolId: string) => void
  },
): (() => void) | undefined {
  if (node.kind === 'template' && handlers.onAddField) {
    return () => handlers.onAddField!(node.data?.kind === 'template' ? node.data.template.id : '')
  }
  if (node.kind === 'pool' && handlers.onAddTrait) {
    return () => handlers.onAddTrait!(node.data?.kind === 'pool' ? node.data.pool.id || '' : '')
  }
  return undefined
}

// ── Small inline icon components to avoid import-per-icon issues ──

function ChevronIcon({ expanded }: { expanded: boolean }) {
  return expanded ? (
    <ChevronDown className="h-3.5 w-3.5" />
  ) : (
    <ChevronRight className="h-3.5 w-3.5" />
  )
}

function PlusIcon() {
  return <Plus className="h-3.5 w-3.5" />
}
