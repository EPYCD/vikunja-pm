<template>
	<li
		class="list-menu loader-container is-loading-small"
		:class="{
			'is-loading': isLoading,
			'is-drop-target': isDropTarget,
		}"
		:data-project-id="project.id"
	>
		<div class="navigation-item">
			<BaseButton
				v-if="canCollapse && childProjects?.length > 0"
				class="collapse-project-button"
				:aria-label="$t('navigation.toggleChildProjects')"
				@click="childProjectsOpen = !childProjectsOpen"
			>
				<Icon
					icon="chevron-down"
					:class="{ 'project-is-collapsed': !childProjectsOpen }"
				/>
			</BaseButton>
			<BaseButton
				:to="{ name: 'project.index', params: { projectId: project.id} }"
				class="list-menu-link"
				:class="{'router-link-exact-active': currentProject?.id === project.id}"
			>
				<span
					v-if="!canCollapse || childProjects?.length === 0"
					class="collapse-project-button-placeholder"
				/>
				<div class="color-bubble-wrapper">
					<ColorBubble
						v-if="project.hexColor !== ''"
						:color="project.hexColor"
						:aria-label="$t('project.color')"
					/>
					<span
						v-else-if="project.id < -1"
						class="saved-filter-icon icon menu-item-icon"
					>
						<Icon icon="filter" />
					</span>
					<span
						v-if="canEditOrder && project.id > 0 && project.maxPermission !== null && project.maxPermission > PERMISSIONS.READ"
						class="icon menu-item-icon handle drag-handle"
						@mousedown.stop
						@click.stop.prevent
						@touchstart.stop
					>
						<Icon icon="grip-lines" />
					</span>
				</div>
				<span class="project-menu-title">{{ getProjectTitle(project) }}</span>
			</BaseButton>
			<BaseButton
				v-if="canToggleFavorite"
				class="favorite"
				:class="{'is-favorite': project.isFavorite}"
				@click="projectStore.toggleProjectFavorite(project)"
			>
				<span class="is-sr-only">{{ project.isFavorite ? $t('project.unfavorite') : $t('project.favorite') }}</span>
				<Icon :icon="project.isFavorite ? 'star' : ['far', 'star']" />
			</BaseButton>
			<ProjectSettingsDropdown
				v-if="project.maxPermission !== null && project.maxPermission > PERMISSIONS.READ"
				class="menu-list-dropdown"
				:project="project"
			>
				<template #trigger="{toggleOpen, open}">
					<BaseButton
						class="menu-list-dropdown-trigger"
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
		<ProjectsNavigation
			v-if="childProjectsOpen && canCollapse"
			:model-value="childProjects"
			:can-edit-order="true"
			:can-collapse="canCollapse"
		/>
	</li>
</template>

<script setup lang="ts">
import {computed, ref, onUnmounted, watch} from 'vue'
import {useProjectStore} from '@/stores/projects'
import {useBaseStore} from '@/stores/base'
import {useTaskStore} from '@/stores/tasks'
import {useStorage} from '@vueuse/core'

import type {IProject} from '@/modelTypes/IProject'

import BaseButton from '@/components/base/BaseButton.vue'
import ProjectSettingsDropdown from '@/components/project/ProjectSettingsDropdown.vue'
import {getProjectTitle} from '@/helpers/getProjectTitle'
import ColorBubble from '@/components/misc/ColorBubble.vue'
import ProjectsNavigation from '@/components/home/ProjectsNavigation.vue'
import {PERMISSIONS} from '@/constants/permissions'
import {isSavedFilter} from '@/services/savedFilter'

const props = defineProps<{
	project: IProject,
	isLoading?: boolean,
	canCollapse?: boolean,
	canEditOrder?: boolean,
}>()

const taskStore = useTaskStore()
const isHoveredDuringDrag = ref(false)

// Track mouse position during drag to detect hover (mouseenter doesn't fire during drag)
function handleMouseMove(e: MouseEvent) {
	if (!taskStore.draggedTask) {
		isHoveredDuringDrag.value = false
		return
	}

	const elementsUnderMouse = document.elementsFromPoint(e.clientX, e.clientY)
	const isOverThisProject = elementsUnderMouse.some(el => {
		const projectId = (el as HTMLElement).dataset?.projectId
		return projectId && parseInt(projectId, 10) === props.project.id
	})

	isHoveredDuringDrag.value = isOverThisProject
}

// Only add the listener when a task is being dragged
// Use capture phase to receive events before Sortable.js can prevent them
watch(() => taskStore.draggedTask, (draggedTask) => {
	if (draggedTask) {
		document.addEventListener('mousemove', handleMouseMove, true)
		document.addEventListener('dragover', handleMouseMove, true)
	} else {
		document.removeEventListener('mousemove', handleMouseMove, true)
		document.removeEventListener('dragover', handleMouseMove, true)
		isHoveredDuringDrag.value = false
	}
}, {immediate: true})

onUnmounted(() => {
	document.removeEventListener('mousemove', handleMouseMove, true)
	document.removeEventListener('dragover', handleMouseMove, true)
})

// Show drop target highlight when a task is being dragged and this project is hovered
const isDropTarget = computed(() => {
	if (!taskStore.draggedTask || !isHoveredDuringDrag.value) {
		return false
	}
	// Highlight any valid project (not a pseudo project, has write permission)
	// The actual drop logic will handle the case when it's the same project (no-op)
	return props.project.id > 0
		&& props.project.maxPermission !== null
		&& props.project.maxPermission > PERMISSIONS.READ
})

const projectStore = useProjectStore()
const baseStore = useBaseStore()
const currentProject = computed(() => baseStore.currentProject)

// Persist open state across browser reloads. Using a separate ref for the state 
// allows us to use only one entry in local storage instead of one for every project id.
type OpenState = { [key: number]: boolean }
const childProjectsOpenState = useStorage<OpenState>('navigation-child-projects-open', {})
const childProjectsOpen = computed({
	get() {
		return childProjectsOpenState.value[props.project.id] ?? true
	},
	set(open) {
		childProjectsOpenState.value[props.project.id] = open
	},
})

const childProjects = computed(() => {
	return projectStore.getChildProjects(props.project.id)
		.filter(p => !p.isArchived)
		.sort((a, b) => a.position - b.position)
})

const canToggleFavorite = computed(() => {
	// Allow favorite toggle for:
	// 1. Regular projects (id > 0) with write permission
	// 2. Saved filters (id < -1) - user owns their own filters
	if (props.project.id === -1) return false  // Favorites pseudo-project
	if (props.project.id > 0) {
		return props.project.maxPermission !== null && props.project.maxPermission > PERMISSIONS.READ
	}
	// Saved filters (negative IDs except -1)
	return isSavedFilter(props.project)
})
</script>

<style lang="scss" scoped>
.list-menu {
	transition: background-color var(--wm-duration) var(--wm-ease);
}

.project-is-collapsed {
	transform: rotate(-90deg);
}

.favorite {
	display: flex;
	align-items: center;
	justify-content: center;
	inline-size: var(--wm-control-height-sm);
	block-size: var(--wm-control-height-sm);
	flex-shrink: 0;
	color: var(--wm-text-tertiary);
	transition:
		opacity var(--wm-duration) var(--wm-ease),
		color var(--wm-duration) var(--wm-ease);
	opacity: 0;

	&:hover,
	&.is-favorite {
		opacity: 1;
		color: var(--warning);
	}
}

.list-menu:hover > div > .favorite {
	opacity: 1;
}

.list-menu:hover .color-bubble-wrapper > .drag-handle {
	opacity: 1;
}

.list-menu:hover .color-bubble-wrapper > .color-bubble {
	opacity: 0;
}

// Sized to the width of an icon box so project bubbles line up with the
// glyphs of the top-level navigation above them.
.color-bubble-wrapper {
	position: relative;
	inline-size: 1.5rem;
	block-size: 1.5rem;
	display: flex;
	align-items: center;
	justify-content: center;
	margin-inline-end: var(--wm-space-2);
	flex-shrink: 0;

	.color-bubble, .icon {
		transition: opacity var(--wm-duration) var(--wm-ease);
		position: absolute;
		inline-size: 12px;
		margin: 0 !important;
		padding: 0 !important;
	}
}

.drag-handle {
	opacity: 0;
	cursor: grab;
	transition: opacity var(--wm-duration) var(--wm-ease);
	position: absolute;
	inset: 0;
	display: flex;
	align-items: center;
	justify-content: center;
	color: var(--wm-text-tertiary);

	&:active {
		cursor: grabbing;
	}
}

.project-menu-title {
	overflow: hidden;
	text-overflow: ellipsis;
}

.saved-filter-icon {
	color: var(--wm-text-tertiary) !important;
	font-size: var(--wm-text-xs);
}

@media (pointer: coarse) {
	.drag-handle {
		display: none !important;
	}

	.color-bubble {
		opacity: 1 !important;
	}
}

.navigation-item {
	position: relative;
	padding-inline-end: var(--wm-space-2);
	transition: background-color var(--wm-duration) var(--wm-ease);
}

// An inset ring: the sidebar scroll container would clip an outer one.
.navigation-item:has(*:focus-visible) {
	outline: 2px solid var(--wm-accent);
	outline-offset: -2px;
	background-color: var(--wm-surface-hover);

	.favorite, .menu-list-dropdown {
		opacity: 1;
	}
}

.navigation-item a:focus-visible {
	// The focus ring is already added to the navigation-item, so we don't need to add it again.
	outline: none;
	box-shadow: none;
}

.is-drop-target {
	background-color: var(--wm-accent-wash);

	.navigation-item {
		background-color: var(--wm-accent-wash-strong);
		box-shadow: inset 0 0 0 1px var(--wm-accent);
	}
}
</style>
