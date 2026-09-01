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
// The way in: a full-bleed console ground with the blueprint graticule, one
// cut panel floating on it, the lockup above. Nothing else.
.no-auth-wrapper {
	position: relative;
	min-block-size: 100vh;
	display: flex;
	flex-direction: column;
	align-items: center;
	padding-block: var(--wm-space-6);
	padding-inline: var(--wm-space-4);
	background-color: var(--wm-canvas);

	@include graticule;
}

// The crimson wedge in the corner of the ground — the same 45° cut the panel
// carries, at page scale.
.no-auth-wrapper::before {
	content: "";
	position: fixed;
	inset-block-end: 0;
	inset-inline-start: 0;
	inline-size: 520px;
	block-size: 520px;
	max-inline-size: 55vw;
	max-block-size: 55vh;
	background: url("@/assets/llama.svg?url") no-repeat left bottom / contain;
	pointer-events: none;
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
	align-self: flex-start;

	:deep(.logo) {
		block-size: 40px;
	}
}

.auth-panel {
	background-color: var(--wm-surface);
	color: var(--wm-text);
	padding: var(--wm-space-6);

	// Panel edge: the outline traces the cut, because a border would be sliced
	// open along the diagonal.
	@include chamfer(var(--wm-chamfer-lg));
	@include chamfer-outline(var(--wm-line));

	// The skip link focuses this panel, and clip-path would swallow the normal
	// box-shadow ring — so the ring is drawn off the clipped mask instead.
	&:focus-visible {
		@include chamfer-focus-ring;
	}

	@media screen and (max-width: $tablet) {
		padding: var(--wm-space-5);
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

	color: var(--wm-text-tertiary);
	text-align: end;
	margin-block-start: 0;

	// The links inherit the container's colour, so they need the underline to
	// read as links at all.
	:deep(.base-button) {
		color: var(--wm-text-secondary);
		text-decoration: underline;
	}
}
</style>
