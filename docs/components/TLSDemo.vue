<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// crypto/tls(Go)の流れをブラウザで追う。DH 共有秘密とトランスクリプトの
// ダイジェストは実際に計算し、正常/MITM で署名検証の可否を見せる。

// --- DH(BigInt modexp) ---
const P = 2147483647n;
const G = 5n;
function modexp(base: bigint, exp: bigint, mod: bigint): bigint {
  let r = 1n;
  base %= mod;
  while (exp > 0n) {
    if (exp & 1n) r = (r * base) % mod;
    base = (base * base) % mod;
    exp >>= 1n;
  }
  return r;
}
const clientPriv = 60001n;
const serverPriv = 150007n;
const malloryPriv = 210013n;
const clientPub = modexp(G, clientPriv, P);
const serverPub = modexp(G, serverPriv, P);
const malloryPub = modexp(G, malloryPriv, P);

// --- hash(トランスクリプトのダイジェスト。crypto/hash 移植) ---
const IV = [0x67452301, 0xefcdab89, 0x98badcfe, 0x10325476];
const HC = [
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
    const m = (((a + (b ^ c ^ d)) >>> 0) + w[r % 4] + HC[r]) >>> 0;
    a = rotl(m, 7);
    [a, b, c, d] = [d, a, b, c];
  }
  return [(s[0] + a) >>> 0, (s[1] + b) >>> 0, (s[2] + c) >>> 0, (s[3] + d) >>> 0];
}
function hpad(n: number): number[] {
  const z = (16 - ((n + 1 + 8) % 16)) % 16;
  const p = new Array(1 + z + 8).fill(0);
  p[0] = 0x80;
  put32(p, 1 + z, (n * 8) >>> 0);
  return p;
}
function digestHex(str: string): string {
  const d = Array.from(str).map((c) => c.charCodeAt(0));
  let s = [...IV];
  const p = [...d, ...hpad(d.length)];
  for (let i = 0; i < p.length; i += 16) s = hcompress(s, p.slice(i, i + 16));
  const o = new Array(16).fill(0);
  for (let i = 0; i < 4; i++) put32(o, i * 4, s[i]);
  return o.map((x) => x.toString(16).padStart(2, "0")).join("");
}

const modes = [
  { key: "normal", label: "正常な接続" },
  { key: "mitm", label: "中間者(MITM)" },
] as const;
const mode = ref<"normal" | "mitm">("normal");
function setMode(m: "normal" | "mitm") {
  mode.value = m;
  at.value = 0;
}

// サーバが署名するのは自分の DH 公開鍵を含むトランスクリプト。
const signedDigest = computed(() => digestHex(`${clientPub}|client-random-01|${serverPub}`));
// クライアントが受け取った DH 公開鍵(MITM ではすり替えられている)。
const seenServerPub = computed(() => (mode.value === "mitm" ? malloryPub : serverPub));
const clientDigest = computed(() => digestHex(`${clientPub}|client-random-01|${seenServerPub.value}`));
const sigOk = computed(() => clientDigest.value === signedDigest.value);

// 共有鍵。正常: g^(client·server)。MITM: クライアントは Mallory と共有(g^(client·mallory))。
const sharedClient = computed(() => modexp(seenServerPub.value, clientPriv, P));
const sharedServer = computed(() => modexp(clientPub, serverPriv, P));

type Frame = { title: string; body: string; status: "info" | "ok" | "bad" | "final" };
const frames = computed<Frame[]>(() => {
  const isMitm = mode.value === "mitm";
  const f: Frame[] = [
    { title: "ClientHello 送信", body: `クライアントが一時DH公開鍵 ${clientPub} と乱数を送る`, status: "info" },
    {
      title: "ServerHello 受信",
      body: isMitm
        ? `Mallory がサーバのDH公開鍵を ${malloryPub} にすり替える。証明書と署名はサーバのものをそのまま中継`
        : `サーバがDH公開鍵 ${serverPub}・証明書(example.com)・署名を返す`,
      status: isMitm ? "bad" : "info",
    },
    { title: "検証① 証明書のCA署名", body: "証明書は信頼するCAが署名 → 有効。証明書自体は書き換えられていない", status: "ok" },
    { title: "検証② 主体名の一致", body: "証明書の主体名 example.com は接続先と一致 → OK", status: "ok" },
    {
      title: "検証③ ハンドシェイク署名",
      body: sigOk.value
        ? `署名対象のダイジェスト ${signedDigest.value.slice(0, 16)}… と、受け取ったDH公開鍵から再計算したダイジェストが一致 → 署名有効`
        : `サーバが署名したのは ${signedDigest.value.slice(0, 16)}…(本物のDH公開鍵)。だがクライアントが見たDH公開鍵から再計算すると ${clientDigest.value.slice(0, 16)}… で食い違う。Mallory はサーバの秘密鍵を持たず署名を作り直せない`,
      status: sigOk.value ? "ok" : "bad",
    },
  ];
  if (sigOk.value) {
    f.push({
      title: "セッション確立",
      body: `両者が同じ共有鍵 ${sharedServer.value} に到達。以後の本文はこの鍵でAEAD暗号化。秘匿・認証・完全性がそろった`,
      status: "final",
    });
  } else {
    f.push({
      title: "接続を拒否(ErrBadSig)",
      body: `署名が検証できないので接続を打ち切る。Mallory の握る鍵 ${sharedClient.value} は使われない。認証を足したことで中間者を弾けた`,
      status: "bad",
    });
  }
  return f;
});

const at = ref(0);
const cur = computed(() => frames.value[at.value]);
const canPrev = computed(() => at.value > 0);
const canNext = computed(() => at.value < frames.value.length - 1);
function first() { at.value = 0; }
function prev() { if (canPrev.value) at.value--; }
function next() { if (canNext.value) at.value++; }
function last() { at.value = frames.value.length - 1; }

const badge = computed(() => (mode.value === "normal" ? "接続成立" : sigOk.value ? "成立" : "中間者を拒否"));
const badgeTone = computed<"ok" | "ng" | "neutral">(() => (mode.value === "normal" ? "ok" : "ng"));
</script>

<template>
  <DemoShell title="TLSハンドシェイク" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <span class="sd-seg">
        <span
          v-for="m in modes"
          :key="m.key"
          class="sd-seg-opt"
          :class="{ on: mode === m.key }"
          @click="setMode(m.key)"
          >{{ m.label }}</span
        >
      </span>
    </div>

    <div class="tl-steps">
      <div
        v-for="(f, i) in frames"
        :key="i"
        class="tl-step"
        :class="[f.status, { on: i === at, dim: i > at }]"
      >
        <span class="tl-dot" :class="f.status"></span>
        <span class="tl-title">{{ f.title }}</span>
      </div>
    </div>

    <div class="tl-detail" :class="cur.status">
      <div class="tl-detail-title">{{ cur.title }}</div>
      <div class="tl-detail-body mono">{{ cur.body }}</div>
    </div>

    <div class="tl-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="tl-nav mono">{{ at + 1 }} / {{ frames.length }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">次へ進む</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="tl-legend">
      DH 共有鍵とトランスクリプトのダイジェストは実際に計算している。TLS は証明書で相手を認証してから
      鍵交換する。サーバは自分の DH 公開鍵に署名するので、中間者が公開鍵をすり替えても署名が合わず
      (③で食い違う)、秘密鍵を持たない Mallory は署名を作り直せない。だから接続は拒否され、鍵交換が
      中間者に耐える。認証・鍵交換・本文の AEAD がそろって初めて安全な通信路になる。
    </p>
  </DemoShell>
</template>

<style scoped>
.tl-steps {
  margin-top: 16px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.tl-step {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 10px;
  border-left: 3px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
}
.tl-step.dim {
  opacity: 0.4;
}
.tl-step.on {
  border-left-color: var(--vp-c-brand-1);
}
.tl-step.on.ok {
  border-left-color: var(--vp-c-green-1);
}
.tl-step.on.bad {
  border-left-color: var(--vp-c-danger-1);
}
.tl-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background-color: var(--vp-c-text-3);
  flex-shrink: 0;
}
.tl-dot.ok { background-color: var(--vp-c-green-1); }
.tl-dot.bad { background-color: var(--vp-c-danger-1); }
.tl-dot.final { background-color: var(--vp-c-brand-1); }
.tl-title {
  font-size: 12.5px;
  color: var(--vp-c-text-1);
}
.tl-detail {
  margin-top: 14px;
  padding: 12px 14px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
}
.tl-detail.ok { border-left-color: var(--vp-c-green-1); }
.tl-detail.bad { border-left-color: var(--vp-c-danger-1); }
.tl-detail.final { border-left-color: var(--vp-c-brand-1); }
.tl-detail-title {
  font-size: 12.5px;
  font-weight: 700;
  color: var(--vp-c-text-1);
  margin-bottom: 6px;
}
.tl-detail-body {
  font-size: 11.5px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  word-break: break-all;
}
.tl-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
  flex-wrap: wrap;
}
.tl-nav {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.tl-legend {
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
