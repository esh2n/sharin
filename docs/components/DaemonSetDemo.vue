<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/daemonset(Go)を移植。数を宣言せず場所を宣言することと、
// ノードの増減に自動で追随することを見せる。

interface NodeT {
  name: string;
  ready: boolean;
  labels: Record<string, string>;
  taint: string;
}

const nodes = ref<NodeT[]>([]);
const pods = ref<Record<string, string>>({}); // ノード名 → Pod 名
const seq = ref(0);
const log = ref<string[]>([]);

const selectorEdge = ref(false);
const tolerateAll = ref(true);

function isTarget(n: NodeT): boolean {
  if (!n.ready) return false;
  if (selectorEdge.value && n.labels.role !== "edge") return false;
  if (n.taint !== "" && !tolerateAll.value) return false;
  return true;
}
const targets = computed(() => nodes.value.filter(isTarget).map((n) => n.name).sort());
const desired = computed(() => targets.value.length);

function reconcile() {
  const msgs: string[] = [];
  const want = new Set(targets.value);
  const next = { ...pods.value };
  for (const name of targets.value) {
    if (!next[name]) {
      next[name] = `log-agent-${name}`;
      msgs.push(`${name} に ${next[name]} を作成`);
    }
  }
  for (const name of Object.keys(next).sort()) {
    if (!want.has(name)) {
      msgs.push(`${name} から ${next[name]} を削除(対象から外れた)`);
      delete next[name];
    }
  }
  pods.value = next;
  if (msgs.length === 0) msgs.push("差が無い。何もしない(冪等)");
  log.value = [...log.value, ...msgs];
}

function addNode(edge = false) {
  seq.value++;
  nodes.value = [
    ...nodes.value,
    { name: `node-${seq.value}`, ready: true, labels: edge ? { role: "edge" } : {}, taint: "" },
  ];
  log.value = [...log.value, `node-${seq.value} が加わった(まだ調整していない)`];
}
function addTainted() {
  seq.value++;
  nodes.value = [
    ...nodes.value,
    { name: `infra-${seq.value}`, ready: true, labels: {}, taint: "dedicated=infra" },
  ];
  log.value = [...log.value, `infra-${seq.value} が加わった(汚れ付き)`];
}
function removeLast() {
  if (nodes.value.length === 0) return;
  const n = nodes.value[nodes.value.length - 1];
  nodes.value = nodes.value.slice(0, -1);
  const next = { ...pods.value };
  delete next[n.name];
  pods.value = next;
  log.value = [...log.value, `${n.name} が取り除かれた(載っていた Pod も消える)`];
}
function toggleReady(n: NodeT) {
  n.ready = !n.ready;
  nodes.value = [...nodes.value];
  log.value = [...log.value, `${n.name} を ${n.ready ? "ready" : "not ready"} にした`];
}
function reset() {
  nodes.value = [];
  pods.value = {};
  seq.value = 0;
  log.value = [];
  addNode();
  addNode();
  addNode(true);
  addTainted();
  reconcile();
  log.value = [];
}
reset();

const converged = computed(
  () =>
    Object.keys(pods.value).length === desired.value &&
    targets.value.every((n) => pods.value[n] !== undefined),
);
const badge = computed(() => `対象 ${desired.value} / 載っている ${Object.keys(pods.value).length}`);
const badgeTone = computed<"ok" | "ng">(() => (converged.value ? "ok" : "ng"));
</script>

<template>
  <DemoShell title="DaemonSet" :badge="badge" :badge-tone="badgeTone">
    <div class="ds-row">
      <span class="ds-label">どこに要るか(数は宣言しない)</span>
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: !selectorEdge }" @click="selectorEdge = false">すべてのノード</span>
        <span class="sd-seg-opt" :class="{ on: selectorEdge }" @click="selectorEdge = true">role=edge のみ</span>
      </span>
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: tolerateAll }" @click="tolerateAll = true">汚れも許容</span>
        <span class="sd-seg-opt" :class="{ on: !tolerateAll }" @click="tolerateAll = false">汚れは避ける</span>
      </span>
    </div>

    <div class="ds-actions">
      <button class="sd-btn sd-btn--primary" @click="reconcile">調整する</button>
      <button class="sd-btn" @click="addNode()">ノードを足す</button>
      <button class="sd-btn" @click="addNode(true)">edge ノードを足す</button>
      <button class="sd-btn" @click="addTainted">汚れ付きノードを足す</button>
      <button class="sd-btn" @click="removeLast">最後のノードを外す</button>
      <span class="ds-spacer" />
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <div class="ds-nodes">
      <div
        v-for="n in nodes"
        :key="n.name"
        class="ds-node"
        :class="[isTarget(n) ? 'target' : 'off', pods[n.name] ? 'has' : '']"
      >
        <div class="ds-node-h">
          <span class="mono ds-node-n">{{ n.name }}</span>
          <span class="ds-toggle" @click="toggleReady(n)">{{ n.ready ? "ready" : "not ready" }}</span>
        </div>
        <div class="ds-meta mono">
          <span v-if="n.labels.role">role={{ n.labels.role }}</span>
          <span v-if="n.taint" class="ds-taint">{{ n.taint }}</span>
          <span v-if="!n.labels.role && !n.taint">(ラベルも汚れも無し)</span>
        </div>
        <div class="ds-pod mono" :class="pods[n.name] ? 'on' : 'none'">
          {{ pods[n.name] ?? (isTarget(n) ? "(未配置。調整すると置かれる)" : "(対象外)") }}
        </div>
      </div>
      <div v-if="nodes.length === 0" class="ds-empty">(ノードが無い)</div>
    </div>

    <div class="ds-verdict" :class="converged ? 'ok' : 'bad'">
      <template v-if="converged">
        対象 {{ desired }} 台すべてに1つずつ載っている。数はどこにも宣言していない
      </template>
      <template v-else>
        対象 {{ desired }} 台に対し {{ Object.keys(pods).length }} 個。調整すると差が埋まる
      </template>
    </div>

    <div class="ds-log">
      <div class="ds-log-h">起きたこと</div>
      <div v-for="(l, i) in log.slice(-7)" :key="i" class="ds-log-line mono">{{ l }}</div>
      <div v-if="log.length === 0" class="ds-empty">(まだ何も起きていない)</div>
    </div>

    <p class="ds-legend">
      宣言しているのは「どこに要るか」だけで、数はどこにも書いていない。ノードを足して「調整する」を押すと、
      そのノードにだけ Pod が作られる。外せば載っていた Pod も消える。ready を外すと対象から外れて Pod が消え、
      戻すとまた置かれる。「role=edge のみ」に切り替えると対象が絞られ、「汚れは避ける」に切り替えると
      汚れ付きノードが対象から外れる。監視や収集は、他の Pod が避けるノードにも置かれてほしいので、
      普通は広く許容する。
    </p>
  </DemoShell>
</template>

<style scoped>
.ds-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}
.ds-label {
  font-size: 11.5px;
  color: var(--vp-c-text-2);
  min-width: 190px;
}
.ds-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.ds-spacer {
  flex: 1;
}
.ds-nodes {
  display: flex;
  gap: 8px;
  margin-top: 14px;
  flex-wrap: wrap;
}
.ds-node {
  flex: 1;
  min-width: 160px;
  border: 1px solid var(--vp-c-divider);
  padding: 8px 10px;
  background-color: var(--vp-c-bg-soft);
}
.ds-node.target {
  border-color: var(--vp-c-brand-1);
}
.ds-node.off {
  opacity: 0.6;
  border-style: dashed;
}
.ds-node-h {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-bottom: 3px;
}
.ds-node-n {
  font-size: 11.5px;
  font-weight: 700;
}
.ds-toggle {
  margin-left: auto;
  font-size: 9px;
  padding: 1px 6px;
  border: 1px solid var(--vp-c-divider);
  color: var(--vp-c-text-3);
  cursor: pointer;
}
.ds-meta {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  font-size: 9px;
  color: var(--vp-c-text-3);
  margin-bottom: 5px;
}
.ds-taint {
  color: var(--vp-c-warning-1);
}
.ds-pod {
  font-size: 9.5px;
  padding: 3px 6px;
  border: 1px solid var(--vp-c-divider);
}
.ds-pod.on {
  border-color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.ds-pod.none {
  color: var(--vp-c-text-3);
  border-style: dashed;
}
.ds-empty {
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.ds-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
  background-color: var(--vp-c-bg-soft);
}
.ds-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.ds-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.ds-log {
  margin-top: 12px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
  min-height: 50px;
}
.ds-log-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 4px;
}
.ds-log-line {
  font-size: 10.5px;
  color: var(--vp-c-text-2);
  padding: 1px 0;
}
.ds-legend {
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
