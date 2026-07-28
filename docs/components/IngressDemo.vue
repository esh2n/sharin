<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/ingress(Go)を移植。規則が特定度で並び替わることと、
// 同じリクエストが規則の有無で別の行き先になることを見せる。

interface Rule {
  host: string;
  path: string;
  service: string;
  port: number;
}

const CATALOG: Rule[] = [
  { host: "", path: "/", service: "catchall", port: 80 },
  { host: "shop.example", path: "/", service: "web", port: 80 },
  { host: "shop.example", path: "/api", service: "api", port: 8080 },
  { host: "shop.example", path: "/api/v2", service: "api-v2", port: 8080 },
  { host: "blog.example", path: "/", service: "blog", port: 80 },
];

const enabled = ref<boolean[]>([false, true, true, false, true]);
const reqHost = ref("shop.example");
const reqPath = ref("/api/v2/items");

const HOSTS = ["shop.example", "blog.example", "other.example"];
const PATHS = ["/", "/api", "/api/v2/items", "/static/logo.png"];

// 特定度の高い順に並べる。ホスト指定が先、次にパスの長い順。
const rules = computed(() =>
  CATALOG.filter((_, i) => enabled.value[i]).sort((a, b) => {
    const ha = a.host ? 1 : 0;
    const hb = b.host ? 1 : 0;
    if (ha !== hb) return hb - ha;
    if (a.path.length !== b.path.length) return b.path.length - a.path.length;
    return a.service < b.service ? -1 : 1;
  }),
);

function match(r: Rule, host: string, path: string) {
  if (r.host !== "" && r.host !== host) return false;
  return path.startsWith(r.path);
}

const result = computed(() => {
  for (const r of rules.value) {
    if (match(r, reqHost.value, reqPath.value)) {
      return { matched: true, rule: r, reason: `ホスト ${r.host || "*"} / パス ${r.path} の規則に一致` };
    }
  }
  return { matched: false, rule: null as Rule | null, reason: "当たる規則がない" };
});

// どの規則が「当たるが、より特定的なものに負けた」かを出す。
const shadowed = computed(() =>
  rules.value.filter((r) => match(r, reqHost.value, reqPath.value) && r !== result.value.rule),
);

function toggle(i: number) {
  const next = [...enabled.value];
  next[i] = !next[i];
  enabled.value = next;
}
const label = (r: Rule) => `${r.host || "*"}${r.path} → ${r.service}:${r.port}`;
const badge = computed(() =>
  result.value.matched ? `${result.value.rule!.service}:${result.value.rule!.port}` : "見つからない",
);
const badgeTone = computed<"ok" | "ng">(() => (result.value.matched ? "ok" : "ng"));
</script>

<template>
  <DemoShell title="Ingress" :badge="badge" :badge-tone="badgeTone">
    <div class="ig-req">
      <div class="ig-req-h">リクエスト</div>
      <div class="ig-row">
        <span class="ig-label">ホスト</span>
        <span class="sd-seg">
          <span v-for="h in HOSTS" :key="h" class="sd-seg-opt" :class="{ on: reqHost === h }" @click="reqHost = h">{{ h }}</span>
        </span>
      </div>
      <div class="ig-row">
        <span class="ig-label">パス</span>
        <span class="sd-seg">
          <span v-for="p in PATHS" :key="p" class="sd-seg-opt" :class="{ on: reqPath === p }" @click="reqPath = p">{{ p }}</span>
        </span>
      </div>
      <div class="ig-url mono">{{ reqHost }}{{ reqPath }}</div>
    </div>

    <div class="ig-rules">
      <div class="ig-rules-h">規則(クリックで有効・無効。並びは特定度の高い順で、書いた順ではない)</div>
      <div class="ig-list">
        <button
          v-for="(r, i) in CATALOG"
          :key="i"
          class="ig-toggle mono"
          :class="{ on: enabled[i] }"
          @click="toggle(i)"
        >{{ label(r) }}</button>
      </div>

      <div class="ig-order">
        <div v-for="(r, n) in rules" :key="n" class="ig-ordered mono" :class="r === result.rule ? 'hit' : shadowed.includes(r) ? 'shadow' : ''">
          <span class="ig-n">{{ n + 1 }}</span>
          <span class="ig-lbl">{{ label(r) }}</span>
          <span class="ig-tag">
            {{ r === result.rule ? "ここに当たった" : shadowed.includes(r) ? "当たるがより特定的なものに負けた" : "" }}
          </span>
        </div>
        <div v-if="rules.length === 0" class="ig-empty">(有効な規則がない)</div>
      </div>
    </div>

    <div class="ig-verdict" :class="result.matched ? 'ok' : 'bad'">
      <template v-if="result.matched">
        {{ reqHost }}{{ reqPath }} → {{ result.rule!.service }}:{{ result.rule!.port }}。{{ result.reason }}
      </template>
      <template v-else>{{ reqHost }}{{ reqPath }} に当たる規則がない。既定も無いので見つからない</template>
    </div>

    <p class="ig-legend">
      規則の一覧は、有効にした順ではなく特定度の高い順に並んでいる。ホストを指定した規則が先に、同じなら
      パスの長い規則が先に来る。だから /api/v2 を有効にすると、同じリクエストの行き先が /api から api-v2 へ移る。
      負けた規則には「当たるがより特定的なものに負けた」と出るので、なぜその行き先になったかが追える。
      いちばん上の `*/` は全部に当たる規則で、有効にすると見つからないケースが消える代わりに、
      意図しないホストまで受けてしまう。
    </p>
  </DemoShell>
</template>

<style scoped>
.ig-req-h,
.ig-rules-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 6px;
}
.ig-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 6px;
}
.ig-label {
  font-size: 11.5px;
  color: var(--vp-c-text-2);
  min-width: 44px;
}
.ig-url {
  font-size: 13px;
  font-weight: 700;
  color: var(--vp-c-brand-1);
  margin-top: 4px;
}
.ig-rules {
  margin-top: 16px;
}
.ig-list {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}
.ig-toggle {
  font-size: 10.5px;
  padding: 4px 9px;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
  color: var(--vp-c-text-3);
  cursor: pointer;
}
.ig-toggle.on {
  border-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-brand-1);
  font-weight: 600;
}
.ig-order {
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
  padding: 8px 10px;
}
.ig-ordered {
  display: flex;
  align-items: baseline;
  gap: 8px;
  font-size: 10.5px;
  padding: 3px 6px;
  color: var(--vp-c-text-2);
  border-left: 3px solid transparent;
}
.ig-ordered.hit {
  border-left-color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
  font-weight: 700;
}
.ig-ordered.shadow {
  border-left-color: var(--vp-c-warning-1);
  color: var(--vp-c-warning-1);
}
.ig-n {
  min-width: 14px;
  color: var(--vp-c-text-3);
}
.ig-lbl {
  min-width: 210px;
}
.ig-tag {
  font-size: 9.5px;
  opacity: 0.9;
}
.ig-empty {
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.ig-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
  background-color: var(--vp-c-bg-soft);
}
.ig-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.ig-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.ig-legend {
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
