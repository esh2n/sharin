<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/statefulset(Go)を移植。序数の順に立ち上がること、
// 途中が詰まると止まること、ボリュームが Pod より長生きすることを見せる。

const STARTUP = 2;

interface Pod {
  ordinal: number;
  name: string;
  ready: boolean;
  broken: boolean;
  readyAt: number;
  pvc: string;
}
interface Vol {
  ordinal: number;
  name: string;
  data: string;
}

const replicas = ref(3);
const pods = ref<Pod[]>([]);
const vols = ref<Vol[]>([]);
const now = ref(0);
const log = ref<string[]>([]);
const brokenOrd = ref<number | null>(null);

const podAt = (o: number) => pods.value.find((p) => p.ordinal === o);
const volAt = (o: number) => vols.value.find((v) => v.ordinal === o);
const maxOrd = () => Math.max(...pods.value.map((p) => p.ordinal));
const allReadyBelow = (o: number) => {
  for (let i = 0; i < o; i++) {
    const p = podAt(i);
    if (!p || !p.ready) return false;
  }
  return true;
};
const nextOrd = () => {
  for (let o = 0; ; o++) if (!podAt(o)) return o;
};

function create(o: number) {
  let v = volAt(o);
  const existed = !!v;
  if (!v) {
    v = { ordinal: o, name: `data-db-${o}`, data: "" };
    vols.value = [...vols.value, v].sort((a, b) => a.ordinal - b.ordinal);
  }
  const broken = brokenOrd.value === o;
  pods.value = [
    ...pods.value,
    { ordinal: o, name: `db-${o}`, ready: false, broken, readyAt: now.value + STARTUP, pvc: v.name },
  ].sort((a, b) => a.ordinal - b.ordinal);
  log.value = [
    ...log.value,
    `step ${now.value}: db-${o} を作成(${existed ? `既存のボリューム ${v.name} を再接続` : `ボリューム ${v.name} を新規作成`})`,
  ];
}

function step() {
  now.value++;
  for (const p of pods.value) {
    if (!p.ready && !p.broken && now.value >= p.readyAt) {
      p.ready = true;
      log.value = [...log.value, `step ${now.value}: ${p.name} が ready になった`];
    }
  }
  pods.value = [...pods.value];

  if (pods.value.length > replicas.value) {
    const last = maxOrd();
    pods.value = pods.value.filter((p) => p.ordinal !== last);
    log.value = [...log.value, `step ${now.value}: db-${last} を削除(ボリュームは残る)`];
    return;
  }
  if (pods.value.length < replicas.value) {
    const next = nextOrd();
    if (!allReadyBelow(next)) {
      log.value = [...log.value, `step ${now.value}: 序数 ${next} はまだ作らない(手前が ready でない)`];
      return;
    }
    create(next);
  }
}
function step5() {
  for (let i = 0; i < 5; i++) step();
}
function scale(n: number) {
  replicas.value = Math.max(0, n);
  log.value = [...log.value, `step ${now.value}: 目標を ${replicas.value} に変更`];
}
function writeTo(o: number) {
  const v = volAt(o);
  if (!v) return;
  v.data = `rev-${now.value}`;
  vols.value = [...vols.value];
  log.value = [...log.value, `step ${now.value}: ${v.name} に "${v.data}" を書いた`];
}
function deletePod(o: number) {
  if (!podAt(o)) return;
  pods.value = pods.value.filter((p) => p.ordinal !== o);
  log.value = [...log.value, `step ${now.value}: db-${o} を削除(ボリュームは残る)`];
}
function deleteVol(o: number) {
  if (!volAt(o)) return;
  vols.value = vols.value.filter((v) => v.ordinal !== o);
  log.value = [...log.value, `step ${now.value}: data-db-${o} を削除(ここで初めてデータが消える)`];
}
function toggleBroken() {
  brokenOrd.value = brokenOrd.value === 1 ? null : 1;
  const p = podAt(1);
  if (p) {
    p.broken = brokenOrd.value === 1;
    if (p.broken) p.ready = false;
    pods.value = [...pods.value];
  }
  log.value = [
    ...log.value,
    `step ${now.value}: db-1 を${brokenOrd.value === 1 ? "壊れた状態にした" : "直した"}`,
  ];
}
function reset() {
  replicas.value = 3;
  pods.value = [];
  vols.value = [];
  now.value = 0;
  log.value = [];
  brokenOrd.value = null;
}
reset();

const ready = computed(() => pods.value.filter((p) => p.ready).length);
const converged = computed(() => ready.value === replicas.value && pods.value.length === replicas.value);
const stuck = computed(() => !converged.value && pods.value.some((p) => p.broken));
const badge = computed(() => `ready ${ready.value} / 目標 ${replicas.value} ・ ボリューム ${vols.value.length}`);
const badgeTone = computed<"ok" | "ng" | "neutral">(() =>
  stuck.value ? "ng" : converged.value ? "ok" : "neutral",
);
</script>

<template>
  <DemoShell title="StatefulSetとPVC" :badge="badge" :badge-tone="badgeTone">
    <div class="ss-actions">
      <button class="sd-btn sd-btn--primary" @click="step">1周期進める</button>
      <button class="sd-btn" @click="step5">5周期進める</button>
      <button class="sd-btn" @click="scale(replicas + 1)">目標 +1</button>
      <button class="sd-btn" @click="scale(replicas - 1)">目標 −1</button>
      <span class="ss-spacer" />
      <span class="ss-clock mono">step={{ now }}</span>
      <button class="sd-btn" @click="reset">リセット</button>
    </div>
    <div class="ss-actions ss-second">
      <button class="sd-btn" @click="toggleBroken">
        {{ brokenOrd === 1 ? "db-1 を直す" : "db-1 を壊す(readyにならない)" }}
      </button>
      <span class="ss-hint mono">起動に {{ STARTUP }} 周期</span>
    </div>

    <div class="ss-lanes">
      <div class="ss-lane-h">Pod(序数で識別され、順に立ち上がる)</div>
      <div class="ss-row">
        <div v-for="o in Math.max(replicas, vols.length, 1)" :key="o" class="ss-cell">
          <div
            v-if="podAt(o - 1)"
            class="ss-pod"
            :class="podAt(o - 1)!.broken ? 'broken' : podAt(o - 1)!.ready ? 'ready' : 'wait'"
          >
            <span class="mono ss-name">{{ podAt(o - 1)!.name }}</span>
            <span class="mono ss-sub">{{ podAt(o - 1)!.broken ? "起動しない" : podAt(o - 1)!.ready ? "ready" : "起動中" }}</span>
            <button class="ss-mini" @click="deletePod(o - 1)">Podを消す</button>
          </div>
          <div v-else class="ss-pod empty"><span class="mono ss-sub">(なし)</span></div>
        </div>
      </div>

      <div class="ss-lane-h">ボリューム(Pod より長生きする)</div>
      <div class="ss-row">
        <div v-for="o in Math.max(replicas, vols.length, 1)" :key="o" class="ss-cell">
          <div v-if="volAt(o - 1)" class="ss-vol">
            <span class="mono ss-name">{{ volAt(o - 1)!.name }}</span>
            <span class="mono ss-sub">{{ volAt(o - 1)!.data || "(空)" }}</span>
            <span class="ss-minis">
              <button class="ss-mini" @click="writeTo(o - 1)">書く</button>
              <button class="ss-mini" @click="deleteVol(o - 1)">消す</button>
            </span>
          </div>
          <div v-else class="ss-vol empty"><span class="mono ss-sub">(なし)</span></div>
        </div>
      </div>
    </div>

    <div class="ss-verdict" :class="stuck ? 'bad' : converged ? 'ok' : 'neutral'">
      <template v-if="stuck">
        db-1 が ready にならないので、それ以降の序数は永久に作られない。順序を守るとは、詰まったら止まること
      </template>
      <template v-else-if="converged">目標どおり {{ ready }} 個が序数順に揃っている</template>
      <template v-else>まだ揃っていない。手前が ready になるまで次は作られない</template>
    </div>

    <div class="ss-log">
      <div class="ss-log-h">起きたこと</div>
      <div v-for="(l, i) in log.slice(-8)" :key="i" class="ss-log-line mono">{{ l }}</div>
      <div v-if="log.length === 0" class="ss-empty">(まだ何も起きていない)</div>
    </div>

    <p class="ss-legend">
      周期を進めると db-0 から順に立ち上がる。手前が ready になるまで次は作られないので、db-1 を壊すと
      db-2 は永久に現れない。ボリュームに何か書いてから「Podを消す」と、Pod だけが消えてボリュームは残り、
      作り直された同じ序数の Pod がまた同じボリュームに繋がる。目標を減らしてもボリュームは残るので、
      戻せばデータも戻る。データが失われるのは、ボリュームを明示的に消したときだけになる。
    </p>
  </DemoShell>
</template>

<style scoped>
.ss-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.ss-second {
  margin-top: 8px;
}
.ss-spacer {
  flex: 1;
}
.ss-clock,
.ss-hint {
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.ss-lanes {
  margin-top: 14px;
}
.ss-lane-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin: 10px 0 5px;
}
.ss-row {
  display: flex;
  gap: 8px;
}
.ss-cell {
  flex: 1;
  min-width: 96px;
}
.ss-pod,
.ss-vol {
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 8px 10px;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
  min-height: 72px;
}
.ss-pod.ready {
  border-color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.ss-pod.wait {
  border-style: dashed;
}
.ss-pod.broken {
  border-color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.ss-pod.empty,
.ss-vol.empty {
  border-style: dotted;
  opacity: 0.6;
}
.ss-vol {
  border-color: var(--vp-c-brand-1);
}
.ss-name {
  font-size: 11px;
  font-weight: 700;
}
.ss-sub {
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.ss-minis {
  display: flex;
  gap: 4px;
}
.ss-mini {
  font-size: 9.5px;
  padding: 1px 6px;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg);
  color: var(--vp-c-text-2);
  cursor: pointer;
  margin-top: auto;
}
.ss-mini:hover {
  background-color: var(--vp-c-default-soft);
}
.ss-empty {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.ss-verdict {
  margin-top: 14px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
  background-color: var(--vp-c-bg-soft);
}
.ss-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.ss-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.ss-log {
  margin-top: 12px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
  min-height: 54px;
}
.ss-log-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 4px;
}
.ss-log-line {
  font-size: 10.5px;
  color: var(--vp-c-text-2);
  padding: 1px 0;
}
.ss-legend {
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
