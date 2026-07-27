<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// llm/gqa(Go)の KVHeadFor / KVCacheFloats をそのまま可視化する。
// 上段: Q ヘッドと K/V の対応表(共有グループ)。図は 8 ヘッドの模式。
// 下段: Llama 2 70B 相当(32ヘッド・head 128・32層・fp16)での KV キャッシュ実測値。

const modes = [
  { key: "mha", label: "MHA", qHeads: 8, kvHeads: 8, realKV: 32 },
  { key: "gqa", label: "GQA", qHeads: 8, kvHeads: 2, realKV: 8 },
  { key: "mqa", label: "MQA", qHeads: 8, kvHeads: 1, realKV: 1 },
] as const;
const mode = ref(0);

const SEQS = [1024, 2048, 4096, 8192, 16384, 32768];
const at = ref(0);

// 実測式: 2(K,V) × 系列長 × KVヘッド数 × ヘッド次元128 × 32層 × 2byte(fp16)
const HEAD_DIM = 128;
const LAYERS = 32;
const BYTES = 2;
function cacheBytes(seq: number, kvHeads: number): number {
  return 2 * seq * kvHeads * HEAD_DIM * LAYERS * BYTES;
}
const MAX_BYTES = cacheBytes(SEQS[SEQS.length - 1], 32);

function fmtBytes(b: number): string {
  const gib = b / 1024 ** 3;
  if (gib >= 1) return `${gib.toFixed(2)} GiB`;
  return `${(b / 1024 ** 2).toFixed(0)} MiB`;
}

const cur = computed(() => modes[mode.value]);
const seq = computed(() => SEQS[at.value]);

// 共有グループ: KV ヘッドごとに、それを引く Q ヘッドの一覧。
const groups = computed(() => {
  const g = cur.value.qHeads / cur.value.kvHeads;
  return Array.from({ length: cur.value.kvHeads }, (_, kv) => ({
    kv,
    qs: Array.from({ length: g }, (_, i) => kv * g + i),
  }));
});

const bars = computed(() =>
  modes.map((m) => {
    const b = cacheBytes(seq.value, m.realKV);
    return { key: m.key, label: m.label, kv: m.realKV, bytes: b, pct: (b / MAX_BYTES) * 100 };
  }),
);

const noteByMode = [
  "MHA: 全 Q ヘッドが自分専用の K/V を持つ。表現力は基準だが、キャッシュもヘッド数ぶんまるごと",
  "GQA: 4 つの Q ヘッドが 1 組の K/V を共有する。Q の多様性(何を探すか)は全ヘッドぶん残し、K/V だけ減らす折衷。Llama 2 70B 以降の主流",
  "MQA: 全 Q ヘッドが 1 組の K/V を共有する。削減は最大だが、K/V の多様性が消えて品質低下が出やすい",
];
const note = computed(() => {
  const m = noteByMode[mode.value];
  const mhaB = cacheBytes(seq.value, 32);
  const curB = cacheBytes(seq.value, cur.value.realKV);
  const ratio = mhaB / curB;
  const extra =
    mode.value === 0
      ? `系列長 ${seq.value.toLocaleString()} で ${fmtBytes(mhaB)}`
      : `系列長 ${seq.value.toLocaleString()} で ${fmtBytes(curB)}(MHA の 1/${ratio})`;
  return `${m}。${extra}`;
});

const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < SEQS.length - 1);
function first() { at.value = 0; }
function prev() { if (canPrev.value) at.value--; }
function next() { if (canNext.value) at.value++; }
function last() { at.value = SEQS.length - 1; }

const badge = computed(() => `${cur.value.label} · ${seq.value.toLocaleString()} tok`);
</script>

<template>
  <DemoShell title="attention変種(KVキャッシュ)" badge-tone="neutral" :badge="badge">
    <div class="sd-controls">
      <span class="sd-seg">
        <span
          v-for="(m, i) in modes"
          :key="m.key"
          class="sd-seg-opt"
          :class="{ on: mode === i }"
          @click="mode = i"
          >{{ m.label }}</span
        >
      </span>
      <span class="spacer" />
      <span class="av-cfg mono">図は 8Q の模式 / 実測は 32Q・128次元・32層・fp16</span>
    </div>

    <div class="av-map">
      <div class="av-map-head">Q ヘッドと K/V の対応({{ cur.qHeads }}Q : {{ cur.kvHeads }}KV)</div>
      <div class="av-groups">
        <div v-for="g in groups" :key="g.kv" class="av-group">
          <div class="av-qrow">
            <span v-for="q in g.qs" :key="q" class="av-chip q mono">Q{{ q }}</span>
          </div>
          <div class="av-share mono">↓ 共有</div>
          <span class="av-chip kv mono">KV{{ g.kv }}</span>
        </div>
      </div>
    </div>

    <div class="av-bars">
      <div class="av-bars-head">
        KV キャッシュ(1 リクエストあたり)
        <span class="mono av-seq">系列長 {{ seq.toLocaleString() }}</span>
      </div>
      <div v-for="b in bars" :key="b.key" class="av-bar-row" :class="{ on: b.key === cur.key }">
        <span class="av-bar-label mono">{{ b.label }} ({{ b.kv }}KV)</span>
        <span class="av-bar-track"><span class="av-bar-fill" :style="{ width: Math.max(b.pct, 0.5) + '%' }"></span></span>
        <span class="av-bar-val mono">{{ fmtBytes(b.bytes) }}</span>
      </div>
    </div>

    <p class="av-note">{{ note }}</p>

    <div class="av-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="av-nav mono">{{ at + 1 }} / {{ SEQS.length }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">系列長を2倍</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="av-legend">
      キャッシュの式 2 × 系列長 × KVヘッド数 × ヘッド次元 × 層数 に Q ヘッド数は現れない。
      だから K/V の共有だけでメモリが 1/4(GQA)や 1/32(MQA)に縮む。
      attention の計算そのものは 3 方式で同じで、違いは対応表だけ。
    </p>
  </DemoShell>
</template>

<style scoped>
.av-cfg {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.av-map {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.av-map-head,
.av-bars-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  padding: 7px 12px;
  border-bottom: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.av-groups {
  padding: 12px;
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  justify-content: center;
}
.av-group {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  border: 1px dashed var(--vp-c-divider);
  border-radius: 3px;
  padding: 8px;
}
.av-qrow {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
  justify-content: center;
}
.av-chip {
  font-size: 11px;
  padding: 2px 7px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  background-color: var(--vp-c-bg-soft);
  color: var(--vp-c-text-2);
}
.av-chip.kv {
  border-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-brand-soft);
  color: var(--vp-c-brand-1);
  font-weight: 700;
}
.av-share {
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.av-bars {
  margin-top: 12px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.av-seq {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.av-bar-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 12px;
  border-left: 3px solid transparent;
  border-radius: 0;
}
.av-bar-row.on {
  border-left-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-bg-soft);
}
.av-bar-label {
  width: 110px;
  font-size: 11.5px;
  color: var(--vp-c-text-2);
}
.av-bar-track {
  flex: 1;
  height: 12px;
  background-color: var(--vp-c-bg-soft);
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.av-bar-fill {
  display: block;
  height: 100%;
  background-color: var(--vp-c-brand-1);
}
.av-bar-val {
  width: 76px;
  text-align: right;
  font-size: 11.5px;
  color: var(--vp-c-text-1);
}
.av-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.av-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
  flex-wrap: wrap;
}
.av-nav {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.av-legend {
  margin: 14px 0 0;
  padding-top: 12px;
  border-top: 1px solid var(--vp-c-divider);
  font-size: 12px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
}
.mono {
  font-family: var(--vp-font-family-mono);
}
</style>
