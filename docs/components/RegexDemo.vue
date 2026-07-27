<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// foundations/regex(Go)の Thompson 構成 + NFA シミュレーションをブラウザに移植。
// 固定パターン (a|b)*abb の NFA を組み、選んだ入力を 1 文字ずつ進めて、
// 「今いる状態集合」がどう動くかを見せる。集合を並行に進める = バックトラッキング不要。

// --- 最小 NFA エンジン(Go の nfa.go の移植) ---
type Edge = { to: number; eps?: boolean; any?: boolean; ch?: string };
type Node =
  | { k: "lit"; ch: string }
  | { k: "alt"; l: Node; r: Node }
  | { k: "cat"; l: Node; r: Node }
  | { k: "star"; x: Node };

// (a|b)*abb の AST を直接組む。
const lit = (ch: string): Node => ({ k: "lit", ch });
const pattern: Node = {
  k: "cat",
  l: {
    k: "cat",
    l: { k: "cat", l: { k: "star", x: { k: "alt", l: lit("a"), r: lit("b") } }, r: lit("a") },
    r: lit("b"),
  },
  r: lit("b"),
};

const trans: Edge[][] = [];
function newState(): number {
  trans.push([]);
  return trans.length - 1;
}
function link(from: number, e: Edge) {
  trans[from].push(e);
}
function build(n: Node): { s: number; a: number } {
  if (n.k === "lit") {
    const s = newState(), a = newState();
    link(s, { to: a, ch: n.ch });
    return { s, a };
  }
  if (n.k === "alt") {
    const s = newState(), a = newState();
    const l = build(n.l), r = build(n.r);
    link(s, { to: l.s, eps: true });
    link(s, { to: r.s, eps: true });
    link(l.a, { to: a, eps: true });
    link(r.a, { to: a, eps: true });
    return { s, a };
  }
  if (n.k === "cat") {
    const l = build(n.l), r = build(n.r);
    link(l.a, { to: r.s, eps: true });
    return { s: l.s, a: r.a };
  }
  // star
  const s = newState(), a = newState();
  const x = build(n.x);
  link(s, { to: x.s, eps: true });
  link(s, { to: a, eps: true });
  link(x.a, { to: x.s, eps: true });
  link(x.a, { to: a, eps: true });
  return { s, a };
}
const root = build(pattern);
const START = root.s;
const ACCEPT = root.a;

function epsClosure(states: number[]): number[] {
  const seen = new Set(states);
  const stack = [...states];
  while (stack.length) {
    const s = stack.pop()!;
    for (const e of trans[s]) {
      if (e.eps && !seen.has(e.to)) {
        seen.add(e.to);
        stack.push(e.to);
      }
    }
  }
  return [...seen].sort((x, y) => x - y);
}
function stepChar(states: number[], c: string): number[] {
  const next = new Set<number>();
  for (const s of states) {
    for (const e of trans[s]) {
      if (e.eps) continue;
      if (e.any || e.ch === c) next.add(e.to);
    }
  }
  return epsClosure([...next]);
}

interface Frame {
  consumed: string;
  rest: string;
  active: number[];
  hasAccept: boolean;
  note: string;
}

function framesFor(input: string): Frame[] {
  const out: Frame[] = [];
  let active = epsClosure([START]);
  out.push({
    consumed: "",
    rest: input,
    active,
    hasAccept: active.includes(ACCEPT),
    note: "開始状態の ε 閉包。まだ 1 文字も読んでいないのに、複数の状態に同時にいる",
  });
  for (let i = 0; i < input.length; i++) {
    active = stepChar(active, input[i]);
    out.push({
      consumed: input.slice(0, i + 1),
      rest: input.slice(i + 1),
      active,
      hasAccept: active.includes(ACCEPT),
      note:
        active.length === 0
          ? "どの状態にもいられなくなった。ここで不一致が確定する"
          : `'${input[i]}' を読んで全状態を並行に進めた。今いるのは ${active.length} 状態。バックトラッキングは一切していない`,
    });
  }
  const last = out[out.length - 1];
  last.note = last.active.includes(ACCEPT)
    ? "入力を読み切り、受理状態が集合に含まれる → マッチ"
    : "入力を読み切ったが、受理状態が集合に無い → 非マッチ";
  return out;
}

const inputs = [
  { label: "ababb", value: "ababb" },
  { label: "aabb", value: "aabb" },
  { label: "abab", value: "abab" },
];
const pick = ref(0);
const frames = computed(() => framesFor(inputs[pick.value].value));
const at = ref(0);
const cur = computed(() => frames.value[at.value]);

function setPick(i: number) {
  pick.value = i;
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
const matched = computed(() => cur.value.active.includes(ACCEPT) && done.value);
const badge = computed(() => {
  if (!done.value) return `step ${at.value + 1}`;
  return matched.value ? "マッチ" : "非マッチ";
});
const badgeTone = computed<"ok" | "ng" | "neutral">(() => (!done.value ? "neutral" : matched.value ? "ok" : "ng"));
</script>

<template>
  <DemoShell title="regex(NFA シミュレーション)" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <span class="rx-pat">パターン <code>(a|b)*abb</code></span>
      <span class="spacer" />
      <span class="sd-seg">
        <span v-for="(inp, i) in inputs" :key="inp.value" class="sd-seg-opt" :class="{ on: pick === i }" @click="setPick(i)">{{ inp.label }}</span>
      </span>
    </div>

    <div class="rx-tape">
      <span class="rx-consumed">{{ cur.consumed || "·" }}</span><span class="rx-rest">{{ cur.rest }}</span>
    </div>

    <div class="rx-panel">
      <div class="rx-panel-head">
        今いる NFA 状態集合
        <span class="rx-count mono">{{ cur.active.length }} states</span>
      </div>
      <div class="rx-states">
        <span v-if="cur.active.length === 0" class="rx-empty">(空 = 詰み)</span>
        <span v-for="s in cur.active" :key="s" class="rx-state mono" :class="{ acc: s === ACCEPT }">
          q{{ s }}<template v-if="s === ACCEPT"> ✓</template>
        </span>
      </div>
    </div>

    <p class="rx-note">{{ cur.note }}</p>

    <div class="rx-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="rx-nav mono">{{ at + 1 }} / {{ frames.length }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">1文字すすめる</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="rx-legend">
      正規表現を NFA に変換し、「今いる状態の集合」を丸ごと持って入力を 1 文字ずつ進める。
      複数の状態に同時にいられるので、どの分岐を選ぶかを試して戻る(バックトラッキング)必要が無い。
      だから <code>(a*)*b</code> のような式でも指数爆発せず、入力長に比例した時間で判定できる。
    </p>
  </DemoShell>
</template>

<style scoped>
.rx-pat {
  font-size: 13px;
  color: var(--vp-c-text-2);
}
.rx-tape {
  margin-top: 16px;
  padding: 12px 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
  font-family: var(--vp-font-family-mono);
  font-size: 18px;
  letter-spacing: 0.15em;
}
.rx-consumed {
  color: var(--vp-c-text-3);
  text-decoration: line-through;
}
.rx-rest {
  color: var(--vp-c-text-1);
  font-weight: 700;
}
.rx-panel {
  margin-top: 12px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.rx-panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  padding: 8px 12px;
  border-bottom: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.rx-count {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.rx-states {
  padding: 12px;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  min-height: 48px;
}
.rx-empty {
  color: var(--vp-c-danger-1);
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
}
.rx-state {
  font-size: 12px;
  padding: 3px 9px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  background-color: var(--vp-c-bg-soft);
  color: var(--vp-c-text-2);
}
.rx-state.acc {
  border-color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
  font-weight: 700;
}
.rx-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.rx-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
  flex-wrap: wrap;
}
.rx-nav {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.rx-legend {
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
