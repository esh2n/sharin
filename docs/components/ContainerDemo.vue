<script setup lang="ts">
import { reactive, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// foundations/container(Go) の考え方をブラウザで動かす移植版。
// 1つの host(共有カーネル)の上に 2つのコンテナ。namespace(PID/ネット/マウント)で
// 見え方を分け、cgroup(メモリ上限)で資源を区切る。global PID は host が通しで振る。

interface Proc {
  global: number;
  local: number;
  name: string;
  mem: number; // MiB
  init?: boolean;
}
interface Cont {
  name: string;
  hostname: string;
  memLimit: number; // MiB
  procs: Proc[];
  ports: number[];
  nextLocal: number;
  oom: boolean;
}

const HOST_TOTAL = 512; // MiB(マシン全体)

interface Host {
  nextPid: number;
  used: number;
}

function newContainer(name: string, hostname: string, memLimit: number): Cont {
  return { name, hostname, memLimit, procs: [], ports: [], nextLocal: 1, oom: false };
}

// spawn: cgroup(コンテナ上限 + host 全体)を確認してから、host が global PID を振る。
// 上限超過なら全か無かで拒否し、OOM を立てる。
function spawn(host: Host, c: Cont, name: string, mem: number): boolean {
  const used = c.procs.reduce((s, p) => s + p.mem, 0);
  if (used + mem > c.memLimit || host.used + mem > HOST_TOTAL) {
    c.oom = true;
    return false;
  }
  const global = host.nextPid++;
  const local = c.nextLocal++;
  c.procs.push({ global, local, name, mem, init: local === 1 });
  host.used += mem;
  return true;
}

function bind(c: Cont, port: number) {
  if (!c.ports.includes(port)) c.ports.push(port);
}

// 決定的な初期状態を、固定の spawn 列で組み立てる(mount 時に必ず同じ絵になる)。
function build() {
  const host: Host = { nextPid: 1000, used: 0 };
  const web = newContainer("web", "web", 128);
  const db = newContainer("db", "db", 128);
  spawn(host, web, "init", 4);
  spawn(host, db, "init", 8);
  spawn(host, web, "nginx", 40);
  spawn(host, db, "postgres", 90);
  bind(web, 80);
  bind(db, 80);
  return { host, conts: [web, db] };
}

const state = reactive(build());

function usedOf(c: Cont): number {
  return c.procs.reduce((s, p) => s + p.mem, 0);
}
function pct(c: Cont): number {
  return Math.min(100, Math.round((usedOf(c) / c.memLimit) * 100));
}

// ボタン: web は余裕があるので通る、db は上限に当たって OOM になる(教材上の対比)。
const WORKLOADS: Record<string, { name: string; mem: number }> = {
  web: { name: "worker", mem: 30 },
  db: { name: "cache", mem: 40 },
};
function run(c: Cont) {
  const w = WORKLOADS[c.name];
  spawn(state.host, c, w.name, w.mem);
}
function reset() {
  const b = build();
  state.host = b.host;
  state.conts = b.conts;
}

const allPids = computed(() =>
  state.conts.flatMap((c) => c.procs.map((p) => ({ global: p.global, cont: c.name, name: p.name }))).sort((a, b) => a.global - b.global),
);
const anyOom = computed(() => state.conts.some((c) => c.oom));
const badge = computed(() => (anyOom.value ? "OOM 発生" : `host: ${allPids.value.length} プロセス`));
const badgeTone = computed<"ok" | "ng">(() => (anyOom.value ? "ng" : "ok"));
</script>

<template>
  <DemoShell title="container(namespace + cgroup)" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <button class="sd-btn" @click="run(state.conts[0])">web に worker(+30MiB)</button>
      <button class="sd-btn" @click="run(state.conts[1])">db に cache(+40MiB)</button>
      <span class="spacer" />
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <div class="ct-grid">
      <div v-for="c in state.conts" :key="c.name" class="ct-card">
        <div class="ct-head">
          <span class="ct-name">{{ c.name }}</span>
          <span class="ct-host">hostname: {{ c.hostname }}</span>
          <span class="ct-ports">:{{ c.ports.join(" :") }}</span>
        </div>

        <div class="ct-label">PID 名前空間(コンテナ内の見え方)</div>
        <div class="ct-procs">
          <div v-for="p in c.procs" :key="p.global" class="ct-proc" :class="{ init: p.init }">
            <span class="ct-pid">{{ p.local }}</span>
            <span class="ct-pname">{{ p.name }}<span v-if="p.init" class="ct-tag">PID 1</span></span>
            <span class="ct-pmem">{{ p.mem }}MiB</span>
          </div>
        </div>

        <div class="ct-label">cgroup メモリ({{ usedOf(c) }} / {{ c.memLimit }}MiB)</div>
        <div class="ct-gauge">
          <div class="ct-gauge-fill" :class="{ hot: pct(c) >= 90 }" :style="{ width: pct(c) + '%' }" />
        </div>
        <div v-if="c.oom" class="ct-oom">OOM: メモリ上限に当たり、次のプロセス起動を拒否</div>
      </div>
    </div>

    <div class="ct-label ct-hostlabel">host(共有カーネル)から見た global PID — 通し番号 = 1つの PID 空間</div>
    <div class="ct-hostrow">
      <span v-for="p in allPids" :key="p.global" class="ct-gpid" :class="p.cont">
        {{ p.global }}<span class="ct-gpid-cont">{{ p.cont }}/{{ p.name }}</span>
      </span>
    </div>

    <div class="ct-legend">
      <span>両コンテナの init はどちらも local PID 1、両方が :80 を bind — namespace が資源を別インスタンスに見せている</span>
      <span>db は cgroup 上限 128MiB に当たると OOM。web は無事 — 資源は cgroup で区切られている</span>
    </div>
  </DemoShell>
</template>

<style scoped>
.ct-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-top: 16px;
}
@media (max-width: 520px) {
  .ct-grid {
    grid-template-columns: 1fr;
  }
}
.ct-card {
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg);
  padding: 12px;
}
.ct-head {
  display: flex;
  align-items: baseline;
  gap: 8px;
  flex-wrap: wrap;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--vp-c-divider);
}
.ct-name {
  font-size: 14px;
  font-weight: 700;
  color: var(--vp-c-text-1);
}
.ct-host {
  font-size: 11px;
  color: var(--vp-c-text-3);
  font-family: var(--vp-font-family-mono);
}
.ct-ports {
  margin-left: auto;
  font-size: 11px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-green-1);
}
.ct-label {
  font-size: 11px;
  color: var(--vp-c-text-3);
  margin: 12px 0 5px;
}
.ct-procs {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.ct-proc {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-alt);
  font-size: 12px;
  font-family: var(--vp-font-family-mono);
}
.ct-proc.init {
  border-left: 3px solid var(--vp-c-brand-2);
}
.ct-pid {
  min-width: 18px;
  font-weight: 700;
  color: var(--vp-c-brand-1);
}
.ct-pname {
  color: var(--vp-c-text-1);
}
.ct-tag {
  margin-left: 6px;
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.ct-pmem {
  margin-left: auto;
  color: var(--vp-c-text-2);
}
.ct-gauge {
  height: 10px;
  background-color: var(--vp-c-bg-alt);
  border: 1px solid var(--vp-c-divider);
  overflow: hidden;
}
.ct-gauge-fill {
  height: 100%;
  background-color: var(--vp-c-green-2);
  transition: width 0.25s;
}
.ct-gauge-fill.hot {
  background-color: var(--vp-c-red-2);
}
.ct-oom {
  margin-top: 6px;
  padding: 4px 8px;
  border-left: 3px solid var(--vp-c-red-2);
  background-color: var(--vp-c-red-soft);
  font-size: 11px;
  color: var(--vp-c-red-1);
}
.ct-hostlabel {
  margin-top: 18px;
}
.ct-hostrow {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.ct-gpid {
  display: inline-flex;
  flex-direction: column;
  padding: 4px 8px;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg);
  font-size: 12px;
  font-weight: 700;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-1);
}
.ct-gpid.web {
  border-top: 2px solid var(--vp-c-brand-2);
}
.ct-gpid.db {
  border-top: 2px solid var(--vp-c-purple-2);
}
.ct-gpid-cont {
  font-size: 9px;
  font-weight: 400;
  color: var(--vp-c-text-3);
}
.ct-legend {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin-top: 14px;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
</style>
