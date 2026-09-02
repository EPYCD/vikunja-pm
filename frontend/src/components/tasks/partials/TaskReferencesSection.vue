<template>
	<section class="task-references">
		<h2 class="task-section-title">
			<span class="icon is-grey">
				<Icon icon="quote-right" />
			</span>
			{{ $t('task.references.title') }}
		</h2>
		<p
			v-if="query.isError.value"
			class="task-references__note"
		>
			{{ $t('task.references.unreachable') }}
		</p>
		<template v-else-if="query.data.value">
			<ul
				v-if="resolutions.length > 0"
				class="task-reference-list"
			>
				<li
					v-for="resolution in resolutions"
					:key="refKey(resolution)"
					class="task-reference"
					:class="{'is-broken': !resolution.found || !resolution.anchor}"
				>
					<blockquote
						v-if="resolution.found && resolution.anchor"
						class="task-reference__quote"
					>
						<header class="task-reference__head">
							<span class="task-reference__id">{{ resolution.anchor.id }}</span>
							<span
								v-if="resolution.anchor.title"
								class="task-reference__title"
							>— {{ resolution.anchor.title }}</span>
						</header>
						<pre
							class="task-reference__text"
							:class="{'is-clamped': needsClamp(resolution.anchor.text) && !expanded.has(refKey(resolution))}"
						>{{ resolution.anchor.text }}</pre>
						<BaseButton
							v-if="needsClamp(resolution.anchor.text)"
							class="task-reference__toggle"
							:aria-expanded="expanded.has(refKey(resolution))"
							@click="toggle(refKey(resolution))"
						>
							{{ expanded.has(refKey(resolution)) ? $t('task.references.collapse') : $t('task.references.expand') }}
						</BaseButton>
						<footer
							v-if="resolution.provenance"
							class="task-reference__provenance"
						>
							{{ resolution.provenance }}
						</footer>
					</blockquote>
					<p
						v-else
						class="task-reference__broken"
					>
						<span class="icon"><Icon icon="exclamation-circle" /></span>
						{{ $t('task.references.broken', {ref: refLabel(resolution.ref)}) }}
					</p>
				</li>
			</ul>
			<div
				v-if="pastes.length > 0"
				class="task-references__pastes"
				role="note"
			>
				<header class="task-references__pastes-head">
					{{ $t('task.references.pastes', {n: pastes.length}) }}
				</header>
				<p class="task-references__pastes-hint">
					{{ $t('task.references.pastesHint') }}
				</p>
				<ul class="task-reference-pastes">
					<li
						v-for="(paste, index) in pastes"
						:key="`${paste.file}:${paste.line}:${index}`"
						class="task-reference-paste"
					>
						<span class="task-reference-paste__where">
							{{ $t('task.references.pasteLine', {file: paste.file, line: paste.line, words: paste.words}) }}
						</span>
						<pre
							v-if="paste.excerpt"
							class="task-reference-paste__excerpt"
						>{{ paste.excerpt }}</pre>
					</li>
				</ul>
			</div>
			<p
				v-if="resolutions.length === 0 && pastes.length === 0"
				class="task-references__note"
			>
				{{ $t('task.references.empty') }}
			</p>
		</template>
	</section>
</template>

<script setup lang="ts">
import {computed, reactive} from 'vue'
import {useQuery} from '@tanstack/vue-query'

import BaseButton from '@/components/base/BaseButton.vue'

import type {MarshalPaste, MarshalRef, MarshalResolution} from '@/client/marshal'
import {taskReferencesQuery} from '@/client/queries/marshal'
import {useConfigStore} from '@/stores/config'

const props = defineProps<{
	taskId: number,
}>()

const configStore = useConfigStore()

const query = useQuery(computed(() => taskReferencesQuery(configStore.marshalUrl, props.taskId)))
const resolutions = computed<MarshalResolution[]>(() => query.data.value?.resolutions ?? [])
const pastes = computed<MarshalPaste[]>(() => query.data.value?.pastes ?? [])

const CLAMP_LINES = 8
const CLAMP_CHARS = 640

function needsClamp(text: string): boolean {
	return text.split('\n').length > CLAMP_LINES || text.length > CLAMP_CHARS
}

const expanded = reactive(new Set<string>())

function toggle(key: string) {
	if (expanded.has(key)) {
		expanded.delete(key)
	} else {
		expanded.add(key)
	}
}

function refKey(resolution: MarshalResolution): string {
	return `${resolution.ref.prefix}:${resolution.ref.id}`
}

// Marshal sometimes sends the bare number and sometimes the full "FR-12".
function refLabel(ref: MarshalRef): string {
	if (!ref.prefix || ref.id.startsWith(ref.prefix)) {
		return ref.id
	}
	return `${ref.prefix}-${ref.id}`
}
</script>

<style lang="scss" scoped>
.task-references {
	inline-size: 100%;
}

.task-references__note {
	margin: 0;
	font-size: var(--wm-text-xs);
	color: var(--wm-text-tertiary);
}

.task-reference-list {
	display: flex;
	flex-direction: column;
	gap: var(--wm-space-2);
	margin: 0;
	padding: 0;
	list-style: none;
}

.task-reference__quote {
	margin: 0;
	padding: var(--wm-space-2) var(--wm-space-3);
	background: var(--wm-surface-sunken);
	border-inline-start: 2px solid var(--wm-line-strong);
	color: var(--wm-text);
	font-size: var(--wm-text-sm);
}

.task-reference__head {
	display: flex;
	flex-wrap: wrap;
	align-items: baseline;
	gap: var(--wm-space-1);
	margin-block-end: var(--wm-space-1);
}

.task-reference__id {
	@include mono-data;

	color: var(--wm-text-secondary);
}

.task-reference__title {
	font-weight: 500;
}

.task-reference__text {
	margin: 0;
	padding: 0;
	background: transparent;
	color: var(--wm-text);
	font-family: inherit;
	font-size: inherit;
	line-height: 1.5;
	white-space: pre-wrap;
	overflow-wrap: anywhere;

	&.is-clamped {
		display: -webkit-box;
		-webkit-line-clamp: 8;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
}

.task-reference__toggle {
	@include mono-label;

	margin-block-start: var(--wm-space-1);
	color: var(--wm-text-secondary);
	transition: color var(--wm-duration-fast) var(--wm-ease);

	&:hover,
	&:focus-visible {
		color: var(--wm-accent-text);
	}
}

.task-reference__provenance {
	@include mono-data;

	margin-block-start: var(--wm-space-1);
	font-size: var(--wm-text-2xs);
	color: var(--wm-text-tertiary);
}

.task-reference__broken {
	display: flex;
	align-items: center;
	gap: var(--wm-space-2);
	margin: 0;
	padding: var(--wm-space-2) var(--wm-space-3);
	border: 1px solid var(--danger);
	background: var(--danger-light);
	color: var(--danger-text);
	font-size: var(--wm-text-sm);
	font-weight: 500;

	.icon {
		color: var(--danger-text);
	}
}

.task-references__pastes {
	margin-block-start: var(--wm-space-3);
	padding: var(--wm-space-2) var(--wm-space-3);
	border: 1px solid var(--warning);
	background: var(--warning-light);
}

.task-references__pastes-head {
	@include mono-label;

	color: var(--wm-text);
}

.task-references__pastes-hint {
	margin: var(--wm-space-1) 0 var(--wm-space-2);
	font-size: var(--wm-text-xs);
	color: var(--wm-text-secondary);
}

.task-reference-pastes {
	margin: 0;
	padding: 0;
	list-style: none;
}

.task-reference-paste {
	padding-block: var(--wm-space-1);
	border-block-start: 1px solid var(--wm-line-faint);
	font-size: var(--wm-text-xs);
}

.task-reference-paste__where {
	@include mono-data;

	color: var(--wm-text-secondary);
}

.task-reference-paste__excerpt {
	margin: var(--wm-space-1) 0 0;
	padding: 0;
	background: transparent;
	color: var(--wm-text-tertiary);
	font-family: inherit;
	font-size: inherit;
	white-space: pre-wrap;
	overflow-wrap: anywhere;
}
</style>
