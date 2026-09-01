<template>
	<div class="content has-text-centered">
		<header class="greeting">
			<p class="greeting__eyebrow">
				<time :datetime="todayISO">{{ today }}</time>
			</p>
			<h1
				v-if="salutation"
				class="greeting__title"
			>
				{{ salutation }}
			</h1>
		</header>

		<Message
			v-if="deletionScheduledAt !== null"
			variant="danger"
			class="mbe-4"
		>
			{{
				$t('user.deletion.scheduled', {
					date: formatDisplayDate(deletionScheduledAt),
					dateSince: formatDateSince(deletionScheduledAt),
				})
			}}
			<RouterLink :to="{name: 'user.settings.deletion'}">
				{{ $t('user.deletion.scheduledCancel') }}
			</RouterLink>
		</Message>
		<AddTask
			class="is-max-width-desktop"
			@tasksAdded="updateTaskKey"
		/>
		<ImportHint v-if="tasksLoaded" />
		<div
			v-if="authStore.settings.frontendSettings.showLastViewed !== false && projectHistory.length > 0"
			class="is-max-width-desktop has-text-start mbs-4"
		>
			<h2 class="section-heading">
				<span class="section-heading__label">{{ $t('home.lastViewed') }}</span>
				<span
					class="section-heading__rule"
					aria-hidden="true"
				/>
			</h2>
			<ProjectCardGrid
				v-cy="'projectCardGrid'"
				:projects="projectHistory"
				:show-even-number-of-projects="true"
			/>
		</div>
		<ShowTasks
			v-if="projectStore.hasProjects"
			:key="showTasksKey"
			:label-ids="labelIds"
			class="show-tasks"
			@tasksLoaded="tasksLoaded = true"
			@clearLabelFilter="handleClearLabelFilter"
		/>
	</div>
</template>

<script lang="ts" setup>
import {ref, computed} from 'vue'
import {useRoute, useRouter} from 'vue-router'

import Message from '@/components/misc/Message.vue'
import ShowTasks from '@/views/tasks/ShowTasks.vue'
import ProjectCardGrid from '@/components/project/partials/ProjectCardGrid.vue'
import AddTask from '@/components/tasks/AddTask.vue'
import ImportHint from '@/components/home/ImportHint.vue'

import {getHistory} from '@/modules/projectHistory'
import {parseDateOrNull} from '@/helpers/parseDateOrNull'
import {formatDate, formatDateSince, formatDisplayDate, formatISO} from '@/helpers/time/formatDate'
import {useDaytimeSalutation} from '@/composables/useDaytimeSalutation'
import {useGlobalNow} from '@/composables/useGlobalNow'

import {useProjectStore} from '@/stores/projects'
import {useAuthStore} from '@/stores/auth'

const salutation = useDaytimeSalutation()

const {now} = useGlobalNow()
const today = computed(() => formatDate(now.value, 'ddd, DD MMM YYYY'))
const todayISO = computed(() => formatISO(now.value))

const authStore = useAuthStore()
const projectStore = useProjectStore()
const route = useRoute()
const router = useRouter()

const projectHistory = computed(() => {
	// If we don't check this, it tries to load the project background right after logging out	
	if(!authStore.authenticated) {
		return []
	}
	
	return getHistory()
		.map(l => projectStore.projects[l.id])
		.filter(l => Boolean(l))
})

const tasksLoaded = ref(false)

const deletionScheduledAt = computed(() => parseDateOrNull(authStore.info?.deletionScheduledAt))

// Extract label IDs from query parameter
const labelIds = computed(() => {
	const labelsParam = route.query.labels
	if (!labelsParam) {
		return undefined
	}
	return Array.isArray(labelsParam) ? labelsParam : [labelsParam]
})

// This is to reload the tasks list after adding a new task through the global task add.
// FIXME: Should use pinia (somehow?)
const showTasksKey = ref(0)

function updateTaskKey() {
	showTasksKey.value++
}

function handleClearLabelFilter() {
	const query = {...route.query}
	delete query.labels
	router.push({
		name: route.name as string,
		query,
	})
}
</script>

<style scoped lang="scss">
// Nested one level deeper than the class alone so these outrank Bulma's
// `.content p:not(:last-child)` / `.content h1:not(:first-child)` margins.
.greeting {
	margin-block-end: var(--wm-space-5);

	.greeting__eyebrow {
		@include mono-label;
		@include mono-data;

		color: var(--wm-text-tertiary);
		margin-block: 0 var(--wm-space-2);
	}

	.greeting__title {
		font-family: $workman-display-font;
		font-size: var(--wm-text-2xl);
		letter-spacing: var(--wm-tracking-tight);
		line-height: 1.1;
		margin-block: 0;
		text-wrap: balance;

		@media screen and (max-width: $tablet) {
			font-size: var(--wm-text-xl);
		}
	}
}

// Section eyebrow: a mono label followed by a hairline that fills the row.
// Prefixed with .content so it outranks Bulma's `.content h2:not(:first-child)`.
.content .section-heading {
	display: flex;
	align-items: center;
	gap: var(--wm-space-3);
	margin-block: 0 var(--wm-space-3);
}

.section-heading__label {
	@include mono-label;

	color: var(--wm-text-secondary);
}

.section-heading__rule {
	flex: 1 1 auto;
	min-inline-size: var(--wm-space-4);
	border-block-start: 1px solid var(--wm-line);
}

.show-tasks {
	margin-block-start: var(--wm-space-6);
}
</style>
