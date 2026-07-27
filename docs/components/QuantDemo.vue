<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// llm/quant(Go)をブラウザに移植。
// 「格子と誤差」: 重み分布を int8/int4/int2 の格子に丸め、誤差を可視化。
// 「メモリ」: モデルサイズ×ビット数で必要メモリを計算し、GPU に載るか判定。

// 固定の重み分布(0対称)
const WEIGHTS = [-0.82, -0.61, -0.44, -0.3, -0.12, 0, 0.09, 0.25, 0.41, 0.58, 0.77, 0.95];

const bitsOptions = [8, 4, 2];
const bitPick = ref(0);

function quantize(x: number[], bits: number) {
  const qmax = (1 << (bits - 1)) - 1;
  const maxAbs = Math.max(...x.map(Math.abs));
  const scale = maxAbs / qmax || 1;
  return x.map((v) => {
    const code = Math.max(-qmax, Math.min(qmax, Math.round(v / scale)));
    return { orig: v, code, deq: code * scale };
  });
}

const bits = computed(() => bitsOptions[bitPick.value]);
const quantized = computed(() => quantize(WEIGHTS, bits.value));
const maxErr = computed(() => Math.max(...quantized.value.map((q) => Math.abs(q.orig - q.deq))));
const levels = computed(() => (1 << bits.value)); // 格子点の数

// メモリ
const modelSizes = [
  { name: "7B", params: 7e9 },
  { name: "13B", params: 13e9 },
  { name: "70B", params: 70e9 },
  { name: "405B", params: 405e9 },
];
const sizePick = ref(2);
const memBitsOptions = [32, 16, 8, 4];
const GPU_VRAM = 24; // GB, コンシューマGPU目安

function memGB(params: number, bits: number) {
  return (params * bits) / 8 / 1e9;
}

const modes = [
  { key: "grid", label: "格子と誤差" },
  { key: "mem", label: "メモリ" },
] as const;
const mode = ref<"grid" | "mem">("grid");

const curSize = computed(() => modelSizes[sizePick.value]);

const note = computed(() => {
  if (mode.value === "grid") {
    return `${bits.value}bit = ${levels.value} 個の格子点。max|誤差| = ${maxErr.value.toFixed(4)}。ビットを減らすと格子が粗くなり、丸め誤差が増える。int8 はほぼ無損失、int4 で実用、int2 は品質が急落する`;
  }
  const fp16 = memGB(curSize.value.params, 16);
  const int4 = memGB(curSize.value.params, 4);
  return `${curSize.value.name} は fp16 で ${fp16.toFixed(0)}GB、4bit で ${int4.toFixed(0)}GB。VRAM ${GPU_VRAM}GB の GPU には ${fp16 > GPU_VRAM ? "fp16 では載らないが" : "fp16 でも載り"}、4bit なら ${int4 <= GPU_VRAM ? "収まる" : "まだ載らない(複数GPUが要る)"}`;
});
</script>

<template>
  <DemoShell title="量子化(格子とメモリ)" badge-tone="neutral" :badge="mode === 'grid' ? `int${bits}` : curSize.name">
    <div class="sd-controls">
      <span class="sd-seg">
        <span v-for="m in modes" :key="m.key" class="sd-seg-opt" :class="{ on: mode === m.key }" @click="mode = m.key">{{ m.label }}</span>
      </span>
      <span class="spacer" />
      <span v-if="mode === 'grid'" class="sd-seg">
        <span v-for="(b, i) in bitsOptions" :key="b" class="sd-seg-opt" :class="{ on: bitPick === i }" @click="bitPick = i">int{{ b }}</span>
      </span>
      <span v-else class="sd-seg">
        <span v-for="(s, i) in modelSizes" :key="s.name" class="sd-seg-opt" :class="{ on: sizePick === i }" @click="sizePick = i">{{ s.name }}</span>
      </span>
    </div>

    <!-- 格子と誤差 -->
    <div v-if="mode === 'grid'" class="qz-grid">
      <div class="qz-grid-head">
        <span>元の重み → 復元値(誤差)</span>
        <span class="mono">{{ levels }} 格子点</span>
      </div>
      <div v-for="(q, i) in quantized" :key="i" class="qz-row">
        <span class="qz-orig mono">{{ q.orig >= 0 ? " " : "" }}{{ q.orig.toFixed(2) }}</span>
        <span class="qz-bar-wrap">
          <span class="qz-mid"></span>
          <span class="qz-dot orig" :style="{ left: (q.orig + 1) / 2 * 100 + '%' }"></span>
          <span class="qz-dot deq" :style="{ left: (q.deq + 1) / 2 * 100 + '%' }"></span>
        </span>
        <span class="qz-err mono" :class="{ big: Math.abs(q.orig - q.deq) > 0.05 }">±{{ Math.abs(q.orig - q.deq).toFixed(3) }}</span>
      </div>
    </div>

    <!-- メモリ -->
    <div v-else class="qz-mem">
      <div v-for="b in memBitsOptions" :key="b" class="qz-mem-row">
        <span class="qz-mem-label mono">{{ b === 32 ? 'fp32' : b === 16 ? 'fp16' : 'int' + b }}</span>
        <span class="qz-mem-track">
          <span class="qz-mem-fill" :class="{ over: memGB(curSize.params, b) > GPU_VRAM }" :style="{ width: Math.min(memGB(curSize.params, b) / memGB(curSize.params, 32) * 100, 100) + '%' }"></span>
          <span class="qz-vram" :style="{ left: Math.min(GPU_VRAM / memGB(curSize.params, 32) * 100, 100) + '%' }"></span>
        </span>
        <span class="qz-mem-val mono">{{ memGB(curSize.params, b).toFixed(0) }}GB</span>
      </div>
      <p class="qz-vram-note">縦線 = コンシューマGPU の VRAM 目安({{ GPU_VRAM }}GB)。これを超えると単一 GPU に載らない</p>
    </div>

    <p class="qz-note">{{ note }}</p>

    <p class="qz-legend">
      量子化は連続値の重みを整数の格子に丸めるだけ。ビットを減らすほどメモリは線形に減り、
      格子は粗くなって誤差は指数的に増える。int8 はほぼ無損失、4bit が実用の主戦場。
      fp32 では GPU に載らないモデルが、4bit なら手元で動く。
    </p>
  </DemoShell>
</template>

<style scoped>
.qz-grid {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 8px 12px;
}
.qz-grid-head {
  display: flex;
  justify-content: space-between;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  padding-bottom: 8px;
  border-bottom: 1px solid var(--vp-c-divider);
  margin-bottom: 6px;
}
.qz-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 2px 0;
}
.qz-orig {
  width: 48px;
  font-size: 11.5px;
  color: var(--vp-c-text-2);
  text-align: right;
}
.qz-bar-wrap {
  flex: 1;
  position: relative;
  height: 16px;
}
.qz-mid {
  position: absolute;
  left: 0;
  right: 0;
  top: 50%;
  height: 1px;
  background-color: var(--vp-c-divider);
}
.qz-dot {
  position: absolute;
  top: 50%;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  transform: translate(-50%, -50%);
}
.qz-dot.orig {
  background-color: var(--vp-c-text-3);
}
.qz-dot.deq {
  background-color: var(--vp-c-brand-1);
  border: 1px solid var(--vp-c-bg);
}
.qz-err {
  width: 54px;
  font-size: 11px;
  color: var(--vp-c-text-3);
  text-align: right;
}
.qz-err.big {
  color: var(--vp-c-warning-1);
  font-weight: 700;
}
.qz-mem {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 12px;
  background-color: var(--vp-c-bg-soft);
}
.qz-mem-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 6px 0;
}
.qz-mem-label {
  width: 46px;
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.qz-mem-track {
  flex: 1;
  position: relative;
  height: 15px;
  background-color: var(--vp-c-bg);
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.qz-mem-fill {
  display: block;
  height: 100%;
  background-color: var(--vp-c-green-1);
}
.qz-mem-fill.over {
  background-color: var(--vp-c-danger-1);
}
.qz-vram {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 2px;
  background-color: var(--vp-c-text-1);
}
.qz-mem-val {
  width: 52px;
  text-align: right;
  font-size: 11.5px;
}
.qz-vram-note {
  margin: 8px 0 0;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.qz-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.qz-legend {
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
