import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Search } from 'lucide-react'
import { Accordion } from '@/components/ui/accordion'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Empty, EmptyHeader, EmptyTitle } from '@/components/ui/empty'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
import { resourceKinds, type ExportResource } from './api'
import { ResourceSelectionGroup } from './ResourceSelectionGroup'

export function ExportResourcePicker({
  choices,
  selected,
  busy,
  onCheckedChange,
}: {
  choices: ExportResource[]
  selected: ExportResource[]
  busy: boolean
  onCheckedChange: (resources: ExportResource[], checked: boolean) => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const [query, setQuery] = useState('')
  const [openGroups, setOpenGroups] = useState<string[]>()
  const search = query.trim().toLowerCase()
  const groups = resourceKinds
    .map((kind) => ({
      kind,
      label: t(`market.kinds.${kind}`),
      items: choices.filter(
        (resource) =>
          resource.local.kind === kind &&
          `${resource.name} ${resource.description || ''} ${t(`market.kinds.${kind}`)}`
            .toLowerCase()
            .includes(search),
      ),
    }))
    .filter((group) => group.items.length > 0)

  return (
    <FieldGroup className="min-w-0 gap-3">
      <InputGroup>
        <InputGroupAddon>
          <Search />
        </InputGroupAddon>
        <InputGroupInput
          aria-label={t('market.export.search')}
          placeholder={t('market.export.search')}
          value={query}
          onChange={(event) => {
            setQuery(event.target.value)
            // Reveal search matches, then return to the compact category list when cleared.
            setOpenGroups(undefined)
          }}
        />
      </InputGroup>
      <div className="flex items-center justify-between gap-2">
        <FieldDescription role="status">
          {t('market.export.selected', { count: selected.length })}
        </FieldDescription>
        <Button
          variant="ghost"
          size="sm"
          disabled={busy || selected.length === 0}
          onClick={() => onCheckedChange(selected, false)}
        >
          {t('market.export.clearSelection')}
        </Button>
      </div>
      <div
        className="max-h-[min(24rem,45dvh)] overflow-y-auto"
        aria-busy={busy}
      >
        {groups.length > 0 && (
          <Accordion
            type="multiple"
            className="overflow-hidden rounded-xl border"
            value={openGroups ?? (search ? groups.map(({ kind }) => kind) : [])}
            onValueChange={setOpenGroups}
          >
            {groups.map(({ kind, label, items }) => {
              const count = items.filter((resource) =>
                selected.includes(resource),
              ).length
              return (
                <ResourceSelectionGroup
                  key={kind}
                  value={kind}
                  label={label}
                  count={count}
                  total={items.length}
                  selectionLabel={t(
                    search
                      ? 'market.export.selectMatches'
                      : 'market.export.selectGroup',
                    { kind: label },
                  )}
                  disabled={busy}
                  onCheckedChange={(checked) => onCheckedChange(items, checked)}
                >
                  <FieldGroup className="gap-0">
                    {items.map((resource) => {
                      const resourceID = `${id}-${choices.indexOf(resource)}`
                      return (
                        <Field
                          key={resourceID}
                          orientation="horizontal"
                          className="items-start gap-3 py-2"
                          data-disabled={busy}
                        >
                          <Checkbox
                            className="mt-0.5"
                            id={resourceID}
                            aria-label={resource.name}
                            checked={selected.includes(resource)}
                            disabled={busy}
                            onCheckedChange={(checked) =>
                              onCheckedChange([resource], checked === true)
                            }
                          />
                          <FieldLabel
                            htmlFor={resourceID}
                            className="min-w-0 flex-1"
                          >
                            <FieldContent className="min-w-0">
                              <span className="[overflow-wrap:anywhere]">
                                {resource.name}
                              </span>
                              {resource.description && (
                                <FieldDescription className="line-clamp-2 [overflow-wrap:anywhere]">
                                  {resource.description}
                                </FieldDescription>
                              )}
                            </FieldContent>
                          </FieldLabel>
                        </Field>
                      )
                    })}
                  </FieldGroup>
                </ResourceSelectionGroup>
              )
            })}
          </Accordion>
        )}
        {groups.length === 0 && (
          <Empty>
            <EmptyHeader>
              <EmptyTitle>
                {t(
                  busy
                    ? 'market.working'
                    : choices.length === 0
                      ? 'market.export.empty'
                      : 'market.contents.noMatches',
                )}
              </EmptyTitle>
            </EmptyHeader>
          </Empty>
        )}
      </div>
    </FieldGroup>
  )
}
