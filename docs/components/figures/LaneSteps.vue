<script setup lang="ts">
// 「手順が進むにつれて、どのファイル(レーン)に何をするか」を表す図。
// 上段にステップのカード列、下段にレーンを引き、ステップがレーンに触る位置へ
// アクション付きのマーカーを落とす。
export interface LaneStep {
  label: string;
  note?: string;
  lane?: number; // 触るレーンの index。省略時はどこにも触らない
  action?: string; // レーン上のマーカーに書く動詞(追記、書換など)
  accent?: "brand" | "danger";
  dim?: boolean;
}

const props = defineProps<{ lanes: string[]; steps: LaneStep[] }>();

const gridStyle = {
  gridTemplateColumns: `max-content repeat(${props.steps.length}, minmax(110px, 1fr))`,
};
</script>

<template>
  <div class="ls" :style="gridStyle">
    <div class="ls-corner"></div>
    <div
      v-for="(s, i) in steps"
      :key="`h${i}`"
      class="ls-step"
      :class="[s.accent, { dim: s.dim, last: i === steps.length - 1 }]"
    >
      <span class="ls-chip" :class="s.accent">{{ i + 1 }}</span>
      <span class="ls-label">{{ s.label }}</span>
      <span v-if="s.note" class="ls-note">{{ s.note }}</span>
    </div>

    <template v-for="(lane, li) in lanes" :key="`l${li}`">
      <div class="ls-lane-label">{{ lane }}</div>
      <div v-for="(s, i) in steps" :key="`c${li}-${i}`" class="ls-cell">
        <span v-if="s.lane === li" class="ls-marker" :class="[s.accent, { dim: s.dim }]">
          {{ s.action ?? "書" }}
        </span>
      </div>
    </template>
  </div>
</template>

<style scoped>
.ls {
  display: grid;
  row-gap: 0;
  column-gap: 12px;
  align-items: stretch;
  min-width: 560px;
}
.ls-step {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 3px;
  padding: 10px 12px 12px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  background-color: var(--vp-c-bg);
  margin-bottom: 14px;
}
.ls-step.brand {
  border-color: var(--vp-c-brand-1);
}
.ls-step.danger {
  border-color: var(--vp-c-danger-1);
}
.ls-step.dim {
  opacity: 0.45;
}
.ls-step:not(.last)::after {
  content: "";
  position: absolute;
  top: 50%;
  right: -12px;
  width: 12px;
  height: 1px;
  background-color: var(--vp-c-text-3);
}
.ls-chip {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-1);
  font-size: 11px;
  font-weight: 700;
}
.ls-chip.brand {
  background-color: var(--vp-c-brand-1);
  color: var(--vp-c-bg);
}
.ls-chip.danger {
  background-color: var(--vp-c-danger-1);
  color: var(--vp-c-bg);
}
.ls-label {
  font-size: 13px;
  font-weight: 600;
  line-height: 1.3;
}
.ls-note {
  font-size: 11px;
  color: var(--vp-c-text-2);
  line-height: 1.4;
}
.ls-corner,
.ls-lane-label {
  display: flex;
  align-items: center;
  font-size: 12px;
  font-weight: 600;
  color: var(--vp-c-text-2);
  padding-right: 4px;
}
.ls-cell {
  position: relative;
  height: 40px;
}
.ls-cell::before {
  content: "";
  position: absolute;
  left: -12px;
  right: -12px;
  top: 50%;
  height: 2px;
  background-color: var(--vp-c-default-soft);
}
.ls-marker {
  position: absolute;
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  padding: 2px 10px;
  border-radius: 10px;
  background-color: var(--vp-c-text-2);
  color: var(--vp-c-bg);
  font-size: 11px;
  font-weight: 600;
  white-space: nowrap;
}
.ls-marker.brand {
  background-color: var(--vp-c-brand-1);
}
.ls-marker.danger {
  background-color: var(--vp-c-danger-1);
}
.ls-marker.dim {
  opacity: 0.45;
}
</style>
