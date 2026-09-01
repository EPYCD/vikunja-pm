<template>
	<div
		class="message-wrapper"
		:role="variant === 'danger' ? 'alert' : undefined"
	>
		<div
			class="message"
			:class="[variant, textAlignClass]"
		>
			<slot />
		</div>
	</div>
</template>

<script lang="ts" setup>
import {computed} from 'vue'

const props = withDefaults(defineProps<{
	variant?: 'info' | 'danger' | 'warning' | 'success',
	textAlign?: TextAlignVariant
}>(), {
	variant: 'info',
	textAlign: 'left',
})

const TEXT_ALIGN_MAP = {
	left: '',
	center: 'has-text-centered',
	right: 'has-text-end',
} as const

export type TextAlignVariant = keyof typeof TEXT_ALIGN_MAP

const textAlignClass = computed(() => TEXT_ALIGN_MAP[props.textAlign])
</script>

<style lang="scss" scoped>
// Status messages read as an annotated rule: a coloured rail on the leading
// edge, a faint wash, square corners.
.message-wrapper {
	background: transparent;
	border-radius: 0;
}

.message {
	padding: var(--wm-space-3) var(--wm-space-4);
	border-radius: 0;
	border: 1px solid var(--wm-line);
	border-inline-start-width: 2px;
	color: var(--wm-text);
}

.info {
	border-color: var(--wm-line);
	border-inline-start-color: var(--wm-accent);
	background: var(--wm-accent-wash);
}

.danger {
	border-color: var(--wm-line);
	border-inline-start-color: var(--danger);
	background: hsla(var(--wm-danger-h), var(--wm-danger-s), var(--wm-danger-l), .1);
}

.warning {
	border-color: var(--wm-line);
	border-inline-start-color: var(--warning);
	background: hsla(var(--wm-warning-h), var(--wm-warning-s), var(--wm-warning-l), .1);
}

.success {
	border-color: var(--wm-line);
	border-inline-start-color: var(--success);
	background: hsla(var(--wm-success-h), var(--wm-success-s), var(--wm-success-l), .1);
}
</style>
