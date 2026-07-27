<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// network/websocket(Go)を移植。SHA-1/base64/フレーム符号化を実際に計算し、
// Upgrade の Accept とフレーミング・マスキングを見せる。

const rotl = (x: number, n: number) => ((x << n) | (x >>> (32 - n))) >>> 0;
function sha1(bytes: number[]): number[] {
  let h0 = 0x67452301, h1 = 0xefcdab89, h2 = 0x98badcfe, h3 = 0x10325476, h4 = 0xc3d2e1f0;
  const ml = bytes.length * 8;
  const msg = [...bytes, 0x80];
  while (msg.length % 64 !== 56) msg.push(0);
  for (let i = 0; i < 4; i++) msg.push(0);
  msg.push((ml >>> 24) & 0xff, (ml >>> 16) & 0xff, (ml >>> 8) & 0xff, ml & 0xff);
  const w = new Array(80);
  for (let c = 0; c < msg.length; c += 64) {
    for (let i = 0; i < 16; i++)
      w[i] = ((msg[c + i * 4] << 24) | (msg[c + i * 4 + 1] << 16) | (msg[c + i * 4 + 2] << 8) | msg[c + i * 4 + 3]) >>> 0;
    for (let i = 16; i < 80; i++) w[i] = rotl((w[i - 3] ^ w[i - 8] ^ w[i - 14] ^ w[i - 16]) >>> 0, 1);
    let a = h0, b = h1, cc = h2, d = h3, e = h4;
    for (let i = 0; i < 80; i++) {
      let f, k;
      if (i < 20) { f = ((b & cc) | ((~b >>> 0) & d)) >>> 0; k = 0x5a827999; }
      else if (i < 40) { f = (b ^ cc ^ d) >>> 0; k = 0x6ed9eba1; }
      else if (i < 60) { f = ((b & cc) | (b & d) | (cc & d)) >>> 0; k = 0x8f1bbcdc; }
      else { f = (b ^ cc ^ d) >>> 0; k = 0xca62c1d6; }
      const tmp = (rotl(a, 5) + f + e + k + w[i]) >>> 0;
      e = d; d = cc; cc = rotl(b, 30); b = a; a = tmp;
    }
    h0 = (h0 + a) >>> 0; h1 = (h1 + b) >>> 0; h2 = (h2 + cc) >>> 0; h3 = (h3 + d) >>> 0; h4 = (h4 + e) >>> 0;
  }
  const out: number[] = [];
  for (const h of [h0, h1, h2, h3, h4]) out.push((h >>> 24) & 0xff, (h >>> 16) & 0xff, (h >>> 8) & 0xff, h & 0xff);
  return out;
}
function b64(d: number[]): string {
  const A = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
  let o = "";
  for (let i = 0; i < d.length; i += 3) {
    const rem = d.length - i;
    let n = d[i] << 16;
    if (rem > 1) n |= d[i + 1] << 8;
    if (rem > 2) n |= d[i + 2];
    o += A[(n >> 18) & 63] + A[(n >> 12) & 63] + (rem > 1 ? A[(n >> 6) & 63] : "=") + (rem > 2 ? A[n & 63] : "=");
  }
  return o;
}
const B = (s: string): number[] => Array.from(s).map((c) => c.charCodeAt(0));
const hex = (d: number[]): string => d.map((x) => x.toString(16).padStart(2, "0")).join(" ");
const asAscii = (d: number[]): string => d.map((x) => (x >= 32 && x < 127 ? String.fromCharCode(x) : "·")).join("");

const MAGIC = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11";
function accept(key: string): string {
  return b64(sha1(B(key + MAGIC)));
}

// フレーム符号化(text, fin=true)。
const OP_TEXT = 0x1;
function encodeFrame(payload: number[], masked: boolean, maskKey: number[]): number[] {
  const b: number[] = [0x80 | OP_TEXT]; // FIN + text
  const n = payload.length;
  const maskBit = masked ? 0x80 : 0;
  if (n <= 125) b.push(maskBit | n);
  else b.push(maskBit | 126, (n >> 8) & 0xff, n & 0xff);
  if (masked) {
    b.push(...maskKey);
    for (let i = 0; i < n; i++) b.push(payload[i] ^ maskKey[i % 4]);
  } else {
    b.push(...payload);
  }
  return b;
}

const modes = [
  { key: "upgrade", label: "Upgrade(昇格)" },
  { key: "frame", label: "フレーム" },
] as const;
const mode = ref<"upgrade" | "frame">("upgrade");

// --- Upgrade ---
const keys = ["dGhlIHNhbXBsZSBub25jZQ==", "x3JJHMbDL1EzLkh9GBhXDw==", "AQIDBAUGBwgJCgsMDQ4PEC=="];
const keyPick = ref(0);
const acceptVal = computed(() => accept(keys[keyPick.value]));
const isRFC = computed(() => keyPick.value === 0);

// --- Frame ---
const MSG = "Hello, WebSocket!";
const MASK_KEY = [0x37, 0xfa, 0x21, 0x3d];
const dirs = [
  { key: "s2c", label: "サーバ→クライアント(マスクなし)", masked: false },
  { key: "c2s", label: "クライアント→サーバ(マスクあり)", masked: true },
] as const;
const dirPick = ref(1);
const masked = computed(() => dirs[dirPick.value].masked);
const wire = computed(() => encodeFrame(B(MSG), masked.value, MASK_KEY));
const plaintextVisible = computed(() => {
  // ワイヤの ASCII に元メッセージがそのまま現れるか。
  return asAscii(wire.value).includes("Hello, WebSocket!");
});

const badge = computed(() =>
  mode.value === "upgrade"
    ? isRFC.value ? "RFC例と一致" : "Accept 計算"
    : masked.value ? "平文が隠れる" : "平文が見える",
);
const badgeTone = computed<"ok" | "ng" | "neutral">(() =>
  mode.value === "upgrade" ? "neutral" : masked.value ? "ok" : "neutral",
);

const note = computed(() => {
  if (mode.value === "upgrade") {
    return `Key に magic GUID を足して SHA-1 → base64 が Accept。${isRFC.value ? "これは RFC 6455 の例で、実物と同じ値が出る。" : ""}サーバがこの値を返せることが WebSocket 理解の証明になり、偶然の昇格を防ぐ`;
  }
  if (masked.value)
    return "クライアント→サーバのフレームは鍵で XOR してマスクする。ワイヤ上のバイト列に平文が現れない。攻撃者が特定のバイト列(偽のHTTPリクエスト等)を意図的に作れず、中継プロキシのキャッシュ汚染を防ぐ";
  return "サーバ→クライアントのフレームはマスクしない。ワイヤ上に平文がそのまま見える。この経路にはキャッシュ汚染の危険が少ないため";
});
</script>

<template>
  <DemoShell title="WebSocket" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <span class="sd-seg">
        <span
          v-for="m in modes"
          :key="m.key"
          class="sd-seg-opt"
          :class="{ on: mode === m.key }"
          @click="mode = m.key"
          >{{ m.label }}</span
        >
      </span>
    </div>

    <!-- Upgrade -->
    <div v-if="mode === 'upgrade'" class="ws-panel">
      <div class="ws-seg-row">
        <span class="ws-lbl">Sec-WebSocket-Key:</span>
        <span class="sd-seg">
          <span
            v-for="(k, i) in keys"
            :key="i"
            class="sd-seg-opt"
            :class="{ on: keyPick === i }"
            @click="keyPick = i"
            >key{{ i + 1 }}</span
          >
        </span>
      </div>
      <div class="ws-row"><span class="ws-k mono">Key</span><span class="ws-v mono">{{ keys[keyPick] }}</span></div>
      <div class="ws-row"><span class="ws-k mono">+ magic</span><span class="ws-v mono small">…{{ MAGIC.slice(-20) }}</span></div>
      <div class="ws-arrow">SHA-1 → base64 ↓</div>
      <div class="ws-row"><span class="ws-k mono">Accept</span><span class="ws-v mono key">{{ acceptVal }}</span></div>
    </div>

    <!-- Frame -->
    <div v-else class="ws-panel">
      <div class="ws-seg-row">
        <span class="sd-seg">
          <span
            v-for="(dd, i) in dirs"
            :key="dd.key"
            class="sd-seg-opt"
            :class="{ on: dirPick === i }"
            @click="dirPick = i"
            >{{ dd.label }}</span
          >
        </span>
      </div>
      <div class="ws-row"><span class="ws-k mono">メッセージ</span><span class="ws-v mono">"{{ MSG }}"</span></div>
      <div class="ws-row" v-if="masked">
        <span class="ws-k mono">マスク鍵</span><span class="ws-v mono">37 fa 21 3d</span>
      </div>
      <div class="ws-row"><span class="ws-k mono">ワイヤ(hex)</span><span class="ws-v mono small">{{ hex(wire) }}</span></div>
      <div class="ws-row">
        <span class="ws-k mono">ワイヤ(ascii)</span>
        <span class="ws-v mono" :class="plaintextVisible ? 'warn' : 'ok'">{{ asAscii(wire) }}</span>
      </div>
      <div class="ws-verdict" :class="plaintextVisible ? 'warn' : 'ok'">
        {{ plaintextVisible ? "平文がワイヤにそのまま見える(サーバ発は許される)" : "平文はワイヤに現れない(マスクで撹拌)" }}
      </div>
    </div>

    <p class="ws-note">{{ note }}</p>

    <p class="ws-legend">
      SHA-1・base64・フレーム符号化を移植して実際に計算している。WebSocket は HTTP として始まり、Key から
      導いた Accept 値の交換で全二重に昇格する。以後は両側がフレームで自由に送れる。クライアント→サーバの
      フレームはマスク(鍵で XOR)が必須で、これは中継プロキシのキャッシュ汚染攻撃を防ぐためのもの。暗号化とは別物だ。
    </p>
  </DemoShell>
</template>

<style scoped>
.ws-panel {
  margin-top: 16px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 14px;
  background-color: var(--vp-c-bg-soft);
}
.ws-seg-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}
.ws-lbl {
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.ws-row {
  display: flex;
  align-items: baseline;
  gap: 10px;
  padding: 3px 0;
}
.ws-k {
  width: 100px;
  flex-shrink: 0;
  font-size: 12px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  text-align: right;
}
.ws-v {
  font-size: 12px;
  color: var(--vp-c-text-1);
  word-break: break-all;
}
.ws-v.small {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.ws-v.key {
  color: var(--vp-c-brand-1);
  font-weight: 700;
}
.ws-v.ok {
  color: var(--vp-c-green-1);
}
.ws-v.warn {
  color: var(--vp-c-warning-1);
}
.ws-arrow {
  margin: 8px 0;
  font-size: 11.5px;
  color: var(--vp-c-text-3);
  text-align: center;
}
.ws-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid;
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
}
.ws-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.ws-verdict.warn {
  border-left-color: var(--vp-c-warning-1);
  color: var(--vp-c-warning-1);
  background-color: var(--vp-c-warning-soft);
}
.ws-note {
  margin: 12px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.ws-legend {
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
