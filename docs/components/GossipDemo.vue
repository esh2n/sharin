<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// distributed/gossip(Go)を移植。落ちたときと、切れただけのときの差を見る。

type State = 0 | 1 | 2; // Alive / Suspect / Dead
const NAMES = ["a", "b", "c", "d", "e"];
const STATE_NAME = ["生きている", "疑わしい", "死んだ"];

interface Member {
  state: State;
  inc: number;
}
type View = Record<string, Member>;

const INDIRECT = 2;
const SUSPECT_FOR = 3;

const views = ref<Record<string, View>>({});
const suspectAt = ref<Record<string, Record<string, number>>>({});
const down = ref<Record<string, boolean>>({});
const blocked = ref(false); // a → b だけ切る
const tick = ref(0);
const pings = ref(0);
const rnd = ref(0n);
const log = ref<string[]>([]);

function merge(a: Member, b: Member): Member {
  if (a.inc > b.inc) return a;
  if (b.inc > a.inc) return b;
  return a.state >= b.state ? a : b;
}
function next(n: number): number {
  rnd.value = (rnd.value * 6364136223846793005n + 1442695040888963407n) & 0xffffffffffffffffn;
  return Number((rnd.value >> 33n) % BigInt(n));
}
function logf(m: string) {
  log.value = [...log.value, `t=${tick.value} ${m}`];
}
function reachable(from: string, to: string): boolean {
  if (down.value[from] || down.value[to]) return false;
  if (blocked.value && from === "a" && to === "b") return false;
  return true;
}

function reset() {
  const v: Record<string, View> = {};
  for (const n of NAMES) {
    v[n] = {};
    for (const m of NAMES) v[n][m] = { state: 0, inc: 0 };
  }
  views.value = v;
  suspectAt.value = Object.fromEntries(NAMES.map((n) => [n, {}]));
  down.value = {};
  tick.value = 0;
  pings.value = 0;
  rnd.value = 7n;
  log.value = [];
}
reset();

function apply(node: string, name: string, m: Member) {
  const cur = views.value[node][name];
  const nx = merge(cur, m);
  if (nx.state !== 1) delete suspectAt.value[node][name];
  views.value[node][name] = nx;
}

function round() {
  for (const n of NAMES) {
    if (down.value[n]) continue;
    // 1台だけ選ぶ
    const cand = NAMES.filter((m) => m !== n && views.value[n][m].state !== 2);
    if (!cand.length) continue;
    const target = cand[next(cand.length)];
    pings.value++;

    let ok = reachable(n, target);
    if (!ok) {
      // 他の台に頼む
      let asked = 0;
      for (const m of NAMES) {
        if (asked >= INDIRECT) break;
        if (m === n || m === target || down.value[m]) continue;
        asked++;
        pings.value++;
        if (reachable(m, target)) {
          logf(`${n} は ${target} に届かないが、${m} からは届いた`);
          ok = true;
          break;
        }
      }
    }
    if (ok) {
      for (const m of NAMES) {
        apply(n, m, views.value[target][m]);
        apply(target, m, views.value[n][m]);
      }
    } else if (views.value[n][target].state === 0) {
      views.value[n][target] = { ...views.value[n][target], state: 1 };
      suspectAt.value[n][target] = tick.value;
      logf(`${n} が ${target} を疑い始めた`);
    }
    // 猶予切れを死んだと決める
    for (const m of NAMES) {
      if (views.value[n][m].state !== 1) continue;
      const since = suspectAt.value[n][m];
      if (since === undefined) {
        suspectAt.value[n][m] = tick.value;
        continue;
      }
      if (tick.value - since >= SUSPECT_FOR) {
        views.value[n][m] = { ...views.value[n][m], state: 2 };
        delete suspectAt.value[n][m];
        logf(`${n} が ${m} を死んだと決めた`);
      }
    }
  }
  tick.value++;
}

function kill(n: string) {
  down.value = { ...down.value, [n]: true };
  logf(`${n} が落ちた`);
}
function revive(n: string) {
  const d = { ...down.value };
  delete d[n];
  down.value = d;
  const me = views.value[n][n];
  views.value[n][n] = { state: 0, inc: me.inc + 1 };
  logf(`${n} が起きて、番号 ${me.inc + 1} で生存を主張する`);
}
function run(k: number) {
  for (let i = 0; i < k; i++) round();
}

const anyDown = computed(() => NAMES.some((n) => down.value[n]));
const agreed = (name: string) => {
  const live = NAMES.filter((n) => !down.value[n]);
  if (!live.length) return null;
  const first = views.value[live[0]][name].state;
  return live.every((n) => views.value[n][name].state === first) ? first : null;
};
const badge = computed(() => `周期 ${tick.value} ・ 問い合わせ ${pings.value} 回`);
const verdict = computed(() => {
  if (blocked.value && !down.value["b"]) {
    const st = views.value["a"]["b"].state;
    return st === 0
      ? "a から b へは届かないが、他の台に頼んで確かめているので生きていると分かる。1台では区別できないことを、他に聞くことで区別している"
      : "a が b を疑い始めた。間接の問い合わせでも届かなかったということになる";
  }
  const dead = NAMES.filter((n) => agreed(n) === 2);
  if (dead.length) return `${dead.join("、")} は、生きている全員が死んだと見ている。疑いを経てから決まった`;
  const susp = NAMES.filter((n) => NAMES.some((m) => !down.value[m] && views.value[m][n].state === 1));
  if (susp.length) return `${susp.join("、")} が疑われている。まだ決まっていないので、本人が起きれば覆せる`;
  return "全員が生きていると見ている。周期を進めると、1台ずつ問い合わせて見立てを交換する";
});
</script>

<template>
  <DemoShell title="ゴシップと障害検知" :badge="badge" badge-tone="ok">
    <div class="gs-actions">
      <button class="sd-btn sd-btn--primary" @click="round">1 周期</button>
      <button class="sd-btn" @click="run(5)">5 周期</button>
      <span class="gs-gap" />
      <button class="sd-btn" :class="blocked ? 'sd-btn--primary' : ''" @click="blocked = !blocked">
        a→b だけ切る: {{ blocked ? "切る" : "繋がる" }}
      </button>
      <button class="sd-btn" @click="down['e'] ? revive('e') : kill('e')">
        {{ down["e"] ? "e を起こす" : "e を落とす" }}
      </button>
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <div class="gs-grid">
      <div class="gs-row gs-head mono">
        <span class="gs-who">見る側 ＼ 見られる側</span>
        <span v-for="m in NAMES" :key="m" class="gs-cell">{{ m }}</span>
      </div>
      <div v-for="n in NAMES" :key="n" class="gs-row" :class="down[n] ? 'off' : ''">
        <span class="gs-who mono">{{ n }}<em v-if="down[n]">落ちている</em></span>
        <span
          v-for="m in NAMES"
          :key="m"
          class="gs-cell mono"
          :class="['s' + views[n][m].state, n === m ? 'self' : '']"
          :title="`${n} から見た ${m}: ${STATE_NAME[views[n][m].state]}(番号 ${views[n][m].inc})`"
        >
          {{ ["生", "疑", "死"][views[n][m].state] }}<i v-if="views[n][m].inc">{{ views[n][m].inc }}</i>
        </span>
      </div>
    </div>

    <div class="gs-verdict" :class="anyDown || blocked ? 'bad' : 'ok'">{{ verdict }}</div>

    <div class="gs-log">
      <div v-for="(l, i) in log.slice(-5)" :key="i" class="gs-log-line mono">{{ l }}</div>
      <div v-if="!log.length" class="gs-empty mono">(まだ何も起きていない)</div>
    </div>

    <p class="gs-legend">
      表の行が見る側、列が見られる側。緑が生きている、黄が疑わしい、赤が死んだ。小さい数字は本人が
      上げた番号で、これが大きいほうが勝つ。「e を落とす」と、疑いを経てから死んだと決まっていく。
      落とさずに「a→b だけ切る」と、a は b に届かないのに生きていると判定し続ける。他の台に頼んで
      確かめているからで、これが無いと断線しただけのノードが次々に外される。
    </p>
  </DemoShell>
</template>

<style scoped>
.gs-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.gs-gap {
  flex: 1;
  min-width: 8px;
}
.gs-grid {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  padding: 9px 12px;
  background-color: var(--vp-c-bg-soft);
}
.gs-row {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-bottom: 3px;
}
.gs-head {
  font-size: 9.5px;
  color: var(--vp-c-text-3);
  border-bottom: 1px solid var(--vp-c-divider);
  padding-bottom: 4px;
}
.gs-who {
  width: 128px;
  flex: none;
  font-size: 11px;
  color: var(--vp-c-text-2);
  display: flex;
  flex-direction: column;
}
.gs-who em {
  font-style: normal;
  font-size: 9px;
  color: var(--vp-c-danger-1);
}
.gs-row.off .gs-who {
  color: var(--vp-c-text-3);
}
.gs-cell {
  flex: 1;
  text-align: center;
  font-size: 10.5px;
  padding: 3px 0;
  border: 1px solid transparent;
}
.gs-head .gs-cell {
  border: none;
}
.gs-cell i {
  font-style: normal;
  font-size: 8px;
  vertical-align: super;
  opacity: 0.75;
}
.gs-cell.s0 {
  border-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.gs-cell.s1 {
  border-color: var(--vp-c-warning-1);
  color: var(--vp-c-warning-1);
  background-color: var(--vp-c-warning-soft);
}
.gs-cell.s2 {
  border-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.gs-cell.self {
  opacity: 0.45;
}
.gs-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  font-weight: 600;
  line-height: 1.6;
  background-color: var(--vp-c-bg-soft);
}
.gs-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.gs-verdict.bad {
  border-left-color: var(--vp-c-warning-1);
  color: var(--vp-c-warning-1);
  background-color: var(--vp-c-warning-soft);
}
.gs-log {
  margin-top: 10px;
  border-top: 1px solid var(--vp-c-divider);
  padding-top: 6px;
  min-height: 52px;
}
.gs-log-line,
.gs-empty {
  font-size: 10px;
  color: var(--vp-c-text-2);
  padding: 1px 0;
}
.gs-empty {
  color: var(--vp-c-text-3);
}
.gs-legend {
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
