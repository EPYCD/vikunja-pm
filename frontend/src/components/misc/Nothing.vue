<template>
	<p class="nothing">
		<slot />
	</p>
</template>

<style lang="scss" scoped>
// The console's empty state: a dashed frame over the blueprint grid, with the
// message set as a mono caption rather than italic body text.
.nothing {
	@include graticule;
	@include mono-label(var(--wm-text-xs));

	display: flex;
	align-items: center;
	justify-content: center;
	text-align: center;
	padding: var(--wm-space-6) var(--wm-space-4);
	margin-block-end: var(--wm-space-4);
	border: 1px dashed var(--wm-line-strong);
	color: var(--wm-text-tertiary);
	text-transform: none;
	letter-spacing: 0.02em;
}
</style>
