import {getToken} from '@/helpers/auth'

// Response shapes of the Marshal service, a sibling of the API that reads the
// spec and the worker fleet. Fields it leaves out arrive as null.

export interface MarshalRef {
	id: string
	prefix: string
}

export interface MarshalAnchor {
	id: string
	file: string
	line: number
	title: string
	text: string
	hash: string
}

export interface MarshalResolution {
	ref: MarshalRef
	found: boolean
	anchor?: MarshalAnchor | null
	provenance?: string
}

export interface MarshalPaste {
	file: string
	line: number
	words: number
	excerpt: string
}

export interface MarshalReferences {
	task_id: number
	identifier: string
	resolutions: MarshalResolution[] | null
	pastes: MarshalPaste[] | null
	broken: boolean
}

export interface MarshalFinding {
	code: string
	message: string
	task_ids?: number[] | null
	paths?: string[] | null
}

export interface MarshalHealth {
	tasks: number
	containers: number
	collisions: number
	cycles: number
	unblocked_roots: number[] | null
	findings: MarshalFinding[] | null
	ok: boolean
	open_tasks: number
	leases: number
	checked_at: string
}

export type MarshalPerson = string | {id?: number, username?: string, name?: string} | null | undefined

export interface MarshalAgent {
	user: {id: number, username: string, name: string}
	owner: MarshalPerson
	open_tasks: number
	leases: number
}

export interface MarshalAllocation {
	worker: string
	task_id: number
	story: string
	branch: string
	checkout: string
	database: string
	port: number
	since: string
}

export interface MarshalWorktree {
	path: string
	branch: string
	head: string
}

export interface MarshalConflict {
	checkout: string
	workers: string[] | null
	missing: string[] | null
}

export interface MarshalWorkers {
	agents: MarshalAgent[] | null
	allocations: MarshalAllocation[] | null
	worktrees: MarshalWorktree[] | null
	conflicts: MarshalConflict[] | null
}

export interface MarshalClaim {
	task_id: number
	identifier: string
	title: string
	assignee: MarshalPerson
	pattern: string
	active: boolean
}

export interface MarshalQueueEntry {
	position: number
	claim: MarshalClaim
	waiting_on: number[] | null
}

export interface MarshalQueue {
	chokepoint: string
	entries: MarshalQueueEntry[] | null
}

export interface MarshalChokepoints {
	source: string
	queues: MarshalQueue[] | null
}

export class MarshalError extends Error {
	status: number

	constructor(status: number, message: string) {
		super(message)
		this.name = 'MarshalError'
		this.status = status
	}
}

export function marshalPersonName(person: MarshalPerson): string {
	if (!person) {
		return ''
	}
	if (typeof person === 'string') {
		return person
	}
	return person.name || person.username || (person.id ? `#${person.id}` : '')
}

// Marshal trusts the same JWT the app sends to /api, so no separate login.
export async function marshalGet<T>(marshalUrl: string, path: string): Promise<T> {
	const headers = new Headers({Accept: 'application/json'})
	const token = getToken()
	if (token) {
		headers.set('Authorization', `Bearer ${token}`)
	}
	const response = await fetch(`${marshalUrl.replace(/\/+$/, '')}${path}`, {headers})
	if (!response.ok) {
		throw new MarshalError(response.status, `Marshal responded with ${response.status}`)
	}
	return await response.json() as T
}
