<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// self-attention の重み行列を可視化。「どのトークンがどのトークンに注目するか」。
// 重みは固定の例(実物は Q·Kᵀ で計算)。因果マスクの ON/OFF を見せる。
const TOKENS = ["The", "cat", "sat", "on", "mat"];

// 素の注目スコア(対称でない例)。行 i = トークン i が各トークンをどれだけ見るか。
const rawScores = [
  [3, 1, 0, 0, 1],
  [1, 3, 1, 0, 0],
  [0, 2, 3, 1, 0],
  [0, 0, 1, 3, 1],
  [1, 0, 0, 1, 3],
];

const causal = ref(true);

function softmax(row: number[]): number[] {
  const max = Math.max(...row.filter((v) => Number.isFinite(v)));
  const exps = row.map((v) => (Number.isFinite(v) ? Math.exp(v - max) : 0));
  const sum = exps.reduce((a, b) => a + b, 0);
  return exps.map((e) => e / sum);
}

const weights = computed(() => {
  return rawScores.map((row, i) => {
    const masked = row.map((v, j) => (causal.value && j > i ? Number.NEGATIVE_INFINITY : v));
    return softmax(masked);
  });
});

const sel = ref(2); // 注目しているクエリ行
</script>

<template>
  <DemoShell title="self-attention の重み" :badge="causal ? '因果マスクON' : 'マスクなし'" :badge-tone="causal ? 'ok' : 'neutral'">
    <div class="sd-controls">
      <div class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: causal }" @click="causal = true">因果マスク(GPT)</span>
        <span class="sd-seg-opt" :class="{ on: !causal }" @click="causal = false">マスクなし</span>
      </div>
      <span class="spacer"></span>
      <span class="at-hint">行 = 見る側、列 = 見られる側。濃いほど強く注目</span>
    </div>

    <div class="at-matrix">
      <div class="at-row header">
        <span class="at-corner"></span>
        <span v-for="(t, j) in TOKENS" :key="j" class="at-collabel">{{ t }}</span>
      </div>
      <div v-for="(row, i) in weights" :key="i" class="at-row" :class="{ active: sel === i }" @mouseenter="sel = i">
        <span class="at-rowlabel">{{ TOKENS[i] }}</span>
        <span
          v-for="(w, j) in row"
          :key="j"
          class="at-cell"
          :class="{ zero: w < 0.001 }"
          :style="{ backgroundColor: `color-mix(in srgb, var(--vp-c-brand-1) ${Math.round(w * 100)}%, transparent)` }"
          :title="`${TOKENS[i]} → ${TOKENS[j]}: ${(w * 100).toFixed(0)}%`"
        >{{ w < 0.001 ? "" : Math.round(w * 100) }}</span>
      </div>
    </div>

    <p class="at-explain">
      「<b>{{ TOKENS[sel] }}</b>」は
      <template v-for="(w, j) in weights[sel]" :key="j">
        <span v-if="w >= 0.1">{{ j > 0 ? "、" : "" }}{{ TOKENS[j] }}に{{ Math.round(w * 100) }}%</span>
      </template>
      注目している{{ causal ? "(未来のトークンは見えない)" : "" }}。
    </p>
  </DemoShell>
</template>

<style scoped>
.at-hint {
  font-size: 12px;
  color: var(--vp-c-text-3);
}
.at-matrix {
  margin-top: 14px;
  display: inline-flex;
  flex-direction: column;
  gap: 3px;
}
.at-row {
  display: flex;
  gap: 3px;
  align-items: center;
}
.at-row.active .at-rowlabel {
  color: var(--vp-c-brand-1);
  font-weight: 700;
}
.at-corner,
.at-rowlabel,
.at-collabel {
  width: 42px;
  font-size: 12px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-2);
  text-align: center;
}
.at-rowlabel {
  text-align: right;
  padding-right: 4px;
}
.at-cell {
  width: 42px;
  height: 30px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 4px;
  border: 1px solid var(--vp-c-divider);
  font-family: var(--vp-font-family-mono);
  font-size: 11px;
  color: var(--vp-c-text-1);
}
.at-cell.zero {
  background-color: var(--vp-c-bg-alt) !important;
  border-style: dashed;
  opacity: 0.5;
}
.at-explain {
  margin: 14px 0 0;
  font-size: 13px;
  color: var(--vp-c-text-2);
}
</style>
