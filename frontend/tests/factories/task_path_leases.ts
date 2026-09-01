import {Factory} from '../support/factory'

export class TaskPathLeaseFactory extends Factory {
	static table = 'task_path_leases'

	static factory() {
		const now = new Date()

		return {
			id: '{increment}',
			task_id: '{increment}',
			project_id: 1,
			user_id: 1,
			pattern: 'pkg/models/**',
			created: now.toISOString(),
		}
	}
}
