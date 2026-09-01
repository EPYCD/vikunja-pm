<template>
	<div class="content-auth">
		<BaseButton
			v-show="menuActive"
			:aria-label="$t('navigation.closeSidebar')"
			class="menu-hide-button d-print-none"
			@click="baseStore.setMenuActive(false)"
		>
			<Icon icon="times" />
		</BaseButton>
		<div
			class="app-container"
			:class="{'has-background': background || blurHash}"
			:style="{'background-image': blurHash && `url(${blurHash})`}"
		>
			<div
				:class="{'is-visible': background}"
				class="app-container-background background-fade-in d-print-none"
				:style="{
					'background-image': background && `url(${background})`,
					'filter': backgroundBrightness && `brightness(${backgroundBrightness}%)`
				}"
			/>
			<Navigation class="d-print-none" />
			<main
				id="main-content"
				tabindex="-1"
				class="app-content"
				:class="[
					{ 'is-menu-enabled': menuActive },
					$route.name,
				]"
				:style="{'--sidebar-width': sidebarWidth}"
			>
				<BaseButton
					v-show="menuActive"
					:aria-label="$t('navigation.closeSidebar')"
					class="mobile-overlay d-print-none"
					@click="baseStore.setMenuActive(false)"
				/>

				<QuickActions />

				<RouterView
					v-slot="{ Component }"
					:route="routeWithModal"
				>
					<keep-alive :include="['project.view']">
						<component :is="Component" />
					</keep-alive>
				</RouterView>

				<Modal
					:enabled="typeof currentModal !== 'undefined'"
					variant="scrolling"
					class="task-detail-view-modal"
					:aria-label="$t('task.detail.title')"
					@close="closeModal()"
				>
					<component
						:is="currentModal"
						@close="closeModal()"
					/>
				</Modal>

				<BaseButton
					v-shortcut="SHORTCUTS.showKeyboardShortcuts"
					class="keyboard-shortcuts-button d-print-none"
					@click="showKeyboardShortcuts()"
				>
					<span class="is-sr-only">{{ $t('keyboardShortcuts.title') }}</span>
					<Icon icon="keyboard" />
				</BaseButton>
			</main>
		</div>
	</div>
</template>

<script lang="ts" setup>
import {watch, computed, onBeforeUnmount} from 'vue'
import {useRoute, useRouter} from 'vue-router'

import {SHORTCUTS} from '@/constants/shortcuts'
import Navigation from '@/components/home/Navigation.vue'
import QuickActions from '@/components/quick-actions/QuickActions.vue'
import BaseButton from '@/components/base/BaseButton.vue'

import {useBaseStore} from '@/stores/base'
import {useProjectStore} from '@/stores/projects'

import {useRouteWithModal} from '@/composables/useRouteWithModal'
import {useRenewTokenOnFocus} from '@/composables/useRenewTokenOnFocus'
import {useSidebarResize} from '@/composables/useSidebarResize'
import {useWebSocket} from '@/composables/useWebSocket'
import {useAuthStore} from '@/stores/auth'

const authStore = useAuthStore()
const backgroundBrightness = computed(() =>
	authStore.settings?.frontendSettings?.backgroundBrightness,
)

const {sidebarWidth} = useSidebarResize()

const {routeWithModal, currentModal, closeModal} = useRouteWithModal()

const baseStore = useBaseStore()
const background = computed(() => baseStore.background)
const blurHash = computed(() => baseStore.blurHash)
const menuActive = computed(() => baseStore.menuActive)

function showKeyboardShortcuts() {
	baseStore.setKeyboardShortcutsActive(true)
}

const route = useRoute()
const router = useRouter()

// FIXME: this is really error prone
// Reset the current project highlight in menu if the current route is not project related.
watch(() => route.name as string, (routeName) => {
	if (
		routeName &&
		(
			[
				'home',
				'teams.index',
				'teams.edit',
				'tasks.range',
				'labels.index',
				'migrate.start',
				'migrate.wunderlist',
				'projects.index',
			].includes(routeName) ||
			routeName.startsWith('user.settings')
		)
	) {
		baseStore.handleSetCurrentProject({project: null})
	}
})

// TODO: Reset the title if the page component does not set one itself

useRenewTokenOnFocus()

const {connect} = useWebSocket()
connect()

const projectStore = useProjectStore()
projectStore.loadAllProjects()

// Listen for task creation from the quick-entry window
const taskUpdateChannel = new BroadcastChannel('vikunja-task-updates')
taskUpdateChannel.onmessage = (event) => {
	if (event.data?.type === 'task-created-open' && event.data?.taskId) {
		router.push({name: 'task.detail', params: {id: event.data.taskId}})
	}
}

onBeforeUnmount(() => {
	taskUpdateChannel.close()
})
</script>

<style lang="scss" scoped>
.menu-hide-button {
	position: fixed;
	inset-block-start: var(--wm-space-2);
	inset-inline-end: var(--wm-space-2);
	z-index: 31;
	inline-size: var(--wm-control-height-lg);
	block-size: var(--wm-control-height-lg);
	display: flex;
	justify-content: center;
	align-items: center;
	font-size: var(--wm-text-lg);
	color: var(--wm-text-tertiary);
	line-height: 1;
	transition: color var(--wm-duration) var(--wm-ease);

	@media screen and (min-width: $tablet) {
		display: none;
	}

	&:hover {
		color: var(--wm-text);
	}

	&:focus-visible {
		@include focus-ring;

		color: var(--wm-text);
	}
}

.app-container {
	min-block-size: calc(100vh - 65px);

	@media screen and (max-width: $tablet) {
		padding-block-start: $navbar-height;
	}
}

.app-content {
	--sidebar-width: #{$navbar-width};

	display: flow-root;
	z-index: 10;
	position: relative;
	padding-block: var(--wm-space-5) 0;
	padding-inline: var(--wm-space-2);
	// TODO refactor: DRY `transition-timing-function` with `./Navigation.vue`.
	transition: margin-inline-start $transition-duration;

	@media screen and (max-width: $tablet) {
		margin-inline-start: 0;
		margin-inline-end: 0;
		min-block-size: calc(100vh - 4rem);
	}

	@media screen and (min-width: $tablet) {
		padding-block: $navbar-height + 1.5rem 0;
		padding-inline: var(--wm-space-5);
	}

	&.is-menu-enabled {
		@media screen and (min-width: $tablet) {
			margin-inline-start: var(--sidebar-width);
		}
	}

	// Used to make sure the spinner is always in the middle while loading
	> .loader-container {
		min-block-size: calc(100vh - #{$navbar-height + 1.5rem + 1rem});
	}

	// FIXME: This should be somehow defined inside Card.vue
	.card {
		background: var(--wm-surface);
	}
}

.mobile-overlay {
	display: none;
	position: fixed;
	inset-block-start: 0;
	inset-block-end: 0;
	inset-inline-start: 0;
	inset-inline-end: 0;
	block-size: 100vh;
	inline-size: 100vw;
	// A scrim in the ground color rather than a tint, so it darkens the content
	// in both themes.
	background: var(--wm-canvas);
	z-index: 5;
	opacity: 0;
	transition: opacity var(--wm-duration) var(--wm-ease);

	@media screen and (max-width: $tablet) {
		display: block;
		opacity: 0.82;
	}
}

.keyboard-shortcuts-button {
	position: fixed;
	inset-block-end: var(--wm-space-4);
	inset-inline-end: var(--wm-space-4);
	z-index: 4500; // The modal has a z-index of 4000
	display: flex;
	align-items: center;
	justify-content: center;
	inline-size: var(--wm-control-height-sm);
	block-size: var(--wm-control-height-sm);
	color: var(--wm-text-tertiary);
	background: var(--wm-surface);
	border: 1px solid var(--wm-line-control);
	transition:
		color var(--wm-duration) var(--wm-ease),
		border-color var(--wm-duration) var(--wm-ease);

	&:hover {
		color: var(--wm-text);
		border-color: var(--wm-text-tertiary);
	}

	&:focus-visible {
		@include focus-ring;

		color: var(--wm-text);
	}

	@media screen and (max-width: $tablet) {
		display: none;
	}
}

.content-auth {
	position: relative;
	z-index: 1;
}

@media (prefers-reduced-motion: reduce) {
	.app-content {
		transition: none;
	}
}
</style>
