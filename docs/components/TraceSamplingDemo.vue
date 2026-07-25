<script setup lang="ts">
import { computed, ref } from "vue";

// Go 版 trace-sampling と同じロジックの JS ミラーによるシミュレーション。
const TOTAL = 5000;

const headRate = ref(0.1); // head の確率 & tail のベースレート(比較を公平にするため共通)
const errRate = ref(0.01);
const seed = ref(1);

// mulberry32: シード付き擬似乱数。スライダーを動かしても結果が暴れないよう固定シードで回す。
function mulberry32(a: number) {
  return () => {
    a |= 0;
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) | 0;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const result = computed(() => {
  const rng = mulberry32(seed.value);
  let errors = 0;
  let headKept = 0;
  let headErrKept = 0;
  let tailKept = 0;
  let tailErrKept = 0;

  for (let i = 0; i < TOTAL; i++) {
    const isErr = rng() < errRate.value;
    const slow = rng() < 0.02; // 2% は遅いトレース
    if (isErr) errors++;

    // head: 開始時に決める。isErr はまだ「存在しない」情報なので使えない
    if (rng() < headRate.value) {
      headKept++;
      if (isErr) headErrKept++;
    }
    // tail: 完結後に中身を見る。エラーと遅いものは必ず、他はベースレートで
    if (isErr || slow || rng() < headRate.value) {
      tailKept++;
      if (isErr) tailErrKept++;
    }
  }
  return { errors, headKept, headErrKept, tailKept, tailErrKept };
});

const rows = computed(() => [
  {
    name: "head-based",
    kept: result.value.headKept,
    capture: result.value.errors ? result.value.headErrKept / result.value.errors : 0,
    errKept: result.value.headErrKept,
  },
  {
    name: "tail-based",
    kept: result.value.tailKept,
    capture: result.value.errors ? result.value.tailErrKept / result.value.errors : 0,
    errKept: result.value.tailErrKept,
  },
]);

function reroll() {
  seed.value = (seed.value * 48271) % 2147483647;
}
</script>

<template>
  <div class="ts-demo">
    <p class="ts-context">
      {{ TOTAL.toLocaleString() }} 件のトレース(うちエラー {{ result.errors }} 件)に両方式を適用
    </p>

    <div class="ts-controls">
      <label class="ts-control">
        <span>サンプリング率: {{ (headRate * 100).toFixed(0) }}%</span>
        <input v-model.number="headRate" type="range" min="0.01" max="1" step="0.01" />
      </label>
      <label class="ts-control">
        <span>エラー率: {{ (errRate * 100).toFixed(1) }}%</span>
        <input v-model.number="errRate" type="range" min="0.001" max="0.05" step="0.001" />
      </label>
      <button class="ts-reroll" type="button" @click="reroll">乱数を引き直す</button>
    </div>

    <div v-for="r in rows" :key="r.name" class="ts-block">
      <p class="ts-name">{{ r.name }}</p>
      <div class="ts-row">
        <span class="ts-label">保存量</span>
        <span class="ts-track">
          <span class="ts-bar cost" :style="{ width: `${(r.kept / TOTAL) * 100}%` }" />
        </span>
        <span class="ts-value">{{ r.kept.toLocaleString() }} 件 ({{ ((r.kept / TOTAL) * 100).toFixed(1) }}%)</span>
      </div>
      <div class="ts-row">
        <span class="ts-label">エラー捕捉率</span>
        <span class="ts-track">
          <span class="ts-bar" :class="r.capture >= 0.999 ? 'good' : 'bad'" :style="{ width: `${r.capture * 100}%` }" />
        </span>
        <span class="ts-value">{{ (r.capture * 100).toFixed(1) }}% ({{ r.errKept }}/{{ result.errors }})</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.ts-demo {
  margin: 16px 0 24px;
  padding: 16px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  background-color: var(--vp-c-bg-soft);
}
.ts-context {
  margin: 0 0 12px;
  font-size: 14px;
  color: var(--vp-c-text-2);
}
.ts-controls {
  display: flex;
  gap: 24px;
  flex-wrap: wrap;
  align-items: flex-end;
  margin-bottom: 8px;
}
.ts-control {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 13px;
  min-width: 180px;
}
.ts-control input {
  accent-color: var(--vp-c-brand-1);
}
.ts-reroll {
  padding: 6px 16px;
  border-radius: 6px;
  font-size: 13px;
  color: var(--vp-c-text-1);
  background-color: var(--vp-c-default-soft);
}
.ts-block {
  margin-top: 10px;
}
.ts-name {
  margin: 0 0 4px;
  font-size: 13px;
  font-weight: 600;
}
.ts-row {
  display: grid;
  grid-template-columns: 7em 1fr 11em;
  align-items: center;
  gap: 8px;
  padding: 2px 0;
  font-size: 13px;
}
.ts-track {
  height: 14px;
  border-radius: 3px;
  background-color: var(--vp-c-default-soft);
  overflow: hidden;
}
.ts-bar {
  display: block;
  height: 100%;
  transition: width 0.15s ease-out;
}
.ts-bar.cost {
  background-color: var(--vp-c-brand-1);
}
.ts-bar.good {
  background-color: var(--vp-c-green-1);
}
.ts-bar.bad {
  background-color: var(--vp-c-danger-1);
}
.ts-value {
  font-variant-numeric: tabular-nums;
  color: var(--vp-c-text-2);
}
</style>
