<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/etcdops(Go)を移植。捨てる・返す・解除するの順序を体で確かめる。

const QUOTA = 8;
interface Ver {
  rev: number;
  value: string;
  del: boolean;
}

const rev = ref(0);
const history = ref<Record<string, Ver[]>>({});
const compactedAt = ref(0);
const physical = ref(0);
const alarm = ref(false);
const log = ref<string[]>([]);
const seq = ref(0);

function logf(m: string) {
  log.value = [...log.value, m];
}
function logical(): number {
  return Object.values(history.value).filter((vs) => vs.length && !vs[vs.length - 1].del).length;
}

function put() {
  if (alarm.value) {
    logf("書けない: 容量を使い切っている");
    return;
  }
  seq.value++;
  const key = `key-${((seq.value - 1) % 3) + 1}`;
  rev.value++;
  const vs = history.value[key] || [];
  history.value = { ...history.value, [key]: [...vs, { rev: rev.value, value: `v${seq.value}`, del: false }] };
  physical.value++;
  logf(`版 ${rev.value}: ${key} を書いた`);
  if (physical.value > QUOTA && !alarm.value) {
    alarm.value = true;
    logf("容量の上限を超えた。書き込みを止める(読み出しは通る)");
  }
}

function del() {
  if (alarm.value) {
    logf("消せない: 容量を使い切っている");
    return;
  }
  const key = Object.keys(history.value).sort()[0];
  if (!key) return;
  rev.value++;
  history.value = {
    ...history.value,
    [key]: [...history.value[key], { rev: rev.value, value: "", del: true }],
  };
  physical.value++;
  logf(`版 ${rev.value}: ${key} を消した(削除も履歴なので量は増える)`);
  if (physical.value > QUOTA && !alarm.value) {
    alarm.value = true;
    logf("容量の上限を超えた。書き込みを止める");
  }
}

function compact() {
  const to = rev.value;
  if (to <= compactedAt.value) {
    logf("すでにそこまで捨てている");
    return;
  }
  let dropped = 0;
  const next: Record<string, Ver[]> = {};
  for (const [k, vs] of Object.entries(history.value)) {
    const keep: Ver[] = [];
    vs.forEach((v, i) => {
      const isLast = i === vs.length - 1;
      if (v.rev < to && !isLast && vs[i + 1].rev <= to) {
        dropped++;
        return;
      }
      keep.push(v);
    });
    next[k] = keep;
  }
  history.value = next;
  compactedAt.value = to;
  logf(`版 ${to} より古い履歴を ${dropped} 件捨てた(ファイルの大きさは変わらない)`);
}

function defrag() {
  const before = physical.value;
  physical.value = Object.values(history.value).reduce((n, vs) => n + vs.length, 0);
  logf(`余っていた ${before - physical.value} を返した(この間そのメンバーは応答しない)`);
}

function disarm() {
  if (!alarm.value) {
    logf("止まっていない");
    return;
  }
  if (physical.value > QUOTA) {
    alarm.value = false;
    logf("解除したが、空きが戻っていないので次の書き込みでまた止まる");
    return;
  }
  alarm.value = false;
  logf("書き込みを再開できる状態になった");
}

function reset() {
  rev.value = 0;
  history.value = {};
  compactedAt.value = 0;
  physical.value = 0;
  alarm.value = false;
  log.value = [];
  seq.value = 0;
  for (let i = 0; i < 10; i++) put();
}
reset();

const canFollow = computed(() => compactedAt.value === 0);
const badge = computed(() => `物理 ${physical.value} / 上限 ${QUOTA} ・ 論理 ${logical()}`);
const badgeTone = computed<"ok" | "ng">(() => (alarm.value || physical.value > QUOTA ? "ng" : "ok"));
const verdict = computed(() => {
  if (alarm.value) return "書き込みが止まっている。設定を変えるのも書き込みなので、捨てる・返す・解除するの3手でしか抜けられない";
  if (physical.value > QUOTA) return "解除はしたが、まだ上限を超えている。次の書き込みでまた止まる";
  if (compactedAt.value > 0 && physical.value > logical()) return "履歴は捨てたが、ファイルはまだ大きいまま。返す操作が要る";
  if (compactedAt.value > 0) return `捨てて返した。版 ${compactedAt.value} より古い写しは、もう差分では追いつけない`;
  return "履歴が全部残っている。どの版からでも差分で追いつけるが、そのぶん量が増える";
});
</script>

<template>
  <DemoShell title="etcd の履歴と容量" :badge="badge" :badge-tone="badgeTone">
    <div class="eo-actions">
      <button class="sd-btn sd-btn--primary" @click="put">書き込む</button>
      <button class="sd-btn" @click="del">削除する</button>
      <span class="eo-gap" />
      <button class="sd-btn" @click="compact">① 捨てる</button>
      <button class="sd-btn" @click="defrag">② 返す</button>
      <button class="sd-btn" @click="disarm">③ 解除する</button>
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <div class="eo-bars">
      <div class="eo-bar-row">
        <span class="eo-bl mono">物理(ファイル)</span>
        <span class="eo-bar">
          <span class="eo-fill phys" :style="{ width: Math.min(100, (physical / (QUOTA * 1.6)) * 100) + '%' }" />
          <span class="eo-limit" :style="{ left: (QUOTA / (QUOTA * 1.6)) * 100 + '%' }" />
        </span>
        <span class="eo-bn mono" :class="physical > QUOTA ? 'over' : ''">{{ physical }}</span>
      </div>
      <div class="eo-bar-row">
        <span class="eo-bl mono">論理(使っている)</span>
        <span class="eo-bar">
          <span class="eo-fill log" :style="{ width: Math.min(100, (logical() / (QUOTA * 1.6)) * 100) + '%' }" />
          <span class="eo-limit" :style="{ left: (QUOTA / (QUOTA * 1.6)) * 100 + '%' }" />
        </span>
        <span class="eo-bn mono">{{ logical() }}</span>
      </div>
      <div class="eo-scale mono">縦線が上限 {{ QUOTA }}</div>
    </div>

    <div class="eo-state">
      <div class="eo-cell mono" :class="alarm ? 'bad' : 'ok'">
        <em>書き込み</em>{{ alarm ? "止まっている" : "できる" }}
      </div>
      <div class="eo-cell mono ok"><em>読み出し</em>いつでもできる</div>
      <div class="eo-cell mono" :class="canFollow ? 'ok' : 'warn'">
        <em>差分での追いつき</em>{{ canFollow ? "版 1 から追える" : `版 ${compactedAt} より後だけ` }}
      </div>
    </div>

    <div class="eo-verdict" :class="badgeTone === 'ng' ? 'bad' : 'ok'">{{ verdict }}</div>

    <div class="eo-log">
      <div v-for="(l, i) in log.slice(-5)" :key="i" class="eo-log-line mono">{{ l }}</div>
    </div>

    <p class="eo-legend">
      上限の小さい置き場に書き込んでいる。上書きも削除も履歴なので、物理の帯だけが伸びる。
      上限を超えると書き込みが止まる。①だけ押して③を押すと、解除はできても次の書き込みでまた止まる。
      ①②③の順に押して初めて抜けられる。そして①を押した時点で、古い写しは差分では追いつけなくなる。
    </p>
  </DemoShell>
</template>

<style scoped>
.eo-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.eo-gap {
  flex: 1;
  min-width: 8px;
}
.eo-bars {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
}
.eo-bar-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 5px;
}
.eo-bl {
  width: 116px;
  flex: none;
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.eo-bar {
  position: relative;
  flex: 1;
  height: 14px;
  background-color: var(--vp-c-bg-alt);
  border: 1px solid var(--vp-c-divider);
}
.eo-fill {
  display: block;
  height: 100%;
}
.eo-fill.phys {
  background-color: var(--vp-c-danger-1);
}
.eo-fill.log {
  background-color: var(--vp-c-brand-1);
}
.eo-limit {
  position: absolute;
  top: -2px;
  bottom: -2px;
  width: 2px;
  background-color: var(--vp-c-text-1);
}
.eo-bn {
  width: 34px;
  flex: none;
  text-align: right;
  font-size: 10px;
  color: var(--vp-c-text-2);
}
.eo-bn.over {
  color: var(--vp-c-danger-1);
  font-weight: 700;
}
.eo-scale {
  font-size: 9.5px;
  color: var(--vp-c-text-3);
  margin-left: 124px;
}
.eo-state {
  display: flex;
  gap: 8px;
  margin-top: 10px;
  flex-wrap: wrap;
}
.eo-cell {
  flex: 1;
  min-width: 150px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 11px;
  padding: 6px 10px;
  border: 1px solid var(--vp-c-divider);
}
.eo-cell em {
  font-style: normal;
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.eo-cell.ok {
  border-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.eo-cell.warn {
  border-color: var(--vp-c-warning-1);
  color: var(--vp-c-warning-1);
  background-color: var(--vp-c-warning-soft);
}
.eo-cell.bad {
  border-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.eo-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  font-weight: 600;
  line-height: 1.6;
  background-color: var(--vp-c-bg-soft);
}
.eo-verdict.ok {
  border-left-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.eo-verdict.bad {
  border-left-color: var(--vp-c-danger-1);
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.eo-log {
  margin-top: 10px;
  border-top: 1px solid var(--vp-c-divider);
  padding-top: 6px;
  min-height: 52px;
}
.eo-log-line {
  font-size: 10px;
  color: var(--vp-c-text-2);
  padding: 1px 0;
}
.eo-legend {
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
