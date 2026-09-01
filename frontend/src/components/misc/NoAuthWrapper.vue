<template>
	<div class="no-auth-wrapper">
		<div class="auth-stack">
			<Logo
				class="auth-logo"
				width="200"
				height="58"
			/>
			<main
				id="main-content"
				tabindex="-1"
				class="auth-panel"
			>
				<header class="auth-panel__head">
					<p class="auth-panel__eyebrow">
						{{ $t("misc.welcomeBack") }}
					</p>
					<h2
						v-if="title"
						class="auth-panel__title"
					>
						{{ title }}
					</h2>
				</header>

				<Message
					v-if="motd !== ''"
					class="mbe-4"
				>
					{{ motd }}
				</Message>
				<ApiConfig v-if="shouldShowApiConfig" />
				<slot />
			</main>
			<Legal class="auth-legal" />
		</div>
	</div>
</template>

<script lang="ts" setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'

import Logo from '@/components/home/Logo.vue'
import Message from '@/components/misc/Message.vue'
import Legal from '@/components/misc/Legal.vue'
import ApiConfig from '@/components/misc/ApiConfig.vue'

import { useTitle } from '@/composables/useTitle'
import { useConfigStore } from '@/stores/config'
import { isDesktopApp } from '@/helpers/desktopAuth'

const props = withDefaults(
	defineProps<{
		showApiConfig?: boolean;
	}>(),
	{
		showApiConfig: false,
	},
)

const isDesktop = isDesktopApp()
const hasStoredApiUrl = isDesktop && localStorage.getItem('API_URL') !== null
const shouldShowApiConfig = computed(() => props.showApiConfig && (!isDesktop || hasStoredApiUrl))

const configStore = useConfigStore()
const motd = computed(() => configStore.motd)

const route = useRoute()
const { t } = useI18n({ useScope: 'global' })
const title = computed(() =>
	route.meta?.title ? t(route.meta.title as string) : '',
)
useTitle(() => title.value)
</script>

<style lang="scss" scoped>
// The way in: a full-bleed ASCII landscape — crimson sun, receding ridges,
// drawn entirely in JetBrains Mono glyphs by scripts/generate-auth-art.py —
// with one cut panel floating over it.
.no-auth-wrapper {
	position: relative;
	min-block-size: 100vh;
	display: flex;
	flex-direction: column;
	align-items: center;
	padding-block: var(--wm-space-6);
	padding-inline: var(--wm-space-4);
	background-color: var(--wm-canvas);
	background-image: url("@/assets/auth-backdrop-light.webp");
	background-repeat: no-repeat;
	background-position: center bottom;
	background-size: cover;
	background-attachment: fixed;
	isolation: isolate;
}

html.dark .no-auth-wrapper {
	background-image: url("@/assets/auth-backdrop.webp");
}

// A scrim over the artwork so the panel always has quiet ground under it and
// the legal links stay readable wherever the ridges happen to fall.
.no-auth-wrapper::before {
	content: "";
	position: absolute;
	inset: 0;
	z-index: -1;
	background:
		radial-gradient(
			ellipse 46rem 34rem at 50% 46%,
			color-mix(in srgb, var(--wm-canvas) 74%, transparent) 0%,
			color-mix(in srgb, var(--wm-canvas) 30%, transparent) 55%,
			transparent 100%
		);
	pointer-events: none;
}

@media (prefers-reduced-motion: no-preference) {
	.no-auth-wrapper {
		// A fixed backdrop would jitter on mobile browsers that resize the
		// viewport as the URL bar hides.
		@media screen and (max-width: $tablet) {
			background-attachment: scroll;
		}
	}
}

.auth-stack {
	position: relative;
	z-index: 1;
	inline-size: 100%;
	max-inline-size: 27rem;
	// `margin: auto` rather than `justify-content: center`: a form taller than
	// the viewport must overflow downwards, not have its top cut off.
	margin-block: auto;
	display: flex;
	flex-direction: column;
	gap: var(--wm-space-4);
}

.auth-logo {
	align-self: center;

	:deep(.logo) {
		block-size: 40px;
	}
}

.auth-panel {
	// Translucent so the artwork reads through it, blurred so the text on top
	// of it never has to fight the character matrix underneath.
	background-color: color-mix(in srgb, var(--wm-surface) 82%, transparent);
	backdrop-filter: blur(18px) saturate(120%);
	color: var(--wm-text);
	padding: var(--wm-space-6);

	// Panel edge: the outline traces the cut, because a border would be sliced
	// open along the diagonal.
	@include chamfer(var(--wm-chamfer-lg));
	@include chamfer-outline(var(--wm-line-strong));

	// The skip link focuses this panel, and clip-path would swallow the normal
	// box-shadow ring — so the ring is drawn off the clipped mask instead.
	&:focus-visible {
		@include chamfer-focus-ring;
	}

	@media screen and (max-width: $tablet) {
		padding: var(--wm-space-5);
	}
}

// backdrop-filter is unevenly supported; without it the translucent panel would
// leave glyphs showing through the form.
@supports not (backdrop-filter: blur(1px)) {
	.auth-panel {
		background-color: var(--wm-surface);
	}
}

.auth-panel__head {
	margin-block-end: var(--wm-space-5);
	padding-block-end: var(--wm-space-4);
	border-block-end: 1px solid var(--wm-line-faint);
}

.auth-panel__eyebrow {
	@include mono-label;

	color: var(--wm-text-tertiary);
	margin-block: 0 var(--wm-space-2);
}

.auth-panel__title {
	font-family: $workman-display-font;
	font-size: var(--wm-text-xl);
	letter-spacing: var(--wm-tracking-tight);
	line-height: 1.15;
	color: var(--wm-text);
	margin-block: 0;
}

// Scoped inside .auth-stack to outrank Legal.vue's own scoped rule, which has
// the same specificity and would otherwise win on source order.
.auth-stack .auth-legal {
	@include mono-label;

	color: var(--wm-text-secondary);
	text-align: center;
	margin-block-start: 0;

	// The links inherit the container's colour, so they need the underline to
	// read as links at all.
	:deep(.base-button) {
		color: var(--wm-text-secondary);
		text-decoration: underline;
	}
}
</style>
