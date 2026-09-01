<template>
	<div class="task-scope">
		<section
			v-for="list in lists"
			:key="list.key"
			class="scope-list"
		>
			<header class="scope-list__head">
				<span class="scope-eyebrow">{{ list.label }} / {{ list.items.length }}</span>
				<span class="scope-hint">{{ list.hint }}</span>
			</header>

			<ul
				v-if="list.items.length > 0"
				class="scope-chips"
			>
				<li
					v-for="entry in list.items"
					:key="entry"
					class="scope-chip"
					:class="{'is-leased': list.key === 'paths_owned' && leasedPatterns.has(entry)}"
				>
					<span
						v-if="list.key === 'paths_owned' && leasedPatterns.has(entry)"
						v-tooltip="$t('task.scope.leasedTooltip')"
						class="scope-chip__lock icon"
					>
						<Icon icon="lock" />
					</span>
					<span class="scope-chip__text">{{ entry }}</span>
					<BaseButton
						v-if="canWrite"
						class="scope-chip__remove"
						:aria-label="$t('task.scope.remove', {entry})"
						:disabled="saving"
						@click="removeEntry(list.key, entry)"
					>
						<Icon icon="xmark" />
					</BaseButton>
				</li>
			</ul>
			<p
				v-else
				class="scope-empty"
			>
				{{ $t('task.scope.none') }}
			</p>

			<form
				v-if="canWrite"
				class="scope-add"
				@submit.prevent="addEntry(list.key)"
			>
				<input
					v-model="drafts[list.key]"
					class="input scope-add__input"
					type="text"
					:placeholder="list.placeholder"
					:disabled="saving"
					spellcheck="false"
					autocomplete="off"
				>
				<XButton
					variant="secondary"
					type="submit"
					:disabled="saving || drafts[list.key].trim() === ''"
				>
					{{ $t('task.scope.add') }}
				</XButton>
			</form>
		</section>

		<section class="scope-list">
			<header class="scope-list__head">
				<span class="scope-eyebrow">{{ $t('task.scope.notes') }}</span>
				<span class="scope-hint">{{ $t('task.scope.notesHint') }}</span>
			</header>
			<textarea
				v-if="canWrite"
				v-model="notesDraft"
				class="textarea scope-notes"
				rows="2"
				:placeholder="$t('task.scope.notesPlaceholder')"
				:disabled="saving"
				@blur="saveNotes"
			/>
			<p
				v-else-if="scope.notes"
				class="scope-notes-static"
			>
				{{ scope.notes }}
			</p>
			<p
				v-else
				class="scope-empty"
			>
				{{ $t('task.scope.none') }}
			</p>
		</section>

		<section
			v-if="leases.length > 0"
			class="scope-list scope-leases"
		>
			<header class="scope-list__head">
				<span class="scope-eyebrow">{{ $t('task.scope.leases') }} / {{ leases.length }}</span>
				<span class="scope-hint">{{ $t('task.scope.leasesHint') }}</span>
			</header>
			<ul class="scope-lease-rows">
				<li
					v-for="lease in leases"
					:key="lease.id"
					class="scope-lease-row"
				>
					<span class="icon scope-chip__lock"><Icon icon="lock" /></span>
					<code class="scope-lease-row__pattern">{{ lease.pattern }}</code>
					<span class="scope-lease-row__meta">
						{{ $t('task.scope.leasedBy', {
							user: holderName(lease),
							date: formatDateSince(lease.created ? new Date(lease.created) : null),
						}) }}
					</span>
				</li>
			</ul>
			<XButton
				v-if="canWrite"
				variant="secondary"
				icon="unlink"
				:loading="releasing"
				@click="release"
			>
				{{ $t('task.scope.release') }}
			</XButton>
		</section>
	</div>
</template>

<script setup lang="ts">
import {computed, ref, watch} from 'vue'
import {useI18n} from 'vue-i18n'
import {useQuery} from '@tanstack/vue-query'

import BaseButton from '@/components/base/BaseButton.vue'

import type {TaskPathLease, TaskScope} from '@/client/generated'
import {
	emptyScope,
	projectLeasesQuery,
	taskScopeQuery,
	useReleaseTaskLeases,
	useUpdateTaskScope,
	type ScopeList,
} from '@/client/queries/taskScope'
import {formatDateSince} from '@/helpers/time/formatDate'
import {success} from '@/message'

const props = defineProps<{
	taskId: number,
	projectId: number,
	canWrite: boolean,
}>()

// Lets the parent decide whether to show the section without a second fetch.
const emit = defineEmits<{
	'loaded': [hasScope: boolean],
}>()

const {t} = useI18n({useScope: 'global'})

const scopeQuery = useQuery(computed(() => taskScopeQuery(props.taskId)))
const scope = computed<TaskScope>(() => scopeQuery.data.value ?? emptyScope(props.taskId))

const leasesQuery = useQuery(computed(() => projectLeasesQuery(props.projectId)))
const leases = computed<TaskPathLease[]>(() =>
	(leasesQuery.data.value ?? []).filter(lease => lease.task_id === props.taskId),
)
const leasedPatterns = computed(() => new Set(leases.value.map(lease => lease.pattern)))

watch(() => scopeQuery.data.value, (data) => {
	if (data) {
		emit('loaded', hasContent(data))
	}
}, {immediate: true})

function hasContent(s: TaskScope): boolean {
	return (s.paths_owned?.length ?? 0) > 0
		|| (s.paths_affected?.length ?? 0) > 0
		|| (s.endpoints?.length ?? 0) > 0
		|| (s.notes ?? '') !== ''
}

const drafts = ref<Record<ScopeList, string>>({paths_owned: '', paths_affected: '', endpoints: ''})
const notesDraft = ref('')
watch(() => scope.value.notes, (notes) => {
	notesDraft.value = notes ?? ''
}, {immediate: true})

const lists = computed(() => [
	{
		key: 'paths_owned' as ScopeList,
		label: t('task.scope.pathsOwned'),
		hint: t('task.scope.pathsOwnedHint'),
		placeholder: t('task.scope.addPath'),
		items: scope.value.paths_owned ?? [],
	},
	{
		key: 'paths_affected' as ScopeList,
		label: t('task.scope.pathsAffected'),
		hint: t('task.scope.pathsAffectedHint'),
		placeholder: t('task.scope.addPath'),
		items: scope.value.paths_affected ?? [],
	},
	{
		key: 'endpoints' as ScopeList,
		label: t('task.scope.endpoints'),
		hint: t('task.scope.endpointsHint'),
		placeholder: t('task.scope.addEndpoint'),
		items: scope.value.endpoints ?? [],
	},
])

const update = useUpdateTaskScope(() => props.taskId, () => props.projectId)
const releaseMutation = useReleaseTaskLeases(() => props.taskId, () => props.projectId)
const saving = computed(() => update.isPending.value)
const releasing = computed(() => releaseMutation.isPending.value)

async function save(next: Partial<TaskScope>) {
	const body = {
		paths_owned: scope.value.paths_owned ?? [],
		paths_affected: scope.value.paths_affected ?? [],
		endpoints: scope.value.endpoints ?? [],
		notes: scope.value.notes ?? '',
		...next,
	}
	await update.mutateAsync(body)
	success({message: t('task.scope.saved')})
}

async function addEntry(key: ScopeList) {
	const value = drafts.value[key].trim()
	if (value === '') {
		return
	}
	const current = scope.value[key] ?? []
	if (current.includes(value)) {
		drafts.value[key] = ''
		return
	}
	await save({[key]: [...current, value]})
	drafts.value[key] = ''
}

async function removeEntry(key: ScopeList, entry: string) {
	await save({[key]: (scope.value[key] ?? []).filter(e => e !== entry)})
}

async function saveNotes() {
	if (notesDraft.value === (scope.value.notes ?? '')) {
		return
	}
	await save({notes: notesDraft.value})
}

function holderName(lease: TaskPathLease): string {
	return lease.user?.name || lease.user?.username || `#${lease.user_id}`
}

async function release() {
	await releaseMutation.mutateAsync()
	success({message: t('task.scope.released')})
}
</script>

<style lang="scss" scoped>
.task-scope {
	display: flex;
	flex-direction: column;
	gap: var(--wm-space-4);
}

.scope-list__head {
	display: flex;
	flex-wrap: wrap;
	align-items: baseline;
	gap: var(--wm-space-1) var(--wm-space-3);
	margin-block-end: var(--wm-space-2);
}

.scope-eyebrow {
	@include mono-label;

	color: var(--wm-text-secondary);
}

.scope-hint,
.scope-empty {
	font-size: var(--wm-text-xs);
	color: var(--wm-text-tertiary);
}

.scope-empty {
	margin: 0;
}

.scope-chips {
	display: flex;
	flex-wrap: wrap;
	gap: var(--wm-space-1);
	margin: 0;
	padding: 0;
	list-style: none;
}

.scope-chip {
	position: relative;
	display: inline-flex;
	align-items: center;
	gap: var(--wm-space-1);
	padding: 2px var(--wm-space-2);
	font-family: $family-monospace;
	font-size: var(--wm-text-xs);
	color: var(--wm-text);
	background: var(--wm-surface-sunken);

	@include chamfer(var(--wm-chamfer-sm), bottom-right);
	@include chamfer-outline(var(--wm-line));

	&.is-leased {
		background: var(--wm-accent-wash);

		@include chamfer-outline(var(--wm-accent-line));
	}
}

.scope-chip__lock {
	color: var(--wm-accent-text);
	font-size: var(--wm-text-2xs);
}

.scope-chip__text {
	word-break: break-all;
}

.scope-chip__remove {
	display: inline-flex;
	align-items: center;
	color: var(--wm-text-tertiary);
	transition: color var(--wm-duration-fast) var(--wm-ease);

	&:hover,
	&:focus-visible {
		color: var(--wm-text);
	}
}

.scope-add {
	display: flex;
	gap: var(--wm-space-2);
	margin-block-start: var(--wm-space-2);
}

.scope-add__input {
	flex: 1;
	font-family: $family-monospace;
	font-size: var(--wm-text-xs);
}

.scope-notes {
	font-size: var(--wm-text-sm);
}

.scope-notes-static {
	margin: 0;
	font-size: var(--wm-text-sm);
	white-space: pre-wrap;
}

.scope-lease-rows {
	margin: 0 0 var(--wm-space-2);
	padding: 0;
	list-style: none;
}

.scope-lease-row {
	display: flex;
	flex-wrap: wrap;
	align-items: center;
	gap: var(--wm-space-2);
	padding-block: var(--wm-space-1);
	border-block-end: 1px solid var(--wm-line-faint);
	font-size: var(--wm-text-xs);
}

.scope-lease-row__pattern {
	background: transparent;
	color: var(--wm-text);
	padding: 0;
}

.scope-lease-row__meta {
	color: var(--wm-text-tertiary);
}
</style>
