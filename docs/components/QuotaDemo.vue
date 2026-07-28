<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/quota(Go)を移植。既定値の有無で「書き忘れ」がどう数えられるかと、
// 総量・1つあたり・個数の3つの上限がそれぞれ別に効くことを見せる。

const HARD = { cpu: 2000, mem: 2048 };
const MAX_PODS = 6;
const PER_POD_MAX = { cpu: 1000, mem: 1024 };

interface Pod {
  name: string;
  cpu: number;
  mem: number;
  defaulted: boolean;
}

const useDefaults = ref(true);
const DEFAULT = { cpu: 200, mem: 256 };

const pods = ref<Pod[]>([]);
const seq = ref(0);
const log = ref<string[]>([]);
const lastReject = ref("");

const used = computed(() =>
  pods.value.reduce((a, p) => ({ cpu: a.cpu + p.cpu, mem: a.mem + p.mem }), { cpu: 0, mem: 0 }),
);

function admit(cpu: number, mem: number, label: string) {
  seq.value++;
  const name = `${label}-${seq.value}`;
  let defaulted = false;
  if (cpu === 0 && mem === 0 && useDefaults.value) {
    cpu = DEFAULT.cpu;
    mem = DEFAULT.mem;
    defaulted = true;
    log.value = [...log.value, `${name} に既定値を入れた(${cpu}m/${mem}Mi)`];
  }
  if (cpu > PER_POD_MAX.cpu || mem > PER_POD_MAX.mem) {
    lastReject.value = `${name} は1つあたりの上限 ${PER_POD_MAX.cpu}m/${PER_POD_MAX.mem}Mi を超える`;
    log.value = [...log.value, `${name} を拒否(1つあたりの上限)`];
    return;
  }
  if (pods.value.length >= MAX_PODS) {
    lastReject.value = `${name} は個数の上限 ${MAX_PODS} に達している`;
    log.value = [...log.value, `${name} を拒否(個数の上限)`];
    return;
  }
  if (used.value.cpu + cpu > HARD.cpu || used.value.mem + mem > HARD.mem) {
    lastReject.value = `${name} を入れると総量が上限 ${HARD.cpu}m/${HARD.mem}Mi を超える`;
    log.value = [...log.value, `${name} を拒否(総量の上限)`];
    return;
  }
  pods.value = [...pods.value, { name, cpu, mem, defaulted }];
  lastReject.value = "";
}

function removeLast() {
  if (pods.value.length === 0) return;
  const p = pods.value[pods.value.length - 1];
  pods.value = pods.value.slice(0, -1);
  log.value = [...log.value, `${p.name} を削除`];
  lastReject.value = "";
}
function reset() {
  pods.value = [];
  seq.value = 0;
  log.value = [];
  lastReject.value = "";
}

const pct = (a: number, b: number) => Math.min(100, Math.floor((a * 100) / b));
const noReqCount = computed(() => pods.value.filter((p) => p.defaulted).length);
const badge = computed(
  () => `${used.value.cpu}m / ${HARD.cpu}m ・ ${pods.value.length} / ${MAX_PODS} 個`,
);
const badgeTone = computed<"ok" | "ng" | "neutral">(() =>
  lastReject.value ? "ng" : pods.value.length > 0 ? "ok" : "neutral",
);
</script>

<template>
  <DemoShell title="ResourceQuotaとLimitRange" :badge="badge" :badge-tone="badgeTone">
    <div class="qt-row">
      <span class="qt-label">要求を書いていない Pod</span>
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: useDefaults }" @click="useDefaults = true">既定値を入れる</span>
        <span class="sd-seg-opt" :class="{ on: !useDefaults }" @click="useDefaults = false">そのまま(0として数える)</span>
      </span>
    </div>

    <div class="qt-actions">
      <button class="sd-btn sd-btn--primary" @click="admit(400, 400, 'web')">Pod を作る(400m/400Mi)</button>
      <button class="sd-btn" @click="admit(0, 0, 'noreq')">要求を書かずに作る</button>
      <button class="sd-btn" @click="admit(1500, 1500, 'huge')">大きすぎる Pod(1500m)</button>
      <button class="sd-btn" @click="removeLast">最後の Pod を消す</button>
      <span class="qt-spacer" />
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <p class="qt-fixed mono">
      総量 {{ HARD.cpu }}m / {{ HARD.mem }}Mi ・ 個数 {{ MAX_PODS }} ・ 1つあたり {{ PER_POD_MAX.cpu }}m / {{ PER_POD_MAX.mem }}Mi
      ・ 既定値 {{ DEFAULT.cpu }}m / {{ DEFAULT.mem }}Mi
    </p>

    <div class="qt-gauges">
      <div class="qt-gauge">
        <span class="qt-g-l mono">CPU 合計</span>
        <span class="qt-bar"><span class="qt-fill" :style="{ width: pct(used.cpu, HARD.cpu) + '%' }" /></span>
        <span class="qt-g-v mono">{{ used.cpu }} / {{ HARD.cpu }}m</span>
      </div>
      <div class="qt-gauge">
        <span class="qt-g-l mono">メモリ合計</span>
        <span class="qt-bar"><span class="qt-fill" :style="{ width: pct(used.mem, HARD.mem) + '%' }" /></span>
        <span class="qt-g-v mono">{{ used.mem }} / {{ HARD.mem }}Mi</span>
      </div>
      <div class="qt-gauge">
        <span class="qt-g-l mono">個数</span>
        <span class="qt-slots">
          <span v-for="i in MAX_PODS" :key="i" class="qt-slot" :class="i <= pods.length ? 'on' : ''" />
        </span>
        <span class="qt-g-v mono">{{ pods.length }} / {{ MAX_PODS }}</span>
      </div>
    </div>

    <div class="qt-pods">
      <span v-for="p in pods" :key="p.name" class="qt-pod mono" :class="p.defaulted ? 'defaulted' : ''">
        {{ p.name }}<small>{{ p.cpu }}m{{ p.defaulted ? "(既定)" : "" }}</small>
      </span>
      <span v-if="pods.length === 0" class="qt-empty">(Pod なし)</span>
    </div>

    <div class="qt-verdict" :class="lastReject ? 'bad' : !useDefaults && noReqCount > 0 ? 'warn' : 'ok'">
      <template v-if="lastReject">{{ lastReject }}</template>
      <template v-else-if="!useDefaults && noReqCount > 0">
        要求を書いていない Pod が 0 として数えられている。いくつ作っても合計は増えず、上限が意味を持たない
      </template>
      <template v-else-if="pods.length === 0">Pod を作ると、総量・個数・1つあたりの3つの上限に照らされる</template>
      <template v-else>
        合計 {{ used.cpu }}m / {{ HARD.cpu }}m、{{ pods.length }} / {{ MAX_PODS }} 個。まだ余裕がある
      </template>
    </div>

    <div class="qt-log">
      <div class="qt-log-h">起きたこと</div>
      <div v-for="(l, i) in log.slice(-6)" :key="i" class="qt-log-line mono">{{ l }}</div>
      <div v-if="log.length === 0" class="qt-empty">(まだ何も起きていない)</div>
    </div>

    <p class="qt-legend">
      1つ1つは正しくても、合計で止まる。「要求を書かずに作る」を既定値ありで押すと 200m として数えられ、
      無しで押すと 0 のまま数えられる。後者を繰り返すと、いくつ作っても合計が増えないまま資源だけが消費される。
      既定値を入れてから数えるという順序が、書き忘れを勘定に載せている。大きすぎる Pod は、総量に余裕があっても
      1つあたりの上限で止まる。3つの上限がそれぞれ別のものを守っている。
    </p>
  </DemoShell>
</template>

<style scoped>
.qt-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}
.qt-label {
  font-size: 11.5px;
  color: var(--vp-c-text-2);
  min-width: 160px;
}
.qt-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.qt-spacer {
  flex: 1;
}
.qt-fixed {
  margin: 10px 0 0;
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.qt-gauges {
  margin-top: 12px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
}
.qt-gauge {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 6px;
}
.qt-g-l {
  font-size: 10.5px;
  color: var(--vp-c-text-2);
  min-width: 62px;
}
.qt-bar {
  flex: 1;
  height: 10px;
  background-color: var(--vp-c-default-soft);
  overflow: hidden;
}
.qt-fill {
  display: block;
  height: 100%;
  background-color: var(--vp-c-brand-1);
  transition: width 0.2s;
}
.qt-slots {
  display: flex;
  gap: 3px;
  flex: 1;
}
.qt-slot {
  flex: 1;
  max-width: 26px;
  height: 10px;
  border: 1px solid var(--vp-c-divider);
}
.qt-slot.on {
  background-color: var(--vp-c-brand-1);
  border-color: var(--vp-c-brand-1);
}
.qt-g-v {
  font-size: 10px;
  color: var(--vp-c-text-3);
  min-width: 92px;
  text-align: right;
}
.qt-pods {
  display: flex;
  gap: 5px;
  flex-wrap: wrap;
  margin-top: 10px;
}
.qt-pod {
  display: flex;
  flex-direction: column;
  align-items: center;
  font-size: 10px;
  padding: 3px 7px;
  border: 1px solid var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.qt-pod.defaulted {
  border-color: var(--vp-c-warning-1);
  background-color: var(--vp-c-warning-soft);
  color: var(--vp-c-warning-1);
}
.qt-pod small {
  font-size: 8.5px;
  opacity: 0.85;
}
.qt-empty {
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.qt-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
  background-color: var(--vp-c-bg-soft);
}
.qt-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.qt-verdict.warn {
  border-left-color: var(--vp-c-warning-1);
  color: var(--vp-c-warning-1);
  background-color: var(--vp-c-warning-soft);
}
.qt-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.qt-log {
  margin-top: 12px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
  min-height: 50px;
}
.qt-log-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 4px;
}
.qt-log-line {
  font-size: 10.5px;
  color: var(--vp-c-text-2);
  padding: 1px 0;
}
.qt-legend {
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
