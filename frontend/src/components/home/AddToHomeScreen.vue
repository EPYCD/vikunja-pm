<template>
	<div
		v-if="shouldShowMessage"
		class="add-to-home-screen"
		:class="{'has-update-available': hasUpdateAvailable}"
	>
		<Icon
			icon="arrow-up-from-bracket"
			class="add-icon"
		/>
		<p>
			{{ $t('home.addToHomeScreen') }}
		</p>
		<BaseButton
			:aria-label="$t('misc.closeBanner')"
			class="hide-button"
			@click="() => hideMessage = true"
		>
			<Icon icon="x" />
		</BaseButton>
	</div>
</template>

<script lang="ts" setup>
import BaseButton from '@/components/base/BaseButton.vue'
import {useLocalStorage} from '@vueuse/core'
import {computed} from 'vue'
import {useBaseStore} from '@/stores/base'

const baseStore = useBaseStore()

const hideMessage = useLocalStorage('hideAddToHomeScreenMessage', false)
const hasUpdateAvailable = computed(() => baseStore.updateAvailable)

const shouldShowMessage = computed(() => {
	if (hideMessage.value) {
		return false
	}

	if (typeof window !== 'undefined' && window.matchMedia('(display-mode: standalone)').matches) {
		return false
	}

	return true
})
</script>

<style lang="scss" scoped>
.add-to-home-screen {
	position: fixed;
	// FIXME: We should prevent usage of z-index or
	// at least define it centrally
	// the highest z-index of a modal is .hint-modal with 4500
	z-index: 5000;
	inset-block-end: var(--wm-space-4);
	inset-inline: var(--wm-space-4);
	max-inline-size: max-content;
	margin-inline: auto;

	display: flex;
	align-items: center;
	justify-content: space-between;
	gap: var(--wm-space-4);
	padding-block: var(--wm-space-2);
	padding-inline: var(--wm-space-4);
	background: var(--wm-surface-raised);
	color: var(--wm-text);
	font-size: var(--wm-text-sm);

	// The banner floats, but a chamfered element can't carry a box-shadow —
	// the outline traces the cut instead.
	@include chamfer(var(--wm-chamfer-sm), bottom-right);
	@include chamfer-outline(var(--wm-line-strong));

	@media screen and (min-width: $tablet) {
		display: none;
	}

	@media print {
		display: none;
	}

	&.has-update-available {
		inset-block-end: 5rem;
	}
}

.add-icon {
	color: var(--wm-accent-text);
}

.hide-button {
	padding-block: var(--wm-space-1);
	padding-inline: var(--wm-space-2);
	cursor: pointer;
	color: var(--wm-text-tertiary);
	transition: color var(--wm-duration) var(--wm-ease);

	&:hover,
	&:focus-visible {
		color: var(--wm-text);
	}
}
</style>
