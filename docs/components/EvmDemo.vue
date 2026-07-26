<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// chain/evm(Go)の counter コントラクトをブラウザで実行する。
// スタック・gas 消費・ストレージ書き換えを 1 命令ずつ。gas を絞ると out-of-gas で
// 「状態は巻き戻るが gas は消費される」が見える。

interface Instr {
  name: string;
  gas: number;
  imm?: number;
}
interface Frame {
  idx: number; // 実行し終えた命令の位置(-1 = 実行前)
  op: string;
  stack: number[];
  gasLeft: number;
  gasUsed: number;
  storage: number; // 実行中の作業ストレージ slot0
  status: "run" | "ok" | "oog";
  note: string;
}

// counter: storage[0] を +1 する。
const program: Instr[] = [
  { name: "PUSH1", gas: 3, imm: 0 }, // slot
  { name: "SLOAD", gas: 50 },        // storage[0]
  { name: "PUSH1", gas: 3, imm: 1 }, // +1
  { name: "ADD", gas: 3 },
  { name: "PUSH1", gas: 3, imm: 0 }, // slot
  { name: "SSTORE", gas: 100 },
  { name: "STOP", gas: 0 },
];
const INITIAL_STORAGE = 0;

function simulate(gasLimit: number): Frame[] {
  const frames: Frame[] = [];
  let stack: number[] = [];
  let storage = INITIAL_STORAGE;
  let gasLeft = gasLimit;

  frames.push({
    idx: -1,
    op: "(実行前)",
    stack: [],
    gasLeft,
    gasUsed: 0,
    storage: INITIAL_STORAGE,
    status: "run",
    note: `gas ${gasLimit} を渡して counter を呼ぶ。storage[0] は ${INITIAL_STORAGE}。命令ごとに gas を引きながら実行する`,
  });

  for (let i = 0; i < program.length; i++) {
    const ins = program[i];
    if (gasLeft < ins.gas) {
      // out-of-gas: 残り gas を使い切り、状態は巻き戻る(storage は初期値のまま報告)。
      gasLeft = 0;
      frames.push({
        idx: i,
        op: ins.name,
        stack: [...stack],
        gasLeft: 0,
        gasUsed: gasLimit,
        storage: INITIAL_STORAGE,
        status: "oog",
        note: `${ins.name} に必要な gas ${ins.gas} が残っていない → out-of-gas。状態は巻き戻り storage[0] は ${INITIAL_STORAGE} のまま。だが gas ${gasLimit} は消費される(計算させた対価)`,
      });
      return frames;
    }
    gasLeft -= ins.gas;

    let note = "";
    switch (ins.name) {
      case "PUSH1":
        stack.push(ins.imm!);
        note = `定数 ${ins.imm} をスタックに積む`;
        break;
      case "SLOAD": {
        stack.pop(); // slot
        stack.push(storage);
        note = `storage[0] = ${storage} を読んで積む(SLOAD は 50 gas と高め)`;
        break;
      }
      case "ADD": {
        const b = stack.pop()!;
        const a = stack.pop()!;
        stack.push(a + b);
        note = `上位 2 つを足す → ${a + b}`;
        break;
      }
      case "SSTORE": {
        stack.pop(); // slot
        const val = stack.pop()!;
        storage = val;
        note = `storage[0] = ${val} を書き込む(SSTORE は 100 gas。永続状態を増やすので最も高い)`;
        break;
      }
      case "STOP":
        note = `正常終了。storage[0] = ${storage} が確定した`;
        break;
    }

    frames.push({
      idx: i,
      op: ins.name,
      stack: [...stack],
      gasLeft,
      gasUsed: gasLimit - gasLeft,
      storage,
      status: ins.name === "STOP" ? "ok" : "run",
      note,
    });
  }
  return frames;
}

const gasLimit = ref(1000);
const frames = computed(() => simulate(gasLimit.value));
const at = ref(0);
const cur = computed(() => frames.value[at.value]);

function setGas(v: number) {
  gasLimit.value = v;
  at.value = 0;
}
const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < frames.value.length - 1);
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
  at.value = frames.value.length - 1;
}

const done = computed(() => at.value === frames.value.length - 1);
const badge = computed(() => {
  if (!done.value) return `step ${at.value + 1}`;
  return cur.value.status === "oog" ? "out-of-gas → 巻き戻し" : "storage[0] = 1 確定";
});
const badgeTone = computed<"ok" | "ng" | "neutral">(() => {
  if (!done.value) return "neutral";
  return cur.value.status === "oog" ? "ng" : "ok";
});

// 逆アセンブル行の表示。
function line(ins: Instr, i: number): string {
  const pad = String(i).padStart(2, " ");
  return ins.imm !== undefined ? `${pad}: ${ins.name} ${ins.imm}` : `${pad}: ${ins.name}`;
}
</script>

<template>
  <DemoShell title="evm(スタックVM + gas + リバート)" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: gasLimit === 1000 }" @click="setGas(1000)">gas 1000（十分）</span>
        <span class="sd-seg-opt" :class="{ on: gasLimit === 40 }" @click="setGas(40)">gas 40（不足）</span>
      </span>
      <span class="spacer" />
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">1命令すすめる</button>
    </div>

    <div class="ev-grid">
      <div class="ev-panel">
        <div class="ev-panel-head">counter コントラクト（逆アセンブル）</div>
        <div class="ev-code">
          <div
            v-for="(ins, i) in program"
            :key="i"
            class="ev-line"
            :class="{ active: cur.idx === i }"
          >
            {{ line(ins, i) }}
          </div>
        </div>
      </div>

      <div class="ev-right">
        <div class="ev-panel">
          <div class="ev-panel-head">スタック</div>
          <div class="ev-stack">
            <div v-if="cur.stack.length === 0" class="ev-empty">(空)</div>
            <div v-for="(v, i) in [...cur.stack].reverse()" :key="i" class="ev-word" :class="{ top: i === 0 }">
              {{ v }}
            </div>
          </div>
        </div>

        <div class="ev-metrics">
          <div class="ev-metric">
            <span class="ev-m-k">gas 残り</span>
            <span class="ev-m-v mono" :class="{ warn: cur.gasLeft === 0 && cur.status === 'oog' }">{{ cur.gasLeft }}</span>
          </div>
          <div class="ev-metric">
            <span class="ev-m-k">gas 消費</span>
            <span class="ev-m-v mono">{{ cur.gasUsed }}</span>
          </div>
          <div class="ev-metric">
            <span class="ev-m-k">storage[0]</span>
            <span class="ev-m-v mono">{{ cur.storage }}</span>
          </div>
        </div>
      </div>
    </div>

    <div class="ev-action" :class="cur.status">
      <span class="ev-op">{{ cur.op }}</span>
      <span v-if="cur.status === 'ok'" class="ev-res ok">正常終了</span>
      <span v-else-if="cur.status === 'oog'" class="ev-res ng">out-of-gas（巻き戻し）</span>
    </div>
    <p class="ev-note">{{ cur.note }}</p>

    <div class="ev-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="ev-count">{{ at + 1 }} / {{ frames.length }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="ev-legend">
      各命令は gas を消費し、尽きれば <strong>out-of-gas</strong> で止まる。失敗すると
      <strong>storage への書き込みは巻き戻る</strong>が、<strong>gas は消費されたまま</strong>——
      信頼できない相手のコードを走らせても、こちらは損をしない仕掛け。
    </p>
  </DemoShell>
</template>

<style scoped>
.ev-grid {
  display: grid;
  grid-template-columns: 1.1fr 1fr;
  gap: 12px;
  margin-top: 16px;
}
@media (max-width: 560px) {
  .ev-grid {
    grid-template-columns: 1fr;
  }
}
.ev-panel {
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg);
  overflow: hidden;
}
.ev-panel-head {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  padding: 8px 12px;
  border-bottom: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.ev-code {
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.ev-line {
  font-family: var(--vp-font-family-mono);
  font-size: 12.5px;
  padding: 3px 8px;
  color: var(--vp-c-text-2);
  white-space: pre;
  border-left: 2px solid transparent;
}
.ev-line.active {
  background-color: var(--vp-c-brand-soft);
  color: var(--vp-c-text-1);
  border-left-color: var(--vp-c-brand-1);
  font-weight: 600;
}
.ev-right {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.ev-stack {
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-height: 96px;
}
.ev-empty {
  color: var(--vp-c-text-3);
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
}
.ev-word {
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
  font-weight: 600;
  padding: 4px 10px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  background-color: var(--vp-c-bg-soft);
  color: var(--vp-c-text-1);
}
.ev-word.top {
  border-color: var(--vp-c-brand-1);
  color: var(--vp-c-brand-1);
}
.ev-metrics {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr;
  gap: 8px;
}
.ev-metric {
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 8px 10px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  background-color: var(--vp-c-bg);
}
.ev-m-k {
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.ev-m-v {
  font-size: 16px;
  font-weight: 700;
  color: var(--vp-c-text-1);
}
.ev-m-v.mono {
  font-family: var(--vp-font-family-mono);
}
.ev-m-v.warn {
  color: var(--vp-c-danger-1);
}
.ev-action {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 12px;
  padding: 10px 12px;
  border: 1px solid var(--vp-c-divider);
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  background-color: var(--vp-c-bg);
  font-size: 13px;
}
.ev-action.ok {
  border-left-color: var(--vp-c-green-1);
}
.ev-action.oog {
  border-left-color: var(--vp-c-danger-1);
}
.ev-op {
  font-family: var(--vp-font-family-mono);
  font-weight: 700;
  color: var(--vp-c-text-1);
  flex: 1;
}
.ev-res {
  font-size: 11px;
  font-weight: 700;
  padding: 2px 8px;
  border-radius: 3px;
}
.ev-res.ok {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.ev-res.ng {
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
}
.ev-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  font-family: var(--vp-font-family-mono);
}
.ev-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 14px;
}
.ev-count {
  font-size: 12px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.ev-legend {
  margin: 14px 0 0;
  padding-top: 12px;
  border-top: 1px solid var(--vp-c-divider);
  font-size: 12px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
}
</style>
