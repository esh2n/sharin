<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// crypto/dh(Go)をブラウザに移植。剰余べき乗を BigInt で実際に計算し、
// Alice/Bob が同じ共有秘密に着くこと、MITM ではそれが崩れることを見せる。

const P = 2147483647n; // 2^31-1(素数)
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

// 読みやすい固定の秘密鍵(実物は無作為な巨大値)。
const a = 60001n; // Alice
const b = 150007n; // Bob
const m = 210013n; // Mallory

const A = computed(() => modexp(G, a, P)); // Alice 公開鍵
const B = computed(() => modexp(G, b, P)); // Bob 公開鍵
const M = computed(() => modexp(G, m, P)); // Mallory 公開鍵

const modes = [
  { key: "normal", label: "正常な鍵交換" },
  { key: "mitm", label: "中間者(MITM)" },
] as const;
const mode = ref<"normal" | "mitm">("normal");

const s = (x: bigint) => x.toString();

// 正常: Alice は B^a、Bob は A^b。
const normal = computed(() => {
  const aliceKey = modexp(B.value, a, P);
  const bobKey = modexp(A.value, b, P);
  return { aliceKey, bobKey, match: aliceKey === bobKey };
});

// MITM: Mallory が M を両者に渡す。
const mitm = computed(() => {
  const aliceKey = modexp(M.value, a, P); // Alice は M を Bob だと誤解
  const bobKey = modexp(M.value, b, P); // Bob は M を Alice だと誤解
  const malloryWithAlice = modexp(A.value, m, P);
  const malloryWithBob = modexp(B.value, m, P);
  return {
    aliceKey,
    bobKey,
    malloryWithAlice,
    malloryWithBob,
    aliceCaught: aliceKey === malloryWithAlice,
    bobCaught: bobKey === malloryWithBob,
    match: aliceKey === bobKey,
  };
});

const badge = computed(() =>
  mode.value === "normal" ? (normal.value.match ? "共有秘密が一致" : "不一致") : "Malloryが両方を握る",
);
const badgeTone = computed<"ok" | "ng" | "neutral">(() => (mode.value === "normal" ? "ok" : "ng"));

const note = computed(() => {
  if (mode.value === "normal")
    return "Alice は Bob の公開鍵を自分の秘密でべき乗し(B^a)、Bob は A^b を計算する。どちらも g^(ab) mod p に着き、共有秘密が一致する。この鍵は一度も通信路を流れていない";
  return "Mallory が A・B を横取りし、自分の M を両者に渡す。Alice は M を Bob だと信じて鍵を作り、Bob も同様。Mallory は両方の鍵を握れる。Alice と Bob の鍵は食い違うが、認証がないので二人は気づけない";
});
</script>

<template>
  <DemoShell title="Diffie–Hellman 鍵交換" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <span class="sd-seg">
        <span
          v-for="mm in modes"
          :key="mm.key"
          class="sd-seg-opt"
          :class="{ on: mode === mm.key }"
          @click="mode = mm.key"
          >{{ mm.label }}</span
        >
      </span>
    </div>

    <div class="dh-params mono">公開パラメータ: p = {{ s(P) }}, g = {{ s(G) }}</div>

    <!-- 正常 -->
    <div v-if="mode === 'normal'" class="dh-panel">
      <div class="dh-cols">
        <div class="dh-party">
          <div class="dh-name">Alice</div>
          <div class="dh-line mono">秘密 a = {{ s(a) }}</div>
          <div class="dh-line mono">公開 A = g^a = {{ s(A) }}</div>
          <div class="dh-line mono key">共有 B^a = {{ s(normal.aliceKey) }}</div>
        </div>
        <div class="dh-party">
          <div class="dh-name">Bob</div>
          <div class="dh-line mono">秘密 b = {{ s(b) }}</div>
          <div class="dh-line mono">公開 B = g^b = {{ s(B) }}</div>
          <div class="dh-line mono key">共有 A^b = {{ s(normal.bobKey) }}</div>
        </div>
      </div>
      <div class="dh-verdict good">
        両者の共有秘密が一致 → g^(ab) mod p。この鍵は通信路を流れていない
      </div>
    </div>

    <!-- MITM -->
    <div v-else class="dh-panel">
      <div class="dh-cols3">
        <div class="dh-party">
          <div class="dh-name">Alice</div>
          <div class="dh-line mono">秘密 a = {{ s(a) }}</div>
          <div class="dh-line mono small">M を Bob と誤解</div>
          <div class="dh-line mono key">鍵 M^a = {{ s(mitm.aliceKey) }}</div>
        </div>
        <div class="dh-party mal">
          <div class="dh-name">Mallory(中間者)</div>
          <div class="dh-line mono">秘密 m = {{ s(m) }}</div>
          <div class="dh-line mono small">A↔Mで {{ s(mitm.malloryWithAlice) }}</div>
          <div class="dh-line mono small">B↔Mで {{ s(mitm.malloryWithBob) }}</div>
        </div>
        <div class="dh-party">
          <div class="dh-name">Bob</div>
          <div class="dh-line mono">秘密 b = {{ s(b) }}</div>
          <div class="dh-line mono small">M を Alice と誤解</div>
          <div class="dh-line mono key">鍵 M^b = {{ s(mitm.bobKey) }}</div>
        </div>
      </div>
      <div class="dh-verdict bad">
        Alice の鍵は Mallory と一致({{ mitm.aliceCaught ? "○" : "×" }})、Bob の鍵も Mallory と一致({{
          mitm.bobCaught ? "○" : "×"
        }})。Alice と Bob 自身は食い違う。認証がないので二人は気づけない
      </div>
    </div>

    <p class="dh-note">{{ note }}</p>

    <p class="dh-legend">
      剰余べき乗を BigInt で実際に計算している。g^a mod p は速く計算できるが、結果から a を逆算する
      (離散対数)のは p が大きいと解けない。この一方向性で、秘密を送らずに共有秘密を作る。ただし
      素の DH は相手が本物かを確かめない。中間者には無力で、証明書などの認証と組み合わせて初めて安全になる。
    </p>
  </DemoShell>
</template>

<style scoped>
.dh-params {
  margin-top: 14px;
  font-size: 12px;
  color: var(--vp-c-text-3);
}
.dh-panel {
  margin-top: 10px;
}
.dh-cols {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}
.dh-cols3 {
  display: grid;
  grid-template-columns: 1fr 1.1fr 1fr;
  gap: 10px;
}
.dh-party {
  border: 1px solid var(--vp-c-divider);
  border-radius: 0;
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
}
.dh-party.mal {
  border-color: var(--vp-c-danger-1);
}
.dh-name {
  font-size: 12px;
  font-weight: 700;
  color: var(--vp-c-text-1);
  margin-bottom: 6px;
}
.dh-line {
  font-size: 11.5px;
  color: var(--vp-c-text-2);
  padding: 2px 0;
  word-break: break-all;
}
.dh-line.small {
  font-size: 10.5px;
  color: var(--vp-c-text-3);
}
.dh-line.key {
  color: var(--vp-c-brand-1);
  font-weight: 700;
}
.dh-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid;
  border-radius: 0;
  font-size: 12.5px;
  font-weight: 600;
  line-height: 1.6;
}
.dh-verdict.good {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.dh-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.dh-note {
  margin: 12px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--vp-c-text-2);
  min-height: 44px;
}
.dh-legend {
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
