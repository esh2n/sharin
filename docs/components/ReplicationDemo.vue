<script setup lang="ts">
import { reactive, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// 単一リーダーのログシッピング複製(Go 実装 distributed/replication の考え方を移植)。
// 耐久性ポリシー(async/quorum/sync)・複製ラグ・フェイルオーバー時のデータ損失を目で見る。
// リーダー1台 + レプリカ2台 = 3ノード。quorum=2、sync=3。

type Durability = "async" | "quorum" | "sync";
interface Entry {
  off: number;
  val: number; // 書き込みの通し番号(値)
}
interface Node {
  id: number; // 0=リーダー
  log: Entry[];
  reachable: boolean; // リーダーと繋がっているか(リーダー自身は常に true)
}

const state = reactive({
  durability: "async" as Durability,
  leader: { id: 0, log: [] as Entry[], reachable: true } as Node,
  replicas: [
    { id: 1, log: [] as Entry[], reachable: true },
    { id: 2, log: [] as Entry[], reachable: true },
  ] as Node[],
  committed: 0,
  seq: 0,
  lostTotal: 0,
  failedOver: false,
  note: "耐久性ポリシーを選んで書き込んでみる。レプリカを切ると挙動が変わる",
});

const DURABILITIES: { key: Durability; label: string }[] = [
  { key: "async", label: "async" },
  { key: "quorum", label: "quorum" },
  { key: "sync", label: "sync" },
];

function nodeCount() {
  return 1 + state.replicas.length;
}
function need() {
  if (state.durability === "async") return 1;
  if (state.durability === "sync") return nodeCount();
  return Math.floor(nodeCount() / 2) + 1; // quorum
}

function shipTo(rp: Node) {
  while (rp.log.length < state.leader.log.length) {
    rp.log.push(state.leader.log[rp.log.length]);
  }
}
function acksFor(off: number) {
  let acks = 1; // リーダー
  for (const rp of state.replicas) if (rp.log.length >= off) acks++;
  return acks;
}
function recomputeCommitted() {
  const n = need();
  for (let off = state.leader.log.length; off > state.committed; off--) {
    if (acksFor(off) >= n) {
      state.committed = off;
      return;
    }
  }
}

function write() {
  state.seq += 1;
  const off = state.leader.log.length + 1;
  state.leader.log.push({ off, val: state.seq });
  for (const rp of state.replicas) if (rp.reachable) shipTo(rp);
  const before = state.committed;
  recomputeCommitted();
  const committed = state.committed >= off;
  if (committed) {
    state.note = `書き込み w${state.seq}(offset ${off})は確定した(${need()}台に複製済み)`;
  } else {
    state.note = `書き込み w${state.seq}(offset ${off})は未確定。${state.durability} は ${need()}台の複製を待つが、届いたのは ${acksFor(off)}台`;
  }
  void before;
}

function toggle(id: number) {
  const rp = state.replicas.find((r) => r.id === id);
  if (!rp) return;
  rp.reachable = !rp.reachable;
  if (rp.reachable) {
    shipTo(rp);
    recomputeCommitted();
    state.note = `レプリカ${id} を再接続。欠けていたぶんを一気に追いつかせた`;
  } else {
    state.note = `レプリカ${id} を切断。以降の書き込みは届かず、ラグが開いていく`;
  }
}

function promote(id: number) {
  const rp = state.replicas.find((r) => r.id === id);
  if (!rp) return;
  const lost = Math.max(0, state.committed - rp.log.length);
  const others = state.replicas.filter((r) => r.id !== id);
  // 昇格: このレプリカが新リーダー。他は配下に残し、新リーダーに合わせる。
  state.leader = { id: rp.id, log: rp.log, reachable: true };
  state.replicas = others;
  state.committed = state.leader.log.length;
  for (const other of state.replicas) {
    other.reachable = true;
    if (other.log.length > state.leader.log.length) other.log = other.log.slice(0, state.leader.log.length);
    shipTo(other);
  }
  recomputeCommitted();
  state.lostTotal += lost;
  state.failedOver = true;
  state.note =
    lost > 0
      ? `フェイルオーバー: レプリカ${id} を昇格。確定済みだった書き込み ${lost}件が消えた(データ損失窓)`
      : `フェイルオーバー: レプリカ${id} を昇格。最新を持っていたので損失ゼロ`;
}

function reset() {
  state.leader = { id: 0, log: [], reachable: true };
  state.replicas = [
    { id: 1, log: [], reachable: true },
    { id: 2, log: [], reachable: true },
  ];
  state.committed = 0;
  state.seq = 0;
  state.lostTotal = 0;
  state.failedOver = false;
  state.note = "耐久性ポリシーを選んで書き込んでみる。レプリカを切ると挙動が変わる";
}

function setDurability(d: Durability) {
  state.durability = d;
  recomputeCommitted();
  state.note = `耐久性を ${d} に変更(確定に必要なのは ${need()}台)`;
}

const allNodes = computed(() => [state.leader, ...state.replicas]);
const maxLen = computed(() => Math.max(1, ...allNodes.value.map((n) => n.log.length)));

function roleOf(n: Node) {
  return n.id === 0 ? "leader" : "replica";
}
function lagOf(n: Node) {
  return state.leader.log.length - n.log.length;
}
function valueOf(n: Node) {
  return n.log.length ? n.log[n.log.length - 1].val : null;
}

const badge = computed(() => {
  const leaderVal = valueOf(state.leader);
  return leaderVal === null ? "書き込みなし" : `確定 ${state.committed}/${state.leader.log.length}`;
});
const badgeTone = computed<"ok" | "ng" | "neutral">(() => {
  if (state.leader.log.length === 0) return "neutral";
  return state.committed === state.leader.log.length ? "ok" : "ng";
});
</script>

<template>
  <DemoShell title="レプリケーション(リーダー + 2レプリカ)" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <div class="sd-seg">
        <span
          v-for="d in DURABILITIES"
          :key="d.key"
          class="sd-seg-opt"
          :class="{ on: state.durability === d.key }"
          @click="setDurability(d.key)"
        >
          {{ d.label }}
        </span>
      </div>
      <button class="sd-btn sd-btn--primary" @click="write">書き込み</button>
      <span class="spacer" />
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <div class="rp-grid">
      <div v-for="n in allNodes" :key="n.id" class="rp-node" :class="[roleOf(n), { off: !n.reachable }]">
        <div class="rp-head">
          <span class="rp-id">{{ n.id === 0 ? "リーダー" : "レプリカ " + n.id }}</span>
          <span class="rp-tag" :class="roleOf(n)">{{ n.id === 0 ? "Leader" : n.reachable ? "追従中" : "切断" }}</span>
        </div>
        <div class="rp-meta">
          値 {{ valueOf(n) === null ? "—" : "w" + valueOf(n) }}
          <template v-if="n.id !== 0"> ・ ラグ {{ lagOf(n) }}</template>
        </div>
        <div class="rp-log">
          <span
            v-for="i in maxLen"
            :key="i"
            class="rp-cell"
            :class="{ filled: i <= n.log.length, committed: i <= n.log.length && i <= state.committed }"
            :title="i <= n.log.length ? 'offset ' + i + ' / w' + n.log[i - 1].val : ''"
          >{{ i <= n.log.length ? n.log[i - 1].val : "" }}</span>
        </div>
        <div v-if="n.id !== 0" class="rp-actions">
          <button class="sd-btn sd-btn--sm" @click="toggle(n.id)">
            {{ n.reachable ? "切断" : "再接続" }}
          </button>
          <button class="sd-btn sd-btn--sm" @click="promote(n.id)">昇格</button>
        </div>
      </div>
    </div>

    <p class="sd-msg">{{ state.note }}</p>
    <p v-if="state.lostTotal > 0" class="rp-loss">
      これまでにフェイルオーバーで失われた確定済み書き込み: {{ state.lostTotal }}件
    </p>
    <div class="rp-legend">
      <span><i class="sw committed" />確定(耐久性条件を満たした)</span>
      <span><i class="sw filled" />複製済みだが未確定</span>
      <span>セル内・値の数字は書き込みの通し番号(w1, w2, …)</span>
    </div>
  </DemoShell>
</template>

<style scoped>
.rp-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(170px, 1fr));
  gap: 10px;
  margin-top: 14px;
}
.rp-node {
  /* 角は落とす。角丸カードに色付き左アクセントを載せる構図は使わない */
  border: 1px solid var(--vp-c-divider);
  border-left: 3px solid var(--vp-c-divider);
  padding: 10px;
  background-color: var(--vp-c-bg);
}
.rp-node.leader {
  border-color: var(--vp-c-green-2);
  border-left-color: var(--vp-c-green-2);
}
.rp-node.off {
  border-left-color: var(--vp-c-yellow-2);
}
.rp-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.rp-id {
  font-size: 13px;
  font-weight: 600;
}
.rp-tag {
  font-size: 10px;
  font-weight: 700;
  padding: 1px 7px;
  border-radius: 8px;
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-2);
}
.rp-tag.leader {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.rp-node.off .rp-tag {
  background-color: var(--vp-c-yellow-soft);
  color: var(--vp-c-yellow-1);
}
.rp-meta {
  margin-top: 6px;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.rp-log {
  display: flex;
  flex-wrap: wrap;
  gap: 3px;
  margin-top: 8px;
  min-height: 20px;
}
.rp-cell {
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
.rp-cell.filled {
  border-color: var(--vp-c-brand-2);
  color: var(--vp-c-brand-1);
}
.rp-cell.committed {
  background-color: var(--vp-c-green-soft);
  border-color: var(--vp-c-green-2);
  color: var(--vp-c-green-1);
}
.rp-actions {
  display: flex;
  gap: 6px;
  margin-top: 8px;
}
.sd-btn--sm {
  padding: 2px 10px;
  font-size: 11px;
}
.rp-loss {
  margin-top: 6px;
  font-size: 12px;
  color: var(--vp-c-red-1);
  font-weight: 600;
}
.rp-legend {
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
  margin-top: 12px;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.rp-legend .sw {
  display: inline-block;
  width: 12px;
  height: 12px;
  border-radius: 3px;
  margin-right: 5px;
  vertical-align: -2px;
  border: 1px solid var(--vp-c-divider);
}
.rp-legend .sw.filled {
  border-color: var(--vp-c-brand-2);
}
.rp-legend .sw.committed {
  background-color: var(--vp-c-green-soft);
  border-color: var(--vp-c-green-2);
}
</style>
