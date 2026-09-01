<template>
	<div class="content has-text-centered not-found">
		<h1 class="not-found__title">
			{{ $t('404.title') }}
		</h1>
		<p class="not-found__text">
			{{ $t('404.text') }}
		</p>
	</div>
</template>

<script setup lang="ts">
import {useTitle} from '@/composables/useTitle'

useTitle(() => '404')
</script>

<style lang="scss" scoped>
.not-found {
	@include graticule;

	background-color: var(--wm-surface);
	border: 1px dashed var(--wm-line-strong);
	padding-block: var(--wm-space-7);
	padding-inline: var(--wm-space-5);
}

.not-found__title {
	font-family: $workman-display-font;
	font-size: var(--wm-text-2xl);
	letter-spacing: var(--wm-tracking-tight);
	color: var(--wm-text);
	margin-block-end: var(--wm-space-3);
}

.not-found__text {
	@include mono-label(var(--wm-text-2xs));

	color: var(--wm-text-tertiary);
}
</style>
