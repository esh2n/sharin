<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// foundations/scheduler(Go)の M:N + work-stealing をブラウザで動かす移植版。
// 3 つの P があり goroutine は全部 P0 に積まれる(偏り)。空になった P が
// 忙しい P から半分を横取りして、各 P の実行量が均されていく様子を追う。

interface PSnap {
  id: number;
  local: string[];
  ran: number;
  steals: number;
}
type Ev =
  | { p: number; kind: "run"; g: string; done: boolean }
  | { p: number; kind: "steal"; from: number; n: number }
  | { p: number; kind: "global"; n: number }
  | { p: number; kind: "idle" };
interface Frame {
  round: number;
  ps: PSnap[];
  global: string[];
  note: string;
  events: Ev[];
  done: boolean;
}

const NP = 3;
const QUANTUM = 1;
const LOCAL_CAP = 6;
const WORKS = [4, 4, 4, 4, 4, 4]; // 6 本を P0 に(spill しない = 純粋に横取りで均す)

interface G {
  name: string;
  work: number;
}

function simulate(): Frame[] {
  const ps = [0, 1, 2].map((id) => ({ id, local: [] as G[], ran: 0, steals: 0 }));
  const global: G[] = [];
  // 全部 P0 に積む(偏り)。
  WORKS.forEach((w, i) => {
    const g: G = { name: String.fromCharCode(65 + i), work: w };
    const p = ps[0];
    if (p.local.length >= LOCAL_CAP) {
      const n = Math.floor(p.local.length / 2);
      global.push(...p.local.splice(0, n));
    }
    p.local.push(g);
  });

  const frames: Frame[] = [];
  const snap = (round: number, note: string, events: Ev[], done: boolean) =>
    frames.push({
      round,
      ps: ps.map((p) => ({ id: p.id, local: p.local.map((g) => g.name), ran: p.ran, steals: p.steals })),
      global: global.map((g) => g.name),
      note,
      events,
      done,
    });

  snap(0, "goroutine を全部 P0 に積んだ(偏り)。P1・P2 は空っぽ", [], false);

  const drained = () => global.length === 0 && ps.every((p) => p.local.length === 0);
  let round = 0;
  let guard = 0;
  while (!drained() && guard++ < 100) {
    round++;
    const events: Ev[] = [];
    for (const p of ps) {
      if (p.local.length === 0) {
        if (global.length > 0) {
          const n = Math.min(Math.floor(global.length / NP) + 1, global.length);
          p.local.push(...global.splice(0, n));
          events.push({ p: p.id, kind: "global", n });
        } else {
          let victim: (typeof ps)[number] | null = null;
          for (const q of ps) {
            if (q === p || q.local.length < 2) continue;
            if (!victim || q.local.length > victim.local.length) victim = q;
          }
          if (victim) {
            const n = Math.floor(victim.local.length / 2);
            const batch = victim.local.splice(0, n);
            p.local.push(...batch);
            p.steals++;
            events.push({ p: p.id, kind: "steal", from: victim.id, n });
          } else {
            events.push({ p: p.id, kind: "idle" });
            continue;
          }
        }
      }
      const g = p.local.shift() as G;
      const n = Math.min(QUANTUM, g.work);
      g.work -= n;
      p.ran += n;
      if (g.work === 0) {
        events.push({ p: p.id, kind: "run", g: g.name, done: true });
      } else {
        p.local.push(g);
        events.push({ p: p.id, kind: "run", g: g.name, done: false });
      }
    }
    snap(round, noteFor(events), events, false);
  }
  snap(round, "全 goroutine 完了。各 P の実行量がほぼ揃った = 偏った投入を負荷分散で均せた", [], true);
  return frames;
}

function noteFor(events: Ev[]): string {
  const steals = events.filter((e): e is Extract<Ev, { kind: "steal" }> => e.kind === "steal");
  if (steals.length) {
    return steals.map((s) => `P${s.p} が P${s.from} から ${s.n} 本横取り`).join(" / ");
  }
  const globals = events.filter((e) => e.kind === "global");
  if (globals.length) return globals.map((g) => `P${g.p} がグローバルから ${(g as any).n} 本取得`).join(" / ");
  return "各 P が手元の goroutine を1量子ずつ実行";
}

const frames = simulate();
const at = ref(0);
const cur = computed(() => frames[at.value]);
// 実行量バーの正規化: 最終フレームの最大 ran を基準にする(伸びて揃うのが見える)。
const maxRan = computed(() => Math.max(1, ...frames[frames.length - 1].ps.map((p) => p.ran)));

const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < frames.length - 1);
function first() {
  at.value = 0;
}
function prev() {
  if (canPrev.value) at.value--;
}
function next() {
  if (canNext.value) at.value++;
}
function last() {
  at.value = frames.length - 1;
}

const badge = computed(() => (cur.value.done ? "均衡達成" : `ラウンド ${cur.value.round}`));
const badgeTone = computed<"ok" | "neutral">(() => (cur.value.done ? "ok" : "neutral"));

// この P がこのラウンドで横取り/被横取りしたか(バッジ表示用)。
function stoleThisRound(pid: number): number | null {
  const e = cur.value.events.find((e) => e.kind === "steal" && e.p === pid) as
    | Extract<Ev, { kind: "steal" }>
    | undefined;
  return e ? e.from : null;
}
function ranG(pid: number): string | null {
  const e = cur.value.events.find((e) => e.kind === "run" && e.p === pid) as
    | Extract<Ev, { kind: "run" }>
    | undefined;
  return e ? e.g : null;
}
</script>

<template>
  <DemoShell title="scheduler(M:N + work-stealing)" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">1手すすめる</button>
      <span class="spacer" />
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="sc-count">{{ at + 1 }} / {{ frames.length }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <div class="sc-ps">
      <div v-for="p in cur.ps" :key="p.id" class="sc-p" :class="{ steal: stoleThisRound(p.id) !== null }">
        <div class="sc-p-head">
          <span class="sc-p-name">P{{ p.id }}</span>
          <span v-if="stoleThisRound(p.id) !== null" class="sc-tag steal">P{{ stoleThisRound(p.id) }} から横取り</span>
          <span v-else-if="ranG(p.id)" class="sc-tag run">{{ ranG(p.id) }} を実行</span>
          <span class="sc-p-steals">盗んだ回数 {{ p.steals }}</span>
        </div>

        <div class="sc-queue-label">ローカルキュー</div>
        <div class="sc-queue">
          <template v-if="p.local.length">
            <span v-for="(g, k) in p.local" :key="k" class="sc-g" :class="{ next: k === 0 }">{{ g }}</span>
          </template>
          <span v-else class="sc-empty">(空 — 盗みに行く)</span>
        </div>

        <div class="sc-ran">
          <div class="sc-ran-track">
            <div class="sc-ran-fill" :style="{ width: (p.ran / maxRan) * 100 + '%' }" />
          </div>
          <span class="sc-ran-val">実行量 {{ p.ran }}</span>
        </div>
      </div>
    </div>

    <div class="sc-global">
      <span class="sc-global-label">global queue</span>
      <div class="sc-queue">
        <template v-if="cur.global.length">
          <span v-for="(g, k) in cur.global" :key="k" class="sc-g glob">{{ g }}</span>
        </template>
        <span v-else class="sc-empty">(空)</span>
      </div>
    </div>

    <p class="sc-note">{{ cur.note }}</p>

    <div class="sc-legend">
      <span class="sc-lrow"><span class="sc-sw next" />各キューの先頭 = 次に走る goroutine</span>
      <span class="sc-lrow"><span class="sc-sw fill" />実行量バー: 偏った投入でも横取りで揃っていく</span>
    </div>
  </DemoShell>
</template>

<style scoped>
.sc-count {
  font-size: 12px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.sc-ps {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 10px;
  margin-top: 16px;
}
@media (max-width: 640px) {
  .sc-ps {
    grid-template-columns: 1fr;
  }
}
/* P パネル。角は落とし、横取り時だけ左アクセント(anti-slop 準拠) */
.sc-p {
  border: 1px solid var(--vp-c-divider);
  border-left: 3px solid transparent;
  border-radius: 0;
  padding: 10px 12px;
  background-color: var(--vp-c-bg);
}
.sc-p.steal {
  border-left-color: var(--vp-c-brand-1);
  background-color: var(--vp-c-brand-soft);
}
.sc-p-head {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 8px;
}
.sc-p-name {
  font-family: var(--vp-font-family-mono);
  font-weight: 700;
  font-size: 15px;
  color: var(--vp-c-text-1);
}
.sc-tag {
  font-size: 10px;
  font-weight: 700;
  padding: 1px 6px;
  border-radius: 4px;
}
.sc-tag.steal {
  background-color: var(--vp-c-brand-1);
  color: var(--vp-c-bg);
}
.sc-tag.run {
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-1);
  font-family: var(--vp-font-family-mono);
}
.sc-p-steals {
  margin-left: auto;
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.sc-queue-label,
.sc-global-label {
  font-size: 10px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-3);
  margin-bottom: 4px;
}
.sc-queue {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  min-height: 26px;
  align-items: center;
}
.sc-g {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 22px;
  height: 22px;
  padding: 0 6px;
  font-size: 12px;
  font-weight: 600;
  font-family: var(--vp-font-family-mono);
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-1);
}
.sc-g.next {
  background-color: var(--vp-c-brand-1);
  color: var(--vp-c-bg);
  border-color: transparent;
}
.sc-g.glob {
  background-color: var(--vp-c-bg-soft);
  color: var(--vp-c-text-2);
}
.sc-empty {
  font-size: 12px;
  color: var(--vp-c-text-3);
  font-style: italic;
}
.sc-ran {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 10px;
}
.sc-ran-track {
  flex: 1;
  height: 8px;
  background-color: var(--vp-c-default-soft);
  border-radius: 0;
  overflow: hidden;
}
.sc-ran-fill {
  height: 100%;
  background-color: var(--vp-c-brand-1);
  transition: width 0.25s;
}
.sc-ran-val {
  font-size: 11px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-2);
  flex: 0 0 auto;
}
.sc-global {
  margin-top: 12px;
  padding: 10px 12px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg);
}
.sc-note {
  margin: 14px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  font-family: var(--vp-font-family-mono);
}
.sc-legend {
  display: flex;
  flex-direction: column;
  gap: 5px;
  margin-top: 14px;
  padding-top: 12px;
  border-top: 1px solid var(--vp-c-divider);
}
.sc-lrow {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.sc-sw {
  flex: 0 0 auto;
  width: 16px;
  height: 16px;
  border-radius: 3px;
}
.sc-sw.next {
  background-color: var(--vp-c-brand-1);
}
.sc-sw.fill {
  background-color: var(--vp-c-brand-1);
  opacity: 0.5;
}
</style>
