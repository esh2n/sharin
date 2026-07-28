<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// foundations/numbers(Go)を移植。表し方の違い、はみ出しの2種類、バイト順を見る。

type Kind = "twos" | "ones" | "signmag";
const KIND_NAMES: Record<Kind, string> = {
  twos: "2の補数",
  ones: "1の補数",
  signmag: "符号と絶対値",
};

const W = 4; // 全パターンを並べるので 4 ビット
const mask = (1 << W) - 1;
const signBit = 1 << (W - 1);

function decode(bits: number, kind: Kind): number {
  bits &= mask;
  const neg = (bits & signBit) !== 0;
  if (kind === "ones") return neg ? -(~bits & mask) : bits;
  if (kind === "signmag") return neg ? -(bits & ~signBit & mask) : bits;
  return neg ? bits - (1 << W) : bits;
}

const kind = ref<Kind>("twos");
const tab = ref<"repr" | "add" | "endian">("repr");

const rows = computed(() =>
  Array.from({ length: 1 << W }, (_, b) => ({
    bits: b.toString(2).padStart(W, "0"),
    twos: decode(b, "twos"),
    ones: decode(b, "ones"),
    signmag: decode(b, "signmag"),
  })),
);
const zeroCount = computed(() => rows.value.filter((r) => r[kind.value] === 0).length);
const range = computed(() => {
  const vs = rows.value.map((r) => r[kind.value]);
  return { min: Math.min(...vs), max: Math.max(...vs) };
});

// 加算(8ビット)
const a = ref(100);
const b = ref(100);
const M8 = 255;
const S8 = 128;
const enc8 = (v: number) => ((v % 256) + 256) % 256;
const add = computed(() => {
  const x = enc8(a.value);
  const y = enc8(b.value);
  const raw = x + y;
  const sum = raw & M8;
  const carry = raw > M8;
  const sameSign = ((x ^ y) & S8) === 0;
  const flipped = ((x ^ sum) & S8) !== 0;
  return { x, y, sum, carry, overflow: sameSign && flipped };
});
const asSigned = (v: number) => (v & S8 ? v - 256 : v);

// バイト順
const value = ref(0x12345678);
const hex = (n: number, w = 2) => n.toString(16).padStart(w, "0");
const le = computed(() => [0, 1, 2, 3].map((i) => (value.value >>> (8 * i)) & 0xff));
const be = computed(() => [3, 2, 1, 0].map((i) => (value.value >>> (8 * i)) & 0xff));
const misread = computed(() => be.value.reduce((v, x) => v * 256 + x, 0));

const badge = computed(() => {
  if (tab.value === "repr") return `${KIND_NAMES[kind.value]} ・ 0 は ${zeroCount.value} 通り`;
  if (tab.value === "add") return add.value.carry || add.value.overflow ? "はみ出している" : "はみ出していない";
  return "書いた順と読む順";
});
const badgeTone = computed<"ok" | "ng">(() => {
  if (tab.value === "repr") return zeroCount.value > 1 ? "ng" : "ok";
  if (tab.value === "add") return add.value.carry || add.value.overflow ? "ng" : "ok";
  return "ng";
});
</script>

<template>
  <DemoShell title="2の補数とバイト順" :badge="badge" :badge-tone="badgeTone">
    <div class="nm-tabs">
      <button class="sd-btn" :class="tab === 'repr' ? 'sd-btn--primary' : ''" @click="tab = 'repr'">表し方</button>
      <button class="sd-btn" :class="tab === 'add' ? 'sd-btn--primary' : ''" @click="tab = 'add'">
        はみ出しの2種類
      </button>
      <button class="sd-btn" :class="tab === 'endian' ? 'sd-btn--primary' : ''" @click="tab = 'endian'">
        バイト順
      </button>
    </div>

    <!-- 表し方 -->
    <div v-if="tab === 'repr'" class="nm-panel">
      <div class="nm-ctl">
        <button
          v-for="(n, k) in KIND_NAMES"
          :key="k"
          class="sd-btn"
          :class="kind === k ? 'sd-btn--primary' : ''"
          @click="kind = k as Kind"
        >
          {{ n }}
        </button>
        <span class="nm-note mono">4ビットの全16通り ・ 範囲 {{ range.min }}..{{ range.max }}</span>
      </div>
      <div class="nm-grid">
        <div
          v-for="r in rows"
          :key="r.bits"
          class="nm-cell mono"
          :class="[r[kind] === 0 ? 'zero' : '', r[kind] < 0 ? 'neg' : '']"
        >
          <span class="nm-bits">{{ r.bits }}</span>
          <span class="nm-val">{{ r[kind] }}</span>
        </div>
      </div>
      <div class="nm-verdict" :class="zeroCount > 1 ? 'bad' : 'ok'">
        <template v-if="zeroCount > 1">
          0 が {{ zeroCount }} 通りある。同じ数なのにビットが違うので、比べるたびに特別扱いが要る。
          そのぶん負の側が1つ少なく、範囲は {{ range.min }}..{{ range.max }} で対称になっている
        </template>
        <template v-else>
          0 は1通りだけ。そのぶん負の側が1つ多く、範囲は {{ range.min }}..{{ range.max }} で非対称になる。
          いちばん小さい {{ range.min }} は符号を反転できない
        </template>
      </div>
    </div>

    <!-- 加算 -->
    <div v-else-if="tab === 'add'" class="nm-panel">
      <div class="nm-ctl">
        <span class="nm-note mono">a</span>
        <input v-model.number="a" type="range" min="0" max="255" class="nm-range" />
        <span class="nm-note mono">{{ add.x }}(符号つきなら {{ asSigned(add.x) }})</span>
      </div>
      <div class="nm-ctl">
        <span class="nm-note mono">b</span>
        <input v-model.number="b" type="range" min="0" max="255" class="nm-range" />
        <span class="nm-note mono">{{ add.y }}(符号つきなら {{ asSigned(add.y) }})</span>
      </div>
      <div class="nm-flags">
        <div class="nm-flag" :class="add.carry ? 'bad' : 'ok'">
          <em>符号なしとして</em>
          <b>{{ add.x }} + {{ add.y }} = {{ add.carry ? add.x + add.y : add.sum }}</b>
          <span>{{ add.carry ? `繰り上がりが出た。8ビットには ${add.sum} しか残らない` : "収まっている" }}</span>
        </div>
        <div class="nm-flag" :class="add.overflow ? 'bad' : 'ok'">
          <em>符号つきとして</em>
          <b>{{ asSigned(add.x) }} + {{ asSigned(add.y) }} = {{ asSigned(add.sum) }}</b>
          <span>{{ add.overflow ? "符号が反転した。同じ符号どうしを足したのに" : "符号は壊れていない" }}</span>
        </div>
      </div>
      <div class="nm-verdict" :class="add.carry !== add.overflow ? 'bad' : 'ok'">
        <template v-if="add.carry !== add.overflow">
          同じ計算なのに、片方だけが壊れている。ビット列は1つで、どちらの旗を見るかだけが違う
        </template>
        <template v-else-if="add.carry"> どちらの見方でも壊れている </template>
        <template v-else> どちらの見方でも収まっている。a と b を動かすと、片方だけ壊れる場面が出る </template>
      </div>
    </div>

    <!-- バイト順 -->
    <div v-else class="nm-panel">
      <div class="nm-ctl">
        <span class="nm-note mono">32ビットの値</span>
        <input v-model.number="value" type="range" min="0" max="4294967295" step="16843009" class="nm-range" />
        <span class="nm-note mono">0x{{ hex(value >>> 0, 8) }}</span>
      </div>
      <div class="nm-bytes">
        <div class="nm-brow">
          <span class="nm-bl mono">下位から並べる</span>
          <span v-for="(x, i) in le" :key="'l' + i" class="nm-byte mono" :class="i === 0 ? 'first' : ''">
            {{ hex(x) }}
          </span>
          <span class="nm-bnote mono">先頭1バイト = 下位8ビット</span>
        </div>
        <div class="nm-brow">
          <span class="nm-bl mono">上位から並べる</span>
          <span v-for="(x, i) in be" :key="'b' + i" class="nm-byte mono" :class="i === 0 ? 'first' : ''">
            {{ hex(x) }}
          </span>
          <span class="nm-bnote mono">先頭1バイト = 上位8ビット</span>
        </div>
      </div>
      <div class="nm-verdict bad">
        下位から並べたものを上位から読むと 0x{{ hex(misread >>> 0, 8) }} になる。長さも形も正しいので、
        壊れたことに気づかないまま先へ進んでしまう
      </div>
    </div>

    <p class="nm-legend">
      「表し方」では、4ビットの全16通りを並べている。表し方を切り替えると、0 が2つある行が現れたり消えたりする。
      「はみ出しの2種類」では、同じ加算に対して2つの旗が別々に立つ。片方だけ立つ組み合わせがあることが、
      符号つきと符号なしが別物である証拠になる。「バイト順」は、書いた順と読む順が食い違うと何が起きるかを見る。
    </p>
  </DemoShell>
</template>

<style scoped>
.nm-tabs {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.nm-panel {
  margin-top: 14px;
}
.nm-ctl {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}
.nm-note {
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.nm-range {
  width: 180px;
}
.nm-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 4px;
}
.nm-cell {
  display: flex;
  justify-content: space-between;
  gap: 6px;
  font-size: 10.5px;
  padding: 4px 8px;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
  color: var(--vp-c-text-2);
}
.nm-cell.neg {
  color: var(--vp-c-text-1);
  background-color: var(--vp-c-bg-alt);
}
.nm-cell.zero {
  border-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
  font-weight: 700;
}
.nm-val {
  font-variant-numeric: tabular-nums;
}
.nm-flags {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}
.nm-flag {
  flex: 1;
  min-width: 210px;
  display: flex;
  flex-direction: column;
  gap: 3px;
  border: 1px solid var(--vp-c-divider);
  padding: 9px 12px;
  background-color: var(--vp-c-bg-soft);
}
.nm-flag em {
  font-style: normal;
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.nm-flag b {
  font-family: var(--vp-font-family-mono);
  font-size: 14px;
}
.nm-flag span:last-child {
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.nm-flag.ok {
  border-color: var(--vp-c-green-1);
}
.nm-flag.ok b {
  color: var(--vp-c-green-1);
}
.nm-flag.bad {
  border-color: var(--vp-c-danger-1);
}
.nm-flag.bad b {
  color: var(--vp-c-danger-1);
}
.nm-bytes {
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
}
.nm-brow {
  display: flex;
  align-items: center;
  gap: 5px;
  margin-bottom: 6px;
  flex-wrap: wrap;
}
.nm-bl {
  width: 104px;
  flex: none;
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.nm-byte {
  font-size: 12px;
  padding: 3px 9px;
  border: 1px solid var(--vp-c-divider);
  color: var(--vp-c-text-2);
}
.nm-byte.first {
  border-color: var(--vp-c-brand-1);
  color: var(--vp-c-brand-1);
  background-color: var(--vp-c-brand-soft);
}
.nm-bnote {
  font-size: 9.5px;
  color: var(--vp-c-text-3);
  margin-left: 6px;
}
.nm-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  font-weight: 600;
  line-height: 1.6;
  background-color: var(--vp-c-bg-soft);
}
.nm-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.nm-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.nm-legend {
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
