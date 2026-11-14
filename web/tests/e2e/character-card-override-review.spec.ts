import type { APIRequestContext } from '@playwright/test'
import { expect, test } from '../support/fixtures'
import { createAndOpenBook, getProjectLoreItems, updateLoreItemContent } from '../support/api'

function tavernCardJSON(name: string): string {
  return JSON.stringify({
    spec: 'chara_card_v2',
    spec_version: '2.0',
    name,
    description: 'A review regression character.',
    character_book: {
      name: `${name} worldbook`,
      entries: [
        { id: 1, keys: [name.toLowerCase()], comment: 'note', content: `Incoming detailed ${name} entry`, enabled: true },
      ],
    },
  })
}

interface PendingOverride {
  name: string
  existing_preview: string
  incoming_preview: string
  suggested: 'override' | 'keep'
  existing_id: string
  existing_version: string
}

async function importWithPending(request: APIRequestContext, projectId: string): Promise<{ pending: PendingOverride[]; reviewPlanID: string }> {
  const response = await request.post(`/api/projects/${projectId}/book/import-character-card`, {
    multipart: {
      file: { name: 'hero.json', mimeType: 'application/json', buffer: Buffer.from(tavernCardJSON('Hero')) },
      lore_classification: 'heuristic',
    },
  })
  expect(response.ok(), await response.text()).toBe(true)
  const body = await response.json() as { pending_overrides?: PendingOverride[]; review_plan_id?: string }
  expect(body.pending_overrides, 'import should surface same-name entries for review').toBeTruthy()
  expect(body.review_plan_id, 'import should hold a review plan').toBeTruthy()
  return { pending: body.pending_overrides!, reviewPlanID: body.review_plan_id! }
}

async function confirmOverrides(
  request: APIRequestContext,
  projectId: string,
  reviewPlanID: string,
  overrideReview: Array<{ name: string; existing_id?: string; existing_version?: string; choice: 'override' | 'keep' }>,
): Promise<{ ok: boolean; body: any }> {
  const response = await request.post(`/api/projects/${projectId}/book/import-character-card`, {
    multipart: {
      file: { name: 'hero.json', mimeType: 'application/json', buffer: Buffer.from(tavernCardJSON('Hero')) },
      lore_classification: 'heuristic',
      confirm_overrides: '1',
      review_plan_id: reviewPlanID,
      override_review: JSON.stringify(overrideReview),
    },
  })
  return { ok: response.ok(), body: await response.json() }
}

test('confirming a stale preview never overwrites an entry edited after the preview', async ({ request }) => {
  const book = await createAndOpenBook(request, 'Override review book')
  const created = await request.post(`/api/projects/${book.projectId}/book/lore/items`, {
    data: { name: 'Hero', type: 'character', content: 'Short local description' },
  })
  expect(created.ok(), await created.text()).toBe(true)

  const { pending, reviewPlanID } = await importWithPending(request, book.projectId)
  const dup = pending[0]
  expect(dup.name).toBe('Hero')
  expect(dup.existing_id).toBeTruthy()
  expect(dup.existing_version).toBeTruthy()

  // The user saves fresh content for the same entry after the preview.
  const items = await getProjectLoreItems(request, book.projectId)
  const hero = items.find((item) => item.name === 'Hero')
  expect(hero).toBeTruthy()
  await updateLoreItemContent(request, book.projectId, hero.id, 'Fresh edit saved after preview')

  // Confirming the stale preview must not replace the fresh edit; nothing is
  // written and a fresh review bound to the current state is returned.
  const stale = await confirmOverrides(request, book.projectId, reviewPlanID, [
    { name: dup.name, existing_id: dup.existing_id, existing_version: dup.existing_version, choice: 'override' },
  ])
  expect(stale.ok, JSON.stringify(stale.body)).toBe(true)
  expect(stale.body.review_changed).toBe(true)
  expect(stale.body.pending_overrides?.length).toBeGreaterThan(0)
  const after = await getProjectLoreItems(request, book.projectId)
  expect(after.find((item) => item.name === 'Hero')?.content).toBe('Fresh edit saved after preview')

  // Reviewing the fresh preview and confirming again applies the override.
  const nextDup = stale.body.pending_overrides[0]
  const redo = await confirmOverrides(request, book.projectId, stale.body.review_plan_id, [
    { name: nextDup.name, existing_id: nextDup.existing_id, existing_version: nextDup.existing_version, choice: 'override' },
  ])
  expect(redo.ok, JSON.stringify(redo.body)).toBe(true)
  expect(redo.body.pending_overrides).toBeFalsy()
  const finalItems = await getProjectLoreItems(request, book.projectId)
  // The character entry content comes from the card description; it must now
  // be the imported content, not the fresh edit saved after the first preview.
  const heroFinal = finalItems.find((item) => item.name === 'Hero')
  expect(heroFinal?.content).toContain('A review regression character.')
  expect(heroFinal?.content).not.toBe('Fresh edit saved after preview')
})

test('confirming without per-entry choices never auto-overrides', async ({ request }) => {
  const book = await createAndOpenBook(request, 'Override empty choice book')
  const created = await request.post(`/api/projects/${book.projectId}/book/lore/items`, {
    data: { name: 'Hero', type: 'character', content: 'Short local description' },
  })
  expect(created.ok(), await created.text()).toBe(true)

  const { reviewPlanID } = await importWithPending(request, book.projectId)

  // An empty override review must not fall back to detail-based automatic
  // overrides: the import stays in review and local content is untouched.
  const empty = await confirmOverrides(request, book.projectId, reviewPlanID, [])
  expect(empty.ok, JSON.stringify(empty.body)).toBe(true)
  expect(empty.body.pending_overrides?.length).toBeGreaterThan(0)
  expect(empty.body.overridden).toBeFalsy()
  const items = await getProjectLoreItems(request, book.projectId)
  expect(items.find((item) => item.name === 'Hero')?.content).toBe('Short local description')
})