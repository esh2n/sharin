<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// crypto/cipher + crypto/hash(Go)をブラウザに移植。Go と同じ計算で
// ECB の模様漏れ・CTR の可鍛性・Seal/Open の改ざん検知を実際に見せる。

// --- hash(HMAC 用) ---
const HSIZE = 16;
const IV = [0x67452301, 0xefcdab89, 0x98badcfe, 0x10325476];
const HCONSTS = [
  0x5a827999, 0x6ed9eba1, 0x8f1bbcdc, 0xca62c1d6, 0x452821e6, 0x38d01377, 0xbe5466cf, 0x34e90c6c,
];
const rotl = (x: number, n: number) => ((x << n) | (x >>> (32 - n))) >>> 0;
const le32 = (b: number[], o: number) => (b[o] | (b[o + 1] << 8) | (b[o + 2] << 16) | (b[o + 3] << 24)) >>> 0;
function put32(o: number[], f: number, w: number) {
  o[f] = w & 0xff;
  o[f + 1] = (w >>> 8) & 0xff;
  o[f + 2] = (w >>> 16) & 0xff;
  o[f + 3] = (w >>> 24) & 0xff;
}
function hcompress(s: number[], bl: number[]): number[] {
  const w = [le32(bl, 0), le32(bl, 4), le32(bl, 8), le32(bl, 12)];
  let [a, b, c, d] = s;
  for (let r = 0; r < 8; r++) {
    const m = (((a + (b ^ c ^ d)) >>> 0) + w[r % 4] + HCONSTS[r]) >>> 0;
    a = rotl(m, 7);
    [a, b, c, d] = [d, a, b, c];
  }
  return [(s[0] + a) >>> 0, (s[1] + b) >>> 0, (s[2] + c) >>> 0, (s[3] + d) >>> 0];
}
function hpad(n: number): number[] {
  const z = (HSIZE - ((n + 1 + 8) % HSIZE)) % HSIZE;
  const p = new Array(1 + z + 8).fill(0);
  p[0] = 0x80;
  const bits = n * 8;
  put32(p, 1 + z, bits >>> 0);
  put32(p, 1 + z + 4, Math.floor(bits / 0x100000000));
  return p;
}
function hsum(d: number[]): number[] {
  let s = [...IV];
  const p = [...d, ...hpad(d.length)];
  for (let i = 0; i < p.length; i += HSIZE) s = hcompress(s, p.slice(i, i + HSIZE));
  const o = new Array(HSIZE).fill(0);
  for (let i = 0; i < 4; i++) put32(o, i * 4, s[i]);
  return o;
}
function hmac(key: number[], msg: number[]): number[] {
  let k = key;
  if (k.length > HSIZE) k = hsum(k);
  const kk = new Array(HSIZE).fill(0);
  for (let i = 0; i < k.length && i < HSIZE; i++) kk[i] = k[i];
  return hsum([...kk.map((x) => x ^ 0x5c), ...hsum([...kk.map((x) => x ^ 0x36), ...msg])]);
}
const eqBytes = (a: number[], b: number[]) => a.length === b.length && a.every((x, i) => x === b[i]);

// --- cipher(Feistel + modes) ---
const BLK = 8;
const ROUNDS = 16;
function expandKey(keyBytes: number[]): number[] {
  let seed = 1469598103934665603n;
  const M = 0xffffffffffffffffn;
  for (const b of keyBytes) seed = ((seed ^ BigInt(b)) * 1099511628211n) & M;
  const rk: number[] = [];
  for (let i = 0; i < ROUNDS; i++) {
    seed = (seed * 6364136223846793005n + 1442695040888963407n) & M;
    rk.push(Number((seed >> 32n) & 0xffffffffn));
  }
  return rk;
}
function feistelF(r: number, k: number): number {
  let x = Math.imul(r ^ k, 2654435761) >>> 0;
  x = rotl((x + 0x9e3779b9) >>> 0, 7);
  x ^= rotl(x, 11);
  return x >>> 0;
}
function encBlock(rk: number[], src: number[]): number[] {
  let l = le32(src, 0),
    r = le32(src, 4);
  for (let i = 0; i < ROUNDS; i++) [l, r] = [r, (l ^ feistelF(r, rk[i])) >>> 0];
  const o = new Array(BLK).fill(0);
  put32(o, 0, l);
  put32(o, 4, r);
  return o;
}
function ecb(rk: number[], plain: number[]): number[] {
  const p = pkcs7(plain);
  const out: number[] = [];
  for (let i = 0; i < p.length; i += BLK) out.push(...encBlock(rk, p.slice(i, i + BLK)));
  return out;
}
function cbc(rk: number[], plain: number[], iv: number[]): number[] {
  const p = pkcs7(plain);
  const out: number[] = [];
  let prev = iv.slice(0, BLK);
  for (let i = 0; i < p.length; i += BLK) {
    const blk = p.slice(i, i + BLK).map((x, j) => x ^ prev[j]);
    const c = encBlock(rk, blk);
    out.push(...c);
    prev = c;
  }
  return out;
}
function ctr(rk: number[], data: number[], nonce: number[]): number[] {
  const out = new Array(data.length).fill(0);
  let counter = 0;
  for (let i = 0; i < data.length; i += BLK) {
    const inb = new Array(BLK).fill(0);
    for (let j = 0; j < nonce.length && j < 4; j++) inb[j] = nonce[j];
    put32(inb, 4, counter >>> 0);
    const ks = encBlock(rk, inb);
    for (let j = 0; j < BLK && i + j < data.length; j++) out[i + j] = data[i + j] ^ ks[j];
    counter++;
  }
  return out;
}
function pkcs7(data: number[]): number[] {
  const n = BLK - (data.length % BLK);
  return [...data, ...new Array(n).fill(n)];
}
function seal(rk: number[], key: number[], plain: number[], nonce: number[]): number[] {
  const c = ctr(rk, plain, nonce);
  const body = [...nonce, ...c];
  return [...body, ...hmac(key, body)];
}
function open(rk: number[], key: number[], sealed: number[], nonce: number[]): { ok: boolean; pt: number[] } {
  const body = sealed.slice(0, sealed.length - HSIZE);
  const tag = sealed.slice(sealed.length - HSIZE);
  if (!eqBytes(tag, hmac(key, body))) return { ok: false, pt: [] };
  return { ok: true, pt: ctr(rk, body.slice(nonce.length), nonce) };
}

const B = (s: string): number[] => Array.from(s).map((c) => c.charCodeAt(0));
const hex = (b: number[]): string => b.map((x) => x.toString(16).padStart(2, "0")).join("");
const txt = (b: number[]): string => b.map((x) => (x >= 32 && x < 127 ? String.fromCharCode(x) : ".")).join("");

const KEY = B("my-secret-key-123");
const RK = expandKey(KEY);
const NONCE = B("nonce123");

const modes = [
  { key: "ecb", label: "ECBの模様漏れ" },
  { key: "ctr", label: "CTRの可鍛性" },
  { key: "aead", label: "Seal/Open(防御)" },
] as const;
const mode = ref<"ecb" | "ctr" | "aead">("ecb");

// --- ECB 漏洩 ---
const ecbData = computed(() => {
  const plain = B("SECRET__SECRET__SECRET__"); // 同じ 8 バイトを 3 回
  const e = ecb(RK, plain);
  const cb = cbc(RK, plain, B("iv-8byte"));
  const toBlocks = (b: number[]) => {
    const blocks: string[] = [];
    for (let i = 0; i < 24; i += BLK) blocks.push(hex(b.slice(i, i + BLK)));
    return blocks;
  };
  return { ecb: toBlocks(e), cbc: toBlocks(cb) };
});
// 同じ暗号文ブロックに同じ色を割り当てる。
const blockColors = computed(() => {
  const map = new Map<string, number>();
  let n = 0;
  const assign = (blocks: string[]) => blocks.map((b) => (map.has(b) ? map.get(b)! : (map.set(b, n), n++)));
  return { ecb: assign(ecbData.value.ecb), cbc: assign(ecbData.value.cbc) };
});

// --- CTR 可鍛性 / AEAD ---
const attack = computed(() => {
  const plain = B("balance=0000100"); // 送金額 100
  const pos = plain.indexOf("1".charCodeAt(0));
  if (mode.value === "aead") {
    const sealed = seal(RK, KEY, plain, NONCE);
    // 暗号文部分(nonce の後)を 1 バイト改ざん。
    const tampered = [...sealed];
    tampered[NONCE.length + pos] ^= "1".charCodeAt(0) ^ "9".charCodeAt(0);
    const r = open(RK, KEY, tampered, NONCE);
    return { plain, ctHex: hex(sealed), result: r.ok ? txt(r.pt) : "(復号せず拒否)", ok: r.ok };
  }
  const ct = ctr(RK, plain, NONCE);
  const tampered = [...ct];
  tampered[pos] ^= "1".charCodeAt(0) ^ "9".charCodeAt(0); // 鍵なしで書き換え
  const got = ctr(RK, tampered, NONCE);
  return { plain, ctHex: hex(ct), result: txt(got), ok: true };
});

const badge = computed(() => {
  if (mode.value === "ecb") return "同一ブロック=同一暗号文";
  if (mode.value === "ctr") return "改ざん成功";
  return "改ざん検知";
});
const badgeTone = computed<"ok" | "ng" | "neutral">(() =>
  mode.value === "ctr" ? "ng" : mode.value === "aead" ? "ok" : "neutral",
);

const note = computed(() => {
  if (mode.value === "ecb")
    return "同じ平文ブロック SECRET__ を 3 回並べた。ECB では暗号文も同じ 3 ブロック(同色)になり、繰り返し模様が透ける。CBC は前の暗号文を混ぜるので全ブロックがばらける";
  if (mode.value === "ctr")
    return "攻撃者は鍵を知らないのに、暗号文の1バイトを XOR で書き換えるだけで、復号結果を balance=0000100 → 0000900 に改変できた。復号は成功し、改ざんは検知されない";
  return "同じ改ざんを Seal/Open に対して試すと、HMAC の tag 検証に引っかかり、復号する前に拒否される。暗号化(秘匿)に認証を足して初めて安全になる";
});
</script>

<template>
  <DemoShell title="対称暗号とモード" :badge="badge" :badge-tone="badgeTone">
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

    <!-- ECB 漏洩 -->
    <div v-if="mode === 'ecb'" class="cp-panel">
      <div class="cp-plain mono">平文: "SECRET__" × 3</div>
      <div class="cp-mode-row">
        <span class="cp-mlbl mono">ECB</span>
        <span class="cp-blocks">
          <span
            v-for="(b, i) in ecbData.ecb"
            :key="i"
            class="cp-block mono"
            :class="'c' + blockColors.ecb[i]"
            >{{ b }}</span
          >
        </span>
      </div>
      <div class="cp-mode-row">
        <span class="cp-mlbl mono">CBC</span>
        <span class="cp-blocks">
          <span
            v-for="(b, i) in ecbData.cbc"
            :key="i"
            class="cp-block mono"
            :class="'c' + blockColors.cbc[i]"
            >{{ b }}</span
          >
        </span>
      </div>
      <div class="cp-hint">同じ色 = 同じ暗号文ブロック。ECB は 3 つとも同色 = 模様が漏れている</div>
    </div>

    <!-- CTR 可鍛性 / AEAD -->
    <div v-else class="cp-panel">
      <div class="cp-row"><span class="cp-k mono">平文</span><span class="cp-v mono">{{ txt(attack.plain) }}</span></div>
      <div class="cp-row"><span class="cp-k mono">暗号文</span><span class="cp-v mono small">{{ attack.ctHex }}</span></div>
      <div class="cp-arrow">攻撃者が暗号文の1バイトを鍵なしで書き換える(送金額 100 → 900 狙い)↓</div>
      <div class="cp-row">
        <span class="cp-k mono">復号結果</span>
        <span class="cp-v mono" :class="mode === 'ctr' ? 'bad' : 'good'">{{ attack.result }}</span>
      </div>
      <div class="cp-verdict" :class="attack.ok && mode === 'ctr' ? 'bad' : 'good'">
        {{ mode === "ctr" ? "改ざんが通った → 暗号化だけでは書き換えを防げない" : "改ざんを検知 → encrypt-then-MAC が守った" }}
      </div>
    </div>

    <p class="cp-note">{{ note }}</p>

    <p class="cp-legend">
      Go 実装をそのまま移植して計算している。ECB は同じ平文ブロックを同じ暗号文にするので模様が漏れる。
      CTR は平文と擬似乱数列の XOR なので、暗号文のビットを反転すると平文の同じ位置が反転する(可鍛性)。
      暗号化は「読めなくする」だけで「書き換えを防ぐ」ものではない。認証(HMAC)を足した AEAD で初めて改ざんを弾ける。
    </p>
  </DemoShell>
</template>

<style scoped>
.cp-panel {
  margin-top: 16px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 14px;
  background-color: var(--vp-c-bg-soft);
}
.cp-plain {
  font-size: 12px;
  color: var(--vp-c-text-2);
  margin-bottom: 12px;
}
.cp-mode-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}
.cp-mlbl {
  width: 34px;
  font-size: 12px;
  font-weight: 700;
  color: var(--vp-c-text-2);
}
.cp-blocks {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}
.cp-block {
  padding: 5px 7px;
  font-size: 11px;
  border-radius: 0;
  color: #fff;
}
.c0 { background-color: #4b5bd6; }
.c1 { background-color: #2f9e6f; }
.c2 { background-color: #c26b2f; }
.c3 { background-color: #a24bb0; }
.cp-hint,
.cp-arrow {
  margin-top: 10px;
  font-size: 11.5px;
  color: var(--vp-c-text-3);
}
.cp-arrow {
  text-align: center;
  margin: 10px 0;
}
.cp-row {
  display: flex;
  align-items: baseline;
  gap: 10px;
  padding: 3px 0;
}
.cp-k {
  width: 72px;
  flex-shrink: 0;
  font-size: 12px;
  font-weight: 700;
  color: var(--vp-c-text-2);
  text-align: right;
}
.cp-v {
  font-size: 13px;
  color: var(--vp-c-text-1);
  word-break: break-all;
}
.cp-v.small {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.cp-v.bad {
  color: var(--vp-c-danger-1);
  font-weight: 700;
}
.cp-v.good {
  color: var(--vp-c-green-1);
  font-weight: 700;
}
.cp-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid;
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
}
.cp-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.cp-verdict.good {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.cp-note {
  margin: 12px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.cp-legend {
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
