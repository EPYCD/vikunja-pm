<template>
	<ProjectWrapper
		class="project-kanban"
		:is-loading-project="isLoadingProject"
		:project-id="projectId"
		:view-id
	>
		<template #header>
			<div class="filter-container">
				<XButton
					v-if="!isSavedFilter(project)"
					v-tooltip="$t('task.leasesPanel.readyOnlyHint')"
					variant="secondary"
					icon="check-double"
					class="kanban-ready-toggle"
					:class="{'is-active': readyOnly}"
					:aria-pressed="readyOnly"
					@click="readyOnly = !readyOnly"
				>
					{{ $t('task.leasesPanel.readyOnly') }}
				</XButton>
				<XButton
					v-if="!isSavedFilter(project)"
					v-tooltip="$t('project.kanban.byAssigneeHint')"
					variant="secondary"
					icon="users"
					class="kanban-lanes-toggle"
					:class="{'is-active': byAssignee}"
					:aria-pressed="byAssignee"
					@click="byAssignee = !byAssignee"
				>
					{{ $t('project.kanban.byAssignee') }}
				</XButton>
				<ProjectLeasesPanel
					v-if="!isSavedFilter(project)"
					:project-id="projectIdWithFallback"
					:can-write="canWrite"
				/>
				<ProjectMarshalPanel v-if="!isSavedFilter(project) && marshalUrl" />
				<FilterPopup
					v-if="!isSavedFilter(project)"
					v-model="params"
					:view-id="viewId"
					:project-id="projectId"
					@update:modelValue="updateFilters"
				/>
			</div>
		</template>

		<template #default>
			<div class="kanban-view">
				<div
					:class="{ 'is-loading': loading && !oneTaskUpdating}"
					class="kanban kanban-bucket-container loader-container"
				>
					<draggable
						v-bind="DRAG_OPTIONS"
						:model-value="buckets"
						group="buckets"
						:disabled="!canWrite || newTaskInputFocused"
						tag="ul"
						:item-key="({id}: IBucket) => `bucket${id}`"
						:component-data="bucketDraggableComponentData"
						@update:modelValue="updateBuckets"
						@end="updateBucketPosition"
						@start="() => dragBucket = true"
					>
						<template #item="{element: bucket, index: bucketIndex }">
							<li
								class="bucket"
								:class="{'is-collapsed': collapsedBuckets[bucket.id]}"
							>
								<div
									class="bucket-header"
									@click="() => unCollapseBucket(bucket)"
								>
									<span
										v-if="bucket.id !== 0 && view?.doneBucketId === bucket.id"
										v-tooltip="$t('project.kanban.doneBucketHint')"
										class="icon is-small has-text-success mie-2"
										@click.stop="() => collapseBucket(bucket)"
									>
										<Icon icon="check-double" />
									</span>
									<span
										v-if="bucket.id !== 0 && view?.claimBucketId === bucket.id"
										v-tooltip="$t('project.kanban.claimBucketHint')"
										class="icon is-small kanban-claim-badge mie-2"
									>
										<Icon icon="lock" />
									</span>
									<h2
										class="title input"
										:contenteditable="(bucketTitleEditable && canWrite && !collapsedBuckets[bucket.id]) ? true : undefined"
										:spellcheck="false"
										@keydown.enter.prevent.stop="!$event.isComposing && ($event.target as HTMLElement).blur()"
										@keydown.esc.prevent.stop="!$event.isComposing && ($event.target as HTMLElement).blur()"
										@blur="saveBucketTitle(bucket.id, ($event.target as HTMLElement).textContent as string)"
										@click="focusBucketTitle"
									>
										{{ bucket.title }}
									</h2>
									<span
										v-if="bucket.limit > 0 || alwaysShowBucketTaskCount"
										:class="{'is-max': bucket.limit > 0 && bucket.count >= bucket.limit}"
										class="limit"
									>
										{{ bucket.limit > 0 ? `${bucket.count}/${bucket.limit}` : bucket.count }}
									</span>
									<Dropdown
										v-if="canWrite && !collapsedBuckets[bucket.id]"
										class="is-right options"
										trigger-icon="ellipsis-v"
										:trigger-label="$t('project.kanban.bucketOptions')"
										@close="() => showSetLimitInput = false"
									>
										<div
											v-if="showSetLimitInput"
											class="field has-addons"
										>
											<div class="control">
												<input
													ref="bucketLimitInputRef"
													v-focus.always
													:value="bucket.limit"
													class="input"
													type="number"
													min="0"
													@keyup.esc="() => showSetLimitInput = false"
													@keyup.enter="() => {setBucketLimit(bucket.id, true); showSetLimitInput = false}"
													@input="setBucketLimit(bucket.id)"
												>
											</div>
											<div class="control">
												<XButton
													v-cy="'setBucketLimit'"
													:aria-label="$t('misc.save')"
													:disabled="bucket.limit < 0"
													:icon="['far', 'save']"
													:shadow="false"
													@click="() => {setBucketLimit(bucket.id, true); showSetLimitInput = false}"
												/>
											</div>
										</div>
										<DropdownItem
											v-else
											@click.stop="showSetLimitInput = true"
										>
											{{
												$t('project.kanban.limit', {limit: bucket.limit > 0 ? bucket.limit : $t('misc.notSet')})
											}}
										</DropdownItem>
										<DropdownItem
											v-tooltip="$t('project.kanban.doneBucketHintExtended')"
											:icon-class="{'has-text-success': bucket.id === view?.doneBucketId}"
											icon="check-double"
											@click.stop="toggleDoneBucket(bucket)"
										>
											{{ $t('project.kanban.doneBucket') }}
										</DropdownItem>
										<DropdownItem
											v-tooltip="$t('project.kanban.claimBucketHintExtended')"
											:icon-class="{'has-text-primary': bucket.id === view?.claimBucketId}"
											icon="lock"
											@click.stop="toggleClaimBucket(bucket)"
										>
											{{ bucket.id === view?.claimBucketId ? $t('project.kanban.claimBucketUnset') : $t('project.kanban.claimBucket') }}
										</DropdownItem>
										<DropdownItem
											v-tooltip="$t('project.kanban.defaultBucketHint')"
											:icon-class="{'has-text-primary': bucket.id === view?.defaultBucketId}"
											icon="th"
											@click.stop="toggleDefaultBucket(bucket)"
										>
											{{ $t('project.kanban.defaultBucket') }}
										</DropdownItem>
										<DropdownItem
											icon="angles-up"
											@click.stop="() => collapseBucket(bucket)"
										>
											{{ $t('project.kanban.collapse') }}
										</DropdownItem>
										<DropdownItem
											v-tooltip="buckets.length <= 1 ? $t('project.kanban.deleteLast') : ''"
											class="has-text-danger"
											:class="{'is-disabled': buckets.length <= 1}"
											icon-class="has-text-danger"
											icon="trash-alt"
											@click.stop="() => deleteBucketModal(bucket.id)"
										>
											{{ $t('misc.delete') }}
										</DropdownItem>
									</Dropdown>
								</div>

								<ul
									v-if="byAssignee"
									class="tasks kanban-lanes"
								>
									<li
										v-for="lane in lanesFor(bucket)"
										:key="lane.key"
										class="kanban-lane"
									>
										<div class="kanban-lane__head">
											<span class="kanban-lane__label">
												{{ lane.label }}
												<span
													v-if="lane.owner"
													class="kanban-lane__owner"
												>· {{ $t('project.kanban.laneOwner', {user: lane.owner}) }}</span>
												/ {{ lane.tasks.length }}
											</span>
											<span
												v-if="lane.leases > 0"
												class="kanban-lane__leases"
											><Icon icon="lock" /> {{ lane.leases }}</span>
										</div>
										<ul class="kanban-lane__tasks">
											<li
												v-for="task in lane.tasks"
												:key="task.id"
												class="task-item"
												:class="{'is-hidden': hiddenByReadiness(bucket, task)}"
												:data-task-id="task.id"
											>
												<KanbanCard
													class="kanban-card"
													:task="task"
													:loading="taskUpdating[task.id] ?? false"
													:project-id="projectId"
													:readiness="readinessByTaskId[task.id]"
													@taskCompletedRecurring="handleRecurringTaskCompletion"
												/>
											</li>
										</ul>
									</li>
								</ul>
								<draggable
									v-else
									v-bind="DRAG_OPTIONS"
									:handle="taskDragHandle"
									:delay="isTouchDevice ? 300 : 1000"
									:model-value="bucket.tasks"
									:group="{name: 'tasks', put: shouldAcceptDrop(bucket) && !dragBucket}"
									:disabled="!canWrite"
									:data-bucket-index="bucketIndex"
									tag="ul"
									:item-key="(task: ITask) => `bucket${bucket.id}-task${task.id}`"
									:component-data="getTaskDraggableTaskComponentData(bucket)"
									@update:modelValue="(tasks) => updateTasks(bucket.id, tasks)"
									@start="handleTaskDragStart"
									@end="updateTaskPosition"
								>
									<template #footer>
										<li
											v-if="canCreateTasks"
											class="bucket-footer"
										>
											<div
												v-if="showNewTaskInput === bucket.id"
												class="field"
											>
												<div
													class="control"
													:class="{'is-loading': loading || taskLoading}"
												>
													<input
														v-model="newTaskText"
														v-focus.always
														class="input"
														:disabled="loading || taskLoading || undefined"
														:placeholder="$t('project.kanban.addTaskPlaceholder')"
														type="text"
														@focusout="toggleShowNewTaskInput(bucket.id)"
														@focusin="() => newTaskInputFocused = true"
														@keyup.enter="addTaskToBucket(bucket.id)"
														@keyup.esc="toggleShowNewTaskInput(bucket.id)"
													>
												</div>
												<p
													v-if="newTaskError[bucket.id] && newTaskText === ''"
													class="help is-danger"
												>
													{{ $t('project.create.addTitleRequired') }}
												</p>
											</div>
											<XButton
												v-else
												v-tooltip="bucket.limit > 0 && bucket.count >= bucket.limit ? $t('project.kanban.bucketLimitReached') : ''"
												class="is-fullwidth has-text-centered"
												:shadow="false"
												icon="plus"
												variant="secondary"
												:disabled="bucket.limit > 0 && bucket.count >= bucket.limit"
												@click="toggleShowNewTaskInput(bucket.id)"
											>
												{{
													bucket.tasks.length === 0 ? $t('project.kanban.addTask') : $t('project.kanban.addAnotherTask')
												}}
											</XButton>
										</li>
									</template>

									<template #item="{element: task}">
										<li
											class="task-item"
											:class="{'is-hidden': hiddenByReadiness(bucket, task)}"
											:data-task-id="task.id"
										>
											<span
												v-if="canWrite && isTouchDevice"
												class="handle"
												@click="openTask(task)"
												@touchstart.passive="onHandleTouchStart"
												@touchmove.passive="onHandleTouchMove"
											/>
											<KanbanCard
												class="kanban-card"
												:task="task"
												:loading="taskUpdating[task.id] ?? false"
												:project-id="projectId"
												:readiness="readinessByTaskId[task.id]"
												@taskCompletedRecurring="handleRecurringTaskCompletion"
											/>
										</li>
									</template>
								</draggable>
							</li>
						</template>
					</draggable>

					<div
						v-if="canWrite && !loading && buckets.length > 0"
						class="bucket new-bucket"
					>
						<input
							v-if="showNewBucketInput"
							v-model="newBucketTitle"
							v-focus.always
							:class="{'is-loading': loading}"
							:disabled="loading || undefined"
							class="input"
							:placeholder="$t('project.kanban.addBucketPlaceholder')"
							type="text"
							@blur="() => showNewBucketInput = false"
							@keyup.enter="createNewBucket"
							@keyup.esc="($event.target as HTMLInputElement).blur()"
						>
						<XButton
							v-else
							:shadow="false"
							class="is-transparent is-fullwidth has-text-centered"
							variant="secondary"
							icon="plus"
							@click="() => showNewBucketInput = true"
						>
							{{ $t('project.kanban.addBucket') }}
						</XButton>
					</div>
				</div>

				<Modal
					:enabled="showBucketDeleteModal"
					@close="showBucketDeleteModal = false"
					@submit="deleteBucket()"
				>
					<template #header>
						<span>{{ $t('project.kanban.deleteHeaderBucket') }}</span>
					</template>

					<template #text>
						<p>
							{{ $t('project.kanban.deleteBucketText1') }}<br>
							{{ $t('project.kanban.deleteBucketText2') }}
						</p>
					</template>
				</Modal>
			</div>
		</template>
	</ProjectWrapper>
</template>

<script setup lang="ts">
import {computed, nextTick, ref, watch, toRef} from 'vue'
import {useRouter} from 'vue-router'
import {useRouteQuery} from '@vueuse/router'
import {useStorage} from '@vueuse/core'
import {useI18n} from 'vue-i18n'
import {useQuery} from '@tanstack/vue-query'
import draggable from 'zhyswan-vuedraggable'
import {klona} from 'klona/lite'

import {PERMISSIONS as Permissions} from '@/constants/permissions'
import BucketModel from '@/models/bucket'

import type {IBucket} from '@/modelTypes/IBucket'
import type {ITask} from '@/modelTypes/ITask'

import {useBaseStore} from '@/stores/base'
import {useTaskStore} from '@/stores/tasks'
import {useKanbanStore} from '@/stores/kanban'
import {useAuthStore} from '@/stores/auth'
import {useConfigStore} from '@/stores/config'

import ProjectWrapper from '@/components/project/ProjectWrapper.vue'
import FilterPopup from '@/components/project/partials/FilterPopup.vue'
import ProjectLeasesPanel from '@/components/project/partials/ProjectLeasesPanel.vue'
import ProjectMarshalPanel from '@/components/project/partials/ProjectMarshalPanel.vue'
import KanbanCard from '@/components/tasks/partials/KanbanCard.vue'
import Dropdown from '@/components/misc/Dropdown.vue'
import DropdownItem from '@/components/misc/DropdownItem.vue'

import {
	type CollapsedBuckets,
	getCollapsedBucketState,
	saveCollapsedBucketState,
} from '@/helpers/saveCollapsedBucketState'
import {calculateItemPosition} from '@/helpers/calculateItemPosition'

import {isSavedFilter, useSavedFilter} from '@/services/savedFilter'
import {useTaskDragToProject} from '@/composables/useTaskDragToProject'
import {success} from '@/message'
import {useProjectStore} from '@/stores/projects'
import {getDisplayName} from '@/models/user'
import {invalidateReadiness, projectAgentsQuery, readinessByTask, viewReadinessQuery} from '@/client/queries/taskScope'
import type {TaskFilterParams} from '@/services/taskCollection'
import type {IProjectView} from '@/modelTypes/IProjectView'
import TaskPositionService from '@/services/taskPosition'
import TaskPositionModel from '@/models/taskPosition'
import {i18n} from '@/i18n'
import ProjectViewService from '@/services/projectViews'
import ProjectViewModel from '@/models/projectView'
import TaskBucketService from '@/services/taskBucket'
import TaskBucketModel from '@/models/taskBucket'

const props = defineProps<{
	isLoadingProject: boolean,
	projectId: number,
	viewId: IProjectView['id'],
}>()

const projectId = toRef(props, 'projectId')

const DRAG_OPTIONS = {
	// sortable options
	animation: 150,
	ghostClass: 'ghost',
	dragClass: 'task-dragging',
	delayOnTouchOnly: true,
	delay: 1000,
} as const

const MIN_SCROLL_HEIGHT_PERCENT = 0.25

const {t} = useI18n({useScope: 'global'})

const baseStore = useBaseStore()
const kanbanStore = useKanbanStore()
const taskStore = useTaskStore()
const projectStore = useProjectStore()
const authStore = useAuthStore()
const configStore = useConfigStore()

const alwaysShowBucketTaskCount = computed(() => authStore.settings.frontendSettings.alwaysShowBucketTaskCount)
const marshalUrl = computed(() => configStore.marshalUrl)
const {handleTaskDropToProject} = useTaskDragToProject()
const taskPositionService = ref(new TaskPositionService())
const taskBucketService = ref(new TaskBucketService())

// Saved filter composable for accessing filter data
const savedFilter = useSavedFilter(() => isSavedFilter({id: projectId.value}) ? projectId.value : undefined).filter

const taskContainerRefs = ref<{ [id: IBucket['id']]: HTMLElement }>({})
const bucketLimitInputRef = ref<HTMLInputElement | null>(null)

const drag = ref(false)
const dragBucket = ref(false)
const sourceBucket = ref(0)

const showBucketDeleteModal = ref(false)
const bucketToDelete = ref(0)
const bucketTitleEditable = ref(false)

const newTaskText = ref('')
const showNewTaskInput = ref<IBucket['id'] | null>(null)

const newBucketTitle = ref('')
const showNewBucketInput = ref(false)
const newTaskError = ref<{ [id: IBucket['id']]: boolean }>({})
const newTaskInputFocused = ref(false)

const showSetLimitInput = ref(false)
const collapsedBuckets = ref<CollapsedBuckets>({})

// We're using this to show the loading animation only at the task when updating it
const taskUpdating = ref<{ [id: ITask['id']]: boolean }>({})
const oneTaskUpdating = ref(false)

// URL-synchronized filter parameters
const filter = useRouteQuery('filter')
const s = useRouteQuery('s')

const params = ref<TaskFilterParams>({
	sort_by: [],
	order_by: [],
	filter: '',
	filter_include_nulls: false,
	s: '',
})

watch([filter, s], ([filterValue, sValue]) => {
	params.value.filter = filterValue ?? ''
	params.value.s = sValue ?? ''
}, { immediate: true })

function updateFilters(newParams: TaskFilterParams) {
	// Update all params
	params.value = { ...newParams }
	
	// Sync only filter and s to URL
	filter.value = newParams.filter || undefined
	s.value = newParams.s || undefined
}

const getTaskDraggableTaskComponentData = computed(() => (bucket: IBucket) => {
	return {
		ref: (el: HTMLElement) => setTaskContainerRef(bucket.id, el),
		onScroll: (event: Event) => handleTaskContainerScroll(bucket.id, event.target as HTMLElement),
		type: 'transition-group',
		name: !drag.value ? 'move-card' : null,
		class: [
			'tasks',
			{'dragging-disabled': !canWrite.value},
		],
	}
})

const bucketDraggableComponentData = computed(() => ({
	type: 'transition-group',
	name: !dragBucket.value ? 'move-bucket' : null,
	class: [
		'kanban-bucket-container',
		{'dragging-disabled': !canWrite.value},
	],
}))
const project = computed(() => projectId.value ? projectStore.projects[projectId.value] : null)
const view = computed(() => project.value?.views.find(v => v.id === props.viewId) as IProjectView || null)
const canWrite = computed(() => baseStore.currentProject?.maxPermission > Permissions.READ && view.value.bucketConfigurationMode === 'manual')
const canCreateTasks = computed(() => canWrite.value && projectId.value > 0)

const isTouchDevice = ref(false)
if (typeof window !== 'undefined') {
	isTouchDevice.value = !window.matchMedia('(hover: hover) and (pointer: fine)').matches
}
const taskDragHandle = computed(() => isTouchDevice.value ? '.handle' : undefined)

const router = useRouter()
const touchStartY = ref(0)

function openTask(task: ITask) {
	router.push({
		name: 'task.detail',
		params: {id: task.id},
		state: {backdropView: router.currentRoute.value.fullPath},
	})
}

function onHandleTouchStart(e: TouchEvent) {
	touchStartY.value = e.touches[0].clientY
}

function onHandleTouchMove(e: TouchEvent) {
	if (drag.value) return

	const currentY = e.touches[0].clientY
	const deltaY = touchStartY.value - currentY
	const scrollContainer = (e.target as HTMLElement).closest('.tasks') as HTMLElement | null
	if (scrollContainer) {
		scrollContainer.scrollTop += deltaY
		touchStartY.value = currentY
	}
}

const buckets = computed(() => kanbanStore.buckets)
const loading = computed(() => kanbanStore.isLoading)
const projectIdWithFallback = computed<number>(() => project.value?.id || projectId.value)

// The board's READY / BLOCKED / PATH LEASED badges come from the same
// server answer agents read, so the two never disagree.
const readinessQuery = useQuery(computed(() => ({
	...viewReadinessQuery(projectIdWithFallback.value, props.viewId),
	enabled: projectIdWithFallback.value > 0,
})))
const readinessByTaskId = computed(() => readinessByTask(readinessQuery.data.value ?? []))

// "Ready only" trims the queue bucket to what an agent could claim now. The
// cards stay in the draggable list and are only hidden, so positions and
// drag targets are unaffected.
const readyOnly = useStorage('kanbanReadyOnly', false)

// "By assignee" answers "who is doing what" per column: cards grouped under
// the agent (or human) holding them, unassigned last. Read-only while on,
// since dragging across lanes has no meaning for positions.
const byAssignee = useStorage('kanbanByAssignee', false)
interface AssigneeLane {
	key: string
	label: string
	owner: string | null
	tasks: ITask[]
	leases: number
}

// A bot lane names the human behind it; that answer lives on the agents
// endpoint, not on the assignee embedded in the task.
const agentsQuery = useQuery(computed(() => ({
	...projectAgentsQuery(projectIdWithFallback.value),
	enabled: byAssignee.value && projectIdWithFallback.value > 0,
})))
const ownerByUserId = computed<Record<number, string>>(() => {
	const out: Record<number, string> = {}
	for (const agent of agentsQuery.data.value ?? []) {
		const owner = agent.owner?.name || agent.owner?.username
		if (agent.user?.id && owner) {
			out[agent.user.id] = owner
		}
	}
	return out
})
function lanesFor(bucket: IBucket): AssigneeLane[] {
	const lanes = new Map<string, AssigneeLane>()
	const unassigned: AssigneeLane = {key: 'unassigned', label: t('project.kanban.unassignedLane'), owner: null, tasks: [], leases: 0}
	for (const task of bucket.tasks) {
		if (task.assignees.length === 0) {
			unassigned.tasks.push(task)
			unassigned.leases += task.leases?.length ?? 0
			continue
		}
		for (const user of task.assignees) {
			const key = `user-${user.id}`
			let lane = lanes.get(key)
			if (!lane) {
				lane = {key, label: getDisplayName(user), owner: ownerByUserId.value[user.id] ?? null, tasks: [], leases: 0}
				lanes.set(key, lane)
			}
			lane.tasks.push(task)
			lane.leases += task.leases?.length ?? 0
		}
	}
	const out = [...lanes.values()].sort((a, b) => a.label.localeCompare(b.label))
	if (unassigned.tasks.length > 0) {
		out.push(unassigned)
	}
	return out
}
function hiddenByReadiness(bucket: IBucket, task: ITask): boolean {
	if (!readyOnly.value || bucket.id !== view.value?.defaultBucketId) {
		return false
	}
	const readiness = readinessByTaskId.value[task.id]
	return readiness !== undefined && !readiness.ready
}

const taskLoading = computed(() => taskStore.isLoading || taskPositionService.value.loading)

watch(
	() => ({
		params: params.value,
		projectId: projectId.value,
		viewId: props.viewId,
	}),
	({params, projectId, viewId}) => {
		if (projectId === undefined || Number(projectId) === 0) {
			return
		}
		collapsedBuckets.value = getCollapsedBucketState(projectId)
		kanbanStore.loadBucketsForProject(projectId, viewId, params)
		invalidateReadiness(projectId, viewId)
	},
	{
		immediate: true,
		deep: true,
	},
)

function setTaskContainerRef(id: IBucket['id'], el: HTMLElement) {
	if (!el) return
	taskContainerRefs.value[id] = el
}

function handleTaskContainerScroll(id: IBucket['id'], el: HTMLElement) {
	if (!el) {
		return
	}
	const scrollTopMax = el.scrollHeight - el.clientHeight
	const threshold = el.scrollTop + el.scrollTop * MIN_SCROLL_HEIGHT_PERCENT
	if (scrollTopMax > threshold) {
		return
	}

	kanbanStore.loadNextTasksForBucket(
		projectId.value,
		props.viewId,
		params.value,
		id,
	)
}

function updateTasks(bucketId: IBucket['id'], tasks: IBucket['tasks']) {
	const bucket = kanbanStore.getBucketById(bucketId)

	if (bucket === undefined) {
		return
	}

	kanbanStore.setBucketById({
		...bucket,
		tasks,
	})
}

async function updateTaskPosition(e) {
	drag.value = false

	// Check if dropped on a sidebar project
	const {moved} = await handleTaskDropToProject(e, (task) => {
		kanbanStore.removeTaskInBucket(task)
	})

	if (moved) {
		return
	}

	// If dropped outside kanban
	if (!e.to.dataset.bucketIndex) {
		return
	}

	// While we could just pass the bucket index in through the function call, this would not give us the
	// new bucket id when a task has been moved between buckets, only the new bucket. Using the data-bucket-id
	// of the drop target works all the time.
	const bucketIndex = parseInt(e.to.dataset.bucketIndex)

	const newBucket = buckets.value[bucketIndex]

	// HACK:
	// this is a hacky workaround for a known problem of vue.draggable.next when using the footer slot
	// the problem: https://github.com/SortableJS/vue.draggable.next/issues/108
	// This hack doesn't remove the problem that the ghost item is still displayed below the footer
	// It just makes releasing the item possible.

	// The newIndex of the event doesn't count in the elements of the footer slot.
	// This is why in case the length of the tasks is identical with the newIndex
	// we have to remove 1 to get the correct index.
	const newTaskIndex = newBucket.tasks.length === e.newIndex
		? e.newIndex - 1
		: e.newIndex

	const task = newBucket.tasks[newTaskIndex]
	const oldBucket = buckets.value.find(b => b.id === sourceBucket.value)
	const taskBefore = newBucket.tasks[newTaskIndex - 1] ?? null
	const taskAfter = newBucket.tasks[newTaskIndex + 1] ?? null
	taskUpdating.value[task.id] = true

	const newTask = klona(task) // cloning the task to avoid pinia store manipulation
	newTask.bucketId = newBucket.id
	const position = calculateItemPosition(
		taskBefore !== null ? taskBefore.position : null,
		taskAfter !== null ? taskAfter.position : null,
	)
	
	let bucketHasChanged = false
	if (
		oldBucket !== undefined && // This shouldn't actually be `undefined`, but let's play it safe.
		newBucket.id !== oldBucket.id
	) {
		kanbanStore.setBucketById({
			...oldBucket,
			count: oldBucket.count - 1,
		})
		kanbanStore.setBucketById({
			...newBucket,
			count: newBucket.count + 1,
		})
		bucketHasChanged = true
	}

	try {
		const newPosition = new TaskPositionModel({
			position,
			projectViewId: props.viewId,
			taskId: newTask.id,
		})
		await taskPositionService.value.update(newPosition)
		newTask.position = position
		
		if(bucketHasChanged) {
			const updatedTaskBucket = await taskBucketService.value.update(new TaskBucketModel({
				taskId: newTask.id,
				bucketId: newTask.bucketId,
				projectViewId: props.viewId,
				projectId: projectIdWithFallback.value,
			}))
			Object.assign(newTask, updatedTaskBucket.task)
			if (updatedTaskBucket.bucketId !== newTask.bucketId) {
				kanbanStore.moveTaskToBucket(newTask, updatedTaskBucket.bucketId)
			}
			newTask.bucketId = updatedTaskBucket.bucketId
			if (updatedTaskBucket.bucket) {
				kanbanStore.setBucketById(updatedTaskBucket.bucket, false)
			}
		}
		kanbanStore.setTaskInBucket(newTask)

		// Make sure the first and second task don't both get position 0 assigned
		if (newTaskIndex === 0 && taskAfter !== null && taskAfter.position === 0) {
			const taskAfterAfter = newBucket.tasks[newTaskIndex + 2] ?? null
			const newTaskAfter = klona(taskAfter) // cloning the task to avoid pinia store manipulation
			newTaskAfter.bucketId = newBucket.id
			newTaskAfter.position = calculateItemPosition(
				0,
				taskAfterAfter !== null ? taskAfterAfter.position : null,
			)

			await taskStore.update(newTaskAfter)
		}
	} finally {
		taskUpdating.value[task.id] = false
		oneTaskUpdating.value = false
	}

	invalidateReadiness(projectIdWithFallback.value, props.viewId)
}

function toggleShowNewTaskInput(bucketId: IBucket['id']) {
	if (loading.value || taskLoading.value) {
		return
	}
	showNewTaskInput.value = showNewTaskInput.value === bucketId 
		? null
		: bucketId
	newTaskInputFocused.value = false
}

async function addTaskToBucket(bucketId: IBucket['id']) {
	if (newTaskText.value === '') {
		newTaskError.value[bucketId] = true
		return
	}
	newTaskError.value[bucketId] = false

	const task = await taskStore.createNewTask({
		title: newTaskText.value,
		bucketId,
		projectId: projectIdWithFallback.value,
	})
	newTaskText.value = ''
	kanbanStore.addTaskToBucket(task)
	scrollTaskContainerToTop(bucketId)

	const bucket = kanbanStore.getBucketById(bucketId)
	if (bucket && bucket.limit && bucket.count >= bucket.limit) {
		toggleShowNewTaskInput(bucketId)
	}
}

function scrollTaskContainerToTop(bucketId: IBucket['id']) {
	const bucketEl = taskContainerRefs.value[bucketId]
	if (!bucketEl) {
		return
	}
	bucketEl.scrollTop = 0
}

async function createNewBucket() {
	if (newBucketTitle.value === '') {
		return
	}

	await kanbanStore.createBucket(new BucketModel({
		title: newBucketTitle.value,
		projectId: projectIdWithFallback.value,
		projectViewId: props.viewId,
	}))
	newBucketTitle.value = ''
}

function deleteBucketModal(bucketId: IBucket['id']) {
	if (buckets.value.length <= 1) {
		return
	}

	bucketToDelete.value = bucketId
	showBucketDeleteModal.value = true
}

async function deleteBucket() {
	try {
		await kanbanStore.deleteBucket({
			bucket: new BucketModel({
				id: bucketToDelete.value,
				projectId: projectIdWithFallback.value,
				projectViewId: props.viewId,
			}),
			params: params.value,
		})
		success({message: t('project.kanban.deleteBucketSuccess')})
	} finally {
		showBucketDeleteModal.value = false
	}
}

/** This little helper allows us to drag a bucket around at the title without focusing on it right away. */
async function focusBucketTitle(e: Event) {
	bucketTitleEditable.value = true
	await nextTick()
	const target = e.target as HTMLInputElement
	target.focus()
}

async function saveBucketTitle(bucketId: IBucket['id'], bucketTitle: string) {
	
	const bucket = kanbanStore.getBucketById(bucketId)
	if (bucket?.title === bucketTitle) {
		bucketTitleEditable.value = false
		return
	}
	
	await kanbanStore.updateBucket({
		id: bucketId,
		title: bucketTitle,
		projectId: projectId.value,
	})
	success({message: i18n.global.t('project.kanban.bucketTitleSavedSuccess')})
	bucketTitleEditable.value = false
}

function updateBuckets(value: IBucket[]) {
	// (1) buckets get updated in store and tasks positions get invalidated
	kanbanStore.setBuckets(value)
}

function handleRecurringTaskCompletion() {
	// Only reload if we're in a saved filter and the filter contains date fields
	if (!isSavedFilter(project.value)) {
		return
	}

	const filterContainsDateFields = savedFilter.value?.filters?.filter?.includes('due_date') ||
		savedFilter.value?.filters?.filter?.includes('start_date') ||
		savedFilter.value?.filters?.filter?.includes('end_date')
		
	if (filterContainsDateFields) {
		// Reload the kanban board to refresh tasks that now match/don't match the filter
		kanbanStore.loadBucketsForProject(projectId.value, props.viewId, params.value)
	}
}

// TODO: fix type
function updateBucketPosition(e: { newIndex: number }) {
	// (2) bucket positon is changed
	dragBucket.value = false

	const bucket = buckets.value[e.newIndex]
	const bucketBefore = buckets.value[e.newIndex - 1] ?? null
	const bucketAfter = buckets.value[e.newIndex + 1] ?? null

	kanbanStore.updateBucket({
		id: bucket.id,
		projectId: projectId.value,
		position: calculateItemPosition(
			bucketBefore !== null ? bucketBefore.position : null,
			bucketAfter !== null ? bucketAfter.position : null,
		),
	})
}

async function saveBucketLimit(bucketId: IBucket['id'], limit: number) {
	if (limit < 0) {
		return
	}

	await kanbanStore.updateBucket({
		...kanbanStore.getBucketById(bucketId),
		projectId: projectId.value,
		limit,
	})
	success({message: t('project.kanban.bucketLimitSavedSuccess')})
}

const setBucketLimitCancel = ref<number | null>(null)

async function setBucketLimit(bucketId: IBucket['id'], now: boolean = false) {
	const limit = parseInt(bucketLimitInputRef.value?.value || '')

	if (setBucketLimitCancel.value !== null) {
		clearTimeout(setBucketLimitCancel.value)
	}

	if (now) {
		return saveBucketLimit(bucketId, limit)
	}

	setBucketLimitCancel.value = setTimeout(saveBucketLimit, 2500, bucketId, limit)
}

function shouldAcceptDrop(bucket: IBucket) {
	return (
		// When dragging from a bucket who has its limit reached, dragging should still be possible
		bucket.id === sourceBucket.value ||
		// If there is no limit set, dragging & dropping should always work
		bucket.limit === 0 ||
		// Disallow dropping to buckets which have their limit reached
		bucket.count < bucket.limit
	)
}

function dragstart(bucket: IBucket) {
	drag.value = true
	sourceBucket.value = bucket.id
}

function handleTaskDragStart(e) {
	const taskId = parseInt(e.item.dataset.taskId, 10)
	const bucketIndex = parseInt(e.from.dataset.bucketIndex, 10)
	const bucket = buckets.value[bucketIndex]
	const task = bucket?.tasks.find(t => t.id === taskId)

	if (task) {
		taskStore.setDraggedTask(task)
	}
	dragstart(bucket)
}

async function saveViewBucketRole(patch: Partial<IProjectView>, message: string) {
	const projectViewService = new ProjectViewService()
	const updatedView = await projectViewService.update(new ProjectViewModel({
		...view.value,
		...patch,
	}))

	const views = project.value.views.map(v => v.id === view.value?.id ? updatedView : v)
	projectStore.setProject({
		...project.value,
		views,
	})

	success({message})
}

async function toggleDefaultBucket(bucket: IBucket) {
	await saveViewBucketRole(
		{defaultBucketId: view.value?.defaultBucketId === bucket.id ? 0 : bucket.id},
		t('project.kanban.defaultBucketSavedSuccess'),
	)
}

async function toggleDoneBucket(bucket: IBucket) {
	await saveViewBucketRole(
		{doneBucketId: view.value?.doneBucketId === bucket.id ? 0 : bucket.id},
		t('project.kanban.doneBucketSavedSuccess'),
	)
}

async function toggleClaimBucket(bucket: IBucket) {
	await saveViewBucketRole(
		{claimBucketId: view.value?.claimBucketId === bucket.id ? 0 : bucket.id},
		t('project.kanban.claimBucketSavedSuccess'),
	)
}

function collapseBucket(bucket: IBucket) {
	collapsedBuckets.value[bucket.id] = true
	saveCollapsedBucketState(projectIdWithFallback.value, collapsedBuckets.value)
}

function unCollapseBucket(bucket: IBucket) {
	if (!collapsedBuckets.value[bucket.id]) {
		return
	}

	collapsedBuckets.value[bucket.id] = false
	saveCollapsedBucketState(projectIdWithFallback.value, collapsedBuckets.value)
}
</script>

<style lang="scss" scoped>
.control.is-loading {
  &::after {
    inset-block-start: 30%;
    inset-inline-end: 50%;
    transform: translate(-50%, 0);

	--loader-border-color: var(--wm-text-tertiary);
  }
}
</style>


<style lang="scss">
$ease-out: all .3s cubic-bezier(0.23, 1, 0.32, 1);
$bucket-width: 300px;
$bucket-header-height: 52px;
$bucket-right-margin: 1rem;
$crazy-height-calculation: '100vh - 4.5rem - 1.5rem - 1rem - 1.5rem - 11px';
$crazy-height-calculation-tasks: '#{$crazy-height-calculation} - 1rem - 2.5rem - 2rem - #{$button-height} - 1rem';
$filter-container-height: '1rem - #{$switch-view-height}';

.filter-container {
	display: flex;
	align-items: center;
	gap: var(--wm-space-2);
}

.kanban-lanes {
	margin: 0;
	padding: 0;
	list-style: none;
}

.kanban-lane {
	margin-block-end: var(--wm-space-3);
}

.kanban-lane__head {
	display: flex;
	align-items: center;
	justify-content: space-between;
	gap: var(--wm-space-2);
	padding: var(--wm-space-1) 0;
	margin-block-end: var(--wm-space-1);
	border-block-end: 1px solid var(--wm-line-faint);
}

.kanban-lane__label,
.kanban-lane__leases {
	@include mono-label;

	color: var(--wm-text-secondary);
}

.kanban-lane__leases {
	display: inline-flex;
	align-items: center;
	gap: 4px;
	color: var(--wm-accent-text);
}

.kanban-lane__owner {
	color: var(--wm-text-tertiary);
}

.kanban-lane__tasks {
	margin: 0;
	padding: 0;
	list-style: none;
}

.kanban-lanes-toggle.is-active,
.kanban-ready-toggle.is-active {
	background: var(--wm-accent-wash);
	color: var(--wm-accent-text);

	@include chamfer-outline(var(--wm-accent-line));
}

// The board floor: a faint blueprint graticule so the columns read as
// instruments placed on a grid rather than cards floating in space.
.kanban {
	@include graticule;

	overflow-x: auto;
	overflow-y: hidden;
	block-size: calc(#{$crazy-height-calculation});
	margin: 0 -1.5rem;
	padding: 0 1.5rem;

	&:focus, .bucket .tasks:focus {
		box-shadow: none;
	}

	@media screen and (max-width: $tablet) {
		block-size: calc(#{$crazy-height-calculation} - #{$filter-container-height} + 9px);
		scroll-snap-type: x mandatory;
		margin: 0 -0.5rem;
	}

	&-bucket-container {
		display: flex;
		list-style: none;
	}

	.ghost {
		position: relative;

		* {
			opacity: 0;
		}

		&::after {
			content: '';
			position: absolute;
			display: block;
			inset-block-start: var(--wm-space-1);
			inset-inline-end: var(--wm-space-2);
			inset-block-end: var(--wm-space-1);
			inset-inline-start: var(--wm-space-2);
			border: 1px dashed var(--wm-line-strong);
			background: var(--wm-accent-wash);
		}
	}

	.bucket {
		position: relative;
		background: var(--wm-surface-sunken);
		border: 1px solid var(--wm-line);

		margin: 0 $bucket-right-margin 0 0;
		max-block-size: calc(100% - 1rem); // 1rem spacing to the bottom
		min-block-size: 20px;
		inline-size: $bucket-width;
		display: flex;
		flex-direction: column;
		overflow: hidden;

		@media screen and (max-width: $tablet) {
			scroll-snap-align: center;
		}

		.tasks {
			overflow: hidden auto;
			block-size: 100%;
			list-style: none;
		}

		.task-item {
			padding: var(--wm-space-1) var(--wm-space-2);
			position: relative;

			&:first-of-type {
				padding-block-start: var(--wm-space-2);
			}

			&:last-of-type {
				padding-block-end: var(--wm-space-2);
			}

			.handle {
				position: absolute;
				inset: 0;
				z-index: 1;
				opacity: 0;
				touch-action: none;
				-webkit-touch-callout: none;
				user-select: none;
			}
		}

		.no-move {
			transition: transform 0s;
		}

		h2 {
			margin: 0;
		}

		&.new-bucket {
			// Because of reasons, this button ignores the margin we gave it to the right.
			// To make it still look like it has some, we modify the container to have a padding of 1rem,
			// which is the same as the margin it should have. Then we make the container itself bigger
			// to hide the fact we just made the button smaller.
			min-inline-size: calc(#{$bucket-width} + 1rem);
			background: transparent;
			border: 1px dashed var(--wm-line-strong);

			.button {
				background: transparent;
				inline-size: 100%;
			}
		}

		&.is-collapsed {
			align-self: flex-start;
			transform: rotate(90deg) translateY(-100%);
			transform-origin: top left;
			// Using negative margins instead of translateY here to make all other buckets fill the empty space
			margin-inline-end: calc((#{$bucket-width} - #{$bucket-header-height} - #{$bucket-right-margin}) * -1);
			cursor: pointer;

			.tasks, .bucket-footer {
				display: none;
			}
		}
	}

	// Column header: a mono micro-label with its count, sitting on a hairline.
	.bucket-header {
		background-color: var(--wm-surface-sunken);
		border-block-end: 1px solid var(--wm-line);
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: var(--wm-space-2);
		block-size: $bucket-header-height;

		.icon.has-text-success {
			cursor: pointer;
		}

		.kanban-claim-badge {
			color: var(--wm-accent-text);
		}

		.limit {
			@include mono-label;

			padding: 0 var(--wm-space-2);
			color: var(--wm-text-tertiary);

			&.is-max {
				color: var(--danger-text);
			}
		}

		.title.input {
			@include mono-label($size: var(--wm-text-xs));

			font-weight: 500 !important;
			color: var(--wm-text);
			background: transparent;
			border: 0;
			box-shadow: none;
			block-size: auto;
			padding: var(--wm-space-1) var(--wm-space-2);
			display: inline-block;
			cursor: pointer;

			&:hover {
				background: var(--wm-surface-hover);
			}
		}
	}

	:deep(.dropdown-trigger) {
		padding: var(--wm-space-2);
	}

	.bucket-footer {
		position: sticky;
		inset-block-end: 0;
		z-index: 2;
		block-size: min-content;
		padding: var(--wm-space-2);
		background-color: var(--wm-surface-sunken);
		border-block-start: 1px solid var(--wm-line-faint);
		transform: none;

		.button {
			background-color: transparent;

			&:hover {
				background-color: var(--wm-surface-hover);
			}
		}
	}
}

// FIXME: This does not seem to work
.task-dragging {
	transform: rotateZ(3deg);
	transition: transform var(--wm-duration-slow) var(--wm-ease);
}

.move-card-move {
	transform: rotateZ(3deg);
	transition: transform var(--wm-duration) var(--wm-ease);
}

.move-card-leave-from,
.move-card-leave-to,
.move-card-leave-active {
	display: none;
}
</style>
