<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// 行列積の可視化。結果のセルにホバー/クリックすると、対応する A の行と B の列が光り、
// その内積が結果セルの値であることが見える。
const A = [
  [1, 2, 3],
  [4, 5, 6],
];
const B = [
  [1, 2],
  [3, 4],
  [5, 6],
];

// C = A·B (2x3 · 3x2 = 2x2)
const C = computed(() => {
  const out: number[][] = [];
  for (let i = 0; i < 2; i++) {
    out[i] = [];
    for (let j = 0; j < 2; j++) {
      let s = 0;
      for (let k = 0; k < 3; k++) s += A[i][k] * B[k][j];
      out[i][j] = s;
    }
  }
  return out;
});

const sel = ref<{ i: number; j: number } | null>({ i: 0, j: 0 });

const detail = computed(() => {
  if (!sel.value) return "";
  const { i, j } = sel.value;
  const terms = [0, 1, 2].map((k) => `${A[i][k]}×${B[k][j]}`);
  return `C[${i}][${j}] = ${terms.join(" + ")} = ${C.value[i][j]}`;
});
</script>

<template>
  <DemoShell title="行列積 A · B" badge="2×3 · 3×2 = 2×2" badge-tone="neutral">
    <div class="mm-layout">
      <div class="mm-mat">
        <span class="mm-name a">A</span>
        <div class="mm-grid" :style="{ gridTemplateColumns: `repeat(3, 1fr)` }">
          <span
            v-for="(v, idx) in A.flat()"
            :key="idx"
            class="mm-cell"
            :class="{ hi: sel && Math.floor(idx / 3) === sel.i }"
          >{{ v }}</span>
        </div>
      </div>
      <span class="mm-op">×</span>
      <div class="mm-mat">
        <span class="mm-name b">B</span>
        <div class="mm-grid" :style="{ gridTemplateColumns: `repeat(2, 1fr)` }">
          <span
            v-for="(v, idx) in B.flat()"
            :key="idx"
            class="mm-cell"
            :class="{ hi: sel && idx % 2 === sel.j }"
          >{{ v }}</span>
        </div>
      </div>
      <span class="mm-op">=</span>
      <div class="mm-mat">
        <span class="mm-name c">C</span>
        <div class="mm-grid" :style="{ gridTemplateColumns: `repeat(2, 1fr)` }">
          <span
            v-for="(v, idx) in C.flat()"
            :key="idx"
            class="mm-cell result"
            :class="{ sel: sel && sel.i === Math.floor(idx / 2) && sel.j === idx % 2 }"
            @mouseenter="sel = { i: Math.floor(idx / 2), j: idx % 2 }"
          >{{ v }}</span>
        </div>
      </div>
    </div>

    <p class="mm-detail">{{ detail }}</p>
    <p class="mm-note">
      結果の1マスは「A の行」と「B の列」の内積。C のマスにマウスを乗せると、
      掛け合わされる行(青)と列(緑)が光る。Transformer の計算はほぼこれの積み重ね。
    </p>
  </DemoShell>
</template>

<style scoped>
.mm-layout {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-top: 8px;
}
.mm-mat {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.mm-name {
  font-weight: 700;
  font-size: 13px;
  font-family: var(--vp-font-family-mono);
}
.mm-name.a {
  color: var(--vp-c-brand-1);
}
.mm-name.b {
  color: var(--vp-c-green-1);
}
.mm-name.c {
  color: var(--vp-c-text-1);
}
.mm-grid {
  display: grid;
  gap: 3px;
}
.mm-cell {
  width: 34px;
  height: 34px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 5px;
  background-color: var(--vp-c-default-soft);
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
}
.mm-cell.hi {
  background-color: color-mix(in srgb, var(--vp-c-brand-1) 25%, transparent);
}
.mm-cell.result {
  cursor: pointer;
}
.mm-cell.result.sel {
  background-color: var(--vp-c-brand-1);
  color: var(--vp-c-bg);
  font-weight: 700;
}
.mm-op {
  font-size: 18px;
  color: var(--vp-c-text-3);
}
.mm-detail {
  margin: 14px 0 0;
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
  color: var(--vp-c-brand-1);
}
.mm-note {
  margin: 8px 0 0;
  font-size: 12px;
  color: var(--vp-c-text-2);
}
</style>
