<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// chain/substrate(Go)の考え方をブラウザに移植。
// extrinsic: pallet.method を dispatch、weight 予算を消費、上限超えは弾く。
// upgrade: v1 に無い staking.bond → set_code で v2 へ → 状態そのままに新機能が使える。

interface Row {
  k: string;
  v: number;
}
interface Frame {
  spec: number;
  pallets: string[];
  storage: Row[];
  weightUsed: number;
  weightLimit: number;
  action: string;
  result: "ok" | "bad" | "upgrade" | null;
  note: string;
}

function extrinsicFrames(): Frame[] {
  const p = ["system", "balances"];
  return [
    {
      spec: 1,
      pallets: p,
      storage: [{ k: "balances:alice", v: 1000 }, { k: "balances:bob", v: 0 }],
      weightUsed: 0,
      weightLimit: 250,
      action: "初期状態(genesis)",
      result: null,
      note: "ランタイムは system と balances の 2 pallet の合成(FRAME)。ブロックが使える weight は 250 まで。extrinsic を順に dispatch して状態を進める",
    },
    {
      spec: 1,
      pallets: p,
      storage: [{ k: "balances:alice", v: 700 }, { k: "balances:bob", v: 300 }],
      weightUsed: 100,
      weightLimit: 250,
      action: "balances.transfer alice→bob 300",
      result: "ok",
      note: "balances pallet の transfer を dispatch。weight 100 を消費してストレージを書き換える。残り 150",
    },
    {
      spec: 1,
      pallets: p,
      storage: [{ k: "balances:alice", v: 500 }, { k: "balances:bob", v: 500 }],
      weightUsed: 200,
      weightLimit: 250,
      action: "balances.transfer alice→bob 200",
      result: "ok",
      note: "もう 100 消費して 200/250。あと 50 しか残っていない",
    },
    {
      spec: 1,
      pallets: p,
      storage: [{ k: "balances:alice", v: 500 }, { k: "balances:bob", v: 500 }],
      weightUsed: 200,
      weightLimit: 250,
      action: "balances.transfer alice→bob 50(weight 100)",
      result: "bad",
      note: "この transfer も weight 100 が要るが、残りは 50。上限を超えるのでブロックに入らず、状態は進まない。weight は「1 ブロックがこなせる仕事量」の予算で、gas を一般化したもの",
    },
  ];
}

function upgradeFrames(): Frame[] {
  return [
    {
      spec: 1,
      pallets: ["system", "balances"],
      storage: [{ k: "balances:alice", v: 1000 }, { k: "balances:bob", v: 0 }],
      weightUsed: 0,
      weightLimit: 10000,
      action: "初期状態(ランタイム v1)",
      result: null,
      note: "v1 のランタイムには system と balances しかない。staking はまだ存在しない",
    },
    {
      spec: 1,
      pallets: ["system", "balances"],
      storage: [{ k: "balances:alice", v: 1000 }, { k: "balances:bob", v: 0 }],
      weightUsed: 0,
      weightLimit: 10000,
      action: "staking.bond alice 300",
      result: "bad",
      note: "v1 に staking pallet は無いので dispatch できない(ErrUnknownPallet)。この機能はまだチェーンに載っていない",
    },
    {
      spec: 2,
      pallets: ["system", "balances", "staking"],
      storage: [{ k: "balances:alice", v: 1000 }, { k: "balances:bob", v: 0 }],
      weightUsed: 200,
      weightLimit: 10000,
      action: "system.set_code(v2)",
      result: "upgrade",
      note: "取引 1 本でランタイムを v2 に差し替える。ノードの更新もハードフォークも要らない(forkless upgrade)。spec_version が 1→2 に上がり、staking pallet が加わる。ストレージ(残高)は一切触れず引き継がれる",
    },
    {
      spec: 2,
      pallets: ["system", "balances", "staking"],
      storage: [{ k: "balances:alice", v: 700 }, { k: "balances:bob", v: 0 }, { k: "staking:alice", v: 300 }],
      weightUsed: 350,
      weightLimit: 10000,
      action: "staking.bond alice 300",
      result: "ok",
      note: "同じ alice の残高(アップグレードをまたいで保たれた 1000)から 300 を staking へ。さっき弾かれた呼び出しが、フォークせずに使えるようになった",
    },
  ];
}

const mode = ref<"extrinsic" | "upgrade">("extrinsic");
const frames = computed(() => (mode.value === "extrinsic" ? extrinsicFrames() : upgradeFrames()));
const at = ref(0);
const cur = computed(() => frames.value[at.value]);

function setMode(m: "extrinsic" | "upgrade") {
  mode.value = m;
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
  return mode.value === "extrinsic" ? "weight 上限で弾かれた" : "フォークせず機能追加";
});
const badgeTone = computed<"ok" | "ng" | "neutral">(() => (done.value ? "ok" : "neutral"));

function weightPct(f: Frame): number {
  return f.weightLimit === 0 ? 0 : Math.min(100, (f.weightUsed / f.weightLimit) * 100);
}
</script>

<template>
  <DemoShell title="substrate(ランタイムと forkless upgrade)" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: mode === 'extrinsic' }" @click="setMode('extrinsic')">extrinsic + weight</span>
        <span class="sd-seg-opt" :class="{ on: mode === 'upgrade' }" @click="setMode('upgrade')">forkless upgrade</span>
      </span>
      <span class="spacer" />
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">1手すすめる</button>
    </div>

    <div class="su-grid">
      <div class="su-panel">
        <div class="su-panel-head">
          ランタイム
          <span class="su-spec mono">spec v{{ cur.spec }}</span>
        </div>
        <div class="su-body">
          <div class="su-pallets">
            <span v-for="p in cur.pallets" :key="p" class="su-pill" :class="{ neo: p === 'staking' }">{{ p }}</span>
          </div>
          <div class="su-weight">
            <div class="su-weight-head">
              <span>block weight</span>
              <span class="mono">{{ cur.weightUsed }} / {{ cur.weightLimit }}</span>
            </div>
            <div class="su-bar">
              <div class="su-bar-fill" :class="{ full: cur.result === 'bad' }" :style="{ width: weightPct(cur) + '%' }" />
            </div>
          </div>
        </div>
      </div>

      <div class="su-panel">
        <div class="su-panel-head">ストレージ(状態トライ)</div>
        <div class="su-body">
          <div v-for="r in cur.storage" :key="r.k" class="su-kv">
            <span class="su-k mono">{{ r.k }}</span>
            <span class="su-v mono">{{ r.v }}</span>
          </div>
        </div>
      </div>
    </div>

    <div class="su-action" :class="cur.result">
      <span class="su-act mono">{{ cur.action }}</span>
      <span v-if="cur.result === 'ok'" class="su-res ok">dispatch 成功</span>
      <span v-else-if="cur.result === 'bad'" class="su-res ng">弾かれた</span>
      <span v-else-if="cur.result === 'upgrade'" class="su-res up">ランタイム差し替え</span>
    </div>
    <p class="su-note">{{ cur.note }}</p>

    <div class="su-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="su-count mono">{{ at + 1 }} / {{ frames.length }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="su-legend">
      ランタイムは pallet の合成でできた状態遷移関数で、extrinsic を dispatch して状態を進める。
      1 ブロックの仕事量は weight で予算化する。ランタイム自体をチェーン上の差し替え可能なコードとして持つので、
      機能追加や規則変更は取引 1 本で済み、ノードの更新もハードフォークも要らない。
    </p>
  </DemoShell>
</template>

<style scoped>
.su-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-top: 16px;
}
@media (max-width: 560px) {
  .su-grid {
    grid-template-columns: 1fr;
  }
}
.su-panel {
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg);
  overflow: hidden;
}
.su-panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  padding: 8px 12px;
  border-bottom: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.su-spec {
  font-size: 11px;
  color: var(--vp-c-brand-1);
}
.su-body {
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-height: 108px;
}
.su-pallets {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.su-pill {
  font-size: 12px;
  font-family: var(--vp-font-family-mono);
  padding: 3px 9px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  background-color: var(--vp-c-bg-soft);
  color: var(--vp-c-text-2);
}
.su-pill.neo {
  border-color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.su-weight-head {
  display: flex;
  justify-content: space-between;
  font-size: 11px;
  color: var(--vp-c-text-3);
  margin-bottom: 4px;
}
.su-bar {
  height: 12px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
  overflow: hidden;
}
.su-bar-fill {
  height: 100%;
  background-color: var(--vp-c-brand-soft);
  transition: width 0.25s ease;
}
.su-bar-fill.full {
  background-color: var(--vp-c-danger-soft);
}
.su-kv {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  font-size: 13px;
}
.su-k {
  color: var(--vp-c-text-3);
}
.su-v {
  color: var(--vp-c-text-1);
  font-weight: 600;
}
.su-action {
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
.su-action.ok {
  border-left-color: var(--vp-c-green-1);
}
.su-action.bad {
  border-left-color: var(--vp-c-danger-1);
}
.su-action.upgrade {
  border-left-color: var(--vp-c-yellow-1, #d0a215);
}
.su-act {
  flex: 1;
  color: var(--vp-c-text-1);
}
.su-res {
  font-size: 11px;
  font-weight: 700;
  padding: 2px 8px;
  border-radius: 3px;
  white-space: nowrap;
}
.su-res.ok {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.su-res.ng {
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
}
.su-res.up {
  background-color: var(--vp-c-yellow-soft, #f5e8c0);
  color: var(--vp-c-yellow-1, #a37e00);
}
.su-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
}
.su-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 14px;
}
.su-count {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.su-legend {
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
