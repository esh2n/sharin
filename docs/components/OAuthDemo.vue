<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// crypto/oauth(Go)の流れをブラウザで追う。PKCE の challenge は
// crypto/hash を移植して実際に計算し、正規/横取りで交換の可否を見せる。

// --- hash(Challenge 用。crypto/hash 移植) ---
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
function challenge(str: string): string {
  const d = Array.from(str).map((c) => c.charCodeAt(0));
  let s = [...IV];
  const p = [...d, ...hpad(d.length)];
  for (let i = 0; i < p.length; i += 16) s = hcompress(s, p.slice(i, i + 16));
  const o = new Array(16).fill(0);
  for (let i = 0; i < 4; i++) put32(o, i * 4, s[i]);
  return o.map((x) => x.toString(16).padStart(2, "0")).join("");
}

const VERIFIER = "s3cr3t-verifier-42";
const CHALLENGE = challenge(VERIFIER);
const ATTACKER_GUESS = "guessed-verifier";
const ATTACKER_CHALLENGE = challenge(ATTACKER_GUESS);

const modes = [
  { key: "normal", label: "正規のアプリ" },
  { key: "steal", label: "コード横取り" },
] as const;
const mode = ref<"normal" | "steal">("normal");
function setMode(m: "normal" | "steal") {
  mode.value = m;
  at.value = 0;
}

type Frame = { title: string; body: string; status: "info" | "ok" | "bad" | "final" };
const frames = computed<Frame[]>(() => {
  const steal = mode.value === "steal";
  const f: Frame[] = [
    {
      title: "検証子を生成し challenge を預ける",
      body: `アプリは検証子 "${VERIFIER}" を秘密に持ち、そのハッシュ ${CHALLENGE.slice(0, 16)}… だけを認可要求に添える`,
      status: "info",
    },
    { title: "① ユーザが認可サーバでログイン", body: "アプリにパスワードは渡らない。ユーザは認可サーバで直接ログインし、photo:read の許可を与える", status: "info" },
    {
      title: "② コードがフロントチャネルで返る",
      body: steal
        ? "短命なコード code_… がブラウザ経由で返る。だがこの経路を攻撃者が盗み見て、コードを横取りした"
        : "短命なコード code_… がブラウザ経由でアプリに返る。この時点ではトークンはまだ出ていない",
      status: steal ? "bad" : "info",
    },
    {
      title: steal ? "③ 攻撃者がコードを交換しようとする" : "③ コード+検証子をバックチャネルで送る",
      body: steal
        ? `攻撃者はコードは持つが検証子を知らない。推測した "${ATTACKER_GUESS}" で交換を試みる`
        : `アプリは直接通信で、コードと検証子の原本 "${VERIFIER}" を認可サーバへ送る`,
      status: steal ? "bad" : "info",
    },
    {
      title: "④ PKCE 検証",
      body: steal
        ? `攻撃者の検証子のハッシュ ${ATTACKER_CHALLENGE.slice(0, 16)}… は、預けた challenge ${CHALLENGE.slice(0, 16)}… と食い違う → ErrBadVerifier`
        : `検証子のハッシュ ${CHALLENGE.slice(0, 16)}… は預けた challenge と一致 → 本人と確認`,
      status: steal ? "bad" : "ok",
    },
  ];
  if (steal) {
    f.push({
      title: "交換を拒否",
      body: "コードを横取りしてもトークンは得られない。検証子はハッシュから逆算できず、原本を示せない。コードは無力なまま",
      status: "bad",
    });
  } else {
    f.push({
      title: "④ トークン発行 → ⑤ 資源アクセス",
      body: "scope=photo:read・期限つきのトークンが発行される。資源サーバは署名と scope を確認し、写真の読み取りだけを許可する",
      status: "final",
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

const badge = computed(() => (mode.value === "normal" ? "トークン発行" : "横取りを拒否"));
const badgeTone = computed<"ok" | "ng" | "neutral">(() => (mode.value === "normal" ? "ok" : "ng"));
</script>

<template>
  <DemoShell title="OAuth 認可コードフロー(PKCE)" :badge="badge" :badge-tone="badgeTone">
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

    <div class="oa-steps">
      <div
        v-for="(f, i) in frames"
        :key="i"
        class="oa-step"
        :class="[f.status, { on: i === at, dim: i > at }]"
      >
        <span class="oa-dot" :class="f.status"></span>
        <span class="oa-title">{{ f.title }}</span>
      </div>
    </div>

    <div class="oa-detail" :class="cur.status">
      <div class="oa-detail-title">{{ cur.title }}</div>
      <div class="oa-detail-body mono">{{ cur.body }}</div>
    </div>

    <div class="oa-foot">
      <button class="sd-btn" :disabled="!canPrev" @click="first">最初へ</button>
      <button class="sd-btn" :disabled="!canPrev" @click="prev">◀</button>
      <span class="oa-nav mono">{{ at + 1 }} / {{ frames.length }}</span>
      <button class="sd-btn" :disabled="!canNext" @click="next">▶</button>
      <button class="sd-btn sd-btn--primary" :disabled="!canNext" @click="next">次へ進む</button>
      <button class="sd-btn" :disabled="!canNext" @click="last">最後へ</button>
    </div>

    <p class="oa-legend">
      PKCE の challenge は実際にハッシュ計算している。OAuth はパスワードでなくトークンを委譲する。
      ブラウザ経由には短命のコードだけを流し、トークンは直接通信で交換するので、経路に残らない。
      PKCE は検証子のハッシュを先に預け、交換時に原本を示させる。コードを横取りしても検証子を
      知らなければ交換できない。範囲(scope)を絞ったトークンなら、漏れても被害はその範囲に留まる。
    </p>
  </DemoShell>
</template>

<style scoped>
.oa-steps {
  margin-top: 16px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.oa-step {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 10px;
  border-left: 3px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
}
.oa-step.dim {
  opacity: 0.4;
}
.oa-step.on {
  border-left-color: var(--vp-c-brand-1);
}
.oa-step.on.ok {
  border-left-color: var(--vp-c-green-1);
}
.oa-step.on.bad {
  border-left-color: var(--vp-c-danger-1);
}
.oa-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background-color: var(--vp-c-text-3);
  flex-shrink: 0;
}
.oa-dot.ok { background-color: var(--vp-c-green-1); }
.oa-dot.bad { background-color: var(--vp-c-danger-1); }
.oa-dot.final { background-color: var(--vp-c-brand-1); }
.oa-title {
  font-size: 12.5px;
  color: var(--vp-c-text-1);
}
.oa-detail {
  margin-top: 14px;
  padding: 12px 14px;
  border-left: 3px solid var(--vp-c-brand-1);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
}
.oa-detail.ok { border-left-color: var(--vp-c-green-1); }
.oa-detail.bad { border-left-color: var(--vp-c-danger-1); }
.oa-detail.final { border-left-color: var(--vp-c-brand-1); }
.oa-detail-title {
  font-size: 12.5px;
  font-weight: 700;
  color: var(--vp-c-text-1);
  margin-bottom: 6px;
}
.oa-detail-body {
  font-size: 11.5px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  word-break: break-all;
}
.oa-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
  flex-wrap: wrap;
}
.oa-nav {
  font-size: 12px;
  color: var(--vp-c-text-2);
  min-width: 52px;
  text-align: center;
}
.oa-legend {
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
