<script setup lang="ts">
import { reactive, computed, onMounted, onUnmounted } from "vue";
import DemoShell from "./DemoShell.vue";

// ブラウザ上で動く Raft クラスタの縮小版(Go 実装 distributed/raft の考え方を移植)。
// 選挙・ログ複製・ネットワーク分断による split brain 防止を目で見えるようにする。
// スナップショットやメンバ変更は省き、選挙と複製に集中する。

type Role = "follower" | "candidate" | "leader";
interface Entry {
  term: number;
  value: string;
}
interface Msg {
  type: "vote" | "voteResp" | "app" | "appResp";
  from: number;
  to: number;
  term: number;
  lastIndex?: number;
  lastTerm?: number;
  prevIndex?: number;
  prevTerm?: number;
  entries?: Entry[];
  commit?: number;
  reject?: boolean;
  matchIndex?: number;
}
interface Node {
  id: number;
  role: Role;
  term: number;
  votedFor: number | null;
  log: Entry[]; // 0始まり配列。位置 i のエントリの Index は i+1
  commit: number; // 確定済みエントリ数
  elapsed: number;
  timeout: number;
  votes: Set<number>;
  next: Record<number, number>; // 各追従者へ次に送る Index(1始まり)
  match: Record<number, number>;
}

// vrt: VRT(見た目回帰テスト)用。値が渡ると自動 tick を止め、その回数だけ
// 決定的にステップを進めた静止画を描く。seed 固定 LCG なので毎回同じ絵になる。
const props = defineProps<{ vrt?: number }>();

const N = 5;
const HEARTBEAT = 1;
const state = reactive({
  nodes: [] as Node[],
  partitioned: false,
  groups: [] as number[][],
  running: true,
  tick: 0,
  seq: 0, // 書き込みの通し番号
  note: "起動中。しばらくすると誰かがリーダーに立候補する",
});

let rngState = 987654321;
function rnd() {
  // 決定的な線形合同法。毎回同じ挙動を再現できるようにする
  rngState = (1103515245 * rngState + 12345) & 0x7fffffff;
  return rngState / 0x7fffffff;
}
function newTimeout() {
  return 6 + Math.floor(rnd() * 6); // [6,12) ティック
}

function init() {
  state.nodes = [];
  for (let i = 1; i <= N; i++) {
    state.nodes.push({
      id: i,
      role: "follower",
      term: 0,
      votedFor: null,
      log: [],
      commit: 0,
      elapsed: 0,
      timeout: newTimeout(),
      votes: new Set(),
      next: {},
      match: {},
    });
  }
  state.partitioned = false;
  state.groups = [];
  state.tick = 0;
  state.seq = 0;
  state.note = "起動中。しばらくすると誰かがリーダーに立候補する";
}

function node(id: number) {
  return state.nodes.find((n) => n.id === id)!;
}
function quorum() {
  return Math.floor(N / 2) + 1;
}

// reachable は from→to にメッセージが届くか(分断されていなければ常に届く)。
function reachable(from: number, to: number) {
  if (!state.partitioned) return true;
  for (const g of state.groups) {
    if (g.includes(from) && g.includes(to)) return true;
  }
  return false;
}

let outbox: Msg[] = [];
function send(m: Msg) {
  outbox.push(m);
}

function lastTerm(n: Node) {
  return n.log.length ? n.log[n.log.length - 1].term : 0;
}
function upToDate(n: Node, idx: number, term: number) {
  const mt = lastTerm(n);
  return term > mt || (term === mt && idx >= n.log.length);
}

function becomeFollower(n: Node, term: number) {
  n.role = "follower";
  if (term > n.term) {
    n.term = term;
    n.votedFor = null;
  }
  n.elapsed = 0;
  n.timeout = newTimeout();
  n.votes = new Set();
}

function becomeCandidate(n: Node) {
  n.role = "candidate";
  n.term += 1;
  n.votedFor = n.id;
  n.votes = new Set([n.id]);
  n.elapsed = 0;
  n.timeout = newTimeout();
  state.note = `node ${n.id} が任期 ${n.term} で立候補`;
  if (quorum() === 1) becomeLeader(n);
  for (const p of state.nodes) {
    if (p.id === n.id) continue;
    send({ type: "vote", from: n.id, to: p.id, term: n.term, lastIndex: n.log.length, lastTerm: lastTerm(n) });
  }
}

function becomeLeader(n: Node) {
  n.role = "leader";
  n.elapsed = 0;
  for (const p of state.nodes) {
    n.next[p.id] = n.log.length + 1;
    n.match[p.id] = 0;
  }
  n.match[n.id] = n.log.length;
  state.note = `node ${n.id} が任期 ${n.term} のリーダーに就任`;
  bcastAppend(n);
}

function bcastAppend(n: Node) {
  for (const p of state.nodes) {
    if (p.id === n.id) continue;
    const prevIndex = n.next[p.id] - 1;
    const prevTerm = prevIndex > 0 ? n.log[prevIndex - 1].term : 0;
    send({
      type: "app",
      from: n.id,
      to: p.id,
      term: n.term,
      prevIndex,
      prevTerm,
      entries: n.log.slice(prevIndex),
      commit: n.commit,
    });
  }
}

function handle(n: Node, m: Msg) {
  if (m.term > n.term && (m.type === "app" || m.type === "vote" || m.type === "voteResp" || m.type === "appResp")) {
    becomeFollower(n, m.term);
  }
  if (m.term < n.term) {
    if (m.type === "vote") send({ type: "voteResp", from: n.id, to: m.from, term: n.term, reject: true });
    return;
  }
  switch (m.type) {
    case "vote": {
      const canVote = n.votedFor === null || n.votedFor === m.from;
      if (canVote && upToDate(n, m.lastIndex!, m.lastTerm!)) {
        n.votedFor = m.from;
        n.elapsed = 0;
        send({ type: "voteResp", from: n.id, to: m.from, term: n.term, reject: false });
      } else {
        send({ type: "voteResp", from: n.id, to: m.from, term: n.term, reject: true });
      }
      break;
    }
    case "voteResp": {
      if (n.role !== "candidate") break;
      if (!m.reject) n.votes.add(m.from);
      if (n.votes.size >= quorum()) becomeLeader(n);
      break;
    }
    case "app": {
      becomeFollower(n, m.term);
      n.elapsed = 0;
      const pi = m.prevIndex!;
      const ok = pi === 0 || (pi <= n.log.length && n.log[pi - 1].term === m.prevTerm);
      if (!ok) {
        send({ type: "appResp", from: n.id, to: m.from, term: n.term, reject: true, matchIndex: 0 });
        break;
      }
      // prevIndex の直後から突き合わせて上書き
      const ents = m.entries ?? [];
      for (let i = 0; i < ents.length; i++) {
        const idx = pi + 1 + i;
        if (idx <= n.log.length && n.log[idx - 1].term === ents[i].term) continue;
        n.log = n.log.slice(0, idx - 1).concat(ents.slice(i));
        break;
      }
      const last = pi + ents.length;
      if (m.commit! > n.commit) n.commit = Math.min(m.commit!, last);
      send({ type: "appResp", from: n.id, to: m.from, term: n.term, reject: false, matchIndex: last });
      break;
    }
    case "appResp": {
      if (n.role !== "leader") break;
      if (m.reject) {
        n.next[m.from] = Math.max(1, n.next[m.from] - 1);
        const prevIndex = n.next[m.from] - 1;
        const prevTerm = prevIndex > 0 ? n.log[prevIndex - 1].term : 0;
        send({ type: "app", from: n.id, to: m.from, term: n.term, prevIndex, prevTerm, entries: n.log.slice(prevIndex), commit: n.commit });
        break;
      }
      n.match[m.from] = Math.max(n.match[m.from], m.matchIndex!);
      n.next[m.from] = n.match[m.from] + 1;
      maybeCommit(n);
      break;
    }
  }
}

// maybeCommit は過半数に複製された位置まで commit を進める(現任期のエントリに限る)。
function maybeCommit(n: Node) {
  const matches = state.nodes.map((p) => (p.id === n.id ? n.log.length : n.match[p.id] ?? 0));
  matches.sort((a, b) => b - a);
  const mci = matches[quorum() - 1];
  if (mci > n.commit && n.log[mci - 1] && n.log[mci - 1].term === n.term) {
    n.commit = mci;
  }
}

function step() {
  state.tick += 1;
  outbox = [];
  for (const n of state.nodes) {
    if (n.role === "leader") {
      n.elapsed += 1;
      if (n.elapsed >= HEARTBEAT) {
        n.elapsed = 0;
        bcastAppend(n);
      }
    } else {
      n.elapsed += 1;
      if (n.elapsed >= n.timeout) becomeCandidate(n);
    }
  }
  // メッセージを届ききる(1ティックで決着させる)
  let guard = 0;
  while (outbox.length && guard++ < 5000) {
    const batch = outbox;
    outbox = [];
    for (const m of batch) {
      if (reachable(m.from, m.to)) handle(node(m.to), m);
    }
  }
}

// --- 操作 ---
function propose() {
  const leaders = state.nodes.filter((n) => n.role === "leader");
  if (!leaders.length) {
    state.note = "リーダー不在。選挙が終わるまで書き込めない";
    return;
  }
  state.seq += 1;
  const v = "w" + state.seq;
  // すべてのリーダーに書き込む(分断中は少数派リーダーにも積まれるが、確定はしない=split brain の観察)
  for (const l of leaders) {
    l.log.push({ term: l.term, value: v });
    l.match[l.id] = l.log.length;
    maybeCommit(l);
    bcastAppend(l);
  }
  state.note = `書き込み ${v} をリーダーに提案`;
}

function togglePartition() {
  if (state.partitioned) {
    state.partitioned = false;
    state.groups = [];
    state.note = "分断を復旧。少数派は多数派のリーダーに追従し、ログが揃っていく";
    return;
  }
  // 現リーダー + 1台を少数派に、残り3台を多数派に割る
  const leader = state.nodes.find((n) => n.role === "leader");
  const lead = leader ? leader.id : 1;
  const others = state.nodes.map((n) => n.id).filter((id) => id !== lead);
  const minority = [lead, others[0]];
  const majority = others.slice(1);
  state.groups = [minority, majority];
  state.partitioned = true;
  state.note = `分断: 少数派 {${minority.join(",")}} と 多数派 {${majority.join(",")}}。少数派のリーダーは確定できなくなる`;
}

function reset() {
  init();
}

// --- 表示用 ---
const leaderInfo = computed(() => {
  const ls = state.nodes.filter((n) => n.role === "leader");
  if (ls.length === 0) return { text: "選挙中", tone: "ng" as const };
  if (ls.length === 1) return { text: `リーダー node ${ls[0].id} / 任期 ${ls[0].term}`, tone: "ok" as const };
  return { text: `リーダー ${ls.length}人(分断中)`, tone: "neutral" as const };
});
const maxLen = computed(() => Math.max(1, ...state.nodes.map((n) => n.log.length)));
function groupOf(id: number) {
  if (!state.partitioned) return null;
  return state.groups.findIndex((g) => g.includes(id));
}

let timer: number | undefined;
onMounted(() => {
  init();
  if (props.vrt !== undefined) {
    // VRT: 自動 tick を回さず、決まった回数だけ進めて静止させる
    state.running = false;
    for (let i = 0; i < props.vrt; i++) step();
    return;
  }
  timer = window.setInterval(() => {
    if (state.running) step();
  }, 700);
});
onUnmounted(() => window.clearInterval(timer));
</script>

<template>
  <DemoShell title="Raft クラスタ(5ノード)" :badge="leaderInfo.text" :badge-tone="leaderInfo.tone">
    <div class="sd-controls">
      <button class="sd-btn sd-btn--primary" @click="propose">書き込みを追加</button>
      <button class="sd-btn" @click="togglePartition">
        {{ state.partitioned ? "分断を復旧" : "ネットワークを分断" }}
      </button>
      <button class="sd-btn" @click="state.running = !state.running">
        {{ state.running ? "一時停止" : "再開" }}
      </button>
      <span class="spacer" />
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <div class="rf-grid">
      <div
        v-for="n in state.nodes"
        :key="n.id"
        class="rf-node"
        :class="[n.role, groupOf(n.id) !== null ? 'g' + groupOf(n.id) : '']"
      >
        <div class="rf-node-head">
          <span class="rf-id">node {{ n.id }}</span>
          <span class="rf-role" :class="n.role">{{
            n.role === "leader" ? "Leader" : n.role === "candidate" ? "Candidate" : "Follower"
          }}</span>
        </div>
        <div class="rf-meta">任期 {{ n.term }} ・ 確定 {{ n.commit }}/{{ n.log.length }}</div>
        <div class="rf-log">
          <span
            v-for="i in maxLen"
            :key="i"
            class="rf-cell"
            :class="{ filled: i <= n.log.length, committed: i <= n.commit }"
            :title="i <= n.log.length ? '任期' + n.log[i - 1].term + ' / ' + n.log[i - 1].value : ''"
          >{{ i <= n.log.length ? n.log[i - 1].term : "" }}</span>
        </div>
      </div>
    </div>

    <p class="sd-msg">{{ state.note }}</p>
    <div class="rf-legend">
      <span><i class="sw committed" />確定(過半数に複製済み)</span>
      <span><i class="sw filled" />未確定(複製途中)</span>
      <span>セル内の数字は、そのエントリが積まれた任期</span>
    </div>
  </DemoShell>
</template>

<style scoped>
.rf-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 10px;
  margin-top: 14px;
}
.rf-node {
  /* 角は落とす。角丸カードに色付き左アクセントを載せる構図は使わない */
  border: 1px solid var(--vp-c-divider);
  border-left: 3px solid var(--vp-c-divider);
  padding: 10px;
  background-color: var(--vp-c-bg);
  transition: border-color 0.2s, box-shadow 0.2s;
}
.rf-node.leader {
  border-color: var(--vp-c-green-2);
  border-left-color: var(--vp-c-green-2);
}
.rf-node.candidate {
  border-color: var(--vp-c-yellow-2);
  border-left-color: var(--vp-c-yellow-2);
}
/* 分断中は島ごとに左端の色を変える(角丸なしなので縦の直線バーになる) */
.rf-node.g0 {
  border-left-color: var(--vp-c-purple-2);
}
.rf-node.g1 {
  border-left-color: var(--vp-c-brand-2);
}
.rf-node-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.rf-id {
  font-size: 13px;
  font-weight: 600;
}
.rf-role {
  font-size: 10px;
  font-weight: 700;
  padding: 1px 7px;
  border-radius: 8px;
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-2);
}
.rf-role.leader {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.rf-role.candidate {
  background-color: var(--vp-c-yellow-soft);
  color: var(--vp-c-yellow-1);
}
.rf-meta {
  margin-top: 6px;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.rf-log {
  display: flex;
  flex-wrap: wrap;
  gap: 3px;
  margin-top: 8px;
  min-height: 20px;
}
.rf-cell {
  width: 18px;
  height: 18px;
  border-radius: 3px;
  border: 1px solid var(--vp-c-divider);
  font-size: 10px;
  line-height: 16px;
  text-align: center;
  color: var(--vp-c-text-3);
  font-family: var(--vp-font-family-mono);
}
.rf-cell.filled {
  border-color: var(--vp-c-brand-2);
  color: var(--vp-c-brand-1);
}
.rf-cell.committed {
  background-color: var(--vp-c-green-soft);
  border-color: var(--vp-c-green-2);
  color: var(--vp-c-green-1);
}
.rf-legend {
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
  margin-top: 12px;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.rf-legend .sw {
  display: inline-block;
  width: 12px;
  height: 12px;
  border-radius: 3px;
  margin-right: 5px;
  vertical-align: -2px;
  border: 1px solid var(--vp-c-divider);
}
.rf-legend .sw.filled {
  border-color: var(--vp-c-brand-2);
}
.rf-legend .sw.committed {
  background-color: var(--vp-c-green-soft);
  border-color: var(--vp-c-green-2);
}
</style>
