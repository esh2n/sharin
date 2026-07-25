<script setup lang="ts">
// 処理の流れを「箱 → 箱 → 箱」で表す汎用図。state: 'hot' で主役の箱を強調する。
export interface FlowStep {
  label: string;
  note?: string;
  state?: "hot" | "dim";
}

defineProps<{ steps: FlowStep[] }>();
</script>

<template>
  <div class="fr-row">
    <template v-for="(s, i) in steps" :key="i">
      <span v-if="i > 0" class="fr-arrow">→</span>
      <div class="fr-box" :class="s.state">
        <span class="fr-label">{{ s.label }}</span>
        <span v-if="s.note" class="fr-note">{{ s.note }}</span>
      </div>
    </template>
  </div>
</template>

<style scoped>
.fr-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.fr-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  min-width: 96px;
  padding: 10px 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  background-color: var(--vp-c-bg);
  font-size: 13px;
}
.fr-box.hot {
  border-color: var(--vp-c-brand-1);
  box-shadow: 0 0 0 1px var(--vp-c-brand-1);
}
.fr-box.dim {
  opacity: 0.4;
}
.fr-label {
  font-weight: 600;
}
.fr-note {
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.fr-arrow {
  color: var(--vp-c-text-3);
}
</style>
