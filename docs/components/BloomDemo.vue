<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// Go 版 bloomfilter の JS ミラー。小さな m, k で「ビットが立つ様子」を見せる。
const M = 32; // ビット数(教材用に小さく)
const K = 3; // ハッシュ関数の数

// FNV-1a 32bit。
function fnv(s: string): number {
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 0x01000193) >>> 0;
  }
  return h >>> 0;
}

function positions(key: string): number[] {
  const sum = fnv(key);
  const h1 = sum & 0xffff;
  const h2 = ((sum >>> 16) | 1) & 0xffff;
  return Array.from({ length: K }, (_, i) => (h1 + i * h2) % M);
}

const bits = ref<boolean[]>(Array(M).fill(false));
const added = ref<string[]>([]);
const input = ref("apple");
const lit = ref<number[]>([]); // 直前の操作で触れたビット位置
const verdict = ref<{ text: string; kind: "add" | "yes" | "no" } | null>(null);

function add() {
  const key = input.value.trim();
  if (!key) return;
  const pos = positions(key);
  const next = [...bits.value];
  for (const p of pos) next[p] = true;
  bits.value = next;
  if (!added.value.includes(key)) added.value = [...added.value, key];
  lit.value = pos;
  verdict.value = { text: `"${key}" を追加。ビット ${pos.join(", ")} を立てた`, kind: "add" };
}

function check() {
  const key = input.value.trim();
  if (!key) return;
  const pos = positions(key);
  lit.value = pos;
  const allSet = pos.every((p) => bits.value[p]);
  if (allSet) {
    const reallyIn = added.value.includes(key);
    verdict.value = {
      text: reallyIn
        ? `"${key}" → 全ビット立っている → たぶん入っている(実際に追加済み)`
        : `"${key}" → 全ビット立っている → たぶん入っている（でも実は未追加＝偽陽性!）`,
      kind: "yes",
    };
  } else {
    verdict.value = { text: `"${key}" → 立っていないビットがある → 絶対に入っていない`, kind: "no" };
  }
}

function reset() {
  bits.value = Array(M).fill(false);
  added.value = [];
  lit.value = [];
  verdict.value = null;
}

const setCount = computed(() => bits.value.filter(Boolean).length);
</script>

<template>
  <DemoShell
    title="ブルームフィルタ"
    :badge="`${setCount}/${M} ビット / ${added.length}件`"
    badge-tone="neutral"
  >
    <div class="sd-controls">
      <input v-model="input" class="bl-input" spellcheck="false" @keyup.enter="check" />
      <button class="sd-btn sd-btn--primary" type="button" @click="add">追加</button>
      <button class="sd-btn sd-btn--primary" type="button" @click="check">あるか調べる</button>
      <span class="spacer"></span>
      <button class="sd-btn" type="button" @click="reset">リセット</button>
    </div>

    <p v-if="verdict" class="bl-verdict" :class="verdict.kind">{{ verdict.text }}</p>

    <div class="bl-bits">
      <span
        v-for="(b, i) in bits"
        :key="i"
        class="bl-bit"
        :class="{ on: b, lit: lit.includes(i) }"
        :title="`bit ${i}`"
      >
        {{ b ? 1 : 0 }}
      </span>
    </div>

    <p v-if="added.length" class="bl-added">追加済み: {{ added.join(", ") }}</p>
  </DemoShell>
</template>

<style scoped>
.bl-input {
  flex: 0 1 180px;
  padding: 7px 10px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 6px;
  background-color: var(--vp-c-bg);
  color: var(--vp-c-text-1);
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
}
.bl-verdict {
  margin: 12px 0 0;
  font-size: 13px;
  font-weight: 600;
}
.bl-verdict.add {
  color: var(--vp-c-text-2);
  font-weight: 400;
}
.bl-verdict.yes {
  color: var(--vp-c-brand-1);
}
.bl-verdict.no {
  color: var(--vp-c-green-1);
}
.bl-bits {
  display: flex;
  flex-wrap: wrap;
  gap: 3px;
  margin-top: 12px;
}
.bl-bit {
  width: 26px;
  height: 26px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 4px;
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-3);
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  transition: background-color 0.15s, box-shadow 0.15s;
}
.bl-bit.on {
  background-color: var(--vp-c-brand-1);
  color: var(--vp-c-bg);
}
.bl-bit.lit {
  box-shadow: 0 0 0 2px var(--vp-c-yellow-1);
}
.bl-added {
  margin: 12px 0 0;
  font-size: 12px;
  color: var(--vp-c-text-3);
  font-family: var(--vp-font-family-mono);
}
</style>
