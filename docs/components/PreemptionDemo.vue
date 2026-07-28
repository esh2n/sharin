<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/preemption(Go)を移植。優先度が「奪う権利」として働くとき、
// 誰が何個犠牲になるかを見る。保護の有無で犠牲が入れ替わることと、
// 全部外してから戻すと犠牲が最小になることの2つを扱う。

interface Pod {
  name: string;
  app: string;
  priority: number;
  req: number;
}
interface Node {
  name: string;
  cap: number;
  pods: Pod[];
}
interface Eviction {
  victim: string;
  by: string;
  node: string;
  violates: boolean;
}
interface TraceStep {
  name: string;
  priority: number;
  req: number;
  freeAfter: number;
  kept: boolean;
}

function freeOf(n: Node, drop: Record<string, boolean>): number {
  let f = n.cap;
  for (const p of n.pods) if (!drop[p.name]) f -= p.req;
  return f;
}

function countApp(nodes: Node[], app: string): number {
  let k = 0;
  for (const n of nodes) for (const p of n.pods) if (p.app === app) k++;
  return k;
}

function violations(nodes: Node[], budgets: Record<string, number>, victims: Pod[]): number {
  const killed: Record<string, number> = {};
  for (const v of victims) killed[v.app] = (killed[v.app] || 0) + 1;
  let total = 0;
  for (const app of Object.keys(killed).sort()) {
    const min = budgets[app];
    if (min === undefined) continue;
    const leftOver = countApp(nodes, app) - killed[app];
    if (leftOver < min) total += min - leftOver;
  }
  return total;
}

// selectVictims は Go の同名関数の移植。戻す過程も記録して見せる。
function selectVictims(n: Node, p: Pod): { victims: Pod[]; ok: boolean; trace: TraceStep[] } {
  const drop: Record<string, boolean> = {};
  const lower: Pod[] = [];
  for (const q of n.pods) {
    if (q.priority < p.priority) {
      lower.push(q);
      drop[q.name] = true;
    }
  }
  if (p.req > freeOf(n, drop)) return { victims: [], ok: false, trace: [] };

  lower.sort((a, b) => (a.priority !== b.priority ? b.priority - a.priority : a.name < b.name ? -1 : 1));

  const victims: Pod[] = [];
  const trace: TraceStep[] = [];
  for (const q of lower) {
    delete drop[q.name];
    const free = freeOf(n, drop);
    if (p.req <= free) {
      trace.push({ name: q.name, priority: q.priority, req: q.req, freeAfter: free, kept: true });
      continue;
    }
    drop[q.name] = true;
    victims.push(q);
    trace.push({ name: q.name, priority: q.priority, req: q.req, freeAfter: free, kept: false });
  }
  return { victims, ok: true, trace };
}

interface RunResult {
  before: Node[];
  after: Node[];
  evictions: Eviction[];
  pending: string[];
  log: string[];
  trace: TraceStep[];
  target: Pod;
}

function clone(nodes: Node[]): Node[] {
  return nodes.map((n) => ({ name: n.name, cap: n.cap, pods: n.pods.map((p) => ({ ...p })) }));
}

function run(seed: Node[], budgets: Record<string, number>, submit: Pod[]): RunResult {
  const nodes = clone(seed);
  const queue: { pod: Pod; seq: number }[] = submit.map((p, i) => ({ pod: p, seq: i }));
  let seq = submit.length;
  const evictions: Eviction[] = [];
  const pending: string[] = [];
  const log: string[] = [];
  let trace: TraceStep[] = [];

  for (let round = 0; round < 64 && queue.length > 0; round++) {
    queue.sort((a, b) => (a.pod.priority !== b.pod.priority ? b.pod.priority - a.pod.priority : a.seq - b.seq));
    const q = queue.shift()!;
    const p = q.pod;

    // 奪わずに置ける場所を探す。空きの大きいほう、同点は名前順。
    let best: Node | null = null;
    for (const n of nodes) {
      if (p.req > freeOf(n, {})) continue;
      if (!best || freeOf(n, {}) > freeOf(best, {}) || (freeOf(n, {}) === freeOf(best, {}) && n.name < best.name)) {
        best = n;
      }
    }
    if (best) {
      best.pods.push(p);
      log.push(`${p.name}(優先度 ${p.priority})を ${best.name} に置いた`);
      continue;
    }

    // 奪いにいく。候補ノードごとに犠牲を計算し、いちばん小さいものを選ぶ。
    let pick: { node: Node; victims: Pod[]; violations: number; top: number; trace: TraceStep[] } | null = null;
    for (const n of nodes) {
      const sel = selectVictims(n, p);
      if (!sel.ok || sel.victims.length === 0) continue;
      const cand = {
        node: n,
        victims: sel.victims,
        violations: violations(nodes, budgets, sel.victims),
        top: Math.max(...sel.victims.map((v) => v.priority)),
        trace: sel.trace,
      };
      const wins =
        !pick ||
        cand.violations < pick.violations ||
        (cand.violations === pick.violations &&
          (cand.top < pick.top ||
            (cand.top === pick.top &&
              (cand.victims.length < pick.victims.length ||
                (cand.victims.length === pick.victims.length && cand.node.name < pick.node.name)))));
      if (wins) pick = cand;
    }
    if (!pick) {
      pending.push(p.name);
      log.push(`${p.name} はどこにも置けない(奪える相手も居ない)`);
      continue;
    }
    if (trace.length === 0) trace = pick.trace;

    for (const v of pick.victims) {
      const violates = violations(nodes, budgets, [v]) > 0;
      pick.node.pods = pick.node.pods.filter((x) => x.name !== v.name);
      evictions.push({ victim: v.name, by: p.name, node: pick.node.name, violates });
      log.push(
        `${p.name}(優先度 ${p.priority})が ${pick.node.name} から ${v.name}(優先度 ${v.priority})を追い出した` +
          (violates ? "。保護を破っている" : ""),
      );
      queue.push({ pod: v, seq: seq++ });
    }
    pick.node.pods.push(p);
    log.push(`${p.name} を ${pick.node.name} に置いた(奪って作った場所)`);
  }
  for (const q of queue) pending.push(q.pod.name);
  return { before: clone(seed), after: nodes, evictions, pending: pending.sort(), log, trace, target: submit[0] };
}

const CASCADE_NODES: Node[] = [
  { name: "node-a", cap: 1000, pods: [{ name: "batch", app: "batch", priority: 50, req: 800 }] },
  { name: "node-b", cap: 1000, pods: [{ name: "log", app: "log", priority: 10, req: 800 }] },
];
const CASCADE_SUBMIT: Pod[] = [{ name: "api", app: "api", priority: 100, req: 900 }];

const MINIMIZE_NODES: Node[] = [
  {
    name: "node-a",
    cap: 1000,
    pods: [
      { name: "small-a", app: "batch", priority: 10, req: 100 },
      { name: "small-b", app: "batch", priority: 10, req: 100 },
      { name: "big", app: "batch", priority: 20, req: 600 },
    ],
  },
];
const MINIMIZE_SUBMIT: Pod[] = [{ name: "api", app: "api", priority: 100, req: 700 }];

const scenario = ref<"cascade" | "minimize">("cascade");
const guarded = ref(true);

const result = computed<RunResult>(() => {
  if (scenario.value === "minimize") return run(MINIMIZE_NODES, {}, MINIMIZE_SUBMIT);
  return run(CASCADE_NODES, guarded.value ? { log: 1 } : {}, CASCADE_SUBMIT);
});

const evictedNames = computed(() => new Set(result.value.evictions.map((e) => e.victim)));
const badge = computed(() => `追い出し ${result.value.evictions.length} 件`);
const badgeTone = computed<"ok" | "ng">(() => (result.value.evictions.length > 1 ? "ng" : "ok"));

function tier(p: Pod): string {
  if (p.priority >= 100) return "hi";
  if (p.priority >= 50) return "mid";
  return "lo";
}
function pct(v: number, cap: number): string {
  return `${(v / cap) * 100}%`;
}
const verdict = computed(() => {
  const r = result.value;
  if (scenario.value === "minimize") {
    const kept = r.trace.filter((t) => t.kept).length;
    return `追い出したのは ${r.evictions.length} 個だけ。優先度の低い順に外していれば ${r.trace.length} 個すべてが止まっていた。戻す向きで確かめたので ${kept} 個が助かった`;
  }
  if (!guarded.value) {
    return "保護が無いので、いちばん優先度の低い log が1回で犠牲になる。玉突きは起きない";
  }
  return "保護のある log を避けて、優先度の高い batch が犠牲になった。行き場を失った batch は node-b へ回り、そこでは選択肢が無いので保護を破って log を追い出す";
});
</script>

<template>
  <DemoShell title="PriorityClass と preemption" :badge="badge" :badge-tone="badgeTone">
    <div class="pe-actions">
      <button
        class="sd-btn"
        :class="scenario === 'cascade' ? 'sd-btn--primary' : ''"
        @click="scenario = 'cascade'"
      >
        保護と玉突き
      </button>
      <button
        class="sd-btn"
        :class="scenario === 'minimize' ? 'sd-btn--primary' : ''"
        @click="scenario = 'minimize'"
      >
        犠牲の最小化
      </button>
      <span class="pe-spacer" />
      <button v-if="scenario === 'cascade'" class="sd-btn" @click="guarded = !guarded">
        log に保護をかける: {{ guarded ? "かける" : "かけない" }}
      </button>
    </div>

    <p class="pe-brief mono">
      <template v-if="scenario === 'cascade'">
        node-a に batch(優先度 50、要求 800)、node-b に log(優先度 10、要求 800)。
        そこへ api(優先度 100、要求 900)が来る
      </template>
      <template v-else>
        node-a に small-a(優先度 10、要求 100)、small-b(優先度 10、要求 100)、big(優先度 20、要求 600)が載っている。
        そこへ api(優先度 100、要求 700)が来る
      </template>
    </p>

    <div class="pe-grid">
      <div class="pe-side">
        <div class="pe-side-h">来る前</div>
        <div v-for="n in result.before" :key="n.name" class="pe-node">
          <div class="pe-node-h mono">
            {{ n.name }}<span>容量 {{ n.cap }}</span>
          </div>
          <div class="pe-bar">
            <span
              v-for="p in n.pods"
              :key="p.name"
              class="pe-seg"
              :class="['t-' + tier(p), evictedNames.has(p.name) ? 'doomed' : '']"
              :style="{ width: pct(p.req, n.cap) }"
            >
              <em class="mono">{{ p.name }}</em>
            </span>
          </div>
          <div class="pe-meta mono">
            <span v-for="p in n.pods" :key="p.name">{{ p.name }} 優先度 {{ p.priority }} ・ 要求 {{ p.req }}</span>
          </div>
        </div>
      </div>

      <div class="pe-side">
        <div class="pe-side-h">落ち着いた後</div>
        <div v-for="n in result.after" :key="n.name" class="pe-node">
          <div class="pe-node-h mono">
            {{ n.name }}<span>容量 {{ n.cap }}</span>
          </div>
          <div class="pe-bar">
            <span
              v-for="p in n.pods"
              :key="p.name"
              class="pe-seg"
              :class="'t-' + tier(p)"
              :style="{ width: pct(p.req, n.cap) }"
            >
              <em class="mono">{{ p.name }}</em>
            </span>
          </div>
          <div class="pe-meta mono">
            <span v-for="p in n.pods" :key="p.name">{{ p.name }} 優先度 {{ p.priority }} ・ 要求 {{ p.req }}</span>
          </div>
        </div>
        <div v-if="result.pending.length" class="pe-pending mono">
          押し出されて置き場所を失った: {{ result.pending.join(", ") }}
        </div>
      </div>
    </div>

    <div v-if="scenario === 'minimize' && result.trace.length" class="pe-trace">
      <div class="pe-trace-h">全部外してから、優先度の高い順に戻す</div>
      <div v-for="(t, i) in result.trace" :key="t.name" class="pe-trace-line mono" :class="t.kept ? 'kept' : 'cut'">
        {{ i + 1 }}. {{ t.name }}(要求 {{ t.req }})を戻す → 空き {{ t.freeAfter }}
        <strong>{{ t.kept ? "まだ api が入る。助かる" : "api が入らなくなる。これが犠牲" }}</strong>
      </div>
    </div>

    <div v-if="result.evictions.length" class="pe-chain">
      <div class="pe-chain-h">追い出しの連鎖</div>
      <div v-for="(e, i) in result.evictions" :key="i" class="pe-chain-line mono" :class="e.violates ? 'bad' : ''">
        {{ i + 1 }} 段目 ・ {{ e.by }} が {{ e.node }} から {{ e.victim }} を追い出した
        <span v-if="e.violates">(保護を破っている)</span>
      </div>
    </div>

    <div class="pe-verdict" :class="badgeTone === 'ng' ? 'bad' : 'ok'">{{ verdict }}</div>

    <p class="pe-legend">
      帯の幅が要求の大きさ、色が優先度の高さ。左が api の来る前、右が落ち着いた後で、左側の赤い枠が
      これから追い出されるものを表す。「保護と玉突き」では、保護をかけると犠牲が log から batch へ入れ替わり、
      押し出された batch が今度は log を追い出す。保護は log を守りきれないが、一度は逸らしている。
      「犠牲の最小化」では、全部外してから戻す過程が1段ずつ見える。戻しても api が入るものは、
      止める必要が無かったものになる。
    </p>
  </DemoShell>
</template>

<style scoped>
.pe-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.pe-spacer {
  flex: 1;
}
.pe-brief {
  margin: 12px 0 0;
  font-size: 11px;
  line-height: 1.7;
  color: var(--vp-c-text-3);
}
.pe-grid {
  display: flex;
  gap: 10px;
  margin-top: 10px;
  flex-wrap: wrap;
}
.pe-side {
  flex: 1;
  min-width: 260px;
}
.pe-side-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 6px;
}
.pe-node {
  border: 1px solid var(--vp-c-divider);
  padding: 8px 10px;
  margin-bottom: 8px;
  background-color: var(--vp-c-bg-soft);
}
.pe-node-h {
  display: flex;
  justify-content: space-between;
  font-size: 10.5px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 5px;
}
.pe-node-h span {
  font-weight: 400;
  color: var(--vp-c-text-3);
}
.pe-bar {
  display: flex;
  height: 22px;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-alt);
  overflow: hidden;
}
.pe-seg {
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  border-right: 1px solid var(--vp-c-bg);
}
.pe-seg em {
  font-style: normal;
  font-size: 9px;
  white-space: nowrap;
}
.t-hi {
  background-color: var(--vp-c-brand-soft);
  color: var(--vp-c-brand-1);
}
.t-mid {
  background-color: var(--vp-c-warning-soft);
  color: var(--vp-c-warning-1);
}
.t-lo {
  background-color: var(--vp-c-bg-alt);
  color: var(--vp-c-text-3);
}
.pe-seg.doomed {
  outline: 2px solid var(--vp-c-danger-1);
  outline-offset: -2px;
}
.pe-meta {
  display: flex;
  flex-direction: column;
  gap: 1px;
  margin-top: 5px;
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.pe-pending {
  border: 1px dashed var(--vp-c-danger-1);
  padding: 6px 10px;
  font-size: 10px;
  color: var(--vp-c-danger-1);
}
.pe-trace,
.pe-chain {
  margin-top: 10px;
  border: 1px solid var(--vp-c-divider);
  padding: 8px 12px;
  background-color: var(--vp-c-bg-soft);
}
.pe-trace-h,
.pe-chain-h {
  font-size: 11px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  margin-bottom: 4px;
}
.pe-trace-line,
.pe-chain-line {
  font-size: 10.5px;
  color: var(--vp-c-text-2);
  padding: 2px 0;
}
.pe-trace-line strong {
  font-weight: 600;
  margin-left: 6px;
}
.pe-trace-line.kept strong {
  color: var(--vp-c-green-1);
}
.pe-trace-line.cut strong {
  color: var(--vp-c-danger-1);
}
.pe-chain-line.bad {
  color: var(--vp-c-danger-1);
}
.pe-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  font-weight: 600;
  background-color: var(--vp-c-bg-soft);
}
.pe-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.pe-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.pe-legend {
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
