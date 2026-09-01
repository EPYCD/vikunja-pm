import {Factory} from '../support/factory'

export class TaskScopeFactory extends Factory {
	static table = 'task_scopes'

	static factory() {
		const now = new Date()

		return {
			id: '{increment}',
			task_id: '{increment}',
			// JSON columns travel as encoded strings through the seed endpoint.
			paths_owned: '["pkg/models/**"]',
			paths_affected: '[]',
			endpoints: '[]',
			notes: '',
			created: now.toISOString(),
			updated: now.toISOString(),
		}
	}
}
