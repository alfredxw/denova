import { expect, test } from '../support/fixtures'
import en from '../../src/i18n/locales/en-US/market'
import zh from '../../src/i18n/locales/zh-CN/market'
import type { Installation, ResourceKind } from '../../src/features/market/api'

for (const language of ['en-US', 'zh-CN']) {
  for (const theme of ['dark', 'light']) {
    test(`installed packages remain usable with long content in ${language} ${theme}`, async ({ page, request }) => {
      const labels = language === 'en-US' ? en : zh
      await request.patch('/api/settings', { data: { layer: 'user', changes: { language, theme } } })
      await page.addInitScript((locale) => {
        localStorage.setItem('nova:mode', 'market')
        localStorage.setItem('nova.locale.configured', locale)
      }, language)
      const kinds: ResourceKind[] = ['lore.collection', 'game.openings', 'preset.narrative', 'preset.image', 'skill', 'preset.actor_state', 'preset.rules', 'extension.game']
      const installed: Installation = {
        installation_id: 'mixed-package',
        package: { id: 'shared', name: language === 'en-US' ? 'Shared writing and game resources with a long package name' : '写作与游戏共用资源包 · 包含世界资料、开场、创作方案与游戏扩展', version: '2.1.0' },
        source: { kind: 'github', url: `https://github.com/author/${'long-repository-name-'.repeat(8)}` },
        tracking: 'tracked', update_mode: 'manual', local_state: 'modified', remote_state: 'update_available',
        updated_at: '2026-09-30T00:00:00Z',
        bindings: kinds.map((kind, index) => ({
          resource_id: `resource-${index}`, source_digest: 'original',
          local: { kind, scope: 'global', id: index === 0 ? 'long-resource-id-'.repeat(20) : `resource-${index}` },
          ownership: index === 7 ? 'reference' : 'owned', upstream_removed: index === 0,
        })),
      }
      const localFile: Installation = {
        ...installed, installation_id: 'local-file', package: { id: 'file', name: 'Local package' },
        source: { kind: 'file', filename: `${'local-package-'.repeat(15)}.zip` },
        remote_state: '', local_state: 'unchanged', bindings: [installed.bindings[4]],
      }
      let installations = [installed, localFile]
      let changedPolicy = '', checkedInstallation = ''
      await page.route('**/api/resource-market/catalog*', route => route.fulfill({ json: { schema_version: 1, entries: [] } }))
      await page.route('**/api/resource-exchange/installations', route => route.fulfill({ json: installations }))
      await page.route('**/api/resource-exchange/installations/mixed-package/policy', route => {
        changedPolicy = route.request().postDataJSON().update_mode
        installed.update_mode = changedPolicy
        return route.fulfill({ json: installed })
      })
      await page.route('**/api/resource-exchange/installations/mixed-package/check', route => {
        checkedInstallation = route.request().url()
        return route.fulfill({ json: {
          preview_id: 'installed-layout-preview', source: installed.source, candidates: [{
            candidate_id: 'mixed', package: installed.package, format: 'denova.resource-pack',
            resources: [{ id: 'lore', kind: 'lore.collection', name: 'World', path: 'lore.json', digest: 'new' }],
          }],
        } })
      })
      await page.route('**/api/resource-exchange/previews/installed-layout-preview', route => route.fulfill({ json: {} }))
      await page.route('**/api/resource-exchange/installations/mixed-package/detach', route => {
        installed.tracking = 'detached'
        return route.fulfill({ json: installed })
      })
      await page.setViewportSize({ width: 1440, height: 1000 })
      await page.goto('/')
      const market = page.getByTestId('resource-market')
      const navigation = page.getByRole('navigation', { name: labels['market.navigation'], exact: true })
      await navigation.getByRole('button', { name: labels['market.acquired'], exact: true }).click()
      const card = market.getByTestId('market-installation-card').filter({ hasText: installed.package.name })
      const trigger = card.getByRole('button', { name: installed.package.name, exact: true })
      // The entire header, including its padding and status area, expands the package.
      await card.locator('[data-slot="card-header"]').click({ position: { x: 12, y: 12 } })
      await expect(trigger).toHaveAttribute('aria-expanded', 'true')
      await expect(card.getByRole('listitem')).toHaveCount(kinds.length)
      await expect(card.getByText(labels['market.states.upstream_removed'], { exact: true })).toBeVisible()
      await expect(card.getByText(labels['market.actions.reference'], { exact: true })).toBeVisible()
      await trigger.press('Enter')
      await expect(trigger).toHaveAttribute('aria-expanded', 'false')
      await trigger.press('Space')
      await expect(trigger).toHaveAttribute('aria-expanded', 'true')

      for (const width of [1440, 768, 390]) {
        await page.setViewportSize({ width, height: 1000 })
        await card.scrollIntoViewIfNeeded()
        expect(await card.evaluate(element => {
          const bounds = element.getBoundingClientRect()
          const sections = [element, ...element.querySelectorAll('[data-slot="card-header"], [data-slot="card-content"], [data-slot="card-footer"], [data-slot="field"]')]
          return bounds.left >= 0 && bounds.right <= window.innerWidth
            && sections.every(section => section.scrollWidth <= section.clientWidth)
        })).toBe(true)
        if (width !== 768) await card.screenshot({ path: `test-results/market-installed-${language}-${theme}-${width}.png` })
      }
      const policy = card.getByRole('combobox', { name: labels['market.updatePolicy'] })
      await policy.click()
      await expect(page.getByRole('option', { name: labels['market.policy.auto_apply'], exact: true })).toHaveCount(0)
      await page.getByRole('option', { name: labels['market.policy.notify'], exact: true }).click()
      await expect.poll(() => changedPolicy).toBe('notify')
      await expect(policy).toContainText(labels['market.policy.notify'])

      await card.getByRole('button', { name: labels['market.checkUpdate'], exact: true }).click()
      await expect(page.getByRole('dialog')).toBeVisible()
      expect(checkedInstallation).toContain('/mixed-package/check')
      await page.getByRole('dialog').press('Escape')
      await expect(page.getByRole('dialog')).toBeHidden()

      await expect(card.getByRole('button', { name: labels['market.export.title'], exact: true })).toBeVisible()
      await expect(card.getByRole('button', { name: labels['market.backups.title'], exact: true })).toBeVisible()
      await card.getByRole('button', { name: labels['market.detach'], exact: true }).click()
      await expect(card.getByText(labels['market.states.detached'], { exact: true })).toBeVisible()
      await expect(policy).toHaveCount(0)
      await expect(card.getByRole('button', { name: labels['market.checkUpdate'], exact: true })).toHaveCount(0)
      await expect(card.getByRole('button', { name: labels['market.detach'], exact: true })).toHaveCount(0)

      const fileCard = market.getByTestId('market-installation-card').filter({ hasText: localFile.package.name })
      await fileCard.getByRole('button', { name: localFile.package.name, exact: true }).click()
      await expect(trigger).toHaveAttribute('aria-expanded', 'false')
      await expect(fileCard.getByText(localFile.source.filename!, { exact: true })).toBeVisible()
      await expect(fileCard.getByRole('combobox')).toHaveCount(0)
      await expect(fileCard.getByRole('button', { name: labels['market.checkUpdate'], exact: true })).toHaveCount(0)

      installations = []
      await market.getByRole('button', { name: labels['market.checkUpdates'], exact: true }).click()
      await expect(market.getByText(labels['market.acquiredEmpty'], { exact: true })).toBeVisible()
      expect(await market.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true)
      await market.getByRole('button', { name: labels['market.discover'], exact: true }).click()
      await expect(market.getByRole('heading', { name: labels['market.discover'], exact: true })).toBeVisible()
    })
  }
}
