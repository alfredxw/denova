import { useState } from 'react'
import { Download, FileCode2, Link2, Loader2, Search, Upload } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { EmptyState } from '@/components/common/EmptyState'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { FormField } from '@/components/forms/form-field'
import { FormSectionHeader } from '@/components/forms/form-section-header'
import { FieldLegend, FieldSet } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { installSkillRemote, installSkillZip, previewSkillRemoteInstall, previewSkillZipInstall } from '@/lib/api'
import type { SkillInstallCandidate, SkillInstallResult, SkillScope, SkillScopeInfo } from '@/lib/api'
import type { SkillInstallSource } from './skill-utils'
import { isInstallableCandidate, requireInstallFile, scopeLabel } from './skill-utils'

interface SkillInstallPanelProps {
  /** 可写 scope 列表；为空时展示不可写提示 */
  scopes: SkillScopeInfo[]
  defaultScope: SkillScope
  onInstalled: (result: SkillInstallResult) => void | Promise<void>
}

/** 导入 Skill 整页表单：自持来源/候选状态，扫描后只安装勾选的条目。 */
export function SkillInstallPanel({ scopes, defaultScope, onInstalled }: SkillInstallPanelProps) {
  const { t } = useTranslation()
  const [source, setSource] = useState<SkillInstallSource>('remote')
  const [scope, setScope] = useState<SkillScope>(defaultScope)
  const [file, setFile] = useState<File | null>(null)
  const [remoteURL, setRemoteURL] = useState('')
  const [remoteRef, setRemoteRef] = useState('')
  const [remoteSubdir, setRemoteSubdir] = useState('')
  const [candidates, setCandidates] = useState<SkillInstallCandidate[]>([])
  const [selectedIds, setSelectedIds] = useState<string[]>([])
  const [message, setMessage] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const installable = candidates.filter(isInstallableCandidate)
  const selectedInstallable = selectedIds.filter((id) => installable.some((candidate) => candidate.id === id))
  const canPreview = source === 'zip' ? Boolean(file) : remoteURL.trim() !== ''
  const resetScan = () => {
    setCandidates([])
    setSelectedIds([])
    setMessage(null)
  }
  const toggleSelected = (id: string, checked: boolean) => {
    if (checked) {
      setSelectedIds((current) => current.includes(id) ? current : [...current, id])
      return
    }
    setSelectedIds((current) => current.filter((item) => item !== id))
  }
  const selectAll = () => setSelectedIds(installable.map((candidate) => candidate.id))

  const applyInstallPreview = (preview: SkillInstallCandidate[]) => {
    setCandidates(preview)
    const nextInstallable = preview.filter(isInstallableCandidate)
    setSelectedIds(nextInstallable.length === 1 ? [nextInstallable[0].id] : [])
    setMessage(preview.length === 0 ? t('skills.install.noCandidates') : null)
  }

  const onPreview = async () => {
    if (scopes.length === 0) {
      setError(t('skills.create.noWritableScope'))
      return
    }
    setSaving(true)
    setError(null)
    setMessage(null)
    try {
      const preview = source === 'zip'
        ? await previewSkillZipInstall(requireInstallFile(file, t), scope)
        : await previewSkillRemoteInstall({
            url: remoteURL.trim(),
            ref: remoteRef.trim(),
            subdir: remoteSubdir.trim(),
            scope,
          })
      applyInstallPreview(preview.candidates || [])
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const onInstall = async () => {
    const candidateIds = selectedIds.filter((id) => candidates.some((candidate) => candidate.id === id && isInstallableCandidate(candidate)))
    if (candidateIds.length === 0) {
      setError(t('skills.install.selectRequired'))
      return
    }
    setSaving(true)
    setError(null)
    setMessage(null)
    try {
      const result = source === 'zip'
        ? await installSkillZip(requireInstallFile(file, t), scope, candidateIds)
        : await installSkillRemote({
            url: remoteURL.trim(),
            ref: remoteRef.trim(),
            subdir: remoteSubdir.trim(),
            scope,
            candidateIds,
          })
      await onInstalled(result)
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <div className="mx-auto flex w-full min-w-0 max-w-5xl flex-col gap-5 px-4 py-5 sm:px-6">
        <section className="border-b border-[var(--nova-border)] pb-4">
          <div className="flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
              <Download className="h-4 w-4 text-[var(--nova-text-muted)]" />
            </div>
            <div className="min-w-0">
              <h1 className="truncate text-sm font-semibold">{t('skills.install.title')}</h1>
              <div className="mt-1 text-[11px] text-[var(--nova-text-faint)]">{t('skills.install.subtitle')}</div>
            </div>
          </div>
        </section>

        {error && <InlineErrorNotice message={error} title={t('skills.error')} />}

        {scopes.length === 0 ? (
          <EmptyState
            variant="compact"
            icon={FileCode2}
            title={t('skills.create.noWritableScope')}
            className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-faint)]"
          />
        ) : (
          <>
            <section className="flex flex-col gap-3 border-b border-[var(--nova-border)] pb-5">
              <FormSectionHeader icon={Search} title={t('skills.install.section.source')} />
              <div className="grid gap-3 md:grid-cols-[minmax(0,16rem)_minmax(0,1fr)]">
                <FieldSet className="min-w-0 gap-1.5 text-xs">
                  <FieldLegend variant="label" className="mb-0 text-[11px] font-normal text-muted-foreground">
                    {t('skills.create.scope')}
                  </FieldLegend>
                  <ToggleGroup
                    type="single"
                    value={scope}
                    onValueChange={(nextScope) => {
                      if (!nextScope) return
                      setScope(nextScope as SkillScope)
                      resetScan()
                    }}
                    variant="outline"
                    size="sm"
                    spacing={1}
                    aria-label={t('skills.create.scope')}
                    className="w-full"
                  >
                    {scopes.map((item) => (
                      <ToggleGroupItem
                        key={item.scope}
                        value={item.scope}
                        className={`nova-nav-item h-8 flex-1 rounded-[var(--nova-radius)] px-2 ${scope === item.scope ? 'is-active' : 'bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]'}`}
                      >
                        {scopeLabel(item.scope, t)}
                      </ToggleGroupItem>
                    ))}
                  </ToggleGroup>
                </FieldSet>
                <FieldSet className="min-w-0 gap-1.5 text-xs">
                  <FieldLegend variant="label" className="mb-0 text-[11px] font-normal text-muted-foreground">
                    {t('skills.install.source')}
                  </FieldLegend>
                  <ToggleGroup
                    type="single"
                    value={source}
                    onValueChange={(nextSource) => {
                      if (!nextSource) return
                      setSource(nextSource as SkillInstallSource)
                      resetScan()
                      setError(null)
                    }}
                    variant="outline"
                    size="sm"
                    spacing={1}
                    aria-label={t('skills.install.source')}
                    className="grid w-full grid-cols-2"
                  >
                    <ToggleGroupItem
                      value="remote"
                      className={`nova-nav-item h-8 min-w-0 rounded-[var(--nova-radius)] px-2 ${source === 'remote' ? 'is-active' : 'bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]'}`}
                    >
                      <Link2 data-icon="inline-start" />
                      {t('skills.install.remote')}
                    </ToggleGroupItem>
                    <ToggleGroupItem
                      value="zip"
                      className={`nova-nav-item h-8 min-w-0 rounded-[var(--nova-radius)] px-2 ${source === 'zip' ? 'is-active' : 'bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]'}`}
                    >
                      <Upload data-icon="inline-start" />
                      {t('skills.install.zip')}
                    </ToggleGroupItem>
                  </ToggleGroup>
                </FieldSet>
              </div>

              {source === 'remote' ? (
                <div className="grid gap-3 md:grid-cols-[minmax(0,1.4fr)_minmax(0,0.7fr)_minmax(0,0.9fr)]">
                  <FormField label={t('skills.install.remoteUrl')}>
                    <Input
                      value={remoteURL}
                      onChange={(event) => setRemoteURL(event.target.value)}
                      aria-label={t('skills.install.remoteUrl')}
                      placeholder="owner/repo or https://github.com/owner/repo/tree/main/skills"
                      className="nova-field h-8 w-full rounded-[var(--nova-radius)] border px-2.5 font-mono outline-none"
                    />
                  </FormField>
                  <FormField label={t('skills.install.ref')}>
                    <Input
                      value={remoteRef}
                      onChange={(event) => setRemoteRef(event.target.value)}
                      aria-label={t('skills.install.ref')}
                      placeholder="main"
                      className="nova-field h-8 w-full rounded-[var(--nova-radius)] border px-2.5 font-mono outline-none"
                    />
                  </FormField>
                  <FormField label={t('skills.install.subdir')}>
                    <Input
                      value={remoteSubdir}
                      onChange={(event) => setRemoteSubdir(event.target.value)}
                      aria-label={t('skills.install.subdir')}
                      placeholder="skills/foo"
                      className="nova-field h-8 w-full rounded-[var(--nova-radius)] border px-2.5 font-mono outline-none"
                    />
                  </FormField>
                </div>
              ) : (
                <FormField label={t('skills.install.zipFile')}>
                  <Input
                    type="file"
                    accept=".zip,application/zip"
                    aria-label={t('skills.install.zipFile')}
                    onChange={(event) => {
                      setFile(event.target.files?.[0] || null)
                      resetScan()
                    }}
                    className="nova-field h-8 w-full rounded-[var(--nova-radius)] border px-2.5 py-1 outline-none"
                  />
                  <div className="mt-1 truncate text-[11px] text-[var(--nova-text-faint)]">{file?.name || t('skills.install.zipHint')}</div>
                </FormField>
              )}

              <div className="flex flex-wrap gap-2">
                <button
                  type="button"
                  onClick={() => void onPreview()}
                  disabled={saving || !canPreview}
                  className="nova-nav-item inline-flex h-8 items-center justify-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-3 disabled:cursor-not-allowed disabled:opacity-45"
                >
                  {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Search className="h-3.5 w-3.5" />}
                  {t('skills.install.scan')}
                </button>
                {message && <span className="inline-flex min-h-8 items-center text-[11px] text-[var(--nova-success)]">{message}</span>}
              </div>
            </section>

            <section className="flex flex-col gap-3 pb-5">
              <div className="flex items-center gap-2">
                <FormSectionHeader icon={FileCode2} title={t('skills.install.section.candidates')} />
                {installable.length > 1 && (
                  <button
                    type="button"
                    onClick={selectAll}
                    className="nova-nav-item ml-auto inline-flex h-7 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 text-[11px]"
                  >
                    {t('skills.install.selectAll')}
                  </button>
                )}
              </div>

              {candidates.length === 0 ? (
                <EmptyState
                  variant="dashed"
                  icon={Search}
                  title={t('skills.install.scanFirst')}
                  className="min-h-28 px-3 py-6 text-[var(--nova-text-faint)]"
                />
              ) : (
                <div className="flex flex-col gap-2">
                  {candidates.map((candidate) => {
                    const installableCandidate = isInstallableCandidate(candidate)
                    const checked = selectedIds.includes(candidate.id)
                    return (
                      <label
                        key={candidate.id}
                        className={`nova-nav-item flex min-h-16 items-start gap-3 rounded-[var(--nova-radius)] border px-3 py-2 ${checked ? 'is-active border-[var(--nova-border)]' : 'border-transparent bg-[var(--nova-surface)] hover:border-[var(--nova-border)]'} ${installableCandidate ? 'cursor-pointer' : 'cursor-not-allowed opacity-70'}`}
                      >
                        <input
                          type="checkbox"
                          checked={checked}
                          disabled={!installableCandidate}
                          onChange={(event) => toggleSelected(candidate.id, event.target.checked)}
                          className="mt-1 h-3.5 w-3.5 shrink-0"
                        />
                        <span className="min-w-0 flex-1">
                          <span className="flex items-center gap-2">
                            <span className="min-w-0 truncate font-mono text-xs text-[var(--nova-text)]">/{candidate.name || candidate.source_path}</span>
                            {candidate.conflict && <span className="rounded bg-[var(--nova-warning-bg)] px-1.5 py-0.5 text-[10px] text-[var(--nova-warning)]">{t('skills.install.conflict')}</span>}
                            {candidate.invalid_reason && <span className="rounded bg-[var(--nova-danger-bg)] px-1.5 py-0.5 text-[10px] text-[var(--nova-danger)]">{t('skills.install.invalid')}</span>}
                          </span>
                          <span className="mt-1 block truncate font-mono text-[10px] text-[var(--nova-text-faint)]">{candidate.source_path}</span>
                          <span className="mt-1 line-clamp-2 block text-[11px] leading-4 text-[var(--nova-text-faint)]">
                            {candidate.invalid_reason || candidate.description || t('skills.install.noDescription')}
                          </span>
                        </span>
                      </label>
                    )
                  })}
                </div>
              )}

              <button
                type="button"
                onClick={() => void onInstall()}
                disabled={saving || selectedInstallable.length === 0}
                className="nova-nav-item inline-flex h-8 items-center justify-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-3 disabled:cursor-not-allowed disabled:opacity-45"
              >
                {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Download className="h-3.5 w-3.5" />}
                {t('skills.install.submit', { count: selectedInstallable.length })}
              </button>
            </section>
          </>
        )}
      </div>
    </div>
  )
}
