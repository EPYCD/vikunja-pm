<template>
	<component
		:is="is"
		class="shortcuts"
	>
		<template
			v-for="(k, i) in keys"
			:key="i"
		>
			<kbd>{{ k }}</kbd>
			<span v-if="i < keys.length - 1">{{ combination }}</span>
		</template>
	</component>
</template>

<script lang="ts" setup>
withDefaults(defineProps<{
	keys: string[],
	combination?: string,
	is?: string
}>(), {
	combination: '+',
	is: 'div',
})
</script>

<style lang="scss" scoped>
.shortcuts {
	display: flex;
	align-items: center;
}

kbd {
	@include mono-label(var(--wm-text-2xs));

	padding: .15rem .4rem;
	border: 1px solid var(--wm-line-strong);
	background: var(--wm-surface-sunken);
	border-radius: var(--wm-radius-xs);
	color: var(--wm-text-secondary);
	text-transform: none;
}

span {
	padding-inline: var(--wm-space-1);
	color: var(--wm-text-tertiary);
}
</style>
