import {queryOptions} from '@tanstack/vue-query'

import {botsList} from '@/client/generated'
import type {BotUser} from '@/client/generated'

export const botKeys = {
	mine: ['bots', 'mine'] as const,
}

export function myBotsQuery() {
	return queryOptions({
		queryKey: botKeys.mine,
		queryFn: async (): Promise<BotUser[]> => {
			const {data} = await botsList({query: {per_page: 1000}})
			return data.items ?? []
		},
		staleTime: 60 * 1000,
	})
}
