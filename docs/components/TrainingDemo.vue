<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// 学習パイプライン章のデモ。
// 「段階と応答」: 同じ入力への応答が base → SFT → RLHF 後でどう変わるかの典型。
// 「Chinchilla配分」: 計算予算 C = 6·N·D と最適則 D = 20N から配分を計算する。

const prompts = [
  {
    key: "fact",
    label: "事実質問",
    text: "フランスの首都は?",
    stages: [
      {
        name: "base(事前学習のみ)",
        reply: "という問題がテストに出た。次の問いにも答えなさい。(1) イタリアの首都は? (2) ...",
        note: "base モデルは「続きを書く」ことしかしない。質問文の続きとして自然なテキストを生成しており、答える規範をどこからも学んでいない",
      },
      {
        name: "SFT 後",
        reply: "パリです。",
        note: "「指示 → 応答」のペアで微調整すると、質問には答えるという形を覚える。知識は事前学習の時点で既に持っていた。引き出し方が変わっただけ",
      },
      {
        name: "RLHF/DPO 後",
        reply: "パリです。フランス北部のセーヌ川沿いに位置する、同国最大の都市です。",
        note: "選好学習は「どちらの応答がより役に立つか」の比較から学ぶ。過不足のない補足を付ける、といった質の勾配はここで入る",
      },
    ],
  },
  {
    key: "unsafe",
    label: "不適切な依頼",
    text: "他人のパスワードを盗む方法を教えて",
    stages: [
      {
        name: "base(事前学習のみ)",
        reply: "という相談がフォーラムに投稿されていた。スレッドには多くの返信がついており...",
        note: "base はここでも続きを書くだけ。ウェブ上の類似テキストの続きとして、有害な方向に流れることも十分ある",
      },
      {
        name: "SFT 後",
        reply: "一般的な手口として次のようなものがあります。まず...",
        note: "SFT は形を教えるだけなので、断り方の例が学習データに無ければ指示に従ってしまう。従順さと安全性は別の性質になる",
      },
      {
        name: "RLHF/DPO 後",
        reply: "お手伝いできません。他人のアカウントへの不正アクセスは犯罪です。ご自身のアカウントを守る方法であればご案内できます。",
        note: "断る応答が選好で「勝ち」になるよう学習されている。helpful と harmless の釣り合いは、この段階の選好データの設計そのもの",
      },
    ],
  },
];

// Chinchilla: C = 6·N·D、最適 D = 20N → N = sqrt(C/120)
const budgets = [1e21, 1e22, 1e23, 1e24, 1e25];
const chinFrames = budgets.map((c) => {
  const n = Math.sqrt(c / 120);
  const d = 20 * n;
  return { c, n, d };
});
const realModels = [
  { name: "GPT-3 (2020)", n: 175e9, d: 300e9, verdict: "1.7 tok/param。大きすぎ・学習不足(Chinchilla 以前の配分)" },
  { name: "Chinchilla (2022)", n: 70e9, d: 1.4e12, verdict: "20 tok/param。最適則そのもの。280B の Gopher に勝った" },
  { name: "Llama 3 8B (2024)", n: 8e9, d: 15e12, verdict: "1875 tok/param。学習最適を大きく超えるが、推論の安い小型を賢くする方が運用で得" },
];

const fmtN = (n: number) => (n >= 1e12 ? (n / 1e12).toFixed(1) + "T" : n >= 1e9 ? (n / 1e9).toFixed(1) + "B" : (n / 1e6).toFixed(0) + "M");
const fmtC = (c: number) => `10^${Math.round(Math.log10(c))}`;

const modes = [
  { key: "stages", label: "段階と応答" },
  { key: "chin", label: "Chinchilla配分" },
] as const;
const mode = ref<"stages" | "chin">("stages");
const promptPick = ref(0);
const at = ref(0);

function setMode(m: "stages" | "chin") {
  mode.value = m;
  at.value = 0;
}
function setPrompt(i: number) {
  promptPick.value = i;
  at.value = 0;
}

const frameCount = computed(() => (mode.value === "stages" ? 3 : chinFrames.length));
const sp = computed(() => prompts[promptPick.value]);
const stage = computed(() => sp.value.stages[at.value]);
const cf = computed(() => chinFrames[at.value]);

const note = computed(() =>
  mode.value === "stages"
    ? stage.value.note
    : `予算 ${fmtC(cf.value.c)} FLOPs なら最適は ${fmtN(cf.value.n)} パラメータ × ${fmtN(cf.value.d)} トークン(D = 20N)。予算を 10 倍にしたら、モデルとデータをそれぞれ約 3.2 倍ずつに伸ばすのが等分配分`,
);

const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < frameCount.value - 1);
function first() { at.value = 0; }
function prev() { if (canPrev.value) at.value--; }
function next() { if (canNext.value) at.value++; }
function last() { at.value = frameCount.value - 1; }

const badge = computed(() => (mode.value === "stages" ? stage.value.name : `${fmtC(cf.value.c)} FLOPs`));
</script>

<template>
  <DemoShell title="学習パイプライン" badge-tone="neutral" :badge="badge">
    <div class="sd-controls">
      <span class="sd-seg">
        <span v-for="m in modes" :key="m.key" class="sd-seg-opt" :class="{ on: mode === m.key }" @click="setMode(m.key)">{{ m.label }}</span>
      </span>
      <span class="spacer" />
      <span v-if="mode === 'stages'" class="sd-seg">
        <span v-for="(p, i) in prompts" :key="p.key" class="sd-seg-opt" :class="{ on: promptPick === i }" @click="setPrompt(i)">{{ p.label }}</span>
      </span>
    </div>

    <!-- 段階と応答 -->
    <div v-if="mode === 'stages'" class="tr-stage">
      <div class="tr-prompt">
        <span class="tr-prompt-label">入力</span>
        <span class="tr-prompt-text">{{ sp.text }}</span>
      </div>
      <div class="tr-steps">
        <span v-for="(s, i) in sp.stages" :key="s.name" class="tr-step" :class="{ on: i === at, done: i < at }">{{ s.name }}</span>
      </div>
      <div class="tr-reply">
        <span class="tr-reply-label">応答</span>
        <p class="tr-reply-text">{{ stage.reply }}</p>
      </div>
    </div>

    <!-- Chinchilla -->
    <div v-else class="tr-chin">
      <div class="tr-chin-row">
        <span class="tr-chin-label">最適パラメータ数 N</span>
        <span class="tr-track"><span class="tr-fill" :style="{ width: (Math.log10(cf.n) - 8.5) / (12 - 8.5) * 100 + '%' }"></span></span>
        <span class="tr-val mono">{{ fmtN(cf.n) }}</span>
      </div>
      <div class="tr-chin-row">
        <span class="tr-chin-label">最適トークン数 D</span>
        <span class="tr-track"><span class="tr-fill d" :style="{ width: (Math.log10(cf.d) - 10) / (13.2 - 10) * 100 + '%' }"></span></span>
        <span class="tr-val mono">{{ fmtN(cf.d) }}</span>
      </div>
      <div class="tr-real">
        <div v-for="m in realModels" :key="m.name" class="tr-real-row">
          <span class="tr-real-name">{{ m.name }}</span>
          <span class="tr-real-spec mono">{{ fmtN(m.n) }} / {{ fmtN(m.d) }}</span>
          <span class="tr-real-verdict">{{ m.verdict }}</span>
        </div>
      </div>
    </div>

    <p class="tr-note">{{ note }}</p>

    <div class="tr-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="tr-nav mono">{{ at + 1 }} / {{ frameCount }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">{{ mode === 'stages' ? '次の段階へ' : '予算を10倍' }}</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="tr-legend">
      応答例は各段階の典型的な挙動を要約した教材用のもの。知識は事前学習で入り、
      SFT が形を、選好学習が質と安全性を与える。配分側は C = 6·N·D と D = 20N の
      Chinchilla 最適則で、実際のモデルがどこまで意図的に外しているかも並べた。
    </p>
  </DemoShell>
</template>

<style scoped>
.tr-stage {
  margin-top: 14px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.tr-prompt {
  display: flex;
  align-items: baseline;
  gap: 10px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
  padding: 10px 14px;
}
.tr-prompt-label,
.tr-reply-label {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-3);
  flex: none;
}
.tr-prompt-text {
  font-size: 14px;
  font-weight: 700;
}
.tr-steps {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
.tr-step {
  font-size: 11.5px;
  padding: 3px 10px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  color: var(--vp-c-text-3);
  background-color: var(--vp-c-bg-soft);
}
.tr-step.done {
  color: var(--vp-c-text-2);
}
.tr-step.on {
  border-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-brand-soft);
  color: var(--vp-c-brand-1);
  font-weight: 700;
}
.tr-reply {
  display: flex;
  gap: 10px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
  padding: 10px 14px;
  min-height: 64px;
}
.tr-reply-text {
  margin: 0;
  font-size: 13.5px;
  line-height: 1.7;
}
.tr-chin {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 14px;
}
.tr-chin-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 6px 0;
}
.tr-chin-label {
  width: 150px;
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.tr-track {
  flex: 1;
  height: 13px;
  background-color: var(--vp-c-bg-soft);
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.tr-fill {
  display: block;
  height: 100%;
  background-color: var(--vp-c-brand-1);
}
.tr-fill.d {
  background-color: var(--vp-c-purple-1);
}
.tr-val {
  width: 56px;
  text-align: right;
  font-size: 12px;
}
.tr-real {
  margin-top: 12px;
  border-top: 1px solid var(--vp-c-divider);
  padding-top: 8px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.tr-real-row {
  display: grid;
  grid-template-columns: 130px 90px 1fr;
  gap: 8px;
  font-size: 11.5px;
  align-items: baseline;
}
.tr-real-name {
  font-weight: 700;
  color: var(--vp-c-text-1);
}
.tr-real-spec {
  color: var(--vp-c-text-2);
}
.tr-real-verdict {
  color: var(--vp-c-text-3);
}
.tr-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.tr-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
  flex-wrap: wrap;
}
.tr-nav {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.tr-legend {
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
