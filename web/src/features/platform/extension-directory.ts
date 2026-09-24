import type { CatalogEntry, DevelopmentSource, Manifest, PackageKind } from './api'

export interface ExtensionEntry {
  key: string
  kind: PackageKind
  sources: DevelopmentSource[]
  installed: CatalogEntry
  manifest?: Manifest
}

/** Installed releases own the directory; source Projects are optional navigation links. */
export function extensionEntries(sources: DevelopmentSource[], catalog: CatalogEntry[]): ExtensionEntry[] {
  return catalog.filter(item => !item.removed).map(installed => ({
    key: `installed:${installed.kind}:${installed.id}`,
    kind: installed.kind,
    installed,
    manifest: installed.releases.find(release => release.ref.releaseId === installed.currentRelease)?.manifest,
    sources: sources.filter(source => source.kind === installed.kind && source.manifest?.id === installed.id),
  }))
}

export function sourceManifestPath(source: DevelopmentSource) {
  const prefix = source.relativePath === '.' ? '' : source.relativePath + '/'
  return prefix + (source.kind === 'game' ? 'denova.game.json' : 'denova.plugin.json')
}
