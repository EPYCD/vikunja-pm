<template>
	<span
		ref="triggerRef"
		class="task-glance-trigger"
		@mouseenter="handleMouseEnter"
		@mouseleave="handleMouseLeave"
		@focusin="handleFocusIn"
		@focusout="handleFocusOut"
	>
		<slot />
	</span>

	<Teleport :to="teleportTarget">
		<CustomTransition name="fade">
			<div
				v-if="showTooltip"
				:id="tooltipId"
				ref="tooltipRef"
				class="task-glance-tooltip"
				role="tooltip"
			>
				<div class="task-glance-content">
					<div class="task-glance-header">
						<div class="task-glance-title-section">
							<span class="task-identifier">{{ taskIdentifier }}</span>
							<span class="task-title">{{ task.title }}</span>
						</div>
						<div class="task-glance-indicators">
							<span
								v-if="task.attachments.length > 0"
								class="task-glance-icon"
							>
								<Icon icon="paperclip" />
							</span>
							<CommentCount
								:task="task"
								class="task-glance-icon"
							/>
						</div>
					</div>

					<div
						v-if="descriptionPreview"
						class="task-glance-description"
					>
						{{ descriptionPreview }}
					</div>
							
					<ChecklistSummary
						:task="task"
						class="task-glance-checklist-summary"
					/>

					<Labels
						v-if="task.labels.length > 0"
						:labels="task.labels"
						class="task-glance-labels"
					/>

					<div
						v-if="task.dueDate"
						class="task-glance-due"
					>
						<Icon icon="calendar" />
						<span>{{ $t('task.detail.due', {at: formatDisplayDate(task.dueDate)}) }}</span>
					</div>

					<div class="task-glance-meta">
						<div class="task-glance-created">
							<i18n-t
								keypath="task.detail.created"
								scope="global"
							>
								<span>{{ formatDisplayDate(task.created) }}</span>
								{{ getDisplayName(task.createdBy) }}
							</i18n-t>
						</div>
					</div>
				</div>
			</div>
		</CustomTransition>
	</Teleport>
</template>

<script setup lang="ts">
import {ref, computed, onUnmounted, nextTick, useId} from 'vue'
import {computePosition, flip, offset, shift} from '@floating-ui/dom'
import {useMediaQuery} from '@vueuse/core'

import type {ITask} from '@/modelTypes/ITask'
import {getTaskIdentifier} from '@/models/task'
import {formatDisplayDate} from '@/helpers/time/formatDate'
import {getDisplayName} from '@/models/user'
import {isEditorContentEmpty} from '@/helpers/editorContentEmpty'
import {getTopLayerContainer} from '@/helpers/getTopLayerContainer'

import Labels from '@/components/tasks/partials/Labels.vue'
import ChecklistSummary from '@/components/tasks/partials/ChecklistSummary.vue'
import CommentCount from '@/components/tasks/partials/CommentCount.vue'
import CustomTransition from '@/components/misc/CustomTransition.vue'

const props = defineProps<{
	task: ITask
}>()

const HOVER_DELAY = 1000 // 1 second
const MAX_DESCRIPTION_LENGTH = 150

// Taps on touch devices emulate mouseenter, which would show the tooltip unexpectedly.
const canHover = useMediaQuery('(hover: hover) and (pointer: fine)')

const triggerRef = ref<HTMLElement | null>(null)
const tooltipRef = ref<HTMLElement | null>(null)
const showTooltip = ref(false)
const tooltipId = useId()
const teleportTarget = ref<HTMLElement | string>('body')
let hoverTimeout: ReturnType<typeof setTimeout> | null = null
// The trigger span is not focusable itself, so aria-describedby has to go on whatever child took focus.
let describedElement: HTMLElement | null = null

const taskIdentifier = computed(() => getTaskIdentifier(props.task))

const descriptionPreview = computed(() => {
	if (isEditorContentEmpty(props.task.description)) {
		return ''
	}

	const doc = new DOMParser().parseFromString(props.task.description, 'text/html')
	const plainText = doc.body.textContent || ''

	const trimmedText = plainText.trim()
	if (trimmedText.length <= MAX_DESCRIPTION_LENGTH) {
		return trimmedText
	}

	return trimmedText.substring(0, MAX_DESCRIPTION_LENGTH) + '…'
})

async function updatePosition() {
	if (!triggerRef.value || !tooltipRef.value) {
		return
	}

	await nextTick()

	const {x, y} = await computePosition(triggerRef.value, tooltipRef.value, {
		strategy: 'absolute',
		placement: 'top',
		middleware: [
			offset(8),
			flip({
				fallbackPlacements: ['bottom', 'top-start', 'top-end', 'bottom-start', 'bottom-end'],
				padding: 8,
			}),
			shift({padding: 8}),
		],
	})

	// Set position directly on the element
	if (tooltipRef.value) {
		tooltipRef.value.style.left = `${x}px`
		tooltipRef.value.style.top = `${y}px`
	}
}

function scheduleShow() {
	if (hoverTimeout) {
		clearTimeout(hoverTimeout)
	}

	hoverTimeout = setTimeout(async () => {
		hoverTimeout = null
		teleportTarget.value = getTopLayerContainer(triggerRef.value)
		showTooltip.value = true
		document.addEventListener('keydown', handleKeydown)
		describedElement?.setAttribute('aria-describedby', tooltipId)

		await nextTick()
		await updatePosition()
	}, HOVER_DELAY)
}

function hide() {
	if (hoverTimeout) {
		clearTimeout(hoverTimeout)
		hoverTimeout = null
	}

	showTooltip.value = false
	document.removeEventListener('keydown', handleKeydown)
	describedElement?.removeAttribute('aria-describedby')
	describedElement = null
}

function handleKeydown(event: KeyboardEvent) {
	if (event.key !== 'Escape' || !showTooltip.value) {
		return
	}

	// Swallow only the Escape that dismissed the tooltip; preventDefault keeps a native <dialog> open.
	event.stopPropagation()
	event.preventDefault()
	hide()
}

function handleMouseEnter() {
	if (!canHover.value) {
		return
	}

	scheduleShow()
}

function handleMouseLeave() {
	hide()
}

function handleFocusIn(event: FocusEvent) {
	describedElement = event.target as HTMLElement
	scheduleShow()
}

function handleFocusOut() {
	hide()
}

onUnmounted(hide)
</script>

<style lang="scss" scoped>
.task-glance-trigger {
	display: inline;
}

// Genuinely floating, so it keeps its shadow — squared off and hairlined like
// every other raised surface.
.task-glance-tooltip {
	position: absolute;
	inset-block-start: 0;
	inset-inline-start: 0;
	z-index: 9999;
	max-inline-size: 400px;
	background: var(--wm-surface-raised);
	border: 1px solid var(--wm-line);
	border-radius: 0;
	box-shadow: var(--shadow-lg);
	pointer-events: none;
}

.task-glance-content {
	padding: var(--wm-space-3);
	display: flex;
	flex-direction: column;
	gap: var(--wm-space-2);
	font-size: var(--wm-text-sm);
	color: var(--wm-text);
}

.task-glance-header {
	display: flex;
	align-items: flex-start;
	justify-content: space-between;
	gap: var(--wm-space-4);
}

.task-glance-title-section {
	display: flex;
	flex-direction: column;
	gap: var(--wm-space-1);
	flex: 1;
	min-inline-size: 0; /* Allow text to wrap */
}

.task-identifier {
	@include mono-label;

	color: var(--wm-text-tertiary);
}

.task-title {
	font-weight: 600;
	color: var(--wm-text);
	word-wrap: break-word;
}

.task-glance-description {
	color: var(--wm-text-secondary);
	font-size: var(--wm-text-sm);
	line-height: 1.4;
	word-wrap: break-word;
	white-space: pre-wrap;
}

.task-glance-labels {
	margin: 0;
}

.task-glance-due {
	display: flex;
	align-items: center;
	gap: var(--wm-space-2);
	color: var(--wm-text-secondary);
	font-size: var(--wm-text-xs);

	.icon {
		inline-size: 1rem;
		block-size: 1rem;
	}
}

.task-glance-meta {
	display: flex;
	flex-direction: column;
	gap: var(--wm-space-1);
	padding-block-start: var(--wm-space-2);
	border-block-start: 1px solid var(--wm-line-faint);
	font-size: var(--wm-text-2xs);
	color: var(--wm-text-tertiary);
}

.task-glance-created {
	@include mono-data;

	display: flex;
	align-items: center;
	gap: var(--wm-space-2);
}

.task-glance-indicators {
	display: flex;
	align-items: center;
	flex-shrink: 0; /* Prevent icons from shrinking */

	:deep(.checklist-summary) {
		padding-inline-end: 6px;

		&:not(:last-child) {
			padding-inline-end: 8px;
		}
	}
}

.task-glance-icon {
	color: var(--wm-text-tertiary);
	font-size: var(--wm-text-xs);
	display: inline-flex;
	align-items: center;
	margin-inline-end: 6px;

	&:not(:last-of-type) {
		margin-inline-end: 8px;
	}
}

.task-glance-checklist-summary {
	margin-block-start: calc(var(--wm-space-1) * -1);
}
</style>
