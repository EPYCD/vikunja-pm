<template>
	<XButton
		variant="secondary"
		icon="heart-pulse"
		class="project-marshal-button"
		:class="{'is-violated': health && !health.ok}"
		@click="open = true"
	>
		{{ $t('project.marshal.button') }}
		<span
			v-if="findings.length > 0"
			class="project-marshal-button__count"
		>/ {{ findings.length }}</span>
	</XButton>
	<Modal
		:enabled="open"
		:overflow="true"
		variant="hint-modal"
		:aria-label="$t('project.marshal.title')"
		@close="open = false"
	>
		<Card
			class="project-marshal-panel"
			:title="$t('project.marshal.title')"
			:show-close="true"
			@close="open = false"
		>
			<p class="project-marshal-panel__hint">
				{{ $t('project.marshal.subtitle') }}
			</p>

			<details
				class="marshal-section"
				open
			>
				<summary class="marshal-section__head">
					<span>{{ $t('project.marshal.invariants') }}</span>
					<span
						v-if="health"
						class="marshal-chip"
						:class="health.ok ? 'is-ok' : 'is-violated'"
					>{{ health.ok ? $t('project.marshal.ok') : $t('project.marshal.violated') }}</span>
				</summary>
				<p
					v-if="healthQuery.isError.value"
					class="marshal-section__note"
				>
					{{ $t('project.marshal.unreachable') }}
				</p>
				<template v-else-if="health">
					<dl class="marshal-stats">
						<div
							v-for="stat in stats"
							:key="stat.label"
							class="marshal-stat"
						>
							<dt>{{ stat.label }}</dt>
							<dd>{{ stat.value }}</dd>
						</div>
					</dl>
					<p
						v-if="unblockedRoots.length > 0"
						class="marshal-roots"
					>
						<span class="marshal-roots__label">{{ $t('project.marshal.unblockedRoots') }}</span>
						<RouterLink
							v-for="id in unblockedRoots"
							:key="id"
							:to="{name: 'task.detail', params: {id}}"
							class="marshal-task-link"
						>
							#{{ id }}
						</RouterLink>
					</p>
					<ul
						v-if="findings.length > 0"
						class="marshal-findings"
					>
						<li
							v-for="(finding, index) in findings"
							:key="`${finding.code}-${index}`"
							class="marshal-finding"
						>
							<code class="marshal-finding__code">{{ finding.code }}</code>
							<span class="marshal-finding__message">{{ finding.message }}</span>
							<span
								v-if="(finding.task_ids ?? []).length > 0"
								class="marshal-finding__tasks"
							>
								<RouterLink
									v-for="id in finding.task_ids"
									:key="id"
									:to="{name: 'task.detail', params: {id}}"
									class="marshal-task-link"
								>#{{ id }}</RouterLink>
							</span>
							<span
								v-if="(finding.paths ?? []).length > 0"
								class="marshal-finding__paths"
							>{{ (finding.paths ?? []).join(', ') }}</span>
						</li>
					</ul>
					<p
						v-else
						class="marshal-section__note"
					>
						{{ $t('project.marshal.noFindings') }}
					</p>
					<p
						v-if="health.checked_at"
						class="marshal-section__note"
					>
						{{ $t('project.marshal.checkedAt', {date: formatDateSince(health.checked_at)}) }}
					</p>
				</template>
				<p
					v-else
					class="marshal-section__note"
				>
					{{ $t('misc.loading') }}
				</p>
			</details>

			<details
				class="marshal-section"
				open
			>
				<summary class="marshal-section__head">
					<span>{{ $t('project.marshal.workers') }}</span>
					<span
						v-if="workers"
						class="marshal-section__count"
					>/ {{ agents.length }}</span>
				</summary>
				<p
					v-if="workersQuery.isError.value"
					class="marshal-section__note"
				>
					{{ $t('project.marshal.unreachable') }}
				</p>
				<template v-else-if="workers">
					<p
						v-if="agents.length === 0"
						class="marshal-section__note"
					>
						{{ $t('project.marshal.noWorkers') }}
					</p>
					<ul
						v-else
						class="marshal-workers"
					>
						<li
							v-for="agent in agents"
							:key="agent.user.id"
							class="marshal-worker"
						>
							<span class="marshal-worker__name">{{ agent.user.name || agent.user.username }}</span>
							<span
								v-if="marshalPersonName(agent.owner)"
								class="marshal-worker__owner"
							>{{ $t('project.marshal.forOwner', {user: marshalPersonName(agent.owner)}) }}</span>
							<span class="marshal-worker__stat">{{ $t('project.marshal.openTasks', {n: agent.open_tasks}) }}</span>
							<span class="marshal-worker__stat">{{ $t('project.marshal.heldLeases', {n: agent.leases}) }}</span>
						</li>
					</ul>
					<template v-if="allocations.length > 0">
						<h4 class="marshal-subhead">
							{{ $t('project.marshal.allocations') }}
						</h4>
						<ul class="marshal-allocations">
							<li
								v-for="allocation in allocations"
								:key="`${allocation.worker}-${allocation.task_id}-${allocation.checkout}`"
								class="marshal-allocation"
							>
								<span class="marshal-allocation__worker">{{ allocation.worker }}</span>
								<span aria-hidden="true">→</span>
								<span>{{ allocation.checkout }}</span>
								<span aria-hidden="true">→</span>
								<span>{{ allocation.branch }}</span>
								<span aria-hidden="true">→</span>
								<span>{{ allocation.database }}<template v-if="allocation.port">/{{ allocation.port }}</template></span>
								<RouterLink
									v-if="allocation.task_id"
									:to="{name: 'task.detail', params: {id: allocation.task_id}}"
									class="marshal-task-link"
								>
									{{ allocation.story || `#${allocation.task_id}` }}
								</RouterLink>
							</li>
						</ul>
					</template>
					<template v-if="conflicts.length > 0">
						<h4 class="marshal-subhead is-danger">
							{{ $t('project.marshal.conflicts') }}
						</h4>
						<ul class="marshal-conflicts">
							<li
								v-for="conflict in conflicts"
								:key="conflict.checkout"
								class="marshal-conflict"
							>
								<span class="marshal-conflict__checkout">{{ conflict.checkout }}</span>
								<span>{{ (conflict.workers ?? []).join(', ') }}</span>
								<span v-if="(conflict.missing ?? []).length > 0">
									{{ $t('project.marshal.missing', {paths: (conflict.missing ?? []).join(', ')}) }}
								</span>
							</li>
						</ul>
					</template>
				</template>
				<p
					v-else
					class="marshal-section__note"
				>
					{{ $t('misc.loading') }}
				</p>
			</details>

			<details
				class="marshal-section"
				open
			>
				<summary class="marshal-section__head">
					<span>{{ $t('project.marshal.chokepoints') }}</span>
					<span
						v-if="chokepoints"
						class="marshal-section__count"
					>/ {{ queues.length }}</span>
				</summary>
				<p
					v-if="chokepointsQuery.isError.value"
					class="marshal-section__note"
				>
					{{ $t('project.marshal.unreachable') }}
				</p>
				<template v-else-if="chokepoints">
					<p
						v-if="queues.length === 0"
						class="marshal-section__note"
					>
						{{ $t('project.marshal.noChokepoints') }}
					</p>
					<div
						v-for="queue in queues"
						:key="queue.chokepoint"
						class="marshal-queue"
					>
						<h4 class="marshal-subhead">
							{{ queue.chokepoint }}
						</h4>
						<ol class="marshal-queue__entries">
							<li
								v-for="entry in queue.entries ?? []"
								:key="`${entry.position}-${entry.claim.task_id}`"
								class="marshal-queue__entry"
							>
								<span class="marshal-queue__position">{{ entry.position }}</span>
								<RouterLink
									:to="{name: 'task.detail', params: {id: entry.claim.task_id}}"
									class="marshal-queue__task"
								>
									<span class="marshal-queue__id">{{ entry.claim.identifier || `#${entry.claim.task_id}` }}</span>
									{{ entry.claim.title }}
								</RouterLink>
								<span
									v-if="marshalPersonName(entry.claim.assignee)"
									class="marshal-queue__assignee"
								>{{ marshalPersonName(entry.claim.assignee) }}</span>
								<span
									class="marshal-chip"
									:class="{'is-ok': entry.claim.active}"
								>{{ entry.claim.active ? $t('project.marshal.active') : $t('project.marshal.declared') }}</span>
								<span
									v-if="(entry.waiting_on ?? []).length > 0"
									class="marshal-queue__waiting"
								>{{ $t('project.marshal.waitingOn', {ids: (entry.waiting_on ?? []).map(id => `#${id}`).join(', ')}) }}</span>
							</li>
						</ol>
					</div>
				</template>
				<p
					v-else
					class="marshal-section__note"
				>
					{{ $t('misc.loading') }}
				</p>
			</details>
		</Card>
	</Modal>
</template>

<script setup lang="ts">
import {computed, ref} from 'vue'
import {useI18n} from 'vue-i18n'
import {useQuery} from '@tanstack/vue-query'

import Card from '@/components/misc/Card.vue'

import {marshalPersonName} from '@/client/marshal'
import {marshalChokepointsQuery, marshalHealthQuery, marshalWorkersQuery} from '@/client/queries/marshal'
import {formatDateSince} from '@/helpers/time/formatDate'
import {useConfigStore} from '@/stores/config'

const {t} = useI18n({useScope: 'global'})
const configStore = useConfigStore()
const marshalUrl = computed(() => configStore.marshalUrl)

const open = ref(false)

// Health is cheap and feeds the button; the fleet views wait until the panel opens.
const healthQuery = useQuery(computed(() => marshalHealthQuery(marshalUrl.value)))
const workersQuery = useQuery(computed(() => ({
	...marshalWorkersQuery(marshalUrl.value),
	enabled: open.value && marshalUrl.value !== '',
})))
const chokepointsQuery = useQuery(computed(() => ({
	...marshalChokepointsQuery(marshalUrl.value),
	enabled: open.value && marshalUrl.value !== '',
})))

const health = computed(() => healthQuery.data.value)
const findings = computed(() => health.value?.findings ?? [])
const unblockedRoots = computed(() => health.value?.unblocked_roots ?? [])
const stats = computed(() => health.value ? [
	{label: t('project.marshal.openStories'), value: health.value.open_tasks},
	{label: t('project.marshal.collisions'), value: health.value.collisions},
	{label: t('project.marshal.cycles'), value: health.value.cycles},
	{label: t('project.marshal.unblockedRoots'), value: unblockedRoots.value.length},
	{label: t('project.marshal.leases'), value: health.value.leases},
] : [])

const workers = computed(() => workersQuery.data.value)
const agents = computed(() => workers.value?.agents ?? [])
const allocations = computed(() => workers.value?.allocations ?? [])
const conflicts = computed(() => workers.value?.conflicts ?? [])

const chokepoints = computed(() => chokepointsQuery.data.value)
const queues = computed(() => chokepoints.value?.queues ?? [])
</script>

<style lang="scss" scoped>
.project-marshal-button__count {
	@include mono-label;

	margin-inline-start: var(--wm-space-1);
	color: var(--wm-text-secondary);
}

.project-marshal-button.is-violated .project-marshal-button__count {
	color: var(--danger-text);
}

.project-marshal-panel {
	inline-size: min(44rem, 92vw);
}

.project-marshal-panel__hint {
	font-size: var(--wm-text-sm);
	color: var(--wm-text-secondary);
}

.marshal-section {
	margin-block-start: var(--wm-space-3);
	padding-block-start: var(--wm-space-2);
	border-block-start: 1px solid var(--wm-line);
}

.marshal-section__head {
	@include mono-label;

	display: flex;
	align-items: center;
	gap: var(--wm-space-2);
	color: var(--wm-text-secondary);
	cursor: pointer;

	&:focus-visible {
		@include focus-ring;
	}
}

.marshal-section__count {
	color: var(--wm-text-tertiary);
}

.marshal-section__note {
	margin: var(--wm-space-2) 0 0;
	font-size: var(--wm-text-xs);
	color: var(--wm-text-tertiary);
}

.marshal-chip {
	@include mono-label;

	display: inline-flex;
	align-items: center;
	padding: 1px var(--wm-space-1);
	color: var(--wm-text);
	border: 1px solid var(--wm-line-strong);

	&.is-ok {
		border-color: var(--success);
	}

	&.is-violated {
		border-color: var(--danger);
		color: var(--danger-text);
	}
}

.marshal-stats {
	display: grid;
	grid-template-columns: repeat(auto-fill, minmax(7rem, 1fr));
	gap: var(--wm-space-2);
	margin: var(--wm-space-2) 0 0;
}

.marshal-stat {
	margin: 0;
	padding: var(--wm-space-1) var(--wm-space-2);
	background: var(--wm-surface-sunken);
	border: 1px solid var(--wm-line-faint);

	dt {
		@include mono-label;

		color: var(--wm-text-tertiary);
	}

	dd {
		@include mono-data;

		margin: 0;
		font-size: var(--wm-text-lg);
		color: var(--wm-text);
	}
}

.marshal-roots {
	display: flex;
	flex-wrap: wrap;
	align-items: center;
	gap: var(--wm-space-1) var(--wm-space-2);
	margin: var(--wm-space-2) 0 0;
	font-size: var(--wm-text-xs);
}

.marshal-roots__label {
	@include mono-label;

	color: var(--wm-text-tertiary);
}

.marshal-task-link {
	@include mono-data;

	color: var(--wm-text);
	text-decoration: none;

	&:hover,
	&:focus-visible {
		color: var(--wm-accent-text);
	}
}

.marshal-findings,
.marshal-workers,
.marshal-allocations,
.marshal-conflicts,
.marshal-queue__entries {
	margin: var(--wm-space-2) 0 0;
	padding: 0;
	list-style: none;
}

.marshal-finding,
.marshal-worker,
.marshal-allocation,
.marshal-conflict,
.marshal-queue__entry {
	display: flex;
	flex-wrap: wrap;
	align-items: baseline;
	gap: var(--wm-space-1) var(--wm-space-2);
	padding-block: var(--wm-space-1);
	border-block-start: 1px solid var(--wm-line-faint);
	font-size: var(--wm-text-xs);
}

.marshal-finding__code {
	@include mono-label;

	padding: 0 var(--wm-space-1);
	background: transparent;
	color: var(--danger-text);
	border: 1px solid var(--danger);
}

.marshal-finding__message {
	color: var(--wm-text);
}

.marshal-finding__tasks {
	display: inline-flex;
	gap: var(--wm-space-1);
}

.marshal-finding__paths {
	@include mono-data;

	color: var(--wm-text-tertiary);
	word-break: break-all;
}

.marshal-worker__name {
	font-weight: 500;
	color: var(--wm-text);
}

.marshal-worker__owner,
.marshal-worker__stat {
	color: var(--wm-text-tertiary);
}

.marshal-worker__stat {
	@include mono-data;
}

.marshal-subhead {
	@include mono-label;

	margin: var(--wm-space-3) 0 0;
	color: var(--wm-text-secondary);

	&.is-danger {
		color: var(--danger-text);
	}
}

.marshal-allocation {
	@include mono-data;

	color: var(--wm-text-secondary);
}

.marshal-allocation__worker {
	font-weight: 500;
	color: var(--wm-text);
}

.marshal-conflict {
	padding-inline-start: var(--wm-space-2);
	border-inline-start: 2px solid var(--danger);
	color: var(--danger-text);
}

.marshal-conflict__checkout {
	@include mono-data;

	font-weight: 500;
}

.marshal-queue__position {
	@include mono-data;

	inline-size: 1.5rem;
	color: var(--wm-text-tertiary);
}

.marshal-queue__task {
	flex: 1 1 12rem;
	color: var(--wm-text);
	text-decoration: none;

	&:hover,
	&:focus-visible {
		color: var(--wm-accent-text);
	}
}

.marshal-queue__id {
	@include mono-data;

	margin-inline-end: var(--wm-space-1);
	color: var(--wm-text-tertiary);
}

.marshal-queue__assignee,
.marshal-queue__waiting {
	color: var(--wm-text-tertiary);
}
</style>
