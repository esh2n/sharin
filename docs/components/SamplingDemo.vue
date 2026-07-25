<script setup lang="ts">
import { computed, ref } from "vue";

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
  <div class="sp-demo">
    <p class="sp-context">「明日の天気は<span class="sp-blank">◯◯</span>」の次トークン分布</p>

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

    <div class="sp-actions">
      <button class="sp-fire" type="button" @click="sampleOnce">1トークン抽選する</button>
      <button class="sp-clear" type="button" @click="reset">集計をクリア</button>
    </div>
  </div>
</template>

<style scoped>
.sp-demo {
  margin: 16px 0 24px;
  padding: 16px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  background-color: var(--vp-c-bg-soft);
}
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
  display: flex;
  gap: 12px;
  margin-top: 12px;
}
.sp-fire {
  padding: 6px 16px;
  border-radius: 6px;
  font-weight: 600;
  font-size: 13px;
  color: var(--vp-button-brand-text);
  background-color: var(--vp-button-brand-bg);
}
.sp-fire:hover {
  background-color: var(--vp-button-brand-hover-bg);
}
.sp-clear {
  padding: 6px 16px;
  border-radius: 6px;
  font-size: 13px;
  color: var(--vp-c-text-1);
  background-color: var(--vp-c-default-soft);
}
</style>
