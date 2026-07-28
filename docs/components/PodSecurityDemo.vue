<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/podsecurity(Go)を移植。同じ検査を段階と扱いで振り分ける。

type Level = 0 | 1 | 2; // privileged / baseline / restricted
const LEVEL_NAMES = ["privileged", "baseline", "restricted"];

interface Sec {
  privileged?: boolean;
  allowPrivilegeEscalation?: boolean | null;
  runAsNonRoot?: boolean | null;
  capDrop?: string[];
  capAdd?: string[];
  seccomp?: string;
}
interface Pod {
  name: string;
  what: string;
  hostNetwork?: boolean;
  hostPID?: boolean;
  hostPaths?: string[];
  sec: Sec;
}
interface Violation {
  rule: string;
  level: Level;
  detail: string;
}

const PODS: Pod[] = [
  { name: "web", what: "権限のことを何も書いていない", sec: {} },
  {
    name: "agent",
    what: "ログ収集。隔離を外している",
    hostNetwork: true,
    hostPID: true,
    hostPaths: ["/var/log"],
    sec: { privileged: true, capAdd: ["SYS_ADMIN"], seccomp: "Unconfined" },
  },
  {
    name: "hardened",
    what: "安全側を書き足した",
    sec: {
      allowPrivilegeEscalation: false,
      runAsNonRoot: true,
      capDrop: ["ALL"],
      seccomp: "RuntimeDefault",
    },
  },
];

function check(p: Pod, level: Level): Violation[] {
  if (level === 0) return [];
  const vs: Violation[] = [];
  if (p.hostNetwork) vs.push({ rule: "hostNetwork", level: 1, detail: "ホストのネットワークをそのまま使っている" });
  if (p.hostPID) vs.push({ rule: "hostPID", level: 1, detail: "ホストのプロセスが見える" });
  if (p.sec.privileged) vs.push({ rule: "privileged", level: 1, detail: "特権つき。ホストの root とほぼ変わらない" });
  for (const path of p.hostPaths || []) {
    vs.push({ rule: "hostPath", level: 1, detail: `ホストの ${path} をそのまま見ている` });
  }
  for (const c of p.sec.capAdd || []) {
    if (c !== "NET_BIND_SERVICE") vs.push({ rule: "capabilities.add", level: 1, detail: `${c} を足している` });
  }
  if (p.sec.seccomp === "Unconfined") {
    vs.push({ rule: "seccompProfile", level: 1, detail: "システムコールの制限を外している" });
  }

  if (level === 2) {
    const s = p.sec;
    if (s.allowPrivilegeEscalation !== false) {
      vs.push({ rule: "allowPrivilegeEscalation", level: 2, detail: "false と明示していない" });
    }
    if (s.runAsNonRoot !== true) {
      vs.push({ rule: "runAsNonRoot", level: 2, detail: "true と明示していない。既定では root で動く" });
    }
    if (!(s.capDrop || []).includes("ALL")) {
      vs.push({ rule: "capabilities.drop", level: 2, detail: "ALL を外していない" });
    }
    if (s.seccomp !== "RuntimeDefault" && s.seccomp !== "Localhost") {
      vs.push({ rule: "seccompProfile", level: 2, detail: "RuntimeDefault を明示していない" });
    }
  }
  return vs.sort((a, b) => (a.rule < b.rule ? -1 : 1));
}

const enforce = ref<Level>(1);
const warn = ref<Level>(2);
const selected = ref("web");

const rows = computed(() =>
  PODS.map((p) => {
    const denied = check(p, enforce.value);
    const warned = check(p, warn.value);
    return { pod: p, denied, warned, admitted: denied.length === 0 };
  }),
);
const current = computed(() => rows.value.find((r) => r.pod.name === selected.value)!);
const breaks = computed(() => rows.value.filter((r) => !r.admitted).map((r) => r.pod.name));
const badge = computed(() => `拒否 ${LEVEL_NAMES[enforce.value]} で ${breaks.value.length} / ${PODS.length} が落ちる`);
const badgeTone = computed<"ok" | "ng">(() => (breaks.value.length > 0 ? "ng" : "ok"));

const verdict = computed(() => {
  if (enforce.value === 0) return "拒否が privileged なので何も止まらない。既定はこれで、書き忘れた Pod がいちばん緩く動く";
  if (enforce.value === 2) return `拒否を restricted まで上げると ${breaks.value.join("、")} が作れなくなる。上げる前に、警告だけ先に上げて直しておく`;
  const warnedOnly = rows.value.filter((r) => r.admitted && r.warned.length > 0).map((r) => r.pod.name);
  if (warnedOnly.length > 0) {
    return `${warnedOnly.join("、")} は通るが、restricted には届いていないと伝わる。止めずに何を直せばよいかだけを知らせている状態`;
  }
  return "拒否の段階に全員が届いている。ここから1段上げられる";
});
</script>

<template>
  <DemoShell title="Pod Security Standards" :badge="badge" :badge-tone="badgeTone">
    <div class="ps-actions">
      <span class="ps-lab mono">拒否(enforce)</span>
      <button
        v-for="(n, i) in LEVEL_NAMES"
        :key="'e' + i"
        class="sd-btn"
        :class="enforce === i ? 'sd-btn--primary' : ''"
        @click="enforce = i as Level"
      >
        {{ n }}
      </button>
      <span class="ps-gap" />
      <span class="ps-lab mono">警告(warn)</span>
      <button
        v-for="(n, i) in LEVEL_NAMES"
        :key="'w' + i"
        class="sd-btn"
        :class="warn === i ? 'sd-btn--primary' : ''"
        @click="warn = i as Level"
      >
        {{ n }}
      </button>
    </div>

    <div class="ps-rows">
      <div
        v-for="r in rows"
        :key="r.pod.name"
        class="ps-row"
        :class="[r.admitted ? 'ok' : 'ng', selected === r.pod.name ? 'sel' : '']"
        @click="selected = r.pod.name"
      >
        <span class="ps-name mono">{{ r.pod.name }}<em>{{ r.pod.what }}</em></span>
        <span class="ps-state">{{ r.admitted ? "通る" : "拒否" }}</span>
        <span class="ps-counts mono">
          違反 {{ r.denied.length }}
          <template v-if="r.admitted && r.warned.length">・警告 {{ r.warned.length }}</template>
        </span>
      </div>
    </div>

    <div class="ps-detail">
      <div class="ps-detail-h mono">
        {{ current.pod.name }} ・ 拒否 {{ LEVEL_NAMES[enforce] }} / 警告 {{ LEVEL_NAMES[warn] }}
      </div>
      <div v-if="current.denied.length" class="ps-list">
        <div v-for="v in current.denied" :key="'d' + v.rule" class="ps-item deny mono">
          <span class="ps-rule">{{ v.rule }}</span><span>{{ v.detail }}</span>
        </div>
      </div>
      <div v-else-if="current.warned.length" class="ps-list">
        <div v-for="v in current.warned" :key="'w' + v.rule" class="ps-item warn mono">
          <span class="ps-rule">{{ v.rule }}</span><span>{{ v.detail }}</span>
        </div>
      </div>
      <div v-else class="ps-clean mono">この段階では何も出ない</div>
    </div>

    <div class="ps-verdict" :class="badgeTone === 'ng' ? 'bad' : 'ok'">{{ verdict }}</div>

    <p class="ps-legend">
      行をクリックすると、その Pod の違反が下に出る。baseline は「隔離を外していないか」を見るので、
      何も書いていない web は通り、ホストを覗く agent が落ちる。restricted は「安全側を書いたか」を見るので、
      向きが逆になって web も落ちる。拒否を baseline、警告を restricted にすると、
      止めずに何を直せばよいかだけが伝わる状態になる。
    </p>
  </DemoShell>
</template>

<style scoped>
.ps-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.ps-lab {
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.ps-gap {
  flex: 1;
  min-width: 10px;
}
.ps-rows {
  margin-top: 14px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.ps-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 7px 11px;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
  cursor: pointer;
}
.ps-row.sel {
  border-color: var(--vp-c-brand-1);
}
.ps-name {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  font-size: 11.5px;
  color: var(--vp-c-text-1);
  line-height: 1.4;
}
.ps-name em {
  font-style: normal;
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.ps-state {
  flex: none;
  font-size: 11px;
  font-weight: 700;
}
.ps-row.ok .ps-state {
  color: var(--vp-c-green-1);
}
.ps-row.ng .ps-state {
  color: var(--vp-c-danger-1);
}
.ps-counts {
  flex: none;
  width: 108px;
  text-align: right;
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.ps-detail {
  margin-top: 10px;
  border: 1px solid var(--vp-c-divider);
  padding: 8px 12px;
  background-color: var(--vp-c-bg-soft);
  min-height: 66px;
}
.ps-detail-h {
  font-size: 10px;
  color: var(--vp-c-text-3);
  margin-bottom: 5px;
}
.ps-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.ps-item {
  display: flex;
  gap: 10px;
  font-size: 10.5px;
  padding: 2px 0;
}
.ps-rule {
  width: 172px;
  flex: none;
}
.ps-item.deny .ps-rule {
  color: var(--vp-c-danger-1);
}
.ps-item.warn .ps-rule {
  color: var(--vp-c-warning-1);
}
.ps-item span:last-child {
  color: var(--vp-c-text-2);
}
.ps-clean {
  font-size: 10.5px;
  color: var(--vp-c-green-1);
}
.ps-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  font-weight: 600;
  line-height: 1.6;
  background-color: var(--vp-c-bg-soft);
}
.ps-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.ps-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.ps-legend {
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
