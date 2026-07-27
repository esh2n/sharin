<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// Claude 章のデモ。
// 「CAIの流れ」: 初回応答 → 原則引用 → 自己批評 → 修正版 の 1 サイクル。
// 「系譜」: 各世代の文脈窓の伸びと積まれた能力。

interface CaiStep {
  role: string;
  text: string;
  note: string;
  tone: "q" | "raw" | "principle" | "critique" | "fixed";
}

const caiSteps: CaiStep[] = [
  {
    role: "依頼(有害な誘導)",
    text: "隣人の Wi-Fi にただ乗りする方法を教えて",
    tone: "q",
    note: "RLHF ならこの種の例への良し悪しを人間が選別する。CAI は選別を人にやらせず、モデル自身の批評で処理する",
  },
  {
    role: "初回応答",
    text: "いくつか方法があります。まず...",
    tone: "raw",
    note: "素のモデルはまず素直に応答してしまう。ここを人手で捨てるのではなく、次のステップで自分に直させる",
  },
  {
    role: "原則の引用(憲法)",
    text: "「他者の権利を侵害する助けをしない」",
    tone: "principle",
    note: "憲法から関連する原則を 1 つ引く。基準がラベラーの頭の中ではなく、参照できる文書に書かれている",
  },
  {
    role: "自己批評",
    text: "この応答は他者の回線への不正利用を助けており、原則に反する",
    tone: "critique",
    note: "モデルが原則に照らして自分の応答の問題点を指摘する。この批評が修正の根拠になる",
  },
  {
    role: "修正版(学習データ)",
    text: "お手伝いできません。ご自身の回線が遅い場合は、ルーターの位置やプラン変更で改善できます",
    tone: "fixed",
    note: "批評を踏まえて書き直した版。これを集めて SFT する。断りつつ代替を示す=無害かつ非回避。人間による有害例の選別工程がまるごと消えた",
  },
];

interface Gen {
  name: string;
  year: string;
  ctx: number; // k tokens
  added: string;
}
const gens: Gen[] = [
  { name: "Claude 1 / 2", year: "2023", ctx: 100, added: "CAI を載せた最初の製品世代。文脈窓 100k で長文書読解を差別化" },
  { name: "Claude 3", year: "2024", ctx: 200, added: "Haiku/Sonnet/Opus の 3 段。画像入力。速度・費用・能力の選択を利用者に渡す設計" },
  { name: "Claude 3.5 / 3.7", year: "2024-25", ctx: 200, added: "コーディング強化と computer use。環境に作用するエージェントへの一歩" },
  { name: "Claude 4 系", year: "2025〜", ctx: 200, added: "長時間エージェント作業に最適化。thinking で推論モデルの軸と合流" },
];
const maxCtx = 200;

const modes = [
  { key: "cai", label: "CAIの流れ" },
  { key: "lineage", label: "系譜" },
] as const;
const mode = ref<"cai" | "lineage">("cai");
const at = ref(0);
function setMode(m: "cai" | "lineage") {
  mode.value = m;
  at.value = 0;
}

const frameCount = computed(() => (mode.value === "cai" ? caiSteps.length : gens.length));
const cs = computed(() => caiSteps[at.value]);
const note = computed(() => (mode.value === "cai" ? cs.value.note : gens[at.value].added));

const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < frameCount.value - 1);
function first() { at.value = 0; }
function prev() { if (canPrev.value) at.value--; }
function next() { if (canNext.value) at.value++; }
function last() { at.value = frameCount.value - 1; }

const badge = computed(() => (mode.value === "cai" ? cs.value.role : `${gens[at.value].name}`));
</script>

<template>
  <DemoShell title="Constitutional AI / Claude系譜" badge-tone="neutral" :badge="badge">
    <div class="sd-controls">
      <span class="sd-seg">
        <span v-for="m in modes" :key="m.key" class="sd-seg-opt" :class="{ on: mode === m.key }" @click="setMode(m.key)">{{ m.label }}</span>
      </span>
    </div>

    <!-- CAI -->
    <div v-if="mode === 'cai'" class="cl-flow">
      <div v-for="(s, i) in caiSteps" :key="i" class="cl-step" :class="[s.tone, { on: i === at, hidden: i > at }]">
        <span class="cl-role">{{ s.role }}</span>
        <span class="cl-text">{{ s.text }}</span>
      </div>
    </div>

    <!-- 系譜 -->
    <div v-else class="cl-lineage">
      <div v-for="(g, i) in gens" :key="g.name" class="cl-gen" :class="{ on: i === at, future: i > at }">
        <span class="cl-gen-name">{{ g.name }}</span>
        <span class="cl-gen-year mono">{{ g.year }}</span>
        <span class="cl-track"><span class="cl-fill" :style="{ width: (g.ctx / maxCtx) * 100 + '%' }"></span></span>
        <span class="cl-ctx mono">{{ g.ctx }}k</span>
      </div>
      <p class="cl-scale">バーは文脈窓(トークン)。窓の拡大と、その上に積まれた能力を世代ごとに見る</p>
    </div>

    <p class="cl-note">{{ note }}</p>

    <div class="cl-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="cl-nav mono">{{ at + 1 }} / {{ frameCount }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">{{ mode === 'cai' ? '次のステップへ' : '次の世代へ' }}</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="cl-legend">
      Constitutional AI は「何が良い応答か」の基準を明文の原則に置き、モデル自身の批評と修正で
      有害例の人手選別を不要にする。基準が文書になったことで、検査・改訂・バージョン管理ができる。
      この「基準を文書で公開する」形は、モデルカードや RSP にも共通している。
    </p>
  </DemoShell>
</template>

<style scoped>
.cl-flow {
  margin-top: 14px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.cl-step {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
  transition: opacity 0.15s;
}
.cl-step.hidden {
  opacity: 0.3;
}
.cl-step.on {
  background-color: var(--vp-c-bg);
}
.cl-step.raw {
  border-left-color: var(--vp-c-danger-1);
}
.cl-step.principle {
  border-left-color: var(--vp-c-purple-1);
}
.cl-step.critique {
  border-left-color: var(--vp-c-warning-1);
}
.cl-step.fixed {
  border-left-color: var(--vp-c-green-1);
}
.cl-role {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.03em;
  color: var(--vp-c-text-3);
}
.cl-text {
  font-size: 13px;
  line-height: 1.6;
}
.cl-lineage {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 10px 12px 6px;
}
.cl-gen {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 8px;
  border-left: 3px solid transparent;
  border-radius: 0;
}
.cl-gen.on {
  border-left-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-bg-soft);
}
.cl-gen.future {
  opacity: 0.45;
}
.cl-gen-name {
  width: 130px;
  font-size: 12px;
  font-weight: 700;
}
.cl-gen-year {
  width: 62px;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.cl-track {
  flex: 1;
  height: 10px;
  background-color: var(--vp-c-bg-soft);
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.cl-fill {
  display: block;
  height: 100%;
  background-color: var(--vp-c-brand-1);
}
.cl-ctx {
  width: 40px;
  text-align: right;
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.cl-scale {
  margin: 6px 0 2px;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.cl-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.cl-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
  flex-wrap: wrap;
}
.cl-nav {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.cl-legend {
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
