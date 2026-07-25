<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// JWT の3パート構造と改竄検出を見せる。HMAC は簡易ハッシュで代用(構造の理解が目的)。
function b64url(s: string): string {
  return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

// 教材用の簡易 HMAC(実物は HMAC-SHA256)。secret + input を混ぜて短い署名にする。
function fakeHmac(input: string, secret: string): string {
  let h = 2166136261;
  const s = secret + "|" + input;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619) >>> 0;
  }
  return b64url(String.fromCharCode((h >> 24) & 255, (h >> 16) & 255, (h >> 8) & 255, h & 255));
}

const secret = ref("my-secret-key");
const sub = ref("user-42");
const tamperSub = ref("admin"); // 攻撃者が書き換えたい値

const header = { alg: "HS256", typ: "JWT" };
const headerB64 = computed(() => b64url(JSON.stringify(header)));

// 正規のトークン
const legit = computed(() => {
  const payload = b64url(JSON.stringify({ sub: sub.value, exp: 9999999999 }));
  const input = headerB64.value + "." + payload;
  return { header: headerB64.value, payload, sig: fakeHmac(input, secret.value) };
});

// 攻撃者がペイロードだけ書き換えたトークン(署名は元のまま = 秘密鍵を知らないので作れない)
const forged = computed(() => {
  const payload = b64url(JSON.stringify({ sub: tamperSub.value, exp: 9999999999 }));
  const input = headerB64.value + "." + payload;
  const recomputed = fakeHmac(input, secret.value); // サーバが検証時に計算する署名
  return { payload, oldSig: legit.value.sig, recomputed, valid: recomputed === legit.value.sig };
});
</script>

<template>
  <DemoShell title="JWT の署名と改竄検出" badge="HS256" badge-tone="neutral">
    <div class="sd-controls">
      <label class="jw-field">秘密鍵(サーバだけが持つ) <input v-model="secret" class="jw-input" spellcheck="false" /></label>
      <label class="jw-field">sub(誰のトークンか) <input v-model="sub" class="jw-input" spellcheck="false" /></label>
    </div>

    <p class="jw-label">サーバが発行する正規のトークン</p>
    <div class="jw-token">
      <span class="jw-part header">{{ legit.header }}</span>
      <span class="jw-dot">.</span>
      <span class="jw-part payload">{{ legit.payload }}</span>
      <span class="jw-dot">.</span>
      <span class="jw-part sig">{{ legit.sig }}</span>
    </div>
    <p class="jw-parts-legend">
      <span class="jw-tag header">header</span> アルゴリズム
      <span class="jw-tag payload">payload</span> sub={{ sub }}(誰でも読める)
      <span class="jw-tag sig">signature</span> 秘密鍵で計算(サーバだけ作れる)
    </p>

    <div class="jw-attack">
      <p class="jw-label">攻撃: ペイロードを書き換えて権限を奪おうとする</p>
      <label class="jw-field">
        書き換えた sub → <input v-model="tamperSub" class="jw-input" spellcheck="false" />
      </label>
      <div class="jw-token">
        <span class="jw-part header">{{ legit.header }}</span>
        <span class="jw-dot">.</span>
        <span class="jw-part payload tampered">{{ forged.payload }}</span>
        <span class="jw-dot">.</span>
        <span class="jw-part sig">{{ forged.oldSig }}</span>
      </div>
      <div class="jw-verify">
        <p>サーバが検証: 受け取ったヘッダ+ペイロードから署名を計算し直す</p>
        <div class="jw-cmp">
          <span>計算した署名 <code>{{ forged.recomputed }}</code></span>
          <span>トークンの署名 <code>{{ forged.oldSig }}</code></span>
        </div>
        <p class="jw-result" :class="forged.valid ? 'ok' : 'ng'">
          {{ forged.valid ? "✓ 一致(秘密鍵を知られている!)" : "✗ 不一致 → 改竄を検出、リクエスト拒否" }}
        </p>
      </div>
    </div>
  </DemoShell>
</template>

<style scoped>
.jw-field {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.jw-input {
  padding: 5px 8px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 6px;
  background-color: var(--vp-c-bg);
  color: var(--vp-c-text-1);
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
}
.jw-label {
  margin: 14px 0 6px;
  font-size: 12px;
  font-weight: 700;
  color: var(--vp-c-text-2);
}
.jw-token {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  padding: 8px 10px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 6px;
  background-color: var(--vp-c-bg);
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  word-break: break-all;
}
.jw-part {
  padding: 1px 3px;
  border-radius: 3px;
}
.jw-part.header {
  color: var(--vp-c-brand-1);
  background-color: var(--vp-c-brand-soft);
}
.jw-part.payload {
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.jw-part.payload.tampered {
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.jw-part.sig {
  color: var(--vp-c-text-2);
  background-color: var(--vp-c-default-soft);
}
.jw-dot {
  color: var(--vp-c-text-3);
  margin: 0 1px;
}
.jw-parts-legend {
  margin: 6px 0 0;
  font-size: 11px;
  color: var(--vp-c-text-3);
  display: flex;
  flex-wrap: wrap;
  gap: 4px 8px;
  align-items: center;
}
.jw-tag {
  padding: 0 6px;
  border-radius: 3px;
  font-weight: 700;
}
.jw-tag.header {
  color: var(--vp-c-brand-1);
  background-color: var(--vp-c-brand-soft);
}
.jw-tag.payload {
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.jw-tag.sig {
  color: var(--vp-c-text-2);
  background-color: var(--vp-c-default-soft);
}
.jw-attack {
  margin-top: 16px;
  padding-top: 12px;
  border-top: 1px dashed var(--vp-c-divider);
}
.jw-verify {
  margin-top: 10px;
  font-size: 12px;
}
.jw-verify > p {
  margin: 0 0 6px;
  color: var(--vp-c-text-2);
}
.jw-cmp {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.jw-cmp code {
  font-size: 12px;
}
.jw-result {
  margin: 8px 0 0 !important;
  font-weight: 600;
}
.jw-result.ok {
  color: var(--vp-c-danger-1);
}
.jw-result.ng {
  color: var(--vp-c-green-1);
}
</style>
