<template>
	<div
		v-cy="'projects-list'"
		class="content loader-container"
		:class="{'is-loading': loading}"
	>
		<header class="project-header">
			<FancyCheckbox
				v-model="showArchived"
				v-cy="'show-archived-check'"
			>
				{{ $t('project.showArchived') }}
			</FancyCheckbox>

			<div class="action-buttons">
				<XButton
					:to="{name: 'filters.create'}"
					icon="filter"
				>
					{{ $t('filters.create.title') }}
				</XButton>
				<XButton
					v-cy="'new-project'"
					:to="{name: 'project.create'}"
					icon="plus"
				>
					{{ $t('project.create.header') }}
				</XButton>
			</div>
		</header>

		<ProjectCardGrid
			:projects="projects"
			:show-archived="showArchived"
		/>
	</div>
</template>

<script setup lang="ts">
import {computed} from 'vue'
import {useI18n} from 'vue-i18n'

import FancyCheckbox from '@/components/input/FancyCheckbox.vue'
import ProjectCardGrid from '@/components/project/partials/ProjectCardGrid.vue'

import {useTitle} from '@/composables/useTitle'
import {useStorage} from '@vueuse/core'

import {useProjectStore} from '@/stores/projects'

const {t} = useI18n()
const projectStore = useProjectStore()

useTitle(() => t('project.title'))
const showArchived = useStorage('showArchived', false)

const loading = computed(() => projectStore.isLoading)
const projects = computed(() => {
	return showArchived.value
		? projectStore.projectsArray
		: projectStore.projectsArray.filter(({isArchived}) => !isArchived)
})
</script>

<style lang="scss" scoped>
.project-header {
	display: flex;
	justify-content: space-between;
	align-items: center;
	gap: var(--wm-space-4);
	margin-block-end: var(--wm-space-4);
	padding-block-end: var(--wm-space-3);
	border-block-end: 1px solid var(--wm-line);

	@media screen and (max-width: $tablet) {
		flex-direction: column;
		align-items: stretch;
	}
}

.action-buttons {
	display: flex;
	justify-content: space-between;
	gap: var(--wm-space-2);

	@media screen and (max-width: $tablet) {
		inline-size: 100%;
		flex-direction: column;
		align-items: stretch;
	}
}

.project:not(:first-child) {
	margin-block-start: var(--wm-space-4);
}

.project-title {
	display: flex;
	align-items: center;
}

.is-archived {
	@include mono-label;

	border: 1px solid var(--wm-line);
	color: var(--wm-text-tertiary);
	padding: 0 var(--wm-space-1);
	background: var(--wm-surface);
	margin-inline-start: var(--wm-space-2);
}
</style>
