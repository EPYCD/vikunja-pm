<template>
	<BaseButton
		class="menu-bottom-link"
		:href="computedUrl"
		target="_blank"
	>
		{{ $t('misc.poweredBy') }}
	</BaseButton>
</template>

<script setup lang="ts">
import {computed} from 'vue'
import BaseButton from '@/components/base/BaseButton.vue'
import {POWERED_BY as poweredByUrl} from '@/urls'

const props = defineProps<{
	utmMedium: string;
}>()

const computedUrl = computed(() => `${poweredByUrl}&utm_medium=${props.utmMedium}`)
</script>

<style lang="scss">
.menu-bottom-link {
	@include mono-label;

	color: var(--wm-text-secondary);
	text-align: center;
	display: block;
	// Sits at the foot of the sidebar, under a hairline; in block layout the
	// auto margin simply resolves to zero.
	margin-block-start: auto;
	border-block-start: 1px solid var(--wm-line);
	padding-block: var(--wm-space-3);
	padding-inline: var(--wm-space-4);
	transition: color var(--wm-duration) var(--wm-ease);

	&:hover {
		color: var(--wm-text);
	}

	&:focus-visible {
		@include focus-ring;
	}
}
</style>
