<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/service(Go)を移植。仮想 IP に実体はなく、各ノードに配られた
// ルールで宛先が書き換わること、その配布に遅れがあることを見せる。

const VIP = "10.0.0.1";
const PROP = 3;

interface Pod {
  name: string;
  ip: string;
  ready: boolean;
}
interface NodeT {
  name: string;
  rules: string[];
  rr: number;
}
interface Delivery {
  at: number;
  node: string;
  backends: string[];
}

const pods = ref<Pod[]>([]);
const nodes = ref<NodeT[]>([]);
const queue = ref<Delivery[]>([]);
const now = ref(0);
const seq = ref(0);
const sent = ref(0);
const black = ref(0);
const dropped = ref(0);
const log = ref<string[]>([]);
const lastHit = ref<{ node: string; ip: string; ok: boolean } | null>(null);

// 制御側から見た「あるべき宛先」。ready な Pod だけ。
const endpoints = computed(() => pods.value.filter((p) => p.ready).map((p) => p.ip));

function publish() {
  const want = endpoints.value;
  for (const n of nodes.value) {
    queue.value = [...queue.value, { at: now.value + PROP, node: n.name, backends: [...want] }];
  }
}

function tick() {
  now.value++;
  const rest: Delivery[] = [];
  for (const d of queue.value) {
    if (now.value < d.at) {
      rest.push(d);
      continue;
    }
    const n = nodes.value.find((x) => x.name === d.node);
    if (n) {
      n.rules = d.backends;
      log.value = [...log.value, `t=${now.value} ${n.name} のルールを更新(宛先 ${d.backends.length} 本)`];
    }
  }
  queue.value = rest;
  nodes.value = [...nodes.value];
}

function send(n: NodeT) {
  if (n.rules.length === 0) {
    dropped.value++;
    lastHit.value = { node: n.name, ip: "", ok: false };
    log.value = [...log.value, `t=${now.value} ${n.name} に ${VIP} のルールがない。行き場を失う`];
    return;
  }
  const ip = n.rules[n.rr % n.rules.length];
  n.rr++;
  nodes.value = [...nodes.value];
  const p = pods.value.find((x) => x.ip === ip);
  if (!p || !p.ready) {
    black.value++;
    lastHit.value = { node: n.name, ip, ok: false };
    log.value = [...log.value, `t=${now.value} ${n.name} のルールが ${ip} を指しているが、そこはもう受けられない`];
    return;
  }
  sent.value++;
  lastHit.value = { node: n.name, ip, ok: true };
}

function sendAll() {
  for (const n of nodes.value) send(n);
}
function addPod() {
  seq.value++;
  pods.value = [...pods.value, { name: `web-${seq.value}`, ip: `10.1.0.${seq.value}`, ready: true }];
  publish();
}
function removePod() {
  if (pods.value.length === 0) return;
  const victim = pods.value[pods.value.length - 1];
  pods.value = pods.value.slice(0, -1);
  log.value = [...log.value, `t=${now.value} ${victim.name} を削除(ルールの更新は ${PROP} 周期後)`];
  publish();
}
function toggleReady(p: Pod) {
  p.ready = !p.ready;
  pods.value = [...pods.value];
  publish();
}
function reset() {
  now.value = 0;
  seq.value = 0;
  pods.value = [];
  nodes.value = [
    { name: "node-a", rules: [], rr: 0 },
    { name: "node-b", rules: [], rr: 0 },
  ];
  queue.value = [];
  sent.value = 0;
  black.value = 0;
  dropped.value = 0;
  log.value = [];
  lastHit.value = null;
  for (let i = 0; i < 3; i++) addPod();
  for (let i = 0; i < PROP + 1; i++) tick();
  log.value = [];
}
reset();

// 各ノードのルールが、あるべき宛先と一致しているか。
const converged = computed(() =>
  nodes.value.every(
    (n) => n.rules.length === endpoints.value.length && n.rules.every((ip, i) => ip === endpoints.value[i]),
  ),
);
const badge = computed(() => `届いた ${sent.value} / 死んだ宛先 ${black.value} / 行き場なし ${dropped.value}`);
const badgeTone = computed<"ok" | "ng" | "neutral">(() =>
  black.value + dropped.value > 0 ? "ng" : sent.value > 0 ? "ok" : "neutral",
);
const podByIP = (ip: string) => pods.value.find((p) => p.ip === ip);
</script>

<template>
  <DemoShell title="Serviceとkube-proxy" :badge="badge" :badge-tone="badgeTone">
    <div class="sv-actions">
      <button class="sd-btn sd-btn--primary" @click="sendAll">両ノードからパケットを1つずつ送る</button>
      <button class="sd-btn" @click="tick">1周期進める</button>
      <span class="sv-spacer" />
      <span class="sv-clock mono">t={{ now }}</span>
      <button class="sd-btn" @click="reset">リセット</button>
    </div>
    <div class="sv-actions sv-second">
      <button class="sd-btn" @click="addPod">Podを増やす</button>
      <button class="sd-btn" @click="removePod">Podを消す</button>
      <span class="sv-hint mono">ルールの配布には {{ PROP }} 周期かかる</span>
    </div>

    <div class="sv-cols">
      <div class="sv-col">
        <div class="sv-col-h">制御側が見ている「あるべき宛先」</div>
        <div class="sv-vip mono">{{ VIP }}<small>ClusterIP(実体を持たない仮想の宛先)</small></div>
        <div class="sv-pods">
          <span
            v-for="p in pods"
            :key="p.name"
            class="sv-pod mono"
            :class="p.ready ? 'on' : 'off'"
            @click="toggleReady(p)"
          >{{ p.name }} {{ p.ip }}<small>{{ p.ready ? "ready" : "not ready" }}</small></span>
          <span v-if="pods.length === 0" class="sv-empty">(Pod なし)</span>
        </div>
        <div class="sv-note">Pod をクリックすると ready を切り替えられる</div>
      </div>

      <div class="sv-col">
        <div class="sv-col-h">各ノードに配られたルール</div>
        <div v-for="n in nodes" :key="n.name" class="sv-node">
          <div class="sv-node-h mono">{{ n.name }}</div>
          <div class="sv-rules">
            <span
              v-for="(ip, i) in n.rules"
              :key="i"
              class="sv-rule mono"
              :class="podByIP(ip)?.ready ? 'live' : 'dead'"
            >{{ VIP }} → {{ ip }}</span>
            <span v-if="n.rules.length === 0" class="sv-empty">(ルールなし)</span>
          </div>
        </div>
      </div>
    </div>

    <div class="sv-verdict" :class="converged ? 'ok' : 'bad'">
      <template v-if="!converged">
        ルールがまだ配り終わっていない。この間に出したパケットは、もう受けられない宛先へ書き換わる
      </template>
      <template v-else-if="lastHit && lastHit.ok">
        {{ lastHit.node }} のルールが {{ lastHit.ip }} へ書き換えた。届いた
      </template>
      <template v-else-if="lastHit">{{ lastHit.node }} から出したパケットが届かなかった</template>
      <template v-else>ルールは配り終わっている。あるべき宛先と一致</template>
    </div>

    <div class="sv-log">
      <div class="sv-log-h">起きたこと</div>
      <div v-for="(l, i) in log.slice(-7)" :key="i" class="sv-log-line mono">{{ l }}</div>
      <div v-if="log.length === 0" class="sv-empty">(まだ何も起きていない)</div>
    </div>

    <p class="sv-legend">
      左が制御側の見ている宛先、右が各ノードに実際に配られたルール。仮想 IP で待ち受けている者はどこにもおらず、
      パケットが出ていく瞬間に、そのノードのルールで宛先が書き換わる。「Podを消す」を押してから周期を進めずに
      パケットを送ると、まだ古いルールが残っているので、消えたはずの宛先へ飛ぶ。これが終了処理の章で
      「転送先一覧が現実に遅れる」と呼んだものの正体になる。
    </p>
  </DemoShell>
</template>

<style scoped>
.sv-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.sv-second {
  margin-top: 8px;
}
.sv-spacer {
  flex: 1;
}
.sv-clock,
.sv-hint {
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.sv-cols {
  display: flex;
  gap: 10px;
  margin-top: 14px;
  flex-wrap: wrap;
}
.sv-col {
  flex: 1;
  min-width: 250px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
}
.sv-col-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 8px;
}
.sv-vip {
  font-size: 15px;
  font-weight: 700;
  color: var(--vp-c-brand-1);
  margin-bottom: 8px;
}
.sv-vip small {
  display: block;
  font-size: 9.5px;
  font-weight: 400;
  color: var(--vp-c-text-3);
}
.sv-pods {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.sv-pod {
  font-size: 10.5px;
  padding: 3px 7px;
  border: 1px solid var(--vp-c-divider);
  cursor: pointer;
}
.sv-pod small {
  float: right;
  font-size: 9px;
  opacity: 0.8;
}
.sv-pod.on {
  border-color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.sv-pod.off {
  border-color: var(--vp-c-text-3);
  color: var(--vp-c-text-3);
}
.sv-note {
  margin-top: 6px;
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.sv-node {
  margin-bottom: 8px;
}
.sv-node-h {
  font-size: 11px;
  font-weight: 700;
  margin-bottom: 3px;
}
.sv-rules {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.sv-rule {
  font-size: 10.5px;
  padding: 2px 7px;
  border: 1px solid var(--vp-c-divider);
}
.sv-rule.live {
  border-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
}
.sv-rule.dead {
  border-color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
}
.sv-empty {
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.sv-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
  background-color: var(--vp-c-bg-soft);
}
.sv-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.sv-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.sv-log {
  margin-top: 12px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
  min-height: 54px;
}
.sv-log-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 4px;
}
.sv-log-line {
  font-size: 10.5px;
  color: var(--vp-c-text-2);
  padding: 1px 0;
}
.sv-legend {
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
