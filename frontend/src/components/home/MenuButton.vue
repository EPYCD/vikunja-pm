<template>
	<BaseButton
		v-shortcut="SHORTCUTS.toggleMenu"
		class="menu-show-button"
		:title="$t('keyboardShortcuts.toggleMenu')"
		:aria-label="menuActive ? $t('misc.hideMenu') : $t('misc.showMenu')"
		:aria-expanded="menuActive"
		@click="baseStore.toggleMenu()"
		@shortkey="() => baseStore.toggleMenu()"
	/>
</template>

<script setup lang="ts">
import {computed} from 'vue'

import {SHORTCUTS} from '@/constants/shortcuts'
import {useBaseStore} from '@/stores/base'

import BaseButton from '@/components/base/BaseButton.vue'

const baseStore = useBaseStore()
const menuActive = computed(() => baseStore.menuActive)
</script>

<style lang="scss" scoped>
$line-width: 1.125rem;
$size: 2.25rem;

.menu-show-button {
	min-block-size: $size;
	inline-size: $size;

	position: relative;
	color: var(--wm-text-tertiary);
	transition: color var(--wm-duration) var(--wm-ease);

	$transform-x: translateX(-50%);

	&::before,
	&::after {
		content: '';
		display: block;
		position: absolute;
		block-size: 2px;
		inline-size: $line-width;
		inset-inline-start: 50%;
		transform: $transform-x;
		background-color: currentcolor;
		border-radius: var(--wm-radius-xs);
		transition: transform var(--wm-duration) var(--wm-ease);
	}

	&::before {
		inset-block-start: 50%;
		transform: $transform-x translateY(-0.3rem)
	}

	&::after {
		inset-block-end: 50%;
		transform: $transform-x translateY(0.3rem)
	}

	&:hover,
	&:focus-visible {
		color: var(--wm-text);

		&::before {
			transform: $transform-x translateY(-0.4rem);
		}

		&::after {
			transform: $transform-x translateY(0.4rem)
		}
	}

	&:focus-visible {
		@include focus-ring;
	}

	@media (prefers-reduced-motion: reduce) {
		&::before,
		&::after {
			transition: none;
		}
	}
}
</style>
