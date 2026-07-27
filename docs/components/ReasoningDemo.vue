<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// 推論モデル章のデモ。
// 「CoTの効果」: 1 トークン = 固定計算の即答が失敗し、途中式 = 計算の借り入れで通る。
// 「GRPO」: グループ平均を基準に強化/抑制が決まる(価値モデル不要)。

interface CotFrame {
  label: string;
  lines: { text: string; kind: "q" | "wrong" | "step" | "ok" }[];
  note: string;
}

const cotFrames: CotFrame[] = [
  {
    label: "即答(直接)",
    lines: [
      { text: "23 × 17 = ?", kind: "q" },
      { text: "→ 401(誤答)", kind: "wrong" },
    ],
    note: "1 トークンの生成に使える計算は固定(層数×ブロック)。繰り上がりを複数回追う逐次計算はその中に収まらず、即答はもっともらしい誤答になる",
  },
  {
    label: "CoT 1手目",
    lines: [
      { text: "23 × 17 = ?", kind: "q" },
      { text: "23 × 10 = 230", kind: "step" },
    ],
    note: "途中式を書き始める。このステップ単体は固定計算に収まる小さな問題で、書いた式は以後 attention で参照できる作業メモになる",
  },
  {
    label: "CoT 2手目",
    lines: [
      { text: "23 × 17 = ?", kind: "q" },
      { text: "23 × 10 = 230", kind: "step" },
      { text: "23 × 7 = 161", kind: "step" },
    ],
    note: "分解した 2 つ目の積。トークンを費やすこと自体が、追加の計算を問題に割り当てる行為になっている",
  },
  {
    label: "CoT 3手目",
    lines: [
      { text: "23 × 17 = ?", kind: "q" },
      { text: "23 × 10 = 230", kind: "step" },
      { text: "23 × 7 = 161", kind: "step" },
      { text: "230 + 161 = 391 ✓", kind: "ok" },
    ],
    note: "前の 2 行を attention で読んで足すだけの小問題に落ちた。思考の文章は説明ではなく、モデル自身の計算資源として働いている",
  },
];

// GRPO: 4 本サンプル、報酬、advantage = r - mean
const samples = [
  { name: "答え1", summary: "途中式を書き 391 と回答", reward: 1 },
  { name: "答え2", summary: "即答で 401 と回答", reward: 0 },
  { name: "答え3", summary: "別の分解で 391 と回答", reward: 1 },
  { name: "答え4", summary: "検算せず 385 と回答", reward: 0 },
];
const mean = samples.reduce((a, s) => a + s.reward, 0) / samples.length;

const grpoNotes = [
  "同じ問題「23 × 17 = ?」に対して、現在のモデルから答えを 4 本サンプリングする",
  "検証器が採点する。数学は答え合わせが機械的にできるので、報酬(正解 1 / 不正解 0)が自動で作れる",
  `グループの平均報酬 ${mean.toFixed(1)} を基準にする。PPO が別ネットワーク(価値モデル)で出していた基準値を、グループ内の相対比較で置き換えるのが GRPO の簡略化`,
  "平均より良い答えの出し方を強化し、悪い答えの出し方を抑える。正解につながる「途中式を書く」振る舞いが、教えなくても報酬経由で育っていく",
];

const modes = [
  { key: "cot", label: "CoTの効果" },
  { key: "grpo", label: "GRPO" },
] as const;
const mode = ref<"cot" | "grpo">("cot");
const at = ref(0);
function setMode(m: "cot" | "grpo") {
  mode.value = m;
  at.value = 0;
}

const frameCount = computed(() => (mode.value === "cot" ? cotFrames.length : grpoNotes.length));
const cf = computed(() => cotFrames[at.value]);
const note = computed(() => (mode.value === "cot" ? cf.value.note : grpoNotes[at.value]));

const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < frameCount.value - 1);
function first() { at.value = 0; }
function prev() { if (canPrev.value) at.value--; }
function next() { if (canNext.value) at.value++; }
function last() { at.value = frameCount.value - 1; }

const badge = computed(() => (mode.value === "cot" ? cf.value.label : ["サンプル", "採点", "基準", "更新"][at.value]));
</script>

<template>
  <DemoShell title="推論モデル(思考と強化)" badge-tone="neutral" :badge="badge">
    <div class="sd-controls">
      <span class="sd-seg">
        <span v-for="m in modes" :key="m.key" class="sd-seg-opt" :class="{ on: mode === m.key }" @click="setMode(m.key)">{{ m.label }}</span>
      </span>
    </div>

    <!-- CoT -->
    <div v-if="mode === 'cot'" class="rs-panel">
      <div v-for="(l, i) in cf.lines" :key="i" class="rs-line mono" :class="l.kind">{{ l.text }}</div>
    </div>

    <!-- GRPO -->
    <div v-else class="rs-grpo">
      <div v-for="(s, i) in samples" :key="s.name" class="rs-sample" :class="{ good: at >= 3 && s.reward > mean, bad: at >= 3 && s.reward < mean }">
        <span class="rs-sample-name">{{ s.name }}</span>
        <span class="rs-sample-sum">{{ s.summary }}</span>
        <span v-if="at >= 1" class="rs-chip mono" :class="s.reward === 1 ? 'okc' : 'ngc'">r = {{ s.reward }}</span>
        <span v-if="at >= 2" class="rs-chip mono adv">A = {{ (s.reward - mean).toFixed(1) }}</span>
        <span v-if="at >= 3" class="rs-verdict mono">{{ s.reward > mean ? "強化 ↑" : "抑制 ↓" }}</span>
      </div>
      <p v-if="at >= 2" class="rs-mean mono">グループ平均 = {{ mean.toFixed(1) }}(これが基準。価値モデルは不要)</p>
    </div>

    <p class="rs-note">{{ note }}</p>

    <div class="rs-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="rs-nav mono">{{ at + 1 }} / {{ frameCount }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">1手すすめる</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="rs-legend">
      左のモードは「思考を書く = 計算を借りる」を最小の例で示す。右のモードは R1 系の学習で、
      検証可能な課題なら報酬が自動で作れるため、この一連(サンプル → 採点 → 相対比較 → 更新)を
      大規模に回すだけで、長い思考や自己検証が教えられずに育つ。
    </p>
  </DemoShell>
</template>

<style scoped>
.rs-panel {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
  padding: 14px;
  min-height: 132px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.rs-line {
  font-size: 14px;
  padding: 4px 10px;
  border-left: 3px solid transparent;
  border-radius: 0;
}
.rs-line.q {
  font-weight: 700;
  border-left-color: var(--vp-c-divider);
}
.rs-line.wrong {
  color: var(--vp-c-danger-1);
  border-left-color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.rs-line.step {
  color: var(--vp-c-text-2);
  border-left-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-bg);
}
.rs-line.ok {
  color: var(--vp-c-green-1);
  border-left-color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
  font-weight: 700;
}
.rs-grpo {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 10px 12px;
  min-height: 132px;
}
.rs-sample {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 8px;
  border-left: 3px solid transparent;
  border-radius: 0;
  font-size: 12.5px;
  flex-wrap: wrap;
}
.rs-sample.good {
  border-left-color: var(--vp-c-green-1);
  background-color: var(--vp-c-bg-soft);
}
.rs-sample.bad {
  border-left-color: var(--vp-c-danger-1);
  opacity: 0.75;
}
.rs-sample-name {
  font-weight: 700;
  width: 46px;
}
.rs-sample-sum {
  color: var(--vp-c-text-2);
  flex: 1;
  min-width: 150px;
}
.rs-chip {
  font-size: 11px;
  padding: 1px 7px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
}
.rs-chip.okc { color: var(--vp-c-green-1); border-color: var(--vp-c-green-1); background-color: var(--vp-c-green-soft); }
.rs-chip.ngc { color: var(--vp-c-danger-1); border-color: var(--vp-c-danger-1); background-color: var(--vp-c-danger-soft); }
.rs-chip.adv { color: var(--vp-c-brand-1); border-color: var(--vp-c-brand-1); background-color: var(--vp-c-brand-soft); }
.rs-verdict {
  font-size: 11.5px;
  font-weight: 700;
}
.rs-mean {
  margin: 8px 0 0;
  font-size: 11.5px;
  color: var(--vp-c-text-3);
}
.rs-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.rs-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
  flex-wrap: wrap;
}
.rs-nav {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.rs-legend {
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
