import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Search } from 'lucide-react'
import { Accordion } from '@/components/ui/accordion'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Empty, EmptyHeader, EmptyTitle } from '@/components/ui/empty'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldTitle,
} from '@/components/ui/field'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
import { dependencySelection, resourceKinds, type PackagePreview } from './api'
import { ResourcePreview } from './ResourcePreview'
import { ResourceSelectionGroup } from './ResourceSelectionGroup'
import { cn } from '@/lib/utils'

export function ImportResourcePicker({
  previewID,
  candidate,
  selected,
  onChange,
}: {
  previewID: string
  candidate: PackagePreview
  selected: string[]
  onChange: (ids: string[]) => void
}) {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const chosen = dependencySelection(candidate.resources, selected)
  // Dependencies stay required even when they were also explicitly selected.
  const required = new Set(
    dependencySelection(
      candidate.resources,
      candidate.resources
        .filter((r) => chosen.includes(r.id))
        .flatMap((r) => r.requires || []),
    ),
  )
  const search = query.trim().toLocaleLowerCase()
  const groups = resourceKinds
    .map((kind) => {
      // Lore and openings are acquired as collections; individual rows are only previews.
      const selectAsGroup = kind === 'lore.item' || kind === 'game.opening'
      const label = t(
        kind === 'lore.item'
          ? 'market.contents.lore'
          : kind === 'game.opening'
            ? 'market.contents.openings'
            : `market.kinds.${kind}`,
      )
      const items = candidate.resources.filter((r) => r.kind === kind)
      const matches = items.filter((r) =>
        `${r.name} ${r.description || ''} ${label} ${t(`market.kinds.${kind}`)}`
          .toLocaleLowerCase()
          .includes(search),
      )
      // Searching narrows previews, never the contents of a collection selection.
      return {
        kind,
        label,
        selectAsGroup,
        items: selectAsGroup ? items : matches,
        matches,
      }
    })
    .filter(({ matches }) => matches.length > 0)
  const select = (ids: string[], checked: boolean) =>
    onChange(
      checked
        ? [...new Set([...selected, ...ids])]
        : selected.filter((id) => !ids.includes(id)),
    )
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
          onChange={(event) => setQuery(event.target.value)}
        />
      </InputGroup>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <FieldDescription role="status">
          {t('market.contents.selected', {
            count: chosen.length,
            total: candidate.resources.length,
          })}
        </FieldDescription>
        <Button
          variant="ghost"
          size="sm"
          disabled={!selected.length}
          onClick={() => onChange([])}
        >
          {t('market.export.clearSelection')}
        </Button>
      </div>
      {groups.some(({ selectAsGroup }) => selectAsGroup) && (
        <FieldDescription>{t('market.contents.groupedHelp')}</FieldDescription>
      )}
      {groups.length > 0 && (
        <Accordion
          type="multiple"
          defaultValue={resourceKinds.filter(
            (kind) => kind !== 'lore.item' && kind !== 'game.opening',
          )}
          className="overflow-hidden rounded-xl border"
        >
          {groups.map(({ kind, label, selectAsGroup, items, matches }) => {
            const count = items.filter((r) => chosen.includes(r.id)).length
            return (
              <ResourceSelectionGroup
                key={kind}
                value={kind}
                label={label}
                count={count}
                total={items.length}
                selectionLabel={t(
                  search && !selectAsGroup
                    ? 'market.export.selectMatches'
                    : 'market.export.selectGroup',
                  { kind: label },
                )}
                onCheckedChange={(checked) =>
                  select(
                    items.map((r) => r.id),
                    checked,
                  )
                }
              >
                <FieldGroup className="gap-0">
                  {matches.map((resource) => (
                    <Field
                      key={resource.id}
                      orientation="horizontal"
                      className={cn(
                        'items-start gap-3 py-2',
                        selectAsGroup && 'pl-7',
                      )}
                    >
                      {!selectAsGroup && (
                        <Checkbox
                          className="mt-0.5"
                          aria-label={resource.name || resource.id}
                          checked={chosen.includes(resource.id)}
                          disabled={required.has(resource.id)}
                          onCheckedChange={(checked) =>
                            select([resource.id], checked === true)
                          }
                        />
                      )}
                      <FieldContent className="min-w-0">
                        <FieldTitle className="max-w-full [overflow-wrap:anywhere]">
                          {resource.name || resource.id}
                        </FieldTitle>
                        {resource.description && (
                          <FieldDescription className="line-clamp-2 [overflow-wrap:anywhere]">
                            {resource.description}
                          </FieldDescription>
                        )}
                        {required.has(resource.id) && (
                          <FieldDescription>
                            {t('market.contents.required')}
                          </FieldDescription>
                        )}
                      </FieldContent>
                      <Dialog>
                        <DialogTrigger asChild>
                          <Button
                            variant="ghost"
                            size="sm"
                            aria-label={t('market.contents.previewResource', {
                              name: resource.name || resource.id,
                            })}
                          >
                            {t('market.contents.preview')}
                          </Button>
                        </DialogTrigger>
                        <DialogContent className="max-h-[85dvh] overflow-y-auto">
                          <DialogHeader className="pr-8">
                            <DialogTitle className="[overflow-wrap:anywhere]">
                              {resource.name || resource.id}
                            </DialogTitle>
                            <DialogDescription className="[overflow-wrap:anywhere]">
                              {resource.description ||
                                t(`market.kinds.${resource.kind}`)}
                            </DialogDescription>
                          </DialogHeader>
                          <ResourcePreview
                            previewID={previewID}
                            candidateID={candidate.candidate_id}
                            resourceID={resource.id}
                          />
                        </DialogContent>
                      </Dialog>
                    </Field>
                  ))}
                </FieldGroup>
              </ResourceSelectionGroup>
            )
          })}
        </Accordion>
      )}
      {!groups.length && (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>{t('market.contents.noMatches')}</EmptyTitle>
          </EmptyHeader>
        </Empty>
      )}
      <FieldDescription>{t('market.import.dependencies')}</FieldDescription>
    </FieldGroup>
  )
}
