<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/kubelet(Go)を移植。置き場が無いところから、ファイルの宣言だけで
// 立ち上がる様子と、一覧を取り直して差を埋める様子を見る。

type State = "creating" | "running" | "exited";
interface Spec {
  name: string;
  startup: number;
}
interface PodSpec {
  name: string;
  containers: Spec[];
  restart: boolean;
}
interface Inst {
  id: string;
  pod: string;
  name: string;
  state: State;
  left: number;
  startup: number;
}

const FILE_PODS: PodSpec[] = [
  { name: "kube-apiserver", containers: [{ name: "apiserver", startup: 2 }], restart: true },
];
const API_PODS: PodSpec[] = [
  { name: "web", containers: [{ name: "app", startup: 1 }], restart: true },
  { name: "worker", containers: [{ name: "app", startup: 1 }], restart: true },
];

const items = ref<Inst[]>([]);
const seq = ref(0);
const linked = ref(false);
const apiPods = ref<PodSpec[]>([]);
const now = ref(0);
const relists = ref(0);
const restarts = ref<Record<string, number>>({});
const waitUntil = ref<Record<string, number>>({});
const log = ref<string[]>([]);

function backoffFor(n: number): number {
  let d = 1;
  for (let i = 1; i < n; i++) {
    d *= 2;
    if (d >= 8) return 8;
  }
  return d;
}
function logf(m: string) {
  log.value = [...log.value, `t=${now.value} ${m}`];
}

function desired(): { pod: PodSpec; src: string }[] {
  return [
    ...FILE_PODS.map((p) => ({ pod: p, src: "file" })),
    ...apiPods.value.map((p) => ({ pod: p, src: "apiserver" })),
  ].sort((a, b) => (a.pod.name < b.pod.name ? -1 : 1));
}

// 世界が1つ進む。kubelet は呼ばない。
function step() {
  items.value = items.value.map((it) => {
    if (it.state === "creating") {
      const left = it.left - 1;
      return left <= 0 ? { ...it, state: "running" as State, left: 0 } : { ...it, left };
    }
    return it;
  });
}

// 一覧を取り直して差を埋める。
function sync() {
  relists.value++;
  const live: Record<string, Inst> = {};
  for (const it of items.value) live[`${it.pod}/${it.name}`] = it;

  const wanted = new Set<string>();
  const next = [...items.value];
  for (const { pod, src } of desired()) {
    for (const c of pod.containers) {
      const key = `${pod.name}/${c.name}`;
      wanted.add(key);
      const cur = live[key];
      if (!cur) {
        const until = waitUntil.value[key];
        if (until !== undefined && now.value < until) continue;
        seq.value++;
        next.push({
          id: `c${seq.value}`,
          pod: pod.name,
          name: c.name,
          state: c.startup > 0 ? "creating" : "running",
          left: c.startup,
          startup: c.startup,
        });
        logf(restarts.value[key] ? `${key} を作り直した` : `${key} を作った(${src} の宣言)`);
      } else if (cur.state === "exited") {
        const i = next.findIndex((x) => x.id === cur.id);
        if (i >= 0) next.splice(i, 1);
        const n = (restarts.value[key] || 0) + 1;
        restarts.value = { ...restarts.value, [key]: n };
        waitUntil.value = { ...waitUntil.value, [key]: now.value + backoffFor(n) };
        logf(`${key} が落ちた。${backoffFor(n)} 待って作り直す(${n} 回目)`);
      }
    }
  }
  for (const it of items.value) {
    if (!wanted.has(`${it.pod}/${it.name}`)) {
      const i = next.findIndex((x) => x.id === it.id);
      if (i >= 0) next.splice(i, 1);
      logf(`${it.pod}/${it.name} は宣言に無い。消す`);
    }
  }
  items.value = next;
}

function tick() {
  step();
  sync();
  now.value++;
  // 置き場が立ったら、置き場からの宣言が届くようになる。
  if (!linked.value && items.value.some((i) => i.pod === "kube-apiserver" && i.state === "running")) {
    linked.value = true;
    apiPods.value = API_PODS;
    logf("置き場が立った。ここから宣言が届く");
  }
}

function toggleLink() {
  linked.value = !linked.value;
  logf(linked.value ? "置き場に届くようになった" : "置き場に届かなくなった。最後の宣言のまま動き続ける");
  if (linked.value) apiPods.value = API_PODS;
}
function killOne() {
  const target = items.value.find((i) => i.pod === "web" && i.state === "running");
  if (!target) return;
  items.value = items.value.map((i) => (i.id === target.id ? { ...i, state: "exited" as State } : i));
  logf("web/app のプロセスが外から落とされた(kubelet はまだ知らない)");
}
function stray() {
  seq.value++;
  items.value = [...items.value, { id: `c${seq.value}`, pod: "stray", name: "x", state: "running", left: 0, startup: 0 }];
  logf("kubelet の知らないところで stray/x が現れた");
}
function reset() {
  items.value = [];
  seq.value = 0;
  linked.value = false;
  apiPods.value = [];
  now.value = 0;
  relists.value = 0;
  restarts.value = {};
  waitUntil.value = {};
  log.value = [];
  for (let i = 0; i < 6; i++) tick();
}
reset();

const declared = computed(() =>
  desired().flatMap(({ pod, src }) => pod.containers.map((c) => ({ key: `${pod.name}/${c.name}`, src }))),
);
const actual = computed(() =>
  [...items.value]
    .sort((a, b) => (a.pod !== b.pod ? (a.pod < b.pod ? -1 : 1) : a.name < b.name ? -1 : 1))
    .map((i) => ({ key: `${i.pod}/${i.name}`, state: i.state })),
);
const declaredKeys = computed(() => new Set(declared.value.map((d) => d.key)));
const actualKeys = computed(() => new Set(actual.value.map((a) => a.key)));
const badge = computed(() => `一覧の取り直し ${relists.value} 回 / t=${now.value}`);
const verdict = computed(() => {
  const missing = declared.value.filter((d) => !actualKeys.value.has(d.key));
  const extra = actual.value.filter((a) => !declaredKeys.value.has(a.key));
  const down = actual.value.filter((a) => a.state === "exited");
  if (extra.length) return `${extra.map((e) => e.key).join("、")} は宣言に無い。次の周で消える`;
  if (down.length) return `${down.map((d) => d.key).join("、")} が落ちている。次の周で気づいて作り直す`;
  if (missing.length) return `${missing.map((m) => m.key).join("、")} がまだ無い。作り直しの待ち時間か、これから作る`;
  return linked.value
    ? "宣言と実際が一致している。置き場からの宣言も、ファイルの宣言も、両方が満たされている"
    : "置き場に届かないまま、最後に知っている宣言のとおりに動き続けている";
});
const verdictTone = computed(() =>
  actual.value.some((a) => a.state === "exited" || !declaredKeys.value.has(a.key)) ? "bad" : "ok",
);
</script>

<template>
  <DemoShell title="kubelet と CRI" :badge="badge" badge-tone="ok">
    <div class="kl-actions">
      <button class="sd-btn sd-btn--primary" @click="tick">1 tick 進める</button>
      <button class="sd-btn" @click="killOne">web を外から落とす</button>
      <button class="sd-btn" @click="stray">知らないコンテナを増やす</button>
      <span class="kl-gap" />
      <button class="sd-btn" :class="linked ? '' : 'sd-btn--primary'" @click="toggleLink">
        置き場: {{ linked ? "届く" : "届かない" }}
      </button>
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <div class="kl-cols">
      <div class="kl-col">
        <div class="kl-col-h">宣言(あるべき姿)</div>
        <div v-for="d in declared" :key="d.key" class="kl-item mono" :class="actualKeys.has(d.key) ? '' : 'missing'">
          <span>{{ d.key }}</span><em>{{ d.src }}</em>
        </div>
        <div v-if="!declared.length" class="kl-empty">(まだ何も宣言されていない)</div>
      </div>
      <div class="kl-col">
        <div class="kl-col-h">実際(ランタイムの一覧)</div>
        <div
          v-for="a in actual"
          :key="a.key"
          class="kl-item mono"
          :class="[a.state, declaredKeys.has(a.key) ? '' : 'extra']"
        >
          <span>{{ a.key }}</span><em>{{ a.state }}</em>
        </div>
        <div v-if="!actual.length" class="kl-empty">(何も動いていない)</div>
      </div>
    </div>

    <div class="kl-verdict" :class="verdictTone">{{ verdict }}</div>

    <div class="kl-log">
      <div v-for="(l, i) in log.slice(-6)" :key="i" class="kl-log-line mono">{{ l }}</div>
    </div>

    <p class="kl-legend">
      置き場が存在しないところから始まっている。ファイルの宣言だけで kube-apiserver が立ち上がり、
      立ってから web と worker が届く。「置き場: 届かない」に切り替えても、最後に知っている宣言のまま
      動き続ける。外からプロセスを落としたり、知らないコンテナを増やしたりすると、次の周で一覧を
      取り直したときに気づいて直す。変化の通知を待っていないので、誰にも知らされなくても必ず気づく。
    </p>
  </DemoShell>
</template>

<style scoped>
.kl-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.kl-gap {
  flex: 1;
  min-width: 8px;
}
.kl-cols {
  display: flex;
  gap: 10px;
  margin-top: 14px;
  flex-wrap: wrap;
}
.kl-col {
  flex: 1;
  min-width: 220px;
  border: 1px solid var(--vp-c-divider);
  padding: 9px 12px;
  background-color: var(--vp-c-bg-soft);
}
.kl-col-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 6px;
}
.kl-item {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  font-size: 10.5px;
  padding: 3px 8px;
  margin-bottom: 3px;
  border: 1px solid var(--vp-c-divider);
  color: var(--vp-c-text-2);
}
.kl-item em {
  font-style: normal;
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.kl-item.running {
  border-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.kl-item.creating {
  border-color: var(--vp-c-warning-1);
  color: var(--vp-c-warning-1);
  background-color: var(--vp-c-warning-soft);
}
.kl-item.exited,
.kl-item.extra,
.kl-item.missing {
  border-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.kl-empty {
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.kl-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  font-weight: 600;
  line-height: 1.6;
  background-color: var(--vp-c-bg-soft);
}
.kl-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.kl-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.kl-log {
  margin-top: 10px;
  border-top: 1px solid var(--vp-c-divider);
  padding-top: 6px;
  min-height: 60px;
}
.kl-log-line {
  font-size: 10px;
  color: var(--vp-c-text-2);
  padding: 1px 0;
}
.kl-legend {
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
