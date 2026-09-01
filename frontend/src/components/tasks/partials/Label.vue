<script setup lang="ts">
import {computed} from 'vue'

import type {Label} from '@/client/generated'
import {getLabelColor} from '@/composables/useLabelStyles'

const props = defineProps<{
	label: Label
}>()

// The chip draws the label color as a hairline plus a wash of itself rather
// than a solid fill, so the color is handed to the styles as a custom property
// instead of a background/foreground pair. See styles/components/labels.scss.
const labelStyles = computed(() => {
	const color = getLabelColor(props.label)
	return color === '' ? {} : {'--label-color': color}
})
</script>

<template>
	<span
		:key="label.id"
		:style="labelStyles"
		class="tag has-label-color"
	>
		<span>{{ label.title }}</span>
	</span>
</template>
