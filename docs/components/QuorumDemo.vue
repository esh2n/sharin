<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// distributed/quorum(Go)を移植。R + W > N の重なりを、読んで確かめる。

const NAMES = ["a", "b", "c", "d", "e"];
const N = 3;
const KEY = "x";

interface Val {
  d: string;
  s: number;
}
interface Hint {
  owner: string;
  d: string;
  s: number;
}

const R = ref(2);
const W = ref(2);
const sloppy = ref(false);
const repair = ref(false);

const data = ref<Record<string, Val | null>>({});
const hints = ref<Record<string, Hint[]>>({});
const down = ref<Record<string, boolean>>({});
const stamp = ref(0);
const writes = ref(0);
const reads = ref(0);
const recent = ref<{ v: string; stale: boolean }[]>([]);
const log = ref<string[]>([]);

function hash(key: string): number {
  let h = 2166136261 >>> 0;
  for (let i = 0; i < key.length; i++) {
    h ^= key.charCodeAt(i);
    h = Math.imul(h, 16777619) >>> 0;
  }
  return h >>> 0;
}
const START = hash(KEY) % NAMES.length;
const HOME = Array.from({ length: N }, (_, i) => NAMES[(START + i) % NAMES.length]);
const isHome = (n: string) => HOME.includes(n);

function newer(a: Val | null, b: Val | null): Val | null {
  if (!a) return b;
  if (!b) return a;
  return b.s > a.s ? b : a;
}
function put(n: string, v: Val) {
  data.value = { ...data.value, [n]: newer(data.value[n], v) };
}
function logf(m: string) {
  log.value = [...log.value, m];
}

function reset() {
  data.value = Object.fromEntries(NAMES.map((n) => [n, null]));
  hints.value = Object.fromEntries(NAMES.map((n) => [n, []]));
  down.value = {};
  stamp.value = 0;
  writes.value = 0;
  reads.value = 0;
  recent.value = [];
  log.value = [];
  write(); // 最初の1件は全台そろった状態で入れておく
  log.value = [];
}

function substitute(used: Set<string>): string {
  for (let i = N; i < NAMES.length; i++) {
    const n = NAMES[(START + i) % NAMES.length];
    if (down.value[n] || used.has(n)) continue;
    return n;
  }
  return "";
}

function write() {
  stamp.value++;
  writes.value++;
  const v: Val = { d: `v${writes.value}`, s: stamp.value };
  const used = new Set(HOME);
  let acks = 0;
  const subs: string[] = [];
  for (const n of HOME) {
    if (!down.value[n]) {
      put(n, v);
      acks++;
      continue;
    }
    if (!sloppy.value) continue;
    const s = substitute(used);
    if (!s) continue;
    used.add(s);
    hints.value = { ...hints.value, [s]: [...hints.value[s], { owner: n, ...v }] };
    subs.push(s);
    acks++;
  }
  const tail = subs.length ? `(うち代役 ${subs.join("、")})` : "";
  if (acks >= W.value) {
    logf(`${KEY}=${v.d} を書いた。返事 ${acks} 台 ${tail}`);
  } else {
    logf(`${KEY}=${v.d} は返事 ${acks} 台で W=${W.value} に届かない。書けた台には残る`);
  }
}

const latest = computed(() => {
  let s = 0;
  for (const n of NAMES) {
    const v = data.value[n];
    if (v && v.s > s) s = v.s;
    for (const h of hints.value[n] ?? []) if (h.s > s) s = h.s;
  }
  return s;
});

function read() {
  reads.value++;
  const asked: string[] = [];
  const got: (Val | null)[] = [];
  for (let i = 0; i < HOME.length && asked.length < R.value; i++) {
    const n = HOME[(reads.value + i) % HOME.length];
    if (down.value[n]) continue;
    asked.push(n);
    got.push(data.value[n]);
  }
  if (asked.length < R.value) {
    recent.value = [...recent.value, { v: "×", stale: true }].slice(-8);
    logf(`返事が ${asked.length} 台で R=${R.value} に届かないので読めない`);
    return;
  }
  let best: Val | null = null;
  for (const v of got) best = newer(best, v);
  const stale = !best || best.s < latest.value;
  recent.value = [...recent.value, { v: best ? best.d : "空", stale }].slice(-8);
  if (repair.value && best) {
    for (let i = 0; i < asked.length; i++) {
      if ((got[i]?.s ?? 0) < best.s) {
        put(asked[i], best);
        logf(`${asked[i]} を読み修復した`);
      }
    }
  }
  logf(`${asked.join("、")} に聞いた → ${best ? best.d : "空"}`);
}
function readMany() {
  for (let i = 0; i < 6; i++) read();
}

function handoff() {
  let moved = 0;
  for (const n of NAMES) {
    const keep: Hint[] = [];
    for (const h of hints.value[n] ?? []) {
      if (down.value[h.owner]) {
        keep.push(h);
        continue;
      }
      put(h.owner, { d: h.d, s: h.s });
      moved++;
      logf(`${n} の預かりを ${h.owner} へ渡した`);
    }
    hints.value = { ...hints.value, [n]: keep };
  }
  if (!moved) logf("渡せる預かりが無い");
}

function toggle(n: string) {
  down.value = { ...down.value, [n]: !down.value[n] };
  logf(`${n} が${down.value[n] ? "落ちた" : "戻った"}`);
}
function bump(which: "R" | "W", by: number) {
  const t = which === "R" ? R : W;
  t.value = Math.min(N, Math.max(1, t.value + by));
}

reset();

const overlaps = computed(() => R.value + W.value > N);
const staleHome = computed(() =>
  HOME.filter((n) => (data.value[n]?.s ?? 0) < latest.value),
);
const pending = computed(() => NAMES.reduce((a, n) => a + (hints.value[n]?.length ?? 0), 0));
const staleReads = computed(() => recent.value.filter((r) => r.stale).length);
const badge = computed(
  () => `R+W = ${R.value + W.value} ${overlaps.value ? ">" : "≦"} N = ${N}`,
);
const verdict = computed(() => {
  if (!overlaps.value)
    return `R + W が N を超えていない。古い台は ${N - W.value} 台あって R = ${R.value} 台で埋まるので、古い値だけが返ることがある`;
  if (pending.value)
    return "返事の数は満たしたが、値は担当の外に預けたままになっている。読みは担当にしか聞かないので、受け渡しが済むまで古い値が返りうる";
  if (staleHome.value.length)
    return `担当のうち ${staleHome.value.join("、")} が古いが、R + W > N なので R 台の中に必ず最新が混ざる`;
  return "担当がそろっている。どの台に聞いても最新が返る。台を落としてから書き、戻してから読むと差が出る";
});
</script>

<template>
  <DemoShell title="クォーラム" :badge="badge" :badge-tone="overlaps ? 'ok' : 'ng'">
    <div class="qr-actions">
      <button class="sd-btn sd-btn--primary" @click="write">書き込む</button>
      <button class="sd-btn" @click="readMany">6 回読む</button>
      <button class="sd-btn" @click="handoff">受け渡し</button>
      <span class="qr-gap" />
      <button class="sd-btn" @click="reset">リセット</button>
    </div>
    <div class="qr-actions qr-actions--sub">
      <span class="qr-knob mono">
        R
        <button class="sd-btn qr-mini" @click="bump('R', -1)">−</button>
        <b>{{ R }}</b>
        <button class="sd-btn qr-mini" @click="bump('R', 1)">＋</button>
      </span>
      <span class="qr-knob mono">
        W
        <button class="sd-btn qr-mini" @click="bump('W', -1)">−</button>
        <b>{{ W }}</b>
        <button class="sd-btn qr-mini" @click="bump('W', 1)">＋</button>
      </span>
      <button class="sd-btn" :class="sloppy ? 'sd-btn--primary' : ''" @click="sloppy = !sloppy">
        担当が落ちたら代役に預ける: {{ sloppy ? "する" : "しない" }}
      </button>
      <button class="sd-btn" :class="repair ? 'sd-btn--primary' : ''" @click="repair = !repair">
        読んだついでに直す: {{ repair ? "する" : "しない" }}
      </button>
    </div>

    <div class="qr-grid">
      <div v-for="n in NAMES" :key="n" class="qr-node" :class="[isHome(n) ? 'home' : 'spare', down[n] ? 'off' : '']">
        <span class="qr-name mono">{{ n }}</span>
        <span class="qr-role">{{ isHome(n) ? "担当" : "担当外" }}</span>
        <span class="qr-val mono" :class="isHome(n) && (data[n]?.s ?? 0) < latest ? 'old' : ''">
          {{ down[n] ? "返事なし" : (data[n]?.d ?? "空") }}
        </span>
        <span class="qr-hint mono">{{ hints[n]?.length ? `預かり ${hints[n].length}` : "" }}</span>
        <button class="sd-btn qr-mini" @click="toggle(n)">{{ down[n] ? "戻す" : "落とす" }}</button>
      </div>
    </div>

    <div class="qr-reads">
      <span class="qr-reads-label">直近の読み</span>
      <span v-for="(r, i) in recent" :key="i" class="qr-chip mono" :class="r.stale ? 'bad' : 'ok'">{{ r.v }}</span>
      <span v-if="!recent.length" class="qr-chip mono empty">まだ読んでいない</span>
      <span v-if="recent.length" class="qr-count mono">古い値 {{ staleReads }} / {{ recent.length }}</span>
    </div>

    <div class="qr-verdict" :class="overlaps && !pending ? 'ok' : 'bad'">{{ verdict }}</div>

    <div class="qr-log">
      <div v-for="(l, i) in log.slice(-3)" :key="i" class="qr-log-line mono">{{ l }}</div>
      <div v-if="!log.length" class="qr-log-line mono empty">(まだ何もしていない)</div>
    </div>

    <p class="qr-legend">
      担当は key ごとに決まる 3 台で、この key では a・b・c。書き込みは担当へ送って W 台の返事で成功、
      読みは担当のうち R 台に聞いていちばん新しい値を返す。聞く順は読むたびに回るので、どの台に当たるかは
      毎回違う。担当を 1 台落として書き、戻してから読むと、R + W が N を超えていれば古い値は返らない。
      代役に預ける設定にすると、返事の数は満たすのに値が担当の外に残り、受け渡しまで古い値が返る。
    </p>
  </DemoShell>
</template>

<style scoped>
.qr-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.qr-actions--sub {
  margin-top: 8px;
}
.qr-gap {
  flex: 1;
  min-width: 8px;
}
.qr-knob {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.qr-mini {
  padding: 1px 7px;
  font-size: 10.5px;
  border-radius: 4px;
}
.qr-knob b {
  min-width: 12px;
  text-align: center;
  color: var(--vp-c-text-1);
}
.qr-grid {
  display: flex;
  gap: 6px;
  margin-top: 14px;
}
.qr-node {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 3px;
  padding: 8px 4px;
  border: 1px solid var(--vp-c-divider);
  background-color: var(--vp-c-bg-soft);
}
.qr-node.home {
  border-color: var(--vp-c-brand-1);
}
.qr-node.off {
  opacity: 0.55;
}
.qr-name {
  font-size: 13px;
  font-weight: 600;
}
.qr-role {
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.qr-node.home .qr-role {
  color: var(--vp-c-brand-1);
}
.qr-val {
  font-size: 11.5px;
  color: var(--vp-c-text-1);
}
.qr-val.old {
  color: var(--vp-c-warning-1);
}
.qr-hint {
  font-size: 9px;
  min-height: 12px;
  color: var(--vp-c-text-3);
}
.qr-reads {
  display: flex;
  align-items: center;
  gap: 5px;
  flex-wrap: wrap;
  margin-top: 12px;
}
.qr-reads-label {
  font-size: 11px;
  color: var(--vp-c-text-2);
}
.qr-chip {
  font-size: 10.5px;
  padding: 2px 7px;
  border: 1px solid transparent;
}
.qr-chip.ok {
  border-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.qr-chip.bad {
  border-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.qr-chip.empty {
  color: var(--vp-c-text-3);
  padding-left: 0;
}
.qr-count {
  font-size: 10.5px;
  color: var(--vp-c-text-2);
  margin-left: 4px;
}
.qr-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  font-weight: 600;
  line-height: 1.6;
  background-color: var(--vp-c-bg-soft);
}
.qr-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.qr-verdict.bad {
  border-left-color: var(--vp-c-warning-1);
  color: var(--vp-c-warning-1);
  background-color: var(--vp-c-warning-soft);
}
.qr-log {
  margin-top: 10px;
  border-top: 1px solid var(--vp-c-divider);
  padding-top: 6px;
  min-height: 36px;
}
.qr-log-line {
  font-size: 10px;
  color: var(--vp-c-text-2);
  padding: 1px 0;
}
.qr-log-line.empty {
  color: var(--vp-c-text-3);
}
.qr-legend {
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
