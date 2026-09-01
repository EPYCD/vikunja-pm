<template>
	<div
		class="gantt-timeline"
		role="columnheader"
		:aria-label="$t('project.gantt.timelineHeader')"
	>
		<!-- Upper timeunit for months -->
		<div
			class="gantt-timeline-months"
			role="row"
			:aria-label="$t('project.gantt.monthsRow')"
		>
			<div
				v-for="monthGroup in monthGroups"
				:key="monthGroup.key"
				class="timeunit-month"
				:style="{ width: `${monthGroup.width}px` }"
				role="columnheader"
				:aria-label="$t('project.gantt.monthLabel', {month: monthGroup.label})"
			>
				{{ monthGroup.label }}
			</div>
		</div>
        
		<!-- Lower timeunit for days -->
		<div
			class="gantt-timeline-days"
			role="row"
			:aria-label="$t('project.gantt.daysRow')"
		>
			<div
				v-for="date in timelineData"
				:key="date.toISOString()"
				class="timeunit"
				:style="{ width: `${dayWidthPixels}px` }"
				role="columnheader"
				:aria-label="dateIsToday(date) 
					? $t('project.gantt.dayLabelToday', {
						date: date.toLocaleDateString(),
						weekday: weekDayFromDate(date)
					})
					: $t('project.gantt.dayLabel', {
						date: date.toLocaleDateString(),
						weekday: weekDayFromDate(date)
					})"
			>
				<div
					class="timeunit-wrapper"
					:class="{'today': dateIsToday(date)}"
				>
					<span>{{ date.getDate() }}</span>
					<span class="weekday">
						{{ weekDayFromDate(date) }}
					</span>
				</div>
			</div>
		</div>
	</div>
</template>

<script setup lang="ts">
import {computed} from 'vue'
import {useGlobalNow} from '@/composables/useGlobalNow'
import {useWeekDayFromDate} from '@/helpers/time/formatDate'
import dayjs from 'dayjs'

const props = defineProps<{
    timelineData: Date[]
    dayWidthPixels: number
}>()

const weekDayFromDate = useWeekDayFromDate()
const { now: today } = useGlobalNow()

const dateIsToday = computed(() => {
	const todayStr = today.value.toDateString()
	return (date: Date) => date.toDateString() === todayStr
})

const monthGroups = computed(() => {
	const groups = props.timelineData.reduce(
		(groups, date) => {
			const month = date.getMonth()
			const year = date.getFullYear()
			const key = `${year}-${month}`

			const lastGroup = groups[groups.length - 1]
			if (lastGroup?.key === key) {
				lastGroup.width += props.dayWidthPixels
			} else {
				groups.push({
					key,
					label: dayjs(date).format('MMMM YYYY'),
					width: props.dayWidthPixels,
				})
			}

			return groups
		},
		[] as Array<{key: string; label: string; width: number}>,
	)

	return groups
})
</script>

<style scoped lang="scss">
// The date axis is an instrument scale: mono figures, hairline ticks, and the
// accent reserved for today.
.gantt-timeline {
	background: var(--wm-surface);
	border-block-end: 1px solid var(--wm-line);
	position: sticky;
	inset-block-start: 0;
	z-index: 10;
}

.gantt-timeline-months {
	display: flex;

	.timeunit-month {
		@include mono-label;

		background: var(--wm-surface-sunken);
		color: var(--wm-text-tertiary);
		border-inline-end: 1px solid var(--wm-line);
		border-block-end: 1px solid var(--wm-line);
		padding: var(--wm-space-2) 0;
		text-align: center;
	}
}

.gantt-timeline-days {
	display: flex;

	.timeunit {
		.timeunit-wrapper {
			@include mono-data;

			padding: var(--wm-space-2) 0;
			font-size: var(--wm-text-xs);
			color: var(--wm-text-secondary);
			border-inline-end: 1px solid var(--wm-line-faint);
			display: flex;
			flex-direction: column;
			align-items: center;
			inline-size: 100%;

			&.today {
				background: var(--wm-accent);
				color: var(--wm-on-accent);
				border-inline-end-color: var(--wm-accent);
				font-weight: 600;
			}

			.weekday {
				@include mono-label;

				color: inherit;
				opacity: .8;
			}
		}
	}
}
</style>
