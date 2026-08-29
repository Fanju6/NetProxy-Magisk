<template>
  <svg :width="size" :height="size" viewBox="0 0 400 400" aria-hidden="true">
    <defs>
      <linearGradient :id="gradientId" x1="0%" y1="0%" x2="100%" y2="100%">
        <stop offset="0%" :stop-color="colors.gradStart" />
        <stop offset="35%" :stop-color="colors.gradStop2" />
        <stop offset="65%" :stop-color="colors.gradStop3" />
        <stop offset="100%" :stop-color="colors.gradEnd" />
      </linearGradient>
    </defs>
    <line x1="100" y1="80" x2="100" y2="320" :stroke="colors.leftColor" :stroke-width="strokeWidth" :stroke-linecap="strokeLinecap" />
    <line x1="300" y1="80" x2="300" y2="320" :stroke="colors.rightColor" :stroke-width="strokeWidth" :stroke-linecap="strokeLinecap" />
    <line x1="100" y1="80" x2="300" y2="320" :stroke="`url(#${gradientId})`" :stroke-width="strokeWidth" :stroke-linecap="strokeLinecap" :opacity="opacity" />
  </svg>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { LogoColors } from '../tools/color'
import type { StrokeLinecap } from '../tools/logo'

const props = withDefaults(defineProps<{
  colors: LogoColors
  uid: string
  size?: number | string
  strokeWidth?: number
  opacity?: number
  strokeLinecap?: StrokeLinecap
}>(), {
  size: '100%',
  strokeWidth: 88,
  opacity: 0.85,
  strokeLinecap: 'round'
})

const gradientId = computed(() => `netproxy-logo-${props.uid.replace(/[^a-z0-9_-]/gi, '-')}`)
</script>
