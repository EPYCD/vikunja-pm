<template>
	<span
		v-if="!done && (showAll || priority >= minimumPriority)"
		:class="{
			'negligible': priority <= priorities.LOW,
			'not-so-high': priority > priorities.LOW && priority < priorities.HIGH,
			'high-priority': priority >= priorities.HIGH
		}"
		class="priority-label"
	>
		<span class="icon">
			<Icon
				v-if="priority >= priorities.HIGH"
				icon="exclamation-circle"
			/>
			<Icon
				v-else
				icon="exclamation"
			/>
		</span>
		<span>
			<template v-if="priority === priorities.UNSET">{{ $t('task.priority.unset') }}</template>
			<template v-if="priority === priorities.LOW">{{ $t('task.priority.low') }}</template>
			<template v-if="priority === priorities.MEDIUM">{{ $t('task.priority.medium') }}</template>
			<template v-if="priority === priorities.HIGH">{{ $t('task.priority.high') }}</template>
			<template v-if="priority === priorities.URGENT">{{ $t('task.priority.urgent') }}</template>
			<template v-if="priority === priorities.DO_NOW">{{ $t('task.priority.doNow') }}</template>
		</span>
	</span>
</template>

<script setup lang="ts">
import {computed} from 'vue'
import {PRIORITIES as priorities} from '@/constants/priorities'
import {useAuthStore} from '@/stores/auth'
	
withDefaults(defineProps<{
	priority: number,
	showAll?: boolean,
	done?: boolean
}>(), {
	showAll: false,
	done: false,
})

const authStore = useAuthStore()

const minimumPriority = computed(() => {
	return authStore.settings.frontendSettings.minimumPriority || priorities.MEDIUM
})
</script>

<style lang="scss" scoped>
// A mono status chip: the level color carries the hairline and the wash, while
// the text stays on a contrast-safe ramp. Only HIGH and above take a colored
// label — everything below would be two things shouting at once.
.priority-label {
	@include mono-label;

	--priority-color: var(--wm-line-strong);

	position: relative;
	display: inline-flex;
	align-items: center;
	gap: var(--wm-space-1);
	min-block-size: 1.375rem;
	padding-inline: var(--wm-space-2);
	border: 1px solid var(--priority-color);
	border-radius: var(--wm-radius-xs);
	line-height: 1;
	color: var(--wm-text-secondary);
	inline-size: auto;
	white-space: nowrap;

	&::before {
		content: '';
		position: absolute;
		inset: 0;
		background: var(--priority-color);
		opacity: 0.12;
		pointer-events: none;
	}

	> span {
		position: relative;
	}
}

.high-priority {
	--priority-color: var(--danger);

	color: var(--danger-text);
	inline-size: auto !important; // To override the width set in tasks
}

.not-so-high {
	--priority-color: var(--warning);

	color: var(--wm-text);
}

.negligible {
	--priority-color: var(--wm-line-strong);

	color: var(--wm-text-tertiary);
}

.icon {
	vertical-align: top;
	inline-size: auto !important;
	font-size: var(--wm-text-2xs);
}
</style>
