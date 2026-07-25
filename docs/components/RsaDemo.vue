<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// Go 版 crypto/rsa の JS ミラー。小さな素数で鍵生成→暗号化/復号、署名/検証。
function gcd(a: number, b: number): number {
  while (b) [a, b] = [b, a % b];
  return a;
}
function modExp(base: number, exp: number, m: number): number {
  if (m === 1) return 0;
  let r = 1;
  base %= m;
  while (exp > 0) {
    if (exp & 1) r = (r * base) % m;
    exp = Math.floor(exp / 2);
    base = (base * base) % m;
  }
  return r;
}
function modInverse(a: number, m: number): number {
  let [old_r, r] = [a, m];
  let [old_s, s] = [1, 0];
  while (r) {
    const q = Math.floor(old_r / r);
    [old_r, r] = [r, old_r - q * r];
    [old_s, s] = [s, old_s - q * s];
  }
  return ((old_s % m) + m) % m;
}

const p = 61;
const q = 53;
const n = p * q; // 3233
const phi = (p - 1) * (q - 1); // 3120
const e = (() => {
  let e = 3;
  while (gcd(e, phi) !== 1) e += 2;
  return e;
})();
const d = modInverse(e, phi);

const mode = ref<"encrypt" | "sign">("encrypt");
const message = ref(42);

const encStep = computed(() => {
  const c = modExp(message.value, e, n);
  const back = modExp(c, d, n);
  return { c, back };
});
const signStep = computed(() => {
  const s = modExp(message.value, d, n);
  const verified = modExp(s, e, n);
  return { s, verified };
});
</script>

<template>
  <DemoShell title="RSA(小さな素数)" :badge="`n=${n}`" badge-tone="neutral">
    <div class="sd-controls">
      <div class="sd-seg">
        <span class="sd-seg-opt" :class="{ on: mode === 'encrypt' }" @click="mode = 'encrypt'">暗号化</span>
        <span class="sd-seg-opt" :class="{ on: mode === 'sign' }" @click="mode = 'sign'">署名</span>
      </div>
      <label class="rs-msg">
        メッセージ m = <strong>{{ message }}</strong>
        <input v-model.number="message" type="range" min="0" :max="n - 1" step="1" />
      </label>
    </div>

    <div class="rs-keys">
      <span class="rs-key pub">公開鍵 (e={{ e }}, n={{ n }})</span>
      <span class="rs-key priv">秘密鍵 (d={{ d }}, n={{ n }})</span>
    </div>

    <div v-if="mode === 'encrypt'" class="rs-steps">
      <div class="rs-step">
        <span class="rs-op pub">公開鍵で暗号化</span>
        <code>c = {{ message }}<sup>{{ e }}</sup> mod {{ n }} = <b>{{ encStep.c }}</b></code>
      </div>
      <div class="rs-step">
        <span class="rs-op priv">秘密鍵で復号</span>
        <code>m = {{ encStep.c }}<sup>{{ d }}</sup> mod {{ n }} = <b>{{ encStep.back }}</b></code>
        <span class="rs-ok" :class="{ bad: encStep.back !== message }">
          {{ encStep.back === message ? "✓ 元に戻った" : "✗" }}
        </span>
      </div>
      <p class="rs-note">誰でも公開鍵で暗号化できるが、復号できるのは秘密鍵 d を持つ人だけ。</p>
    </div>

    <div v-else class="rs-steps">
      <div class="rs-step">
        <span class="rs-op priv">秘密鍵で署名</span>
        <code>s = {{ message }}<sup>{{ d }}</sup> mod {{ n }} = <b>{{ signStep.s }}</b></code>
      </div>
      <div class="rs-step">
        <span class="rs-op pub">公開鍵で検証</span>
        <code>s<sup>{{ e }}</sup> mod {{ n }} = <b>{{ signStep.verified }}</b></code>
        <span class="rs-ok" :class="{ bad: signStep.verified !== message }">
          {{ signStep.verified === message ? `✓ m(${message})と一致 = 本物` : "✗" }}
        </span>
      </div>
      <p class="rs-note">署名を作れるのは秘密鍵 d を持つ本人だけ。検証は公開鍵で誰でもできる。暗号化と鍵の向きが逆。</p>
    </div>
  </DemoShell>
</template>

<style scoped>
.rs-msg {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}
.rs-msg input {
  accent-color: var(--vp-c-brand-1);
}
.rs-keys {
  display: flex;
  gap: 8px;
  margin-top: 14px;
  flex-wrap: wrap;
}
.rs-key {
  padding: 3px 10px;
  border-radius: 6px;
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
}
.rs-key.pub {
  background-color: var(--vp-c-brand-soft);
  color: var(--vp-c-brand-1);
}
.rs-key.priv {
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
}
.rs-steps {
  margin-top: 14px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.rs-step {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.rs-op {
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 600;
  white-space: nowrap;
}
.rs-op.pub {
  background-color: var(--vp-c-brand-soft);
  color: var(--vp-c-brand-1);
}
.rs-op.priv {
  background-color: var(--vp-c-danger-soft);
  color: var(--vp-c-danger-1);
}
.rs-step code {
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
}
.rs-ok {
  font-size: 12px;
  font-weight: 600;
  color: var(--vp-c-green-1);
}
.rs-ok.bad {
  color: var(--vp-c-danger-1);
}
.rs-note {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--vp-c-text-2);
}
</style>
