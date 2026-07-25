<script setup lang="ts">
defineProps<{ caption?: string }>();
</script>

<template>
  <figure class="fig">
    <div class="fig-body">
      <slot />
    </div>
    <figcaption v-if="caption" class="fig-caption">{{ caption }}</figcaption>
  </figure>
</template>

<style scoped>
.fig {
  margin: 16px 0 24px;
  padding: 16px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  background-color: var(--vp-c-bg-soft);
}
.fig-body {
  overflow-x: auto;
  padding: 4px 0;
}
.fig-caption {
  margin-top: 10px;
  font-size: 13px;
  color: var(--vp-c-text-2);
  text-align: center;
}
</style>
