<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// foundations/eventloop(Go)の epoll 風イベントループをブラウザで動かす移植版。
// FD はノンブロッキング、poll(epoll_wait)で今 ready な接続をまとめて受け取り、
// 1 本のループがそれらを順に dispatch(read→echo)する。到着は tick で予定して決定化。

type Phase = "poll" | "dispatch" | "idle" | "done";
interface ConnSnap {
  name: string;
  recv: string; // 受信済み・未読
  sent: string; // これまでエコーした全バイト
}
interface Frame {
  clock: number;
  phase: Phase;
  note: string;
  ready: number[]; // ready な接続 index
  active: number; // dispatch 対象の接続 index(それ以外は -1)
  conns: ConnSnap[];
}

const CONNS = ["c1", "c2", "c3"];
// 到着スケジュール(決定的)。t=1 は c2/c3 同時到着、t=2 は誰も来ず眠る、t=3 で c1 に追加。
const ARRIVALS = [
  { at: 0, fd: 0, data: "GET" },
  { at: 1, fd: 1, data: "PI" },
  { at: 1, fd: 2, data: "PING" },
  { at: 3, fd: 0, data: "!" },
];

function fmtReady(ready: number[], conns: ConnSnap[]): string {
  if (ready.length === 0) return "{}";
  return "{" + ready.map((i) => conns[i].name + ":r").join(", ") + "}";
}

// Go の Loop.Run を1周ずつ・1 dispatch ずつスナップショットしながら再現する。
function simulate(): Frame[] {
  const conns: ConnSnap[] = CONNS.map((name) => ({ name, recv: "", sent: "" }));
  let world = ARRIVALS.map((a) => ({ ...a }));
  let clock = 0;
  const frames: Frame[] = [];
  const snap = (phase: Phase, note: string, ready: number[], active: number) =>
    frames.push({
      clock,
      phase,
      note,
      ready: [...ready],
      active,
      conns: conns.map((c) => ({ ...c })),
    });

  let guard = 0;
  while (guard++ < 100) {
    // 外界を反映(その時刻までの到着を受信バッファへ)。
    for (const e of world.filter((e) => e.at <= clock)) conns[e.fd].recv += e.data;
    world = world.filter((e) => e.at > clock);

    const ready = conns.map((_, i) => i).filter((i) => conns[i].recv.length > 0);
    if (ready.length === 0) {
      if (world.length === 0) {
        snap("done", "ready も未来の到着も無い — 全接続をさばき終わった", [], -1);
        break;
      }
      const next = Math.min(...world.map((e) => e.at));
      snap("idle", `ready 無し — epoll_wait は t=${next} の到着まで眠る(CPU を手放す)`, [], -1);
      clock = next;
      continue;
    }

    // epoll_wait が今 ready な接続をまとめて返す。
    snap("poll", `epoll_wait → ready ${fmtReady(ready, conns)}`, ready, -1);
    // 1 本のループが ready な接続を順に処理(read→echo)する。
    for (const i of ready) {
      const data = conns[i].recv;
      conns[i].recv = "";
      conns[i].sent += data;
      snap("dispatch", `${conns[i].name}: read "${data}" → そのまま echo で書き返す`, ready, i);
    }
    clock++;
  }
  return frames;
}

const frames = simulate();
const at = ref(0); // マウント時は先頭フレーム(clock=0, 最初の poll)で決定的

const cur = computed(() => frames[at.value]);
const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < frames.length - 1);
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
  at.value = frames.length - 1;
}

const phaseLabel: Record<Phase, string> = {
  poll: "poll(epoll_wait)",
  dispatch: "dispatch",
  idle: "idle(眠り)",
  done: "done",
};
const badge = computed(() => {
  const f = cur.value;
  if (f.phase === "dispatch") return `t=${f.clock} · dispatch ${f.conns[f.active].name}`;
  if (f.phase === "idle") return `t=${f.clock} · idle`;
  if (f.phase === "done") return "完了";
  return `t=${f.clock} · poll`;
});
const badgeTone = computed<"ok" | "neutral">(() => (cur.value.phase === "done" ? "ok" : "neutral"));

// 接続の表示状態: dispatch 対象 / ready(準備済み) / idle。
function connState(i: number): "active" | "ready" | "idle" {
  const f = cur.value;
  if (f.active === i) return "active";
  if (f.ready.includes(i) && f.phase !== "done") return "ready";
  return "idle";
}
function chars(s: string): string[] {
  return s.split("");
}
</script>

<template>
  <DemoShell title="event-loop(epoll 風 I/O 多重化)" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">1手すすめる</button>
      <span class="spacer" />
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="el-count">{{ at + 1 }} / {{ frames.length }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <!-- イベントループの状態: 論理時計 + フェーズ + epoll_wait の結果 -->
    <div class="el-loop" :class="cur.phase">
      <div class="el-loop-head">
        <span class="el-clock">t = {{ cur.clock }}</span>
        <span class="el-phase" :class="cur.phase">{{ phaseLabel[cur.phase] }}</span>
      </div>
      <div class="el-ready">
        <span class="el-ready-label">epoll_wait →</span>
        <template v-if="cur.ready.length">
          <span v-for="i in cur.ready" :key="i" class="el-rchip" :class="{ on: cur.active === i }">
            {{ cur.conns[i].name }}:r
          </span>
        </template>
        <span v-else class="el-ready-empty">ready 無し(眠る)</span>
      </div>
    </div>

    <!-- 接続(FD)たち。1 本のループがこの中の ready なものを渡り歩く -->
    <div class="el-conns">
      <div v-for="(c, i) in cur.conns" :key="c.name" class="el-conn" :class="connState(i)">
        <div class="el-conn-name">
          {{ c.name }}
          <span v-if="connState(i) === 'active'" class="el-tag active">処理中</span>
          <span v-else-if="connState(i) === 'ready'" class="el-tag ready">ready</span>
        </div>
        <div class="el-io">
          <span class="el-io-label">recv</span>
          <div class="el-bytes">
            <template v-if="c.recv">
              <span v-for="(ch, k) in chars(c.recv)" :key="k" class="el-byte in">{{ ch }}</span>
            </template>
            <span v-else class="el-empty">—</span>
          </div>
        </div>
        <div class="el-io">
          <span class="el-io-label">echo</span>
          <div class="el-bytes">
            <template v-if="c.sent">
              <span v-for="(ch, k) in chars(c.sent)" :key="k" class="el-byte out">{{ ch }}</span>
            </template>
            <span v-else class="el-empty">—</span>
          </div>
        </div>
      </div>
    </div>

    <p class="el-note">{{ cur.note }}</p>

    <div class="el-legend">
      <span class="el-lrow"><span class="el-sw in" />recv: 到着済み・未読(read できる)</span>
      <span class="el-lrow"><span class="el-sw out" />echo: 書き返したバイト</span>
      <span class="el-lrow"><span class="el-sw active-sw" />処理中 = いまループが dispatch している接続</span>
    </div>
  </DemoShell>
</template>

<style scoped>
.el-count {
  font-size: 12px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-2);
  min-width: 44px;
  text-align: center;
}

/* イベントループの状態パネル。角は落とす(anti-slop) */
.el-loop {
  margin-top: 16px;
  border: 1px solid var(--vp-c-divider);
  border-left: 3px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 12px 14px;
  background-color: var(--vp-c-bg);
}
.el-loop.poll,
.el-loop.dispatch {
  border-left-color: var(--vp-c-brand-1);
}
.el-loop.idle {
  border-left-color: var(--vp-c-text-3);
}
.el-loop.done {
  border-left-color: var(--vp-c-green-1);
}
.el-loop-head {
  display: flex;
  align-items: center;
  gap: 12px;
}
.el-clock {
  font-family: var(--vp-font-family-mono);
  font-weight: 700;
  font-size: 14px;
  color: var(--vp-c-text-1);
}
.el-phase {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  padding: 2px 8px;
  border-radius: 4px;
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-2);
}
.el-phase.poll,
.el-phase.dispatch {
  background-color: var(--vp-c-brand-soft);
  color: var(--vp-c-brand-1);
}
.el-phase.done {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.el-ready {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 10px;
}
.el-ready-label {
  font-size: 12px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-2);
}
.el-rchip {
  font-size: 12px;
  font-family: var(--vp-font-family-mono);
  padding: 2px 8px;
  border-radius: 4px;
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-1);
}
.el-rchip.on {
  background-color: var(--vp-c-brand-1);
  color: var(--vp-c-bg);
  font-weight: 700;
}
.el-ready-empty {
  font-size: 12px;
  color: var(--vp-c-text-3);
  font-style: italic;
}

/* 接続の行。左レールで状態を示す。角は落とす(anti-slop 準拠) */
.el-conns {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 14px;
}
.el-conn {
  display: grid;
  grid-template-columns: 84px 1fr;
  align-items: center;
  gap: 8px 12px;
  padding: 10px 12px;
  border: 1px solid var(--vp-c-divider);
  border-left: 3px solid transparent;
  border-radius: 0;
  background-color: var(--vp-c-bg);
}
.el-conn.ready {
  border-left-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-bg-soft);
}
.el-conn.active {
  border-left-color: var(--vp-c-brand-1);
  border-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-brand-soft);
}
.el-conn.idle {
  opacity: 0.62;
}
.el-conn-name {
  grid-row: 1 / span 2;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
  font-family: var(--vp-font-family-mono);
  font-weight: 700;
  font-size: 14px;
  color: var(--vp-c-text-1);
}
.el-tag {
  font-size: 10px;
  font-weight: 700;
  padding: 1px 6px;
  border-radius: 4px;
}
.el-tag.ready {
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-brand-1);
}
.el-tag.active {
  background-color: var(--vp-c-brand-1);
  color: var(--vp-c-bg);
}
.el-io {
  display: flex;
  align-items: center;
  gap: 10px;
}
.el-io-label {
  font-size: 11px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-3);
  width: 34px;
  flex: 0 0 auto;
}
.el-bytes {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.el-byte {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 22px;
  font-size: 12px;
  font-family: var(--vp-font-family-mono);
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
}
.el-byte.in {
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-1);
}
.el-byte.out {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
  border-color: transparent;
}
.el-empty {
  font-size: 12px;
  color: var(--vp-c-text-3);
}
.el-note {
  margin: 14px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  font-family: var(--vp-font-family-mono);
}
.el-legend {
  display: flex;
  flex-direction: column;
  gap: 5px;
  margin-top: 14px;
  padding-top: 12px;
  border-top: 1px solid var(--vp-c-divider);
}
.el-lrow {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.el-sw {
  flex: 0 0 auto;
  width: 16px;
  height: 16px;
  border-radius: 3px;
  border: 1px solid var(--vp-c-divider);
}
.el-sw.in {
  background-color: var(--vp-c-default-soft);
}
.el-sw.out {
  background-color: var(--vp-c-green-soft);
  border-color: transparent;
}
.el-sw.active-sw {
  background-color: var(--vp-c-brand-1);
  border-color: transparent;
}
</style>
