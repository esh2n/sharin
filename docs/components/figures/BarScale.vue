<script setup lang="ts">
// 量の比較を横バーで表す汎用図。frac は 0..1 の相対長。
export interface Bar {
  label: string;
  text: string;
  frac: number;
  tone?: "good" | "bad";
}

defineProps<{ bars: Bar[] }>();
</script>

<template>
  <div class="bs-list">
    <div v-for="b in bars" :key="b.label" class="bs-row">
      <span class="bs-label">{{ b.label }}</span>
      <span class="bs-track">
        <span class="bs-bar" :class="b.tone" :style="{ width: `${Math.max(b.frac * 100, 0.8)}%` }" />
      </span>
      <span class="bs-text">{{ b.text }}</span>
    </div>
  </div>
</template>

<style scoped>
.bs-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 480px;
}
.bs-row {
  display: grid;
  grid-template-columns: 11em 1fr 9em;
  align-items: center;
  gap: 10px;
  font-size: 13px;
}
.bs-track {
  height: 14px;
  border-radius: 3px;
  background-color: var(--vp-c-default-soft);
  overflow: hidden;
}
.bs-bar {
  display: block;
  height: 100%;
  background-color: var(--vp-c-brand-1);
}
.bs-bar.good {
  background-color: var(--vp-c-green-1);
}
.bs-bar.bad {
  background-color: var(--vp-c-danger-1);
}
.bs-text {
  font-variant-numeric: tabular-nums;
  color: var(--vp-c-text-2);
}
</style>
