<template>
	<div
		class="task loader-container draggable"
		:class="{
			'is-loading': loadingInternal || loading,
			'draggable': !(loadingInternal || loading),
			'has-light-text': !colorIsDark(color),
			'has-custom-background-color': color ?? undefined,
		}"
		:style="{'background-color': color ?? undefined}"
		:data-task-id="task.id"
		:data-project-id="task.projectId"
		:data-is-overdue="isOverdue || undefined"
		@click.exact="openTaskDetail()"
		@click.ctrl="() => toggleTaskDone(task)"
		@click.meta="() => toggleTaskDone(task)"
	>
		<img
			v-if="coverImageBlobUrl"
			:src="coverImageBlobUrl"
			alt=""
			class="tw:w-full"
		>
		<div class="p-2">
			<div class="tw:flex tw:justify-between">
				<span class="task-id">
					<Done
						class="kanban-card__done"
						:is-done="task.done"
						variant="small"
					/>
					{{ getTaskIdentifier(task) }}
					<span
						v-if="showTaskPosition"
						class="tw:text-red-600 tw:ps-2"
					>
						{{ task.position }}
					</span>
				</span>
				<span
					v-if="task.dueDate > 0"
					v-tooltip="formatDateLong(task.dueDate)"
					class="due-date"
				>
					<span class="icon">
						<Icon :icon="['far', 'calendar-alt']" />
					</span>
					<time :datetime="formatISO(task.dueDate)">
						{{ formatDisplayDate(task.dueDate) }}
					</time>
				</span>
			</div>
			
			<h3>
				<RouterLink
					:to="{ name: 'task.detail', params: {id: task.id} }"
					class="kanban-card__title-link"
					draggable="false"
					@click.exact.prevent.stop="openTaskDetail()"
					@click.ctrl.stop
					@click.meta.stop
				>
					{{ task.title }}
				</RouterLink>
			</h3>
			
			<span
				v-if="projectTitle"
				class="project-title"
			>
				{{ projectTitle }}
			</span>

			<ProgressBar
				v-if="task.percentDone > 0"
				class="task-progress"
				:value="task.percentDone * 100"
			/>
			<div class="footer">
				<Labels :labels="task.labels" />
				<PriorityLabel
					:priority="task.priority"
					:done="task.done"
					class="is-inline-flex is-align-items-center"
				/>
				<span
					v-if="task.attachments.length > 0"
					class="icon"
					role="img"
					:aria-label="$t('task.attributes.attachment', task.attachments.length)"
				>
					<Icon icon="paperclip" />
				</span>
				<span
					v-if="!isEditorContentEmpty(task.description)"
					class="icon"
				>
					<Icon icon="align-left" />
				</span>
				<span
					v-if="task.repeatAfter.amount > 0"
					class="icon"
				>
					<Icon icon="history" />
				</span>
				<CommentCount
					:task="task"
					class="project-task-icon"
				/>
				<span
					v-if="readiness?.ready"
					class="wm-badge is-ready"
				>{{ $t('task.scope.badges.ready') }}</span>
				<span
					v-else-if="readinessReason === 'blocked'"
					v-tooltip="$t('task.scope.badges.blockedTooltip')"
					class="wm-badge is-halted"
				>{{ $t('task.scope.badges.blocked') }}</span>
				<span
					v-else-if="readinessReason === 'lease_conflict'"
					v-tooltip="$t('task.scope.badges.leaseConflictTooltip')"
					class="wm-badge is-halted"
				>{{ $t('task.scope.badges.leaseConflict') }}</span>
				<span
					v-if="leaseCount > 0"
					v-tooltip="leasePatterns"
					class="wm-badge"
					role="img"
					:aria-label="$t('task.scope.badges.leases', leaseCount)"
				>
					<Icon icon="lock" />
					{{ leaseCount }}
				</span>
				<AssigneeList
					v-if="task.assignees.length > 0"
					:assignees="task.assignees"
					:avatar-size="24"
				/>
				<ChecklistSummary
					:task="task"
					class="checklist"
				/>
			</div>
		</div>
	</div>
</template>

<script lang="ts" setup>
import {computed, ref, watch} from 'vue'
import {useRouter} from 'vue-router'

import {useGlobalNow} from '@/composables/useGlobalNow'

import PriorityLabel from '@/components/tasks/partials/PriorityLabel.vue'
import ProgressBar from '@/components/misc/ProgressBar.vue'
import Done from '@/components/misc/Done.vue'
import Labels from '@/components/tasks/partials/Labels.vue'
import ChecklistSummary from './ChecklistSummary.vue'
import CommentCount from './CommentCount.vue'

import {getHexColor, getTaskIdentifier} from '@/models/task'
import type {ITask} from '@/modelTypes/ITask'
import type {TaskReadiness} from '@/client/generated'
import {READINESS_REASON} from '@/client/queries/taskScope'
import type {IProject} from '@/modelTypes/IProject'
import {SUPPORTED_IMAGE_SUFFIX} from '@/models/attachment'
import AttachmentService, {PREVIEW_SIZE} from '@/services/attachment'

import {formatDateLong, formatDisplayDate, formatISO} from '@/helpers/time/formatDate'
import {colorIsDark} from '@/helpers/color/colorIsDark'
import {useTaskStore} from '@/stores/tasks'
import AssigneeList from '@/components/tasks/partials/AssigneeList.vue'
import {playPopSound} from '@/helpers/playPop'
import {isEditorContentEmpty} from '@/helpers/editorContentEmpty'
import {useProjectStore} from '@/stores/projects'
import {TASK_REPEAT_MODES} from '@/types/IRepeatMode'

const props = withDefaults(defineProps<{
	task: ITask,
	projectId: IProject['id'],
	loading?: boolean,
	readiness?: TaskReadiness,
}>(), {
	loading: false,
	readiness: undefined,
})

const emit = defineEmits<{
	'taskCompletedRecurring': [task: ITask]
}>()
// Assigned tasks already show their avatars; only the two reasons a human
// has to act on get a badge.
const readinessReason = computed(() => {
	const reasons = props.readiness?.reasons ?? []
	if (reasons.includes(READINESS_REASON.BLOCKED)) {
		return READINESS_REASON.BLOCKED
	}
	if (reasons.includes(READINESS_REASON.LEASE_CONFLICT)) {
		return READINESS_REASON.LEASE_CONFLICT
	}
	return null
})
const leaseCount = computed(() => props.task.leases?.length ?? 0)
const leasePatterns = computed(() => (props.task.leases ?? []).map(l => l.pattern).join('\n'))

const router = useRouter()

const loadingInternal = ref(false)

const color = computed(() => getHexColor(props.task.hexColor))

const projectStore = useProjectStore()

const projectTitle = computed(() => {
	if (props.projectId === props.task.projectId) {
		return
	}
	
	const project = projectStore.projects[props.task.projectId]
	return project?.title
})

const showTaskPosition = computed(() => window.DEBUG_TASK_POSITION)

const {now} = useGlobalNow()
const isOverdue = computed(() => (
	!props.task.done &&
	props.task.dueDate !== null &&
	props.task.dueDate.getTime() > 0 &&
	props.task.dueDate.getTime() <= now.value.getTime()
))

async function toggleTaskDone(task: ITask) {
	const isRecurringTask = task.repeatAfter.amount > 0 || task.repeatMode === TASK_REPEAT_MODES.REPEAT_MODE_MONTH
	const wasBeingMarkedDone = !task.done
	
	loadingInternal.value = true
	try {
		const updatedTask = await useTaskStore().update({
			...task,
			done: !task.done,
		})

		if (updatedTask.done) {
			playPopSound()
		}
		
		// Emit event if this was a recurring task being marked as done
		if (isRecurringTask && wasBeingMarkedDone && updatedTask.done) {
			emit('taskCompletedRecurring', updatedTask)
		}
	} finally {
		loadingInternal.value = false
	}
}

function openTaskDetail() {
	router.push({
		name: 'task.detail',
		params: {id: props.task.id},
		state: {backdropView: router.currentRoute.value.fullPath},
	})
}

const coverImageBlobUrl = ref<string | null>(null)

async function maybeDownloadCoverImage() {
	if (!props.task.coverImageAttachmentId) {
		coverImageBlobUrl.value = null
		return
	}

	const attachment = props.task.attachments.find(a => a.id === props.task.coverImageAttachmentId)
	if (!attachment || !SUPPORTED_IMAGE_SUFFIX.some((suffix) => attachment.file.name.toLowerCase().endsWith(suffix))) {
		return
	}

	const attachmentService = new AttachmentService()
	coverImageBlobUrl.value = await attachmentService.getBlobUrl(attachment, PREVIEW_SIZE.LG)
}

watch(
	() => props.task.coverImageAttachmentId,
	maybeDownloadCoverImage,
	{immediate: true},
)
</script>

<style lang="scss" scoped>
// The card is a flat panel: surface, hairline, one cut corner. clip-path slices
// a border open along the diagonal, so the hairline is traced off the clipped
// silhouette instead.
.task {
	-webkit-touch-callout: none; // iOS Safari
	user-select: none;
	cursor: pointer;
	display: block;

	font-size: var(--wm-text-sm);
	border-radius: 0;
	background: var(--wm-surface);
	color: var(--wm-text);
	overflow: hidden;

	@include chamfer(var(--wm-chamfer-sm), bottom-right);
	@include chamfer-outline(var(--wm-line));

	&.loader-container.is-loading::after {
		inline-size: 1.5rem;
		block-size: 1.5rem;
		inset-block-start: calc(50% - .75rem);
		inset-inline-start: calc(50% - .75rem);
		border-width: 2px;
	}

	h3 {
		font-family: $family-sans-serif;
		font-size: var(--wm-text-sm);
		font-weight: 500;
		letter-spacing: 0;
		line-height: 1.35;
		color: inherit;
		word-break: break-word;
	}

	.kanban-card__title-link {
		color: inherit;
		text-decoration: none;
	}

	.due-date {
		float: inline-end;
		display: flex;
		align-items: center;
		color: var(--wm-text-tertiary);

		@include mono-data;

		font-size: var(--wm-text-2xs);

		.icon {
			margin-inline-end: var(--wm-space-1);
		}

	}

	&[data-is-overdue] .due-date {
		color: var(--danger-text);
	}

	.label-wrapper .tag {
		margin: var(--wm-space-2) var(--wm-space-2) 0 0;
	}

	// The metadata row: mono, quiet, and evenly spaced — a readout, not chips.
	.footer {
		background: transparent;
		padding: 0;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--wm-space-2);
		margin-block-start: var(--wm-space-2);
		color: var(--wm-text-tertiary);
		font-size: var(--wm-text-2xs);

		.wm-badge {
			@include mono-label;

			display: inline-flex;
			align-items: center;
			gap: 4px;
			padding: 1px var(--wm-space-1);
			color: var(--wm-text-secondary);
			border: 1px solid var(--wm-line-strong);

			&.is-ready,
			&.is-halted {
				color: var(--wm-text);

				&::before {
					content: '';
					inline-size: 6px;
					block-size: 6px;
				}
			}

			&.is-ready {
				border-color: var(--success);

				&::before {
					background: var(--success);
				}
			}

			&.is-halted {
				border-color: var(--warning);

				&::before {
					background: var(--warning);
				}
			}

			.icon {
				font-size: inherit;
			}
		}

		:deep(.checklist-summary) {
			padding-inline-start: 0;
		}

		.assignees {
			display: flex;

			.user {
				display: inline;
				margin: 0;

				img {
					margin: 0;
				}
			}
		}
	}

	.footer .icon {
		color: var(--wm-text-tertiary);
	}

	// The identifier is the card's call sign: mono, tracked, quiet.
	.task-id {
		@include mono-label;

		color: var(--wm-text-tertiary);
		margin-block-end: var(--wm-space-1);
		display: flex;
		align-items: center;
	}

	.project-title {
		color: var(--wm-text-tertiary);
		font-size: var(--wm-text-2xs);
		margin-block-end: var(--wm-space-1);
		display: flex;
	}

	&.is-moving {
		opacity: .5;
	}

	span {
		inline-size: auto;
	}

	&.has-custom-background-color {
		color: #000000; // pure black, not grey-800: guarantees 4.5:1 at the luminance flip point

		// beat component-level color: var(--wm-text-tertiary) so secondary text tracks the guaranteed main text color
		.task-id,
		.project-title,
		.due-date,
		.footer,
		.footer .icon {
			color: inherit;
		}

		.footer :deep(.checklist-summary) {
			color: inherit;
		}
	}

	&.has-light-text {
		--white: hsla(var(--white-h), var(--white-s), var(--white-l), var(--white-a)) !important;
		color: var(--white);

		.footer {
			.icon svg {
				fill: var(--white);
			}
		}

		// beat component-level color: var(--wm-text-tertiary) so secondary text tracks the guaranteed main text color
		.task-id,
		.project-title,
		.due-date,
		.footer,
		.footer .icon {
			color: inherit;
		}

		.footer :deep(.checklist-summary) {
			color: inherit;
		}

		// var(--danger-text) fails on a dark custom card colour; brightened red keeps hue/sat, hits >= 4.5:1
		&[data-is-overdue] .due-date,
		:deep(.priority-label.high-priority) {
			color: hsl(var(--danger-h), var(--danger-s), 68%);
		}
	}
}

// The dragged card is one of the few things that genuinely floats. A clipped
// element cannot cast a box-shadow, so the elevation is traced off the same
// alpha mask the hairline uses.
.task-dragging .task {
	filter:
		drop-shadow(1px 0 0 var(--wm-line-strong))
		drop-shadow(-1px 0 0 var(--wm-line-strong))
		drop-shadow(0 1px 0 var(--wm-line-strong))
		drop-shadow(0 -1px 0 var(--wm-line-strong))
		drop-shadow(0 10px 20px hsla(var(--dark-h), var(--dark-s), var(--dark-l), 0.35));
}

.kanban-card__done {
	margin-inline-end: var(--wm-space-1);
}

.task-progress {
	margin: var(--wm-space-2) 0 0;
	inline-size: 100%;
	block-size: 0.375rem;
}

:deep(.comment-count) {
	font-size: var(--wm-text-2xs);
}
</style>
