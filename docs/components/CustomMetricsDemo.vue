<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/custommetrics(Go)を移植。同じ負荷の跳ねに CPU 目標と
// 待ち行列目標を同時に当て、追いつく速さと落ち着き先の違いを見る。

type Kind = "utilization" | "averageValue";
interface Metric {
  name: string;
  kind: Kind;
  target: number;
}
interface Config {
  metrics: Metric[];
  min: number;
  max: number;
  activation: number;
  cooldown: number;
}
interface Snap {
  t: number;
  replicas: number;
  backlog: number;
  cpu: number;
  arrivals: number;
  served: number;
}

function ceilDiv(a: number, b: number): number {
  const n = Math.trunc(a / b);
  return n * b < a ? n + 1 : n;
}

function desired(replicas: number, m: Metric, current: number): number {
  if (m.target <= 0) return replicas;
  if (m.kind === "utilization") {
    if (replicas <= 0) return replicas;
    return ceilDiv(replicas * current, m.target);
  }
  return ceilDiv(current, m.target);
}

function run(cfg: Config, arrivals: number[], capacity: number, start: number): Snap[] {
  let replicas = start;
  let backlog = 0;
  let cpu = 0;
  let idle = 0;
  const out: Snap[] = [];

  for (let t = 0; t < arrivals.length; t++) {
    const readings: Record<string, number> = { cpu, queue: backlog };
    const work = Math.max(...Object.values(readings));

    let next: number;
    if (replicas === 0) {
      if (cfg.min > 0) next = cfg.min;
      else if (work > cfg.activation) {
        idle = 0;
        next = 1;
      } else next = 0;
    } else {
      if (cfg.min === 0 && work <= cfg.activation) idle++;
      else idle = 0;
      let want = 0;
      let first = true;
      for (const m of cfg.metrics) {
        const v = readings[m.name];
        if (v === undefined) continue;
        const d = desired(replicas, m, v);
        if (first || d > want) {
          want = d;
          first = false;
        }
      }
      if (first) want = replicas;
      next = Math.min(cfg.max, Math.max(cfg.min, want));
      if (next === 0 && idle <= cfg.cooldown) next = 1;
    }
    replicas = next;

    backlog += arrivals[t];
    const capTotal = replicas * capacity;
    const served = Math.min(backlog, capTotal);
    backlog -= served;
    cpu = capTotal > 0 ? (served / capTotal) * 100 : 0;

    out.push({ t, replicas, backlog, cpu, arrivals: arrivals[t], served });
  }
  return out;
}

function stabilizedAt(h: Snap[]): number {
  let last = -1;
  for (let i = 0; i < h.length; i++) {
    if (h[i].arrivals > 0 && h[i].served < h[i].arrivals) last = i;
  }
  if (last < 0) return 0;
  if (last === h.length - 1) return -1;
  return h[last + 1].t;
}

const CAPACITY = 10;
const STEPS = 34;
const allowZero = ref(false);
const stops = ref(false); // 途中で仕事が止まる台本にするか

const arrivals = computed(() =>
  Array.from({ length: STEPS }, (_, i) => {
    if (i < 5) return 20;
    if (stops.value && i >= 18) return 0;
    return 200;
  }),
);

function cfgFor(kind: Kind): Config {
  const m: Metric =
    kind === "utilization"
      ? { name: "cpu", kind: "utilization", target: 50 }
      : { name: "queue", kind: "averageValue", target: 10 };
  return {
    metrics: [m],
    min: allowZero.value ? 0 : 1,
    max: 100,
    activation: 0,
    cooldown: 2,
  };
}

const cpuRun = computed(() => run(cfgFor("utilization"), arrivals.value, CAPACITY, 2));
const queueRun = computed(() => run(cfgFor("averageValue"), arrivals.value, CAPACITY, 2));

const runs = computed(() => [
  { label: "CPU 使用率 50% を目標", key: "cpu", h: cpuRun.value, note: "上限のある指標" },
  { label: "待ち行列 10 件/個 を目標", key: "queue", h: queueRun.value, note: "上限の無い指標" },
]);

const W = 540;
const H = 74;
const maxRep = computed(() => Math.max(...runs.value.flatMap((r) => r.h.map((s) => s.replicas)), 1));
const maxBack = computed(() => Math.max(...runs.value.flatMap((r) => r.h.map((s) => s.backlog)), 1));

function path(vals: number[], max: number): string {
  const step = W / (vals.length - 1);
  return vals.map((v, i) => `${i === 0 ? "M" : "L"}${(i * step).toFixed(1)},${(H - (v / max) * H).toFixed(1)}`).join(" ");
}

const badge = computed(
  () => `追いつく t: CPU ${stabilizedAt(cpuRun.value)} / 待ち行列 ${stabilizedAt(queueRun.value)}`,
);
const summary = computed(() => {
  const c = cpuRun.value[cpuRun.value.length - 1];
  const q = queueRun.value[queueRun.value.length - 1];
  if (stops.value) {
    return allowZero.value
      ? `仕事が止まったあと、どちらも 0 へ落ちた(CPU ${c.replicas} 個 / 待ち行列 ${q.replicas} 個)。0 の判断は指標の値でなく、仕事の有無で決まっている`
      : `仕事が止まっても、下限が 1 なのでどちらも 1 個は残る(CPU ${c.replicas} 個 / 待ち行列 ${q.replicas} 個)。0 を許すに切り替えると落ちる`;
  }
  return `落ち着き先が違う。CPU は ${c.replicas} 個で行列 ${c.backlog} 件、待ち行列は ${q.replicas} 個で行列 ${q.backlog} 件。どちらも目標どおりに動いている`;
});
</script>

<template>
  <DemoShell title="カスタム指標とKEDA" :badge="badge" badge-tone="ng">
    <div class="cm-actions">
      <button class="sd-btn" :class="stops ? 'sd-btn--primary' : ''" @click="stops = !stops">
        t=18 で仕事が止まる: {{ stops ? "止まる" : "止まらない" }}
      </button>
      <button class="sd-btn" :class="allowZero ? 'sd-btn--primary' : ''" @click="allowZero = !allowZero">
        0 まで縮めてよい: {{ allowZero ? "許す" : "下限 1" }}
      </button>
      <span class="cm-spacer" />
      <span class="cm-hint mono">到着 20 件/tick → t=5 で 200 件/tick ・ 1 個は 10 件/tick を捌く</span>
    </div>

    <div v-for="r in runs" :key="r.key" class="cm-run">
      <div class="cm-run-h">
        <span class="cm-title">{{ r.label }}</span>
        <span class="cm-note">{{ r.note }}</span>
        <span class="cm-stat mono">
          追いつく t={{ stabilizedAt(r.h) }} ・ 山 {{ Math.max(...r.h.map((s) => s.backlog)) }} ・
          最後 {{ r.h[r.h.length - 1].replicas }} 個 / 行列 {{ r.h[r.h.length - 1].backlog }}
        </span>
      </div>
      <svg class="cm-chart" :viewBox="`0 0 ${W} ${H}`" preserveAspectRatio="none">
        <path :d="path(r.h.map((s) => s.backlog), maxBack)" class="cm-backlog" />
        <path :d="path(r.h.map((s) => s.replicas), maxRep)" class="cm-replicas" />
      </svg>
      <div class="cm-axis mono">
        <span><i class="sw rep"></i>レプリカ数(上限 {{ maxRep }})</span>
        <span><i class="sw back"></i>待ち行列(上限 {{ maxBack }} 件)</span>
        <span>t=0 … t={{ STEPS - 1 }}</span>
      </div>
    </div>

    <div class="cm-table">
      <div class="cm-th mono"><span>t</span><span>到着</span><span>CPU 個数</span><span>CPU 行列</span><span>行列 個数</span><span>行列 行列</span></div>
      <div v-for="t in [5, 6, 7, 8, 9]" :key="t" class="cm-tr mono">
        <span>{{ t }}</span>
        <span>{{ cpuRun[t].arrivals }}</span>
        <span>{{ cpuRun[t].replicas }}</span>
        <span :class="cpuRun[t].backlog > queueRun[t].backlog ? 'hot' : ''">{{ cpuRun[t].backlog }}</span>
        <span>{{ queueRun[t].replicas }}</span>
        <span :class="queueRun[t].backlog > cpuRun[t].backlog ? 'hot' : ''">{{ queueRun[t].backlog }}</span>
      </div>
    </div>

    <div class="cm-verdict">{{ summary }}</div>

    <p class="cm-legend">
      CPU は 100% で頭打ちになるので、どれだけ遅れていても 1 周期で倍にしかならない。待ち行列は
      200 件が見えた時点で「20 個要る」と出る。ただし落ち着き先は違い、CPU 50% は必要な数の倍を抱え、
      1 個 10 件は行列が残る。「0 まで縮めてよい」で仕事を止めると、値でなく有無で 0 へ落ちるのが見える。
    </p>
  </DemoShell>
</template>

<style scoped>
.cm-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.cm-spacer {
  flex: 1;
}
.cm-hint {
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.cm-run {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  padding: 9px 12px;
  background-color: var(--vp-c-bg-soft);
}
.cm-run-h {
  display: flex;
  min-width: 0;
  align-items: baseline;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 6px;
}
.cm-title {
  font-size: 12.5px;
  font-weight: 700;
  color: var(--vp-c-text-1);
}
.cm-note {
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.cm-stat {
  font-size: 10px;
  color: var(--vp-c-text-3);
  margin-left: auto;
}
.cm-chart {
  width: 100%;
  height: 74px;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-alt);
  display: block;
}
.cm-replicas {
  fill: none;
  stroke: var(--vp-c-brand-1);
  stroke-width: 2;
  vector-effect: non-scaling-stroke;
}
.cm-backlog {
  fill: none;
  stroke: var(--vp-c-danger-1);
  stroke-width: 1.5;
  stroke-dasharray: 4 3;
  vector-effect: non-scaling-stroke;
}
.cm-axis {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 3px;
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.sw {
  display: inline-block;
  width: 12px;
  height: 2px;
  margin-right: 5px;
  vertical-align: middle;
}
.sw.rep {
  background-color: var(--vp-c-brand-1);
}
.sw.back {
  background-color: var(--vp-c-danger-1);
}
.cm-table {
  margin-top: 12px;
  border: 1px solid var(--vp-c-divider);
  padding: 8px 12px;
  background-color: var(--vp-c-bg-soft);
  overflow-x: auto;
}
.cm-th,
.cm-tr {
  display: grid;
  grid-template-columns: 32px 48px minmax(0, 1fr) minmax(0, 1fr) minmax(0, 1fr) minmax(0, 1fr);
  min-width: 320px;
  gap: 6px;
  font-size: 10.5px;
  padding: 2px 0;
  color: var(--vp-c-text-2);
}
.cm-th {
  font-size: 9.5px;
  font-weight: 700;
  color: var(--vp-c-text-3);
  border-bottom: 1px solid var(--vp-c-divider);
  padding-bottom: 4px;
  margin-bottom: 3px;
}
.cm-tr .hot {
  color: var(--vp-c-danger-1);
  font-weight: 700;
}
.cm-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  font-weight: 600;
  line-height: 1.6;
  color: var(--vp-c-text-1);
  background-color: var(--vp-c-bg-soft);
}
.cm-legend {
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
