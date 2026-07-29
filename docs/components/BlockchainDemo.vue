<script setup lang="ts">
import { ref } from "vue";
import DemoShell from "./DemoShell.vue";

// ブロックチェーンの改竄検出を可視化。SHA-256 は Web Crypto を使う。
interface Block {
  index: number;
  data: string;
  prevHash: string;
  nonce: number;
  hash: string;
}

const DIFFICULTY = 2;
const prefix = "0".repeat(DIFFICULTY);
const blocks = ref<Block[]>([]);
const mining = ref(false);

async function sha256(s: string): Promise<string> {
  const buf = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(s));
  return [...new Uint8Array(buf)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

async function computeHash(b: Block): Promise<string> {
  return sha256(`${b.index}${b.data}${b.prevHash}${b.nonce}`);
}

// mine は条件を満たす nonce を総当たりし、試した回数を返す。
async function mine(b: Block): Promise<number> {
  for (b.nonce = 0; ; b.nonce++) {
    const h = await computeHash(b);
    if (h.startsWith(prefix)) {
      b.hash = h;
      return b.nonce + 1;
    }
    if (b.nonce > 200000) {
      b.hash = h;
      return b.nonce + 1;
    } // 安全弁
  }
}

async function init() {
  mining.value = true;
  const genesis: Block = { index: 0, data: "genesis", prevHash: "", nonce: 0, hash: "" };
  await mine(genesis);
  const chain = [genesis];
  // 最初から数ブロック積んでおく。1つでは「後ろが壊れる」が見えない。
  for (let i = 1; i <= 3; i++) {
    const prev = chain[chain.length - 1];
    const b: Block = { index: i, data: `tx-${i}`, prevHash: prev.hash, nonce: 0, hash: "" };
    await mine(b);
    chain.push(b);
  }
  blocks.value = chain;
  broken.value = {};
  lastRepair.value = null;
  mining.value = false;
}

async function add() {
  mining.value = true;
  const prev = blocks.value[blocks.value.length - 1];
  const b: Block = { index: prev.index + 1, data: `tx-${prev.index + 1}`, prevHash: prev.hash, nonce: 0, hash: "" };
  await mine(b);
  blocks.value = [...blocks.value, b];
  mining.value = false;
}

// 各ブロックの「壊れているか」を判定(表示用)。改竄すると true になる。
const broken = ref<Record<number, boolean>>({});
async function recheck() {
  const result: Record<number, boolean> = {};
  for (let i = 0; i < blocks.value.length; i++) {
    const b = blocks.value[i];
    const validHash = (await computeHash(b)) === b.hash && b.hash.startsWith(prefix);
    const linked = i === 0 || b.prevHash === blocks.value[i - 1].hash;
    result[b.index] = !(validHash && linked);
  }
  broken.value = result;
}

async function tamper(idx: number) {
  const b = blocks.value.find((x) => x.index === idx);
  if (b) {
    b.data = "改竄!";
    blocks.value = [...blocks.value]; // 再描画
    await recheck();
  }
}

// 直した内訳。何個を作り直して、ハッシュを何回試したか。
const lastRepair = ref<{ blocks: number; tries: number; whole: boolean } | null>(null);

async function remineBlock(idx: number) {
  mining.value = true;
  const b = blocks.value.find((x) => x.index === idx);
  if (b) {
    const tries = await mine(b);
    blocks.value = [...blocks.value];
    await recheck();
    lastRepair.value = { blocks: 1, tries, whole: false };
  }
  mining.value = false;
}

// Go 版 Repair と同じ。改竄したところから末尾まで、繋ぎ直しながら作り直す。
async function repairFrom(idx: number) {
  mining.value = true;
  let tries = 0;
  let count = 0;
  for (let i = idx; i < blocks.value.length; i++) {
    const b = blocks.value[i];
    if (i > 0) b.prevHash = blocks.value[i - 1].hash;
    tries += await mine(b);
    count++;
  }
  blocks.value = [...blocks.value];
  await recheck();
  lastRepair.value = { blocks: count, tries, whole: true };
  mining.value = false;
}

init();
</script>

<template>
  <DemoShell title="ブロックチェーンと改竄" :badge="`難易度 ${DIFFICULTY}`" badge-tone="neutral">
    <div class="sd-controls">
      <button class="sd-btn sd-btn--primary" type="button" :disabled="mining" @click="add">
        ブロックを追加(マイニング)
      </button>
      <span class="spacer"></span>
      <span v-if="mining" class="bc-mining">マイニング中…</span>
      <button class="sd-btn" type="button" :disabled="mining" @click="init">リセット</button>
    </div>

    <div class="bc-chain">
      <template v-for="(b, i) in blocks" :key="b.index">
        <div v-if="i > 0" class="bc-link" :class="{ broken: broken[b.index] }">↓ prevHash</div>
        <div class="bc-block" :class="{ broken: broken[b.index] }">
          <div class="bc-head">
            <span class="bc-index">#{{ b.index }}</span>
            <span class="bc-data">{{ b.data }}</span>
            <span class="bc-nonce">nonce {{ b.nonce }}</span>
          </div>
          <div class="bc-hashes">
            <span class="bc-h">prev: {{ b.prevHash ? b.prevHash.slice(0, 12) + "…" : "(なし)" }}</span>
            <span class="bc-h hash">hash: {{ b.hash.slice(0, 12) }}…</span>
          </div>
          <div class="bc-actions">
            <button class="bc-mini" type="button" :disabled="mining" @click="tamper(b.index)">中身を改竄</button>
            <button v-if="broken[b.index]" class="bc-mini remine" type="button" :disabled="mining" @click="remineBlock(b.index)">
              この1つだけ作り直す
            </button>
            <button v-if="broken[b.index]" class="bc-mini remine" type="button" :disabled="mining" @click="repairFrom(b.index)">
              ここから後ろを全部作り直す
            </button>
          </div>
        </div>
      </template>
    </div>

    <p v-if="lastRepair" class="bc-cost">
      <b>{{ lastRepair.blocks }}</b> 個を作り直すのに、ハッシュを <b>{{ lastRepair.tries }}</b> 回試した。
      <span v-if="!lastRepair.whole">1つ直しても、後ろの prevHash は古いままなので鎖はまだ壊れている。</span>
    </p>

    <p class="bc-note">
      過去のブロックを改竄すると、そのハッシュが変わって後続の prevHash と食い違い、鎖が壊れる(赤)。
      隠すには改竄したところから末尾まで全部を作り直すしかない。値段は「難易度あたりの試行回数 ×
      後ろに積まれたブロック数」で決まる。
    </p>
  </DemoShell>
</template>

<style scoped>
.bc-cost {
  margin: 12px 0 0;
  padding: 8px 12px;
  background-color: var(--vp-c-bg-soft);
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--vp-c-text-1);
}
.bc-cost b {
  font-family: var(--vp-font-family-mono);
  color: var(--vp-c-brand-1);
}
.bc-mining {
  font-size: 12px;
  color: var(--vp-c-brand-1);
}
.bc-chain {
  margin-top: 14px;
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 2px;
}
.bc-link {
  text-align: center;
  font-size: 11px;
  color: var(--vp-c-text-3);
  padding: 2px 0;
}
.bc-link.broken {
  color: var(--vp-c-danger-1);
  font-weight: 700;
}
.bc-block {
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  background-color: var(--vp-c-bg);
  padding: 10px 12px;
}
.bc-block.broken {
  border-color: var(--vp-c-danger-1);
  box-shadow: 0 0 0 1px var(--vp-c-danger-1);
}
.bc-head {
  display: flex;
  align-items: baseline;
  gap: 10px;
}
.bc-index {
  font-family: var(--vp-font-family-mono);
  font-weight: 700;
  color: var(--vp-c-brand-1);
}
.bc-data {
  font-size: 13px;
  flex: 1;
}
.bc-nonce {
  font-size: 11px;
  color: var(--vp-c-text-3);
  font-family: var(--vp-font-family-mono);
}
.bc-hashes {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 14px;
  margin-top: 4px;
}
.bc-h {
  font-family: var(--vp-font-family-mono);
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.bc-h.hash {
  color: var(--vp-c-green-1);
}
.bc-actions {
  display: flex;
  gap: 6px;
  margin-top: 8px;
}
.bc-mini {
  padding: 3px 10px;
  border-radius: 5px;
  font-size: 11px;
  background-color: var(--vp-c-default-soft);
  color: var(--vp-c-text-1);
}
.bc-mini.remine {
  background-color: var(--vp-c-brand-soft);
  color: var(--vp-c-brand-1);
  font-weight: 600;
}
.bc-mini:disabled {
  opacity: 0.5;
}
.bc-note {
  margin: 12px 0 0;
  font-size: 12px;
  color: var(--vp-c-text-2);
}
</style>
