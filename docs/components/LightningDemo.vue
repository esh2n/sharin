<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// chain/lightning(Go)の考え方をブラウザに移植。
// channel: オフチェーン更新 → 古い状態の提出(不正)→ ペナルティで全額没収。
// htlc: A—B—C を preimage のハッシュで縛り、多段送金を仲介者を信頼せず成立させる。

interface Bar {
  label: string;
  left: number;
  right: number;
  leftName: string;
  rightName: string;
  locked: number; // HTLC でロック中の額(宙に浮く)
}
interface Commit {
  no: number;
  a: number;
  b: number;
  revoked: boolean;
  live: boolean;
  broadcast: boolean;
}
interface Frame {
  action: string;
  result: "ok" | "bad" | "penalty" | null;
  note: string;
  bars: Bar[];
  commits: Commit[] | null;
  state: string;
}

function bar(label: string, left: number, right: number, locked = 0): Bar {
  return { label, left, right, leftName: "alice", rightName: "bob", locked };
}

function channelFrames(): Frame[] {
  const c = (no: number, a: number, b: number, revoked: boolean, live: boolean, broadcast = false): Commit => ({
    no,
    a,
    b,
    revoked,
    live,
    broadcast,
  });
  return [
    {
      action: "チャネルを開く(funding)",
      result: null,
      note: "alice が 100 をチェーン上の 2-of-2 マルチシグにロックする。ここがチェーンに触れる 1 回目。以降の送金はチェーンに載らない",
      bars: [bar("channel", 100, 0)],
      commits: [c(0, 100, 0, false, true)],
      state: "open",
    },
    {
      action: "alice → bob 40(オフチェーン)",
      result: null,
      note: "新しい commitment #1 を 2 人で署名して差し替えるだけ。チェーンには一切触れない。瞬時でほぼ無料——これを何千回でも繰り返せる",
      bars: [bar("channel", 60, 40)],
      commits: [c(0, 100, 0, true, false), c(1, 60, 40, false, true)],
      state: "open",
    },
    {
      action: "alice → bob 30(オフチェーン)",
      result: null,
      note: "さらに更新して #2 へ。#0 と #1 は revoked(置き換え済み)。revoke すると、その状態のリボケーション秘密が相手に渡る——後で効いてくる",
      bars: [bar("channel", 30, 70)],
      commits: [c(0, 100, 0, true, false), c(1, 60, 40, true, false), c(2, 30, 70, false, true)],
      state: "open",
    },
    {
      action: "alice が古い #0(100/0)を提出",
      result: "bad",
      note: "alice が「まだ払う前」の #0 をチェーンに出し、払った 70 を無かったことにしようとする。即確定はせず係争に入り、bob が咎める猶予(異議申立て期間)が始まる",
      bars: [bar("channel", 100, 0)],
      commits: [c(0, 100, 0, true, false, true), c(1, 60, 40, true, false), c(2, 30, 70, false, true)],
      state: "disputed",
    },
    {
      action: "bob がペナルティを行使",
      result: "penalty",
      note: "bob は #0 のリボケーション秘密を握っている。それを示して全額 100 を没収する。不正提出は「成功すれば元本、失敗すれば全額没収」の割に合わない賭け——だからオフチェーン更新が安全になる",
      bars: [bar("channel", 0, 100)],
      commits: [c(0, 100, 0, true, false, true), c(1, 60, 40, true, false), c(2, 30, 70, true, false)],
      state: "penalty",
    },
  ];
}

function htlcBar(label: string, l: string, r: string, left: number, right: number, locked = 0): Bar {
  return { label, left, right, leftName: l, rightName: r, locked };
}

function htlcFrames(): Frame[] {
  return [
    {
      action: "A—B—C(A は C と直接繋がらない)",
      result: null,
      note: "alice は carol と直接チャネルを持たない。bob を経由して送りたいが、bob が金だけ受け取って渡さないかもしれない。これを暗号で縛るのが HTLC",
      bars: [htlcBar("alice—bob", "alice", "bob", 100, 100), htlcBar("bob—carol", "bob", "carol", 100, 100)],
      commits: null,
      state: "open",
    },
    {
      action: "carol が preimage を用意、hash H を共有",
      result: null,
      note: "受取人 carol だけが知る秘密 preimage を作り、そのハッシュ H を送金側 alice に伝える。各ホップは「H の preimage を出せた者にだけ払う」条件でロックされる",
      bars: [htlcBar("alice—bob", "alice", "bob", 100, 100), htlcBar("bob—carol", "bob", "carol", 100, 100)],
      commits: null,
      state: "open",
    },
    {
      action: "alice が A—B に HTLC 20 をロック(期限 10)",
      result: null,
      note: "alice の 20 が宙に浮く(誰の残高でもないロック状態)。alice—bob は 80 / [20] / 100。まだ carol までは届いていない",
      bars: [htlcBar("alice—bob", "alice", "bob", 80, 100, 20), htlcBar("bob—carol", "bob", "carol", 100, 100)],
      commits: null,
      state: "open",
    },
    {
      action: "bob が B—C に HTLC 20 をロック(期限 8)",
      result: null,
      note: "bob は同じ H で下流にもロックを張る。期限は上流(10)より短い(8)——下流が確定してから上流を確定する時間差を作るため。bob はまだ得も損もしていない",
      bars: [htlcBar("alice—bob", "alice", "bob", 80, 100, 20), htlcBar("bob—carol", "bob", "carol", 80, 100, 20)],
      commits: null,
      state: "open",
    },
    {
      action: "carol が preimage を公開 → 経路が成立",
      result: "ok",
      note: "carol が preimage を出して B—C を成立させ 20 を得る。その瞬間 bob は preimage を知り、それで A—B を成立させて 20 を回収。preimage が下流から上流へ伝播し、経路全体が同時に成立した。alice が 20 減り carol が 20 増え、仲介 bob の純資産は不変(素通し)",
      bars: [htlcBar("alice—bob", "alice", "bob", 80, 120), htlcBar("bob—carol", "bob", "carol", 80, 120)],
      commits: null,
      state: "open",
    },
  ];
}

const mode = ref<"channel" | "htlc">("channel");
const frames = computed(() => (mode.value === "channel" ? channelFrames() : htlcFrames()));
const at = ref(0);
const cur = computed(() => frames.value[at.value]);

function setMode(m: "channel" | "htlc") {
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
  return mode.value === "channel" ? "不正は全額没収された" : "多段送金が成立";
});
const badgeTone = computed<"ok" | "ng" | "neutral">(() => (done.value ? "ok" : "neutral"));

function pct(v: number, total: number): number {
  return total === 0 ? 0 : (v / total) * 100;
}
function barTotal(b: Bar): number {
  return b.left + b.right + b.locked;
}
</script>

<template>
  <DemoShell title="lightning(決済チャネル)" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <span class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: mode === 'channel' }" @click="setMode('channel')">オフチェーン + ペナルティ</span>
        <span class="sd-seg-opt" :class="{ on: mode === 'htlc' }" @click="setMode('htlc')">HTLC マルチホップ</span>
      </span>
      <span class="spacer" />
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">1手すすめる</button>
    </div>

    <div class="lt-bars">
      <div v-for="b in cur.bars" :key="b.label" class="lt-barwrap">
        <div class="lt-barhead">
          <span class="lt-barname">{{ b.label }}</span>
          <span class="lt-barvals mono">
            {{ b.leftName }} {{ b.left }} · {{ b.rightName }} {{ b.right }}<template v-if="b.locked"> · ロック {{ b.locked }}</template>
          </span>
        </div>
        <div class="lt-bar">
          <div class="lt-seg left" :style="{ width: pct(b.left, barTotal(b)) + '%' }" />
          <div v-if="b.locked" class="lt-seg lock" :style="{ width: pct(b.locked, barTotal(b)) + '%' }" />
          <div class="lt-seg right" :style="{ width: pct(b.right, barTotal(b)) + '%' }" />
        </div>
      </div>
    </div>

    <div v-if="cur.commits" class="lt-panel">
      <div class="lt-panel-head">commitment(番号が進むほど新しい・過去は revoked)</div>
      <div class="lt-body">
        <div
          v-for="cm in cur.commits"
          :key="cm.no"
          class="lt-commit"
          :class="{ revoked: cm.revoked, live: cm.live, broadcast: cm.broadcast }"
        >
          <span class="lt-cm-no mono">#{{ cm.no }}</span>
          <span class="lt-cm-bal mono">alice {{ cm.a }} / bob {{ cm.b }}</span>
          <span v-if="cm.live" class="lt-cm-tag live">最新</span>
          <span v-else-if="cm.broadcast" class="lt-cm-tag bad">提出(不正)</span>
          <span v-else-if="cm.revoked" class="lt-cm-tag rev">revoked</span>
        </div>
      </div>
    </div>

    <div class="lt-action" :class="cur.result">
      <span class="lt-act">{{ cur.action }}</span>
      <span v-if="cur.result === 'ok'" class="lt-res ok">成立</span>
      <span v-else-if="cur.result === 'bad'" class="lt-res ng">不正提出 → 係争</span>
      <span v-else-if="cur.result === 'penalty'" class="lt-res warn">全額没収</span>
    </div>
    <p class="lt-note">{{ cur.note }}</p>

    <div class="lt-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="lt-count mono">{{ at + 1 }} / {{ frames.length }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="lt-legend">
      チェーンに触れるのは開閉の 2 回だけで、送金はオフチェーンの commitment 差し替えで進む。
      古い状態を出す不正はリボケーション秘密による全額没収で割に合わなくし、直接繋がらない相手へは
      preimage のハッシュで縛った HTLC を多段に張って届ける。
    </p>
  </DemoShell>
</template>

<style scoped>
.lt-bars {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-top: 16px;
}
.lt-barwrap {
  display: flex;
  flex-direction: column;
  gap: 5px;
}
.lt-barhead {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 10px;
  font-size: 12px;
}
.lt-barname {
  font-weight: 700;
  color: var(--vp-c-text-2);
}
.lt-barvals {
  color: var(--vp-c-text-3);
  font-size: 11px;
}
.lt-bar {
  display: flex;
  height: 26px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
  background-color: var(--vp-c-bg-soft);
}
.lt-seg {
  transition: width 0.25s ease;
}
.lt-seg.left {
  background-color: var(--vp-c-brand-soft);
}
.lt-seg.right {
  background-color: var(--vp-c-green-soft);
}
.lt-seg.lock {
  background-color: var(--vp-c-yellow-soft, #f5e8c0);
  background-image: repeating-linear-gradient(
    45deg,
    transparent,
    transparent 4px,
    rgba(0, 0, 0, 0.08) 4px,
    rgba(0, 0, 0, 0.08) 8px
  );
}
.lt-panel {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  overflow: hidden;
}
.lt-panel-head {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  padding: 8px 12px;
  border-bottom: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.lt-body {
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.lt-commit {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 10px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  background-color: var(--vp-c-bg-soft);
  font-size: 13px;
}
.lt-commit.revoked {
  opacity: 0.5;
}
.lt-commit.live {
  border-color: var(--vp-c-green-1);
}
.lt-commit.broadcast {
  border-color: var(--vp-c-danger-1);
  opacity: 1;
}
.lt-cm-no {
  color: var(--vp-c-text-3);
}
.lt-cm-bal {
  flex: 1;
}
.lt-cm-tag {
  font-size: 11px;
  font-weight: 700;
  padding: 2px 8px;
  border-radius: 3px;
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-2);
}
.lt-cm-tag.live {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.lt-cm-tag.bad {
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
}
.lt-action {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 14px;
  padding: 10px 12px;
  border: 1px solid var(--vp-c-divider);
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  background-color: var(--vp-c-bg);
  font-size: 13px;
}
.lt-action.ok {
  border-left-color: var(--vp-c-green-1);
}
.lt-action.bad {
  border-left-color: var(--vp-c-danger-1);
}
.lt-action.penalty {
  border-left-color: var(--vp-c-yellow-1, #d0a215);
}
.lt-act {
  flex: 1;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-1);
}
.lt-res {
  font-size: 11px;
  font-weight: 700;
  padding: 2px 8px;
  border-radius: 3px;
}
.lt-res.ok {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.lt-res.ng {
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
}
.lt-res.warn {
  background-color: var(--vp-c-yellow-soft, #f5e8c0);
  color: var(--vp-c-yellow-1, #a37e00);
}
.lt-note {
  margin: 10px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
}
.lt-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 14px;
}
.lt-count {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.lt-legend {
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
