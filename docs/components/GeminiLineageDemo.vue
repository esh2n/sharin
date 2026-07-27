<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// Gemini 章のデモ。
// 「トークン化」: 画像/音声/テキストがパッチ/符号/サブワードを経て共通トークン列に合流。
// 「文脈窓の比較」: 各モデル・入力サイズの文脈長を対数スケールで並べる。

interface TokFrame {
  title: string;
  rows: { modality: string; raw: string; tokens: string[]; kind: string }[];
  merged: boolean;
  note: string;
}

const tokFrames: TokFrame[] = [
  {
    title: "生の入力",
    rows: [
      { modality: "画像", raw: "224×224 の写真", tokens: [], kind: "img" },
      { modality: "音声", raw: "3 秒の波形", tokens: [], kind: "aud" },
      { modality: "テキスト", raw: "「猫が鳴いた」", tokens: [], kind: "txt" },
    ],
    merged: false,
    note: "種類の違う 3 つの入力。このままでは Transformer に流せない。それぞれを「切ってトークンにする」工程に通す",
  },
  {
    title: "モダリティごとに切る",
    rows: [
      { modality: "画像", raw: "16×16 パッチ格子", tokens: ["P1", "P2", "P3", "…196"], kind: "img" },
      { modality: "音声", raw: "時間窓ごとに量子化", tokens: ["A1", "A2", "…150"], kind: "aud" },
      { modality: "テキスト", raw: "BPE でサブワード", tokens: ["猫", "が", "鳴い", "た"], kind: "txt" },
    ],
    merged: false,
    note: "画像はパッチ(ViT)、音声は符号、テキストは BPE。工程はモダリティごとに違うが、出てくるのはどれも「トークン列」で揃う",
  },
  {
    title: "1つの系列に合流",
    rows: [],
    merged: true,
    note: "全部を 1 本のトークン列に連結する。attention は種類を区別しないので、キャプションの「猫」とそれが写ったパッチの間に直接の注目を張れる。これがネイティブマルチモーダルの原理",
  },
];

const mergedTokens = [
  { t: "P1", k: "img" }, { t: "P2", k: "img" }, { t: "…", k: "img" },
  { t: "A1", k: "aud" }, { t: "A2", k: "aud" }, { t: "…", k: "aud" },
  { t: "猫", k: "txt" }, { t: "が", k: "txt" }, { t: "鳴い", k: "txt" }, { t: "た", k: "txt" },
];

// 文脈窓の比較(対数)
const ctxItems = [
  { name: "GPT-3(2020)", tokens: 4096, kind: "model" },
  { name: "Claude 2", tokens: 100000, kind: "model" },
  { name: "Claude 3 / GPT-4o", tokens: 200000, kind: "model" },
  { name: "Gemini 1.5", tokens: 1000000, kind: "model" },
  { name: "── 1時間の動画", tokens: 600000, kind: "input" },
  { name: "── 中規模コードベース", tokens: 800000, kind: "input" },
  { name: "── 書籍1冊", tokens: 120000, kind: "input" },
];
const logMin = Math.log10(4000);
const logMax = Math.log10(1e6);
const ctxPct = (n: number) => ((Math.log10(n) - logMin) / (logMax - logMin)) * 100;
const fmtTok = (n: number) => (n >= 1e6 ? (n / 1e6).toFixed(0) + "M" : n >= 1000 ? (n / 1000).toFixed(0) + "k" : "" + n);

const modes = [
  { key: "token", label: "トークン化" },
  { key: "ctx", label: "文脈窓の比較" },
] as const;
const mode = ref<"token" | "ctx">("token");
const at = ref(0);
function setMode(m: "token" | "ctx") {
  mode.value = m;
  at.value = 0;
}

const frameCount = computed(() => (mode.value === "token" ? tokFrames.length : 1));
const tf = computed(() => tokFrames[at.value]);
const note = computed(() =>
  mode.value === "token"
    ? tf.value.note
    : "文脈窓を対数で並べた。Gemini 1.5 の 100 万トークンは、1 時間の動画や中規模コードベースを丸ごと 1 つの窓に収められる桁。マルチモーダルは入力のトークン数を押し上げるので、この長さと相乗する",
);

const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < frameCount.value - 1);
function first() { at.value = 0; }
function prev() { if (canPrev.value) at.value--; }
function next() { if (canNext.value) at.value++; }
function last() { at.value = frameCount.value - 1; }

const badge = computed(() => (mode.value === "token" ? tf.value.title : "文脈窓(対数)"));
</script>

<template>
  <DemoShell title="Gemini(マルチモーダルと長文脈)" badge-tone="neutral" :badge="badge">
    <div class="sd-controls">
      <span class="sd-seg">
        <span v-for="m in modes" :key="m.key" class="sd-seg-opt" :class="{ on: mode === m.key }" @click="setMode(m.key)">{{ m.label }}</span>
      </span>
    </div>

    <!-- トークン化 -->
    <div v-if="mode === 'token'" class="gm-token">
      <div v-if="!tf.merged" class="gm-rows">
        <div v-for="r in tf.rows" :key="r.modality" class="gm-row" :class="r.kind">
          <span class="gm-mod">{{ r.modality }}</span>
          <span class="gm-raw">{{ r.raw }}</span>
          <span v-if="r.tokens.length" class="gm-arrow mono">→</span>
          <span class="gm-toks">
            <span v-for="(t, i) in r.tokens" :key="i" class="gm-tok mono" :class="r.kind">{{ t }}</span>
          </span>
        </div>
      </div>
      <div v-else class="gm-merged">
        <div class="gm-merged-label">共通のトークン列</div>
        <div class="gm-merged-row">
          <span v-for="(m, i) in mergedTokens" :key="i" class="gm-tok mono" :class="m.k">{{ m.t }}</span>
        </div>
        <div class="gm-attn mono">↑ attention は種類を区別せず、全トークン間に注目を張れる</div>
      </div>
    </div>

    <!-- 文脈窓 -->
    <div v-else class="gm-ctx">
      <div v-for="c in ctxItems" :key="c.name" class="gm-ctx-row" :class="c.kind">
        <span class="gm-ctx-name">{{ c.name }}</span>
        <span class="gm-ctx-track"><span class="gm-ctx-fill" :class="c.kind" :style="{ width: ctxPct(c.tokens) + '%' }"></span></span>
        <span class="gm-ctx-val mono">{{ fmtTok(c.tokens) }}</span>
      </div>
      <p class="gm-ctx-scale">横軸は対数(4k〜1M)。── は入力サイズの目安(モデルではない)</p>
    </div>

    <p class="gm-note">{{ note }}</p>

    <div class="gm-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="gm-nav mono">{{ at + 1 }} / {{ frameCount }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">1手すすめる</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="gm-legend">
      どんな入力も「切ってトークンにする」工程さえ用意すれば、Transformer 本体は共通のまま
      1 つの系列として処理できる。Gemini はこれを最初から一体で学習し(ネイティブ)、
      さらに 100 万トークン級の窓で動画やコードベースを丸ごと入力に置く設計を取った。
    </p>
  </DemoShell>
</template>

<style scoped>
.gm-token {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
  padding: 14px;
  min-height: 150px;
}
.gm-rows {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.gm-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  border-left: 3px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 6px 10px;
  background-color: var(--vp-c-bg);
}
.gm-row.img { border-left-color: var(--vp-c-brand-1); }
.gm-row.aud { border-left-color: var(--vp-c-purple-1); }
.gm-row.txt { border-left-color: var(--vp-c-green-1); }
.gm-mod {
  width: 54px;
  font-size: 12px;
  font-weight: 700;
}
.gm-raw {
  font-size: 12.5px;
  color: var(--vp-c-text-2);
  min-width: 120px;
}
.gm-arrow {
  color: var(--vp-c-text-3);
}
.gm-toks {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}
.gm-tok {
  font-size: 11.5px;
  padding: 2px 7px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  background-color: var(--vp-c-bg-soft);
}
.gm-tok.img { color: var(--vp-c-brand-1); border-color: var(--vp-c-brand-1); }
.gm-tok.aud { color: var(--vp-c-purple-1); border-color: var(--vp-c-purple-1); }
.gm-tok.txt { color: var(--vp-c-green-1); border-color: var(--vp-c-green-1); }
.gm-merged {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.gm-merged-label {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-3);
}
.gm-merged-row {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}
.gm-attn {
  font-size: 11.5px;
  color: var(--vp-c-text-3);
  margin-top: 4px;
}
.gm-ctx {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 10px 12px 6px;
}
.gm-ctx-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
}
.gm-ctx-name {
  width: 160px;
  font-size: 12px;
}
.gm-ctx-row.input .gm-ctx-name {
  color: var(--vp-c-text-3);
  font-style: italic;
}
.gm-ctx-track {
  flex: 1;
  height: 11px;
  background-color: var(--vp-c-bg-soft);
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.gm-ctx-fill {
  display: block;
  height: 100%;
  background-color: var(--vp-c-brand-1);
}
.gm-ctx-fill.input {
  background: repeating-linear-gradient(-45deg, var(--vp-c-text-3), var(--vp-c-text-3) 3px, transparent 3px, transparent 6px);
  opacity: 0.6;
}
.gm-ctx-val {
  width: 40px;
  text-align: right;
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.gm-ctx-scale {
  margin: 6px 0 2px;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.gm-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.gm-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
  flex-wrap: wrap;
}
.gm-nav {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.gm-legend {
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
