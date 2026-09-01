<template>
	<span
		v-if="task.commentCount && task.commentCount > 0"
		v-tooltip="tooltip"
		class="comment-count"
		:class="{'is-unread': task.isUnread}"
		role="img"
		:aria-label="tooltip"
	>
		<Icon :icon="['far', 'comments']" />
		<span class="comment-count-badge">{{ task.commentCount }}</span>
		<span
			v-if="task.isUnread"
			class="unread-indicator"
		/>
	</span>
</template>

<script setup lang="ts">
import {computed} from 'vue'
import {useI18n} from 'vue-i18n'

import type {ITask} from '@/modelTypes/ITask'

const props = defineProps<{
	task: ITask
}>()

const {t} = useI18n({useScope: 'global'})

const tooltip = computed(() => t('task.attributes.comment', props.task.commentCount))
</script>

<style scoped lang="scss">
.comment-count {
	display: inline-flex;
	align-items: center;
	gap: var(--wm-space-1);
	font-size: var(--wm-text-xs);
	color: var(--wm-text-tertiary);
	transition: color var(--wm-duration) var(--wm-ease);

	.comment-count-badge {
		@include mono-data;

		font-size: var(--wm-text-2xs);
		line-height: 1;
	}

	&:hover {
		color: var(--wm-accent-text);
	}

	&.is-unread {
		color: var(--wm-accent-text);

		.unread-indicator {
			display: inline-block;
			inline-size: 0.375rem;
			block-size: 0.375rem;
			background-color: var(--wm-accent);
			border-radius: var(--wm-radius-full);
			margin-inline-start: 0.125rem;
			animation: pulse 2s infinite;

			@media (prefers-reduced-motion: reduce) {
				animation: none;
			}
		}
	}
}
</style>
