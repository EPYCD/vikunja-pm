import {test, expect} from '../../support/fixtures'
import {ProjectFactory} from '../../factories/project'
import {TaskFactory} from '../../factories/task'
import {TaskScopeFactory} from '../../factories/task_scopes'
import {TaskPathLeaseFactory} from '../../factories/task_path_leases'
import {createDefaultViews} from '../project/prepareProjects'

test.describe('Task scope', () => {
	test.beforeEach(async () => {
		const projects = await ProjectFactory.create(1)
		await createDefaultViews(projects[0].id)
	})

	test('shows the stored scope with its leased paths and lets the owner add one', async ({authenticatedPage: page}) => {
		const tasks = await TaskFactory.create(1, {id: 1, index: 1})
		await TaskScopeFactory.create(1, {
			task_id: tasks[0].id,
			paths_owned: '["pkg/models/**","pkg/routes/api/v2/task_claim.go"]',
			paths_affected: '["frontend/src/stores/kanban.ts"]',
			endpoints: '["POST /api/v2/tasks/{id}/claim"]',
			notes: 'No frontend changes.',
		})
		await TaskPathLeaseFactory.create(1, {
			task_id: tasks[0].id,
			project_id: 1,
			pattern: 'pkg/models/**',
		})

		await page.goto(`/tasks/${tasks[0].id}`)

		const scope = page.locator('.task-view .task-scope')
		await expect(scope).toBeVisible()
		await expect(scope.locator('.scope-chip').filter({hasText: 'pkg/models/**'})).toHaveClass(/is-leased/)
		await expect(scope.locator('.scope-chip').filter({hasText: 'pkg/routes/api/v2/task_claim.go'})).not.toHaveClass(/is-leased/)
		await expect(scope).toContainText('frontend/src/stores/kanban.ts')
		await expect(scope).toContainText('POST /api/v2/tasks/{id}/claim')
		await expect(scope.locator('.scope-notes')).toHaveValue('No frontend changes.')
		await expect(scope.locator('.scope-lease-row')).toHaveCount(1)

		const owned = scope.locator('.scope-list').first()
		await owned.locator('input.scope-add__input').fill('docs/api.md')
		await owned.locator('button[type="submit"]').click()
		await expect(owned.locator('.scope-chip').filter({hasText: 'docs/api.md'})).toBeVisible()
		await expect(page.locator('.global-notification .vue-notification.success')).toBeVisible()
	})

	test('releasing drops the leases but keeps the scope', async ({authenticatedPage: page}) => {
		const tasks = await TaskFactory.create(1, {id: 1, index: 1})
		await TaskScopeFactory.create(1, {task_id: tasks[0].id})
		await TaskPathLeaseFactory.create(1, {task_id: tasks[0].id, project_id: 1})

		await page.goto(`/tasks/${tasks[0].id}`)
		const scope = page.locator('.task-view .task-scope')
		await scope.locator('.scope-leases .button').filter({hasText: 'Release leases'}).click()

		await expect(scope.locator('.scope-lease-row')).toHaveCount(0)
		await expect(scope.locator('.scope-chip').filter({hasText: 'pkg/models/**'})).toBeVisible()
		await expect(scope.locator('.scope-chip').filter({hasText: 'pkg/models/**'})).not.toHaveClass(/is-leased/)
	})

	test('stays hidden for a task without a scope until Set Scope is clicked', async ({authenticatedPage: page}) => {
		const tasks = await TaskFactory.create(1, {id: 1, index: 1})
		await page.goto(`/tasks/${tasks[0].id}`)

		await expect(page.locator('.task-view .task-scope')).not.toBeVisible()
		await page.locator('.task-view .action-buttons .button').filter({hasText: 'Set Scope'}).click()
		await expect(page.locator('.task-view .task-scope')).toBeVisible()
	})
})
