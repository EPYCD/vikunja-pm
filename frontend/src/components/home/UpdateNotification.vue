<template>
	<div
		v-if="updateAvailable"
		class="update-notification"
	>
		<p class="update-notification__message">
			{{ $t('update.available') }}
		</p>
		<XButton
			:shadow="false"
			:wrap="false"
			@click="refreshApp()"
		>
			{{ $t('update.do') }}
		</XButton>
	</div>
</template>

<script lang="ts" setup>
import {computed, ref} from 'vue'
import {useBaseStore} from '@/stores/base'

const baseStore = useBaseStore()

const updateAvailable = computed(() => baseStore.updateAvailable)
const registration = ref<ServiceWorkerRegistration | null>(null)
const refreshing = ref(false)

document.addEventListener('swUpdated', showRefreshUI, {once: true})

navigator?.serviceWorker?.addEventListener(
	'controllerchange', () => {
		// clientsClaim() also fires this on first install — only reload after
		// the user opted into an update, or unsaved state (e.g. login form
		// input) gets wiped.
		if (!refreshing.value) return
		refreshing.value = false
		window.location.reload()
	},
)

function showRefreshUI(e: Event) {
	console.log('recieved refresh event', e)
	const customEvent = e as CustomEvent<ServiceWorkerRegistration>
	registration.value = customEvent.detail
	baseStore.setUpdateAvailable(true)
}

function refreshApp() {
	baseStore.setUpdateAvailable(false)
	if (!registration.value || !registration.value.waiting) {
		return
	}
	refreshing.value = true
	// Notify the service worker to actually do the update
	registration.value.waiting.postMessage('skipWaiting')
}
</script>

<style lang="scss" scoped>
.update-notification {
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
	padding-inline: var(--wm-space-4) var(--wm-space-2);
	background: var(--warning);
	// --warning-invert is the readable-on-warning pair in both themes.
	color: var(--warning-invert);

	@include chamfer(var(--wm-chamfer-sm), bottom-right);
}

.update-notification__message {
	@include mono-label(var(--wm-text-2xs));

	inline-size: 100%;
	text-align: center;
	color: inherit;
}
</style>
