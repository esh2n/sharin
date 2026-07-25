<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// mini-GPT のパラメータ数を config から計算し、実物のモデルとスケール比較する。
const dModel = ref(64);
const nLayers = ref(4);
const vocab = ref(1000);
const dFF = computed(() => dModel.value * 4); // 慣例: FFN は 4×dModel

// パラメータ数(概算): 埋め込み + 層×(attention 4·d² + FFN 2·d·dff)
const params = computed(() => {
  const d = dModel.value;
  const perLayer = 4 * d * d + 2 * d * dFF.value;
  return vocab.value * d + nLayers.value * perLayer;
});

// 実物との比較(パラメータ数)。
const REFS = [
  { name: "あなたの mini-GPT", get: () => params.value, tone: "you" as const },
  { name: "GPT-2 (2019)", n: 1.5e9, tone: "ref" as const },
  { name: "GPT-3 (2020)", n: 175e9, tone: "ref" as const },
  { name: "GPT-4 級 (推定)", n: 1.5e12, tone: "ref" as const },
];

function fmt(n: number): string {
  if (n >= 1e12) return (n / 1e12).toFixed(1) + "兆";
  if (n >= 1e8) return (n / 1e8).toFixed(1) + "億";
  if (n >= 1e4) return (n / 1e4).toFixed(1) + "万";
  return Math.round(n).toLocaleString();
}

const rows = computed(() =>
  REFS.map((r) => {
    const n = "get" in r && r.get ? r.get() : (r as { n: number }).n;
    // 対数スケールでバーの長さを決める(1兆を100%に)
    const frac = Math.max(0.01, Math.log10(n) / Math.log10(1.5e12));
    return { name: r.name, n, frac, tone: r.tone };
  }),
);

const ratio = computed(() => Math.round(175e9 / params.value));
</script>

<template>
  <DemoShell title="GPT のパラメータ数とスケール" :badge="`${fmt(params)} パラメータ`" badge-tone="neutral">
    <div class="gs-controls">
      <label class="gs-slider">
        次元 dModel: <strong>{{ dModel }}</strong>
        <input v-model.number="dModel" type="range" min="8" max="512" step="8" />
      </label>
      <label class="gs-slider">
        層数: <strong>{{ nLayers }}</strong>
        <input v-model.number="nLayers" type="range" min="1" max="48" step="1" />
      </label>
      <label class="gs-slider">
        語彙: <strong>{{ vocab }}</strong>
        <input v-model.number="vocab" type="range" min="100" max="50000" step="100" />
      </label>
    </div>

    <div class="gs-bars">
      <div v-for="r in rows" :key="r.name" class="gs-row">
        <span class="gs-name" :class="r.tone">{{ r.name }}</span>
        <span class="gs-track">
          <span class="gs-bar" :class="r.tone" :style="{ width: `${r.frac * 100}%` }" />
        </span>
        <span class="gs-val">{{ fmt(r.n) }}</span>
      </div>
    </div>

    <p class="gs-note">
      バーは対数スケール(1桁ごと)。あなたの mini-GPT は <b>{{ fmt(params) }}</b> パラメータ、
      GPT-3 はその <b>約 {{ ratio.toLocaleString() }} 倍</b>。
      でも中身の Transformer は同じ——違いは規模と学習データ。
    </p>
  </DemoShell>
</template>

<style scoped>
.gs-controls {
  display: flex;
  gap: 20px;
  flex-wrap: wrap;
}
.gs-slider {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 13px;
  min-width: 160px;
}
.gs-slider input {
  accent-color: var(--vp-c-brand-1);
}
.gs-bars {
  margin-top: 16px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.gs-row {
  display: grid;
  grid-template-columns: 12em 1fr 5em;
  align-items: center;
  gap: 10px;
  font-size: 13px;
}
.gs-name.you {
  color: var(--vp-c-brand-1);
  font-weight: 700;
}
.gs-name.ref {
  color: var(--vp-c-text-2);
}
.gs-track {
  height: 16px;
  border-radius: 3px;
  background-color: var(--vp-c-default-soft);
  overflow: hidden;
}
.gs-bar {
  display: block;
  height: 100%;
  transition: width 0.2s;
}
.gs-bar.you {
  background-color: var(--vp-c-brand-1);
}
.gs-bar.ref {
  background-color: var(--vp-c-text-3);
}
.gs-val {
  font-variant-numeric: tabular-nums;
  text-align: right;
  color: var(--vp-c-text-2);
}
.gs-note {
  margin: 14px 0 0;
  font-size: 12px;
  color: var(--vp-c-text-2);
}
</style>
