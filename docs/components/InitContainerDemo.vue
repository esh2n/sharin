<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/initcontainer(Go)を移植。同じ3つのコンテナを
// 「順序を宣言する」「並べただけ」の2通りで並行に動かし、
// 本体が出口なしで動いている時間を数えて比べる。

type Kind = "init" | "sidecar" | "app";
type Phase = "waiting" | "booting" | "running" | "failed" | "draining" | "done";

interface Spec {
  name: string;
  kind: Kind;
  boot: number;
  fails: number;
  drain: number;
  proxy: boolean;
}
interface C {
  spec: Spec;
  phase: Phase;
  left: number;
  attempts: number;
  backoff: number;
  hist: Phase[];
}
interface Pod {
  label: string;
  note: string;
  all: C[];
  gate: C[];
  main: C[];
  idx: number;
  terminating: boolean;
  now: number;
  exposed: number;
  expHist: boolean[];
  log: string[];
}

const MAX_TICKS = 26;

function backoffFor(attempts: number): number {
  let d = 1;
  for (let i = 1; i < attempts; i++) {
    d *= 2;
    if (d >= 8) return 8;
  }
  return d;
}

function makePod(label: string, note: string, proxyKind: Kind, fails: number): Pod {
  const specs: Spec[] = [
    { name: "config-fetch", kind: "init", boot: 2, fails, drain: 0, proxy: false },
    { name: "proxy", kind: proxyKind, boot: 3, fails: 0, drain: 1, proxy: true },
    { name: "web", kind: "app", boot: 1, fails: 0, drain: 3, proxy: false },
  ];
  const all: C[] = specs.map((s) => ({
    spec: s,
    phase: "waiting" as Phase,
    left: 0,
    attempts: 0,
    backoff: 0,
    hist: [],
  }));
  return {
    label,
    note,
    all,
    gate: all.filter((c) => c.spec.kind !== "app"),
    main: all.filter((c) => c.spec.kind === "app"),
    idx: 0,
    terminating: false,
    now: 0,
    exposed: 0,
    expHist: [],
    log: [],
  };
}

function logf(p: Pod, msg: string) {
  p.log.push(`t=${p.now} ${msg}`);
}

function settle(p: Pod, c: C) {
  if (c.attempts < c.spec.fails) {
    c.attempts++;
    c.phase = "failed";
    c.backoff = backoffFor(c.attempts);
    logf(p, `${c.spec.name} が失敗。${c.backoff} tick 待って再試行(後続は始まらない)`);
    return;
  }
  if (c.spec.kind === "init") {
    c.phase = "done";
    logf(p, `${c.spec.name} が完了`);
    return;
  }
  c.phase = "running";
  logf(p, `${c.spec.name} が稼働開始`);
}

function begin(p: Pod, c: C) {
  c.phase = "booting";
  c.left = c.spec.boot;
  logf(p, `${c.spec.name} の起動を開始`);
  if (c.left <= 0) settle(p, c);
}

function progress(p: Pod, c: C) {
  if (c.phase === "failed") {
    c.backoff--;
    if (c.backoff <= 0) {
      c.phase = "booting";
      c.left = c.spec.boot;
      logf(p, `${c.spec.name} を再試行する`);
    }
    return;
  }
  if (c.phase !== "booting") return;
  c.left--;
  if (c.left > 0) return;
  settle(p, c);
}

function startStep(p: Pod) {
  while (p.idx < p.gate.length) {
    const c = p.gate[p.idx];
    if (c.phase === "waiting") {
      begin(p, c);
      return;
    }
    progress(p, c);
    if (c.phase === "done" || (c.spec.kind === "sidecar" && c.phase === "running")) {
      p.idx++;
      continue;
    }
    return;
  }
  for (const c of p.main) {
    if (c.phase === "waiting") {
      begin(p, c);
      continue;
    }
    progress(p, c);
  }
}

function drain(p: Pod, c: C) {
  if (c.phase === "running") {
    c.phase = "draining";
    c.left = c.spec.drain;
    logf(p, `${c.spec.name} の停止を開始`);
    if (c.left <= 0) {
      c.phase = "done";
      logf(p, `${c.spec.name} が停止`);
    }
    return;
  }
  if (c.phase === "draining") {
    c.left--;
    if (c.left <= 0) {
      c.phase = "done";
      logf(p, `${c.spec.name} が停止`);
    }
    return;
  }
  if (c.phase === "waiting" || c.phase === "booting" || c.phase === "failed") {
    c.phase = "done";
  }
}

function mainDone(p: Pod): boolean {
  return p.main.every((c) => c.phase === "done");
}

function terminateStep(p: Pod) {
  for (const c of p.gate) if (c.spec.kind === "init") drain(p, c);
  for (const c of p.main) drain(p, c);
  if (!mainDone(p)) return;
  for (const c of p.gate) if (c.spec.kind === "sidecar") drain(p, c);
}

function isExposed(p: Pod): boolean {
  let live = false;
  let gateway = false;
  for (const c of p.all) {
    const alive = c.phase === "running" || c.phase === "draining";
    if (c.spec.proxy) {
      gateway = gateway || alive;
      continue;
    }
    if (c.spec.kind === "app" && alive) live = true;
  }
  return live && !gateway;
}

function tick(p: Pod) {
  if (p.terminating) terminateStep(p);
  else startStep(p);
  const ex = isExposed(p);
  if (ex) p.exposed++;
  p.expHist.push(ex);
  for (const c of p.all) c.hist.push(c.phase);
  p.now++;
}

function ready(p: Pod): boolean {
  if (p.idx < p.gate.length || p.main.length === 0) return false;
  return p.main.every((c) => c.phase === "running");
}
function finished(p: Pod): boolean {
  return p.all.every((c) => c.phase === "done");
}

const failInit = ref(false);
const left = ref<Pod>(makePod("順序を宣言する", "proxy を sidecar 枠に置く", "sidecar", 0));
const right = ref<Pod>(makePod("並べただけ", "proxy も本体と同じ枠に置く", "app", 0));
const pods = computed(() => [left.value, right.value]);

function step() {
  for (const p of pods.value) {
    if (p.now >= MAX_TICKS || finished(p)) continue;
    tick(p);
  }
}
function runToReady() {
  for (let i = 0; i < MAX_TICKS; i++) {
    if (pods.value.every((p) => ready(p) || finished(p) || p.now >= MAX_TICKS)) break;
    step();
  }
}
function terminate() {
  for (const p of pods.value) p.terminating = true;
}
function runToEnd() {
  terminate();
  for (let i = 0; i < MAX_TICKS; i++) {
    if (pods.value.every((p) => finished(p) || p.now >= MAX_TICKS)) break;
    step();
  }
}
// 起動しきったところから始める。並べただけの側で穴が空いていることが、
// 何も操作しない時点で見えているようにしたい。
function reset() {
  const f = failInit.value ? 1 : 0;
  left.value = makePod("順序を宣言する", "proxy を sidecar 枠に置く", "sidecar", f);
  right.value = makePod("並べただけ", "proxy も本体と同じ枠に置く", "app", f);
  runToReady();
}
function toggleFail() {
  failInit.value = !failInit.value;
  reset();
}
reset();

const width = computed(() => Math.max(left.value.now, right.value.now));
const cols = computed(() => Array.from({ length: width.value }, (_, i) => i));
const anyTerminating = computed(() => pods.value.some((p) => p.terminating));
const badge = computed(() => `出口なし 宣言 ${left.value.exposed} / 並べただけ ${right.value.exposed}`);
const badgeTone = computed<"ok" | "ng">(() => (right.value.exposed > left.value.exposed ? "ng" : "ok"));

const kindLabel: Record<Kind, string> = { init: "init", sidecar: "sidecar", app: "app" };
const phaseLabel: Record<Phase, string> = {
  waiting: "順番待ち",
  booting: "起動中",
  running: "稼働中",
  failed: "失敗・再試行待ち",
  draining: "停止処理中",
  done: "終了",
};
const verdict = computed(() => {
  const l = left.value.exposed;
  const r = right.value.exposed;
  if (r <= l) return "時刻を進めると差が出る。並べただけの側では、起動の速い本体が出口より先に立ち上がる";
  if (!anyTerminating.value) {
    return `並べただけの Pod は、起動の途中で本体が出口なしのまま ${r} tick 動いていた。停止させると、今度は出口のほうが先に消えて同じ穴が空く`;
  }
  return `並べただけの Pod は、起動と停止の両端で合計 ${r} tick、本体が出口なしで動いていた。順序を宣言した Pod は ${l} tick`;
});

function statusOf(p: Pod): string {
  if (finished(p)) return "全コンテナ終了";
  if (p.terminating) return "停止処理中";
  if (ready(p)) return "本体が稼働中";
  return "起動中";
}
</script>

<template>
  <DemoShell title="init container と sidecar" :badge="badge" :badge-tone="badgeTone">
    <div class="ic-actions">
      <button class="sd-btn sd-btn--primary" @click="step">1 tick 進める</button>
      <button class="sd-btn" @click="runToReady">起動しきるまで進める</button>
      <button class="sd-btn" :disabled="anyTerminating" @click="runToEnd">停止させて終わりまで進める</button>
      <span class="ic-spacer" />
      <button class="sd-btn" :class="failInit ? 'sd-btn--primary' : ''" @click="toggleFail">
        init を1回失敗させる: {{ failInit ? "する" : "しない" }}
      </button>
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <div class="ic-cols">
      <div v-for="p in pods" :key="p.label" class="ic-col" :class="p.exposed > 0 ? 'bad' : ''">
        <div class="ic-col-h">
          <span class="ic-title">{{ p.label }}</span>
          <span class="ic-note">{{ p.note }}</span>
        </div>

        <div class="ic-rows">
          <div v-for="c in p.all" :key="c.spec.name" class="ic-row">
            <span class="ic-name mono">
              {{ c.spec.name }}<em>{{ kindLabel[c.spec.kind] }}</em>
            </span>
            <span class="ic-track">
              <span
                v-for="t in cols"
                :key="t"
                class="ic-cell"
                :class="c.hist[t] ? 'ph-' + c.hist[t] : 'ph-none'"
                :title="'t=' + t + ' ' + (c.hist[t] ? phaseLabel[c.hist[t]] : '')"
              />
            </span>
            <span class="ic-now mono">{{ phaseLabel[c.phase] }}</span>
          </div>

          <div class="ic-row ic-exp">
            <span class="ic-name mono">出口なし</span>
            <span class="ic-track">
              <span v-for="t in cols" :key="t" class="ic-cell" :class="p.expHist[t] ? 'exp-hit' : 'exp-ok'" />
            </span>
            <span class="ic-now mono">{{ p.exposed }} tick</span>
          </div>
        </div>

        <div class="ic-state mono">t={{ p.now }} ・ {{ statusOf(p) }}</div>

        <div class="ic-log">
          <div v-for="(l, i) in p.log.slice(-5)" :key="i" class="ic-log-line mono">{{ l }}</div>
          <div v-if="p.log.length === 0" class="ic-empty">(まだ何も起きていない)</div>
        </div>
      </div>
    </div>

    <div class="ic-verdict" :class="right.exposed > left.exposed ? 'bad' : 'ok'">{{ verdict }}</div>

    <p class="ic-legend">
      同じ3つのコンテナを2通りの書き方で並行に動かしている。違いは proxy をどの枠に置くかだけ。
      帯の色は各時刻の状態で、薄い灰が順番待ち、黄が起動中と停止処理中、緑が稼働中、赤が失敗。
      いちばん下の行が、本体が生きているのに出口が居ない時刻を表す。左は起動が1 tick 遅い代わりに
      この行が空のままで、右は起動が速い代わりに両端が埋まる。
      「init を1回失敗させる」を入れると、後続が1つも始まらないまま待ち時間が伸びる様子が見える。
    </p>
  </DemoShell>
</template>

<style scoped>
.ic-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.ic-spacer {
  flex: 1;
}
.ic-cols {
  display: flex;
  gap: 10px;
  margin-top: 14px;
  flex-wrap: wrap;
}
.ic-col {
  flex: 1;
  min-width: 280px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
}
.ic-col.bad {
  border-color: var(--vp-c-danger-1);
}
.ic-col-h {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-bottom: 10px;
  flex-wrap: wrap;
}
.ic-title {
  font-size: 12px;
  font-weight: 700;
  color: var(--vp-c-text-1);
}
.ic-note {
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.ic-rows {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.ic-row {
  display: flex;
  align-items: center;
  gap: 6px;
}
.ic-name {
  width: 92px;
  flex: none;
  font-size: 10px;
  color: var(--vp-c-text-2);
  display: flex;
  flex-direction: column;
  line-height: 1.3;
}
.ic-name em {
  font-style: normal;
  font-size: 9px;
  color: var(--vp-c-text-3);
}
.ic-track {
  display: flex;
  gap: 1px;
  flex: 1;
  min-width: 0;
}
.ic-cell {
  flex: 1;
  min-width: 3px;
  height: 14px;
  border: 1px solid transparent;
}
.ph-none {
  background-color: transparent;
}
.ph-waiting {
  background-color: var(--vp-c-bg-alt);
  border-color: var(--vp-c-divider);
}
.ph-booting,
.ph-draining {
  background-color: var(--vp-c-warning-soft);
  border-color: var(--vp-c-warning-1);
}
.ph-running {
  background-color: var(--vp-c-green-soft);
  border-color: var(--vp-c-green-1);
}
.ph-failed {
  background-color: var(--vp-c-danger-soft);
  border-color: var(--vp-c-danger-1);
}
.ph-done {
  background-color: var(--vp-c-bg-alt);
}
.ic-exp {
  margin-top: 5px;
  padding-top: 5px;
  border-top: 1px solid var(--vp-c-divider);
}
.exp-ok {
  background-color: var(--vp-c-bg-alt);
}
.exp-hit {
  background-color: var(--vp-c-danger-1);
}
.ic-now {
  width: 74px;
  flex: none;
  font-size: 9.5px;
  color: var(--vp-c-text-3);
  text-align: right;
}
.ic-state {
  margin-top: 8px;
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.ic-log {
  margin-top: 8px;
  border-top: 1px solid var(--vp-c-divider);
  padding-top: 6px;
  min-height: 56px;
}
.ic-log-line {
  font-size: 10px;
  color: var(--vp-c-text-2);
  padding: 1px 0;
}
.ic-empty {
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.ic-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  font-weight: 600;
  background-color: var(--vp-c-bg-soft);
}
.ic-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.ic-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.ic-legend {
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
