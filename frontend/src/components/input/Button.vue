<template>
	<BaseButton
		class="button"
		:class="[
			variantClass,
			{
				'is-loading': loading,
				'has-no-shadow': !shadow || variant === 'tertiary',
				'is-danger': danger,
			}
		]"
		:disabled="disabled || loading"
		:style="{
			'--button-white-space': wrap ? 'break-spaces' : 'nowrap',
		}"
	>
		<template v-if="icon">
			<Icon
				v-if="!$slots.default"
				:icon="icon"
				:style="{color: iconColor}"
			/>
			<span
				v-else
				class="icon is-small"
			>
				<Icon
					:icon="icon"
					:style="{color: iconColor}"
				/>
			</span>
		</template>
		<span>
			<slot />
		</span>
	</BaseButton>
</template>

<script setup lang="ts">
import {computed, type PropType} from 'vue'
import BaseButton from '@/components/base/BaseButton.vue'
import type {IconProp} from '@fortawesome/fontawesome-svg-core'

export type ButtonTypes = 'primary' | 'secondary' | 'tertiary'

// Runtime prop declaration: with a type-only one, Vue casts absent boolean props
// to false, so `shadow` and `wrap` would default to false instead of true.
// Combining IconProp with withDefaults blows up the type checker (TS2590).
const props = defineProps({
	variant: {type: String as PropType<ButtonTypes>, default: 'primary'},
	icon: {type: [String, Array, Object] as PropType<IconProp>, default: undefined},
	iconColor: {type: String, default: undefined},
	loading: {type: Boolean, default: false},
	disabled: {type: Boolean, default: false},
	shadow: {type: Boolean, default: true},
	wrap: {type: Boolean, default: true},
	danger: {type: Boolean, default: false},
})

defineOptions({name: 'XButton'})

const VARIANT_CLASS_MAP: Record<ButtonTypes, string> = {
	primary: 'is-primary',
	secondary: 'is-outlined',
	tertiary: 'is-text is-inverted underline-none',
}

const variantClass = computed<string>(() => VARIANT_CLASS_MAP[props.variant])
</script>

<style lang="scss" scoped>
.button {
	// Base structure (replaces Bulma's .button)
	display: inline-flex;
	align-items: center;
	justify-content: center;
	vertical-align: top;
	cursor: pointer;
	text-align: center;
	white-space: var(--button-white-space);

	// Workman controls are compact, mono-labelled and square. Only the primary
	// action takes the chamfer — the same restraint the rest of the system uses.
	font-family: $workman-mono-font;
	font-size: var(--wm-text-2xs);
	font-weight: 500;
	letter-spacing: var(--wm-tracking-label);
	text-transform: uppercase;
	line-height: 1;
	block-size: auto;
	min-block-size: var(--wm-control-height);
	padding-inline: var(--wm-space-3);
	gap: var(--wm-space-2);
	border: 1px solid transparent;
	border-radius: 0;
	box-shadow: none;
	transition:
		background-color var(--wm-duration) var(--wm-ease),
		border-color var(--wm-duration) var(--wm-ease),
		color var(--wm-duration) var(--wm-ease);

	// Filled accent is the default (primary) look.
	background-color: var(--wm-accent);
	color: var(--wm-on-accent);

	[dir="rtl"] & {
		flex-direction: row-reverse;
	}

	&:hover {
		background-color: var(--wm-accent-hover);
	}

	&:focus-visible {
		outline: 2px solid var(--wm-accent);
		outline-offset: 2px;
	}

	&[disabled] {
		opacity: 0.45;
		cursor: not-allowed;
		pointer-events: none;
	}

	.icon {
		margin: 0 !important;
	}

	// Primary — the one filled, chamfered action per view.
	&.is-primary {
		background-color: var(--wm-accent);
		color: var(--wm-on-accent);
		@include chamfer(var(--wm-chamfer-sm), bottom-right);

		&:hover {
			background-color: var(--wm-accent-hover);
		}

		&:active {
			background-color: var(--wm-accent-active);
		}

		// clip-path removes both outline and box-shadow, so the focus ring has
		// to be traced off the clipped silhouette instead.
		&:focus-visible {
			outline: none;
			@include chamfer-focus-ring;
		}
	}

	// Secondary — hairline outline, transparent fill, square corners.
	&.is-outlined {
		background-color: transparent;
		border-color: var(--wm-line);
		color: var(--wm-text);

		&:hover {
			background-color: var(--wm-surface-hover);
			border-color: var(--wm-line-strong);
			color: var(--wm-text);
		}

		&:active {
			background-color: var(--wm-surface-sunken);
		}
	}

	// Tertiary — text only.
	&.is-text {
		background-color: transparent;
		border-color: transparent;
		color: var(--wm-text-secondary);

		&:hover {
			background-color: var(--wm-surface-hover);
			color: var(--wm-text);
		}
	}

	&.is-inverted {
		// Used with is-text for tertiary buttons
		color: inherit;
	}

	// Danger modifier — solid fill. Uses the hotter status red so it never
	// reads as the brand crimson.
	&.is-danger {
		background-color: var(--danger);
		border-color: transparent;
		color: var(--wm-text-inverted);

		&:hover {
			background-color: var(--danger-dark);
		}

		&:focus-visible {
			outline-color: var(--danger);
		}

		&:active {
			background-color: var(--danger-dark);
		}
	}

	&.is-danger.is-primary:focus-visible {
		outline: none;
		@include chamfer-focus-ring(hsla(var(--wm-danger-h), var(--wm-danger-s), var(--wm-danger-l), 0.9));
	}

	&.is-danger.is-outlined {
		background-color: transparent;
		border-color: var(--danger);
		color: var(--danger-text);

		&:hover {
			background-color: var(--danger);
			border-color: var(--danger);
			color: var(--wm-text-inverted);
		}
	}

	&.is-danger.is-text {
		background-color: transparent;
		color: var(--danger-text);

		&:hover {
			background-color: hsla(var(--wm-danger-h), var(--wm-danger-s), var(--wm-danger-l), 0.12);
		}
	}

	&.is-danger.is-loading::after {
		border-color: transparent transparent var(--wm-text-inverted) var(--wm-text-inverted);
	}

	&.is-danger.is-outlined.is-loading::after,
	&.is-danger.is-text.is-loading::after {
		border-color: transparent transparent var(--danger-text) var(--danger-text);
	}

	// Loading state
	&.is-loading {
		color: transparent !important;
		pointer-events: none;
		position: relative;

		&::after {
			content: "";
			position: absolute;
			display: block;
			block-size: 1em;
			inline-size: 1em;
			border: 2px solid var(--wm-on-accent);
			border-radius: var(--wm-radius-full);
			border-inline-end-color: transparent;
			border-block-start-color: transparent;
			animation: spin-around 500ms infinite linear;

			// Center the spinner
			inset-inline-start: calc(50% - 0.5em);
			inset-block-start: calc(50% - 0.5em);
		}
	}

	&.is-outlined.is-loading::after,
	&.is-text.is-loading::after {
		border-color: var(--wm-text-secondary);
		border-inline-end-color: transparent;
		border-block-start-color: transparent;
	}
}

@keyframes spin-around {
	from {
		transform: rotate(0deg);
	}
	to {
		transform: rotate(360deg);
	}
}

@media (prefers-reduced-motion: reduce) {
	.button.is-loading::after {
		animation-duration: 1.5s;
	}
}

.underline-none {
	text-decoration: none !important;
}
</style>
