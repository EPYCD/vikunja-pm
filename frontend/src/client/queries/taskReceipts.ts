import {queryOptions} from '@tanstack/vue-query'

import {tasksReceiptsList} from '@/client/generated'
import type {TaskReceipt} from '@/client/generated'

export const receiptKeys = {
	all: ['task-receipts'] as const,
	task: (taskId: number) => ['task-receipts', taskId] as const,
}

// Newest first: the latest run is the one that decides whether the task may close.
function byNewest(a: TaskReceipt, b: TaskReceipt): number {
	const at = a.created ? Date.parse(a.created) : 0
	const bt = b.created ? Date.parse(b.created) : 0
	return (bt - at) || ((b.id ?? 0) - (a.id ?? 0))
}

export function taskReceiptsQuery(taskId: number) {
	return queryOptions({
		queryKey: receiptKeys.task(taskId),
		queryFn: async (): Promise<TaskReceipt[]> => {
			const {data} = await tasksReceiptsList({path: {projecttask: taskId}})
			return [...(data.items ?? [])].sort(byNewest)
		},
		staleTime: 30 * 1000,
	})
}

// 1.2s below ten seconds, 45s below a minute, 2m03s above.
export function formatGateDuration(ms: number | undefined | null): string {
	const seconds = Math.max(0, ms ?? 0) / 1000
	if (seconds < 10) {
		return `${seconds.toFixed(1)}s`
	}
	const whole = Math.round(seconds)
	if (whole < 60) {
		return `${whole}s`
	}
	const minutes = Math.floor(whole / 60)
	const rest = whole % 60
	return `${minutes}m${String(rest).padStart(2, '0')}s`
}
