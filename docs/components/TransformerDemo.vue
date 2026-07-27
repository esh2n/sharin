<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// Transformer の骨格を 2 つの見方で追う。
// 「ブロックを追う」: 4 トークンが埋め込み → ブロック → logits と流れる工程を 1 手ずつ。
//   列をまたいで混ざるのは attention だけ、FFN はトークンごと、を色で示す。
// 「3系統のマスク」: 同じ 4 トークンに対する attention の可視範囲(マスク)を切り替え、
//   decoder-only / encoder-only / encoder-decoder の違いがマスクだけで説明できることを見る。

const TOKENS = ["猫", "が", "好き", "だ"];

type Visual = "vector" | "position" | "attn" | "ffn" | "stack" | "logits";

interface FlowFrame {
  stage: number;
  visual: Visual;
  note: string;
}

const STAGES = [
  "埋め込み",
  "位置情報を足す",
  "正規化 → attention(+残差)",
  "正規化 → FFN(+残差)",
  "ブロックを N 段重ねる",
  "語彙への射影 → logits",
];

const flowFrames: FlowFrame[] = [
  {
    stage: 0,
    visual: "vector",
    note: "トークン ID を埋め込み表で d 次元ベクトルに変える。この時点では各トークンは孤立していて、順序も知らない",
  },
  {
    stage: 1,
    visual: "position",
    note: "位置埋め込みを足して「何番目か」を注入する。attention は順序を見ないので、ここで教えておく必要がある",
  },
  {
    stage: 2,
    visual: "attn",
    note: "attention がトークン間で情報を混ぜる。因果マスクにより各トークンは自分より過去だけを見る。列をまたぐ計算はブロック内でここだけ。出力は残差で入力に足し戻される",
  },
  {
    stage: 3,
    visual: "ffn",
    note: "FFN が各トークンを独立に変換する。d → 4d → d と広げて戻す非線形変換で、列はまたがない。パラメータはここが最大(モデルの約 2/3)。これも残差で足し戻す",
  },
  {
    stage: 4,
    visual: "stack",
    note: "混ぜる → 変換するの 1 ブロックを N 段重ねる。GPT-2 で 12〜48 段。残差のおかげで深く積んでも学習が壊れない",
  },
  {
    stage: 5,
    visual: "logits",
    note: "最終正規化のあと語彙数へ射影し、各位置で「次のトークン」の分布(logits)を得る。ここから先の選び方は llm-sampling 編の仕事",
  },
];

// 「だ」の次のトークン分布(教材用の固定値)
const logitBars = [
  { tok: "。", p: 0.62 },
  { tok: "ね", p: 0.21 },
  { tok: "よ", p: 0.12 },
  { tok: "その他", p: 0.05 },
];

// --- 3系統のマスク ---
interface MaskFrame {
  name: string;
  models: string;
  tokens: string[];
  // visible[q][k] = query 行 q が key 列 k を見てよいか
  visible: boolean[][];
  note: string;
}

const causal = (n: number) => Array.from({ length: n }, (_, q) => Array.from({ length: n }, (_, k) => k <= q));
const full = (n: number) => Array.from({ length: n }, () => Array.from({ length: n }, () => true));
// encoder-decoder: 前半 2 つが入力(相互に全部見える)、後半 2 つが出力(入力全部 + 出力の過去)
const encDec = [
  [true, true, false, false],
  [true, true, false, false],
  [true, true, true, false],
  [true, true, true, true],
];

const maskFrames: MaskFrame[] = [
  {
    name: "decoder-only",
    models: "GPT / Claude / Llama",
    tokens: TOKENS,
    visible: causal(4),
    note: "因果マスク。各トークンは自分より過去だけを見る。学習は「次の語の予測」で、全位置が同時に教師信号になる。生成 LLM の主流",
  },
  {
    name: "encoder-only",
    models: "BERT / 検索用埋め込み",
    tokens: TOKENS,
    visible: full(4),
    note: "マスクなし。全位置が相互に見えるので文全体の表現が濃い。そのかわり左から右への生成はできない。分類・検索・埋め込みで現役",
  },
  {
    name: "encoder-decoder",
    models: "T5 / Whisper / 翻訳",
    tokens: ["入1", "入2", "出1", "出2"],
    visible: encDec,
    note: "入力側は相互に全部見え、出力側は入力の全部と出力の過去を見る。入力と出力の様式が違う変換(音声 → テキストなど)で今も自然な形",
  },
];

const modes = [
  { key: "flow", label: "ブロックを追う" },
  { key: "mask", label: "3系統のマスク" },
] as const;
const mode = ref<"flow" | "mask">("flow");
const at = ref(0);

function setMode(m: "flow" | "mask") {
  mode.value = m;
  at.value = 0;
}

const frameCount = computed(() => (mode.value === "flow" ? flowFrames.length : maskFrames.length));
const flowCur = computed(() => flowFrames[at.value]);
const maskCur = computed(() => maskFrames[at.value]);
const note = computed(() => (mode.value === "flow" ? flowCur.value.note : maskCur.value.note));

const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < frameCount.value - 1);
function first() {
  at.value = 0;
}
function prev() {
  if (canPrev.value) at.value--;
}
function next() {
  if (canNext.value) at.value++;
}
function last() {
  at.value = frameCount.value - 1;
}

const badge = computed(() => (mode.value === "flow" ? STAGES[flowCur.value.stage] : maskCur.value.name));
const badgeTone = computed<"ok" | "neutral">(() =>
  mode.value === "flow" && flowCur.value.visual === "logits" ? "ok" : "neutral",
);
</script>

<template>
  <DemoShell title="Transformer(骨格の解剖)" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <span class="sd-seg">
        <span
          v-for="m in modes"
          :key="m.key"
          class="sd-seg-opt"
          :class="{ on: mode === m.key }"
          @click="setMode(m.key)"
          >{{ m.label }}</span
        >
      </span>
    </div>

    <!-- ブロックを追う -->
    <template v-if="mode === 'flow'">
      <div class="tf-stages">
        <div
          v-for="(s, i) in STAGES"
          :key="s"
          class="tf-stage"
          :class="{ on: i === flowCur.stage, done: i < flowCur.stage }"
        >
          <span class="tf-stage-n mono">{{ i + 1 }}</span>{{ s }}
        </div>
      </div>

      <div class="tf-panel">
        <!-- 埋め込み / 位置 -->
        <div v-if="flowCur.visual === 'vector' || flowCur.visual === 'position'" class="tf-cols">
          <div v-for="(t, i) in TOKENS" :key="t" class="tf-col">
            <span class="tf-tok">{{ t }}</span>
            <span class="tf-vec mono">d次元</span>
            <span v-if="flowCur.visual === 'position'" class="tf-pos mono">+位置{{ i }}</span>
          </div>
        </div>

        <!-- attention: 因果マスクの行列 -->
        <div v-else-if="flowCur.visual === 'attn'" class="tf-gridwrap">
          <div class="tf-grid" :style="{ '--n': 4 }">
            <span class="tf-cell head"></span>
            <span v-for="t in TOKENS" :key="'h' + t" class="tf-cell head mono">{{ t }}</span>
            <template v-for="(row, q) in TOKENS" :key="'r' + q">
              <span class="tf-cell head mono">{{ row }}</span>
              <span
                v-for="(col, k) in TOKENS"
                :key="q + '-' + k"
                class="tf-cell"
                :class="{ vis: k <= q }"
              ></span>
            </template>
          </div>
          <p class="tf-gridnote">行 = 情報を集めるトークン、列 = 見てよい相手。塗りが可視(因果マスク)</p>
        </div>

        <!-- FFN: トークンごとに独立 -->
        <div v-else-if="flowCur.visual === 'ffn'" class="tf-cols">
          <div v-for="t in TOKENS" :key="t" class="tf-col ffn">
            <span class="tf-tok">{{ t }}</span>
            <span class="tf-vec mono">d→4d→d</span>
          </div>
        </div>

        <!-- ×N 段 -->
        <div v-else-if="flowCur.visual === 'stack'" class="tf-stack">
          <div class="tf-block mono">ブロック 12</div>
          <div class="tf-block dots mono">⋮</div>
          <div class="tf-block mono">ブロック 2</div>
          <div class="tf-block mono">ブロック 1</div>
        </div>

        <!-- logits -->
        <div v-else class="tf-logits">
          <div class="tf-logits-head">「…猫 が 好き だ」の次のトークン</div>
          <div v-for="b in logitBars" :key="b.tok" class="tf-bar-row">
            <span class="tf-bar-tok mono">{{ b.tok }}</span>
            <span class="tf-bar-track"><span class="tf-bar-fill" :style="{ width: b.p * 100 + '%' }"></span></span>
            <span class="tf-bar-p mono">{{ (b.p * 100).toFixed(0) }}%</span>
          </div>
        </div>
      </div>
    </template>

    <!-- 3系統のマスク -->
    <template v-else>
      <div class="tf-gridwrap">
        <div class="tf-grid" :style="{ '--n': 4 }">
          <span class="tf-cell head"></span>
          <span v-for="t in maskCur.tokens" :key="'h' + t" class="tf-cell head mono">{{ t }}</span>
          <template v-for="(row, q) in maskCur.tokens" :key="'r' + q">
            <span class="tf-cell head mono">{{ row }}</span>
            <span
              v-for="(v, k) in maskCur.visible[q]"
              :key="q + '-' + k"
              class="tf-cell"
              :class="{ vis: v }"
            ></span>
          </template>
        </div>
        <p class="tf-gridnote">行 = 情報を集める位置、列 = 見てよい相手。代表: {{ maskCur.models }}</p>
      </div>
    </template>

    <p class="tf-note">{{ note }}</p>

    <div class="tf-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="tf-nav mono">{{ at + 1 }} / {{ frameCount }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">1手すすめる</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="tf-legend">
      ブロックの中でトークンの列をまたぐのは attention だけで、FFN はトークンごとに独立に働く。
      そして decoder-only / encoder-only / encoder-decoder の違いは、attention のマスクと学習目標の違いに還元できる。
      この 2 点がこの後の部品章すべての前提になる。
    </p>
  </DemoShell>
</template>

<style scoped>
.tf-stages {
  margin-top: 14px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.tf-stage {
  font-size: 12.5px;
  color: var(--vp-c-text-3);
  padding: 4px 10px;
  border-left: 3px solid transparent;
  border-radius: 0;
}
.tf-stage.done {
  color: var(--vp-c-text-2);
}
.tf-stage.on {
  color: var(--vp-c-text-1);
  font-weight: 700;
  border-left-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-bg-soft);
}
.tf-stage-n {
  display: inline-block;
  width: 18px;
  color: var(--vp-c-text-3);
  font-size: 11px;
}
.tf-panel {
  margin-top: 12px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
  padding: 14px;
  min-height: 132px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.tf-cols {
  display: flex;
  gap: 10px;
}
.tf-col {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  background-color: var(--vp-c-bg);
  padding: 8px 12px;
}
.tf-col.ffn {
  border-color: var(--vp-c-brand-1);
}
.tf-tok {
  font-size: 14px;
  font-weight: 700;
}
.tf-vec,
.tf-pos {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.tf-pos {
  color: var(--vp-c-brand-1);
  font-weight: 700;
}
.tf-gridwrap {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
}
.tf-grid {
  display: grid;
  grid-template-columns: repeat(calc(var(--n) + 1), 34px);
  gap: 3px;
}
.tf-cell {
  height: 26px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  background-color: var(--vp-c-bg);
}
.tf-cell.head {
  border: none;
  background: none;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.tf-cell.vis {
  background-color: var(--vp-c-brand-soft);
  border-color: var(--vp-c-brand-1);
}
.tf-gridnote {
  margin: 0;
  font-size: 11.5px;
  color: var(--vp-c-text-3);
}
.tf-stack {
  display: flex;
  flex-direction: column;
  gap: 4px;
  width: 180px;
}
.tf-block {
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  background-color: var(--vp-c-bg);
  text-align: center;
  font-size: 12px;
  color: var(--vp-c-text-2);
  padding: 5px 0;
}
.tf-block.dots {
  border-style: dashed;
  color: var(--vp-c-text-3);
}
.tf-logits {
  width: 100%;
  max-width: 380px;
}
.tf-logits-head {
  font-size: 12px;
  color: var(--vp-c-text-2);
  margin-bottom: 8px;
}
.tf-bar-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 4px 0;
}
.tf-bar-tok {
  width: 52px;
  font-size: 12px;
  text-align: right;
}
.tf-bar-track {
  flex: 1;
  height: 12px;
  background-color: var(--vp-c-bg);
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.tf-bar-fill {
  display: block;
  height: 100%;
  background-color: var(--vp-c-brand-1);
}
.tf-bar-p {
  width: 40px;
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.tf-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.tf-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
  flex-wrap: wrap;
}
.tf-nav {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.tf-legend {
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
