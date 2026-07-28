<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/rbac(Go)を移植。既定が全拒否であること、役割を与えて
// 初めて通ること、広い許可が打ち消せないことを判定表で見せる。

type Verb = "get" | "list" | "create" | "delete";
const VERBS: Verb[] = ["get", "list", "create", "delete"];
const SUBJECTS = ["alice", "bob", "ci-bot"];
const RESOURCES = ["pods", "deployments", "secrets"];

interface Rule {
  resources: string[];
  verbs: Verb[] | ["*"];
}
interface Role {
  name: string;
  rules: Rule[];
  desc: string;
}

const ROLES: Role[] = [
  { name: "viewer", desc: "pods と deployments を見るだけ", rules: [{ resources: ["pods", "deployments"], verbs: ["get", "list"] }] },
  { name: "deployer", desc: "deployments を作り変えられる", rules: [{ resources: ["deployments"], verbs: ["get", "list", "create"] }] },
  { name: "secret-reader", desc: "secrets を読める", rules: [{ resources: ["secrets"], verbs: ["get"] }] },
  { name: "admin", desc: "すべての資源にすべての操作", rules: [{ resources: ["*"], verbs: ["*"] }] },
];

// subject → 与えた役割名
const bindings = ref<Record<string, string[]>>({ alice: [], bob: [], "ci-bot": [] });
const verb = ref<Verb>("get");
const probe = ref<{ s: string; r: string } | null>(null);

const matchList = (list: string[], v: string) => list.includes(v) || list.includes("*");

function can(subject: string, res: string, v: Verb) {
  for (const name of bindings.value[subject] ?? []) {
    const role = ROLES.find((x) => x.name === name);
    if (!role) continue;
    for (const rule of role.rules) {
      if (matchList(rule.resources, res) && matchList(rule.verbs as string[], v)) {
        return { allowed: true, role: name, reason: `${name} が ${res} への ${v} を許可している` };
      }
    }
  }
  return { allowed: false, role: "", reason: `${subject} に ${res} への ${v} を許す役割が無い` };
}

function toggle(subject: string, role: string) {
  const cur = bindings.value[subject] ?? [];
  bindings.value = {
    ...bindings.value,
    [subject]: cur.includes(role) ? cur.filter((r) => r !== role) : [...cur, role],
  };
}
function reset() {
  bindings.value = { alice: [], bob: [], "ci-bot": [] };
  probe.value = null;
}

const openCells = computed(() =>
  SUBJECTS.flatMap((s) => RESOURCES.map((r) => can(s, r, verb.value))).filter((d) => d.allowed).length,
);
const totalCells = SUBJECTS.length * RESOURCES.length;
const probeResult = computed(() => (probe.value ? can(probe.value.s, probe.value.r, verb.value) : null));
const anyBinding = computed(() => Object.values(bindings.value).some((v) => v.length > 0));
const badge = computed(() => `通る ${openCells.value} / ${totalCells}`);
const badgeTone = computed<"ok" | "ng" | "neutral">(() =>
  !anyBinding.value ? "ng" : openCells.value < totalCells ? "ok" : "neutral",
);
</script>

<template>
  <DemoShell title="RBAC" :badge="badge" :badge-tone="badgeTone">
    <div class="rb-grants">
      <div class="rb-h">役割を与える(クリックで付け外し)</div>
      <div v-for="s in SUBJECTS" :key="s" class="rb-subject">
        <span class="mono rb-subject-n">{{ s }}</span>
        <span class="rb-roles">
          <button
            v-for="r in ROLES"
            :key="r.name"
            class="rb-role mono"
            :class="{ on: bindings[s].includes(r.name), wide: r.name === 'admin' }"
            :title="r.desc"
            @click="toggle(s, r.name)"
          >{{ r.name }}</button>
        </span>
      </div>
      <button class="sd-btn rb-reset" @click="reset">すべて外す</button>
    </div>

    <div class="rb-row">
      <span class="rb-label">判定する操作</span>
      <span class="sd-seg">
        <span v-for="v in VERBS" :key="v" class="sd-seg-opt" :class="{ on: verb === v }" @click="verb = v">{{ v }}</span>
      </span>
    </div>

    <table class="rb-table">
      <thead>
        <tr>
          <th class="mono"></th>
          <th v-for="r in RESOURCES" :key="r" class="mono">{{ r }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="s in SUBJECTS" :key="s">
          <th class="mono">{{ s }}</th>
          <td v-for="r in RESOURCES" :key="r">
            <span class="rb-cell mono" :class="can(s, r, verb).allowed ? 'ok' : 'ng'" @click="probe = { s, r }">
              {{ can(s, r, verb).allowed ? "通る" : "拒否" }}
            </span>
          </td>
        </tr>
      </tbody>
    </table>

    <div class="rb-verdict" :class="!anyBinding ? 'bad' : 'ok'">
      <template v-if="probeResult">
        {{ probe!.s }} → {{ probe!.r }} の {{ verb }}: {{ probeResult.allowed ? "通る" : "拒否" }}。{{ probeResult.reason }}
      </template>
      <template v-else-if="!anyBinding">
        誰にも役割を与えていないので、何も通らない。これが既定で、通信とは向きが逆になっている
      </template>
      <template v-else>判定表のマスを押すと、そう判定された理由が出る</template>
    </div>

    <p class="rb-legend">
      最初は判定表が全部「拒否」になっている。通信は何も書かなければ全通しだったが、API は何も書かなければ
      何も通らない。役割を与えて初めてマスが開く。admin は資源も操作もワイルドカードなので、1つ与えるだけで
      その行が全部開く。そして拒否は書けないので、admin を持ったまま狭い役割を足しても打ち消せない。
      通らないものは禁止されているのではなく、どこにも書いていないだけになる。
    </p>
  </DemoShell>
</template>

<style scoped>
.rb-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 6px;
}
.rb-subject {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 5px;
  flex-wrap: wrap;
}
.rb-subject-n {
  font-size: 11.5px;
  font-weight: 700;
  min-width: 54px;
}
.rb-roles {
  display: flex;
  gap: 5px;
  flex-wrap: wrap;
}
.rb-role {
  font-size: 10.5px;
  padding: 3px 9px;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
  color: var(--vp-c-text-3);
  cursor: pointer;
}
.rb-role.on {
  border-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-brand-1);
  font-weight: 600;
}
.rb-role.wide.on {
  border-color: var(--vp-c-warning-1);
  background-color: var(--vp-c-warning-soft);
  color: var(--vp-c-warning-1);
}
.rb-reset {
  margin-top: 6px;
}
.rb-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-top: 14px;
  margin-bottom: 10px;
}
.rb-label {
  font-size: 11.5px;
  color: var(--vp-c-text-2);
}
.rb-table {
  width: 100%;
  border-collapse: collapse;
}
.rb-table th,
.rb-table td {
  border: 1px solid var(--vp-c-divider);
  padding: 5px 6px;
  text-align: center;
  font-size: 10.5px;
}
.rb-table thead th {
  color: var(--vp-c-text-2);
  font-weight: 700;
}
.rb-table tbody th {
  text-align: right;
  color: var(--vp-c-text-2);
  font-weight: 700;
}
.rb-cell {
  display: block;
  padding: 2px 0;
  cursor: pointer;
}
.rb-cell.ok {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.rb-cell.ng {
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
}
.rb-verdict {
  margin-top: 14px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
  background-color: var(--vp-c-bg-soft);
}
.rb-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.rb-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.rb-legend {
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
