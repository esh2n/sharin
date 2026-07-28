<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/probe(Go)を移植。readiness / liveness / 起動用の検査を
// 個別に切り替えて、同じ Pod がどう扱われるかの違いを見せる。

const TICKS = 26;
const RPS = 2;

// pod-1 は最初から応答できる。pod-2 は起動が遅く、途中で一時的に詰まる。
const BEHAVIORS = [
  { startup: 0, hangAt: 0, hangFor: 0 },
  { startup: 8, hangAt: 14, hangFor: 3 },
];

// 何も設定していない状態から始める。1つずつ足すと事故が消えていく。
const useReadiness = ref(false);
const strictLiveness = ref(false);
const useStartup = ref(false);

interface Cell {
  state: "ok" | "out" | "bad" | "restart";
}
interface Row {
  name: string;
  cells: Cell[];
  restarts: number;
}

function healthy(b: (typeof BEHAVIORS)[number], age: number): boolean {
  if (age < b.startup) return false;
  if (b.hangAt <= 0 || age < b.hangAt) return true;
  if (b.hangFor <= 0) return false;
  return age >= b.hangAt + b.hangFor;
}

interface Gate {
  ok: number;
  ng: number;
  passing: boolean;
}
function record(g: Gate, pass: boolean, fail: number, succ: number): boolean {
  if (pass) {
    g.ok++;
    g.ng = 0;
    if (!g.passing && g.ok >= succ) {
      g.passing = true;
      return true;
    }
    return false;
  }
  g.ng++;
  g.ok = 0;
  if (g.passing && g.ng >= fail) {
    g.passing = false;
    return true;
  }
  return false;
}

function simulate() {
  const livenessFail = strictLiveness.value ? 1 : 5;
  const pods = BEHAVIORS.map((b, i) => ({
    name: `pod-${i + 1}`,
    b,
    age: 0,
    restarts: 0,
    startup: { ok: 0, ng: 0, passing: false } as Gate,
    ready: { ok: 0, ng: 0, passing: false } as Gate,
    live: { ok: 0, ng: 0, passing: true } as Gate,
  }));
  const rows: Row[] = pods.map((p) => ({ name: p.name, cells: [], restarts: 0 }));
  let rr = 0;
  let served = 0;
  let dropped = 0;
  const log: string[] = [];

  for (let t = 0; t < TICKS; t++) {
    const restarted = [false, false];

    // ① 検査を回す。
    pods.forEach((p, i) => {
      const h = healthy(p.b, p.age);
      if (useStartup.value && !p.startup.passing) {
        if (!record(p.startup, h, 30, 1)) return; // まだ起動が終わっていない
        log.push(`t=${t} ${p.name} の起動検査が通った。ここから readiness と liveness が動き出す`);
      }
      if (useReadiness.value && record(p.ready, h, 1, 1)) {
        log.push(
          p.ready.passing
            ? `t=${t} ${p.name} が readiness を通った。転送先に入る`
            : `t=${t} ${p.name} が readiness に落ちた。転送先から外す(再起動はしない)`,
        );
      }
      if (record(p.live, h, livenessFail, 1) && !p.live.passing) {
        log.push(`t=${t} ${p.name} が liveness に落ちた。再起動する(${p.restarts + 1} 回目)`);
        p.age = -1; // このあと age++ されて 0 になる
        p.restarts++;
        p.startup = { ok: 0, ng: 0, passing: false };
        p.ready = { ok: 0, ng: 0, passing: false };
        p.live = { ok: 0, ng: 0, passing: true };
        restarted[i] = true;
      }
    });

    // ② リクエストを振る。
    const eps = pods.filter((p) => !useReadiness.value || p.ready.passing);
    const hit = [0, 0];
    if (eps.length === 0) {
      dropped += RPS;
    } else {
      for (let i = 0; i < RPS; i++) {
        const p = eps[rr % eps.length];
        rr++;
        const idx = pods.indexOf(p);
        if (healthy(p.b, Math.max(0, p.age))) served++;
        else {
          dropped++;
          hit[idx] = 1;
        }
      }
    }

    // ③ この周期の見え方を記録して時刻を進める。
    pods.forEach((p, i) => {
      const inEp = eps.includes(p);
      let state: Cell["state"] = "out";
      if (restarted[i]) state = "restart";
      else if (hit[i]) state = "bad";
      else if (inEp) state = "ok";
      rows[i].cells.push({ state });
      p.age++;
    });
  }
  pods.forEach((p, i) => (rows[i].restarts = p.restarts));
  return { rows, served, dropped, log, restarts: pods.reduce((a, p) => a + p.restarts, 0) };
}

const run = computed(() => simulate());
const clean = computed(() => run.value.dropped === 0 && run.value.restarts === 0);
const badge = computed(() => `失敗 ${run.value.dropped} / 再起動 ${run.value.restarts}`);
const badgeTone = computed<"ok" | "ng">(() => (clean.value ? "ok" : "ng"));
</script>

<template>
  <DemoShell title="ヘルスチェック(probe)" :badge="badge" :badge-tone="badgeTone">
    <div class="pb-row">
      <span class="pb-label">readiness(受けられるかを見る)</span>
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: !useReadiness }" @click="useReadiness = false">使わない</span>
        <span class="sd-seg-opt" :class="{ on: useReadiness }" @click="useReadiness = true">使う</span>
      </span>
    </div>
    <div class="pb-row">
      <span class="pb-label">liveness(生きているかを見る)</span>
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: strictLiveness }" @click="strictLiveness = true">厳しい(1回で再起動)</span>
        <span class="sd-seg-opt" :class="{ on: !strictLiveness }" @click="strictLiveness = false">緩い(5回続いたら)</span>
      </span>
    </div>
    <div class="pb-row">
      <span class="pb-label">起動用の検査(startup)</span>
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: !useStartup }" @click="useStartup = false">使わない</span>
        <span class="sd-seg-opt" :class="{ on: useStartup }" @click="useStartup = true">使う</span>
      </span>
    </div>

    <p class="pb-fixed mono">
      pod-1 は最初から応答できる / pod-2 は起動に 8 周期かかり、t=14 から 3 周期だけ詰まる / 毎周期 {{ RPS }} 件が届く
    </p>

    <div class="pb-grid">
      <div v-for="r in run.rows" :key="r.name" class="pb-lane">
        <span class="pb-name mono">{{ r.name }}</span>
        <span class="pb-cells">
          <span v-for="(c, i) in r.cells" :key="i" class="pb-cell" :class="c.state" />
        </span>
        <span class="pb-restarts mono">{{ r.restarts }}回</span>
      </div>
      <div class="pb-axis mono"><span>t=0</span><span>t={{ TICKS - 1 }}</span></div>
    </div>

    <div class="pb-key">
      <span class="pb-k ok">転送先にいて応答できる</span>
      <span class="pb-k out">転送先から外れている</span>
      <span class="pb-k bad">応答できないのに振られた</span>
      <span class="pb-k restart">再起動された</span>
    </div>

    <div class="pb-verdict" :class="clean ? 'ok' : 'bad'">
      <template v-if="clean">
        1 件も落ちず、再起動も 0 回: 起動中は転送先から外れ、一時的な詰まりは自力で回復した
      </template>
      <template v-else-if="run.restarts > 0 && run.dropped > 0">
        {{ run.dropped }} 件が落ち、{{ run.restarts }} 回の再起動が起きた
      </template>
      <template v-else-if="run.restarts > 0">
        {{ run.restarts }} 回の再起動が起きた: 自力で戻れる状態なのに、戻る前に殺されている
      </template>
      <template v-else>
        {{ run.dropped }} 件が落ちた: まだ応答できない Pod が転送先に入っている
      </template>
    </div>

    <div class="pb-log">
      <div class="pb-log-h">起きたこと</div>
      <div v-for="(l, i) in run.log.slice(0, 12)" :key="i" class="pb-log-line mono">{{ l }}</div>
      <div v-if="run.log.length > 12" class="pb-log-line mono">…ほか {{ run.log.length - 12 }} 行</div>
      <div v-if="run.log.length === 0" class="pb-log-line mono">(検査を使わない設定なので、何も判定されない)</div>
    </div>

    <p class="pb-legend">
      readiness を使わないと、まだ起動が終わっていない pod-2 にも順番で振られて落ちる。使えば、起動が終わるまで
      転送先に入らないので、隣の pod-1 が全部受ける。liveness を厳しくすると、起動を終える前の pod-2 を殺し続け、
      いつまでも立ち上がらない。起動用の検査を足すと、それが通るまで liveness が動かないので守られる。
      3つとも正しく設定したときだけ、失敗も再起動も 0 になる。
    </p>
  </DemoShell>
</template>

<style scoped>
.pb-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 8px;
}
.pb-label {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 200px;
}
.pb-fixed {
  margin: 10px 0 0;
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.pb-grid {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
}
.pb-lane {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 6px;
}
.pb-name {
  font-size: 11px;
  font-weight: 700;
  min-width: 44px;
}
.pb-cells {
  display: flex;
  gap: 2px;
  flex: 1;
}
.pb-cell {
  flex: 1;
  height: 18px;
  min-width: 6px;
}
.pb-cell.ok {
  background-color: var(--vp-c-green-1);
}
.pb-cell.out {
  background-color: var(--vp-c-default-soft);
  border: 1px solid var(--vp-c-divider);
}
.pb-cell.bad {
  background-color: var(--vp-c-danger-1);
}
.pb-cell.restart {
  background-color: var(--vp-c-warning-1);
}
.pb-restarts {
  font-size: 10.5px;
  color: var(--vp-c-text-3);
  min-width: 30px;
  text-align: right;
}
.pb-axis {
  display: flex;
  justify-content: space-between;
  font-size: 9.5px;
  color: var(--vp-c-text-3);
  padding: 0 40px 0 54px;
}
.pb-key {
  display: flex;
  gap: 14px;
  margin-top: 8px;
  flex-wrap: wrap;
}
.pb-k {
  font-size: 10.5px;
  padding-left: 12px;
  position: relative;
  color: var(--vp-c-text-3);
}
.pb-k::before {
  content: "";
  position: absolute;
  left: 0;
  top: 3px;
  width: 8px;
  height: 8px;
}
.pb-k.ok::before {
  background-color: var(--vp-c-green-1);
}
.pb-k.out::before {
  background-color: var(--vp-c-default-soft);
  border: 1px solid var(--vp-c-divider);
}
.pb-k.bad::before {
  background-color: var(--vp-c-danger-1);
}
.pb-k.restart::before {
  background-color: var(--vp-c-warning-1);
}
.pb-verdict {
  margin-top: 14px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
  background-color: var(--vp-c-bg-soft);
}
.pb-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.pb-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.pb-log {
  margin-top: 12px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
}
.pb-log-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 4px;
}
.pb-log-line {
  font-size: 10.5px;
  color: var(--vp-c-text-2);
  padding: 1px 0;
}
.pb-legend {
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
