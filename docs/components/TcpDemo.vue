<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// network/tcp(Go)の信頼転送を、決定的な "HELLO" 送信シナリオで見せる。
// ハンドシェイク → データ → セグメント損失 → 再送 → 並べ替えで順序復元、を1ステップずつ。

interface Seg {
  dir: "→" | "←";
  label: string; // フラグ + seq/ack
  payload?: string;
  dropped?: boolean;
  retransmit?: boolean;
  kind: "syn" | "ack" | "data" | "fin";
}
interface Frame {
  c: string; // client 状態
  s: string; // server 状態
  seg: Seg | null;
  recv: string; // サーバが順序どおり受け取った文字列
  ooo: string[]; // 並べ替えバッファ(seq:payload)
  rcvNxt: number;
  note: string;
}

// seq: client SYN=100, データ先頭=101。server SYN=300。"HELLO" を mss=2 で送る。
const FRAMES: Frame[] = [
  {
    c: "SYN_SENT", s: "LISTEN",
    seg: { dir: "→", label: "SYN seq=100", kind: "syn" },
    recv: "", ooo: [], rcvNxt: 101,
    note: "接続開始: クライアントが SYN を送る(初期シーケンス番号 100 を通知)",
  },
  {
    c: "SYN_SENT", s: "SYN_RCVD",
    seg: { dir: "←", label: "SYN ACK seq=300 ack=101", kind: "syn" },
    recv: "", ooo: [], rcvNxt: 101,
    note: "サーバが SYN-ACK。自分の seq=300 を通知しつつ ack=101 で SYN を確認",
  },
  {
    c: "ESTABLISHED", s: "ESTABLISHED",
    seg: { dir: "→", label: "ACK ack=301", kind: "ack" },
    recv: "", ooo: [], rcvNxt: 101,
    note: "クライアントが最終 ACK。3-way ハンドシェイク完了 → 接続確立",
  },
  {
    c: "ESTABLISHED", s: "ESTABLISHED",
    seg: { dir: "→", label: "seq=101", payload: "HE", kind: "data" },
    recv: "HE", ooo: [], rcvNxt: 103,
    note: "データ送信: seq=101 \"HE\"。順序どおりなので受領し rcvNxt=103 へ",
  },
  {
    c: "ESTABLISHED", s: "ESTABLISHED",
    seg: { dir: "→", label: "seq=103", payload: "LL", dropped: true, kind: "data" },
    recv: "HE", ooo: [], rcvNxt: 103,
    note: "seq=103 \"LL\" が途中で失われる(✗)。サーバには届かない",
  },
  {
    c: "ESTABLISHED", s: "ESTABLISHED",
    seg: { dir: "→", label: "seq=105", payload: "O", kind: "data" },
    recv: "HE", ooo: ["105:O"], rcvNxt: 103,
    note: "seq=105 \"O\" が先に到着。rcvNxt=103 の先なので並べ替えバッファへ退避",
  },
  {
    c: "ESTABLISHED", s: "ESTABLISHED",
    seg: { dir: "←", label: "ACK ack=103", kind: "ack" },
    recv: "HE", ooo: ["105:O"], rcvNxt: 103,
    note: "累積 ACK は 103 のまま = 穴がある合図。後続が届いても先へ進めない",
  },
  {
    c: "ESTABLISHED", s: "ESTABLISHED",
    seg: { dir: "→", label: "seq=103", payload: "LL", retransmit: true, kind: "data" },
    recv: "HE", ooo: ["105:O"], rcvNxt: 103,
    note: "再送: 再送タイマ満了で、クライアントが seq=103 \"LL\" を送り直す",
  },
  {
    c: "ESTABLISHED", s: "ESTABLISHED",
    seg: { dir: "←", label: "ACK ack=106", kind: "ack" },
    recv: "HELLO", ooo: [], rcvNxt: 106,
    note: "穴が埋まった! \"LL\" で rcvNxt=105、退避していた \"O\" も連続するのでまとめて確定 → \"HELLO\"",
  },
  {
    c: "FIN_WAIT", s: "ESTABLISHED",
    seg: { dir: "→", label: "FIN ACK seq=106", kind: "fin" },
    recv: "HELLO", ooo: [], rcvNxt: 106,
    note: "送信終了: クライアントが FIN(seq=106)。FIN もシーケンス番号を 1 消費する",
  },
  {
    c: "FIN_WAIT", s: "LAST_ACK",
    seg: { dir: "←", label: "FIN ACK seq=306 ack=107", kind: "fin" },
    recv: "HELLO", ooo: [], rcvNxt: 107,
    note: "サーバも受領を ACK し、自分の FIN を送る",
  },
  {
    c: "CLOSED", s: "CLOSED",
    seg: { dir: "→", label: "ACK ack=307", kind: "ack" },
    recv: "HELLO", ooo: [], rcvNxt: 107,
    note: "クライアントが最終 ACK。接続クローズ完了。\"HELLO\" が欠けも乱れもなく届いた",
  },
];

const at = ref(0);
const cur = computed(() => FRAMES[at.value]);
const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < FRAMES.length - 1);
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
  at.value = FRAMES.length - 1;
}

const done = computed(() => cur.value.c === "CLOSED");
const badge = computed(() => (done.value ? "転送完了" : `step ${at.value + 1}`));
const badgeTone = computed<"ok" | "neutral">(() => (done.value ? "ok" : "neutral"));
const recvChars = computed(() => cur.value.recv.split(""));
</script>

<template>
  <DemoShell title="tcp(信頼できるバイトストリーム)" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">1手すすめる</button>
      <span class="spacer" />
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="tc-count">{{ at + 1 }} / {{ FRAMES.length }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <div class="tc-net">
      <div class="tc-host">
        <div class="tc-host-name">client</div>
        <div class="tc-state">{{ cur.c }}</div>
      </div>

      <div class="tc-wire">
        <div
          v-if="cur.seg"
          class="tc-seg"
          :class="[cur.seg.kind, { dropped: cur.seg.dropped, left: cur.seg.dir === '←' }]"
        >
          <span class="tc-arrow">{{ cur.seg.dir === "→" ? "▶" : "◀" }}</span>
          <span class="tc-seg-label">
            {{ cur.seg.label }}
            <span v-if="cur.seg.payload" class="tc-pay">"{{ cur.seg.payload }}"</span>
          </span>
          <span v-if="cur.seg.retransmit" class="tc-badge rex">再送</span>
          <span v-if="cur.seg.dropped" class="tc-badge drop">✗ 損失</span>
        </div>
        <div v-else class="tc-wire-empty">—</div>
      </div>

      <div class="tc-host">
        <div class="tc-host-name">server</div>
        <div class="tc-state">{{ cur.s }}</div>
      </div>
    </div>

    <div class="tc-recv">
      <div class="tc-recv-row">
        <span class="tc-recv-label">受信(順序どおり確定)</span>
        <div class="tc-bytes">
          <span v-for="(ch, k) in recvChars" :key="k" class="tc-byte">{{ ch }}</span>
          <span v-if="!recvChars.length" class="tc-empty">—</span>
        </div>
        <span class="tc-rcvnxt">rcvNxt={{ cur.rcvNxt }}</span>
      </div>
      <div class="tc-recv-row">
        <span class="tc-recv-label">並べ替えバッファ</span>
        <div class="tc-bytes">
          <span v-for="(o, k) in cur.ooo" :key="k" class="tc-byte ooo">{{ o }}</span>
          <span v-if="!cur.ooo.length" class="tc-empty">(空)</span>
        </div>
      </div>
    </div>

    <p class="tc-note">{{ cur.note }}</p>

    <p class="tc-legend">
      累積 ACK は「連続して受け取った先頭」までしか進めない。だから穴が 1 つあると後続が届いても
      ACK は止まり、送信側が抜けを再送する。受信側は先行分を退避しておき、穴が埋まればまとめて確定する。
    </p>
  </DemoShell>
</template>

<style scoped>
.tc-count {
  font-size: 12px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.tc-net {
  display: grid;
  grid-template-columns: 120px 1fr 120px;
  align-items: center;
  gap: 10px;
  margin-top: 16px;
}
.tc-host {
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 12px 10px;
  text-align: center;
  background-color: var(--vp-c-bg);
}
.tc-host-name {
  font-family: var(--vp-font-family-mono);
  font-weight: 700;
  font-size: 14px;
  color: var(--vp-c-text-1);
}
.tc-state {
  margin-top: 6px;
  font-size: 11px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-brand-1);
  font-weight: 700;
}
.tc-wire {
  min-height: 52px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-bottom: 1px dashed var(--vp-c-divider);
  border-top: 1px dashed var(--vp-c-divider);
}
.tc-seg {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 4px;
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-1);
}
.tc-seg.left {
  flex-direction: row-reverse;
}
.tc-seg.syn {
  border-color: var(--vp-c-brand-1);
  color: var(--vp-c-brand-1);
}
.tc-seg.data {
  background-color: var(--vp-c-brand-soft);
}
.tc-seg.fin {
  border-color: var(--vp-c-text-3);
}
.tc-seg.dropped {
  opacity: 0.55;
  text-decoration: line-through;
  border-color: var(--vp-c-danger-1);
}
.tc-arrow {
  color: var(--vp-c-brand-1);
}
.tc-pay {
  font-weight: 700;
}
.tc-badge {
  font-size: 10px;
  font-weight: 700;
  padding: 1px 6px;
  border-radius: 3px;
  text-decoration: none;
}
.tc-badge.rex {
  background-color: var(--vp-c-brand-1);
  color: var(--vp-c-bg);
}
.tc-badge.drop {
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
}
.tc-wire-empty {
  color: var(--vp-c-text-3);
}
.tc-recv {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg);
}
.tc-recv-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
}
.tc-recv-row + .tc-recv-row {
  border-top: 1px solid var(--vp-c-divider);
}
.tc-recv-label {
  font-size: 11px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-3);
  width: 150px;
  flex: 0 0 auto;
}
.tc-bytes {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  flex: 1;
}
.tc-byte {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 22px;
  height: 24px;
  padding: 0 6px;
  font-size: 13px;
  font-weight: 600;
  font-family: var(--vp-font-family-mono);
  border: 1px solid var(--vp-c-divider);
  border-radius: 3px;
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.tc-byte.ooo {
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-2);
  border-color: var(--vp-c-divider);
}
.tc-empty {
  font-size: 12px;
  color: var(--vp-c-text-3);
  font-style: italic;
}
.tc-rcvnxt {
  font-size: 11px;
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-text-2);
  flex: 0 0 auto;
}
.tc-note {
  margin: 14px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  font-family: var(--vp-font-family-mono);
}
.tc-legend {
  margin: 14px 0 0;
  padding-top: 12px;
  border-top: 1px solid var(--vp-c-divider);
  font-size: 12px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
}
</style>
