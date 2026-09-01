<template>
	<span
		v-if="checklist.total > 0"
		class="checklist-summary"
	>
		<svg
			width="12"
			height="12"
			:class="{'is-all-done': allDone}"
		>
			<circle
				stroke-width="2"
				fill="transparent"
				cx="50%"
				cy="50%"
				r="5"
			/>
			<circle
				stroke-width="2"
				stroke-dasharray="31"
				:stroke-dashoffset="checklistCircleDone"
				stroke-linecap="round"
				fill="transparent"
				cx="50%"
				cy="50%"
				r="5"
			/>
		</svg>
		<span>{{ label }}</span>
	</span>
</template>

<script setup lang="ts">
import {computed} from 'vue'
import { useI18n } from 'vue-i18n'

import {getChecklistStatistics} from '@/helpers/checklistFromText'
import type {ITask} from '@/modelTypes/ITask'

const props = defineProps<{
	task: ITask
}>()

const checklist = computed(() => getChecklistStatistics(props.task.description))

const checklistCircleDone = computed(() => {
	const r = 5
	const c = Math.PI * (r * 2)

	const progress = checklist.value.checked / checklist.value.total * 100

	return ((100 - progress) / 100) * c
})

const allDone = computed(() => checklist.value.total === checklist.value.checked)

const {t} = useI18n({useScope: 'global'})
const label = computed(() => {
	return allDone.value
		? t('task.checklistAllDone', checklist.value)
		: t('task.checklistTotal', checklist.value)
})
</script>

<style scoped lang="scss">
.checklist-summary {
	@include mono-data;

	color: var(--wm-text-tertiary);
	display: inline-flex;
	align-items: center;
	padding-inline-start: var(--wm-space-2);
	font-size: var(--wm-text-2xs);
}

svg {
	transform: rotate(-90deg);
	transition: stroke-dashoffset var(--wm-duration-slow) var(--wm-ease);
	margin-inline-end: var(--wm-space-1);

	@media (prefers-reduced-motion: reduce) {
		transition: none;
	}
}

circle {
	stroke: var(--wm-line-strong);

	&:last-child {
		stroke: var(--wm-accent);
	}
}

svg.is-all-done circle {
	&:last-child {
		stroke: var(--success);
	}
}
</style>
