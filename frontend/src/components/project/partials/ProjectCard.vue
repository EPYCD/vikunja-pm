<template>
	<div
		class="project-card"
		:class="{
			'has-light-text': background !== null,
			'has-background': blurHashUrl !== '' || background !== null
		}"
		:style="{
			'border-inline-start': project.hexColor ? `0.25rem solid ${project.hexColor}` : undefined,
			'background-image': blurHashUrl !== '' ? `url(${blurHashUrl})` : undefined,
		}"
	>
		<div
			class="project-background background-fade-in"
			:class="{'is-visible': background}"
			:style="{'background-image': background !== null ? `url(${background})` : undefined}"
		/>
		<span
			v-if="project.isArchived"
			class="is-archived"
		>{{ $t('project.archived') }}</span>

		<div
			class="project-title"
			aria-hidden="true"
		>
			<span
				v-if="project.id < -1"
				class="saved-filter-icon icon"
			>
				<Icon icon="filter" />
			</span>
			{{ getProjectTitle(project) }}
		</div>
		<BaseButton
			class="project-button"
			:aria-label="project.title"
			:title="textOnlyDescription"
			:to="{
				name: 'project.index',
				params: { projectId: project.id}
			}"
		/>
		<BaseButton
			v-if="!project.isArchived && project.id > -1"
			class="favorite"
			:aria-label="project.isFavorite ? $t('project.unfavorite') : $t('project.favorite')"
			:class="{'is-favorite': project.isFavorite}"
			@click.prevent.stop="projectStore.toggleProjectFavorite(project)"
		>
			<Icon :icon="project.isFavorite ? 'star' : ['far', 'star']" />
		</BaseButton>
	</div>
</template>

<script lang="ts" setup>
import {computed} from 'vue'
import type {IProject} from '@/modelTypes/IProject'

import BaseButton from '@/components/base/BaseButton.vue'

import {useProjectBackground} from '@/composables/useProjectBackground'
import {useProjectStore} from '@/stores/projects'
import {getProjectTitle} from '@/helpers/getProjectTitle'

const props = defineProps<{
	project: IProject,
}>()

const {background, blurHashUrl} = useProjectBackground(() => props.project)

const projectStore = useProjectStore()

const textOnlyDescription = computed(() => {
	return props.project.description ? props.project.description.replace(/<[^>]*>/g, '') : ''
})
</script>

<style lang="scss" scoped>
.project-card {
	--project-card-padding: var(--wm-space-4);

	// A chamfered panel carries its edge as an outline: clip-path would slice a
	// real border open along the 45° cut.
	@include chamfer(var(--wm-chamfer));
	@include chamfer-outline(var(--wm-line));

	background: var(--wm-surface);
	padding: var(--project-card-padding);
	transition: filter var(--wm-duration) var(--wm-ease);
	position: relative;
	overflow: hidden; // hide background

	display: flex;
	justify-content: space-between;
	flex-wrap: wrap;

	&:hover {
		@include chamfer-outline(var(--wm-accent-line));
	}

	> * {
		// so the elements are on top of the background
		position: relative;
	}
}

.has-background,
.project-background {
	background-size: cover;
	background-repeat: no-repeat;
	background-position: center;
}

.project-background,
.project-button {
	position: absolute;
	inset-block-start: 0;
	inset-inline-end: 0;
	inset-block-end: 0;
	inset-inline-start: 0;
}

// The hit area fills the whole card, so an outset ring would be clipped away
// by the chamfer. Draw it inside instead.
.project-card .project-button:focus-visible {
	box-shadow: none;
	outline: 2px solid var(--wm-accent);
	outline-offset: -3px;
}

// The card's metadata line: mono, tracked, quiet.
.is-archived {
	@include mono-label;

	align-self: flex-start;
	color: var(--wm-text-tertiary);
	padding: 0 var(--wm-space-1);
	border: 1px solid var(--wm-line);
}

.project-title {
	align-self: flex-end;
	font-family: $workman-display-font;
	font-weight: 500;
	font-size: var(--wm-text-lg);
	letter-spacing: var(--wm-tracking-tight);
	line-height: var(--title-line-height);
	color: var(--wm-text);
	inline-size: 100%;
	margin-block-end: 0;
	max-block-size: calc(100% - (var(--project-card-padding) + 1rem)); // padding & height of the "is archived" badge
	overflow: hidden;
	text-overflow: ellipsis;
	word-break: break-word;

	display: -webkit-box;
	-webkit-line-clamp: 3;
	-webkit-box-orient: vertical;
}

// Over a background photo the title needs a fixed light on dark, not the
// theme ramp — both themes show the same image.
.has-light-text .project-title,
.has-background .project-title {
	color: var(--wm-on-accent);
}

.has-background .project-title {
	text-shadow: 0 1px 6px hsla(var(--black-h), var(--black-s), var(--black-l), 0.9);
}

.favorite {
	position: absolute;
	inset-block-start: var(--project-card-padding);
	inset-inline-end: var(--project-card-padding);
	transition: opacity var(--wm-duration) var(--wm-ease), color var(--wm-duration) var(--wm-ease);
	opacity: 1;

	&:hover {
		color: var(--warning);
	}

	&.is-favorite {
		display: inline-block;
		opacity: 1;
		color: var(--warning);
	}
}

@media(hover: hover) and (pointer: fine) {
	.project-card .favorite {
		opacity: 0;
	}

	.project-card:hover .favorite {
		opacity: 1;
	}
}

.background-fade-in {
  opacity: 0;
  transition: opacity var(--wm-duration) var(--wm-ease);
  transition-delay: var(--wm-duration-slow); // To fake an appearing background

  &.is-visible {
    opacity: 1;
  }
}

.saved-filter-icon {
	color: var(--wm-text-tertiary);
	font-size: .75em;
}
</style>
