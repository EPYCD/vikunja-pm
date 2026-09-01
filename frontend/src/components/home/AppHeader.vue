<template>
	<header
		:class="{ 'has-background': background, 'menu-active': menuActive }"
		aria-label="main navigation"
		class="navbar d-print-none"
	>
		<RouterLink
			:to="{ name: 'home' }"
			class="logo-link"
			:aria-label="$t('navigation.home')"
		>
			<Logo
				width="164"
				height="48"
			/>
		</RouterLink>

		<MenuButton class="menu-button" />

		<div
			v-if="currentProject?.id"
			class="project-title-wrapper"
		>
			<div class="project-heading">
				<span
					v-if="currentViewTitle"
					class="project-eyebrow"
				>{{ currentViewTitle }}</span>
				<span class="project-title">
					{{ currentProject.title === '' ? $t('misc.loading') : getProjectTitle(currentProject) }}
				</span>
			</div>

			<BaseButton
				v-if="!isEditorContentEmpty(currentProject.description)"
				:to="{ name: 'project.info', params: { projectId: currentProject.id } }"
				class="project-title-button"
			>
				<span class="is-sr-only">{{ $t('project.description') }}</span>
				<Icon icon="circle-info" />
			</BaseButton>

			<ProjectSettingsDropdown
				v-if="canWriteCurrentProject && currentProject.id !== -1"
				class="project-title-dropdown"
				:project="currentProject"
			>
				<template #trigger="{ toggleOpen, open }">
					<BaseButton
						class="project-title-button"
						:aria-expanded="open"
						@click="toggleOpen"
					>
						<span class="is-sr-only">{{ $t('project.openSettingsMenu') }}</span>
						<Icon
							icon="ellipsis-h"
							class="icon"
						/>
					</BaseButton>
				</template>
			</ProjectSettingsDropdown>
		</div>

		<div
			v-else-if="pageTitle"
			class="project-title-wrapper"
		>
			<div class="project-heading">
				<span class="project-title">{{ pageTitle }}</span>
			</div>
		</div>

		<div class="navbar-end">
			<TimerBadge />
			<OpenQuickActions />
			<Notifications />
			<Dropdown>
				<template #trigger="{ toggleOpen, open }">
					<BaseButton
						class="username-dropdown-trigger"
						variant="secondary"
						:shadow="false"
						:aria-expanded="open"
						@click="toggleOpen"
					>
						<img
							:src="authStore.avatarUrl"
							alt=""
							class="avatar"
							width="40"
							height="40"
						>
						<span class="username">{{ authStore.userDisplayName }}</span>
						<span
							class="mis-1 dropdown-icon icon is-small"
							:style="{
								transform: open ? 'rotate(180deg)' : 'rotate(0)',
							}"
						>
							<Icon icon="chevron-down" />
						</span>
					</BaseButton>
				</template>

				<DropdownItem :to="{ name: 'user.settings' }">
					{{ $t('user.settings.title') }}
				</DropdownItem>
				<DropdownItem
					v-if="adminPanelEnabled && authStore.info?.isAdmin"
					:to="{ name: 'admin.overview' }"
				>
					{{ $t('admin.title') }}
				</DropdownItem>
				<DropdownItem
					v-if="imprintUrl"
					:href="imprintUrl"
				>
					{{ $t('navigation.imprint') }}
				</DropdownItem>
				<DropdownItem
					v-if="privacyPolicyUrl"
					:href="privacyPolicyUrl"
				>
					{{ $t('navigation.privacy') }}
				</DropdownItem>
				<DropdownItem @click="baseStore.setKeyboardShortcutsActive(true)">
					{{ $t('keyboardShortcuts.title') }}
				</DropdownItem>
				<DropdownItem :to="{ name: 'about' }">
					{{ $t('about.title') }}
				</DropdownItem>
				<DropdownItem @click="authStore.logout()">
					{{ $t('user.auth.logout') }}
				</DropdownItem>
			</Dropdown>
		</div>
	</header>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'

import { PERMISSIONS as Permissions } from '@/constants/permissions'
import { PRO_FEATURE } from '@/constants/proFeatures'

import ProjectSettingsDropdown from '@/components/project/ProjectSettingsDropdown.vue'
import Dropdown from '@/components/misc/Dropdown.vue'
import DropdownItem from '@/components/misc/DropdownItem.vue'
import Notifications from '@/components/notifications/Notifications.vue'
import TimerBadge from '@/components/time-tracking/TimerBadge.vue'
import Logo from '@/components/home/Logo.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import MenuButton from '@/components/home/MenuButton.vue'
import OpenQuickActions from '@/components/misc/OpenQuickActions.vue'

import { getProjectTitle } from '@/helpers/getProjectTitle'
import { isEditorContentEmpty } from '@/helpers/editorContentEmpty'

import { useBaseStore } from '@/stores/base'
import { useConfigStore } from '@/stores/config'
import { useAuthStore } from '@/stores/auth'
import type { IProject } from '@/modelTypes/IProject'

const baseStore = useBaseStore()
// Create a mutable copy to satisfy type requirements (readonly deep -> mutable)
const currentProject = computed<IProject | null>(() => {
	const project = baseStore.currentProject
	return project ? { ...project } as IProject : null
})
const background = computed(() => baseStore.background)
const canWriteCurrentProject = computed(() => baseStore.currentProject?.maxPermission !== null && baseStore.currentProject?.maxPermission !== undefined && baseStore.currentProject.maxPermission > Permissions.READ)
const menuActive = computed(() => baseStore.menuActive)

// The header eyebrow names the view the project is currently shown through.
const currentViewTitle = computed(() => {
	const viewId = baseStore.currentProjectViewId
	if (!viewId) {
		return ''
	}
	return currentProject.value?.views?.find(view => view.id === viewId)?.title ?? ''
})

// Standalone pages (no project) surface their route's title in the header.
const route = useRoute()
const { t } = useI18n()
const pageTitle = computed(() => {
	const title = route.meta.title as string | undefined
	return title ? t(title) : ''
})

const authStore = useAuthStore()

const configStore = useConfigStore()
const imprintUrl = computed(() => configStore.legal.imprintUrl)
const privacyPolicyUrl = computed(() => configStore.legal.privacyPolicyUrl)
const adminPanelEnabled = computed(() => configStore.isProFeatureEnabled(PRO_FEATURE.ADMIN_PANEL))
</script>

<style lang="scss" scoped>
.navbar {
	--navbar-button-min-width: 2.25rem;
	--navbar-gap-width: var(--wm-space-3);
	--navbar-icon-size: 1rem;

	position: fixed;
	inset-block-start: 0;
	inset-inline-start: 0;
	inset-inline-end: 0;
	z-index: 30;

	display: flex;
	align-items: stretch;
	gap: var(--navbar-gap-width);
	min-block-size: $navbar-height;

	// Flat surface with a single hairline rule — the header never casts a shadow.
	background: var(--wm-surface);
	border-block-end: 1px solid var(--wm-line);

	@media screen and (min-width: $tablet) {
		padding-inline: var(--wm-space-5) var(--wm-space-4);
	}

	&.menu-active {
		@media screen and (max-width: $tablet) {
			z-index: 0;
		}
	}

	// FIXME: notifications should provide a slot for the icon instead, so that we can style it as we want
	:deep() {
		.trigger-button {
			color: var(--wm-text-tertiary);
			font-size: var(--navbar-icon-size);
			transition: color var(--wm-duration) var(--wm-ease);

			&:hover,
			&:focus-visible {
				color: var(--wm-text);
			}
		}
	}
}

.logo-link {
	display: none;

	@media screen and (min-width: $tablet) {
		align-self: center;
		display: flex;
		align-items: center;
		margin-inline-end: var(--wm-space-2);
	}

	&:focus-visible {
		@include focus-ring;
	}
}

.menu-button {
	align-self: center;
	flex: 0 0 auto;

	@media screen and (max-width: $tablet) {
		margin-inline-start: var(--wm-space-2);
	}
}

.project-title-wrapper {
	display: flex;
	align-items: center;
	gap: var(--wm-space-1);
	margin-inline-end: auto;

	// this makes the truncated text of the project title work
	// inside the flexbox parent
	min-inline-size: 0;

	@media screen and (min-width: $tablet) {
		padding-inline-start: var(--wm-space-2);
	}
}

.project-heading {
	display: flex;
	flex-direction: column;
	justify-content: center;
	gap: var(--wm-space-1);
	min-inline-size: 0;
}

.project-eyebrow {
	@include mono-label;

	color: var(--wm-text-tertiary);
	line-height: 1;
	white-space: nowrap;
	text-overflow: ellipsis;
	overflow: hidden;
}

.project-title {
	font-family: $workman-display-font;
	font-size: var(--wm-text-md);
	font-weight: 600;
	line-height: 1.2;
	letter-spacing: var(--wm-tracking-tight);
	color: var(--wm-text);

	// We need the following for overflowing ellipsis to work
	text-overflow: ellipsis;
	overflow: hidden;
	white-space: nowrap;

	@media screen and (min-width: $tablet) {
		font-size: var(--wm-text-lg);
	}
}

.project-title-dropdown {
	align-self: center;

	.project-title-button {
		flex-grow: 1;
	}
}

.project-title-button {
	align-self: center;
	min-inline-size: var(--navbar-button-min-width);
	block-size: var(--wm-control-height-sm);
	display: flex;
	align-items: center;
	justify-content: center;
	font-size: var(--navbar-icon-size);
	color: var(--wm-text-tertiary);
	transition:
		color var(--wm-duration) var(--wm-ease),
		background-color var(--wm-duration) var(--wm-ease);

	&:hover {
		color: var(--wm-text);
		background: var(--wm-surface-hover);
	}

	&:focus-visible {
		@include focus-ring;

		color: var(--wm-text);
	}
}

.navbar-end {
	flex: 0 0 auto;
	display: flex;
	align-items: stretch;
	margin-inline-start: auto;

	> * {
		min-inline-size: var(--navbar-button-min-width);
	}
}

.username-dropdown-trigger {
	align-self: center;
	display: inline-flex;
	align-items: center;
	gap: var(--wm-space-2);
	block-size: var(--wm-control-height);
	padding-inline: var(--wm-space-1) var(--wm-space-2);
	border: 1px solid var(--wm-line);
	background: var(--wm-surface);
	color: var(--wm-text-secondary);
	font-size: var(--wm-text-sm);
	font-weight: 500;
	transition:
		color var(--wm-duration) var(--wm-ease),
		border-color var(--wm-duration) var(--wm-ease),
		background-color var(--wm-duration) var(--wm-ease);

	&:hover {
		color: var(--wm-text);
		border-color: var(--wm-line-strong);
		background: var(--wm-surface-hover);
	}

	&:focus-visible {
		@include focus-ring;

		border-color: var(--wm-accent);
		color: var(--wm-text);
	}

	:deep(.avatar) {
		margin-inline-end: 0;
	}

	[dir="rtl"] & {
		flex-direction: row-reverse;
	}

	@media screen and (max-width: $tablet) {
		padding-inline: var(--wm-space-1);
	}
}

.username {
	max-inline-size: 10rem;
	overflow: hidden;
	text-overflow: ellipsis;
	white-space: nowrap;

	@media screen and (max-width: $tablet) {
		display: none;
	}
}

.dropdown-icon {
	color: var(--wm-text-tertiary);
	font-size: var(--wm-text-2xs);
	transition: transform var(--wm-duration) var(--wm-ease);
}

.avatar {
	border-radius: var(--wm-radius-full);
	vertical-align: middle;
	block-size: 1.375rem;
	inline-size: 1.375rem;
	flex-shrink: 0;
	margin-inline-end: 0;
}
</style>
