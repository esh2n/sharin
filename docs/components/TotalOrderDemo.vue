<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// distributed/total(Go)を移植。係を置く方式と、全員で決める方式を切り替える。

const NAMES = ["a", "b", "c"];
type Mode = "seq" | "vote";
const mode = ref<Mode>("seq");
const step = ref(0);
const log = ref<string[]>([]);

// 係を置く方式 ---------------------------------------------------------------
interface Numbered {
  seq: number;
  body: string;
}
interface SeqNode {
  next: number;
  hold: Numbered[];
  done: Numbered[];
}
const sq = ref<Record<string, SeqNode>>({});
const assigned = ref<Numbered[]>([]);

function seqAssign(body: string): Numbered {
  const m = { seq: assigned.value.length + 1, body };
  assigned.value = [...assigned.value, m];
  log.value = [...log.value, `係が「${body}」に番号 ${m.seq} を付けた`];
  return m;
}
function seqDeliver(to: string, m: Numbered) {
  const n = sq.value[to];
  if (m.seq <= n.next || n.hold.some((h) => h.seq === m.seq)) return;
  n.hold.push(m);
  let moved = 0;
  for (;;) {
    const i = n.hold.findIndex((h) => h.seq === n.next + 1);
    if (i < 0) break;
    const [h] = n.hold.splice(i, 1);
    n.next = h.seq;
    n.done.push(h);
    moved++;
  }
  log.value = [
    ...log.value,
    moved
      ? `${to} が ${moved} 件渡した(次は ${n.next + 1})`
      : `${to} に ${m.seq} が届いたが、${n.next + 1} がまだ来ていない`,
  ];
  sq.value = { ...sq.value };
}

// 全員で決める方式 -----------------------------------------------------------
interface Msg {
  from: string;
  t: number;
  body: string;
}
interface VoteNode {
  clock: number;
  heard: Record<string, number>;
  queue: Msg[];
  done: Msg[];
}
const vt = ref<Record<string, VoteNode>>({});
const silent = ref(false); // c を黙らせる

function less(a: Msg, b: Msg) {
  return a.t !== b.t ? a.t < b.t : a.from < b.from;
}
function voteRequest(from: string, body: string): Msg {
  const n = vt.value[from];
  n.clock++;
  const m = { from, t: n.clock, body };
  n.queue.push(m);
  n.heard = { ...n.heard, [from]: m.t };
  log.value = [...log.value, `${from} が時刻 ${m.t} で「${body}」を出した`];
  return m;
}
function voteBeat(from: string): Msg {
  const n = vt.value[from];
  n.clock++;
  n.heard = { ...n.heard, [from]: n.clock };
  log.value = [...log.value, `${from} が時刻 ${n.clock} を知らせた`];
  return { from, t: n.clock, body: "" };
}
function voteDeliver(to: string, m: Msg) {
  const n = vt.value[to];
  n.clock = Math.max(n.clock, m.t) + 1;
  n.heard = { ...n.heard, [to]: n.clock, [m.from]: m.t };
  if (m.body && !n.queue.some((q) => q.from === m.from && q.t === m.t)) n.queue.push(m);
  for (;;) {
    if (!n.queue.length) break;
    n.queue.sort((x, y) => (less(x, y) ? -1 : 1));
    const head = n.queue[0];
    const ok = NAMES.every((k) => k === head.from || (n.heard[k] ?? 0) > head.t);
    if (!ok) break;
    n.queue.shift();
    n.done.push(head);
  }
  vt.value = { ...vt.value };
}
function voteRound() {
  for (const from of NAMES) {
    if (silent.value && from === "c") continue;
    const b = voteBeat(from);
    for (const to of NAMES) if (to !== from) voteDeliver(to, b);
  }
}

// 筋書き ---------------------------------------------------------------------
const SEQ_SCRIPT: { label: string; run: () => void }[] = [
  {
    label: "a が「質問」を出す。係へ向かう",
    run: () => {
      log.value = [...log.value, "a が「質問」を出した(係へ向かう)"];
    },
  },
  {
    label: "b が質問を見て「回答」を出す。こちらのほうが係に早く着く",
    run: () => {
      log.value = [...log.value, "b が質問を見てから「回答」を出した"];
    },
  },
  {
    label: "係には回答が先に届いた。番号 1 が回答、番号 2 が質問になる",
    run: () => {
      seqAssign("回答");
      seqAssign("質問");
    },
  },
  {
    label: "全員に配る",
    run: () => {
      for (const n of NAMES) for (const m of assigned.value) seqDeliver(n, m);
    },
  },
];
const VOTE_SCRIPT: { label: string; run: () => void }[] = [
  {
    label: "a が「A」、c が「C」を出す。どちらも時刻 1 になる",
    run: () => {
      vote.a = voteRequest("a", "A");
      vote.c = voteRequest("c", "C");
    },
  },
  {
    label: "互いに届く。待ち行列には並ぶが、まだ渡せない",
    run: () => {
      voteDeliver("b", vote.a!);
      voteDeliver("c", vote.a!);
      voteDeliver("a", vote.c!);
      voteDeliver("b", vote.c!);
    },
  },
  { label: "全員が今の時刻を知らせ合う", run: () => voteRound() },
  { label: "もう1巡", run: () => voteRound() },
];
const vote: { a?: Msg; c?: Msg } = {};

const SCRIPT = computed(() => (mode.value === "seq" ? SEQ_SCRIPT : VOTE_SCRIPT));

function reset() {
  sq.value = Object.fromEntries(NAMES.map((n) => [n, { next: 0, hold: [], done: [] }]));
  assigned.value = [];
  vt.value = Object.fromEntries(
    NAMES.map((n) => [n, { clock: 0, heard: {}, queue: [], done: [] }]),
  );
  vote.a = undefined;
  vote.c = undefined;
  log.value = [];
  step.value = 0;
}
function next() {
  if (step.value >= SCRIPT.value.length) return;
  SCRIPT.value[step.value].run();
  step.value++;
}
function all() {
  while (step.value < SCRIPT.value.length) next();
}
function switchTo(m: Mode) {
  mode.value = m;
  reset();
  all();
}
reset();
all();

const orders = computed(() =>
  mode.value === "seq"
    ? NAMES.map((n) => sq.value[n].done.map((m) => m.body))
    : NAMES.map((n) => vt.value[n].done.map((m) => m.body)),
);
const agreed = computed(() =>
  orders.value.every((o) => o.length === orders.value[0].length && o.every((v, i) => v === orders.value[0][i])),
);
const badge = computed(() => {
  const head = `${mode.value === "seq" ? "係を置く" : "全員で決める"} ・ ${step.value} / ${SCRIPT.value.length}`;
  if (!orders.value[0].length) return `${head} ・ まだ誰も渡していない`;
  return agreed.value ? `${head} ・ 全員一致` : `${head} ・ ばらばら`;
});
const verdict = computed(() => {
  if (mode.value === "seq") {
    if (step.value < 4) return "係に届いた順に番号が付く。送り手が出した順でも、原因の順でもない";
    return "3台とも完全に一致している。一致したまま、答えが問いより先に来ている。順がそろうことと、順が正しいことは別になる";
  }
  if (step.value < 2) return "同じ時刻で出た2件は、名前で順が決まる。a が先になる";
  if (!orders.value[1].length)
    return silent.value
      ? "c が黙っているので、c より後の時刻を誰も聞けない。全員から聞く決まりなので、1台の沈黙で全体が止まる"
      : "待ち行列には並んでいるが、まだ渡せない。先頭より後の時刻を、他の全員から聞く必要がある";
  return "全員から後の時刻を聞けたので、3台とも同じ順で渡した。係は要らないが、全員から聞くことが条件になっている";
});
</script>

<template>
  <DemoShell title="全順序放送" :badge="badge" :badge-tone="agreed && step > 0 ? 'ok' : 'neutral'">
    <div class="to-actions">
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: mode === 'seq' }" @click="switchTo('seq')">係を置く</span>
        <span class="sd-seg-opt" :class="{ on: mode === 'vote' }" @click="switchTo('vote')">全員で決める</span>
      </span>
      <span class="to-gap" />
      <button v-if="mode === 'vote'" class="sd-btn" :class="silent ? 'sd-btn--primary' : ''" @click="silent = !silent; switchTo('vote')">
        c を黙らせる: {{ silent ? "する" : "しない" }}
      </button>
      <button class="sd-btn sd-btn--primary" :disabled="step >= SCRIPT.length" @click="next">1 手進める</button>
      <button class="sd-btn" @click="reset">最初から</button>
    </div>

    <p class="to-step mono">
      {{ step < SCRIPT.length ? `次の手 — ${SCRIPT[step].label}` : "筋書きは終わり" }}
    </p>

    <div class="to-grid">
      <div v-for="(n, idx) in NAMES" :key="n" class="to-node">
        <div class="to-name mono">{{ n }}</div>
        <div class="to-label">渡した順</div>
        <div class="to-chips">
          <span v-for="(b, i) in orders[idx]" :key="i" class="to-chip"><i>{{ i + 1 }}</i>{{ b }}</span>
          <span v-if="!orders[idx].length" class="to-empty mono">まだ無い</span>
        </div>
        <div class="to-label">{{ mode === "seq" ? "預かり" : "待ち行列" }}</div>
        <div class="to-chips">
          <template v-if="mode === 'seq'">
            <span v-for="(m, i) in sq[n].hold" :key="i" class="to-chip wait">{{ m.seq }}:{{ m.body }}</span>
            <span v-if="!sq[n].hold.length" class="to-empty mono">無し</span>
          </template>
          <template v-else>
            <span v-for="(m, i) in vt[n].queue" :key="i" class="to-chip wait">{{ m.body }}({{ m.from }},{{ m.t }})</span>
            <span v-if="!vt[n].queue.length" class="to-empty mono">無し</span>
          </template>
        </div>
        <div class="to-foot mono">
          <template v-if="mode === 'seq'">次に渡す番号 {{ sq[n].next + 1 }}</template>
          <template v-else>聞いた時刻 {{ NAMES.map((k) => `${k}:${vt[n].heard[k] ?? 0}`).join(" ") }}</template>
        </div>
      </div>
    </div>

    <div class="to-verdict" :class="mode === 'seq' && step >= 4 ? 'bad' : agreed && step > 0 && orders[0].length ? 'ok' : 'wait'">
      {{ verdict }}
    </div>

    <div class="to-log">
      <div v-for="(l, i) in log.slice(-3)" :key="i" class="to-log-line mono">{{ l }}</div>
      <div v-if="!log.length" class="to-log-line mono empty">(まだ何も起きていない)</div>
    </div>

    <p class="to-legend">
      係を置く方式は、係に届いた順に番号が付き、全員がその番号順に渡す。番号は原因の順を見ていないので、
      回答が先に係へ着けば全員がそろって回答を先に渡す。全員で決める方式は係を置かない代わりに、
      先頭より後の時刻を他の全員から聞くまで渡さない。だから 1 台が黙るだけで全体が止まる。
    </p>
  </DemoShell>
</template>

<style scoped>
.to-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.to-gap {
  flex: 1;
  min-width: 8px;
}
.to-step {
  margin: 12px 0 0;
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.to-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 8px;
  margin-top: 10px;
}
.to-node {
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
  padding: 9px 10px;
}
.to-name {
  font-size: 13px;
  font-weight: 600;
  padding-bottom: 5px;
  border-bottom: 1px solid var(--vp-c-divider);
}
.to-label {
  margin-top: 7px;
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.to-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 3px;
  min-height: 20px;
}
.to-chip {
  font-size: 11px;
  padding: 1px 7px;
  border: 1px solid var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.to-chip i {
  font-style: normal;
  font-size: 8px;
  vertical-align: super;
  opacity: 0.7;
  margin-right: 2px;
}
.to-chip.wait {
  border-color: var(--vp-c-warning-1);
  color: var(--vp-c-warning-1);
  background-color: var(--vp-c-warning-soft);
}
.to-empty {
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.to-foot {
  margin-top: 7px;
  padding-top: 5px;
  border-top: 1px solid var(--vp-c-divider);
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.to-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  font-weight: 600;
  line-height: 1.6;
  background-color: var(--vp-c-bg-soft);
}
.to-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.to-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.to-verdict.wait {
  border-left-color: var(--vp-c-warning-1);
  color: var(--vp-c-warning-1);
  background-color: var(--vp-c-warning-soft);
}
.to-log {
  margin-top: 10px;
  border-top: 1px solid var(--vp-c-divider);
  padding-top: 6px;
  min-height: 36px;
}
.to-log-line {
  font-size: 10px;
  color: var(--vp-c-text-2);
  padding: 1px 0;
}
.to-log-line.empty {
  color: var(--vp-c-text-3);
}
.to-legend {
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
