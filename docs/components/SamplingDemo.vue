<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// 「明日の天気は◯◯」の次トークンを予測している、という体の手作り logits。
const VOCAB = [
  { token: "晴れ", logit: 4.0 },
  { token: "曇り", logit: 3.3 },
  { token: "雨", logit: 2.9 },
  { token: "雪", logit: 1.2 },
  { token: "台風", logit: 0.4 },
  { token: "虹", logit: -0.3 },
  { token: "猫", logit: -1.0 },
  { token: "無", logit: -2.0 },
] as const;

const props = withDefaults(
  defineProps<{ controls?: Array<"temperature" | "topk" | "topp" | "minp"> }>(),
  { controls: () => ["temperature"] },
);

const temperature = ref(1.0);
const topK = ref(VOCAB.length);
const topP = ref(1.0);
const minP = ref(0.0);
const picks = ref<Record<string, number>>({});

function has(c: string) {
  return props.controls.includes(c as never);
}

// Go 版 sampling パッケージと同じパイプラインの JS ミラー。
function softmax(logits: number[]): number[] {
  const max = Math.max(...logits.filter((l) => Number.isFinite(l)));
  const exps = logits.map((l) => (Number.isFinite(l) ? Math.exp(l - max) : 0));
  const sum = exps.reduce((a, b) => a + b, 0);
  return exps.map((e) => e / sum);
}

const probs = computed(() => {
  let logits = VOCAB.map((v) => v.logit / (has("temperature") ? temperature.value : 1));

  if (has("topk") && topK.value < VOCAB.length) {
    const order = [...logits.keys()].sort((a, b) => logits[b] - logits[a]);
    const cut = new Set(order.slice(topK.value));
    logits = logits.map((l, i) => (cut.has(i) ? Number.NEGATIVE_INFINITY : l));
  }
  if (has("topp") && topP.value < 1) {
    const p = softmax(logits);
    const order = [...p.keys()].sort((a, b) => p[b] - p[a]);
    const keep = new Set<number>();
    let cum = 0;
    for (const idx of order) {
      keep.add(idx);
      cum += p[idx];
      if (cum >= topP.value) break;
    }
    logits = logits.map((l, i) => (keep.has(i) ? l : Number.NEGATIVE_INFINITY));
  }
  if (has("minp") && minP.value > 0) {
    const p = softmax(logits);
    const threshold = minP.value * Math.max(...p);
    logits = logits.map((l, i) => (p[i] >= threshold ? l : Number.NEGATIVE_INFINITY));
  }
  return softmax(logits);
});

function sampleOnce() {
  const r = Math.random();
  let cum = 0;
  let idx = probs.value.length - 1;
  for (let i = 0; i < probs.value.length; i++) {
    cum += probs.value[i];
    if (r < cum) {
      idx = i;
      break;
    }
  }
  const token = VOCAB[idx].token;
  picks.value = { ...picks.value, [token]: (picks.value[token] ?? 0) + 1 };
}

function reset() {
  picks.value = {};
}
</script>

<template>
  <DemoShell title="次トークンのサンプリング">
    <p class="sp-context">「明日の天気は<span class="sp-blank">◯◯</span>」の次に来るトークンの確率分布</p>

    <div class="sp-controls">
      <label v-if="has('temperature')" class="sp-control">
        <span>temperature: {{ temperature.toFixed(2) }}</span>
        <input v-model.number="temperature" type="range" min="0.1" max="3" step="0.05" @input="reset" />
      </label>
      <label v-if="has('topk')" class="sp-control">
        <span>top-k: {{ topK }}</span>
        <input v-model.number="topK" type="range" min="1" :max="VOCAB.length" step="1" @input="reset" />
      </label>
      <label v-if="has('topp')" class="sp-control">
        <span>top-p: {{ topP.toFixed(2) }}</span>
        <input v-model.number="topP" type="range" min="0.05" max="1" step="0.05" @input="reset" />
      </label>
      <label v-if="has('minp')" class="sp-control">
        <span>min-p: {{ minP.toFixed(2) }}</span>
        <input v-model.number="minP" type="range" min="0" max="1" step="0.05" @input="reset" />
      </label>
    </div>

    <div class="sp-bars">
      <div v-for="(v, i) in VOCAB" :key="v.token" class="sp-row" :class="{ cut: probs[i] === 0 }">
        <span class="sp-token">{{ v.token }}</span>
        <span class="sp-track">
          <span class="sp-bar" :style="{ width: `${probs[i] * 100}%` }" />
        </span>
        <span class="sp-pct">{{ (probs[i] * 100).toFixed(1) }}%</span>
        <span class="sp-count">{{ picks[v.token] ? `×${picks[v.token]}` : "" }}</span>
      </div>
    </div>

    <div class="sd-controls sp-actions">
      <button class="sd-btn sd-btn--primary" type="button" @click="sampleOnce">1トークン抽選する</button>
      <button class="sd-btn" type="button" @click="reset">集計をクリア</button>
    </div>
  </DemoShell>
</template>

<style scoped>
.sp-context {
  margin: 0 0 12px;
  font-size: 14px;
  color: var(--vp-c-text-2);
}
.sp-blank {
  font-weight: 600;
  color: var(--vp-c-brand-1);
}
.sp-controls {
  display: flex;
  gap: 24px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.sp-control {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 13px;
  min-width: 180px;
}
.sp-control input {
  accent-color: var(--vp-c-brand-1);
}
.sp-row {
  display: grid;
  grid-template-columns: 3.5em 1fr 4em 3em;
  align-items: center;
  gap: 8px;
  padding: 2px 0;
  font-size: 13px;
}
.sp-row.cut {
  opacity: 0.35;
}
.sp-token {
  text-align: right;
}
.sp-track {
  height: 14px;
  border-radius: 3px;
  background-color: var(--vp-c-default-soft);
  overflow: hidden;
}
.sp-bar {
  display: block;
  height: 100%;
  background-color: var(--vp-c-brand-1);
  transition: width 0.15s ease-out;
}
.sp-pct {
  font-variant-numeric: tabular-nums;
  text-align: right;
}
.sp-count {
  font-variant-numeric: tabular-nums;
  color: var(--vp-c-text-2);
}
.sp-actions {
  margin-top: 14px;
}
</style>
