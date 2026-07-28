<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/networkpolicy(Go)を移植。方針を足すと既定が反転すること、
// 守られるのは受け側であることを、判定表で一望させる。

const PODS = ["web", "api", "db", "batch"];
const PORT = 0; // すべてのポート

interface Rule {
  from: string;
}
interface Policy {
  name: string;
  selector: string;
  rules: Rule[];
}

// 用意した方針。有効にすると効く。
const CATALOG: Policy[] = [
  { name: "api-allow-web", selector: "api", rules: [{ from: "web" }] },
  { name: "db-allow-api", selector: "db", rules: [{ from: "api" }] },
  { name: "db-allow-batch", selector: "db", rules: [{ from: "batch" }] },
  { name: "db-deny-all", selector: "db", rules: [] },
];

const enabled = ref<Record<string, boolean>>({
  "api-allow-web": false,
  "db-allow-api": false,
  "db-allow-batch": false,
  "db-deny-all": false,
});
const probe = ref<{ from: string; to: string } | null>(null);

const active = computed(() => CATALOG.filter((p) => enabled.value[p.name]));

// dst に向けられた方針が1つでもあるか。あれば既定は拒否に変わる。
function protectedBy(dst: string): Policy[] {
  return active.value.filter((p) => p.selector === dst);
}

function decide(src: string, dst: string): { allowed: boolean; policy: string; reason: string } {
  const pols = protectedBy(dst);
  if (pols.length === 0) {
    return { allowed: true, policy: "", reason: `${dst} に向けられた方針が無いので既定で通る` };
  }
  for (const pol of pols) {
    for (const r of pol.rules) {
      if (r.from === src) {
        return { allowed: true, policy: pol.name, reason: `${pol.name} が ${src} からの接続を許可している` };
      }
    }
  }
  return { allowed: false, policy: "", reason: `${dst} は方針で守られていて、${src} を許可する規則が無い` };
}

const matrix = computed(() =>
  PODS.flatMap((src) =>
    PODS.filter((dst) => dst !== src).map((dst) => ({ src, dst, ...decide(src, dst) })),
  ),
);
const cell = (src: string, dst: string) => matrix.value.find((e) => e.src === src && e.dst === dst);
const openCount = computed(() => matrix.value.filter((e) => e.allowed).length);

function toggle(name: string) {
  enabled.value = { ...enabled.value, [name]: !enabled.value[name] };
}
function reset() {
  enabled.value = {
    "api-allow-web": false,
    "db-allow-api": false,
    "db-allow-batch": false,
    "db-deny-all": false,
  };
  probe.value = null;
}

const probeResult = computed(() => (probe.value ? decide(probe.value.from, probe.value.to) : null));
const badge = computed(() => `通る経路 ${openCount.value} / ${matrix.value.length}`);
const badgeTone = computed<"ok" | "ng" | "neutral">(() =>
  active.value.length === 0 ? "ng" : openCount.value < matrix.value.length ? "ok" : "neutral",
);
</script>

<template>
  <DemoShell title="NetworkPolicy" :badge="badge" :badge-tone="badgeTone">
    <div class="np-policies">
      <div class="np-policies-h">方針(クリックで有効・無効)</div>
      <div class="np-list">
        <button
          v-for="p in CATALOG"
          :key="p.name"
          class="np-policy mono"
          :class="{ on: enabled[p.name] }"
          @click="toggle(p.name)"
        >
          <span class="np-policy-n">{{ p.name }}</span>
          <span class="np-policy-d">
            {{ p.selector }} を守る ・ {{ p.rules.length ? `${p.rules.map((r) => r.from).join(", ")} から許可` : "何も許可しない" }}
          </span>
        </button>
      </div>
      <button class="sd-btn np-reset" @click="reset">すべて無効に戻す</button>
    </div>

    <div class="np-matrix">
      <div class="np-matrix-h">判定表(行=送り元、列=宛先)</div>
      <table class="np-table">
        <thead>
          <tr>
            <th class="mono"></th>
            <th v-for="dst in PODS" :key="dst" class="mono">
              → {{ dst }}
              <small v-if="protectedBy(dst).length">守られている</small>
              <small v-else class="np-open">既定のまま</small>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="src in PODS" :key="src">
            <th class="mono">{{ src }} →</th>
            <td v-for="dst in PODS" :key="dst">
              <span v-if="src === dst" class="np-self">—</span>
              <span
                v-else
                class="np-cell mono"
                :class="cell(src, dst)!.allowed ? 'ok' : 'ng'"
                @click="probe = { from: src, to: dst }"
              >{{ cell(src, dst)!.allowed ? "通る" : "遮断" }}</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="np-verdict" :class="active.length === 0 ? 'bad' : 'ok'">
      <template v-if="probeResult">
        {{ probe!.from }} → {{ probe!.to }}: {{ probeResult.allowed ? "通る" : "遮断" }}。{{ probeResult.reason }}
      </template>
      <template v-else-if="active.length === 0">
        方針が1つも無いので、全部が全部に繋がる。web が乗っ取られたら db まで直行できる
      </template>
      <template v-else>
        方針が {{ active.length }} 件有効。判定表のマスを押すと、そう判定された理由が出る
      </template>
    </div>

    <p class="np-legend">
      最初は方針が無く、判定表は全部「通る」になっている。これが既定で、そうでないと何も動かないからそうなっている。
      db-allow-api を有効にすると、db の列だけが閉じて api からの1本だけが残る。許可を1つ書いた瞬間に、
      書かなかった分がすべて閉じている。方針は受け側に付くので、db に方針を向けても db から出ていく通信は縛られない。
      db-deny-all は規則を1つも持たない方針で、これを足しても他の許可は消えない。許可は足し算で、後から塞ぐことはできない。
    </p>
  </DemoShell>
</template>

<style scoped>
.np-policies-h,
.np-matrix-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 6px;
}
.np-list {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
.np-policy {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  padding: 6px 10px;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
  cursor: pointer;
  text-align: left;
}
.np-policy.on {
  border-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-default-soft);
}
.np-policy-n {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-1);
}
.np-policy.on .np-policy-n {
  color: var(--vp-c-brand-1);
}
.np-policy-d {
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.np-reset {
  margin-top: 8px;
}
.np-matrix {
  margin-top: 16px;
}
.np-table {
  width: 100%;
  border-collapse: collapse;
}
.np-table th,
.np-table td {
  border: 1px solid var(--vp-c-divider);
  padding: 5px 6px;
  text-align: center;
  font-size: 10.5px;
}
.np-table thead th {
  color: var(--vp-c-text-2);
  font-weight: 700;
}
.np-table thead th small {
  display: block;
  font-size: 8.5px;
  font-weight: 400;
  color: var(--vp-c-danger-1);
}
.np-table thead th small.np-open {
  color: var(--vp-c-text-3);
}
.np-table tbody th {
  text-align: right;
  color: var(--vp-c-text-2);
  font-weight: 700;
}
.np-cell {
  display: block;
  padding: 2px 0;
  cursor: pointer;
}
.np-cell.ok {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.np-cell.ng {
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
}
.np-self {
  color: var(--vp-c-text-3);
}
.np-verdict {
  margin-top: 14px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
  background-color: var(--vp-c-bg-soft);
}
.np-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.np-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.np-legend {
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
