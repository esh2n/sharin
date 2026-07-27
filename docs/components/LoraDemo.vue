<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// llm/lora(Go)の会計とマージ性質をブラウザで可視化。
// 「パラメータ削減」: 行列サイズ×ランクでフル微調整と LoRA の学習量を比較。
// 「学習と恒等性」: B=0 の初期から補正が育ち、マージしても出力不変。

// --- パラメータ削減 ---
const dims = [1024, 2048, 4096];
const ranks = [4, 8, 16, 64];
const dimPick = ref(2);
const rankPick = ref(1);

const d = computed(() => dims[dimPick.value]);
const r = computed(() => ranks[rankPick.value]);
const fullParams = computed(() => d.value * d.value);
const loraParams = computed(() => 2 * d.value * r.value);
const pct = computed(() => (loraParams.value / fullParams.value) * 100);
const fmtN = (n: number) => (n >= 1e6 ? (n / 1e6).toFixed(1) + "M" : n >= 1000 ? (n / 1000).toFixed(0) + "k" : "" + n);

// --- 学習と恒等性 ---
// スカラー近似: base 出力 = 1.0、補正 = step 進むごとに B が育つ
const STEPS = 6;
const trainAt = ref(0);
const merged = ref(false);

const baseOut = 1.0;
const delta = computed(() => (trainAt.value / STEPS) * 0.4); // 最大 0.4 ズレる
const loraOut = computed(() => baseOut + delta.value);

const trainNote = computed(() => {
  if (merged.value) {
    return "学習後、A·B を base に足し込んで 1 枚の行列にした。出力は Forward(base+補正を別々に計算)と完全一致し、行列サイズは base と同じ。推論時の追加コストはゼロ";
  }
  if (trainAt.value === 0) {
    return "初期状態: B = 0 なので補正は 0。出力は base 単独と完全に一致する。学習を始めた瞬間にモデルの挙動を変えないのが LoRA の要件";
  }
  return `学習 ${trainAt.value}/${STEPS}: B が育ち、補正 ${delta.value.toFixed(3)} が base に足される。base の重み W は凍結したまま、細い A・B だけが更新される`;
});

const modes = [
  { key: "params", label: "パラメータ削減" },
  { key: "train", label: "学習と恒等性" },
] as const;
const mode = ref<"params" | "train">("params");
function setMode(m: "params" | "train") {
  mode.value = m;
  trainAt.value = 0;
  merged.value = false;
}

const note = computed(() =>
  mode.value === "params"
    ? `${d.value}×${d.value} の重みを rank ${r.value} で適応。フル微調整は ${fmtN(fullParams.value)} 個を学習するが、LoRA は A·B の ${fmtN(loraParams.value)} 個(${pct.value.toFixed(2)}%)だけ。凍結した W には勾配が要らないので、省くのはこの差分のメモリ`
    : trainNote.value,
);

const canPrev = computed(() => mode.value === "train" && trainAt.value > 0 && !merged.value);
const canNext = computed(() => mode.value === "train" && (trainAt.value < STEPS || !merged.value));
function stepNext() {
  if (trainAt.value < STEPS) trainAt.value++;
  else merged.value = true;
}
function stepPrev() {
  if (merged.value) merged.value = false;
  else if (trainAt.value > 0) trainAt.value--;
}
function first() { trainAt.value = 0; merged.value = false; }
function last() { trainAt.value = STEPS; merged.value = true; }

const badge = computed(() =>
  mode.value === "params" ? `${pct.value.toFixed(2)}%` : merged.value ? "マージ済" : `step ${trainAt.value}/${STEPS}`,
);
</script>

<template>
  <DemoShell title="LoRA(低ランク適応)" badge-tone="neutral" :badge="badge">
    <div class="sd-controls">
      <span class="sd-seg">
        <span v-for="m in modes" :key="m.key" class="sd-seg-opt" :class="{ on: mode === m.key }" @click="setMode(m.key)">{{ m.label }}</span>
      </span>
      <span v-if="mode === 'params'" class="spacer" />
      <span v-if="mode === 'params'" class="sd-seg">
        <span v-for="(rk, i) in ranks" :key="rk" class="sd-seg-opt" :class="{ on: rankPick === i }" @click="rankPick = i">r={{ rk }}</span>
      </span>
    </div>

    <!-- パラメータ削減 -->
    <div v-if="mode === 'params'" class="lo-params">
      <div class="lo-dim-seg">
        重み行列:
        <span v-for="(dm, i) in dims" :key="dm" class="lo-dim" :class="{ on: dimPick === i }" @click="dimPick = i">{{ dm }}×{{ dm }}</span>
      </div>
      <div class="lo-bar-row">
        <span class="lo-bar-label">フル微調整</span>
        <span class="lo-track"><span class="lo-fill full" style="width: 100%"></span></span>
        <span class="lo-bar-val mono">{{ fmtN(fullParams) }}</span>
      </div>
      <div class="lo-bar-row">
        <span class="lo-bar-label">LoRA (A·B)</span>
        <span class="lo-track"><span class="lo-fill lora" :style="{ width: Math.max(pct, 0.4) + '%' }"></span></span>
        <span class="lo-bar-val mono">{{ fmtN(loraParams) }}</span>
      </div>
      <p class="lo-sub">学習するパラメータの比。LoRA は全体の {{ pct.toFixed(2) }}% だけ</p>
    </div>

    <!-- 学習と恒等性 -->
    <div v-else class="lo-train">
      <div class="lo-formula mono">y = x·<span class="lo-w">W</span> <span class="lo-frozen">(凍結)</span> + <span class="lo-ab" :class="{ active: trainAt > 0 }">A·B</span> <span class="lo-frozen">(学習)</span></div>
      <div class="lo-out-row">
        <span class="lo-out-label">base 単独</span>
        <span class="lo-track"><span class="lo-fill full" :style="{ width: baseOut / 1.5 * 100 + '%' }"></span></span>
        <span class="lo-bar-val mono">{{ baseOut.toFixed(3) }}</span>
      </div>
      <div class="lo-out-row">
        <span class="lo-out-label">{{ merged ? 'マージ後' : 'LoRA 出力' }}</span>
        <span class="lo-track">
          <span class="lo-fill lora" :style="{ width: loraOut / 1.5 * 100 + '%' }"></span>
          <span class="lo-marker" :style="{ left: baseOut / 1.5 * 100 + '%' }"></span>
        </span>
        <span class="lo-bar-val mono">{{ loraOut.toFixed(3) }}</span>
      </div>
      <p class="lo-sub">縦線 = base の出力位置。補正が育つとそこからズレ、マージしても値は変わらない</p>
    </div>

    <p class="lo-note">{{ note }}</p>

    <div class="lo-foot" v-if="mode === 'train'">
      <button class="sd-btn" :disabled="trainAt === 0 && !merged" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="stepPrev">◀</button>
      <span class="lo-nav mono">{{ merged ? 'merged' : trainAt + '/' + STEPS }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="stepNext">▶</button>
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="stepNext">{{ trainAt < STEPS ? '1ステップ学習' : 'マージ' }}</button>
      <button class="sd-btn" :disabled="merged" @click="last">最後へ</button>
    </div>

    <p class="lo-legend">
      LoRA は元の重みを凍結し、隣に低ランクの A·B を足して学習する。学習量は全体の 1% 未満で、
      B=0 初期化により始めは base と恒等。学習後は A·B を W に畳み込めば 1 枚に戻り、
      推論の追加コストはゼロ。量子化と組めば(QLoRA)単一 GPU で大規模微調整ができる。
    </p>
  </DemoShell>
</template>

<style scoped>
.lo-params,
.lo-train {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 14px;
  background-color: var(--vp-c-bg-soft);
}
.lo-dim-seg {
  font-size: 12px;
  color: var(--vp-c-text-2);
  margin-bottom: 12px;
}
.lo-dim {
  display: inline-block;
  font-family: var(--vp-font-family-mono);
  font-size: 11.5px;
  padding: 2px 8px;
  margin-left: 4px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  cursor: pointer;
  color: var(--vp-c-text-2);
}
.lo-dim.on {
  border-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-brand-soft);
  color: var(--vp-c-brand-1);
  font-weight: 700;
}
.lo-bar-row,
.lo-out-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 6px 0;
}
.lo-bar-label,
.lo-out-label {
  width: 92px;
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.lo-track {
  flex: 1;
  position: relative;
  height: 14px;
  background-color: var(--vp-c-bg);
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.lo-fill {
  display: block;
  height: 100%;
}
.lo-fill.full { background-color: var(--vp-c-text-3); }
.lo-fill.lora { background-color: var(--vp-c-brand-1); }
.lo-marker {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 2px;
  background-color: var(--vp-c-text-1);
}
.lo-bar-val {
  width: 56px;
  text-align: right;
  font-size: 11.5px;
}
.lo-sub {
  margin: 8px 0 0;
  font-size: 11.5px;
  color: var(--vp-c-text-3);
}
.lo-formula {
  font-size: 13px;
  margin-bottom: 12px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--vp-c-divider);
}
.lo-w { color: var(--vp-c-text-3); font-weight: 700; }
.lo-frozen { font-size: 10.5px; color: var(--vp-c-text-3); }
.lo-ab { color: var(--vp-c-text-3); font-weight: 700; }
.lo-ab.active { color: var(--vp-c-brand-1); }
.lo-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.lo-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
  flex-wrap: wrap;
}
.lo-nav {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.lo-legend {
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
