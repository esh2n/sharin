<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/apiserver(Go)を移植。真実(置き場)と写し(informer)を並べ、
// watch が切れている間にずれること、張り直すと追いつくことを見せる。

interface Obj {
  name: string;
  value: string;
  version: number;
}
interface Ev {
  type: "added" | "modified" | "deleted";
  name: string;
  version: number;
}

const HISTORY_KEEP = 4;

const objs = ref<Record<string, Obj>>({});
const version = ref(0);
const history = ref<Ev[]>([]);
const reads = ref(0);

const cache = ref<Record<string, Obj>>({});
const cacheVersion = ref(0);
const connected = ref(true);
const resyncs = ref(0);
const log = ref<string[]>([]);
const seq = ref(0);

function put(name: string, value: string) {
  version.value++;
  const type = objs.value[name] ? "modified" : "added";
  objs.value = { ...objs.value, [name]: { name, value, version: version.value } };
  history.value = [...history.value, { type, name, version: version.value }].slice(-HISTORY_KEEP);
  log.value = [...log.value, `置き場: ${name} を ${type === "added" ? "作成" : "更新"}(版 ${version.value})`];
}
function del(name: string) {
  if (!objs.value[name]) return;
  version.value++;
  const next = { ...objs.value };
  delete next[name];
  objs.value = next;
  history.value = [...history.value, { type: "deleted", name, version: version.value }].slice(-HISTORY_KEEP);
  log.value = [...log.value, `置き場: ${name} を削除(版 ${version.value})`];
}

function fullResync() {
  reads.value++;
  resyncs.value++;
  cache.value = { ...objs.value };
  cacheVersion.value = version.value;
  log.value = [...log.value, `写し: 全件を読み込んだ(版 ${version.value})`];
}

function sync() {
  if (!connected.value) return;
  const oldest = history.value.length ? history.value[0].version : version.value + 1;
  if (history.value.length > 0 && cacheVersion.value < oldest - 1) {
    log.value = [...log.value, "写し: 履歴が古すぎて差分で追いつけない"];
    fullResync();
    return;
  }
  const next = { ...cache.value };
  for (const e of history.value) {
    if (e.version <= cacheVersion.value) continue;
    if (e.type === "deleted") delete next[e.name];
    else next[e.name] = objs.value[e.name];
  }
  cache.value = next;
  cacheVersion.value = version.value;
}

function addPod() {
  seq.value++;
  put(`web-${seq.value}`, "running");
  sync();
}
function updateFirst() {
  const first = Object.keys(objs.value).sort()[0];
  if (!first) return;
  put(first, "stopped");
  sync();
}
function deleteFirst() {
  const first = Object.keys(objs.value).sort()[0];
  if (!first) return;
  del(first);
  sync();
}
function disconnect() {
  connected.value = false;
  log.value = [...log.value, "写し: watch が切れた。この間の変更は届かない"];
}
function reconnect() {
  connected.value = true;
  log.value = [...log.value, "写し: watch を張り直した"];
  sync();
}
function reset() {
  objs.value = {};
  version.value = 0;
  history.value = [];
  reads.value = 0;
  cache.value = {};
  cacheVersion.value = 0;
  connected.value = true;
  resyncs.value = 0;
  log.value = [];
  seq.value = 0;
  addPod();
  addPod();
  fullResync();
  log.value = [];
}
reset();

const truth = computed(() => Object.values(objs.value).sort((a, b) => (a.name < b.name ? -1 : 1)));
const copy = computed(() => Object.values(cache.value).sort((a, b) => (a.name < b.name ? -1 : 1)));
const lag = computed(() => version.value - cacheVersion.value);
const stale = computed(() => lag.value > 0);
const badge = computed(() => `真実 版${version.value} / 写し 版${cacheVersion.value}`);
const badgeTone = computed<"ok" | "ng">(() => (stale.value ? "ng" : "ok"));
const diff = (name: string) => {
  const inTruth = !!objs.value[name];
  const inCopy = !!cache.value[name];
  if (inTruth && inCopy && objs.value[name].value !== cache.value[name].value) return "diff";
  if (inTruth !== inCopy) return "diff";
  return "";
};
</script>

<template>
  <DemoShell title="APIサーバとinformer" :badge="badge" :badge-tone="badgeTone">
    <div class="as-actions">
      <button class="sd-btn sd-btn--primary" @click="addPod">Pod を作る</button>
      <button class="sd-btn" @click="updateFirst">先頭を更新</button>
      <button class="sd-btn" @click="deleteFirst">先頭を削除</button>
      <span class="as-spacer" />
      <button class="sd-btn" @click="reset">リセット</button>
    </div>
    <div class="as-actions as-second">
      <button class="sd-btn" :disabled="!connected" @click="disconnect">watch を切る</button>
      <button class="sd-btn" :disabled="connected" @click="reconnect">watch を張り直す</button>
      <span class="as-hint mono">
        watch: {{ connected ? "繋がっている" : "切れている" }} ・ 全件読み出し {{ reads }} 回 ・ 履歴は直近 {{ HISTORY_KEEP }} 件だけ保持
      </span>
    </div>

    <div class="as-cols">
      <div class="as-col truth">
        <div class="as-col-h">
          <span>置き場(唯一の真実)</span><span class="mono as-ver">版 {{ version }}</span>
        </div>
        <div v-for="o in truth" :key="o.name" class="as-obj mono" :class="diff(o.name)">
          {{ o.name }}<small>{{ o.value }}</small>
        </div>
        <div v-if="truth.length === 0" class="as-empty">(なし)</div>
      </div>

      <div class="as-col copy" :class="connected ? '' : 'cut'">
        <div class="as-col-h">
          <span>写し(informer)</span><span class="mono as-ver">版 {{ cacheVersion }}</span>
        </div>
        <div v-for="o in copy" :key="o.name" class="as-obj mono" :class="diff(o.name)">
          {{ o.name }}<small>{{ o.value }}</small>
        </div>
        <div v-if="copy.length === 0" class="as-empty">(なし)</div>
      </div>
    </div>

    <div class="as-verdict" :class="stale ? 'bad' : 'ok'">
      <template v-if="stale">
        写しが {{ lag }} 版遅れている。読み手はこの古い状態を見て判断する。取りこぼしても数え直せば追いつく設計でなければ動けない
      </template>
      <template v-else>写しが真実に追いついている。読むのは写しなので、何度読んでも置き場には触らない</template>
    </div>

    <div class="as-log">
      <div class="as-log-h">起きたこと</div>
      <div v-for="(l, i) in log.slice(-8)" :key="i" class="as-log-line mono">{{ l }}</div>
      <div v-if="log.length === 0" class="as-empty">(まだ何も起きていない)</div>
    </div>

    <p class="as-legend">
      左が唯一の真実、右がコントローラの手元にある写し。繋がっている間は、変更が差分で届いて写しが追いつく。
      「watch を切る」を押してから Pod を作ったり消したりすると、写しだけが取り残される。消えたはずのものが
      写しには残り、読み手はそれを見て判断する。張り直せば追いつくが、履歴は直近 {{ HISTORY_KEEP }} 件しか
      残っていないので、切れている間に変更が多すぎると差分では追いつけず、全件を取り直すことになる。
    </p>
  </DemoShell>
</template>

<style scoped>
.as-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.as-second {
  margin-top: 8px;
}
.as-spacer {
  flex: 1;
}
.as-hint {
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.as-cols {
  display: flex;
  gap: 10px;
  margin-top: 14px;
  flex-wrap: wrap;
}
.as-col {
  flex: 1;
  min-width: 200px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
}
.as-col.truth {
  border-color: var(--vp-c-brand-1);
}
.as-col.copy.cut {
  border-style: dashed;
  border-color: var(--vp-c-warning-1);
}
.as-col-h {
  display: flex;
  align-items: baseline;
  gap: 8px;
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 8px;
}
.as-ver {
  margin-left: auto;
  font-size: 10px;
  font-weight: 400;
  color: var(--vp-c-text-3);
}
.as-obj {
  display: flex;
  justify-content: space-between;
  font-size: 10.5px;
  padding: 3px 7px;
  border: 1px solid var(--vp-c-divider);
  margin-bottom: 3px;
  color: var(--vp-c-text-2);
}
.as-obj small {
  color: var(--vp-c-text-3);
  font-size: 9.5px;
}
.as-obj.diff {
  border-color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
}
.as-empty {
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.as-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
  background-color: var(--vp-c-bg-soft);
}
.as-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.as-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.as-log {
  margin-top: 12px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
  min-height: 50px;
}
.as-log-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 4px;
}
.as-log-line {
  font-size: 10.5px;
  color: var(--vp-c-text-2);
  padding: 1px 0;
}
.as-legend {
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
