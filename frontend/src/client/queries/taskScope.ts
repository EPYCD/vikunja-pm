import {toValue, type MaybeRefOrGetter} from 'vue'
import {queryOptions, useMutation} from '@tanstack/vue-query'

import {
	projectsLeasesList,
	projectsViewsReadiness,
	tasksLeasesRelease,
	tasksScopeUpdate,
	tasksScopeRead,
} from '@/client/generated'
import type {TaskPathLease, TaskReadiness, TaskScope, TaskScopeWritable} from '@/client/generated'
import {queryClient} from '@/client/queryClient'

export const scopeKeys = {
	all: ['task-scope'] as const,
	task: (taskId: number) => ['task-scope', 'task', taskId] as const,
	projectLeases: (projectId: number) => ['task-scope', 'leases', projectId] as const,
	readiness: (projectId: number, viewId: number) => ['task-scope', 'readiness', projectId, viewId] as const,
}

export type ScopeList = 'paths_owned' | 'paths_affected' | 'endpoints'

export const READINESS_REASON = {
	DONE: 'done',
	ASSIGNED: 'assigned',
	BLOCKED: 'blocked',
	LEASE_CONFLICT: 'lease_conflict',
} as const

export function emptyScope(taskId: number): TaskScope {
	return {task_id: taskId, paths_owned: [], paths_affected: [], endpoints: [], notes: ''}
}

// The server omits a list it has nothing for; the editor wants arrays.
function normaliseScope(scope: TaskScope, taskId: number): TaskScope {
	return {
		...scope,
		task_id: scope.task_id ?? taskId,
		paths_owned: scope.paths_owned ?? [],
		paths_affected: scope.paths_affected ?? [],
		endpoints: scope.endpoints ?? [],
		notes: scope.notes ?? '',
	}
}

export function taskScopeQuery(taskId: number) {
	return queryOptions({
		queryKey: scopeKeys.task(taskId),
		queryFn: async () => {
			const {data} = await tasksScopeRead({path: {projecttask: taskId}})
			return normaliseScope(data, taskId)
		},
		staleTime: 30 * 1000,
	})
}

export function projectLeasesQuery(projectId: number) {
	return queryOptions({
		queryKey: scopeKeys.projectLeases(projectId),
		queryFn: async (): Promise<TaskPathLease[]> => {
			const {data} = await projectsLeasesList({path: {project: projectId}})
			return data.items ?? []
		},
		staleTime: 15 * 1000,
	})
}

export function viewReadinessQuery(projectId: number, viewId: number) {
	return queryOptions({
		queryKey: scopeKeys.readiness(projectId, viewId),
		queryFn: async (): Promise<TaskReadiness[]> => {
			const {data} = await projectsViewsReadiness({path: {project: projectId, view: viewId}})
			return data.items ?? []
		},
		staleTime: 15 * 1000,
	})
}

export function readinessByTask(rows: TaskReadiness[]): Record<number, TaskReadiness> {
	const out: Record<number, TaskReadiness> = {}
	for (const row of rows) {
		if (row.task?.id) {
			out[row.task.id] = row
		}
	}
	return out
}

export function invalidateReadiness(projectId: number, viewId?: number) {
	return queryClient.invalidateQueries({
		queryKey: viewId ? scopeKeys.readiness(projectId, viewId) : ['task-scope', 'readiness', projectId],
	})
}

function invalidateLeaseViews(projectId: number) {
	return Promise.all([
		queryClient.invalidateQueries({queryKey: scopeKeys.projectLeases(projectId)}),
		invalidateReadiness(projectId),
	])
}

export function useUpdateTaskScope(taskId: MaybeRefOrGetter<number>, projectId: MaybeRefOrGetter<number>) {
	return useMutation({
		mutationFn: async (scope: TaskScopeWritable) => {
			const id = toValue(taskId)
			const {data} = await tasksScopeUpdate({path: {projecttask: id}, body: scope})
			return normaliseScope(data, id)
		},
		onSuccess: async (scope) => {
			queryClient.setQueryData(scopeKeys.task(toValue(taskId)), scope)
			await invalidateLeaseViews(toValue(projectId))
		},
	})
}

export function useReleaseTaskLeases(taskId: MaybeRefOrGetter<number>, projectId: MaybeRefOrGetter<number>) {
	return useMutation({
		mutationFn: async () => {
			await tasksLeasesRelease({path: {projecttask: toValue(taskId)}})
		},
		onSuccess: () => invalidateLeaseViews(toValue(projectId)),
	})
}
