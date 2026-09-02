import {test, expect} from '../../support/fixtures'
import {BucketFactory} from '../../factories/bucket'
import {ProjectFactory} from '../../factories/project'
import {ProjectViewFactory} from '../../factories/project_view'
import {TaskFactory} from '../../factories/task'
import {TaskBucketFactory} from '../../factories/task_buckets'
import {TaskRelationFactory} from '../../factories/task_relation'
import {TaskScopeFactory} from '../../factories/task_scopes'
import {TaskPathLeaseFactory} from '../../factories/task_path_leases'
import {TaskAssigneeFactory} from '../../factories/task_assignee'
import {UserFactory} from '../../factories/user'

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

	test('ready badges carry the queue rank in board order', async ({authenticatedPage: page}) => {
		const {projects, views, tasks} = await seedBoard()
		await page.goto(`/projects/${projects[0].id}/${views[0].id}`)

		await expect(card(page, tasks.ready.id).locator('.wm-badge.is-ready')).toContainText('Ready · 1')
	})

	test('by assignee groups each column into lanes, unassigned last', async ({authenticatedPage: page}) => {
		const {projects, views, tasks} = await seedBoard()
		await TaskAssigneeFactory.create(1, {task_id: tasks.holder.id, user_id: 1})
		await page.goto(`/projects/${projects[0].id}/${views[0].id}`)

		await page.locator('.kanban-lanes-toggle').click()
		const lanes = page.locator('.kanban .bucket').nth(1).locator('.kanban-lane')
		await expect(lanes).toHaveCount(1)
		await expect(lanes.first().locator('.kanban-lane__label')).toContainText('/ 1')
		await expect(lanes.first().locator('.kanban-lane__leases')).toContainText('1')
		await expect(lanes.first().locator(`.task[data-task-id="${tasks.holder.id}"]`)).toBeVisible()

		const queueLanes = page.locator('.kanban .bucket').first().locator('.kanban-lane')
		await expect(queueLanes).toHaveCount(1)
		await expect(queueLanes.first().locator('.kanban-lane__label')).toContainText('Unassigned / 3')

		await page.locator('.kanban-lanes-toggle').click()
		await expect(page.locator('.kanban-lane')).toHaveCount(0)
	})

	test('bot lanes and leases name the human behind the bot', async ({authenticatedPage: page, currentUser}) => {
		const {projects, views, tasks} = await seedBoard()
		await UserFactory.create(1, {id: 2, username: 'bot-lane', bot_owner_id: currentUser.id}, false)
		await TaskPathLeaseFactory.create(1, {task_id: tasks.holder.id, project_id: projects[0].id, pattern: 'pkg/models/**', user_id: 2})
		await TaskAssigneeFactory.create(1, {task_id: tasks.holder.id, user_id: 2})
		await page.goto(`/projects/${projects[0].id}/${views[0].id}`)

		await page.locator('.kanban-lanes-toggle').click()
		const lane = page.locator('.kanban .bucket').nth(1).locator('.kanban-lane').first()
		await expect(lane.locator('.kanban-lane__label')).toContainText('bot-lane')
		await expect(lane.locator('.kanban-lane__owner')).toContainText(`for ${currentUser.username}`)
		await page.locator('.kanban-lanes-toggle').click()

		await page.locator('.project-leases-button').click()
		await expect(page.locator('.project-leases-panel .project-leases-row__meta'))
			.toContainText(`held by bot-lane · for ${currentUser.username}`)
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
