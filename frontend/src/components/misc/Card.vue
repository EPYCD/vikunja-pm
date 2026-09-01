<template>
	<div
		class="card"
		:class="{'has-no-shadow': !shadow}"
	>
		<header
			v-if="title !== ''"
			class="card-header"
		>
			<p class="card-header-title">
				{{ title }}
			</p>
			<BaseButton
				v-if="showClose"
				class="card-header-icon close"
				:aria-label="$t('misc.close')"
				@click="$emit('close')"
			>	
				<span class="icon">
					<Icon icon="times" />
				</span>
			</BaseButton>
		</header>
		<div
			class="card-content loader-container"
			:class="{
				'p-0': !padding,
				'is-loading': loading
			}"
		>
			<div :class="{'content': hasContent}">
				<slot />
			</div>
		</div>

		<footer
			v-if="$slots.footer"
			class="card-footer"
		>
			<slot name="footer" />
		</footer>
	</div>
</template>

<script setup lang="ts">
import BaseButton from '@/components/base/BaseButton.vue'

withDefaults(defineProps<{
	title?: string
	padding?: boolean
	shadow?: boolean
	hasContent?: boolean
	loading?: boolean
	showClose?: boolean
}>(), {
	title: '',
	padding: true,
	shadow: true,
	hasContent: true,
	loading: false,
	showClose: false,
})

defineEmits<{
	'close': []
}>()
</script>

<style lang="scss" scoped>
// The Workman panel: flat surface, hairline rule, one cut corner. Structure
// comes from the border, not from elevation — so no radius and no shadow.
.card {
	background-color: var(--wm-surface);
	border: 1px solid var(--card-border-color);
	border-radius: 0;
	box-shadow: none;
	margin-block-end: var(--wm-space-4);
	color: var(--wm-text);
	max-inline-size: 100%;
	position: relative;

	// The cut corner is drawn rather than clipped: clip-path would slice the
	// border open along the diagonal and swallow any overflowing dropdown.
	&::after {
		content: '';
		position: absolute;
		inset-block-end: -1px;
		inset-inline-end: -1px;
		inline-size: var(--wm-chamfer);
		block-size: var(--wm-chamfer);
		background: linear-gradient(
			to bottom left,
			var(--site-background) 0 50%,
			var(--card-border-color) 50% calc(50% + 1px),
			var(--wm-surface) calc(50% + 1px) 100%
		);
		pointer-events: none;
	}

	@media print {
		box-shadow: none;
		border: none;

		&::after {
			display: none;
		}
	}
}

.card-header {
	background-color: transparent;
	align-items: stretch;
	display: flex;
	box-shadow: none;
	border-block-end: 1px solid var(--wm-line-faint);
	border-radius: 0;
}

.card-header-title {
	align-items: center;
	color: var(--wm-text-secondary);
	display: flex;
	flex-grow: 1;
	padding: var(--wm-space-3) var(--wm-space-4);

	@include mono-label;

	&.is-centered {
		justify-content: center;
	}
}

.card-header-icon {
	align-items: center;
	cursor: pointer;
	display: flex;
	justify-content: center;
	padding: var(--wm-space-3) var(--wm-space-4);
	color: var(--wm-text-tertiary);
	transition: color var(--wm-duration) var(--wm-ease);

	&:hover {
		color: var(--wm-text);
	}
}

.card-content {
	background-color: transparent;
	padding: var(--wm-space-5);

	// Utility classes like .p-0 are defined globally with lower specificity
	// than Vue-scoped selectors; restore precedence explicitly.
	&.p-0 {
		padding: 0;
	}
}

.card-footer {
	align-items: stretch;
	background-color: var(--wm-surface-sunken);
	border-block-start: 1px solid var(--wm-line-faint);
	padding: var(--wm-space-4);
	display: flex;
	justify-content: flex-end;
	gap: var(--wm-space-2);
}
</style>
