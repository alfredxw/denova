import { describe, expect, it } from 'vitest'
import { extensionEntries } from './extension-directory'
import type { CatalogEntry, DevelopmentSource, Manifest } from './api'

const manifest: Manifest = { id: 'test.game', version: '1.0.0', apiMajor: 1, name: { 'zh-CN': '游戏', 'en-US': 'Game' }, permissions: { required: [], optional: [] } }
const source: DevelopmentSource = { developmentId: 'source-a', projectId: 'project-a', relativePath: '.', kind: 'game', projectName: 'Source', manifest }
const installed: CatalogEntry = { kind: 'game', id: manifest.id, enabled: true, currentRelease: 'release', grants: [], releases: [{ ref: { package: { kind: 'game', id: manifest.id }, releaseId: 'release' }, manifest, digest: 'abc', installedAt: '' }] }

describe('extension directory', () => {
  it('lists installed releases and associates source without replacing installed metadata', () => {
    const plugin = { ...installed, kind: 'plugin' as const }
    const draft = { ...source, manifest: { ...manifest, version: '2.0.0' } }
    expect(extensionEntries([draft], [installed, plugin])).toEqual([
      { key: 'installed:game:test.game', kind: 'game', sources: [draft], manifest, installed },
      { key: 'installed:plugin:test.game', kind: 'plugin', sources: [], installed: plugin, manifest },
    ])
  })

  it('keeps multiple source links on one installed item and omits uninstalled drafts', () => {
    const broken = { ...source, developmentId: 'broken', manifest: undefined, messageKey: 'platform.sourceInvalid' }
    const copy = { ...source, developmentId: 'copy', projectId: 'project-b' }
    const entries = extensionEntries([source, copy, broken], [installed])
    expect(entries).toEqual([{ key: 'installed:game:test.game', kind: 'game', sources: [source, copy], manifest, installed }])
    expect(extensionEntries([source, copy, broken], [{ ...installed, removed: true }])).toEqual([])
    expect(extensionEntries([source], [])).toEqual([])
  })
})
