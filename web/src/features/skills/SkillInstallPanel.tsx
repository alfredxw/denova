import { useState } from 'react'
import { Download } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { ImportDialog } from '@/features/market/ImportDialog'
import { getSkills } from '@/lib/api'
import type {
  SkillCatalogTarget,
  SkillInstallResult,
  SkillScope,
  SkillScopeInfo,
} from '@/lib/api'

interface SkillInstallPanelProps {
  target: SkillCatalogTarget
  scopes: SkillScopeInfo[]
  defaultScope: SkillScope
  onInstalled: (result: SkillInstallResult) => void | Promise<void>
}

/** Domain navigation supplies the Project; exchange owns preview and commit. */
export function SkillInstallPanel({
  target,
  scopes,
  defaultScope,
  onInstalled,
}: SkillInstallPanelProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(true)
  return (
    <div className="min-h-0 flex-1 overflow-auto">
      <Empty>
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <Download />
          </EmptyMedia>
          <EmptyTitle>{t('market.import.title')}</EmptyTitle>
          <EmptyDescription>{t('market.import.help')}</EmptyDescription>
        </EmptyHeader>
        <Button disabled={!scopes.length} onClick={() => setOpen(true)}>
          {t('market.import.title')}
        </Button>
      </Empty>
      {open && (
        <ImportDialog
          initialScope={defaultScope === 'workspace' ? 'workspace' : 'user'}
          projectID={target.kind === 'project' ? target.projectId : undefined}
          onClose={() => setOpen(false)}
          onInstalled={async (installation) => {
            const snapshot = await getSkills(target)
            const installed = snapshot.skills.filter((skill) =>
              installation.bindings.some(
                (binding) =>
                  binding.local.kind === 'skill' &&
                  binding.local.id === skill.name &&
                  binding.local.scope === skill.scope,
              ),
            )
            await onInstalled({ installed })
          }}
        />
      )}
    </div>
  )
}
