<template>
	<section
		v-if="receipts.length > 0"
		class="task-receipts"
	>
		<h2 class="task-section-title">
			<span class="icon is-grey">
				<Icon icon="code-merge" />
			</span>
			{{ $t('task.receipts.title') }}
		</h2>
		<ul class="task-receipt-rows">
			<li
				v-for="receipt in receipts"
				:key="receipt.id ?? receipt.commit_sha"
				class="task-receipt-row"
				:class="receipt.passed ? 'is-passed' : 'is-failed'"
			>
				<div class="task-receipt-row__head">
					<span class="task-receipt-row__status">
						{{ receipt.passed ? $t('task.receipts.passed') : $t('task.receipts.failed') }}
					</span>
					<code class="task-receipt-row__commit">{{ shortSha(receipt.commit_sha) }}</code>
					<span
						v-if="receipt.branch"
						class="task-receipt-row__branch"
					>{{ receipt.branch }}</span>
					<a
						v-if="receipt.ci_run_url"
						:href="receipt.ci_run_url"
						target="_blank"
						rel="noopener noreferrer"
						class="task-receipt-row__ci"
					>
						{{ $t('task.receipts.ciRun') }}
						<span class="icon is-small"><Icon icon="arrow-up-right-from-square" /></span>
					</a>
				</div>
				<ul class="task-receipt-row__gates">
					<li
						v-for="(gate, index) in receipt.gates ?? []"
						:key="`${gate.name}-${index}`"
						class="task-receipt-gate"
						:class="`is-${gate.status ?? 'skipped'}`"
					>
						<span class="task-receipt-gate__name">{{ gate.name }}</span>
						<span
							class="task-receipt-gate__mark"
							role="img"
							:aria-label="gateStatusLabel(gate.status)"
						>{{ GATE_MARKS[gate.status ?? 'skipped'] }}</span>
						<span class="task-receipt-gate__duration">{{ formatGateDuration(gate.duration_ms) }}</span>
					</li>
				</ul>
				<div class="task-receipt-row__meta">
					<span
						class="task-receipt-row__docs"
						:class="{'is-missing': receipt.docs_api_required && !receipt.docs_api_regenerated}"
					>{{ docsState(receipt) }}</span>
					<span
						class="task-receipt-row__merged"
						:class="{'is-merged': receipt.merged}"
					>
						{{ receipt.merged ? $t('task.receipts.merged') : $t('task.receipts.notMerged') }}
						<code v-if="receipt.merged && receipt.merge_sha">{{ shortSha(receipt.merge_sha) }}</code>
					</span>
					<span class="task-receipt-row__posted">
						{{ $t('task.receipts.postedBy', {user: posterName(receipt), date: formatDateSince(receipt.created ?? null)}) }}
					</span>
				</div>
			</li>
		</ul>
	</section>
</template>

<script setup lang="ts">
import {computed, watch} from 'vue'
import {useI18n} from 'vue-i18n'
import {useQuery} from '@tanstack/vue-query'

import type {GateResult, TaskReceipt} from '@/client/generated'
import {formatGateDuration, taskReceiptsQuery} from '@/client/queries/taskReceipts'
import {formatDateSince} from '@/helpers/time/formatDate'

const props = defineProps<{
	taskId: number,
}>()

// Lets the parent hide the section's frame without a second fetch.
const emit = defineEmits<{
	'loaded': [hasReceipts: boolean],
}>()

const {t} = useI18n({useScope: 'global'})

const query = useQuery(computed(() => ({
	...taskReceiptsQuery(props.taskId),
	enabled: props.taskId > 0,
})))
const receipts = computed<TaskReceipt[]>(() => query.data.value ?? [])

watch(() => query.data.value, (data) => {
	if (data) {
		emit('loaded', data.length > 0)
	}
}, {immediate: true})

const GATE_MARKS: Record<NonNullable<GateResult['status']>, string> = {
	passed: '✓',
	failed: '✗',
	skipped: '⏭',
}

function gateStatusLabel(status: GateResult['status']): string {
	switch (status) {
		case 'passed':
			return t('task.receipts.gatePassed')
		case 'failed':
			return t('task.receipts.gateFailed')
		default:
			return t('task.receipts.gateSkipped')
	}
}

function shortSha(sha: string | undefined): string {
	return (sha ?? '').slice(0, 7)
}

function docsState(receipt: TaskReceipt): string {
	if (!receipt.docs_api_required) {
		return t('task.receipts.docsNotNeeded')
	}
	return receipt.docs_api_regenerated
		? t('task.receipts.docsRegenerated')
		: t('task.receipts.docsMissing')
}

function posterName(receipt: TaskReceipt): string {
	return receipt.posted_by?.name || receipt.posted_by?.username || ''
}
</script>

<style lang="scss" scoped>
.task-receipts {
	inline-size: 100%;
}

.task-receipt-rows {
	margin: 0;
	padding: 0;
	list-style: none;
}

.task-receipt-row {
	display: flex;
	flex-direction: column;
	gap: var(--wm-space-2);
	padding: var(--wm-space-2) var(--wm-space-3);
	border-block-end: 1px solid var(--wm-line-faint);
	border-inline-start: 2px solid var(--wm-line-strong);
	font-size: var(--wm-text-xs);

	&.is-passed {
		border-inline-start-color: var(--success);
	}

	&.is-failed {
		border-inline-start-color: var(--danger);
		background: var(--danger-light);
	}
}

.task-receipt-row__head {
	display: flex;
	flex-wrap: wrap;
	align-items: center;
	gap: var(--wm-space-2) var(--wm-space-3);
}

.task-receipt-row__status {
	@include mono-label;

	display: inline-flex;
	align-items: center;
	gap: var(--wm-space-1);
	padding: 1px var(--wm-space-1);
	color: var(--wm-text);
	border: 1px solid var(--wm-line-strong);

	&::before {
		content: '';
		inline-size: 6px;
		block-size: 6px;
	}

	.is-passed & {
		border-color: var(--success);

		&::before {
			background: var(--success);
		}
	}

	.is-failed & {
		border-color: var(--danger);

		&::before {
			background: var(--danger);
		}
	}
}

.task-receipt-row__commit,
.task-receipt-row__merged code {
	@include mono-data;

	padding: 0;
	background: transparent;
	color: var(--wm-text);
}

.task-receipt-row__branch {
	@include mono-data;

	color: var(--wm-text-secondary);
	word-break: break-all;
}

.task-receipt-row__ci {
	margin-inline-start: auto;
	color: var(--wm-text-secondary);
	transition: color var(--wm-duration-fast) var(--wm-ease);

	&:hover,
	&:focus-visible {
		color: var(--wm-accent-text);
	}

	.icon {
		font-size: var(--wm-text-2xs);
	}
}

.task-receipt-row__gates {
	display: flex;
	flex-wrap: wrap;
	gap: var(--wm-space-1);
	margin: 0;
	padding: 0;
	list-style: none;
}

.task-receipt-gate {
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

	&.is-passed .task-receipt-gate__mark {
		color: var(--success);
	}

	&.is-failed {
		background: var(--danger-light);

		@include chamfer-outline(var(--danger));

		.task-receipt-gate__mark {
			color: var(--danger-text);
		}
	}

	&.is-skipped {
		color: var(--wm-text-tertiary);
	}
}

.task-receipt-gate__duration {
	@include mono-data;

	color: var(--wm-text-tertiary);
}

.task-receipt-row__meta {
	display: flex;
	flex-wrap: wrap;
	gap: var(--wm-space-1) var(--wm-space-3);
	color: var(--wm-text-tertiary);
}

.task-receipt-row__docs.is-missing {
	color: var(--danger-text);
}

.task-receipt-row__merged.is-merged {
	color: var(--wm-text-secondary);
}
</style>
