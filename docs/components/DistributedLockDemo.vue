<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// distributed/dlock(Go)の「リース失効による二重取得 → フェンシングで防ぐ」を
// 決定的シナリオで。フェンシング有無を切り替えて、資源が守られる/破壊されるを比較。

interface Frame {
  clock: number;
  holder: string;
  token: number;
  expiry: number;
  data: string;
  maxToken: number;
  rejected: number;
  actor: string;
  action: string;
  accepted: boolean | null; // null = 書き込み以外
  note: string;
}

function simulate(fenced: boolean): Frame[] {
  let clock = 0;
  let holder = "";
  let expiry = 0;
  let token = 0;
  let nextToken = 1;
  const res = { data: "", maxToken: 0, rejected: 0 };
  const frames: Frame[] = [];

  const refresh = () => {
    if (holder !== "" && clock >= expiry) holder = "";
  };
  const acquire = (c: string, lease: number): number => {
    refresh();
    if (holder !== "") return 0;
    holder = c;
    expiry = clock + lease;
    token = nextToken;
    nextToken++;
    return token;
  };
  const write = (tok: number, val: string): boolean => {
    if (fenced && tok < res.maxToken) {
      res.rejected++;
      return false;
    }
    if (tok > res.maxToken) res.maxToken = tok;
    res.data = val;
    return true;
  };
  const snap = (actor: string, action: string, accepted: boolean | null, note: string) => {
    refresh();
    frames.push({
      clock,
      holder,
      token: holder ? token : 0,
      expiry,
      data: res.data,
      maxToken: res.maxToken,
      rejected: res.rejected,
      actor,
      action,
      accepted,
      note,
    });
  };

  snap("", "初期状態", null, "まだ誰もロックを持っていない。資源 row-42 は空");
  const t1 = acquire("A", 10);
  snap("A", `Acquire → token ${t1}`, null, `A がロック取得。フェンシングトークン ${t1} が発行され、リースは t=${expiry} まで`);
  const w1 = write(1, "A-value");
  snap("A", 'Write(token 1, "A-value")', w1, 'A が token 1 で資源に書き込み → "A-value"、maxToken=1');
  clock += 10;
  snap("A", "GC 停止 → リース失効", null, "A が GC で一時停止。その間に時間が進み、A のリースが失効する(A は気づかない)");
  const t2 = acquire("B", 10);
  snap("B", `Acquire → token ${t2}`, null, `失効したので B がロックを取得。token ${t2} が発行される(二重取得の芽)`);
  const w2 = write(2, "B-value");
  snap("B", 'Write(token 2, "B-value")', w2, 'B が token 2 で書き込み → "B-value"、maxToken=2');
  const w3 = write(1, "A-STALE");
  snap(
    "A",
    'Write(token 1, "A-STALE")',
    w3,
    fenced
      ? "A が再開し、古い token 1 で書き込み → 1 < 2 でフェンス落ち(拒否)。資源は守られた"
      : "フェンシング無し → A の出遅れた書き込みが素通りし、B の値を破壊してしまう",
  );
  return frames;
}

const fenced = ref(true);
const frames = computed(() => simulate(fenced.value));
const at = ref(0);
const cur = computed(() => frames.value[at.value]);

function setFenced(v: boolean) {
  fenced.value = v;
  at.value = 0;
}
const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < frames.value.length - 1);
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
  at.value = frames.value.length - 1;
}

const done = computed(() => at.value === frames.value.length - 1);
const badge = computed(() => {
  if (!done.value) return `step ${at.value + 1}`;
  return fenced.value ? "資源は守られた" : "資源が破壊された";
});
const badgeTone = computed<"ok" | "ng" | "neutral">(() => {
  if (!done.value) return "neutral";
  return fenced.value ? "ok" : "ng";
});
const writeState = computed<"ok" | "rejected" | null>(() => {
  if (cur.value.accepted === null) return null;
  return cur.value.accepted ? "ok" : "rejected";
});
</script>

<template>
  <DemoShell title="distributed-lock(リース + フェンシング)" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: fenced }" @click="setFenced(true)">フェンシング 有</span>
        <span class="sd-seg-opt" :class="{ on: !fenced }" @click="setFenced(false)">無</span>
      </span>
      <span class="spacer" />
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">1手すすめる</button>
    </div>

    <div class="dl-grid">
      <div class="dl-panel">
        <div class="dl-panel-head">ロックサービス</div>
        <div class="dl-body">
          <div class="dl-kv">
            <span class="dl-k">持ち主</span>
            <span class="dl-v">{{ cur.holder || "(空き)" }}</span>
          </div>
          <div class="dl-kv">
            <span class="dl-k">発行トークン</span>
            <span class="dl-v mono">{{ cur.token || "—" }}</span>
          </div>
          <div class="dl-kv">
            <span class="dl-k">リース</span>
            <span class="dl-v mono">失効 t={{ cur.expiry }} / 現在 t={{ cur.clock }}</span>
          </div>
        </div>
      </div>

      <div class="dl-panel">
        <div class="dl-panel-head">
          資源 row-42
          <span class="dl-fence" :class="{ off: !fenced }">{{ fenced ? "フェンシング有" : "フェンシング無" }}</span>
        </div>
        <div class="dl-body">
          <div class="dl-data">{{ cur.data || "(空)" }}</div>
          <div class="dl-kv">
            <span class="dl-k">maxToken</span>
            <span class="dl-v mono">{{ cur.maxToken }}</span>
          </div>
          <div class="dl-kv">
            <span class="dl-k">拒否した書き込み</span>
            <span class="dl-v mono" :class="{ warn: cur.rejected > 0 }">{{ cur.rejected }}</span>
          </div>
        </div>
      </div>
    </div>

    <div class="dl-action" :class="writeState">
      <span class="dl-actor" v-if="cur.actor">{{ cur.actor }}</span>
      <span class="dl-act">{{ cur.action }}</span>
      <span v-if="writeState === 'ok'" class="dl-res ok">受理</span>
      <span v-else-if="writeState === 'rejected'" class="dl-res ng">フェンス落ち(拒否)</span>
    </div>

    <p class="dl-note">{{ cur.note }}</p>

    <div class="dl-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="dl-count">{{ at + 1 }} / {{ frames.length }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="dl-legend">
      リースは持ち主のクラッシュに耐えるが、GC 停止で「持っているつもり」の古い持ち主を生む。
      資源側が<strong>トークンを検査</strong>してはじめて、出遅れた書き込みを弾いて破壊を防げる。
    </p>
  </DemoShell>
</template>

<style scoped>
.dl-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-top: 16px;
}
@media (max-width: 560px) {
  .dl-grid {
    grid-template-columns: 1fr;
  }
}
.dl-panel {
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg);
  overflow: hidden;
}
.dl-panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  padding: 8px 12px;
  border-bottom: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.dl-fence {
  font-size: 10px;
  font-weight: 700;
  padding: 1px 6px;
  border-radius: 3px;
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.dl-fence.off {
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
}
.dl-body {
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.dl-kv {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 10px;
  font-size: 12px;
}
.dl-k {
  color: var(--vp-c-text-3);
}
.dl-v {
  color: var(--vp-c-text-1);
}
.dl-v.mono {
  font-family: var(--vp-font-family-mono);
}
.dl-v.warn {
  color: var(--vp-c-danger-1);
  font-weight: 700;
}
.dl-data {
  font-family: var(--vp-font-family-mono);
  font-size: 18px;
  font-weight: 700;
  color: var(--vp-c-text-1);
  padding: 6px 10px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  background-color: var(--vp-c-bg-soft);
  text-align: center;
}
.dl-action {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 12px;
  padding: 10px 12px;
  border: 1px solid var(--vp-c-divider);
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  background-color: var(--vp-c-bg);
  font-size: 13px;
}
.dl-action.rejected {
  border-left-color: var(--vp-c-danger-1);
}
.dl-action.ok {
  border-left-color: var(--vp-c-green-1);
}
.dl-actor {
  font-family: var(--vp-font-family-mono);
  font-weight: 700;
  color: var(--vp-c-text-1);
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  padding: 1px 8px;
}
.dl-act {
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-2);
  flex: 1;
}
.dl-res {
  font-size: 11px;
  font-weight: 700;
  padding: 2px 8px;
  border-radius: 3px;
}
.dl-res.ok {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.dl-res.ng {
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
}
.dl-note {
  margin: 14px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  font-family: var(--vp-font-family-mono);
}
.dl-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
}
.dl-count {
  font-size: 12px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.dl-legend {
  margin: 14px 0 0;
  padding-top: 12px;
  border-top: 1px solid var(--vp-c-divider);
  font-size: 12px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
}
</style>
