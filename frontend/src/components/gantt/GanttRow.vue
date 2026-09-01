<template>
	<GanttRowPrimitive
		:id="id"
		@focus="onFocus"
		@select="onSelect"
	>
		<div
			class="w-full flex items-center"
			:class="index % 2 ? 'bg-row-alt' : 'bg-row'"
			role="presentation"
		>
			<slot />
		</div>
	</GanttRowPrimitive>
</template>

<script setup lang="ts">
import GanttRowPrimitive from '@/components/gantt/primitives/GanttRowPrimitive.vue'

const props = defineProps<{ 
	id: string
	index: number 
}>()
const emit = defineEmits<{
  (e: 'focus', id: string): void
  (e: 'select', id: string): void
}>()
const onFocus = () => emit('focus', props.id)
const onSelect = () => emit('select', props.id)
</script>

<style scoped lang="scss">
// Rows are separated by a hairline, not a fill; the alternate band is only
// just visible so the eye can track across the timeline.
.bg-row,
.bg-row-alt {
	border-block-end: 1px solid var(--wm-line-faint);
}

.bg-row-alt {
	background: var(--wm-surface-sunken);
}
</style>
