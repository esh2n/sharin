<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// foundations/oom(Go)を移植。物理 1000 に対して 4 つが 400 ずつ申し込む。
const TOTAL = 1000;
const NAMES = ["a", "b", "c", "d"];
const WANT = 400;

interface Proc {
  name: string;
  reserved: number;
  touched: number;
  adj: number;
  dead: boolean;
}
interface Kill {
  requester: string;
  victim: string;
  freed: number;
}

const overcommit = ref(true);
const byScore = ref(true);
const step = ref(8); // 予約4回 + 触る4回

function score(p: Proc): number {
  if (p.adj <= -1000) return -1000;
  return Math.floor((p.touched * 1000) / TOTAL) + p.adj;
}

interface State {
  procs: Proc[];
  kills: Kill[];
  refused: string;
  log: string[];
}

function simulate(steps: number, oc: boolean, pickBiggest: boolean): State {
  const procs: Proc[] = NAMES.map((name) => ({ name, reserved: 0, touched: 0, adj: 0, dead: false }));
  const kills: Kill[] = [];
  const log: string[] = [];
  let refused = "";
  const reserved = () => procs.filter((p) => !p.dead).reduce((a, p) => a + p.reserved, 0);
  const touched = () => procs.filter((p) => !p.dead).reduce((a, p) => a + p.touched, 0);

  for (let i = 0; i < steps; i++) {
    if (i < NAMES.length) {
      const p = procs[i];
      if (!oc && reserved() + WANT > TOTAL) {
        refused = `${p.name} の申し込みを断った(${reserved()} + ${WANT} > ${TOTAL})`;
        log.push(`${p.name}: 予約 ${WANT} → 断られた`);
        continue;
      }
      p.reserved += WANT;
      log.push(`${p.name}: 予約 ${WANT} → 通った`);
      continue;
    }
    const p = procs[i - NAMES.length];
    if (p.dead || p.reserved === 0) continue;
    let ok = true;
    while (touched() + WANT > TOTAL) {
      const alive = procs.filter((q) => !q.dead && q.touched > 0 && q.adj > -1000);
      const victim = pickBiggest
        ? alive.reduce((a, b) => (score(b) > score(a) ? b : a), alive[0])
        : p.touched > 0
          ? p
          : undefined;
      if (!victim) {
        log.push(`${p.name}: 触る ${WANT} → 殺せる相手が居ない`);
        ok = false;
        break;
      }
      kills.push({ requester: p.name, victim: victim.name, freed: victim.touched });
      log.push(`${p.name} が触ろうとして ${victim.name} が殺された(${victim.touched} 空いた)`);
      victim.dead = true;
      victim.touched = 0;
      victim.reserved = 0;
      if (p.dead) {
        ok = false;
        break;
      }
    }
    if (ok) {
      p.touched += WANT;
      log.push(`${p.name}: 触る ${WANT} → 通った`);
    }
  }
  return { procs, kills, refused, log };
}

const state = computed(() => simulate(step.value, overcommit.value, byScore.value));
const alive = computed(() => state.value.procs.filter((p) => !p.dead));
const totalReserved = computed(() => alive.value.reduce((a, p) => a + p.reserved, 0));
const totalTouched = computed(() => alive.value.reduce((a, p) => a + p.touched, 0));

const badge = computed(() =>
  state.value.kills.length ? `${state.value.kills.length} 個殺した` : "誰も死んでいない",
);
const badgeTone = computed(() => (state.value.kills.length ? "ng" : "ok"));

const verdict = computed(() => {
  const k = state.value.kills;
  if (state.value.refused) return `${state.value.refused}。断られるのは申し込んだ本人だけで、誰も死なない`;
  if (!k.length) return "まだ足りている";
  const other = k.find((x) => x.victim !== x.requester);
  if (other) return `${other.requester} が触ろうとして、${other.victim} が殺された。足りなくした本人と、殺された相手が違う`;
  return `${k[0].requester} が自分で触って、自分が殺された`;
});
</script>

<template>
  <DemoShell title="OOM killer" :badge="badge" :badge-tone="badgeTone">
    <div class="om-actions">
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: !overcommit }" @click="overcommit = false">申し込みを断る</span>
        <span class="sd-seg-opt" :class="{ on: overcommit }" @click="overcommit = true">断らず、あとで殺す</span>
      </span>
      <span class="spacer"></span>
      <span class="sd-seg" :class="{ off: !overcommit }">
        <span class="sd-seg-opt" :class="{ on: byScore }" @click="byScore = true">効く相手を殺す</span>
        <span class="sd-seg-opt" :class="{ on: !byScore }" @click="byScore = false">本人を殺す</span>
      </span>
    </div>

    <p class="om-setting mono">
      物理 {{ TOTAL }} ・ 4 つのプロセスがそれぞれ {{ WANT }} を申し込み、順に触る
    </p>

    <div class="om-meters">
      <div class="om-meter">
        <span class="om-meter-label">申し込んだ量</span>
        <span class="om-meter-bar">
          <span class="om-meter-fill res" :style="{ width: Math.min(100, (totalReserved / TOTAL) * 100) + '%' }"></span>
          <span v-if="totalReserved > TOTAL" class="om-meter-over">物理を超えている</span>
        </span>
        <span class="om-meter-num mono">{{ totalReserved }}</span>
      </div>
      <div class="om-meter">
        <span class="om-meter-label">実際に触った量</span>
        <span class="om-meter-bar">
          <span class="om-meter-fill" :style="{ width: (totalTouched / TOTAL) * 100 + '%' }"></span>
        </span>
        <span class="om-meter-num mono">{{ totalTouched }}</span>
      </div>
    </div>

    <table class="om-table">
      <thead>
        <tr><th>プロセス</th><th>申し込み</th><th>抱えている</th><th>oom_score</th><th>状態</th></tr>
      </thead>
      <tbody>
        <tr v-for="p in state.procs" :key="p.name" :class="{ dead: p.dead }">
          <td class="mono">{{ p.name }}</td>
          <td class="mono num">{{ p.dead ? "—" : p.reserved }}</td>
          <td class="mono num">{{ p.dead ? "—" : p.touched }}</td>
          <td class="mono num">{{ p.dead ? "—" : score(p) }}</td>
          <td>{{ p.dead ? "殺された" : "生きている" }}</td>
        </tr>
      </tbody>
    </table>

    <div class="om-verdict">{{ verdict }}</div>

    <p class="om-note">
      申し込みを断る側は、断られた本人がそれを知って諦められる。断らない側は、申し込みが通ったあとで
      実際にページを触った瞬間に足りなくなり、そこで誰かが死ぬ。<b>足りなくした本人が死ぬとは限らない</b>のは、
      殺す相手を「誰のせいか」ではなく「殺してどれだけ空くか」で選んでいるためになる。
    </p>
  </DemoShell>
</template>

<style scoped>
.om-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.om-actions .sd-seg.off {
  opacity: 0.4;
}
.om-setting {
  margin: 12px 0 0;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.om-meters {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 10px;
}
.om-meter {
  display: flex;
  align-items: center;
  gap: 10px;
}
.om-meter-label {
  width: 100px;
  flex: none;
  font-size: 11.5px;
  color: var(--vp-c-text-2);
}
.om-meter-bar {
  position: relative;
  flex: 1 1 auto;
  height: 12px;
  background-color: var(--vp-c-default-soft);
}
.om-meter-fill {
  display: block;
  height: 100%;
  background-color: var(--vp-c-brand-1);
}
.om-meter-fill.res {
  background-color: var(--vp-c-text-3);
}
.om-meter-over {
  position: absolute;
  right: 4px;
  top: -1px;
  font-size: 9.5px;
  color: var(--vp-c-danger-1);
}
.om-meter-num {
  width: 44px;
  flex: none;
  text-align: right;
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.om-table {
  width: 100%;
  border-collapse: collapse;
  margin-top: 14px;
  font-size: 11.5px;
}
.om-table th {
  text-align: left;
  font-size: 10px;
  font-weight: 700;
  color: var(--vp-c-text-3);
  padding: 0 10px 5px 0;
  border-bottom: 1px solid var(--vp-c-divider);
}
.om-table td {
  padding: 5px 10px 5px 0;
  border-bottom: 1px solid var(--vp-c-divider);
  color: var(--vp-c-text-2);
}
.om-table td.num {
  text-align: right;
  padding-right: 18px;
}
.om-table tr.dead td {
  color: var(--vp-c-text-3);
  text-decoration: line-through;
}
.om-verdict {
  margin-top: 14px;
  padding: 8px 12px;
  background-color: var(--vp-c-bg-soft);
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--vp-c-text-1);
}
.om-note {
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
