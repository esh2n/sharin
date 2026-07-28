<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/config(Go)を移植。同じ設定を同じように更新しても、
// 受け取り方だけで届くかどうかが変わることを並べて見せる。

interface PodState {
  name: string;
  source: "env" | "file";
  env: Record<string, string>;
  bornVersion: number;
}

const store = ref<{ data: Record<string, string>; version: number }>({
  data: { log_level: "info", timeout: "30" },
  version: 1,
});
const pods = ref<PodState[]>([]);
const log = ref<string[]>([]);

function start(name: string, source: "env" | "file") {
  const p: PodState = {
    name,
    source,
    env: source === "env" ? { ...store.value.data } : {},
    bornVersion: store.value.version,
  };
  pods.value = [...pods.value.filter((x) => x.name !== name), p].sort((a, b) =>
    a.name < b.name ? -1 : 1,
  );
  log.value = [...log.value, `${name} を起動(${source === "env" ? "環境変数は起動時の値を写し取る" : "ファイルは実体を見に行く"})`];
}

function read(p: PodState, key: string): string {
  return p.source === "env" ? (p.env[key] ?? "") : (store.value.data[key] ?? "");
}
const isStale = (p: PodState) => p.source === "env" && store.value.version > p.bornVersion;

function update(level: string) {
  store.value = {
    data: { ...store.value.data, log_level: level },
    version: store.value.version + 1,
  };
  log.value = [...log.value, `ConfigMap app-config を更新(log_level=${level}、版 ${store.value.version})`];
}
function restart(name: string) {
  const p = pods.value.find((x) => x.name === name);
  if (!p) return;
  log.value = [...log.value, `${name} を作り直す`];
  start(name, p.source);
}
function reset() {
  store.value = { data: { log_level: "info", timeout: "30" }, version: 1 };
  pods.value = [];
  log.value = [];
  start("env-pod", "env");
  start("file-pod", "file");
  log.value = [];
}
reset();

const staleCount = computed(() => pods.value.filter(isStale).length);
const badge = computed(() => `ConfigMap 版 ${store.value.version} ・ 古い Pod ${staleCount.value}`);
const badgeTone = computed<"ok" | "ng">(() => (staleCount.value > 0 ? "ng" : "ok"));
</script>

<template>
  <DemoShell title="ConfigMapとSecret" :badge="badge" :badge-tone="badgeTone">
    <div class="cf-actions">
      <span class="cf-label">ConfigMap を書き換える</span>
      <button class="sd-btn sd-btn--primary" @click="update('debug')">log_level=debug</button>
      <button class="sd-btn" @click="update('warn')">log_level=warn</button>
      <button class="sd-btn" @click="update('info')">log_level=info</button>
      <span class="cf-spacer" />
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <div class="cf-store">
      <div class="cf-store-h">
        <span class="mono">ConfigMap app-config</span>
        <span class="mono cf-ver">版 {{ store.version }}</span>
      </div>
      <div class="cf-kv mono" v-for="(v, k) in store.data" :key="k"><span>{{ k }}</span><b>{{ v }}</b></div>
    </div>

    <div class="cf-pods">
      <div v-for="p in pods" :key="p.name" class="cf-pod" :class="isStale(p) ? 'stale' : 'fresh'">
        <div class="cf-pod-h">
          <span class="mono cf-pod-n">{{ p.name }}</span>
          <span class="cf-pod-src mono">{{ p.source === "env" ? "環境変数で受け取る" : "ファイルで受け取る" }}</span>
        </div>
        <div class="cf-kv mono"><span>log_level</span><b>{{ read(p, "log_level") }}</b></div>
        <div class="cf-kv mono"><span>timeout</span><b>{{ read(p, "timeout") }}</b></div>
        <div class="cf-pod-f">
          <span class="mono cf-born">起動時に見えていた版 {{ p.bornVersion }}</span>
          <button v-if="isStale(p)" class="cf-mini" @click="restart(p.name)">作り直す</button>
        </div>
      </div>
    </div>

    <div class="cf-verdict" :class="staleCount > 0 ? 'bad' : 'ok'">
      <template v-if="staleCount > 0">
        env-pod だけが古い値のまま。環境変数は起動時に写し取られているので、置き場が変わっても届かない。反映するには作り直すしかない
      </template>
      <template v-else>
        両方が今の値を見ている。ファイル側は実体を見に行くので、書き換えればそのまま変わる
      </template>
    </div>

    <div class="cf-log">
      <div class="cf-log-h">起きたこと</div>
      <div v-for="(l, i) in log.slice(-7)" :key="i" class="cf-log-line mono">{{ l }}</div>
      <div v-if="log.length === 0" class="cf-empty">(まだ何も起きていない)</div>
    </div>

    <p class="cf-legend">
      同じ ConfigMap を、env-pod は環境変数で、file-pod はファイルで受け取っている。書き換えると file-pod だけが
      新しい値を見る。env-pod はプロセスに渡された時点で値が確定しているので、置き場が変わっても届かない。
      設定を変えたのに反映されない、という形で表に出るのがこれで、直すには作り直すしかない。
      Secret も仕組みは同じで、違うのは扱いの慎重さだけになる。仕組みとしての秘匿はほとんど無い。
    </p>
  </DemoShell>
</template>

<style scoped>
.cf-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.cf-label {
  font-size: 11.5px;
  color: var(--vp-c-text-2);
}
.cf-spacer {
  flex: 1;
}
.cf-store {
  margin-top: 14px;
  border: 1px solid var(--vp-c-brand-1);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
}
.cf-store-h {
  display: flex;
  align-items: baseline;
  gap: 10px;
  margin-bottom: 6px;
  font-size: 12px;
  font-weight: 700;
  color: var(--vp-c-brand-1);
}
.cf-ver {
  margin-left: auto;
  font-size: 10px;
  font-weight: 400;
  color: var(--vp-c-text-3);
}
.cf-kv {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  font-size: 11px;
  padding: 2px 0;
  color: var(--vp-c-text-3);
}
.cf-kv b {
  color: var(--vp-c-text-1);
  font-weight: 700;
}
.cf-pods {
  display: flex;
  gap: 10px;
  margin-top: 12px;
  flex-wrap: wrap;
}
.cf-pod {
  flex: 1;
  min-width: 220px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
}
.cf-pod.fresh {
  border-color: var(--vp-c-green-1);
}
.cf-pod.stale {
  border-color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.cf-pod-h {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-bottom: 6px;
}
.cf-pod-n {
  font-size: 12px;
  font-weight: 700;
}
.cf-pod-src {
  font-size: 9.5px;
  color: var(--vp-c-text-3);
  margin-left: auto;
}
.cf-pod-f {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
}
.cf-born {
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.cf-mini {
  font-size: 9.5px;
  padding: 2px 8px;
  border: 1px solid var(--vp-c-danger-1);
  background-color: var(--vp-c-bg);
  color: var(--vp-c-danger-1);
  cursor: pointer;
  margin-left: auto;
}
.cf-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
  background-color: var(--vp-c-bg-soft);
}
.cf-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.cf-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.cf-log {
  margin-top: 12px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
  min-height: 50px;
}
.cf-log-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 4px;
}
.cf-log-line {
  font-size: 10.5px;
  color: var(--vp-c-text-2);
  padding: 1px 0;
}
.cf-empty {
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.cf-legend {
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
