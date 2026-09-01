import type {ITask} from '@/modelTypes/ITask'
import type {IProject} from '@/modelTypes/IProject'
import type {IUser} from '@/modelTypes/IUser'

// Camel-cased because these arrive on a task through the v1 expand path
// (TaskModel converts keys). The scope editor talks v2 and uses the
// generated snake_case types instead.
export interface ITaskScope {
	id: number
	taskId: ITask['id']
	pathsOwned: string[]
	pathsAffected: string[]
	endpoints: string[]
	notes: string
}

export interface ITaskPathLease {
	id: number
	taskId: ITask['id']
	projectId: IProject['id']
	userId: IUser['id']
	pattern: string
	created: string
}
