<script setup lang="ts">
import { computed } from 'vue'
import { useNow } from '@vueuse/core'
import { useAuthStore } from '@/stores/auth'
import { useConfigStore } from '@/stores/config'
import { useColorScheme } from '@/composables/useColorScheme'

import LogoFull from '@/assets/logo-full.svg?component'
import LogoFullPride from '@/assets/logo-full-pride.svg?component'
import {MILLISECONDS_A_HOUR} from '@/constants/date'

const now = useNow({
	interval: MILLISECONDS_A_HOUR,
})

const authStore = useAuthStore()
const configStore = useConfigStore()
const { isDark } = useColorScheme()

const Logo = computed(() => configStore.allowIconChanges
	&& authStore.settings.frontendSettings.allowIconChanges
	&& now.value.getMonth() === 5
	? LogoFullPride
	: LogoFull)

const isPride = computed(() => Logo.value === LogoFullPride)

const CustomLogo = computed(() => {
	const lightLogo = window.CUSTOM_LOGO_URL
	const darkLogo = window.CUSTOM_LOGO_URL_DARK

	if (!lightLogo && !darkLogo) return ''
	if (!darkLogo) return lightLogo
	if (!lightLogo) return darkLogo

	return isDark.value ? darkLogo : lightLogo
})
</script>

<template>
	<div>
		<Logo
			v-if="!CustomLogo"
			alt="Workman"
			class="logo"
			:class="{ 'logo--pride': isPride }"
		/>
		<img
			v-show="CustomLogo"
			:src="CustomLogo"
			alt="Workman"
			class="logo"
		>
	</div>
</template>

<style lang="scss" scoped>
.logo {
	color: var(--logo-text-color);
	// The lockup is 205x64; pin the height and let the width follow so the
	// wordmark never squashes. Without an explicit size an inlined SVG with
	// only a viewBox collapses to 0x0.
	block-size: 34px;
	inline-size: auto;
	max-inline-size: 100%;
}

// The wordmark inherits `currentColor`; the tile is repainted from the accent
// token so it tracks the theme. The pride variant keeps its own gradient.
.logo:not(.logo--pride) :deep(.wm-logo__tile) {
	fill: var(--wm-accent);
}

.logo :deep(.wm-logo__glyph) {
	stroke: var(--wm-on-accent);
}
</style>
