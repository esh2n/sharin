<script setup lang="ts">
// ディスク上のページ列を表す汎用図。cell.state で強調/淡色、cell.note で下部注記。
export interface PageCell {
  label: string;
  state?: "hot" | "dim";
  note?: string;
}

defineProps<{ cells: PageCell[] }>();
</script>

<template>
  <div class="pr-row">
    <div v-for="(c, i) in cells" :key="i" class="pr-cell" :class="c.state">
      <span class="pr-label">{{ c.label }}</span>
      <span v-if="c.note" class="pr-note">{{ c.note }}</span>
    </div>
  </div>
</template>

<style scoped>
.pr-row {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}
.pr-cell {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  min-width: 56px;
  padding: 10px 6px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 6px;
  background-color: var(--vp-c-bg);
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
}
.pr-cell.hot {
  border-color: var(--vp-c-brand-1);
  box-shadow: 0 0 0 1px var(--vp-c-brand-1);
}
.pr-cell.dim {
  opacity: 0.35;
}
.pr-note {
  font-size: 10px;
  color: var(--vp-c-text-2);
  font-family: var(--vp-font-family-base);
}
</style>
