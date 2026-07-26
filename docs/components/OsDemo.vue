<script setup lang="ts">
import { ref, computed, watch } from "vue";
import DemoShell from "./DemoShell.vue";

// foundations/os(Go)の協調スケジューラをブラウザで動かす移植版。
// タスク = 命令(Run/Yield/Sleep)の並び。カーネルは run queue(FIFO=round-robin)の
// 先頭を dispatch し、yield / sleep / 完了 まで走らせる。横取りはしない(協調)。

type OpK = "run" | "yield" | "sleep";
interface Op {
  k: OpK;
  n: number;
}
const R = (n: number): Op => ({ k: "run", n });
const Y = (): Op => ({ k: "yield", n: 0 });
const S = (n: number): Op => ({ k: "sleep", n });

interface TaskDef {
  name: string;
  prog: Op[];
}
interface Scenario {
  id: string;
  label: string;
  lesson: string;
  tasks: TaskDef[];
}

// 3つのシナリオ。round-robin(公平)/ 貪欲(独占)/ sleep と空転(idle)。
const SCENARIOS: Scenario[] = [
  {
    id: "rr",
    label: "round-robin",
    lesson: "全員が Run のあと yield する。CPU は先頭 → 末尾の順で公平に回り、きれいに交代する。",
    tasks: [
      { name: "A", prog: [R(2), Y(), R(2)] },
      { name: "B", prog: [R(2), Y(), R(2)] },
      { name: "C", prog: [R(2), Y(), R(2)] },
    ],
  },
  {
    id: "greedy",
    label: "貪欲タスク",
    lesson: "greedy は yield を書かず 8 tick 走りっぱなし。協調方式は横取りできないので、他の2つは greedy が終わるまで一切走れない。",
    tasks: [
      { name: "greedy", prog: [R(8)] },
      { name: "X", prog: [R(2), Y(), R(2)] },
      { name: "Y", prog: [R(2), Y(), R(2)] },
    ],
  },
  {
    id: "sleep",
    label: "sleep と空転",
    lesson: "worker と ticker は sleep で run queue から外れる。走れるタスクが尽きた tick 6 では、CPU は次の起床(7)まで空転(idle)する。",
    tasks: [
      { name: "worker", prog: [R(2), S(5), R(2)] },
      { name: "helper", prog: [R(2), Y(), R(1)] },
      { name: "ticker", prog: [R(1), S(2), R(1)] },
    ],
  },
];

const IDLE = "(idle)";

interface Frame {
  running: string;
  ready: string[];
  blocked: { name: string; wake: number }[];
}
interface SimResult {
  total: number;
  switches: number;
  ticks: string[]; // ticks[c] = そのtickにCPUを握った名前 / (idle)
  frames: Frame[]; // frames[c] = そのtick時点の run queue の様子
}

// Go の Kernel をそのまま移植した決定的シミュレータ。
function simulate(defs: TaskDef[]): SimResult {
  const state = defs.map((d, i) => ({
    name: d.name,
    prog: d.prog,
    pc: 0,
    cpu: 0,
    wake: 0,
    seq: i,
  }));
  type T = (typeof state)[number];
  let clock = 0;
  let switches = 0;
  const ready: T[] = state.slice();
  let blocked: T[] = [];
  const ticks: string[] = [];
  const frames: Frame[] = [];

  const byWakeSeq = (a: T, b: T) => (a.wake !== b.wake ? a.wake - b.wake : a.seq - b.seq);
  const snap = (running: string): Frame => ({
    running,
    ready: ready.map((t) => t.name),
    blocked: blocked
      .slice()
      .sort(byWakeSeq)
      .map((t) => ({ name: t.name, wake: t.wake })),
  });
  const wake = () => {
    if (blocked.length === 0) return;
    const woken: T[] = [];
    const still: T[] = [];
    for (const t of blocked) (t.wake <= clock ? woken : still).push(t);
    if (woken.length === 0) return;
    woken.sort(byWakeSeq);
    for (const t of woken) ready.push(t);
    blocked = still;
  };

  let guard = 0;
  while (guard++ < 10000) {
    wake();
    if (ready.length === 0) {
      if (blocked.length === 0) break;
      const next = Math.min(...blocked.map((t) => t.wake));
      const s = snap(IDLE);
      for (let c = clock; c < next; c++) {
        ticks[c] = IDLE;
        frames[c] = s;
      }
      clock = next;
      wake();
    }
    const t = ready.shift() as T;
    let broke = false;
    while (!broke) {
      if (t.pc >= t.prog.length) {
        switches++;
        broke = true;
        break;
      }
      const op = t.prog[t.pc];
      t.pc++;
      if (op.k === "run") {
        const n = op.n < 1 ? 1 : op.n;
        const s = snap(t.name);
        for (let c = clock; c < clock + n; c++) {
          ticks[c] = t.name;
          frames[c] = s;
        }
        clock += n;
        t.cpu += n;
        continue; // 協調: yield するまで CPU を握り続ける
      }
      if (op.k === "yield") {
        ready.push(t);
        switches++;
        broke = true;
        break;
      }
      t.wake = clock + op.n;
      blocked.push(t);
      switches++;
      broke = true;
    }
  }
  return { total: clock, switches, ticks, frames };
}

const scenarioId = ref("rr");
const t = ref(0); // プレイヘッド(0..total)

const scenario = computed(() => SCENARIOS.find((s) => s.id === scenarioId.value) as Scenario);
const sim = computed(() => simulate(scenario.value.tasks));
const names = computed(() => scenario.value.tasks.map((d) => d.name));

// タスク名 → 色クラス(生成順で t0/t1/t2)。
const colorOf = (name: string): string => {
  if (name === IDLE) return "idle";
  const i = names.value.indexOf(name);
  return i >= 0 ? `t${i}` : "idle";
};

// 現在のプレイヘッド時点の run queue。末尾(t==total)は全完了。
const frame = computed<Frame | null>(() => {
  if (t.value >= sim.value.total) return null;
  return sim.value.frames[t.value] ?? null;
});

const progSummary = (prog: Op[]): string =>
  prog.map((o) => (o.k === "run" ? `R${o.n}` : o.k === "yield" ? "Y" : `S${o.n}`)).join(" ");

const badge = computed(() => `${sim.value.switches} 回切替 · clock ${sim.value.total}`);

function pick(id: string) {
  scenarioId.value = id;
}
function step(d: number) {
  t.value = Math.max(0, Math.min(sim.value.total, t.value + d));
}
watch(scenarioId, () => {
  t.value = 0;
});
</script>

<template>
  <DemoShell title="os(協調スケジューラ)" :badge="badge" badge-tone="neutral">
    <div class="sd-controls">
      <div class="sd-seg">
        <span
          v-for="s in SCENARIOS"
          :key="s.id"
          class="sd-seg-opt"
          :class="{ on: s.id === scenarioId }"
          @click="pick(s.id)"
          >{{ s.label }}</span
        >
      </div>
    </div>

    <div class="os-lesson" :class="scenarioId">{{ scenario.lesson }}</div>

    <!-- タイムライン(どの tick に誰が CPU を握ったか) -->
    <div class="os-label">CPU タイムライン — 縦線がいまの時刻(t = {{ t }})</div>
    <div class="os-gantt">
      <div class="os-axis">
        <span class="os-rowlabel" />
        <span v-for="c in sim.total" :key="c" class="os-tick" :class="{ cur: c - 1 === t }">{{ c - 1 }}</span>
      </div>
      <div v-for="name in names" :key="name" class="os-row">
        <span class="os-rowlabel" :class="colorOf(name)">{{ name }}</span>
        <span
          v-for="c in sim.total"
          :key="c"
          class="os-cell"
          :class="[sim.ticks[c - 1] === name ? colorOf(name) : 'empty', { cur: c - 1 === t }]"
        />
      </div>
      <div class="os-row">
        <span class="os-rowlabel muted">idle</span>
        <span
          v-for="c in sim.total"
          :key="c"
          class="os-cell"
          :class="[sim.ticks[c - 1] === IDLE ? 'idle' : 'empty', { cur: c - 1 === t }]"
        />
      </div>
    </div>

    <div class="os-controls">
      <button class="sd-btn" :disabled="t === 0" @click="step(-sim.total)">最初へ</button>
      <button class="sd-btn" :disabled="t === 0" @click="step(-1)">◀ 1手</button>
      <button class="sd-btn" :disabled="t >= sim.total" @click="step(1)">1手 ▶</button>
      <button class="sd-btn" :disabled="t >= sim.total" @click="step(sim.total)">最後へ</button>
    </div>

    <!-- いまの時刻の run queue の様子 -->
    <div class="os-label">t = {{ t }} の run queue</div>
    <div v-if="frame" class="os-queues">
      <div class="os-qrow">
        <span class="os-qkey">Running</span>
        <span v-if="frame.running === IDLE" class="os-chip idle">CPU 空転(idle)</span>
        <span v-else class="os-chip" :class="colorOf(frame.running)">{{ frame.running }}</span>
      </div>
      <div class="os-qrow">
        <span class="os-qkey">Ready</span>
        <span class="os-qhint">先頭が次に走る →</span>
        <template v-if="frame.ready.length">
          <span v-for="(nm, i) in frame.ready" :key="i" class="os-chip" :class="colorOf(nm)">{{ nm }}</span>
        </template>
        <span v-else class="os-qempty">(空)</span>
      </div>
      <div class="os-qrow">
        <span class="os-qkey">Blocked</span>
        <template v-if="frame.blocked.length">
          <span v-for="(b, i) in frame.blocked" :key="i" class="os-chip block" :class="colorOf(b.name)"
            >{{ b.name }}<span class="os-wake">wake @ {{ b.wake }}</span></span
          >
        </template>
        <span v-else class="os-qempty">(なし)</span>
      </div>
    </div>
    <div v-else class="os-done">t = {{ t }}: 全タスク完了(run queue も blocked も空)</div>

    <!-- 各タスクのプログラム -->
    <div class="os-label">タスクのプログラム(命令の並び)</div>
    <div class="os-progs">
      <div v-for="d in scenario.tasks" :key="d.name" class="os-prog">
        <span class="os-chip" :class="colorOf(d.name)">{{ d.name }}</span>
        <code class="os-code">{{ progSummary(d.prog) }}</code>
      </div>
    </div>
    <div class="os-legend">R=Run(CPUを使う, yieldしない) · Y=Yield(手放す) · S=Sleep(ブロック)</div>
  </DemoShell>
</template>

<style scoped>
.os-lesson {
  margin: 14px 0 4px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-2);
  background-color: var(--vp-c-default-soft);
  font-size: 12px;
  line-height: 1.6;
  color: var(--vp-c-text-2);
  /* 左アクセントレールは角を落とす(角丸との併用禁止) */
  border-radius: 0;
}
.os-lesson.greedy {
  border-left-color: var(--vp-c-red-2);
}
.os-lesson.sleep {
  border-left-color: var(--vp-c-green-2);
}
.os-label {
  font-size: 11px;
  color: var(--vp-c-text-3);
  margin: 18px 0 6px;
}
.os-gantt {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.os-axis,
.os-row {
  display: flex;
  align-items: stretch;
  gap: 3px;
}
.os-rowlabel {
  flex: 0 0 58px;
  font-size: 11px;
  font-family: var(--vp-font-family-mono);
  font-weight: 700;
  color: var(--vp-c-text-1);
  display: flex;
  align-items: center;
}
.os-rowlabel.muted {
  font-weight: 400;
  color: var(--vp-c-text-3);
}
.os-rowlabel.t0 {
  color: var(--vp-c-brand-1);
}
.os-rowlabel.t1 {
  color: var(--vp-c-purple-1);
}
.os-rowlabel.t2 {
  color: var(--vp-c-green-1);
}
.os-tick {
  flex: 1 1 0;
  min-width: 22px;
  text-align: center;
  font-size: 10px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-3);
  padding-bottom: 2px;
}
.os-tick.cur {
  color: var(--vp-c-text-1);
  font-weight: 700;
}
.os-cell {
  flex: 1 1 0;
  min-width: 22px;
  height: 20px;
  border: 1px solid var(--vp-c-divider);
}
.os-cell.empty {
  background-color: var(--vp-c-bg-alt);
}
.os-cell.cur {
  outline: 2px solid var(--vp-c-text-1);
  outline-offset: -1px;
  position: relative;
  z-index: 1;
}
.os-cell.t0,
.os-chip.t0 {
  background-color: var(--vp-c-brand-2);
}
.os-cell.t1,
.os-chip.t1 {
  background-color: var(--vp-c-purple-2);
}
.os-cell.t2,
.os-chip.t2 {
  background-color: var(--vp-c-green-2);
}
.os-cell.idle {
  background-color: var(--vp-c-bg-alt);
  background-image: repeating-linear-gradient(
    45deg,
    var(--vp-c-divider),
    var(--vp-c-divider) 2px,
    transparent 2px,
    transparent 5px
  );
}
.os-controls {
  display: flex;
  gap: 6px;
  margin-top: 10px;
  flex-wrap: wrap;
}
.os-queues {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.os-qrow {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.os-qkey {
  flex: 0 0 64px;
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
}
.os-qhint,
.os-qempty {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.os-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 3px 10px;
  border: 1px solid var(--vp-c-divider);
  font-size: 12px;
  font-weight: 700;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-1);
}
.os-chip.idle {
  background-color: var(--vp-c-bg-alt);
  color: var(--vp-c-text-3);
  font-weight: 400;
}
.os-chip.block {
  opacity: 0.85;
}
.os-wake {
  font-size: 10px;
  font-weight: 400;
  color: var(--vp-c-text-2);
}
.os-done {
  padding: 8px 12px;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-alt);
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.os-progs {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.os-prog {
  display: flex;
  align-items: center;
  gap: 10px;
}
.os-code {
  font-size: 12px;
  color: var(--vp-c-text-2);
  background: none;
  padding: 0;
}
.os-legend {
  margin-top: 8px;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
</style>
