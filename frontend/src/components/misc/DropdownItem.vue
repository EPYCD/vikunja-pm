<template>
	<BaseButton
		class="dropdown-item"
		:disabled="disabled"
		:class="{'is-disabled': disabled}"
	>
		<span
			v-if="icon"
			class="icon is-small"
			:class="iconClass"
		>
			<Icon :icon="icon" />
		</span>
		<span>
			<slot />
		</span>
	</BaseButton>
</template>

<script lang="ts" setup>
import BaseButton, {type BaseButtonProps} from '@/components/base//BaseButton.vue'
import Icon from '@/components/misc/Icon'
import type {IconProp} from '@fortawesome/fontawesome-svg-core'

export interface DropDownItemProps extends /* @vue-ignore */ BaseButtonProps {
	icon?: IconProp,
	iconClass?: object | string,
	disabled?: boolean,
}

defineProps<DropDownItemProps>()
</script>

<style scoped lang="scss">
.dropdown-item {
	color: var(--wm-text-secondary);
	font-size: var(--wm-text-sm);
	line-height: 1.5;
	padding: var(--wm-space-2) var(--wm-space-3);
	position: relative;
	text-align: inherit;
	white-space: nowrap;
	inline-size: 100%;
	display: flex;
	align-items: center;
	justify-content: left !important;

	// Active items take the accent rail + wash, not a filled block.
	&.is-active {
		background-color: var(--wm-accent-wash);
		color: var(--wm-text);
		box-shadow: inset 2px 0 0 var(--wm-accent);
	}

	&:hover:not(.is-disabled) {
		background-color: var(--wm-surface-hover);
		color: var(--wm-text);
	}

	&.is-disabled {
		cursor: not-allowed;
	}
}

.icon {
	padding-inline-end: var(--wm-space-2);
	color: var(--wm-text-tertiary);
}

.has-text-danger .icon {
	color: var(--danger-text) !important;
}
</style>
