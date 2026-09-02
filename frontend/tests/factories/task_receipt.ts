import {Factory} from '../support/factory'

export class TaskReceiptFactory extends Factory {
	static table = 'task_receipts'

	static factory() {
		const now = new Date()

		return {
			id: '{increment}',
			task_id: '{increment}',
			project_id: 1,
			commit_sha: 'abc1234def5678',
			branch: 'feat/story',
			// JSON columns travel as encoded strings through the seed endpoint.
			gates: '[{"name":"typecheck","status":"passed","duration_ms":1200}]',
			docs_api_required: false,
			docs_api_regenerated: false,
			ci_run_url: 'https://ci.example.com/runs/1',
			merged: true,
			merge_sha: 'fedcba9876543210',
			passed: true,
			posted_by_id: 1,
			created: now.toISOString(),
		}
	}
}
