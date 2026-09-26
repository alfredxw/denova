import { useRef, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { projectFileAssetURL, type LoreItem, type LoreMaterial } from '@/lib/api'
import { LoreMaterialsPanel } from './LoreMaterialsPanel'

/** Both editors share one media surface. Media responses never replace text
 * being edited while an upload or generation request is in flight. */
export function LoreDetailTabs({
  projectId,
  item,
  onChange,
  onInspectMaterial,
  children,
}: {
  projectId: string
  item: LoreItem
  onChange: (item: LoreItem) => void
  onInspectMaterial?: (material: LoreMaterial) => void
  children: ReactNode
}) {
  const { t } = useTranslation()
  const current = useRef(item)
  current.current = item
  const applyMedia = (saved: LoreItem) => {
    if (saved.id !== current.current.id) return
    onChange({
      ...current.current,
      image: saved.image,
      materials: saved.materials,
      resolved_materials: saved.resolved_materials,
    })
  }
  return (
    <Tabs
      key={item.id}
      defaultValue="setting"
      className="flex min-h-0 min-w-0 flex-1 flex-col gap-0"
    >
      <div className="flex shrink-0 flex-wrap items-center gap-3 border-b px-3 py-1">
        {item.image && (
          <img
            src={projectFileAssetURL(projectId, item.image.image_path)}
            alt=""
            className="size-8 rounded object-cover"
          />
        )}
        <TabsList variant="line">
          <TabsTrigger value="setting">{t('lore.materials.settingTab')}</TabsTrigger>
          <TabsTrigger value="materials">
            {t('lore.materials.tab', { count: item.resolved_materials?.length ?? 0 })}
          </TabsTrigger>
        </TabsList>
      </div>
      <TabsContent value="setting" className="flex min-h-0 flex-1 flex-col">
        {children}
      </TabsContent>
      <TabsContent value="materials" className="min-h-0 flex-1 overflow-y-auto">
        <LoreMaterialsPanel
          key={`${projectId}:${item.id}`}
          projectId={projectId}
          item={item}
          onChanged={applyMedia}
          onInspectMaterial={onInspectMaterial}
        />
      </TabsContent>
    </Tabs>
  )
}
