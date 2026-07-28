<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/gatewayapi(Go)を移植。指名と受け入れの両方が揃って
// はじめて繋がることと、勝つ規則が特定度で決まることを見る。

interface Backend {
  service: string;
  port: number;
  weight?: number;
}
interface Match {
  pathType?: "Exact" | "PathPrefix";
  path: string;
  headers?: Record<string, string>;
}
interface Rule {
  matches: Match[];
  backends: Backend[];
}
interface Route {
  name: string;
  ns: string;
  parentRefs: string[];
  hostnames: string[];
  rules: Rule[];
}
interface Prio {
  exactHost: number;
  exactPath: number;
  pathLen: number;
  headerHits: number;
}

const GATEWAY = { name: "public", ns: "infra", hostname: "shop.example" };

const ROUTES: Route[] = [
  {
    name: "web",
    ns: "team-a",
    parentRefs: ["public"],
    hostnames: ["shop.example"],
    rules: [
      { matches: [{ pathType: "PathPrefix", path: "/" }], backends: [{ service: "web", port: 80 }] },
      {
        matches: [{ pathType: "PathPrefix", path: "/api" }],
        backends: [
          { service: "api-stable", port: 8080, weight: 90 },
          { service: "api-canary", port: 8080, weight: 10 },
        ],
      },
      {
        matches: [{ path: "/", headers: { "x-internal": "yes" } }],
        backends: [{ service: "web-internal", port: 80 }],
      },
    ],
  },
  {
    name: "admin",
    ns: "team-b",
    parentRefs: ["public"],
    hostnames: ["shop.example"],
    rules: [
      {
        matches: [{ pathType: "PathPrefix", path: "/api/admin" }],
        backends: [{ service: "admin", port: 9000 }],
      },
    ],
  },
];

const allowTeamB = ref(false);
const allowedFrom = computed(() => (allowTeamB.value ? ["team-a", "team-b"] : ["team-a"]));

function attached(r: Route): { ok: boolean; why: string } {
  if (!r.parentRefs.includes(GATEWAY.name)) {
    return { ok: false, why: "この Gateway を親に指名していない" };
  }
  if (!allowedFrom.value.includes(r.ns)) {
    return { ok: false, why: `Listener が名前空間 ${r.ns} を受け入れていない` };
  }
  if (!r.hostnames.includes(GATEWAY.hostname)) {
    return { ok: false, why: `ホスト名が ${GATEWAY.hostname} と重ならない` };
  }
  return { ok: true, why: "" };
}

function beats(p: Prio, q: Prio): boolean {
  for (const [a, b] of [
    [p.exactHost, q.exactHost],
    [p.exactPath, q.exactPath],
    [p.pathLen, q.pathLen],
    [p.headerHits, q.headerHits],
  ]) {
    if (a !== b) return a > b;
  }
  return false;
}

function prefixOf(prefix: string, path: string): boolean {
  if (prefix === "" || prefix === "/") return true;
  if (path.length < prefix.length || path.slice(0, prefix.length) !== prefix) return false;
  return path.length === prefix.length || path[prefix.length] === "/";
}

interface Req {
  path: string;
  headers: Record<string, string>;
}

function matchOne(m: Match, r: Route, req: Req): Prio | null {
  const p: Prio = { exactHost: 0, exactPath: 0, pathLen: 0, headerHits: 0 };
  if (m.pathType === "Exact") {
    if (m.path !== req.path) return null;
    p.exactPath = 1;
  } else if (!prefixOf(m.path, req.path)) {
    return null;
  }
  p.pathLen = m.path.length;
  for (const [k, v] of Object.entries(m.headers || {})) {
    if (req.headers[k] !== v) return null;
    p.headerHits++;
  }
  if (r.hostnames.includes(GATEWAY.hostname)) p.exactHost = 1;
  return p;
}

const hits = ref<Record<string, number>>({});

function pick(key: string, backends: Backend[]): Backend {
  if (backends.length === 1) return backends[0];
  const sorted = [...backends].sort((a, b) => (a.service < b.service ? -1 : 1));
  const total = sorted.reduce((s, b) => s + (b.weight || 0), 0);
  if (total <= 0) return sorted[0];
  const n = (hits.value[key] || 0) % total;
  hits.value = { ...hits.value, [key]: (hits.value[key] || 0) + 1 };
  let acc = 0;
  for (const b of sorted.slice(0, -1)) {
    acc += b.weight || 0;
    if (n < acc) return b;
  }
  return sorted[sorted.length - 1];
}

interface Outcome {
  found: boolean;
  route: string;
  backend: Backend | null;
  prio: Prio | null;
  skipped: { route: string; why: string }[];
}

function route(req: Req, consume: boolean): Outcome {
  let best: { route: string; rule: Rule; prio: Prio } | null = null;
  const skipped: { route: string; why: string }[] = [];
  for (const r of ROUTES) {
    const a = attached(r);
    if (!a.ok) {
      for (const rule of r.rules) {
        if (rule.matches.some((m) => matchOne(m, r, req))) {
          skipped.push({ route: `${r.ns}/${r.name}`, why: a.why });
          break;
        }
      }
      continue;
    }
    for (const rule of r.rules) {
      let rp: Prio | null = null;
      for (const m of rule.matches) {
        const p = matchOne(m, r, req);
        if (p && (!rp || beats(p, rp))) rp = p;
      }
      if (!rp) continue;
      const key = `${r.ns}/${r.name}`;
      if (best === null || beats(rp, best.prio) || (!beats(best.prio, rp) && key < best.route)) {
        best = { route: key, rule, prio: rp };
      }
    }
  }
  if (!best) return { found: false, route: "", backend: null, prio: null, skipped };
  const backend = consume
    ? pick(best.route + best.rule.matches[0].path, best.rule.backends)
    : best.rule.backends[0];
  return { found: true, route: best.route, backend, prio: best.prio, skipped };
}

const REQUESTS: { label: string; req: Req }[] = [
  { label: "GET /", req: { path: "/", headers: {} } },
  { label: "GET / (x-internal: yes)", req: { path: "/", headers: { "x-internal": "yes" } } },
  { label: "GET /api/users", req: { path: "/api/users", headers: {} } },
  { label: "GET /api/admin/reset", req: { path: "/api/admin/reset", headers: {} } },
];

const table = computed(() => REQUESTS.map((r) => ({ ...r, out: route(r.req, false) })));
const attachments = computed(() => ROUTES.map((r) => ({ key: `${r.ns}/${r.name}`, ...attached(r) })));

// 重み付き分岐の実測。押すたびに 1 件投げて数える。
const tally = ref<Record<string, number>>({});
const sent = ref(0);
function send(n: number) {
  const next = { ...tally.value };
  for (let i = 0; i < n; i++) {
    const out = route({ path: "/api/users", headers: {} }, true);
    const name = out.backend ? out.backend.service : "(なし)";
    next[name] = (next[name] || 0) + 1;
  }
  tally.value = next;
  sent.value += n;
}
function resetTally() {
  tally.value = {};
  sent.value = 0;
  hits.value = {};
}
send(100);

const badge = computed(() => `繋がった Route ${attachments.value.filter((a) => a.ok).length} / ${ROUTES.length}`);
const badgeTone = computed<"ok" | "ng">(() => (allowTeamB.value ? "ng" : "ok"));
</script>

<template>
  <DemoShell title="Gateway API" :badge="badge" :badge-tone="badgeTone">
    <div class="ga-actions">
      <button class="sd-btn" :class="allowTeamB ? 'sd-btn--primary' : ''" @click="allowTeamB = !allowTeamB">
        Listener が team-b を受け入れる: {{ allowTeamB ? "受け入れる" : "受け入れない" }}
      </button>
      <span class="ga-spacer" />
      <span class="ga-hint mono">Gateway public(infra) ・ hostname shop.example ・ allowedRoutes {{ allowedFrom.join(", ") }}</span>
    </div>

    <div class="ga-attach">
      <div class="ga-h">繋がり</div>
      <div v-for="a in attachments" :key="a.key" class="ga-att mono" :class="a.ok ? 'on' : 'off'">
        <span class="ga-att-name">{{ a.key }}</span>
        <span>{{ a.ok ? "指名と受け入れが揃った。繋がっている" : a.why }}</span>
      </div>
    </div>

    <div class="ga-table">
      <div class="ga-h">振り分け</div>
      <div class="ga-row ga-head mono">
        <span>リクエスト</span><span>勝った Route</span><span>振り分け先</span><span>勝った目盛り</span>
      </div>
      <div v-for="t in table" :key="t.label" class="ga-row mono" :class="t.out.skipped.length ? 'note' : ''">
        <span>{{ t.label }}</span>
        <span>{{ t.out.found ? t.out.route : "(当たらない)" }}</span>
        <span class="ga-be">{{ t.out.backend ? t.out.backend.service : "-" }}</span>
        <span class="ga-prio">
          <template v-if="t.out.prio">
            path {{ t.out.prio.pathLen }}{{ t.out.prio.exactPath ? " (Exact)" : "" }}
            <template v-if="t.out.prio.headerHits">・header {{ t.out.prio.headerHits }}</template>
          </template>
        </span>
      </div>
      <div v-for="t in table.filter((x) => x.out.skipped.length)" :key="t.label + '-s'" class="ga-skip mono">
        {{ t.label }}: {{ t.out.skipped[0].route }} も一致したが使われない({{ t.out.skipped[0].why }})
      </div>
    </div>

    <div class="ga-weight">
      <div class="ga-h">重み付き分岐(/api/users を投げた実測)</div>
      <div class="ga-wrow">
        <button class="sd-btn" @click="send(100)">100 件投げる</button>
        <button class="sd-btn" @click="resetTally">数え直す</button>
        <span class="ga-hint mono">stable 90 : canary 10 と宣言している ・ 合計 {{ sent }} 件</span>
      </div>
      <div v-for="(n, name) in tally" :key="name" class="ga-bar-row mono">
        <span class="ga-bar-name">{{ name }}</span>
        <span class="ga-bar"><span class="ga-fill" :style="{ width: (n / Math.max(sent, 1)) * 100 + '%' }" /></span>
        <span class="ga-bar-n">{{ n }} 件</span>
      </div>
    </div>

    <div class="ga-verdict" :class="allowTeamB ? 'bad' : 'ok'">
      <template v-if="allowTeamB">
        team-b を受け入れたので admin の規則が有効になり、/api/admin/reset の行き先が変わった。
        受け入れは運用側の宣言で、Route 側の指名だけでは起きない
      </template>
      <template v-else>
        team-b の Route は Gateway を指名しているが、Listener が受け入れていないので繋がらない。
        /api/admin/reset により長く一致していても、繋がっていなければ使われない
      </template>
    </div>

    <p class="ga-legend">
      1つの Gateway に2つのチームの Route を繋ごうとしている。team-a は受け入れられていて、
      team-b は既定では受け入れられていない。切り替えると、同じ Route が繋がったり繋がらなくなったりする。
      振り分けの表は、勝った規則とその目盛りを並べたもの。パスの長さで決まる行と、
      ヘッダの一致で決まる行がある。いちばん下は重み付き分岐の実測で、乱数を使っていないので
      宣言した比率がそのまま出る。
    </p>
  </DemoShell>
</template>

<style scoped>
.ga-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.ga-spacer {
  flex: 1;
}
.ga-hint {
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.ga-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 5px;
}
.ga-attach,
.ga-table,
.ga-weight {
  margin-top: 12px;
  border: 1px solid var(--vp-c-divider);
  padding: 9px 12px;
  background-color: var(--vp-c-bg-soft);
}
.ga-att {
  display: flex;
  gap: 10px;
  font-size: 10.5px;
  padding: 3px 8px;
  margin-bottom: 3px;
  border-left: 3px solid var(--vp-c-divider);
}
.ga-att-name {
  width: 110px;
  flex: none;
}
.ga-att.on {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.ga-att.off {
  border-left-color: var(--vp-c-text-3);
  color: var(--vp-c-text-3);
  background-color: var(--vp-c-bg-alt);
}
.ga-row {
  display: grid;
  grid-template-columns: 1.5fr 1fr 1fr 1.1fr;
  gap: 8px;
  font-size: 10.5px;
  padding: 3px 0;
  color: var(--vp-c-text-2);
  border-bottom: 1px solid var(--vp-c-divider);
}
.ga-head {
  font-weight: 700;
  color: var(--vp-c-text-3);
  font-size: 9.5px;
}
.ga-be {
  color: var(--vp-c-brand-1);
}
.ga-prio {
  color: var(--vp-c-text-3);
  font-size: 9.5px;
}
.ga-skip {
  margin-top: 5px;
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.ga-wrow {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 8px;
}
.ga-bar-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 10px;
  margin-bottom: 3px;
}
.ga-bar-name {
  width: 92px;
  flex: none;
  color: var(--vp-c-text-2);
}
.ga-bar {
  flex: 1;
  height: 12px;
  background-color: var(--vp-c-bg-alt);
  border: 1px solid var(--vp-c-divider);
}
.ga-fill {
  display: block;
  height: 100%;
  background-color: var(--vp-c-brand-1);
}
.ga-bar-n {
  width: 52px;
  flex: none;
  text-align: right;
  color: var(--vp-c-text-3);
}
.ga-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  font-weight: 600;
  line-height: 1.6;
  background-color: var(--vp-c-bg-soft);
}
.ga-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.ga-verdict.bad {
  border-left-color: var(--vp-c-warning-1);
  color: var(--vp-c-warning-1);
  background-color: var(--vp-c-warning-soft);
}
.ga-legend {
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
