<template>
	<XButton
		variant="secondary"
		icon="lock"
		class="project-leases-button"
		:class="{'has-leases': leases.length > 0}"
		@click="open = true"
	>
		{{ $t('task.leasesPanel.button') }}
		<span
			v-if="leases.length > 0"
			class="project-leases-button__count"
		>/ {{ leases.length }}</span>
	</XButton>
	<Modal
		:enabled="open"
		:overflow="true"
		variant="hint-modal"
		:aria-label="$t('task.leasesPanel.title')"
		@close="open = false"
	>
		<Card
			class="project-leases-panel"
			:title="$t('task.leasesPanel.title')"
			:show-close="true"
			@close="open = false"
		>
			<p class="project-leases-panel__hint">
				{{ $t('task.leasesPanel.subtitle') }}
			</p>
			<p
				v-if="groups.length === 0"
				class="project-leases-panel__empty"
			>
				{{ $t('task.leasesPanel.empty') }}
			</p>
			<ul
				v-else
				class="project-leases-panel__rows"
			>
				<li
					v-for="group in groups"
					:key="group.taskId"
					class="project-leases-row"
				>
					<div class="project-leases-row__head">
						<RouterLink
							:to="{name: 'task.detail', params: {id: group.taskId}}"
							class="project-leases-row__task"
						>
							<span class="project-leases-row__id">{{ group.identifier }}</span>
							{{ group.title }}
						</RouterLink>
						<span class="project-leases-row__meta">
							{{ $t('task.leasesPanel.holder', {user: group.holder}) }}
							<template v-if="group.owner">
								· {{ $t('task.leasesPanel.forOwner', {user: group.owner}) }}
							</template>
							· {{ $t('task.leasesPanel.since', {date: formatDateSince(group.since)}) }}
							<template v-if="group.lastActive">
								· {{ $t('task.leasesPanel.lastActive', {date: formatDateSince(group.lastActive)}) }}
							</template>
						</span>
						<span
							v-if="group.stale"
							v-tooltip="$t('task.leasesPanel.staleTooltip')"
							class="project-leases-row__stale"
						>{{ $t('task.leasesPanel.stale') }}</span>
						<XButton
							v-if="canWrite"
							variant="tertiary"
							icon="unlink"
							:loading="releasing === group.taskId"
							class="project-leases-row__release"
							@click="release(group.taskId)"
						>
							{{ $t('task.leasesPanel.release') }}
						</XButton>
					</div>
					<ul class="project-leases-row__patterns">
						<li
							v-for="pattern in group.patterns"
							:key="pattern"
							class="project-leases-row__pattern"
						>
							<span class="icon"><Icon icon="lock" /></span>
							{{ pattern }}
						</li>
					</ul>
				</li>
			</ul>
		</Card>
	</Modal>
</template>

<script setup lang="ts">
import {computed, ref} from 'vue'
import {useI18n} from 'vue-i18n'
import {useQuery} from '@tanstack/vue-query'

import Card from '@/components/misc/Card.vue'

import type {TaskPathLease} from '@/client/generated'
import {projectLeasesQuery, useReleaseTaskLeases} from '@/client/queries/taskScope'
import {formatDateSince} from '@/helpers/time/formatDate'
import {error, success} from '@/message'

const props = defineProps<{
	projectId: number,
	canWrite: boolean,
}>()

const {t} = useI18n({useScope: 'global'})

const open = ref(false)

const leasesQuery = useQuery(computed(() => ({
	...projectLeasesQuery(props.projectId),
	enabled: props.projectId > 0,
})))
const leases = computed<TaskPathLease[]>(() => leasesQuery.data.value ?? [])

interface LeaseGroup {
	taskId: number
	identifier: string
	title: string
	holder: string
	owner: string | null
	since: Date | null
	lastActive: Date | null
	stale: boolean
	patterns: string[]
}

// One row per holding task: that is the unit a human reasons about
// ("what is this agent editing"), not the individual pattern.
const groups = computed<LeaseGroup[]>(() => {
	const byTask = new Map<number, LeaseGroup>()
	for (const lease of leases.value) {
		if (!lease.task_id) {
			continue
		}
		let group = byTask.get(lease.task_id)
		if (!group) {
			group = {
				taskId: lease.task_id,
				identifier: lease.task?.identifier || `#${lease.task?.index ?? lease.task_id}`,
				title: lease.task?.title ?? '',
				holder: lease.user?.name || lease.user?.username || `#${lease.user_id}`,
				owner: lease.owner ? (lease.owner.name || lease.owner.username || null) : null,
				since: lease.created ? new Date(lease.created) : null,
				lastActive: lease.last_active ? new Date(lease.last_active) : null,
				stale: false,
				patterns: [],
			}
			byTask.set(lease.task_id, group)
		}
		group.stale = group.stale || Boolean(lease.stale)
		if (lease.pattern) {
			group.patterns.push(lease.pattern)
		}
	}
	return [...byTask.values()]
})

const releasing = ref<number | null>(null)
const releaseTarget = ref(0)
const releaseMutation = useReleaseTaskLeases(() => releaseTarget.value, () => props.projectId)

async function release(taskId: number) {
	releaseTarget.value = taskId
	releasing.value = taskId
	try {
		await releaseMutation.mutateAsync()
		success({message: t('task.leasesPanel.released')})
	} catch (e) {
		error(e)
	} finally {
		releasing.value = null
	}
}
</script>

<style lang="scss" scoped>
.project-leases-button__count {
	@include mono-label;

	margin-inline-start: var(--wm-space-1);
	color: var(--wm-accent-text);
}

.project-leases-panel {
	inline-size: min(40rem, 90vw);
}

.project-leases-panel__hint,
.project-leases-panel__empty {
	font-size: var(--wm-text-sm);
	color: var(--wm-text-secondary);
}

.project-leases-panel__rows {
	margin: var(--wm-space-3) 0 0;
	padding: 0;
	list-style: none;
}

.project-leases-row {
	padding-block: var(--wm-space-3);
	border-block-start: 1px solid var(--wm-line-faint);
}

.project-leases-row__head {
	display: flex;
	flex-wrap: wrap;
	align-items: center;
	gap: var(--wm-space-2) var(--wm-space-3);
}

.project-leases-row__task {
	flex: 1 1 12rem;
	color: var(--wm-text);
	font-weight: 500;
	text-decoration: none;

	&:hover,
	&:focus-visible {
		color: var(--wm-accent-text);
	}
}

.project-leases-row__id {
	@include mono-data;

	margin-inline-end: var(--wm-space-2);
	color: var(--wm-text-tertiary);
}

.project-leases-row__meta {
	font-size: var(--wm-text-xs);
	color: var(--wm-text-tertiary);
}

.project-leases-row__stale {
	@include mono-label;

	padding: 1px var(--wm-space-1);
	color: var(--wm-text);
	border: 1px solid var(--warning);
}

.project-leases-row__patterns {
	display: flex;
	flex-wrap: wrap;
	gap: var(--wm-space-1);
	margin: var(--wm-space-2) 0 0;
	padding: 0;
	list-style: none;
}

.project-leases-row__pattern {
	display: inline-flex;
	align-items: center;
	gap: var(--wm-space-1);
	padding: 2px var(--wm-space-2);
	font-family: $family-monospace;
	font-size: var(--wm-text-xs);
	background: var(--wm-accent-wash);
	color: var(--wm-text);

	@include chamfer(var(--wm-chamfer-sm), bottom-right);
	@include chamfer-outline(var(--wm-accent-line));

	.icon {
		font-size: var(--wm-text-2xs);
		color: var(--wm-accent-text);
	}
}
</style>
