<script setup lang="ts">
import {computed, ref} from 'vue'
import {useConfigStore} from '@/stores/config'
import BaseButton from '@/components/base/BaseButton.vue'

const configStore = useConfigStore()
const hide = ref(false)
const enabled = computed(() => configStore.demoModeEnabled && !hide.value)
</script>

<template>
	<div
		v-if="enabled"
		class="demo-mode-banner"
	>
		<p>
			{{ $t('demo.title') }}
			<strong class="is-uppercase">{{ $t('demo.everythingWillBeDeleted') }}</strong>
		</p>
		<BaseButton
			:aria-label="$t('misc.closeBanner')"
			class="hide-button"
			@click="() => hide = true"
		>
			<Icon icon="times" />
		</BaseButton>
	</div>
</template>

<style scoped lang="scss">
.demo-mode-banner {
	position: fixed;
	inset-block-end: 0;
	inset-inline: 0;
	z-index: 100;
	padding: var(--wm-space-2);
	text-align: center;
	font-size: var(--wm-text-sm);
	background: var(--danger);
	// --danger-invert flips with the theme so the text keeps its contrast on
	// the (brighter) dark-mode danger red.
	&, strong {
		color: var(--danger-invert) !important;
	}

	strong {
		@include mono-label(var(--wm-text-2xs));
	}
}

.hide-button {
	padding-block: var(--wm-space-1);
	padding-inline: var(--wm-space-2);
	cursor: pointer;
	position: absolute;
	inset-inline-end: var(--wm-space-2);
	inset-block-start: var(--wm-space-1);
}
</style>
