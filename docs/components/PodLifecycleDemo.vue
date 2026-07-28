<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/lifecycle(Go)を移植。preStop と猶予の設定を変えると、
// 同じ削除でリクエストが落ちたり落ちなかったりする様子を見せる。

const PROPAGATION = 4; // 転送先一覧から外れるまでの遅れ(こちらでは短くできない)
const WORK = 3; // 1 リクエストの処理時間
const RPS = 4; // 1 周期に届くリクエスト数
const TICKS = 16;

const preStop = ref(0); // SIGTERM の前に待つ時間
const grace = ref(10); // SIGTERM から SIGKILL までの猶予

interface Frame {
  t: number;
  served: number;
  dropped: number;
  inEndpoints: boolean;
  accepting: boolean;
  stopped: boolean;
  inflight: number;
}

interface Run {
  frames: Frame[];
  served: number;
  dropped: number;
  log: string[];
}

// Go の Sim と同じ順序で回す。予定を先に反映してから振る。
function simulate(preStopV: number, graceV: number): Run {
  const pods = [1, 2].map((i) => ({
    name: `pod-${i}`,
    accepting: true,
    stopped: false,
    inflight: [] as number[],
    removeAt: -1,
    sigtermAt: -1,
    killAt: -1,
  }));
  let now = 0;
  let rr = 0;
  let served = 0;
  let dropped = 0;
  const log: string[] = [];
  const frames: Frame[] = [];
  const p1 = pods[0];

  const endpoints = () => pods.filter((p) => p.removeAt < 0 || now < p.removeAt);

  for (let step = 0; step < TICKS; step++) {
    if (step === 1) {
      // 削除開始。切り離しと停止が同時に動き出す。
      p1.removeAt = now + PROPAGATION;
      p1.sigtermAt = now + preStopV;
      p1.killAt = p1.sigtermAt + graceV;
      log.push(`t=${now} pod-1 の削除を開始(転送先から外れるのは t=${p1.removeAt}、SIGTERM は t=${p1.sigtermAt})`);
    }

    // ① 予定された終了処理を今の時刻に反映する。
    for (const p of pods) {
      if (p.removeAt < 0 || p.stopped) continue;
      if (now === p.removeAt) log.push(`t=${now} ${p.name} が転送先一覧から外れた`);
      if (now === p.sigtermAt) {
        p.accepting = false;
        log.push(`t=${now} ${p.name} に SIGTERM。新規の受け付けを止める(処理中 ${p.inflight.length} 件)`);
      }
      if (!p.accepting && p.inflight.length === 0) {
        p.stopped = true;
        log.push(`t=${now} ${p.name} は処理中を捌き終えて停止`);
        continue;
      }
      if (p.killAt >= 0 && now >= p.killAt && p.inflight.length > 0) {
        dropped += p.inflight.length;
        log.push(`t=${now} ${p.name} が猶予切れで SIGKILL。処理中 ${p.inflight.length} 件を道連れにする`);
        p.inflight = [];
        p.stopped = true;
      }
    }

    // ② リクエストを転送先へ順に振る。
    const eps = endpoints();
    let tickDropped = 0;
    if (eps.length === 0) {
      dropped += RPS;
      tickDropped += RPS;
      log.push(`t=${now} 転送先が空。${RPS} 件が行き場を失う`);
    } else {
      for (let i = 0; i < RPS; i++) {
        const p = eps[rr % eps.length];
        rr++;
        if (!p.accepting) {
          dropped++;
          tickDropped++;
          log.push(`t=${now} ${p.name} は受け付けを止めているのに振られた。1 件失う`);
        } else {
          p.inflight = [...p.inflight, WORK];
        }
      }
    }

    // ③ 処理中を1つ進める。
    let tickServed = 0;
    for (const p of pods) {
      const rest: number[] = [];
      for (const left of p.inflight) {
        if (left - 1 <= 0) {
          served++;
          tickServed++;
        } else rest.push(left - 1);
      }
      p.inflight = rest;
    }

    frames.push({
      t: now,
      served: tickServed,
      dropped: tickDropped,
      inEndpoints: eps.includes(p1),
      accepting: p1.accepting,
      stopped: p1.stopped,
      inflight: p1.inflight.length,
    });
    now++;
  }
  return { frames, served, dropped, log };
}

const run = computed(() => simulate(preStop.value, grace.value));
const safe = computed(() => run.value.dropped === 0);
const badge = computed(() => `成功 ${run.value.served} / 失敗 ${run.value.dropped}`);
const badgeTone = computed<"ok" | "ng">(() => (safe.value ? "ok" : "ng"));

// pod-1 が「転送先に残っているのに受け付けていない」周期。ここが事故の窓。
const gap = computed(() => run.value.frames.filter((f) => f.inEndpoints && !f.accepting).length);

function cellClass(f: Frame) {
  if (f.dropped > 0) return "bad";
  if (f.inEndpoints && !f.accepting) return "warn";
  return "ok";
}
</script>

<template>
  <DemoShell title="Podの終了(graceful shutdown)" :badge="badge" :badge-tone="badgeTone">
    <div class="pl-row">
      <span class="pl-label">preStop(SIGTERM の前に待つ時間)</span>
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: preStop === 0 }" @click="preStop = 0">0(待たない)</span>
        <span class="sd-seg-opt" :class="{ on: preStop === 2 }" @click="preStop = 2">2(伝播より短い)</span>
        <span class="sd-seg-opt" :class="{ on: preStop === 4 }" @click="preStop = 4">4(伝播を覆う)</span>
      </span>
    </div>
    <div class="pl-row">
      <span class="pl-label">猶予(SIGKILL までの時間)</span>
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: grace === 1 }" @click="grace = 1">1(処理時間より短い)</span>
        <span class="sd-seg-opt" :class="{ on: grace === 10 }" @click="grace = 10">10(十分)</span>
      </span>
    </div>

    <p class="pl-fixed mono">
      転送先から外れるまでの遅れ {{ PROPAGATION }} / 1リクエストの処理時間 {{ WORK }} / 毎周期 {{ RPS }} 件が届く / レプリカ 2
    </p>

    <div class="pl-timeline">
      <div v-for="f in run.frames" :key="f.t" class="pl-cell" :class="cellClass(f)">
        <span class="pl-t mono">{{ f.t }}</span>
        <span class="pl-n mono">{{ f.dropped > 0 ? "×" + f.dropped : f.served }}</span>
      </div>
    </div>
    <div class="pl-key">
      <span class="pl-k ok">処理できた</span>
      <span class="pl-k warn">転送先に残っているが受け付けていない</span>
      <span class="pl-k bad">リクエストが落ちた</span>
    </div>

    <div class="pl-verdict" :class="safe ? 'ok' : 'bad'">
      <template v-if="safe">
        1 件も落ちなかった: 切り離しが伝わりきってから停止が始まり、処理中も最後まで捌けた
      </template>
      <template v-else>
        {{ run.dropped }} 件が落ちた: 転送先に残ったまま受け付けを止めた周期が {{ gap }} 回ある
      </template>
    </div>

    <div class="pl-log">
      <div class="pl-log-h">起きたこと</div>
      <div v-for="(l, i) in run.log.slice(0, 14)" :key="i" class="pl-log-line mono">{{ l }}</div>
      <div v-if="run.log.length > 14" class="pl-log-line mono">…ほか {{ run.log.length - 14 }} 行</div>
    </div>

    <p class="pl-legend">
      削除を決めると、転送先一覧からの除去と SIGTERM が同時に動き出す。除去が伝わるには時間がかかるので、
      preStop を 0 にすると、まだ振られてくるのに受け付けない Pod ができる。preStop を伝播以上に取れば、
      その窓が閉じて 1 件も落ちない。猶予を短くすると、今度は処理中のリクエストが SIGKILL に巻き込まれる。
      preStop は外から来る分を、猶予は中で走っている分を守っていて、どちらが欠けても落ちる。
    </p>
  </DemoShell>
</template>

<style scoped>
.pl-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 8px;
}
.pl-label {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 200px;
}
.pl-fixed {
  margin: 10px 0 0;
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.pl-timeline {
  display: flex;
  gap: 3px;
  margin-top: 14px;
  flex-wrap: wrap;
}
.pl-cell {
  flex: 1;
  min-width: 26px;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 5px 2px;
  border: 1px solid var(--vp-c-divider);
}
.pl-cell.ok {
  background-color: var(--vp-c-green-soft);
  border-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
}
.pl-cell.warn {
  background-color: var(--vp-c-warning-soft);
  border-color: var(--vp-c-warning-1);
  color: var(--vp-c-warning-1);
}
.pl-cell.bad {
  background-color: var(--vp-c-danger-soft);
  border-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
}
.pl-t {
  font-size: 9px;
  opacity: 0.7;
}
.pl-n {
  font-size: 12px;
  font-weight: 700;
}
.pl-key {
  display: flex;
  gap: 14px;
  margin-top: 8px;
  flex-wrap: wrap;
}
.pl-k {
  font-size: 10.5px;
  padding-left: 12px;
  position: relative;
  color: var(--vp-c-text-3);
}
.pl-k::before {
  content: "";
  position: absolute;
  left: 0;
  top: 3px;
  width: 8px;
  height: 8px;
}
.pl-k.ok::before {
  background-color: var(--vp-c-green-1);
}
.pl-k.warn::before {
  background-color: var(--vp-c-warning-1);
}
.pl-k.bad::before {
  background-color: var(--vp-c-danger-1);
}
.pl-verdict {
  margin-top: 14px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
  background-color: var(--vp-c-bg-soft);
}
.pl-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.pl-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.pl-log {
  margin-top: 12px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
}
.pl-log-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 4px;
}
.pl-log-line {
  font-size: 10.5px;
  color: var(--vp-c-text-2);
  padding: 1px 0;
}
.pl-legend {
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
