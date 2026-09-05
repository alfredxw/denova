import { expect, test } from '../support/fixtures'
import { createAndOpenBook } from '../support/api'
import { openAgentChatWorkbench } from '../support/agent-chat'

test('restores a Book review tab from the production bundle without crashing', async ({ page, request }) => {
  const book = await createAndOpenBook(request, 'Production Review Book')
  const threadID = 'production-review'
  await page.route(`**/api/projects/${book.projectId}/changes/review-threads/${threadID}`, route => route.fulfill({
    json: { project_id: book.projectId, workspace: book.workspace, review_thread: { id: threadID, latest_group_id: '', groups: [], files: [], comments: [] } },
  }))
  await page.addInitScript(({ book, threadID }) => {
    localStorage.setItem('nova.agentchat.workbenches.v1', JSON.stringify({
      activeProjectId: book.projectId,
      projects: {
        [book.projectId]: {
          tabs: [{ kind: 'review', id: 'review-tab', projectId: book.projectId, workspace: book.workspace, group: 'primary', threadID }],
          activeTabIds: { primary: 'review-tab', secondary: null },
          focusedGroup: 'primary',
          secondaryVisible: false,
        },
      },
    }))
  }, { book, threadID })

  await page.goto('/')
  await openAgentChatWorkbench(page)
  await expect(page.locator(`[data-change-review-workspace="${threadID}"]`)).toBeVisible()
  await page.reload()
  await expect(page.locator(`[data-change-review-workspace="${threadID}"]`)).toBeVisible()
})
