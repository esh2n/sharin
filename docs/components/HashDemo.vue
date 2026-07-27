<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// crypto/hash(Go)をブラウザに移植。Go の実装と同じ計算をして、
// なだれ効果・長さ拡張攻撃の成功・HMAC の防御を実際に見せる。

const SIZE = 16;
const IV = [0x67452301, 0xefcdab89, 0x98badcfe, 0x10325476];
const CONSTS = [
  0x5a827999, 0x6ed9eba1, 0x8f1bbcdc, 0xca62c1d6, 0x452821e6, 0x38d01377, 0xbe5466cf, 0x34e90c6c,
];

function rotl(x: number, n: number): number {
  return ((x << n) | (x >>> (32 - n))) >>> 0;
}
function le32(b: number[], off: number): number {
  return (b[off] | (b[off + 1] << 8) | (b[off + 2] << 16) | (b[off + 3] << 24)) >>> 0;
}
function put32(out: number[], off: number, w: number) {
  out[off] = w & 0xff;
  out[off + 1] = (w >>> 8) & 0xff;
  out[off + 2] = (w >>> 16) & 0xff;
  out[off + 3] = (w >>> 24) & 0xff;
}

function compress(s: number[], block: number[]): number[] {
  const w = [le32(block, 0), le32(block, 4), le32(block, 8), le32(block, 12)];
  let [a, b, c, d] = s;
  for (let r = 0; r < 8; r++) {
    const mixed = (((a + (b ^ c ^ d)) >>> 0) + w[r % 4] + CONSTS[r]) >>> 0;
    a = rotl(mixed, 7);
    [a, b, c, d] = [d, a, b, c];
  }
  return [(s[0] + a) >>> 0, (s[1] + b) >>> 0, (s[2] + c) >>> 0, (s[3] + d) >>> 0];
}

function padding(msgLen: number): number[] {
  const zeros = (SIZE - ((msgLen + 1 + 8) % SIZE)) % SIZE;
  const pad = new Array(1 + zeros + 8).fill(0);
  pad[0] = 0x80;
  const bits = msgLen * 8;
  put32(pad, 1 + zeros, bits >>> 0);
  put32(pad, 1 + zeros + 4, Math.floor(bits / 0x100000000));
  return pad;
}

function sum(data: number[]): number[] {
  let s = [...IV];
  const padded = [...data, ...padding(data.length)];
  for (let i = 0; i < padded.length; i += SIZE) s = compress(s, padded.slice(i, i + SIZE));
  const out = new Array(SIZE).fill(0);
  for (let i = 0; i < 4; i++) put32(out, i * 4, s[i]);
  return out;
}

function extend(digest: number[], origLen: number, ext: number[]): { forged: number[]; glue: number[] } {
  const glue = padding(origLen);
  let s = [le32(digest, 0), le32(digest, 4), le32(digest, 8), le32(digest, 12)];
  const totalLen = origLen + glue.length + ext.length;
  const tail = [...ext, ...padding(totalLen)];
  for (let i = 0; i < tail.length; i += SIZE) s = compress(s, tail.slice(i, i + SIZE));
  const out = new Array(SIZE).fill(0);
  for (let i = 0; i < 4; i++) put32(out, i * 4, s[i]);
  return { forged: out, glue };
}

function hmac(key: number[], msg: number[]): number[] {
  let k = key;
  if (k.length > SIZE) k = sum(k);
  const kk = new Array(SIZE).fill(0);
  for (let i = 0; i < k.length && i < SIZE; i++) kk[i] = k[i];
  const ipad = kk.map((x) => x ^ 0x36);
  const opad = kk.map((x) => x ^ 0x5c);
  const inner = sum([...ipad, ...msg]);
  return sum([...opad, ...inner]);
}

const bytes = (s: string): number[] => Array.from(s).map((c) => c.charCodeAt(0));
const hex = (b: number[]): string => b.map((x) => x.toString(16).padStart(2, "0")).join("");
const eq = (a: number[], b: number[]): boolean => a.length === b.length && a.every((x, i) => x === b[i]);
function popcountDiff(a: number[], b: number[]): number {
  let d = 0;
  for (let i = 0; i < a.length; i++) {
    let x = a[i] ^ b[i];
    while (x) {
      d += x & 1;
      x >>= 1;
    }
  }
  return d;
}

const modes = [
  { key: "avalanche", label: "なだれ効果" },
  { key: "extension", label: "長さ拡張(素朴なMAC)" },
  { key: "hmac", label: "HMAC(防御)" },
] as const;
const mode = ref<"avalanche" | "extension" | "hmac">("avalanche");

// --- なだれ効果 ---
const baseline = "amount=100";
const variants = ["amount=100", "amount=101", "Amount=100", "amount=100 "];
const vPick = ref(1);
const avalanche = computed(() => {
  const base = sum(bytes(baseline));
  const cur = sum(bytes(variants[vPick.value]));
  return { base, cur, bits: popcountDiff(base, cur), same: variants[vPick.value] === baseline };
});

// --- 長さ拡張 / HMAC 共通の攻撃設定 ---
const SECRET = "supersecretkey";
const MSG = "amount=100&to=alice";
const EXT = "&to=attacker";

const attack = computed(() => {
  const useHmac = mode.value === "hmac";
  const tag = useHmac ? hmac(bytes(SECRET), bytes(MSG)) : sum([...bytes(SECRET), ...bytes(MSG)]);
  const { forged, glue } = extend(tag, SECRET.length + MSG.length, bytes(EXT));
  const forgedMsg = [...bytes(MSG), ...glue, ...bytes(EXT)];
  const actual = useHmac ? hmac(bytes(SECRET), forgedMsg) : sum([...bytes(SECRET), ...forgedMsg]);
  return { tag, forged, actual, forgedMsg, success: eq(forged, actual) };
});

function showBytes(b: number[]): string {
  return b.map((x) => (x >= 32 && x < 127 ? String.fromCharCode(x) : x === 0x80 ? "‖" : ".")).join("");
}

const badge = computed(() => {
  if (mode.value === "avalanche") return avalanche.value.same ? "同一入力" : `${avalanche.value.bits}/128 bit 変化`;
  return attack.value.success ? "偽造成功" : "偽造失敗";
});
const badgeTone = computed<"ok" | "ng" | "neutral">(() => {
  if (mode.value === "avalanche") return "neutral";
  // 素朴MACで偽造成功=危険(ng)、HMACで失敗=安全(ok)
  return attack.value.success ? "ng" : "ok";
});

const note = computed(() => {
  if (mode.value === "avalanche") {
    if (avalanche.value.same) return "同じ入力は同じ指紋。ここから 1 文字だけ変えてみる";
    return `1 文字違いで指紋の ${avalanche.value.bits}/128 ビットが変わる。入力の微差が指紋を総崩れにする(なだれ効果)。だから改ざんは指紋の一致で検知できる`;
  }
  if (mode.value === "extension") {
    return "攻撃者は secret を知らないのに、tag と長さだけで末尾に &to=attacker を継ぎ足した正しい tag を偽造できた。素朴な H(secret‖msg) は認証符号に使えない";
  }
  return "同じ攻撃を HMAC に対して試すと、偽造 tag は本物と一致しない。内側のハッシュを外側で包むので、末尾を継ぎ足しても正しい tag は作れない";
});
</script>

<template>
  <DemoShell title="ハッシュとHMAC" :badge="badge" :badge-tone="badgeTone">
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

    <!-- なだれ効果 -->
    <div v-if="mode === 'avalanche'" class="hs-panel">
      <div class="hs-seg-row">
        <span class="hs-lbl">入力を選ぶ:</span>
        <span class="sd-seg">
          <span
            v-for="(v, i) in variants"
            :key="i"
            class="sd-seg-opt"
            :class="{ on: vPick === i }"
            @click="vPick = i"
            >"{{ v }}"</span
          >
        </span>
      </div>
      <div class="hs-row">
        <span class="hs-k mono">H("{{ baseline }}")</span>
        <span class="hs-hex mono">{{ hex(avalanche.base) }}</span>
      </div>
      <div class="hs-row">
        <span class="hs-k mono">H("{{ variants[vPick] }}")</span>
        <span class="hs-hex mono" :class="{ diff: !avalanche.same }">{{ hex(avalanche.cur) }}</span>
      </div>
    </div>

    <!-- 長さ拡張 / HMAC -->
    <div v-else class="hs-panel">
      <div class="hs-row">
        <span class="hs-k mono">secret</span>
        <span class="hs-hex mono muted">●●●●●●●●(攻撃者は知らない)</span>
      </div>
      <div class="hs-row">
        <span class="hs-k mono">msg</span>
        <span class="hs-hex mono">{{ MSG }}</span>
      </div>
      <div class="hs-row">
        <span class="hs-k mono">tag</span>
        <span class="hs-hex mono">{{ hex(attack.tag) }}</span>
      </div>
      <div class="hs-arrow">攻撃者が末尾に "{{ EXT }}" を継ぎ足して偽造を試みる ↓</div>
      <div class="hs-row">
        <span class="hs-k mono">偽造msg</span>
        <span class="hs-hex mono small">{{ showBytes(attack.forgedMsg) }}</span>
      </div>
      <div class="hs-row">
        <span class="hs-k mono">偽造tag</span>
        <span class="hs-hex mono">{{ hex(attack.forged) }}</span>
      </div>
      <div class="hs-row">
        <span class="hs-k mono">本物tag</span>
        <span class="hs-hex mono">{{ hex(attack.actual) }}</span>
      </div>
      <div class="hs-verdict" :class="attack.success ? 'bad' : 'good'">
        {{ attack.success ? "一致 → 秘密なしで偽造成功(素朴なMACは破れる)" : "不一致 → 偽造失敗(HMACが防いだ)" }}
      </div>
    </div>

    <p class="hs-note">{{ note }}</p>

    <p class="hs-legend">
      Go 実装をそのまま移植して計算している。ハッシュは 1 ビットの差を指紋全体に広げる(なだれ効果)。
      だが素朴な H(secret‖msg) は、指紋が内部状態そのものなので、末尾を継ぎ足した正しい指紋を秘密なしで
      偽造できる(長さ拡張攻撃)。HMAC は内側を外側で包むことでこれを防ぐ。認証符号には HMAC を使う。
    </p>
  </DemoShell>
</template>

<style scoped>
.hs-panel {
  margin-top: 16px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 14px;
  background-color: var(--vp-c-bg-soft);
}
.hs-seg-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}
.hs-lbl {
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.hs-row {
  display: flex;
  align-items: baseline;
  gap: 10px;
  padding: 3px 0;
}
.hs-k {
  width: 118px;
  flex-shrink: 0;
  font-size: 12px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  text-align: right;
}
.hs-hex {
  font-size: 12px;
  color: var(--vp-c-text-1);
  word-break: break-all;
}
.hs-hex.small {
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.hs-hex.muted,
.hs-hex.small {
  color: var(--vp-c-text-3);
}
.hs-hex.diff {
  color: var(--vp-c-danger-1);
  font-weight: 600;
}
.hs-arrow {
  margin: 10px 0;
  font-size: 11.5px;
  color: var(--vp-c-text-3);
  text-align: center;
}
.hs-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid;
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
}
.hs-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.hs-verdict.good {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.hs-note {
  margin: 12px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.hs-legend {
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
