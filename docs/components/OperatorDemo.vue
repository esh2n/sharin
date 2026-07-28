<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/operator(Go)を移植。自作リソースの Spec と Status を並べ、
// 1回の調整で1手ずつ差が埋まる様子と、障害からの自己修復を見せる。

interface Member {
  name: string;
  ready: boolean;
}

const wantMembers = ref(3);
const restoreFrom = ref("backup-2026-07-28");
const backupExists = ref(true);

const members = ref<Member[]>([]);
const seq = ref(0);
const phase = ref("Pending");
const leader = ref("");
const restored = ref(false);
const log = ref<string[]>([]);
const lastAction = ref("");

const readyMembers = computed(() => members.value.filter((m) => m.ready));

function reconcile() {
  // ① 復元が済んでいなければ、まずそれを埋める。
  if (restoreFrom.value !== "" && !restored.value) {
    if (!backupExists.value) {
      phase.value = "Degraded";
      lastAction.value = "noop";
      log.value = [...log.value, `復元元 ${restoreFrom.value} が見つからない`];
      return;
    }
    restored.value = true;
    phase.value = "Restoring";
    lastAction.value = "restore";
    log.value = [...log.value, `${restoreFrom.value} から復元した`];
    return;
  }

  // ② メンバーが足りなければ1つ作る。
  if (members.value.length < wantMembers.value) {
    seq.value++;
    const m = { name: `db-${seq.value}`, ready: false };
    members.value = [...members.value, m];
    phase.value = "Creating";
    lastAction.value = "create";
    log.value = [...log.value, `${m.name} を作成`];
    return;
  }

  // ③ 全員が立ち上がるまで待つ。
  if (readyMembers.value.length < wantMembers.value) {
    phase.value = "Creating";
    lastAction.value = "noop";
    return;
  }

  // ④ リーダーが居ないか消えていれば選び直す。
  const aliveLeader = members.value.find((m) => m.name === leader.value && m.ready);
  if (!aliveLeader) {
    if (leader.value) log.value = [...log.value, `リーダー ${leader.value} が居なくなった`];
    const sorted = [...readyMembers.value].sort((a, b) => (a.name < b.name ? -1 : 1));
    leader.value = sorted[0].name;
    phase.value = "Creating";
    lastAction.value = "elect";
    log.value = [...log.value, `${leader.value} をリーダーに選出`];
    return;
  }

  // ⑤ 差が無い。
  phase.value = "Ready";
  lastAction.value = "noop";
}

function step() {
  reconcile();
  for (const m of members.value) m.ready = true; // 作ったものが立ち上がる
  members.value = [...members.value];
}
function run() {
  for (let i = 0; i < 15; i++) {
    step();
    if (phase.value === "Ready" || phase.value === "Degraded") break;
  }
}
function killLeader() {
  if (!leader.value) return;
  members.value = members.value.filter((m) => m.name !== leader.value);
  log.value = [...log.value, `${leader.value} が落ちた(まだ調整していない)`];
  phase.value = "Degraded";
}
function killAll() {
  members.value = [];
  log.value = [...log.value, "全メンバーが落ちた(まだ調整していない)"];
  phase.value = "Degraded";
}
function reset() {
  members.value = [];
  seq.value = 0;
  phase.value = "Pending";
  leader.value = "";
  restored.value = false;
  log.value = [];
  lastAction.value = "";
}

const converged = computed(() => phase.value === "Ready");
const degraded = computed(() => phase.value === "Degraded");
const badge = computed(() => `${phase.value} ・ ${readyMembers.value.length}/${wantMembers.value} メンバー`);
const badgeTone = computed<"ok" | "ng" | "neutral">(() =>
  degraded.value ? "ng" : converged.value ? "ok" : "neutral",
);
const actionLabel = computed(() => {
  switch (lastAction.value) {
    case "restore":
      return "restore(復元した)";
    case "create":
      return "create(メンバーを1つ作った)";
    case "elect":
      return "elect(リーダーを選んだ)";
    case "noop":
      return "何もしなかった";
    default:
      return "まだ調整していない";
  }
});
</script>

<template>
  <DemoShell title="Operatorパターン" :badge="badge" :badge-tone="badgeTone">
    <div class="op-actions">
      <button class="sd-btn sd-btn--primary" @click="step">1回調整する</button>
      <button class="sd-btn" @click="run">揃うまで調整する</button>
      <button class="sd-btn" @click="killLeader">リーダーを落とす</button>
      <button class="sd-btn" @click="killAll">全メンバーを落とす</button>
      <span class="op-spacer" />
      <button class="sd-btn" @click="reset">リセット</button>
    </div>
    <div class="op-actions op-second">
      <span class="op-label">欲しいメンバー数</span>
      <span class="sd-seg">
        <span v-for="n in [2, 3, 4]" :key="n" class="sd-seg-opt" :class="{ on: wantMembers === n }" @click="wantMembers = n">{{ n }}</span>
      </span>
      <span class="op-label">バックアップ</span>
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: backupExists }" @click="backupExists = true">ある</span>
        <span class="sd-seg-opt" :class="{ on: !backupExists }" @click="backupExists = false">見つからない</span>
      </span>
    </div>

    <div class="op-cols">
      <div class="op-col">
        <div class="op-col-h">Spec — 人が書く「あるべき姿」</div>
        <div class="op-kv mono"><span>members</span><b>{{ wantMembers }}</b></div>
        <div class="op-kv mono"><span>restoreFrom</span><b>{{ restoreFrom }}</b></div>
        <div class="op-note">手順は書かない。状態だけを書く</div>
      </div>
      <div class="op-col">
        <div class="op-col-h">Status — コントローラが書き戻す「今の姿」</div>
        <div class="op-kv mono"><span>phase</span><b :class="degraded ? 'bad' : converged ? 'ok' : ''">{{ phase }}</b></div>
        <div class="op-kv mono"><span>members</span><b>{{ readyMembers.length }}</b></div>
        <div class="op-kv mono"><span>leader</span><b>{{ leader || "(未選出)" }}</b></div>
        <div class="op-kv mono"><span>restored</span><b>{{ restored ? "true" : "false" }}</b></div>
      </div>
    </div>

    <div class="op-members">
      <span class="op-members-h">現実のメンバー</span>
      <span
        v-for="m in members"
        :key="m.name"
        class="op-member mono"
        :class="[m.ready ? 'ready' : 'wait', m.name === leader ? 'leader' : '']"
      >{{ m.name }}<small>{{ m.name === leader ? "leader" : m.ready ? "ready" : "起動中" }}</small></span>
      <span v-if="members.length === 0" class="op-empty">(なし)</span>
    </div>

    <div class="op-verdict" :class="degraded ? 'bad' : converged ? 'ok' : 'neutral'">
      <template v-if="degraded && !backupExists">
        復元元が見つからないので先へ進めない。差を埋められないことを状態で示して止まっている
      </template>
      <template v-else-if="degraded">崩れた。次の調整で同じループが埋め直す</template>
      <template v-else-if="converged">
        宣言どおりに揃った。この状態で何度調整しても何も起こらない(冪等)
      </template>
      <template v-else>この回の手: {{ actionLabel }}</template>
    </div>

    <div class="op-log">
      <div class="op-log-h">調整が打った手</div>
      <div v-for="(l, i) in log.slice(-8)" :key="i" class="op-log-line mono">{{ l }}</div>
      <div v-if="log.length === 0" class="op-empty">(まだ調整していない)</div>
    </div>

    <p class="op-legend">
      左の Spec には手順でなく状態しか書いていない。「1回調整する」を押すたびに、コントローラは Spec と現実を
      見比べて、まだ埋まっていない差を1つだけ埋める。復元が先で、次にメンバー、最後にリーダーという順序は、
      手順として書かれているのではなく「どの差がまだ残っているか」の判定として現れる。だからリーダーを落とすと、
      障害イベントを購読していないのに次の調整で選び直される。調整ループの章と同じ性質が、自分の型に対して
      そのまま手に入っている。
    </p>
  </DemoShell>
</template>

<style scoped>
.op-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.op-second {
  margin-top: 8px;
}
.op-spacer {
  flex: 1;
}
.op-label {
  font-size: 11.5px;
  color: var(--vp-c-text-2);
}
.op-cols {
  display: flex;
  gap: 10px;
  margin-top: 14px;
  flex-wrap: wrap;
}
.op-col {
  flex: 1;
  min-width: 230px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
}
.op-col-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 8px;
}
.op-kv {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  font-size: 11px;
  padding: 2px 0;
  color: var(--vp-c-text-3);
}
.op-kv b {
  color: var(--vp-c-text-1);
  font-weight: 700;
}
.op-kv b.ok {
  color: var(--vp-c-green-1);
}
.op-kv b.bad {
  color: var(--vp-c-danger-1);
}
.op-note {
  margin-top: 6px;
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.op-members {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  margin-top: 12px;
  padding: 8px 12px;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.op-members-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-right: 4px;
}
.op-member {
  display: flex;
  flex-direction: column;
  align-items: center;
  font-size: 10.5px;
  padding: 3px 8px;
  border: 1px solid var(--vp-c-divider);
}
.op-member small {
  font-size: 8.5px;
  opacity: 0.85;
}
.op-member.ready {
  border-color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.op-member.wait {
  border-style: dashed;
  color: var(--vp-c-text-3);
}
.op-member.leader {
  border-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-brand-1);
  font-weight: 700;
}
.op-empty {
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.op-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
  background-color: var(--vp-c-bg-soft);
}
.op-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.op-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.op-log {
  margin-top: 12px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
  min-height: 54px;
}
.op-log-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 4px;
}
.op-log-line {
  font-size: 10.5px;
  color: var(--vp-c-text-2);
  padding: 1px 0;
}
.op-legend {
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
