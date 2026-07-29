<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// Go 版 dns の BuildQuery を JS で再現し、バイト列を色分けして見せる。
const name = ref("example.com");
const id = 0x1234;

interface Seg {
  label: string;
  bytes: number[];
  cls: "header" | "name" | "type";
  note: string;
}

const segments = computed<Seg[]>(() => {
  const header = [
    (id >> 8) & 0xff,
    id & 0xff, // ID
    0x01,
    0x00, // フラグ RD=1
    0x00,
    0x01, // QDCOUNT=1
    0x00,
    0x00,
    0x00,
    0x00,
    0x00,
    0x00, // AN/NS/AR = 0
  ];
  const nameBytes: number[] = [];
  for (const label of name.value.split(".")) {
    nameBytes.push(label.length);
    for (const ch of label) nameBytes.push(ch.charCodeAt(0));
  }
  nameBytes.push(0);
  return [
    { label: "ヘッダ (12B)", bytes: header, cls: "header", note: "ID, フラグ, 各セクションの件数" },
    { label: "名前", bytes: nameBytes, cls: "name", note: "[長さ][ラベル]... を 0 で終端" },
    { label: "QTYPE/QCLASS", bytes: [0, 1, 0, 1], cls: "type", note: "A レコード / IN(インターネット)" },
  ];
});

function toHex(b: number) {
  return b.toString(16).padStart(2, "0");
}
function toChar(b: number) {
  return b >= 33 && b <= 126 ? String.fromCharCode(b) : "·";
}

const totalBytes = computed(() => segments.value.reduce((a, s) => a + s.bytes.length, 0));

// Go 版 size.go と同じ計算。512 バイトに A レコードが何件入るか。
const UDP_LIMIT = 512;
const nameBytes = computed(() => segments.value[1].bytes.length);
const answerOn = 16; // 名前をポインタで指すと、名前の長さによらず 16 バイト
const answerOff = computed(() => nameBytes.value + 14);
const capOn = computed(() => Math.floor((UDP_LIMIT - totalBytes.value) / answerOn));
const capOff = computed(() => Math.floor((UDP_LIMIT - totalBytes.value) / answerOff.value));
const caps = computed(() => [
  { label: "名前を指す(圧縮あり)", per: answerOn, n: capOn.value, hot: true },
  { label: "名前を書き直す", per: answerOff.value, n: capOff.value, hot: false },
]);
const maxCap = computed(() => Math.max(capOn.value, capOff.value, 1));
</script>

<template>
  <DemoShell title="DNS 問い合わせメッセージ" :badge="`${totalBytes} バイト`" badge-tone="neutral">
    <div class="sd-controls">
      <span class="dn-caption">ドメイン名:</span>
      <input v-model="name" class="dn-input" spellcheck="false" />
      <span class="spacer"></span>
      <span class="dn-hint">これを UDP で 8.8.8.8:53 に送る</span>
    </div>

    <div class="dn-segments">
      <div v-for="seg in segments" :key="seg.label" class="dn-seg">
        <div class="dn-seg-head">
          <span class="dn-seg-name" :class="seg.cls">{{ seg.label }}</span>
          <span class="dn-seg-note">{{ seg.note }}</span>
        </div>
        <div class="dn-bytes">
          <span v-for="(b, i) in seg.bytes" :key="i" class="dn-byte" :class="seg.cls">
            <span class="dn-hex">{{ toHex(b) }}</span>
            <span class="dn-ascii">{{ toChar(b) }}</span>
          </span>
        </div>
      </div>
    </div>

    <div class="dn-cap">
      <div class="dn-cap-head">
        この問い合わせに対する応答が 512 バイトに収まる件数(A レコード)
      </div>
      <div v-for="c in caps" :key="c.label" class="dn-cap-row">
        <span class="dn-cap-label">{{ c.label }}</span>
        <span class="dn-cap-bar">
          <span
            class="dn-cap-fill"
            :class="{ hot: c.hot }"
            :style="{ width: (c.n / maxCap) * 100 + '%' }"
          ></span>
        </span>
        <span class="dn-cap-num mono">
          <b>{{ c.n }}</b> 件 <span class="dn-cap-per">/ 1件 {{ c.per }}B</span>
        </span>
      </div>
      <p class="dn-cap-note">
        名前を長くすると差が開く。指すほうは 1 件 16 バイトのまま動かないので、減るのは質問が太ったぶんだけ。書き直すほうは、名前がそのままレコードの大きさになる。
      </p>
    </div>
  </DemoShell>
</template>

<style scoped>
.dn-caption {
  font-size: 12px;
  font-weight: 600;
  color: var(--vp-c-text-2);
}
.dn-input {
  flex: 0 1 200px;
  padding: 6px 10px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 6px;
  background-color: var(--vp-c-bg);
  color: var(--vp-c-text-1);
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
}
.dn-hint {
  font-size: 12px;
  color: var(--vp-c-text-3);
}
.dn-segments {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-top: 14px;
}
.dn-seg-head {
  display: flex;
  align-items: baseline;
  gap: 10px;
  margin-bottom: 5px;
}
.dn-seg-name {
  padding: 1px 8px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 700;
}
.dn-seg-name.header {
  background-color: var(--vp-c-brand-soft);
  color: var(--vp-c-brand-1);
}
.dn-seg-name.name {
  background-color: var(--vp-c-green-soft);
  color: var(--vp-c-green-1);
}
.dn-seg-name.type {
  background-color: var(--vp-c-yellow-soft);
  color: var(--vp-c-yellow-1);
}
.dn-seg-note {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.dn-bytes {
  display: flex;
  flex-wrap: wrap;
  gap: 3px;
}
.dn-byte {
  display: flex;
  flex-direction: column;
  align-items: center;
  min-width: 24px;
  padding: 2px 0;
  border-radius: 4px;
  font-family: var(--vp-font-family-mono);
}
.dn-byte.header {
  background-color: color-mix(in srgb, var(--vp-c-brand-1) 12%, transparent);
}
.dn-byte.name {
  background-color: color-mix(in srgb, var(--vp-c-green-1) 14%, transparent);
}
.dn-byte.type {
  background-color: color-mix(in srgb, var(--vp-c-yellow-1) 16%, transparent);
}
.dn-cap {
  margin-top: 16px;
  padding-top: 12px;
  border-top: 1px solid var(--vp-c-divider);
}
.dn-cap-head {
  font-size: 12px;
  font-weight: 600;
  color: var(--vp-c-text-2);
  margin-bottom: 8px;
}
.dn-cap-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 3px 0;
}
.dn-cap-label {
  width: 168px;
  flex: none;
  font-size: 11.5px;
  color: var(--vp-c-text-2);
}
.dn-cap-bar {
  flex: 1 1 auto;
  height: 10px;
  background-color: var(--vp-c-default-soft);
}
.dn-cap-fill {
  display: block;
  height: 100%;
  background-color: var(--vp-c-text-3);
}
.dn-cap-fill.hot {
  background-color: var(--vp-c-brand-1);
}
.dn-cap-num {
  width: 108px;
  flex: none;
  text-align: right;
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.dn-cap-num b {
  font-size: 13px;
  color: var(--vp-c-text-1);
}
.dn-cap-per {
  font-size: 10px;
}
.dn-cap-note {
  margin: 10px 0 0;
  font-size: 11.5px;
  line-height: 1.7;
  color: var(--vp-c-text-3);
}
.mono {
  font-family: var(--vp-font-family-mono);
}
.dn-hex {
  font-size: 12px;
  color: var(--vp-c-text-1);
}
.dn-ascii {
  font-size: 9px;
  color: var(--vp-c-text-3);
}
</style>
