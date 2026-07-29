<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// distributed/causal(Go)を移植。届くことと渡すことを分ける。

type Vec = Record<string, number>;
interface Msg {
  from: string;
  seq: number;
  body: string;
  deps: Vec;
}
interface NodeState {
  seen: Vec;
  hold: Msg[];
  done: Msg[];
}

const NAMES = ["a", "b", "c"];
const causal = ref(true);
const step = ref(0);

const nodes = ref<Record<string, NodeState>>({});
const sent = ref<Record<string, Msg>>({});
const log = ref<string[]>([]);

function deliverable(m: Msg, seen: Vec): boolean {
  if ((m.deps[m.from] ?? 0) !== (seen[m.from] ?? 0) + 1) return false;
  if (!causal.value) return true;
  for (const k of Object.keys(m.deps)) {
    if (k === m.from) continue;
    if (m.deps[k] > (seen[k] ?? 0)) return false;
  }
  return true;
}
function drain(n: NodeState): number {
  let moved = 0;
  for (;;) {
    const i = n.hold.findIndex((m) => deliverable(m, n.seen));
    if (i < 0) return moved;
    const [m] = n.hold.splice(i, 1);
    n.seen = { ...n.seen, [m.from]: m.deps[m.from] };
    n.done.push(m);
    moved++;
  }
}
function known(n: NodeState, m: Msg): boolean {
  if ((m.deps[m.from] ?? 0) <= (n.seen[m.from] ?? 0)) return true;
  return n.hold.some((h) => h.from === m.from && h.seq === m.seq);
}

function broadcast(from: string, body: string): Msg {
  const n = nodes.value[from];
  n.seen = { ...n.seen, [from]: (n.seen[from] ?? 0) + 1 };
  const m: Msg = { from, seq: n.seen[from], body, deps: { ...n.seen } };
  n.done.push(m);
  log.value = [...log.value, `${from} が「${body}」を流した`];
  return m;
}
function deliver(to: string, m: Msg) {
  const n = nodes.value[to];
  if (known(n, m)) {
    log.value = [...log.value, `${to} に「${m.body}」が届いたが、すでに渡し済み`];
    return;
  }
  n.hold.push(m);
  const moved = drain(n);
  log.value = [
    ...log.value,
    moved === 0
      ? `${to} に「${m.body}」が届いたが、まだ渡せない(預かり ${n.hold.length} 件)`
      : moved > 1
        ? `${to} に「${m.body}」が届いて、預かりもまとめて ${moved} 件渡した`
        : `${to} に「${m.body}」が届いて、そのまま渡した`,
  ];
  nodes.value = { ...nodes.value };
}

const SCRIPT: { label: string; run: () => void }[] = [
  {
    label: "a が「質問」を流す。b には届くが、c には届かない",
    run: () => {
      sent.value.q = broadcast("a", "質問");
      deliver("b", sent.value.q);
    },
  },
  {
    label: "b が「質問」を見てから「回答」を流す。a には届くが、c には届かない",
    run: () => {
      sent.value.r = broadcast("b", "回答");
      deliver("a", sent.value.r);
    },
  },
  {
    label: "c に「回答」が先に届く。ここが分かれ目になる",
    run: () => deliver("c", sent.value.r),
  },
  {
    label: "c に「質問」が遅れて届く",
    run: () => deliver("c", sent.value.q),
  },
];

function reset() {
  nodes.value = Object.fromEntries(NAMES.map((n) => [n, { seen: {}, hold: [], done: [] }]));
  sent.value = {};
  log.value = [];
  step.value = 0;
}
function next() {
  if (step.value >= SCRIPT.length) return;
  SCRIPT[step.value].run();
  step.value++;
}
function all() {
  while (step.value < SCRIPT.length) next();
}
function flip() {
  const s = step.value;
  causal.value = !causal.value;
  reset();
  for (let i = 0; i < s; i++) next();
}
// 最初から分かれ目(3手目)の状態にしておく。空の表から始めても何も見えない。
reset();
for (let i = 0; i < 3; i++) next();

const badge = computed(() => `${causal.value ? "原因の順" : "送り手ごとの順"} ・ ${step.value} / ${SCRIPT.length}`);
const cOrder = computed(() => nodes.value["c"].done.map((m) => m.body));
const verdict = computed(() => {
  if (step.value < 3) return "まだ c には何も届いていない。3 手目で「回答」が先に届く";
  if (step.value === 3)
    return causal.value
      ? "c は「回答」を預かったまま渡していない。載っている依存が自分の見たものに収まっていないので、原因が来るまで待つ"
      : "c は「回答」を渡してしまった。b からの1件目なので、送り手ごとの順では止められない";
  return causal.value
    ? `c が渡した順は ${cOrder.value.join(" → ")}。届いた順とは逆だが、原因の順にはなっている`
    : `c が渡した順は ${cOrder.value.join(" → ")}。回答が質問より先に見えている`;
});
</script>

<template>
  <DemoShell title="因果順序の配送" :badge="badge" :badge-tone="causal ? 'ok' : 'ng'">
    <div class="cz-actions">
      <button class="sd-btn sd-btn--primary" :disabled="step >= SCRIPT.length" @click="next">1 手進める</button>
      <button class="sd-btn" @click="all">最後まで</button>
      <span class="cz-gap" />
      <button class="sd-btn" @click="flip">
        配る順: {{ causal ? "原因の順" : "送り手ごとの順" }}
      </button>
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <p class="cz-step mono">
      {{ step < SCRIPT.length ? `次の手 — ${SCRIPT[step].label}` : "筋書きは終わり" }}
    </p>

    <div class="cz-grid">
      <div v-for="n in NAMES" :key="n" class="cz-node">
        <div class="cz-name mono">{{ n }}</div>
        <div class="cz-label">渡した順</div>
        <div class="cz-chips">
          <span v-for="(m, i) in nodes[n].done" :key="i" class="cz-chip">
            <i>{{ i + 1 }}</i>{{ m.body }}
          </span>
          <span v-if="!nodes[n].done.length" class="cz-empty mono">まだ無い</span>
        </div>
        <div class="cz-label">預かり</div>
        <div class="cz-chips">
          <span v-for="(m, i) in nodes[n].hold" :key="i" class="cz-chip held">{{ m.body }}</span>
          <span v-if="!nodes[n].hold.length" class="cz-empty mono">無し</span>
        </div>
        <div class="cz-seen mono">
          渡し終えた数 {{ NAMES.map((k) => `${k}:${nodes[n].seen[k] ?? 0}`).join(" ") }}
        </div>
      </div>
    </div>

    <div class="cz-verdict" :class="causal ? 'ok' : 'bad'">{{ verdict }}</div>

    <div class="cz-log">
      <div v-for="(l, i) in log.slice(-3)" :key="i" class="cz-log-line mono">{{ l }}</div>
      <div v-if="!log.length" class="cz-log-line mono empty">(まだ何も起きていない)</div>
    </div>

    <p class="cz-legend">
      各メッセージには「流した時点で送り手が見ていたもの」が載っている。受け取り側は、それが自分の
      渡し終えた数に収まっているかを見て、収まっていなければ預かりに置く。収まった瞬間に、預かりも
      まとめて渡る。送り手ごとの順に切り替えると、別の相手からの原因を見ないので、回答が質問より
      先に渡ってしまう。
    </p>
  </DemoShell>
</template>

<style scoped>
.cz-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.cz-gap {
  flex: 1;
  min-width: 8px;
}
.cz-step {
  margin: 12px 0 0;
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.cz-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 8px;
  margin-top: 10px;
}
.cz-node {
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
  padding: 9px 10px;
}
.cz-name {
  font-size: 13px;
  font-weight: 600;
  padding-bottom: 5px;
  border-bottom: 1px solid var(--vp-c-divider);
}
.cz-label {
  margin-top: 7px;
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.cz-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 3px;
  min-height: 20px;
}
.cz-chip {
  font-size: 11px;
  padding: 1px 7px;
  border: 1px solid var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.cz-chip i {
  font-style: normal;
  font-size: 8px;
  vertical-align: super;
  opacity: 0.7;
  margin-right: 2px;
}
.cz-chip.held {
  border-color: var(--vp-c-warning-1);
  color: var(--vp-c-warning-1);
  background-color: var(--vp-c-warning-soft);
}
.cz-empty {
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.cz-seen {
  margin-top: 7px;
  padding-top: 5px;
  border-top: 1px solid var(--vp-c-divider);
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.cz-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  font-weight: 600;
  line-height: 1.6;
  background-color: var(--vp-c-bg-soft);
}
.cz-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.cz-verdict.bad {
  border-left-color: var(--vp-c-warning-1);
  color: var(--vp-c-warning-1);
  background-color: var(--vp-c-warning-soft);
}
.cz-log {
  margin-top: 10px;
  border-top: 1px solid var(--vp-c-divider);
  padding-top: 6px;
  min-height: 36px;
}
.cz-log-line {
  font-size: 10px;
  color: var(--vp-c-text-2);
  padding: 1px 0;
}
.cz-log-line.empty {
  color: var(--vp-c-text-3);
}
.cz-legend {
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
