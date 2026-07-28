<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/rollout(Go)を移植。maxSurge と maxUnavailable の幅を変えて、
// 入れ替えの速さと保たれる容量の交換を見せる。壊れた版では止まることも。

const REPLICAS = 4;
const STARTUP = 2;
const MAX_STEPS = 40;

const surge = ref(1);
const unavail = ref(0);
const broken = ref(false);

interface P {
  name: string;
  version: number;
  age: number;
  broken: boolean;
}
interface Snap {
  oldReady: number;
  oldWait: number;
  newReady: number;
  newWait: number;
  available: number;
}

const isReady = (p: P) => !p.broken && p.age >= STARTUP;

function simulate() {
  const minAvail = Math.max(0, REPLICAS - unavail.value);
  const maxTotal = REPLICAS + surge.value;
  let seq = 0;
  const pods: P[] = [];
  for (let i = 0; i < REPLICAS; i++) {
    seq++;
    pods.push({ name: `pod-${seq}`, version: 1, age: STARTUP, broken: false });
  }
  const target = 2;
  const snaps: Snap[] = [];
  const log: string[] = [];

  const available = () => pods.filter(isReady).length;
  const countV = (v: number) => pods.filter((p) => p.version === v).length;
  const oldest = () => {
    let fallback: P | undefined;
    for (const p of pods) {
      if (p.version === target) continue;
      if (!isReady(p)) return p;
      if (!fallback) fallback = p;
    }
    return fallback;
  };
  const canRemove = (p: P) => !isReady(p) || available() > minAvail;
  const done = () => pods.every((p) => p.version === target) && pods.filter(isReady).length === REPLICAS;

  for (let step = 0; step < MAX_STEPS && !done(); step++) {
    for (const p of pods) p.age++;
    const acts: string[] = [];

    if (countV(target) < REPLICAS && pods.length < maxTotal) {
      seq++;
      const p: P = { name: `pod-${seq}`, version: target, age: 0, broken: broken.value };
      pods.push(p);
      acts.push(`create ${p.name}(v2)`);
    }
    const old = oldest();
    if (old && canRemove(old)) {
      pods.splice(pods.indexOf(old), 1);
      acts.push(`delete ${old.name}(v${old.version})`);
    }
    log.push(`step ${step}: ${acts.length ? acts.join(" / ") : "動けない(新しい版が ready になるのを待つ)"}`);

    const s: Snap = { oldReady: 0, oldWait: 0, newReady: 0, newWait: 0, available: 0 };
    for (const p of pods) {
      const r = isReady(p);
      if (p.version === target) r ? s.newReady++ : s.newWait++;
      else r ? s.oldReady++ : s.oldWait++;
    }
    s.available = s.oldReady + s.newReady;
    snaps.push(s);

    // 進みも退きもしない状態になったら、そこで打ち切る。
    const stuck =
      !(countV(target) < REPLICAS && pods.length < maxTotal) && !(oldest() && canRemove(oldest()!));
    if (stuck && !pods.some((p) => !isReady(p) && !p.broken)) break;
  }

  const minSeen = snaps.length ? Math.min(REPLICAS, ...snaps.map((s) => s.available)) : REPLICAS;
  return { snaps, log, done: done(), minSeen, minAvail, maxTotal, steps: snaps.length };
}

const run = computed(() => simulate());
const deadlocked = computed(() => surge.value === 0 && unavail.value === 0);
const badge = computed(() =>
  run.value.done ? `完了 ${run.value.steps} 周期 / 最小容量 ${run.value.minSeen}` : `未完了 / 最小容量 ${run.value.minSeen}`,
);
const badgeTone = computed<"ok" | "ng">(() => (run.value.done ? "ok" : "ng"));
const UNIT = 15;
</script>

<template>
  <DemoShell title="ローリング更新" :badge="badge" :badge-tone="badgeTone">
    <div class="ro-row">
      <span class="ro-label">maxSurge(何個多く作ってよいか)</span>
      <span class="sd-seg">
        <span v-for="n in [0, 1, 2]" :key="n" class="sd-seg-opt" :class="{ on: surge === n }" @click="surge = n">{{ n }}</span>
      </span>
    </div>
    <div class="ro-row">
      <span class="ro-label">maxUnavailable(何個まで減ってよいか)</span>
      <span class="sd-seg">
        <span v-for="n in [0, 1, 2]" :key="n" class="sd-seg-opt" :class="{ on: unavail === n }" @click="unavail = n">{{ n }}</span>
      </span>
    </div>
    <div class="ro-row">
      <span class="ro-label">新しい版 v2</span>
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: !broken }" @click="broken = false">正常</span>
        <span class="sd-seg-opt" :class="{ on: broken }" @click="broken = true">壊れている(readyにならない)</span>
      </span>
    </div>

    <p class="ro-fixed mono">
      目標 {{ REPLICAS }} レプリカ / 起動に {{ STARTUP }} 周期 / 保つべき ready 数 {{ run.minAvail }} / 持ってよい総数 {{ run.maxTotal }}
    </p>

    <div class="ro-chart" :style="{ height: (REPLICAS + 2) * UNIT + 22 + 'px' }">
      <div class="ro-floor" :style="{ bottom: run.minAvail * UNIT + 20 + 'px' }">
        <span class="ro-floor-l mono">下限 {{ run.minAvail }}</span>
      </div>
      <div v-for="(s, i) in run.snaps" :key="i" class="ro-col">
        <span class="ro-stack">
          <span v-for="n in s.oldWait" :key="'ow' + n" class="ro-b old wait" :style="{ height: UNIT - 2 + 'px' }" />
          <span v-for="n in s.newWait" :key="'nw' + n" class="ro-b new wait" :style="{ height: UNIT - 2 + 'px' }" />
          <span v-for="n in s.newReady" :key="'nr' + n" class="ro-b new" :style="{ height: UNIT - 2 + 'px' }" />
          <span v-for="n in s.oldReady" :key="'or' + n" class="ro-b old" :style="{ height: UNIT - 2 + 'px' }" />
        </span>
        <span class="ro-t mono">{{ i }}</span>
      </div>
      <div v-if="run.snaps.length === 0" class="ro-empty">この設定では1歩も進めない</div>
    </div>

    <div class="ro-key">
      <span class="ro-k old">v1 稼働中</span>
      <span class="ro-k new">v2 稼働中</span>
      <span class="ro-k wait">起動待ち(まだ受けられない)</span>
    </div>

    <div class="ro-verdict" :class="run.done ? 'ok' : 'bad'">
      <template v-if="deadlocked">
        どちらの幅も 0 なので、多く作ることも減らすこともできない。入れ替えは1歩も進まない
      </template>
      <template v-else-if="run.done">
        {{ run.steps }} 周期で全部 v2 になった。その間 ready な数は最小 {{ run.minSeen }} まで(下限 {{ run.minAvail }})
      </template>
      <template v-else-if="broken">
        止まった: v2 が ready にならないので v1 は消えない。容量は {{ run.minSeen }} で踏みとどまり、全滅していない
      </template>
      <template v-else>{{ MAX_STEPS }} 周期では終わらなかった</template>
    </div>

    <div class="ro-log">
      <div class="ro-log-h">打った手</div>
      <div v-for="(l, i) in run.log.slice(0, 10)" :key="i" class="ro-log-line mono">{{ l }}</div>
      <div v-if="run.log.length > 10" class="ro-log-line mono">…ほか {{ run.log.length - 10 }} 行</div>
      <div v-if="run.log.length === 0" class="ro-log-line mono">(打てる手がない)</div>
    </div>

    <p class="ro-legend">
      maxSurge を上げると多く作れるので速く終わり、maxUnavailable を上げると先に消せるので容量が落ちる。
      速さと容量は交換になっている。新しい版を「壊れている」に切り替えると、v2 が ready にならないので v1 が消えず、
      置き換えは途中で止まる。このとき容量がどこまで落ちるかは maxUnavailable だけが決めている。
      進めてよいかの判断を readiness に委ねていることが、そのまま全滅を防ぐ仕掛けになっている。
    </p>
  </DemoShell>
</template>

<style scoped>
.ro-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 8px;
}
.ro-label {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 210px;
}
.ro-fixed {
  margin: 10px 0 0;
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.ro-chart {
  position: relative;
  display: flex;
  align-items: flex-end;
  gap: 4px;
  margin-top: 14px;
  padding: 8px 10px 0;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.ro-floor {
  position: absolute;
  left: 0;
  right: 0;
  border-top: 1px dashed var(--vp-c-text-3);
}
.ro-floor-l {
  position: absolute;
  right: 4px;
  top: -13px;
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.ro-col {
  position: relative;
  flex: 1;
  min-width: 12px;
  display: flex;
  flex-direction: column;
  justify-content: flex-end;
  height: 100%;
  padding-bottom: 20px;
}
.ro-stack {
  display: flex;
  flex-direction: column;
  justify-content: flex-end;
  gap: 2px;
}
.ro-b {
  display: block;
  width: 100%;
}
.ro-b.old {
  background-color: var(--vp-c-brand-1);
}
.ro-b.new {
  background-color: var(--vp-c-green-1);
}
.ro-b.wait {
  background-color: transparent;
  border: 1px dashed var(--vp-c-text-3);
}
.ro-t {
  position: absolute;
  bottom: 4px;
  left: 0;
  right: 0;
  text-align: center;
  font-size: 9px;
  color: var(--vp-c-text-3);
}
.ro-empty {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  color: var(--vp-c-text-3);
}
.ro-key {
  display: flex;
  gap: 14px;
  margin-top: 8px;
  flex-wrap: wrap;
}
.ro-k {
  font-size: 10.5px;
  padding-left: 12px;
  position: relative;
  color: var(--vp-c-text-3);
}
.ro-k::before {
  content: "";
  position: absolute;
  left: 0;
  top: 3px;
  width: 8px;
  height: 8px;
}
.ro-k.old::before {
  background-color: var(--vp-c-brand-1);
}
.ro-k.new::before {
  background-color: var(--vp-c-green-1);
}
.ro-k.wait::before {
  border: 1px dashed var(--vp-c-text-3);
}
.ro-verdict {
  margin-top: 14px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
  background-color: var(--vp-c-bg-soft);
}
.ro-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.ro-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.ro-log {
  margin-top: 12px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
}
.ro-log-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 4px;
}
.ro-log-line {
  font-size: 10.5px;
  color: var(--vp-c-text-2);
  padding: 1px 0;
}
.ro-legend {
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
