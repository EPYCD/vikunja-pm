<template>
	<Modal
		:enabled="active"
		:overflow="isNewTaskCommand"
		variant="top"
		:aria-label="$t('quickActions.title')"
		@close="closeQuickActions"
	>
		<div
			ref="quickActionsCard"
			class="card quick-actions"
			:class="{'is-quick-add-mode': isQuickAddMode}"
			:style="isQuickAddMode ? {maxHeight: quickEntryMaxHeight + 'px', overflowY: 'auto'} : undefined"
		>
			<div
				class="action-input"
				:class="{'has-active-cmd': selectedCmd !== null}"
			>
				<div
					v-if="selectedCmd !== null"
					class="active-cmd tag"
				>
					{{ selectedCmd.title }}
				</div>
				<input
					ref="searchInput"
					v-model="query"
					v-focus
					class="input"
					:class="{'is-loading': loading}"
					:placeholder="placeholder"
					@keyup="search"
					@keydown.down.prevent="select(0, 0)"
					@keyup.prevent.delete="unselectCmd"
					@keydown.prevent.enter="onEnter"
					@keyup.prevent.esc="closeQuickActions"
				>
				<QuickAddMagic
					v-if="isNewTaskCommand"
				/>
				<BaseButton
					:aria-label="$t('misc.closeQuickActions')"
					class="close"
					@click="closeQuickActions"
				>
					<Icon icon="times" />
				</BaseButton>
			</div>

			<div
				v-if="hintText !== '' && !isNewTaskCommand"
				class="help quick-actions-hint"
			>
				{{ hintText }}
			</div>

			<div
				class="is-sr-only"
				role="status"
				aria-live="polite"
			>
				{{ resultAnnouncement }}
			</div>

			<div
				v-if="selectedCmd === null"
				class="results"
			>
				<div
					v-for="(r, k) in results"
					:key="k"
					class="result"
				>
					<span class="result-title">
						{{ r.title }}
					</span>
					<div class="result-items">
						<BaseButton
							v-for="(i, key) in r.items"
							:key="key"
							:ref="(el: Element | ComponentPublicInstance | null) => setResultRefs(el, k, key)"
							class="result-item-button"
							:class="{'is-strikethrough': isDone(i)}"
							@keydown.up.prevent="select(k, key - 1)"
							@keydown.down.prevent="select(k, key + 1)"
							@click.prevent.stop="doAction(r.type, i)"
							@keyup.prevent.enter="doAction(r.type, i)"
							@keyup.prevent.esc="searchInput?.focus()"
						>
							<template v-if="r.type === ACTION_TYPE.LABELS">
								<XLabel :label="i" />
							</template>
							<template v-else-if="r.type === ACTION_TYPE.TASK">
								<SingleTaskInlineReadonly
									:task="i"
									:show-project="true"
								/>
								<span
									v-if="isDone(i)"
									class="is-sr-only"
								>{{ $t('task.attributes.done') }}</span>
							</template>
							<template v-else>
								<span
									v-if="i.id < -1"
									class="saved-filter-icon icon"
								>
									<Icon icon="filter" />
								</span>
								{{ i.title }}
							</template>
							<span class="is-sr-only">{{ r.typeLabel }}</span>
						</BaseButton>
					</div>
				</div>
			</div>
		</div>
	</Modal>
</template>

<script setup lang="ts">
import {type ComponentPublicInstance, computed, ref, shallowReactive, watch, watchEffect, onBeforeUnmount} from 'vue'
import {useQuickAddMode} from '@/composables/useQuickAddMode'
import {useI18n} from 'vue-i18n'
import {useRouter} from 'vue-router'

import TaskService from '@/services/task'
import TeamService from '@/services/team'

import TeamModel from '@/models/team'
import ProjectModel from '@/models/project'

import BaseButton from '@/components/base/BaseButton.vue'
import QuickAddMagic from '@/components/tasks/partials/QuickAddMagic.vue'
import XLabel from '@/components/tasks/partials/Label.vue'
import SingleTaskInlineReadonly from '@/components/tasks/partials/SingleTaskInlineReadonly.vue'

import {useBaseStore} from '@/stores/base'
import {useProjectStore} from '@/stores/projects'
import {useTaskStore} from '@/stores/tasks'
import {useAuthStore} from '@/stores/auth'
import {useLabels} from '@/composables/useLabels'

import {getHistory} from '@/modules/projectHistory'
import {parseTaskText, PREFIXES, PrefixMode} from '@/modules/quickAddMagic'
import {success} from '@/message'

import type {ITeam} from '@/modelTypes/ITeam'
import type {ITask} from '@/modelTypes/ITask'
import type {IProject} from '@/modelTypes/IProject'
import type {IAbstract} from '@/modelTypes/IAbstract'
import {isSavedFilter} from '@/services/savedFilter'
import type {TaskFilterParams} from '@/services/taskCollection'

const {t} = useI18n({useScope: 'global'})
const router = useRouter()

const baseStore = useBaseStore()
const projectStore = useProjectStore()
const {filterLabelsByQuery, getLabelsByExactTitles} = useLabels()
const taskStore = useTaskStore()
const authStore = useAuthStore()

const {isQuickAddMode} = useQuickAddMode()

type DoAction<Type> = { type: ACTION_TYPE } & Type

enum ACTION_TYPE {
	CMD = 'cmd',
	TASK = 'task',
	PROJECT = 'project',
	TEAM = 'team',
	LABELS = 'labels',
}

enum COMMAND_TYPE {
	NEW_TASK = 'newTask',
	NEW_PROJECT = 'newProject',
	NEW_TEAM = 'newTeam',
}

enum SEARCH_MODE {
	ALL = 'all',
	TASKS = 'tasks',
	PROJECTS = 'projects',
	TEAMS = 'teams',
}

const query = ref('')
const selectedCmd = ref<Command | null>(null)

const foundTasks = ref<DoAction<ITask>[]>([])
const taskService = shallowReactive(new TaskService())

const foundTeams = ref<ITeam[]>([])
const teamService = shallowReactive(new TeamService())

const active = computed(() => baseStore.quickActionsActive)

watchEffect(() => {
	if (!active.value) {
		reset()
	}
})

let focusRafId: number | null = null

watchEffect(() => {
	if (active.value) {
		if (isQuickAddMode) {
			selectedCmd.value = commands.value.newTask
		}

		// The input may not be focusable yet due to:
		// 1. Modal mounts the <dialog> via v-if and then calls showModal() in a
		//    follow-up flush, so v-focus fires while the dialog is still closed
		//    and the focus() call is dropped.
		// 2. In quick-add mode the Electron window isn't visible until
		//    did-finish-load.
		// Retry with rAF until focus actually lands on the input.
		const tryFocus = () => {
			if (!active.value) {
				focusRafId = null
				return
			}
			if (searchInput.value) {
				searchInput.value.focus()
				if (document.activeElement === searchInput.value) {
					focusRafId = null
					return
				}
			}
			focusRafId = requestAnimationFrame(tryFocus)
		}
		focusRafId = requestAnimationFrame(tryFocus)
	} else if (focusRafId !== null) {
		cancelAnimationFrame(focusRafId)
		focusRafId = null
	}
})

function closeQuickActions() {
	baseStore.setQuickActionsActive(false)
}

const foundProjects = computed(() => {
	const {project, text, labels, assignees} = parsedQuery.value

	if (project !== null) {
		return projectStore.searchProjectAndFilter(project ?? text)
			.filter(p => Boolean(p))
	}

	if (labels.length > 0 || assignees.length > 0) {
		return []
	}

	if (text === '') {
		const history = getHistory()
		return history.map((p) => projectStore.projects[p.id])
			.filter(p => Boolean(p))
	}

	return projectStore.searchProjectAndFilter(project ?? text)
		.filter(p => Boolean(p))
})

const foundLabels = computed(() => {
	const {labels, text} = parsedQuery.value
	if (text === '' && labels.length === 0) {
		return []
	}

	if (labels.length > 0) {
		return filterLabelsByQuery([], labels[0])
	}

	return filterLabelsByQuery([], text)
})

// FIXME: use fuzzysearch
const foundCommands = computed(() => availableCmds.value.filter((a) =>
	a.title.toLowerCase().includes(query.value.toLowerCase()),
))

interface Result {
	type: ACTION_TYPE
	title: string
	// singular, unlike the plural group heading in `title`: it is announced per item
	typeLabel: string
	items: DoAction<IAbstract>
}

const results = computed<Result[]>(() => {
	return [
		{
			type: ACTION_TYPE.CMD,
			title: t('quickActions.commands'),
			typeLabel: t('quickActions.resultTypes.command'),
			items: foundCommands.value,
		},
		{
			type: ACTION_TYPE.PROJECT,
			title: t('quickActions.projects'),
			typeLabel: t('quickActions.resultTypes.project'),
			items: foundProjects.value,
		},
		{
			type: ACTION_TYPE.TASK,
			title: t('quickActions.tasks'),
			typeLabel: t('quickActions.resultTypes.task'),
			items: foundTasks.value,
		},
		{
			type: ACTION_TYPE.LABELS,
			title: t('quickActions.labels'),
			typeLabel: t('quickActions.resultTypes.label'),
			items: foundLabels.value,
		},
		{
			type: ACTION_TYPE.TEAM,
			title: t('quickActions.teams'),
			typeLabel: t('quickActions.resultTypes.team'),
			items: foundTeams.value,
		},
	].filter((i) => i.items.length > 0)
})

// `unknown` because Result.items isn't typed as an array, so v-for widens each item to its property union
function isDone(item: unknown): boolean {
	return Boolean((item as ITask | undefined)?.done)
}

const loading = computed(() =>
	taskService.loading ||
	projectStore.isLoading ||
	teamService.loading,
)

interface Command {
	type: COMMAND_TYPE
	title: string
	placeholder: string
	action: () => Promise<void>
}

const commands = computed<{ [key in COMMAND_TYPE]: Command }>(() => ({
	newTask: {
		type: COMMAND_TYPE.NEW_TASK,
		title: t('quickActions.cmds.newTask'),
		placeholder: t('quickActions.newTask'),
		action: newTask,
	},
	newProject: {
		type: COMMAND_TYPE.NEW_PROJECT,
		title: t('quickActions.cmds.newProject'),
		placeholder: t('quickActions.newProject'),
		action: newProject,
	},
	newTeam: {
		type: COMMAND_TYPE.NEW_TEAM,
		title: t('quickActions.cmds.newTeam'),
		placeholder: t('quickActions.newTeam'),
		action: newTeam,
	},
}))

const placeholder = computed(() => selectedCmd.value?.placeholder || t('quickActions.placeholder'))

const currentProject = computed(() => {
	if (Object.keys(baseStore.currentProject).length === 0 || isSavedFilter(baseStore.currentProject)) {
		return null
	}

	return baseStore.currentProject
})

const hintText = computed(() => {
	if (selectedCmd.value !== null && currentProject.value !== null) {
		switch (selectedCmd.value.type) {
			case COMMAND_TYPE.NEW_TASK:
				return t('quickActions.createTask', {
					title: currentProject.value.title,
				})
			case COMMAND_TYPE.NEW_PROJECT:
				return t('quickActions.createProject')
		}
	}
	const prefixes =
		PREFIXES[authStore.settings.frontendSettings.quickAddMagicMode] ?? PREFIXES[PrefixMode.Default]
	return t('quickActions.hint', prefixes)
})

const availableCmds = computed(() => {
	return [
		commands.value.newTask,
		commands.value.newProject,
		commands.value.newTeam,
	]
})

const parsedQuery = computed(() => parseTaskText(query.value, authStore.settings.frontendSettings.quickAddMagicMode))

const searchMode = computed(() => {
	if (query.value === '') {
		return SEARCH_MODE.ALL
	}

	const {text, project, labels, assignees} = parsedQuery.value
	if (assignees.length === 0 && text !== '') {
		return SEARCH_MODE.TASKS
	}

	if (
		assignees.length === 0 &&
		project !== null &&
		text === '' &&
		labels.length === 0
	) {
		return SEARCH_MODE.PROJECTS
	}

	if (
		assignees.length > 0 &&
		project === null &&
		text === '' &&
		labels.length === 0
	) {
		return SEARCH_MODE.TEAMS
	}

	return SEARCH_MODE.ALL
})

const isNewTaskCommand = computed(() => (
	selectedCmd.value !== null &&
	selectedCmd.value.type === COMMAND_TYPE.NEW_TASK
))

const taskSearchTimeout = ref<ReturnType<typeof setTimeout> | null>(null)

function searchTasks() {
	if (
		searchMode.value !== SEARCH_MODE.ALL &&
		searchMode.value !== SEARCH_MODE.TASKS &&
		searchMode.value !== SEARCH_MODE.PROJECTS
	) {
		foundTasks.value = []
		return
	}

	if (selectedCmd.value !== null) {
		return
	}

	if (taskSearchTimeout.value !== null) {
		clearTimeout(taskSearchTimeout.value)
		taskSearchTimeout.value = null
	}

	const {text, project: projectName, labels} = parsedQuery.value

	let filter = ''

	if (projectName !== null) {
		const project = projectStore.findProjectByExactname(projectName)
		console.log({project})
		if (project !== null) {
			filter += ' project = ' + project.id
		}
	}

	if (labels.length > 0) {
		const labelIds = getLabelsByExactTitles(labels)
			.map(label => label.id)
			.filter((id): id is number => typeof id === 'number')
		if (labelIds.length > 0) {
			filter += 'labels in ' + labelIds.join(', ')
		}
	}

	const params: Partial<TaskFilterParams> = {
		s: text,
		// undone tasks first, most relevant first within each group (relevance is
		// only honored on backends that can score the search, see the API docs)
		sort_by: ['done', 'relevance'],
		filter,
	}

	taskSearchTimeout.value = setTimeout(async () => {
		const r = await taskService.getAll({}, params) as DoAction<ITask>[]
		foundTasks.value = r.map((t) => {
			t.type = ACTION_TYPE.TASK
			return t
		})
	}, 150)
}

const teamSearchTimeout = ref<ReturnType<typeof setTimeout> | null>(null)

function searchTeams() {
	if (
		searchMode.value !== SEARCH_MODE.ALL &&
		searchMode.value !== SEARCH_MODE.TEAMS
	) {
		foundTeams.value = []
		return
	}
	if (query.value === '' || selectedCmd.value !== null) {
		return
	}
	if (teamSearchTimeout.value !== null) {
		clearTimeout(teamSearchTimeout.value)
		teamSearchTimeout.value = null
	}
	const {assignees} = parsedQuery.value
	teamSearchTimeout.value = setTimeout(async () => {
		const teamSearchPromises = assignees.map((t) =>
			teamService.getAll({}, {s: t}),
		)
		const teamsResult = await Promise.all(teamSearchPromises)
		foundTeams.value = teamsResult.flat().map((team) => {
			team.title = team.name
			return team
		})
	}, 150)
}

function search() {
	searchTasks()
	searchTeams()
}

const searchInput = ref<HTMLElement | null>(null)
const quickActionsCard = ref<HTMLElement | null>(null)

const QUICK_ENTRY_WIDTH = 680
const quickEntryMaxHeight = Math.round(window.screen.availHeight * 0.7)
let resizeObserver: ResizeObserver | null = null

if (isQuickAddMode) {
	watch(quickActionsCard, (el) => {
		resizeObserver?.disconnect()
		if (!el) return
		resizeObserver = new ResizeObserver((entries) => {
			for (const entry of entries) {
				const height = Math.min(
					Math.ceil(entry.borderBoxSize[0].blockSize),
					quickEntryMaxHeight,
				)
				window.quickEntry?.resize?.(QUICK_ENTRY_WIDTH, height)
			}
		})
		resizeObserver.observe(el)
	})

	onBeforeUnmount(() => {
		resizeObserver?.disconnect()
	})
}

async function doAction(type: ACTION_TYPE, item: DoAction) {
	switch (type) {
		case ACTION_TYPE.PROJECT:
			closeQuickActions()
			if (!isQuickAddMode) {
				await router.push({
					name: 'project.index',
					params: {projectId: (item as DoAction<IProject>).id},
				})
			}
			break
		case ACTION_TYPE.TASK:
			if (isQuickAddMode) {
				const channel = new BroadcastChannel('vikunja-task-updates')
				channel.postMessage({type: 'task-created-open', taskId: (item as DoAction<ITask>).id})
				channel.close()
				window.quickEntry?.showMainWindow()
			} else {
				await router.push({
					name: 'task.detail',
					params: {id: (item as DoAction<ITask>).id},
				})
			}
			closeQuickActions()
			break
		case ACTION_TYPE.TEAM:
			closeQuickActions()
			if (!isQuickAddMode) {
				await router.push({
					name: 'teams.edit',
					params: {id: (item as DoAction<ITeam>).id},
				})
			}
			break
		case ACTION_TYPE.CMD:
			query.value = ''
			selectedCmd.value = item as DoAction<Command>
			searchInput.value?.focus()
			break
		case ACTION_TYPE.LABELS:
			if (/\s/.test(item.title)) {
				query.value = '*"' + item.title + '"'
			} else {
				query.value = '*' + item.title
			}
			searchInput.value?.focus()
			searchTasks()
			break
	}
}

let openTaskAfterCreate = false

function onEnter(event: KeyboardEvent) {
	openTaskAfterCreate = event.ctrlKey || event.metaKey
	doCmd()
}

async function doCmd() {
	if (results.value.length === 1 && results.value[0].items.length === 1) {
		const result = results.value[0]
		doAction(result.type, result.items[0])
		openTaskAfterCreate = false
		return
	}

	if (selectedCmd.value === null || query.value === '') {
		openTaskAfterCreate = false
		return
	}

	if (!isQuickAddMode) {
		closeQuickActions()
	}
	await selectedCmd.value.action()
}

async function newTask() {
	let projectId = authStore.settings.defaultProjectId
	if (currentProject.value?.id && currentProject.value.id > 0) {
		projectId = currentProject.value.id
	}
	const task = await taskStore.createNewTask({
		title: query.value,
		projectId,
	})
	success({message: t('task.createSuccess')})

	if (isQuickAddMode) {
		const channel = new BroadcastChannel('vikunja-task-updates')
		const type = openTaskAfterCreate ? 'task-created-open' : 'task-created'
		channel.postMessage({type, taskId: task.id})
		channel.close()

		if (openTaskAfterCreate) {
			window.quickEntry?.showMainWindow()
		}

		closeQuickActions()
		openTaskAfterCreate = false
		return
	}

	await router.push({name: 'task.detail', params: {id: task.id}})
}

async function newProject() {
	const parentProjectId = currentProject.value?.id ?? 0
	await projectStore.createProject(new ProjectModel({
		title: query.value,
		parentProjectId: Math.max(parentProjectId, 0),
	}))
	success({message: t('project.create.createdSuccess')})
}

async function newTeam() {
	const newTeam = new TeamModel({name: query.value})
	const team = await teamService.create(newTeam)
	await router.push({
		name: 'teams.edit',
		params: {id: team.id},
	})
	success({message: t('team.create.success')})
}

type BaseButtonInstance = InstanceType<typeof BaseButton>
const resultRefs = ref<(BaseButtonInstance | null)[][]>([])

function setResultRefs(el: Element | ComponentPublicInstance | null, index: number, key: number) {
	if (resultRefs.value[index] === undefined) {
		resultRefs.value[index] = []
	}

	resultRefs.value[index][key] = el as (BaseButtonInstance | null)
}

function select(parentIndex: number, index: number) {
	if (index < 0 && parentIndex === 0) {
		searchInput.value?.focus()
		return
	}
	if (index < 0) {
		parentIndex--
		index = results.value[parentIndex].items.length - 1
	}
	let elems = resultRefs.value[parentIndex][index]
	if (results.value[parentIndex].items.length === index) {
		elems = resultRefs.value[parentIndex + 1] ? resultRefs.value[parentIndex + 1][0] : undefined
	}
	if (
		typeof elems === 'undefined'
		/* || elems.length === 0 */
	) {
		return
	}
	if (Array.isArray(elems)) {
		elems[0].focus()
		return
	}
	elems?.focus()
}

function unselectCmd() {
	if (query.value !== '') {
		return
	}
	selectedCmd.value = null
}

function reset() {
	query.value = ''
	selectedCmd.value = null
}

const resultCount = computed(() => results.value.reduce((total, group) => total + group.items.length, 0))

// Announce the result count to assistive technology, debounced so it doesn't
// fire on every keystroke while the user is still typing.
const resultAnnouncement = ref('')
let announceTimeout: ReturnType<typeof setTimeout> | null = null
watch(resultCount, count => {
	if (!active.value || selectedCmd.value !== null) {
		return
	}
	if (announceTimeout !== null) {
		clearTimeout(announceTimeout)
	}
	announceTimeout = setTimeout(() => {
		resultAnnouncement.value = t('quickActions.results', count)
	}, 300)
})
watch(active, isActive => {
	if (!isActive) {
		resultAnnouncement.value = ''
	}
})
onBeforeUnmount(() => {
	if (announceTimeout !== null) {
		clearTimeout(announceTimeout)
	}
})
</script>

<style lang="scss" scoped>
// The command palette reads as a terminal: a cut panel on the raised surface,
// a monospace prompt line, mono group headers, and an accent rail marking the
// row that is about to fire.
.quick-actions {
	background-color: var(--wm-surface-raised);
	color: var(--wm-text);
	border: none;
	border-radius: 0;
	box-shadow: none;
	overflow: hidden;
	justify-content: flex-start !important;

	@include chamfer(var(--wm-chamfer-lg));
	@include chamfer-outline(var(--wm-line-strong));

	// In quick-add mode the Electron window *is* the panel, so it keeps its
	// square edges and full bleed.
	&.is-quick-add-mode {
		padding: 0;
		margin: 0;
		border: none;
		box-shadow: none;
		clip-path: none;
		filter: none;
	}
}

.action-input {
	display: flex;
	align-items: center;
	background-color: var(--wm-surface-sunken);
	border-block-end: 1px solid var(--wm-line);

	.input {
		border: 0;
		background: transparent;
		block-size: var(--wm-control-height-lg);
		font-family: $workman-mono-font;
		font-size: var(--wm-text-lg);
		color: var(--wm-text);
		padding-inline: var(--wm-space-4);

		&::placeholder {
			color: var(--wm-text-tertiary);
		}

		&:focus {
			border: 0;
			box-shadow: none;
		}

		@media screen and (max-width: $tablet) {
			padding-inline-end: var(--wm-space-1);
		}
	}

	&.has-active-cmd .input {
		padding-inline-start: var(--wm-space-2);
	}

	.close {
		padding-block: 0;
		padding-inline: var(--wm-space-2) var(--wm-space-4);
		font-size: var(--wm-text-lg);
		color: var(--wm-text-tertiary);
		transition: color var(--wm-duration) var(--wm-ease);

		&:hover {
			color: var(--wm-text);
		}

		@media screen and (min-width: $tablet + 1) {
			display: none;
		}
	}
}

// The selected command reads as a mode chip, not a pill — the accent in this
// panel belongs to the highlighted result. Nested one level deeper than the
// class alone because Bulma's `.tag:not(body)` ties on specificity and lands
// later in the bundle.
.action-input .active-cmd {
	@include mono-label;

	margin-inline-start: var(--wm-space-4);
	padding-block: var(--wm-space-1);
	padding-inline: var(--wm-space-2);
	block-size: auto;
	border: 1px solid var(--wm-line);
	border-radius: 0;
	background-color: var(--wm-surface-hover);
	color: var(--wm-text-secondary);
}

.quick-actions-hint {
	font-family: $workman-mono-font;
	font-size: var(--wm-text-2xs);
	line-height: 1.5;
	color: var(--wm-text-tertiary);
	padding-block: var(--wm-space-2);
	padding-inline: var(--wm-space-4);
	border-block-end: 1px solid var(--wm-line-faint);
}

.results {
	text-align: start;
	inline-size: 100%;
	color: var(--wm-text);
}

.result-title {
	@include mono-label;

	display: block;
	background: var(--wm-surface-sunken);
	color: var(--wm-text-tertiary);
	border-block-end: 1px solid var(--wm-line-faint);
	padding-block: var(--wm-space-2);
	padding-inline: var(--wm-space-4);
}

.result-item-button {
	position: relative;
	inline-size: 100%;
	background: transparent;
	color: var(--wm-text);
	text-align: start;
	box-shadow: none;
	border: none;
	border-radius: 0;
	text-transform: none;
	font-family: $family-sans-serif;
	font-size: var(--wm-text-base);
	font-weight: normal;
	padding-block: var(--wm-space-2);
	padding-inline: var(--wm-space-4);
	cursor: pointer;
	transition: background-color var(--wm-duration) var(--wm-ease);

	// The rail that marks the row the palette will act on.
	&::before {
		content: '';
		position: absolute;
		inset-block: 0;
		inset-inline-start: 0;
		inline-size: $vikunja-nav-selected-width;
		background: var(--wm-accent);
		opacity: 0;
		transition: opacity var(--wm-duration) var(--wm-ease);
	}

	&:focus,
	&:hover,
	&:active {
		background: var(--wm-surface-hover);
		box-shadow: none;

		&::before {
			opacity: 1;
		}
	}

	// The panel is clipped, so an outset focus ring would be cut off at the
	// edges — this one is drawn inside the row instead.
	&:focus-visible {
		box-shadow: inset 0 0 0 2px hsla(var(--wm-accent-hsl), 0.9);
	}

	.saved-filter-icon {
		font-size: var(--wm-text-xs);
		inline-size: var(--wm-text-xs);
		margin-inline-end: var(--wm-space-1);
		color: var(--wm-text-tertiary);
	}

	&:has(.saved-filter-icon) {
		display: inline-flex;
		align-items: center;
	}
}

@media (prefers-reduced-motion: reduce) {
	.result-item-button,
	.result-item-button::before,
	.action-input .close {
		transition-duration: 1ms;
	}
}
</style>
