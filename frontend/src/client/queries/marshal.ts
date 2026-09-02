import {queryOptions} from '@tanstack/vue-query'

import {
	marshalGet,
	type MarshalChokepoints,
	type MarshalHealth,
	type MarshalReferences,
	type MarshalWorkers,
} from '@/client/marshal'

export const marshalKeys = {
	all: ['marshal'] as const,
	references: (marshalUrl: string, taskId: number) => ['marshal', marshalUrl, 'references', taskId] as const,
	health: (marshalUrl: string) => ['marshal', marshalUrl, 'health'] as const,
	workers: (marshalUrl: string) => ['marshal', marshalUrl, 'workers'] as const,
	chokepoints: (marshalUrl: string) => ['marshal', marshalUrl, 'chokepoints'] as const,
}

// Marshal being down is a muted line in the UI, not a retry storm.
const MARSHAL_QUERY = {retry: false, staleTime: 30 * 1000} as const

export function taskReferencesQuery(marshalUrl: string, taskId: number) {
	return queryOptions({
		...MARSHAL_QUERY,
		queryKey: marshalKeys.references(marshalUrl, taskId),
		queryFn: () => marshalGet<MarshalReferences>(marshalUrl, `/api/tasks/${taskId}/references`),
		enabled: marshalUrl !== '' && taskId > 0,
	})
}

export function marshalHealthQuery(marshalUrl: string) {
	return queryOptions({
		...MARSHAL_QUERY,
		queryKey: marshalKeys.health(marshalUrl),
		queryFn: () => marshalGet<MarshalHealth>(marshalUrl, '/api/health'),
		enabled: marshalUrl !== '',
	})
}

export function marshalWorkersQuery(marshalUrl: string) {
	return queryOptions({
		...MARSHAL_QUERY,
		queryKey: marshalKeys.workers(marshalUrl),
		queryFn: () => marshalGet<MarshalWorkers>(marshalUrl, '/api/workers'),
		enabled: marshalUrl !== '',
	})
}

export function marshalChokepointsQuery(marshalUrl: string) {
	return queryOptions({
		...MARSHAL_QUERY,
		queryKey: marshalKeys.chokepoints(marshalUrl),
		queryFn: () => marshalGet<MarshalChokepoints>(marshalUrl, '/api/chokepoints'),
		enabled: marshalUrl !== '',
	})
}
