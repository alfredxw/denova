import { AnimatePresence, motion } from 'motion/react'
import { Copy, PanelLeft, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useTranslation } from 'react-i18next'
import { subtlePresence } from '@/features/motion/motion-tokens'
import { cn } from '@/lib/utils'
import type { TreeNode } from '../types'
import { Breadcrumb } from '../shared/Breadcrumb'
import { getBreadcrumb } from '../build-tree'
import type { ExplorerProps } from '../types'
import { TemplateDetailEditor } from './TemplateDetailEditor'
import { FieldDetailEditor } from './FieldDetailEditor'
import { ActorDetailEditor } from './ActorDetailEditor'
import { PoolDetailEditor } from './PoolDetailEditor'
import { TraitDetailEditor } from './TraitDetailEditor'
import { DetailContentFrame } from './DetailLayout'

interface StateDetailAreaProps {
  attached?: boolean
  tree: TreeNode[]
  selectedNode: TreeNode | null
  selectedId: string
  value: ExplorerProps['value']
  onChange: (value: ExplorerProps['value']) => void
  onNodeIdChange: (previousId: string, nextId: string) => void
  onOpenNavigation?: () => void
  onSelect: (id: string) => void
  onDuplicateNode?: (node: TreeNode) => void
  onDeleteNode?: (node: TreeNode) => void
}

export function StateDetailArea({
  attached = false,
  tree,
  selectedNode,
  selectedId,
  value,
  onChange,
  onNodeIdChange,
  onOpenNavigation,
  onSelect,
  onDuplicateNode,
  onDeleteNode,
}: StateDetailAreaProps) {
  const { t } = useTranslation()
  const breadcrumbPath = selectedNode
    ? getBreadcrumb(tree, selectedNode.id)
    : []

  const breadcrumbItems = breadcrumbPath.map((n) => ({
    id: n.id,
    label: n.label,
    selectable: n.selectable,
  }))

  return (
    <div className={cn(
      'flex h-full min-h-0 flex-col overflow-hidden',
      attached
        ? 'rounded-none bg-[var(--nova-bg)]'
        : 'rounded-[20px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]',
    )}>
      {/* Breadcrumb header */}
      <div className="flex min-h-10 items-center border-b border-[var(--nova-border)] px-4 py-2">
        {onOpenNavigation ? (
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            className="actor-state-navigation-trigger mr-2 rounded-full text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]"
            onClick={onOpenNavigation}
            aria-label={t('settingPanel.actorState.explorer.openStructure')}
          >
            <PanelLeft />
          </Button>
        ) : null}
        <Breadcrumb
          items={breadcrumbItems}
          onSelect={onSelect}
          className="flex-1"
        />
        {selectedNode?.selectable && selectedNode.data ? (
          <div className="flex items-center gap-1">
            {onDuplicateNode ? (
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                className="h-7 w-7 rounded-full text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]"
                onClick={() => onDuplicateNode(selectedNode)}
                aria-label={t('settingPanel.presetConfig.copy')}
                title={t('settingPanel.presetConfig.copy')}
              >
                <Copy className="h-3.5 w-3.5" />
              </Button>
            ) : null}
            {onDeleteNode ? (
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                className="h-7 w-7 rounded-full text-[var(--nova-text-faint)] hover:bg-[var(--nova-danger-bg)] hover:text-[var(--nova-danger)]"
                onClick={() => onDeleteNode(selectedNode)}
                aria-label={t('common.delete')}
                title={t('common.delete')}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
            ) : null}
          </div>
        ) : null}
      </div>

      {/* Detail content */}
      <div className="min-h-0 flex-1 overflow-y-auto" data-testid="actor-state-detail-scroll">
        <AnimatePresence mode="wait">
          <motion.div
            key={detailNodeKey(selectedNode)}
            variants={subtlePresence}
            initial="initial"
            animate="animate"
            exit="exit"
            className="p-4 2xl:p-5"
          >
            <DetailContentFrame>
              <DetailContent
                node={selectedNode}
                selectedId={selectedId}
                tree={tree}
                value={value}
                onChange={onChange}
                onNodeIdChange={onNodeIdChange}
                onSelect={onSelect}
              />
            </DetailContentFrame>
          </motion.div>
        </AnimatePresence>
      </div>
    </div>
  )
}

function DetailContent({
  node,
  selectedId,
  tree,
  value,
  onChange,
  onNodeIdChange,
  onSelect,
}: {
  node: TreeNode | null
  selectedId: string
  tree: TreeNode[]
  value: ExplorerProps['value']
  onChange: (value: ExplorerProps['value']) => void
  onNodeIdChange: (previousId: string, nextId: string) => void
  onSelect: (id: string) => void
}) {
  const { t } = useTranslation()
  if (!node || !node.data) {
    return (
      <div className="flex h-64 items-center justify-center">
        <div className="text-center">
          <div className="text-sm text-[var(--nova-text-muted)]">
            {t('settingPanel.actorState.explorer.selectTitle')}
          </div>
          <div className="mt-1 text-xs text-[var(--nova-text-faint)]">
            {t('settingPanel.actorState.explorer.selectDesc')}
          </div>
        </div>
      </div>
    )
  }

  switch (node.data.kind) {
    case 'template':
      return (
        <TemplateDetailEditor
          template={node.data.template}
          templateIndex={node.data.index}
          selectedId={selectedId}
          tree={tree}
          value={value}
          onChange={onChange}
          onIdChange={(nextId) => onNodeIdChange(node.id, nextId)}
          onSelect={onSelect}
        />
      )

    case 'field':
      return (
        <FieldDetailEditor
          field={node.data.field}
          fieldIndex={node.data.fieldIndex}
          template={node.data.template}
          templateIndex={node.data.templateIndex}
          value={value}
          onChange={onChange}
          onIdChange={(nextId) => onNodeIdChange(node.id, nextId)}
        />
      )

    case 'actor':
      return (
        <ActorDetailEditor
          actor={node.data.actor}
          actorIndex={node.data.actorIndex}
          template={node.data.template}
          value={value}
          onChange={onChange}
          onIdChange={(nextId) => onNodeIdChange(node.id, nextId)}
        />
      )

    case 'pool':
      return (
        <PoolDetailEditor
          pool={node.data.pool}
          poolIndex={node.data.poolIndex}
          selectedId={selectedId}
          tree={tree}
          value={value}
          onChange={onChange}
          onIdChange={(nextId) => onNodeIdChange(node.id, nextId)}
          onSelect={onSelect}
        />
      )

    case 'trait':
      return (
        <TraitDetailEditor
          trait={node.data.trait}
          traitIndex={node.data.traitIndex}
          pool={node.data.pool}
          poolIndex={node.data.poolIndex}
          value={value}
          onChange={onChange}
          onIdChange={(nextId) => onNodeIdChange(node.id, nextId)}
        />
      )

    default:
      return (
        <div className="flex h-32 items-center justify-center text-xs text-[var(--nova-text-faint)]">
          {t('settingPanel.actorState.explorer.unavailable')}
        </div>
      )
  }
}

function detailNodeKey(node: TreeNode | null): string {
  const data = node?.data
  if (!data) return 'empty'
  switch (data.kind) {
    case 'template': return `template:${data.index}`
    case 'field': return `field:${data.templateIndex}:${data.fieldIndex}`
    case 'actor': return `actor:${data.actorIndex}`
    case 'pool': return `pool:${data.poolIndex}`
    case 'trait': return `trait:${data.poolIndex}:${data.traitIndex}`
  }
}
