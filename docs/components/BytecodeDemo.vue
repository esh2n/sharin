<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// foundations/bytecode(Go)の「ソース→バイトコード→スタックマシン実行」をブラウザで。
// コンパイル結果(逆アセンブル)は各プリセットに固定データとして持ち、VM(スタックマシン)を
// JS で忠実に回す。スタックの伸び縮みと if のジャンプが見えるのが主眼。

type Val = { t: "int"; v: number } | { t: "bool"; v: boolean } | { t: "null" };
interface Ins {
  op: string;
  arg?: number; // OpConstant/Set/Get の番号、または(レイアウト後)ジャンプの飛び先オフセット
  to?: string; // ジャンプの飛び先ラベル(レイアウトで arg=オフセットに解決)
  label?: string; // この命令の位置に付くラベル
  offset?: number; // 命令列内のバイトオフセット(レイアウトで計算)
  index?: number;
}
interface Preset {
  key: string;
  src: string;
  consts: Val[];
  code: Ins[];
}

function i(n: number): Val {
  return { t: "int", v: n };
}
// 3 バイト命令(オペランド 2 バイト)か、1 バイト命令か。
const WIDE = new Set(["OpConstant", "OpJump", "OpJumpNotTruthy", "OpSetGlobal", "OpGetGlobal"]);

const PRESETS: Preset[] = [
  {
    key: "算術",
    src: "1 + 2 * 3",
    consts: [i(1), i(2), i(3)],
    code: [
      { op: "OpConstant", arg: 0 },
      { op: "OpConstant", arg: 1 },
      { op: "OpConstant", arg: 2 },
      { op: "OpMul" },
      { op: "OpAdd" },
      { op: "OpPop" },
    ],
  },
  {
    key: "if 分岐",
    src: "if (1 < 2) { 10 } else { 20 }",
    // 1 < 2 は「2 を積む → 1 を積む → OpGreater(左>右=1>...ではなく入れ替え済み)」
    consts: [i(2), i(1), i(10), i(20)],
    code: [
      { op: "OpConstant", arg: 0 },
      { op: "OpConstant", arg: 1 },
      { op: "OpGreater" },
      { op: "OpJumpNotTruthy", to: "ELSE" },
      { op: "OpConstant", arg: 2 },
      { op: "OpJump", to: "END" },
      { op: "OpConstant", arg: 3, label: "ELSE" },
      { op: "OpPop", label: "END" },
    ],
  },
  {
    key: "let 束縛",
    src: "let x = 5;\nlet y = x * 2;\ny",
    consts: [i(5), i(2)],
    code: [
      { op: "OpConstant", arg: 0 },
      { op: "OpSetGlobal", arg: 0 },
      { op: "OpGetGlobal", arg: 0 },
      { op: "OpConstant", arg: 1 },
      { op: "OpMul" },
      { op: "OpSetGlobal", arg: 1 },
      { op: "OpGetGlobal", arg: 1 },
      { op: "OpPop" },
    ],
  },
];

// レイアウト: 各命令のバイトオフセットを計算し、ジャンプのラベルをオフセットへ解決する。
function layout(p: Preset): Preset {
  const labels: Record<string, number> = {};
  let off = 0;
  p.code.forEach((ins, idx) => {
    ins.offset = off;
    ins.index = idx;
    if (ins.label) labels[ins.label] = off;
    off += WIDE.has(ins.op) ? 3 : 1;
  });
  for (const ins of p.code) if (ins.to !== undefined) ins.arg = labels[ins.to];
  return p;
}
PRESETS.forEach(layout);

interface Frame {
  ip: number; // ハイライトする命令 index(-1 = 実行終了)
  stack: Val[];
  globals: Record<number, Val>;
  note: string;
  result: string | null;
}

function fmt(v: Val): string {
  if (v.t === "int") return String(v.v);
  if (v.t === "bool") return v.v ? "true" : "false";
  return "null";
}
function truthy(v: Val): boolean {
  if (v.t === "bool") return v.v;
  if (v.t === "null") return false;
  return true;
}

// VM を回してフレーム列を作る。各命令の実行「前」に状態を1枚撮る + 実行して次へ。
function trace(p: Preset): Frame[] {
  const offToIndex: Record<number, number> = {};
  p.code.forEach((ins) => (offToIndex[ins.offset as number] = ins.index as number));
  const stack: Val[] = [];
  const globals: Record<number, Val> = {};
  let ip = 0;
  let lastPopped: Val | null = null;
  const frames: Frame[] = [];
  const snap = (note: string, hl: number, result: string | null) =>
    frames.push({
      ip: hl,
      stack: stack.map((v) => ({ ...v })),
      globals: { ...globals },
      note,
      result,
    });
  const push = (v: Val) => stack.push(v);
  const pop = (): Val => stack.pop() ?? { t: "null" };
  const bin = (f: (a: number, b: number) => number) => {
    const r = pop();
    const l = pop();
    push({ t: "int", v: f((l as { v: number }).v, (r as { v: number }).v) });
  };
  const cmp = (f: (a: number, b: number) => boolean) => {
    const r = pop();
    const l = pop();
    push({ t: "bool", v: f((l as { v: number }).v, (r as { v: number }).v) });
  };

  let guard = 0;
  while (ip < p.code.length && guard++ < 200) {
    const ins = p.code[ip];
    snap(describe(ins, p), ip, null);
    let nextIp = ip + 1;
    switch (ins.op) {
      case "OpConstant":
        push(p.consts[ins.arg as number]);
        break;
      case "OpAdd":
        bin((a, b) => a + b);
        break;
      case "OpSub":
        bin((a, b) => a - b);
        break;
      case "OpMul":
        bin((a, b) => a * b);
        break;
      case "OpDiv":
        bin((a, b) => Math.trunc(a / b));
        break;
      case "OpGreater":
        cmp((a, b) => a > b);
        break;
      case "OpEqual":
        cmp((a, b) => a === b);
        break;
      case "OpNotEqual":
        cmp((a, b) => a !== b);
        break;
      case "OpPop":
        lastPopped = pop();
        break;
      case "OpJump":
        nextIp = offToIndex[ins.arg as number];
        break;
      case "OpJumpNotTruthy":
        if (!truthy(pop())) nextIp = offToIndex[ins.arg as number];
        break;
      case "OpSetGlobal":
        globals[ins.arg as number] = pop();
        break;
      case "OpGetGlobal":
        push(globals[ins.arg as number]);
        break;
    }
    ip = nextIp;
  }
  snap(`実行終了 — 結果は ${lastPopped ? fmt(lastPopped) : "—"}`, -1, lastPopped ? fmt(lastPopped) : "—");
  return frames;
}

// 命令の人間向け説明。
function describe(ins: Ins, p: Preset): string {
  switch (ins.op) {
    case "OpConstant":
      return `定数 ${fmt(p.consts[ins.arg as number])} をスタックに積む`;
    case "OpAdd":
      return "上位2つを pop して足し、結果を積む";
    case "OpSub":
      return "上位2つを pop して引く";
    case "OpMul":
      return "上位2つを pop して掛ける";
    case "OpDiv":
      return "上位2つを pop して割る";
    case "OpGreater":
      return "上位2つを比較(左 > 右)して真偽を積む";
    case "OpEqual":
      return "上位2つが等しいか";
    case "OpPop":
      return "スタック先頭を捨てる(式文の後始末)";
    case "OpJump":
      return `無条件に ${pad(ins.arg as number)} へ飛ぶ`;
    case "OpJumpNotTruthy":
      return `pop した値が偽なら ${pad(ins.arg as number)} へ飛ぶ`;
    case "OpSetGlobal":
      return `pop した値をグローバル #${ins.arg} に束縛(let)`;
    case "OpGetGlobal":
      return `グローバル #${ins.arg} を積む`;
    default:
      return ins.op;
  }
}
function pad(n: number): string {
  return String(n).padStart(4, "0");
}
function operandText(ins: Ins): string {
  if (ins.arg === undefined) return "";
  return String(ins.arg);
}

const preset = ref(0);
const frames = computed(() => trace(PRESETS[preset.value]));
const at = ref(0);
const cur = computed(() => frames.value[at.value]);
const code = computed(() => PRESETS[preset.value].code);

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

const badge = computed(() => (cur.value.result ? `結果 ${cur.value.result}` : `ip → ${pad(code.value[cur.value.ip]?.offset ?? 0)}`));
const badgeTone = computed<"ok" | "neutral">(() => (cur.value.result ? "ok" : "neutral"));
// スタックは下が底・上が先頭になるよう逆順表示。
const stackTopDown = computed(() => [...cur.value.stack].reverse());
const globalEntries = computed(() =>
  Object.entries(cur.value.globals).map(([k, v]) => ({ k: Number(k), v: fmt(v) })),
);
</script>

<template>
  <DemoShell title="bytecode(コンパイラ + スタックマシン)" :badge="badge" :badge-tone="badgeTone">
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

    <div class="bc-src">
      <span class="bc-src-label">source</span>
      <code class="bc-src-code">{{ PRESETS[preset].src }}</code>
    </div>

    <div class="bc-grid">
      <!-- 逆アセンブル: コンパイル結果の命令列。現在の ip をハイライト -->
      <div class="bc-panel">
        <div class="bc-panel-head">bytecode(逆アセンブル)</div>
        <div class="bc-code">
          <div v-for="(ins, idx) in code" :key="idx" class="bc-row" :class="{ cur: cur.ip === idx }">
            <span class="bc-off">{{ pad(ins.offset ?? 0) }}</span>
            <span class="bc-op">{{ ins.op }}</span>
            <span class="bc-operand">{{ operandText(ins) }}</span>
          </div>
        </div>
      </div>

      <!-- スタックマシンの状態: スタック + グローバル + 定数プール -->
      <div class="bc-panel">
        <div class="bc-panel-head">stack(先頭が上)</div>
        <div class="bc-stack">
          <transition-group name="bc-pop">
            <div v-for="(v, k) in stackTopDown" :key="k" class="bc-cell" :class="v.t">{{ fmt(v) }}</div>
          </transition-group>
          <div v-if="cur.stack.length === 0" class="bc-empty">(空)</div>
        </div>

        <div class="bc-sub">
          <div class="bc-sub-title">globals</div>
          <div v-if="globalEntries.length" class="bc-globals">
            <span v-for="g in globalEntries" :key="g.k" class="bc-gchip">#{{ g.k }} = {{ g.v }}</span>
          </div>
          <div v-else class="bc-empty">—</div>
        </div>

        <div class="bc-sub">
          <div class="bc-sub-title">constants</div>
          <div class="bc-consts">
            <span v-for="(c, k) in PRESETS[preset].consts" :key="k" class="bc-cchip">#{{ k }}: {{ fmt(c) }}</span>
          </div>
        </div>
      </div>
    </div>

    <p class="bc-note">{{ cur.note }}</p>

    <div class="bc-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="bc-count">{{ at + 1 }} / {{ frames.length }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>
  </DemoShell>
</template>

<style scoped>
.bc-src {
  display: flex;
  align-items: baseline;
  gap: 10px;
  margin-top: 16px;
  padding: 10px 12px;
  border: 1px solid var(--vp-c-divider);
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  background-color: var(--vp-c-bg);
}
.bc-src-label {
  font-size: 11px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-3);
  flex: 0 0 auto;
}
.bc-src-code {
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
  color: var(--vp-c-text-1);
  white-space: pre-wrap;
  background: none;
  padding: 0;
}
.bc-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-top: 12px;
}
@media (max-width: 560px) {
  .bc-grid {
    grid-template-columns: 1fr;
  }
}
.bc-panel {
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg);
  overflow: hidden;
}
.bc-panel-head {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  padding: 8px 12px;
  border-bottom: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.bc-code {
  padding: 6px 0;
}
.bc-row {
  display: grid;
  grid-template-columns: 48px 1fr 40px;
  align-items: center;
  gap: 8px;
  padding: 3px 12px;
  font-family: var(--vp-font-family-mono);
  font-size: 12.5px;
  border-left: 3px solid transparent;
}
.bc-row.cur {
  border-left-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-brand-soft);
}
.bc-off {
  color: var(--vp-c-text-3);
}
.bc-op {
  color: var(--vp-c-text-1);
  font-weight: 600;
}
.bc-row.cur .bc-op {
  color: var(--vp-c-brand-1);
}
.bc-operand {
  color: var(--vp-c-text-2);
  text-align: right;
}
.bc-stack {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px 12px;
  min-height: 96px;
  justify-content: flex-end;
}
.bc-cell {
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
.bc-cell.bool {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
  border-color: transparent;
}
.bc-cell.null {
  color: var(--vp-c-text-3);
  font-style: italic;
}
.bc-empty {
  font-size: 12px;
  color: var(--vp-c-text-3);
  font-style: italic;
  text-align: center;
  padding: 4px 0;
}
.bc-sub {
  padding: 10px 12px;
  border-top: 1px solid var(--vp-c-divider);
}
.bc-sub-title {
  font-size: 11px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-3);
  margin-bottom: 6px;
}
.bc-globals,
.bc-consts {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.bc-gchip,
.bc-cchip {
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  padding: 2px 8px;
  border-radius: 4px;
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-1);
}
.bc-cchip {
  color: var(--vp-c-text-2);
}
.bc-note {
  margin: 14px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  font-family: var(--vp-font-family-mono);
}
.bc-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
}
.bc-count {
  font-size: 12px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.bc-pop-enter-from {
  opacity: 0;
  transform: translateY(-6px);
}
.bc-pop-enter-active {
  transition: opacity 0.15s, transform 0.15s;
}
</style>
