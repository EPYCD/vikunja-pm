<template>
	<div
		v-if="isDone"
		class="is-done"
		:class="{ 'is-done--small': variant === 'small' }"
	>
		{{ $t('task.attributes.done') }}
	</div>
</template>

<script lang="ts" setup>
withDefaults(defineProps<{
	isDone?: boolean
	variant?: 'default' | 'small'
}>(), {
	isDone: false,
	variant: 'default',
})
</script>


<style lang="scss" scoped>
// A status chip in the same language as labels and priority: hairline outline,
// tinted wash, mono uppercase. A saturated green fill fought the crimson
// identity and read louder than the accent it sat next to.
.is-done {
	@include mono-label;

	display: inline-flex;
	align-items: center;
	padding: .2rem .4rem;
	border: 1px solid var(--success);
	border-radius: var(--wm-radius-xs);
	background: hsla(var(--wm-success-h), var(--wm-success-s), var(--wm-success-l), .14);
	color: var(--success);
	line-height: 1;
	text-align: center;
}

.is-done--small {
	padding: .15rem .3rem;
}
</style>
