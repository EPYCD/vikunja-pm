import {test, expect} from '../../support/fixtures'
import {BucketFactory} from '../../factories/bucket'
import {ProjectFactory} from '../../factories/project'
import {ProjectViewFactory} from '../../factories/project_view'
import {TaskFactory} from '../../factories/task'
import {TaskBucketFactory} from '../../factories/task_buckets'
import {TaskRelationFactory} from '../../factories/task_relation'
import {TaskScopeFactory} from '../../factories/task_scopes'
import {TaskPathLeaseFactory} from '../../factories/task_path_leases'

// Seeds a two-bucket board: three tasks queued in the default bucket (one
// ready, one blocked, one whose path another task leases) and the lease
// holder in the second bucket.
async function seedBoard() {
	const projects = await ProjectFactory.create(1)
	const views = await ProjectViewFactory.create(1, {
		id: 4,
		project_id: projects[0].id,
		view_kind: 3,
		bucket_configuration_mode: 1,
		default_bucket_id: 1,
	})
	const buckets = await BucketFactory.create(2, {project_view_id: views[0].id})
	const tasks = await TaskFactory.create(4, {project_id: projects[0].id})
	const [ready, blocked, conflicted, holder] = tasks

	for (const t of [ready, blocked, conflicted]) {
		await TaskBucketFactory.create(1, {task_id: t.id, bucket_id: buckets[0].id, project_view_id: views[0].id}, false)
	}
	await TaskBucketFactory.create(1, {task_id: holder.id, bucket_id: buckets[1].id, project_view_id: views[0].id}, false)

	await TaskRelationFactory.create(1, {task_id: blocked.id, other_task_id: holder.id, relation_kind: 'blocked'})
	// Each create() restarts {increment}, so sibling rows need explicit ids.
	await TaskScopeFactory.create(1, {id: 1, task_id: conflicted.id, paths_owned: '["pkg/models/tasks.go"]'})
	await TaskScopeFactory.create(1, {id: 2, task_id: holder.id, paths_owned: '["pkg/models/**"]'}, false)
	await TaskPathLeaseFactory.create(1, {task_id: holder.id, project_id: projects[0].id, pattern: 'pkg/models/**'})

	return {projects, views, tasks: {ready, blocked, conflicted, holder}}
}

function card(page, taskId: number) {
	return page.locator(`.kanban .task[data-task-id="${taskId}"]`)
}

test.describe('Kanban readiness', () => {
	test('badges show ready, blocked, leased-path and held leases', async ({authenticatedPage: page}) => {
		const {projects, views, tasks} = await seedBoard()
		await page.goto(`/projects/${projects[0].id}/${views[0].id}`)

		await expect(card(page, tasks.ready.id).locator('.wm-badge.is-ready')).toContainText('Ready')
		await expect(card(page, tasks.blocked.id).locator('.wm-badge.is-halted')).toContainText('Blocked')
		await expect(card(page, tasks.conflicted.id).locator('.wm-badge.is-halted')).toContainText('Path leased')
		await expect(card(page, tasks.holder.id).locator('.wm-badge')).toContainText('1')
		await expect(card(page, tasks.holder.id).locator('.wm-badge .fa-lock')).toBeVisible()
	})

	test('ready only hides unclaimable tasks in the queue bucket', async ({authenticatedPage: page}) => {
		const {projects, views, tasks} = await seedBoard()
		await page.goto(`/projects/${projects[0].id}/${views[0].id}`)
		await expect(card(page, tasks.blocked.id)).toBeVisible()

		await page.locator('.kanban-ready-toggle').click()

		await expect(card(page, tasks.ready.id)).toBeVisible()
		await expect(card(page, tasks.blocked.id)).not.toBeVisible()
		await expect(card(page, tasks.conflicted.id)).not.toBeVisible()
		await expect(card(page, tasks.holder.id)).toBeVisible()

		await page.locator('.kanban-ready-toggle').click()
		await expect(card(page, tasks.blocked.id)).toBeVisible()
	})

	test('the leases panel lists holders and can release', async ({authenticatedPage: page}) => {
		const {projects, views, tasks} = await seedBoard()
		await page.goto(`/projects/${projects[0].id}/${views[0].id}`)

		const button = page.locator('.project-leases-button')
		await expect(button).toContainText('1')
		await button.click()

		const panel = page.locator('.project-leases-panel')
		await expect(panel.locator('.project-leases-row')).toHaveCount(1)
		await expect(panel).toContainText(tasks.holder.title)
		await expect(panel).toContainText('pkg/models/**')

		await panel.locator('.project-leases-row__release').click()
		await expect(panel).toContainText('No paths are leased right now.')
		await page.keyboard.press('Escape')

		await expect(card(page, tasks.conflicted.id).locator('.wm-badge.is-ready')).toContainText('Ready')
	})
})
