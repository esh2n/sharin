<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/pdb(Go)を移植。下限の宣言が自発的な退避だけを止め、
// 落ちる分は止められないことを見せる。

const STARTUP = 3;
const REPLICAS = 3;

interface Pod {
  name: string;
  node: string;
  ready: boolean;
  readyAt: number;
}

const minAvailable = ref(2);
const pods = ref<Pod[]>([]);
const now = ref(0);
const seq = ref(0);
const evicted = ref(0);
const denied = ref(0);
const crashed = ref(0);
const log = ref<string[]>([]);
const last = ref<"" | "ok" | "denied" | "crash">("");

const available = computed(() => pods.value.filter((p) => p.ready).length);

function replace(old: Pod) {
  seq.value++;
  pods.value = [
    ...pods.value,
    { name: `${old.name}-r${seq.value}`, node: "node-c", ready: false, readyAt: now.value + STARTUP },
  ];
}

// 自発的な退避。消した後も下限を保てるときだけ通る。
function evict(p: Pod) {
  const after = available.value - (p.ready ? 1 : 0);
  if (after < minAvailable.value) {
    denied.value++;
    last.value = "denied";
    log.value = [
      ...log.value,
      `t=${now.value} ${p.name} の退避を断った(下限 ${minAvailable.value} を割る。今 ${available.value})`,
    ];
    return false;
  }
  pods.value = pods.value.filter((x) => x !== p);
  evicted.value++;
  last.value = "ok";
  log.value = [...log.value, `t=${now.value} ${p.name} を退避した`];
  replace(p);
  return true;
}

// 止められない理由での消失。宣言は一切参照しない。
function crash(p: Pod) {
  pods.value = pods.value.filter((x) => x !== p);
  crashed.value++;
  last.value = "crash";
  log.value = [...log.value, `t=${now.value} ${p.name} が落ちた(宣言では止められない)`];
  replace(p);
}

function evictFirst() {
  const p = pods.value.find((x) => x.node !== "node-c");
  if (p) evict(p);
  else if (pods.value[0]) evict(pods.value[0]);
}
function crashFirst() {
  const p = pods.value.find((x) => x.ready);
  if (p) crash(p);
}
function drain() {
  const targets = pods.value.filter((p) => p.node === "node-a");
  if (targets.length === 0) {
    log.value = [...log.value, `t=${now.value} node-a に Pod はもう無い`];
    return;
  }
  for (const p of targets) evict(p);
}
function tick() {
  now.value++;
  for (const p of pods.value) {
    if (!p.ready && now.value >= p.readyAt) {
      p.ready = true;
      log.value = [...log.value, `t=${now.value} ${p.name} が ready になった`];
    }
  }
  pods.value = [...pods.value];
}
function reset() {
  now.value = 0;
  seq.value = 0;
  evicted.value = 0;
  denied.value = 0;
  crashed.value = 0;
  log.value = [];
  last.value = "";
  pods.value = Array.from({ length: REPLICAS }, (_, i) => ({
    name: `web-${i + 1}`,
    node: i % 2 === 0 ? "node-a" : "node-b",
    ready: true,
    readyAt: -1,
  }));
}
reset();

const blocked = computed(() => minAvailable.value >= REPLICAS);
const belowFloor = computed(() => available.value < minAvailable.value);
const badge = computed(() => `稼働 ${available.value} / 下限 ${minAvailable.value}`);
const badgeTone = computed<"ok" | "ng" | "neutral">(() =>
  belowFloor.value ? "ng" : available.value >= minAvailable.value ? "ok" : "neutral",
);
</script>

<template>
  <DemoShell title="PodDisruptionBudget" :badge="badge" :badge-tone="badgeTone">
    <div class="pd-row">
      <span class="pd-label">下限(常に何個以上)</span>
      <span class="sd-seg">
        <span v-for="n in [1, 2, 3]" :key="n" class="sd-seg-opt" :class="{ on: minAvailable === n }" @click="minAvailable = n">{{ n }}</span>
      </span>
      <span class="pd-hint mono">レプリカは {{ REPLICAS }}</span>
    </div>

    <div class="pd-actions">
      <button class="sd-btn sd-btn--primary" @click="evictFirst">退避を試みる(自発的)</button>
      <button class="sd-btn" @click="drain">node-a を空にする(drain)</button>
      <button class="sd-btn" @click="crashFirst">Podが落ちる(止められない)</button>
      <button class="sd-btn" @click="tick">1周期進める</button>
      <span class="pd-spacer" />
      <span class="pd-clock mono">t={{ now }}</span>
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <p class="pd-fixed mono">
      許可 {{ evicted }} / 拒否 {{ denied }} / 落ちた {{ crashed }} ・ 作り直しは node-c に、{{ STARTUP }} 周期で ready
    </p>

    <div class="pd-pods">
      <div v-for="p in pods" :key="p.name" class="pd-pod" :class="p.ready ? 'ready' : 'wait'">
        <span class="mono pd-pod-n">{{ p.name }}</span>
        <span class="mono pd-pod-m">{{ p.node }} / {{ p.ready ? "ready" : `起動中(あと ${Math.max(0, p.readyAt - now)})` }}</span>
      </div>
      <div v-if="pods.length === 0" class="pd-empty">(Pod なし)</div>
    </div>

    <div class="pd-gauge">
      <div v-for="i in Math.max(REPLICAS, pods.length)" :key="i" class="pd-slot" :class="i <= available ? 'on' : 'off'" />
      <span class="pd-floor mono">下限 {{ minAvailable }}</span>
    </div>

    <div class="pd-verdict" :class="belowFloor ? 'bad' : last === 'denied' ? 'warn' : 'ok'">
      <template v-if="belowFloor">
        下限 {{ minAvailable }} を割って稼働 {{ available }}。落ちた分は宣言では止められない
      </template>
      <template v-else-if="blocked">
        下限がレプリカ数と同じ。1つも退避できないので、更新も集約もノードの入れ替えも止まる
      </template>
      <template v-else-if="last === 'denied'">
        退避を断った。前に退避したぶんが立ち上がるまで、次は通らない
      </template>
      <template v-else-if="last === 'ok'">退避を許可した。下限は保たれている</template>
      <template v-else>稼働 {{ available }}、下限 {{ minAvailable }}。退避には余裕がある</template>
    </div>

    <div class="pd-log">
      <div class="pd-log-h">起きたこと</div>
      <div v-for="(l, i) in log.slice(-7)" :key="i" class="pd-log-line mono">{{ l }}</div>
      <div v-if="log.length === 0" class="pd-empty">(まだ何も起きていない)</div>
    </div>

    <p class="pd-legend">
      「退避を試みる」は更新や集約が Pod を止めようとする動きで、下限を割るなら断られる。作り直し中の Pod は
      頭数に入らないので、立ち上がるまで次の退避は通らない。この待ちが、実質的に「一度に何個まで止めてよいか」を
      決めている。一方「Podが落ちる」は宣言を一切参照しない。止められないものを止める約束はできないからで、
      宣言が守るのはこちらから止められる分だけになる。下限を 3 にすると、何も動かせなくなる。
    </p>
  </DemoShell>
</template>

<style scoped>
.pd-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}
.pd-label {
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.pd-hint,
.pd-clock {
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.pd-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.pd-spacer {
  flex: 1;
}
.pd-fixed {
  margin: 10px 0 0;
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.pd-pods {
  display: flex;
  gap: 8px;
  margin-top: 12px;
  flex-wrap: wrap;
}
.pd-pod {
  flex: 1;
  min-width: 130px;
  padding: 8px 10px;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.pd-pod.ready {
  border-color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.pd-pod.wait {
  border-style: dashed;
}
.pd-pod-n {
  display: block;
  font-size: 11.5px;
  font-weight: 700;
}
.pd-pod-m {
  display: block;
  font-size: 9.5px;
  color: var(--vp-c-text-3);
  margin-top: 2px;
}
.pd-gauge {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: 12px;
}
.pd-slot {
  width: 26px;
  height: 12px;
  border: 1px solid var(--vp-c-divider);
}
.pd-slot.on {
  background-color: var(--vp-c-green-1);
  border-color: var(--vp-c-green-1);
}
.pd-floor {
  font-size: 10.5px;
  color: var(--vp-c-text-3);
  margin-left: 6px;
}
.pd-empty {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.pd-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
  background-color: var(--vp-c-bg-soft);
}
.pd-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.pd-verdict.warn {
  border-left-color: var(--vp-c-warning-1);
  color: var(--vp-c-warning-1);
  background-color: var(--vp-c-warning-soft);
}
.pd-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.pd-log {
  margin-top: 12px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
  min-height: 54px;
}
.pd-log-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 4px;
}
.pd-log-line {
  font-size: 10.5px;
  color: var(--vp-c-text-2);
  padding: 1px 0;
}
.pd-legend {
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
