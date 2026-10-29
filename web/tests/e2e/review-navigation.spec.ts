import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'
import { openAgentChatWorkbench } from '../support/agent-chat'
import type { ReviewThreadFile } from '../../src/features/changes/types'

for (const theme of ['dark', 'light'] as const) {
  test(`review file navigation settles on wrapped and collapsed files in ${theme}`, async ({ page, request }, testInfo) => {
    await page.setViewportSize({ width: 1600, height: 1100 })
    const book = await createAndOpenBook(request, 'Review navigation')
    const themed = await request.patch('/api/settings', { data: { layer: 'user', changes: { theme } } })
    expect(themed.ok(), await themed.text()).toBe(true)
    const threadID = 'review-navigation'
    const paths = ['chapters/chapter.md', 'setting/chapter-groups/group.md', 'setting/character-states.md', 'setting/progress.md']
    // Nearby files share a virtual window: a re-entrant render can restore the
    // previous anchor even though a long-distance jump succeeds.
    const files: ReviewThreadFile[] = paths.map((path, index) => ({
      path,
      before_content: '',
      after_content: Array.from({ length: index === 0 ? 493 : index === 3 ? 100 : 24 }, (_, line) => `${line + 1}. ${'角色状态与剧情进展，长段落需要自动换行。'.repeat(index === 0 ? 1 : 4)}`).join('\n'),
      before_exists: false, after_exists: true,
      base_revision: '', revision: `revision-${index}`,
      base_group_id: 'group', latest_group_id: 'group',
      base_change_set_id: path, latest_change_set_id: path,
      group_ids: ['group'], change_set_ids: [path], pending_edit_ids: [],
      review_status: 'pending', apply_state: 'applied', continuity: 'continuous',
    }))
    await page.route(`**/api/projects/${book.projectId}/changes/review-threads/${threadID}`, route => route.fulfill({
      json: { project_id: book.projectId, workspace: book.workspace, review_thread: { id: threadID, latest_group_id: '', groups: [], files, comments: [] } },
    }))
    await page.addInitScript(({ book, threadID }) => {
      localStorage.setItem('nova.agentchat.workbenches.v1', JSON.stringify({
        activeProjectId: book.projectId,
        projects: {
          [book.projectId]: {
            tabs: [{ kind: 'review', id: 'review-tab', projectId: book.projectId, workspace: book.workspace, group: 'primary', threadID }],
            activeTabIds: { primary: 'review-tab', secondary: null }, focusedGroup: 'primary', secondaryVisible: false,
          },
        },
      }))
    }, { book, threadID })
    await page.goto('/')
    await openAgentChatWorkbench(page)
    const review = page.locator(`[data-change-review-workspace="${threadID}"]`)
    const view = review.locator('.nova-code-diff-view')
    await expect(view).toBeVisible()

    for (const layout of ['统一', '并排']) {
      await review.getByRole('button', { name: layout, exact: true }).click()
      for (const index of [2, 3, 1, 0]) {
        await review.getByRole('treeitem', { name: paths[index].split('/').at(-1)!, exact: true }).click()
        const header = review.locator(`[data-code-diff-header="${paths[index]}"]`)
        await expect(header).toBeInViewport()
        await expect.poll(async () => {
          const viewport = await view.boundingBox()
          const target = await header.boundingBox()
          return target && viewport ? Math.abs(target.y - viewport.y) : Infinity
        }).toBeLessThan(40)
        await expect(review.getByRole('treeitem', { name: paths[index].split('/').at(-1)!, exact: true })).toHaveAttribute('aria-selected', 'true')
      }
      await review.getByRole('button', { name: '折叠全部 Diff', exact: true }).click()
      await review.getByRole('treeitem', { name: 'progress.md', exact: true }).click()
      await expect(review.getByRole('button', { name: '折叠 setting/progress.md', exact: true })).toBeInViewport()
      await page.screenshot({ path: testInfo.outputPath(`review-navigation-${layout}.png`) })
    }
  })
}
