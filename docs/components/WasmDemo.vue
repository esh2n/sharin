<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// foundations/wasm(Go)の実行モデルをブラウザで。値スタック + 制御スタック +
// 構造化制御フロー(block/loop/if と br)。loop への br で制御が本体先頭へ戻る
// = ループが回る、という WASM 独特の仕組みを追う。

interface Ins {
  op: string;
  imm?: number;
  end?: number;
  else?: number;
  indent?: number;
}
interface Ctrl {
  op: string;
  target: number;
  end: number;
}
interface Preset {
  key: string;
  desc: string;
  localNames: string[];
  locals: number[];
  code: Ins[];
}

const PRESETS: Preset[] = [
  {
    key: "sum_to(5)",
    desc: "block/loop/br で 5+4+…+1",
    localNames: ["n", "acc"],
    locals: [5, 0],
    code: [
      { op: "i32.const", imm: 0 },
      { op: "local.set", imm: 1 },
      { op: "block" },
      { op: "loop" },
      { op: "local.get", imm: 0 },
      { op: "i32.eqz" },
      { op: "br_if", imm: 1 },
      { op: "local.get", imm: 1 },
      { op: "local.get", imm: 0 },
      { op: "i32.add" },
      { op: "local.set", imm: 1 },
      { op: "local.get", imm: 0 },
      { op: "i32.const", imm: 1 },
      { op: "i32.sub" },
      { op: "local.set", imm: 0 },
      { op: "br", imm: 0 },
      { op: "end" },
      { op: "end" },
      { op: "local.get", imm: 1 },
    ],
  },
  {
    key: "add(3,4)",
    desc: "スタックで加算",
    localNames: ["a", "b"],
    locals: [3, 4],
    code: [
      { op: "local.get", imm: 0 },
      { op: "local.get", imm: 1 },
      { op: "i32.add" },
    ],
  },
  {
    key: "max(9,4)",
    desc: "if/else で分岐",
    localNames: ["a", "b"],
    locals: [9, 4],
    code: [
      { op: "local.get", imm: 0 },
      { op: "local.get", imm: 1 },
      { op: "i32.gt_s" },
      { op: "if" },
      { op: "local.get", imm: 0 },
      { op: "else" },
      { op: "local.get", imm: 1 },
      { op: "end" },
    ],
  },
];

// block/loop/if を end/else に結びつけ、表示用のインデントも計算する。
function layout(code: Ins[]): Ins[] {
  const c = code.map((x) => ({ ...x }));
  const st: number[] = [];
  c.forEach((ins, i) => {
    if (ins.op === "block" || ins.op === "loop" || ins.op === "if") {
      st.push(i);
      if (ins.op === "if") ins.else = -1;
    } else if (ins.op === "else") {
      c[st[st.length - 1]].else = i;
    } else if (ins.op === "end") {
      const t = st.pop();
      if (t !== undefined) {
        c[t].end = i;
        if (c[t].op === "if" && c[t].else === -1) c[t].else = i;
      }
    }
  });
  let d = 0;
  c.forEach((ins) => {
    if (ins.op === "end" || ins.op === "else") d = Math.max(0, d - 1);
    ins.indent = d;
    if (ins.op === "block" || ins.op === "loop" || ins.op === "if" || ins.op === "else") d++;
  });
  return c;
}

interface Frame {
  ip: number;
  stack: number[];
  locals: number[];
  control: { op: string }[];
  note: string;
  done: boolean;
  result: number | null;
}

function insText(ins: Ins): string {
  if (ins.imm === undefined) return ins.op;
  return `${ins.op} ${ins.imm}`;
}
function describe(ins: Ins): string {
  switch (ins.op) {
    case "i32.const":
      return `定数 ${ins.imm} を積む`;
    case "local.get":
      return `ローカル #${ins.imm} を積む`;
    case "local.set":
      return `pop してローカル #${ins.imm} に格納`;
    case "i32.add":
      return "上位2つを pop して加算";
    case "i32.sub":
      return "上位2つを pop して減算";
    case "i32.mul":
      return "上位2つを pop して乗算";
    case "i32.eqz":
      return "先頭が 0 か(==0 で 1)";
    case "i32.gt_s":
      return "上位2つを比較(左 > 右)";
    case "block":
      return "block に入る(br の脱出先ラベル)";
    case "loop":
      return "loop に入る(br の継続先=本体先頭)";
    case "if":
      return "pop した条件で then/else へ";
    case "else":
      return "then を終え、end へ飛ぶ";
    case "end":
      return "ブロックを抜ける(制御スタックを畳む)";
    case "br":
      return `br ${ins.imm}: ${ins.imm} 個外側のブロックへ`;
    case "br_if":
      return `br_if ${ins.imm}: 先頭が真なら ${ins.imm} 個外側へ`;
    default:
      return ins.op;
  }
}

function trace(p: Preset): Frame[] {
  const code = layout(p.code);
  const stack: number[] = [];
  const control: Ctrl[] = [];
  const locals = [...p.locals];
  let ip = 0;
  const frames: Frame[] = [];
  const snap = (note: string, done: boolean, result: number | null) =>
    frames.push({
      ip,
      stack: [...stack],
      locals: [...locals],
      control: control.map((c) => ({ op: c.op })),
      note,
      done,
      result,
    });
  const pop = () => stack.pop() ?? 0;
  const branch = (label: number): number => {
    const idx = control.length - 1 - label;
    const target = control[idx];
    if (target.op === "loop") {
      control.length = idx + 1;
      return target.target;
    }
    control.length = idx;
    return target.target;
  };

  let guard = 0;
  while (ip < code.length && guard++ < 500) {
    const ins = code[ip];
    snap(describe(ins), false, null);
    switch (ins.op) {
      case "i32.const":
        stack.push(ins.imm as number);
        ip++;
        break;
      case "local.get":
        stack.push(locals[ins.imm as number]);
        ip++;
        break;
      case "local.set":
        locals[ins.imm as number] = pop();
        ip++;
        break;
      case "i32.add": {
        const b = pop(), a = pop();
        stack.push(a + b);
        ip++;
        break;
      }
      case "i32.sub": {
        const b = pop(), a = pop();
        stack.push(a - b);
        ip++;
        break;
      }
      case "i32.mul": {
        const b = pop(), a = pop();
        stack.push(a * b);
        ip++;
        break;
      }
      case "i32.gt_s": {
        const b = pop(), a = pop();
        stack.push(a > b ? 1 : 0);
        ip++;
        break;
      }
      case "i32.eqz":
        stack.push(pop() === 0 ? 1 : 0);
        ip++;
        break;
      case "block":
        control.push({ op: "block", target: (ins.end as number) + 1, end: ins.end as number });
        ip++;
        break;
      case "loop":
        control.push({ op: "loop", target: ip + 1, end: ins.end as number });
        ip++;
        break;
      case "if": {
        const c = pop();
        control.push({ op: "if", target: (ins.end as number) + 1, end: ins.end as number });
        if (c !== 0) ip++;
        else if (ins.else !== ins.end) ip = (ins.else as number) + 1;
        else ip = ins.end as number;
        break;
      }
      case "else":
        ip = control[control.length - 1].end;
        break;
      case "end":
        if (control.length) control.pop();
        ip++;
        break;
      case "br":
        ip = branch(ins.imm as number);
        break;
      case "br_if":
        ip = pop() !== 0 ? branch(ins.imm as number) : ip + 1;
        break;
      default:
        ip++;
    }
  }
  const result = stack.length ? stack[stack.length - 1] : null;
  snap("実行終了", true, result);
  return frames;
}

const preset = ref(0);
const frames = computed(() => trace(PRESETS[preset.value]));
const at = ref(0);
const cur = computed(() => frames.value[at.value]);
const code = computed(() => layout(PRESETS[preset.value].code));

function selectPreset(idx: number) {
  preset.value = idx;
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

const badge = computed(() => (cur.value.done ? `結果 ${cur.value.result}` : `ip ${cur.value.ip}`));
const badgeTone = computed<"ok" | "neutral">(() => (cur.value.done ? "ok" : "neutral"));
const stackTopDown = computed(() => [...cur.value.stack].reverse());
const controlTopDown = computed(() => [...cur.value.control].reverse());
const localNames = computed(() => PRESETS[preset.value].localNames);
</script>

<template>
  <DemoShell title="wasm(最小 WebAssembly VM)" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <span class="sd-seg">
        <span
          v-for="(p, idx) in PRESETS"
          :key="p.key"
          class="sd-seg-opt"
          :class="{ on: preset === idx }"
          @click="selectPreset(idx)"
          >{{ p.key }}</span
        >
      </span>
      <span class="spacer" />
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">1手すすめる</button>
    </div>

    <div class="wa-src">{{ PRESETS[preset].desc }}</div>

    <div class="wa-grid">
      <div class="wa-panel">
        <div class="wa-panel-head">命令列(構造化)</div>
        <div class="wa-code">
          <div v-for="(ins, idx) in code" :key="idx" class="wa-row" :class="{ cur: cur.ip === idx }">
            <span class="wa-idx">{{ idx }}</span>
            <span class="wa-ins" :style="{ paddingLeft: (ins.indent ?? 0) * 14 + 'px' }">{{ insText(ins) }}</span>
          </div>
        </div>
      </div>

      <div class="wa-panel">
        <div class="wa-panel-head">value stack(先頭が上)</div>
        <div class="wa-stack">
          <div v-for="(v, k) in stackTopDown" :key="k" class="wa-cell">{{ v }}</div>
          <div v-if="cur.stack.length === 0" class="wa-empty">(空)</div>
        </div>

        <div class="wa-sub">
          <div class="wa-sub-title">control stack(構造化制御フロー)</div>
          <div v-if="controlTopDown.length" class="wa-ctrl">
            <span v-for="(c, k) in controlTopDown" :key="k" class="wa-cchip" :class="c.op">{{ c.op }}</span>
          </div>
          <div v-else class="wa-empty">(空)</div>
        </div>

        <div class="wa-sub">
          <div class="wa-sub-title">locals</div>
          <div class="wa-locals">
            <span v-for="(v, k) in cur.locals" :key="k" class="wa-lchip">{{ localNames[k] }}=#{{ k }}: {{ v }}</span>
          </div>
        </div>
      </div>
    </div>

    <p class="wa-note">{{ cur.note }}</p>

    <div class="wa-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="wa-count">{{ at + 1 }} / {{ frames.length }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="wa-legend">
      <code>loop</code> への <code>br</code> は本体先頭へ戻り(継続)、<code>block</code>/<code>if</code> への
      <code>br</code> は end の先へ抜ける(脱出)。任意ジャンプ無しでループも分岐も書ける = 実行前に検証できる。
    </p>
  </DemoShell>
</template>

<style scoped>
.wa-src {
  margin-top: 16px;
  padding: 8px 12px;
  border: 1px solid var(--vp-c-divider);
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  font-size: 12px;
  color: var(--vp-c-text-2);
  background-color: var(--vp-c-bg);
}
.wa-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-top: 12px;
}
@media (max-width: 560px) {
  .wa-grid {
    grid-template-columns: 1fr;
  }
}
.wa-panel {
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg);
  overflow: hidden;
}
.wa-panel-head {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  padding: 8px 12px;
  border-bottom: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.wa-code {
  padding: 6px 0;
  max-height: 340px;
  overflow-y: auto;
}
.wa-row {
  display: grid;
  grid-template-columns: 28px 1fr;
  align-items: center;
  gap: 8px;
  padding: 2px 12px;
  font-family: var(--vp-font-family-mono);
  font-size: 12.5px;
  border-left: 3px solid transparent;
}
.wa-row.cur {
  border-left-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-brand-soft);
}
.wa-idx {
  color: var(--vp-c-text-3);
  text-align: right;
}
.wa-ins {
  color: var(--vp-c-text-1);
}
.wa-row.cur .wa-ins {
  color: var(--vp-c-brand-1);
  font-weight: 600;
}
.wa-stack {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px 12px;
  min-height: 70px;
  justify-content: flex-end;
}
.wa-cell {
  padding: 5px 10px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
  font-weight: 600;
  text-align: center;
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-1);
}
.wa-empty {
  font-size: 12px;
  color: var(--vp-c-text-3);
  font-style: italic;
  padding: 2px 0;
}
.wa-sub {
  padding: 10px 12px;
  border-top: 1px solid var(--vp-c-divider);
}
.wa-sub-title {
  font-size: 11px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-3);
  margin-bottom: 6px;
}
.wa-ctrl {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.wa-cchip {
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  padding: 2px 8px;
  border-radius: 3px;
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-1);
}
.wa-cchip.loop {
  background-color: var(--vp-c-brand-1);
  color: var(--vp-c-bg);
}
.wa-locals {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.wa-lchip {
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  padding: 2px 8px;
  border-radius: 4px;
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-2);
}
.wa-note {
  margin: 14px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  font-family: var(--vp-font-family-mono);
}
.wa-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
}
.wa-count {
  font-size: 12px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.wa-legend {
  margin: 14px 0 0;
  padding-top: 12px;
  border-top: 1px solid var(--vp-c-divider);
  font-size: 12px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
}
</style>
