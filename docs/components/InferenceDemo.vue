<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// llm/kvcache(Go)の 2 つの仕組みをブラウザに移植。
// 「KVキャッシュ」: 射影回数が二次(無し)と線形(有り)で開いていく。
// 「speculative」: ドラフト γ=4 の下書きを本命が一括検証。出力は本命単独と常に一致。

// --- KV キャッシュ ---
const PROMPT_LEN = 3;
const GEN_STEPS = 10;
// step k: 無し = コンテキスト全体(3+k-1)を作り直し / 有り = 初回はプロンプト3、以降は新トークン1
const cacheFrames = Array.from({ length: GEN_STEPS + 1 }, (_, k) => {
  let no = 0, withC = 0;
  for (let s = 1; s <= k; s++) {
    no += PROMPT_LEN + s - 1;
    withC += s === 1 ? PROMPT_LEN : 1;
  }
  return { step: k, no, withC, stepNo: k === 0 ? 0 : PROMPT_LEN + k - 1, stepWith: k === 0 ? 0 : k === 1 ? PROMPT_LEN : 1 };
});
const CACHE_MAX = cacheFrames[GEN_STEPS].no;

// --- speculative ---
const VOCAB = 32;
const GAMMA = 4;
const SPEC_N = 12;
const PROMPT = [5, 9];

function targetFn(p: number[]): number {
  const last = p[p.length - 1];
  const prev = p.length >= 2 ? p[p.length - 2] : 0;
  return (last * 3 + prev + 1) % VOCAB;
}
const draftKinds = [
  { key: "good", label: "良いドラフト", fn: targetFn },
  { key: "half", label: "半分当たる", fn: (p: number[]) => targetFn(p) & ~1 },
  { key: "bad", label: "ほぼ外す", fn: (p: number[]) => (p[p.length - 1] + 7) % VOCAB },
];

interface Tok {
  v: number;
  kind: "prompt" | "accept" | "fix" | "bonus";
}
interface Round {
  props: { v: number; ok: boolean }[];
  seq: Tok[];
  passes: number;
  produced: number;
  note: string;
}

function simulate(draft: (p: number[]) => number): Round[] {
  const seq: Tok[] = PROMPT.map((v) => ({ v, kind: "prompt" as const }));
  const rounds: Round[] = [
    { props: [], seq: [...seq], passes: 0, produced: 0, note: "プロンプトから開始。ドラフトが 4 トークン先読みし、本命が 1 パスで検証する" },
  ];
  let passes = 0;
  let remaining = SPEC_N;
  while (remaining > 0) {
    const cur = seq.map((t) => t.v);
    const props: number[] = [];
    const work = [...cur];
    for (let i = 0; i < GAMMA; i++) {
      const t = draft(work);
      props.push(t);
      work.push(t);
    }
    passes++;
    const marked: { v: number; ok: boolean }[] = [];
    let accepted = 0;
    let fixed = false;
    let bonus = false;
    let all = true;
    for (let i = 0; i < props.length && remaining > 0; i++) {
      const want = targetFn(seq.map((t) => t.v));
      const ok = props[i] === want;
      marked.push({ v: props[i], ok });
      seq.push({ v: want, kind: ok ? "accept" : "fix" });
      remaining--;
      if (ok) accepted++;
      else {
        all = false;
        fixed = true;
        break;
      }
    }
    if (all && remaining > 0) {
      seq.push({ v: targetFn(seq.map((t) => t.v)), kind: "bonus" });
      remaining--;
      bonus = true;
    }
    const producedNow = accepted + (fixed ? 1 : 0) + (bonus ? 1 : 0);
    const parts = [`一致 ${accepted}/${marked.length}`];
    if (fixed) parts.push("不一致 1 を本命が訂正");
    if (bonus) parts.push("全一致ボーナス +1");
    rounds.push({
      props: marked,
      seq: [...seq],
      passes,
      produced: SPEC_N - remaining,
      note: `パス ${passes}: ${parts.join("、")} → このパスで ${producedNow} トークン進んだ`,
    });
  }
  return rounds;
}

const modes = [
  { key: "cache", label: "KVキャッシュ" },
  { key: "spec", label: "speculative" },
] as const;
const mode = ref<"cache" | "spec">("cache");
const draftPick = ref(0);
const at = ref(0);

const specFrames = computed(() => simulate(draftKinds[draftPick.value].fn));
const frameCount = computed(() => (mode.value === "cache" ? cacheFrames.length : specFrames.value.length));

function setMode(m: "cache" | "spec") {
  mode.value = m;
  at.value = 0;
}
function setDraft(i: number) {
  draftPick.value = i;
  at.value = 0;
}

const cf = computed(() => cacheFrames[at.value]);
const sf = computed(() => specFrames.value[at.value]);

const note = computed(() => {
  if (mode.value === "spec") return sf.value.note;
  if (cf.value.step === 0) return "プロンプト 3 トークンから 10 トークン生成する。各ステップで作る K/V の本数を数える";
  return `ステップ ${cf.value.step}: キャッシュ無しはコンテキスト全体 ${cf.value.stepNo} 本を作り直し、有りは${cf.value.step === 1 ? `プロンプトの ${PROMPT_LEN} 本(初回だけ)` : "新トークンの 1 本だけ"}。累計の差が開いていく`;
});

const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < frameCount.value - 1);
function first() { at.value = 0; }
function prev() { if (canPrev.value) at.value--; }
function next() { if (canNext.value) at.value++; }
function last() { at.value = frameCount.value - 1; }

const badge = computed(() =>
  mode.value === "cache" ? `step ${cf.value.step}/${GEN_STEPS}` : `${sf.value.produced} tok / ${sf.value.passes} パス`,
);
</script>

<template>
  <DemoShell title="推論高速化" badge-tone="neutral" :badge="badge">
    <div class="sd-controls">
      <span class="sd-seg">
        <span v-for="m in modes" :key="m.key" class="sd-seg-opt" :class="{ on: mode === m.key }" @click="setMode(m.key)">{{ m.label }}</span>
      </span>
      <span class="spacer" />
      <span v-if="mode === 'spec'" class="sd-seg">
        <span v-for="(d, i) in draftKinds" :key="d.key" class="sd-seg-opt" :class="{ on: draftPick === i }" @click="setDraft(i)">{{ d.label }}</span>
      </span>
    </div>

    <!-- KV キャッシュ -->
    <div v-if="mode === 'cache'" class="if-cache">
      <div class="if-row">
        <span class="if-label">キャッシュ無し</span>
        <span class="if-track"><span class="if-fill no" :style="{ width: (cf.no / CACHE_MAX) * 100 + '%' }"></span></span>
        <span class="if-val mono">{{ cf.no }} 回</span>
      </div>
      <div class="if-row">
        <span class="if-label">キャッシュ有り</span>
        <span class="if-track"><span class="if-fill yes" :style="{ width: (cf.withC / CACHE_MAX) * 100 + '%' }"></span></span>
        <span class="if-val mono">{{ cf.withC }} 回</span>
      </div>
      <p class="if-sub">K/V 射影の累計回数。生成列そのものは両者で完全に同じ</p>
    </div>

    <!-- speculative -->
    <div v-else class="if-spec">
      <div class="if-spec-block" v-if="sf.props.length">
        <div class="if-spec-head">ドラフトの下書き(γ = {{ GAMMA }})</div>
        <div class="if-chips">
          <span v-for="(p, i) in sf.props" :key="i" class="if-chip mono" :class="p.ok ? 'okc' : 'ngc'">
            {{ p.v }}<template v-if="p.ok"> ✓</template><template v-else> ✗</template>
          </span>
        </div>
      </div>
      <div class="if-spec-block">
        <div class="if-spec-head">確定した列(本命単独の生成と常に一致)</div>
        <div class="if-chips">
          <span v-for="(t, i) in sf.seq" :key="i" class="if-chip mono" :class="t.kind">{{ t.v }}</span>
        </div>
        <div class="if-key">
          <span class="if-key-item"><span class="if-swatch prompt"></span>プロンプト</span>
          <span class="if-key-item"><span class="if-swatch accept"></span>一致採用</span>
          <span class="if-key-item"><span class="if-swatch fix"></span>本命の訂正</span>
          <span class="if-key-item"><span class="if-swatch bonus"></span>ボーナス</span>
        </div>
      </div>
    </div>

    <p class="if-note">{{ note }}</p>

    <div class="if-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="if-nav mono">{{ at + 1 }} / {{ frameCount }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">{{ mode === 'cache' ? '1ステップ生成' : '1パスすすめる' }}</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="if-legend">
      KV キャッシュは同じ計算の重複を省くだけなので出力は不変、代償はメモリ。
      speculative はドラフトの質がパス数にだけ効き、出力は本命単独と完全一致する。
      どちらも「速くしても品質が落ちない」ことが構造で保証された高速化になっている。
    </p>
  </DemoShell>
</template>

<style scoped>
.if-cache {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 14px;
  background-color: var(--vp-c-bg-soft);
}
.if-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 6px 0;
}
.if-label {
  width: 110px;
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.if-track {
  flex: 1;
  height: 14px;
  background-color: var(--vp-c-bg);
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.if-fill {
  display: block;
  height: 100%;
}
.if-fill.no { background-color: var(--vp-c-danger-1); }
.if-fill.yes { background-color: var(--vp-c-green-1); }
.if-val {
  width: 64px;
  text-align: right;
  font-size: 12px;
}
.if-sub {
  margin: 8px 0 0;
  font-size: 11.5px;
  color: var(--vp-c-text-3);
}
.if-spec {
  margin-top: 14px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.if-spec-block {
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.if-spec-head {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  padding: 7px 12px;
  border-bottom: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.if-chips {
  padding: 10px 12px;
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  min-height: 40px;
}
.if-chip {
  font-size: 12px;
  padding: 3px 8px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  background-color: var(--vp-c-bg-soft);
  color: var(--vp-c-text-2);
}
.if-chip.okc, .if-chip.accept { border-color: var(--vp-c-green-1); background-color: var(--vp-c-green-soft); color: var(--vp-c-green-1); }
.if-chip.ngc, .if-chip.fix { border-color: var(--vp-c-danger-1); background-color: var(--vp-c-danger-soft); color: var(--vp-c-danger-1); }
.if-chip.bonus { border-color: var(--vp-c-brand-1); background-color: var(--vp-c-brand-soft); color: var(--vp-c-brand-1); }
.if-key {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  padding: 0 12px 10px;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.if-key-item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.if-swatch {
  width: 10px;
  height: 10px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 2px;
  background-color: var(--vp-c-bg-soft);
}
.if-swatch.accept { border-color: var(--vp-c-green-1); background-color: var(--vp-c-green-soft); }
.if-swatch.fix { border-color: var(--vp-c-danger-1); background-color: var(--vp-c-danger-soft); }
.if-swatch.bonus { border-color: var(--vp-c-brand-1); background-color: var(--vp-c-brand-soft); }
.if-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.if-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
  flex-wrap: wrap;
}
.if-nav {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.if-legend {
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
