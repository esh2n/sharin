<script setup lang="ts">
import { computed } from "vue";

// ビット配置を色分きの帯で表す汎用図。帯の幅はビット数に比例する。
// 色分けは IdGenDemo と同じ意味(時刻/ノード/連番/乱数/メタ)。
export interface BitSegment {
  label: string;
  bits: number;
  cls: "time" | "node" | "seq" | "rand" | "meta";
}

export interface BitRow {
  name: string;
  segments: BitSegment[];
}

const props = defineProps<{ rows: BitRow[] }>();

const maxBits = computed(() =>
  Math.max(...props.rows.map((r) => r.segments.reduce((a, s) => a + s.bits, 0))),
);
</script>

<template>
  <div class="bf-list">
    <div v-for="r in rows" :key="r.name" class="bf-row">
      <span class="bf-name">{{ r.name }}</span>
      <div class="bf-band">
        <span
          v-for="(s, i) in r.segments"
          :key="i"
          class="bf-seg"
          :class="s.cls"
          :style="{ width: `${(s.bits / maxBits) * 100}%` }"
          :title="`${s.label} (${s.bits}bit)`"
        >
          {{ s.bits >= 10 ? s.label : "" }}
        </span>
      </div>
    </div>
    <p class="bf-legend">
      <span class="bf-chip time">時刻</span>
      <span class="bf-chip node">ノードID</span>
      <span class="bf-chip seq">連番</span>
      <span class="bf-chip rand">乱数</span>
      <span class="bf-chip meta">version/variant</span>
    </p>
  </div>
</template>

<style scoped>
.bf-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 520px;
}
.bf-row {
  display: grid;
  grid-template-columns: 7em 1fr;
  align-items: center;
  gap: 10px;
}
.bf-name {
  font-size: 13px;
  font-weight: 600;
  text-align: right;
}
.bf-band {
  display: flex;
  height: 26px;
  border-radius: 4px;
  overflow: hidden;
}
.bf-seg {
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  color: var(--vp-c-bg);
  white-space: nowrap;
  overflow: hidden;
}
.bf-seg.time { background-color: var(--vp-c-brand-1); }
.bf-seg.node { background-color: var(--vp-c-green-1); }
.bf-seg.seq { background-color: var(--vp-c-yellow-1); }
.bf-seg.rand { background-color: var(--vp-c-text-3); }
.bf-seg.meta { background-color: var(--vp-c-purple-1); }
.bf-legend {
  margin: 4px 0 0;
  display: flex;
  gap: 10px;
  font-size: 12px;
}
.bf-chip {
  padding: 1px 8px;
  border-radius: 4px;
  color: var(--vp-c-bg);
}
.bf-chip.time { background-color: var(--vp-c-brand-1); }
.bf-chip.node { background-color: var(--vp-c-green-1); }
.bf-chip.seq { background-color: var(--vp-c-yellow-1); }
.bf-chip.rand { background-color: var(--vp-c-text-3); }
.bf-chip.meta { background-color: var(--vp-c-purple-1); }
</style>
