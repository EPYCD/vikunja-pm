import {test, expect} from '../../support/fixtures'
import {ProjectFactory} from '../../factories/project'
import {TaskFactory} from '../../factories/task'
import {TaskReceiptFactory} from '../../factories/task_receipt'
import {createDefaultViews} from '../project/prepareProjects'

test.describe('Task gate receipts', () => {
	test.beforeEach(async () => {
		const projects = await ProjectFactory.create(1)
		await createDefaultViews(projects[0].id)
	})

	test('lists receipts newest first with their gates and CI link', async ({authenticatedPage: page}) => {
		const tasks = await TaskFactory.create(1, {id: 1, index: 1})
		const anHourAgo = new Date(Date.now() - 60 * 60 * 1000).toISOString()

		await TaskReceiptFactory.create(1, {
			id: 1,
			task_id: tasks[0].id,
			project_id: 1,
			commit_sha: 'abc1234def5678',
			passed: true,
			merged: true,
			ci_run_url: 'https://ci.example.com/runs/1',
			created: anHourAgo,
		})
		// Each create() restarts {increment}, so sibling rows need explicit ids.
		await TaskReceiptFactory.create(1, {
			id: 2,
			task_id: tasks[0].id,
			project_id: 1,
			commit_sha: '9876543fedcba0',
			gates: '[{"name":"typecheck","status":"passed","duration_ms":1200},{"name":"test","status":"failed","duration_ms":123000}]',
			passed: false,
			merged: false,
			merge_sha: '',
			ci_run_url: 'https://ci.example.com/runs/2',
		}, false)

		await page.goto(`/tasks/${tasks[0].id}`)

		const rows = page.locator('.task-view .task-receipts .task-receipt-row')
		await expect(rows).toHaveCount(2)
		await expect(rows.nth(0)).toHaveClass(/is-failed/)
		await expect(rows.nth(1)).toHaveClass(/is-passed/)
		await expect(rows.nth(0).locator('.task-receipt-row__gates')).toContainText('typecheck')
		await expect(rows.nth(0).locator('.task-receipt-row__gates')).toContainText('2m03s')
		await expect(rows.nth(0).locator('.task-receipt-row__ci')).toHaveAttribute('href', 'https://ci.example.com/runs/2')
		await expect(rows.nth(1).locator('.task-receipt-row__ci')).toHaveAttribute('href', 'https://ci.example.com/runs/1')
	})

	test('renders nothing for a task without receipts', async ({authenticatedPage: page}) => {
		const tasks = await TaskFactory.create(1, {id: 1, index: 1})
		await page.goto(`/tasks/${tasks[0].id}`)

		await expect(page.locator('.task-view .heading')).toBeVisible()
		await expect(page.locator('.task-view .task-receipts')).toHaveCount(0)
	})
})
