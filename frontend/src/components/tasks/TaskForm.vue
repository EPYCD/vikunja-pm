<template>
	<form
		class="add-new-task"
		@submit.prevent="createTask"
	>
		<CustomTransition name="width">
			<input
				v-if="newTaskFieldActive"
				ref="newTaskTitleField"
				v-model="newTaskTitle"
				class="input"
				type="text"
				@blur="hideCreateNewTask"
				@keyup.esc="newTaskFieldActive = false"
			>
		</CustomTransition>
		<XButton
			:shadow="false"
			icon="plus"
			@click="showCreateTaskOrCreate"
		>
			{{ $t('task.new') }}
		</XButton>
	</form>
</template>

<script setup lang="ts">
import {nextTick, ref} from 'vue'
import type {ITask} from '@/modelTypes/ITask'

import CustomTransition from '@/components/misc/CustomTransition.vue'

const emit = defineEmits<{
	(e: 'createTask', title: string): Promise<ITask>
}>()

const newTaskFieldActive = ref(false)
const newTaskTitleField = ref()
const newTaskTitle = ref('')

function showCreateTaskOrCreate() {
	if (!newTaskFieldActive.value) {
		// Timeout to not send the form if the field isn't even shown
		setTimeout(() => {
			newTaskFieldActive.value = true
			nextTick(() => newTaskTitleField.value.focus())
		}, 100)
	} else {
		createTask()
	}
}

function hideCreateNewTask() {
	if (newTaskTitle.value === '') {
		nextTick(() => (newTaskFieldActive.value = false))
	}
}

async function createTask() {
	if (!newTaskFieldActive.value) {
		return
	}
	await emit('createTask', newTaskTitle.value)
	newTaskTitle.value = ''
	hideCreateNewTask()
}
</script>

<style scoped lang="scss">
.add-new-task {
	padding: var(--wm-space-4) var(--wm-space-3) var(--wm-space-2);
	display: flex;
	max-inline-size: 450px;

	.input {
		margin-inline-end: var(--wm-space-3);
		font-size: var(--wm-text-sm);
	}

	.button {
		font-size: var(--wm-text-2xs);
	}
}
</style>
