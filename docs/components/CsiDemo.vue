<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/csi(Go)を移植。3段の単位の違いと、確保の時機を見る。

type Mode = "ReadWriteOnce" | "ReadWriteMany";
type Binding = "Immediate" | "WaitForFirstConsumer";

const mode = ref<Mode>("ReadWriteOnce");
const binding = ref<Binding>("Immediate");
const zoned = ref(false);

const vol = ref<{ zone: string; attachedTo: string; mountedBy: string[] } | null>(null);
const log = ref<string[]>([]);

const NODES = [
  { name: "node-a", zone: "zone-a" },
  { name: "node-b", zone: "zone-b" },
];

function logf(m: string) {
  log.value = [...log.value, m];
}

function reset() {
  log.value = [];
  if (binding.value === "Immediate") {
    const z = zoned.value ? "zone-a" : "";
    vol.value = { zone: z, attachedTo: "", mountedBy: [] };
    logf(z ? `pv-1 を作った(区画 ${z})` : "pv-1 を作った(区画の指定なし)");
  } else {
    vol.value = null;
    logf("使う Pod の置き場所が決まるまで待つ");
  }
}
reset();

function attach(node: string) {
  const n = NODES.find((x) => x.name === node)!;
  if (!vol.value) {
    const z = zoned.value ? n.zone : "";
    vol.value = { zone: z, attachedTo: node, mountedBy: [] };
    logf(`置き場所が決まったので pv-1 を作った${z ? `(区画 ${z})` : ""}。そのまま ${node} に繋いだ`);
    return;
  }
  const v = vol.value;
  if (v.zone && zoned.value && v.zone !== n.zone) {
    logf(`繋がらない: 実体は区画 ${v.zone} にある。${node} は区画 ${n.zone}`);
    return;
  }
  if (v.attachedTo === node) return;
  if (v.attachedTo && mode.value === "ReadWriteOnce") {
    logf(`繋がらない: すでに ${v.attachedTo} に繋がっている。ReadWriteOnce は1つのノードから`);
    return;
  }
  vol.value = { ...v, attachedTo: node };
  logf(`pv-1 を ${node} に繋いだ`);
}

function mount(node: string, pod: string) {
  const v = vol.value;
  if (!v) {
    logf("見せられない: 実体がまだ無い");
    return;
  }
  if (v.attachedTo !== node) {
    logf(`見せられない: ${node} に繋がっていない`);
    return;
  }
  if (v.mountedBy.includes(pod)) return;
  vol.value = { ...v, mountedBy: [...v.mountedBy, pod].sort() };
  logf(`pv-1 が ${pod} から見えるようになった`);
}

function unmountAll() {
  if (!vol.value) return;
  vol.value = { ...vol.value, mountedBy: [] };
  logf("すべての Pod から見えなくした");
}

function detach() {
  const v = vol.value;
  if (!v) return;
  if (v.mountedBy.length > 0) {
    logf(`外せない: まだ ${v.mountedBy.length} 個の Pod から見えている`);
    return;
  }
  if (!v.attachedTo) return;
  logf(`pv-1 を ${v.attachedTo} から外した`);
  vol.value = { ...v, attachedTo: "" };
}

function forceDetach() {
  if (!vol.value) return;
  vol.value = { ...vol.value, attachedTo: "", mountedBy: [] };
  logf("ノードの応答を待たずに外した(二重書き込みの危険を承知で)");
}

function change(fn: () => void) {
  fn();
  reset();
}

const podsOn = (node: string) =>
  vol.value && vol.value.attachedTo === node ? vol.value.mountedBy : [];
const badge = computed(() => {
  const v = vol.value;
  if (!v) return "実体はまだ無い";
  return `pv-1 ・ ${v.attachedTo || "未接続"} ・ ${v.mountedBy.length} 個から見えている`;
});
const verdict = computed(() => {
  const v = vol.value;
  if (!v) return "待つ設定なので、まだ実体は無い。どこかのノードに繋ごうとした時点で、その区画に合わせて作られる";
  if (v.mountedBy.length > 1) {
    return `${v.mountedBy.length} 個の Pod が同じボリュームを見ている。${mode.value} でも、同じノードの上なら共有できる`;
  }
  if (v.mountedBy.length > 0 && v.attachedTo) {
    return `${v.attachedTo} に繋がり、${v.mountedBy[0]} から見えている。別のノードに繋ごうとすると止まる`;
  }
  if (v.attachedTo) return `${v.attachedTo} に繋がっている。ここから Pod に見せられる`;
  return "実体はあるが、まだどのノードにも繋がっていない";
});
</script>

<template>
  <DemoShell title="CSI とボリューム" :badge="badge" badge-tone="ok">
    <div class="cs-actions">
      <button class="sd-btn" @click="change(() => (mode = mode === 'ReadWriteOnce' ? 'ReadWriteMany' : 'ReadWriteOnce'))">
        {{ mode }}
      </button>
      <button class="sd-btn" @click="change(() => (binding = binding === 'Immediate' ? 'WaitForFirstConsumer' : 'Immediate'))">
        {{ binding }}
      </button>
      <button class="sd-btn" :class="zoned ? 'sd-btn--primary' : ''" @click="change(() => (zoned = !zoned))">
        区画あり: {{ zoned ? "あり" : "なし" }}
      </button>
      <span class="cs-gap" />
      <button class="sd-btn" @click="reset">リセット</button>
    </div>

    <div class="cs-nodes">
      <div v-for="n in NODES" :key="n.name" class="cs-node" :class="vol && vol.attachedTo === n.name ? 'on' : ''">
        <div class="cs-node-h mono">
          {{ n.name }}<span v-if="zoned">{{ n.zone }}</span>
        </div>
        <div class="cs-disk mono" :class="vol && vol.attachedTo === n.name ? 'here' : 'away'">
          {{ vol && vol.attachedTo === n.name ? "pv-1 が繋がっている" : "繋がっていない" }}
        </div>
        <div class="cs-pods">
          <span v-for="p in podsOn(n.name)" :key="p" class="cs-pod mono">{{ p }}</span>
          <span v-if="!podsOn(n.name).length" class="cs-pod empty mono">見えている Pod なし</span>
        </div>
        <div class="cs-btns">
          <button class="sd-btn sd-btn--sm" @click="attach(n.name)">繋ぐ</button>
          <button class="sd-btn sd-btn--sm" @click="mount(n.name, 'web-' + (podsOn(n.name).length + 1))">
            Pod に見せる
          </button>
        </div>
      </div>
    </div>

    <div class="cs-ops">
      <button class="sd-btn" @click="unmountAll">すべて見えなくする</button>
      <button class="sd-btn" @click="detach">ノードから外す</button>
      <button class="sd-btn" @click="forceDetach">応答を待たずに外す</button>
    </div>

    <div class="cs-verdict">{{ verdict }}</div>

    <div class="cs-log">
      <div v-for="(l, i) in log.slice(-5)" :key="i" class="cs-log-line mono">{{ l }}</div>
    </div>

    <p class="cs-legend">
      「繋ぐ」はノード単位、「Pod に見せる」は Pod 単位。同じノードで「Pod に見せる」を何回か押すと、
      ReadWriteOnce のまま複数の Pod が共有できる。別のノードで「繋ぐ」を押すと止まる。
      見えている Pod がある間は外せず、応答が無いノードには最後の手段しか残らない。
      区画ありにすると、先に作る設定では実体が区画に固定され、待つ設定では Pod の区画に合わせて作られる。
    </p>
  </DemoShell>
</template>

<style scoped>
.cs-actions,
.cs-ops {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.cs-ops {
  margin-top: 10px;
}
.cs-gap {
  flex: 1;
  min-width: 8px;
}
.cs-nodes {
  display: flex;
  gap: 10px;
  margin-top: 14px;
  flex-wrap: wrap;
}
.cs-node {
  flex: 1;
  min-width: 210px;
  border: 1px solid var(--vp-c-divider);
  padding: 9px 12px;
  background-color: var(--vp-c-bg-soft);
}
.cs-node.on {
  border-color: var(--vp-c-brand-1);
}
.cs-node-h {
  display: flex;
  justify-content: space-between;
  font-size: 11.5px;
  font-weight: 700;
  color: var(--vp-c-text-1);
  margin-bottom: 6px;
}
.cs-node-h span {
  font-weight: 400;
  font-size: 9.5px;
  color: var(--vp-c-text-3);
}
.cs-disk {
  font-size: 10.5px;
  padding: 4px 8px;
  border: 1px solid var(--vp-c-divider);
  margin-bottom: 5px;
}
.cs-disk.here {
  border-color: var(--vp-c-green-1);
  color: var(--vp-c-green-1);
  background-color: var(--vp-c-green-soft);
}
.cs-disk.away {
  color: var(--vp-c-text-3);
}
.cs-pods {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
  min-height: 20px;
  margin-bottom: 6px;
}
.cs-pod {
  font-size: 9.5px;
  padding: 2px 6px;
  border: 1px solid var(--vp-c-brand-1);
  color: var(--vp-c-brand-1);
  background-color: var(--vp-c-brand-soft);
}
.cs-pod.empty {
  border-color: transparent;
  background: none;
  color: var(--vp-c-text-3);
  padding-left: 0;
}
.cs-btns {
  display: flex;
  gap: 6px;
}
.sd-btn--sm {
  font-size: 10.5px;
  padding: 3px 8px;
}
.cs-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-brand-1);
  font-size: 12.5px;
  font-weight: 600;
  line-height: 1.6;
  color: var(--vp-c-text-1);
  background-color: var(--vp-c-bg-soft);
}
.cs-log {
  margin-top: 10px;
  border-top: 1px solid var(--vp-c-divider);
  padding-top: 6px;
  min-height: 52px;
}
.cs-log-line {
  font-size: 10px;
  color: var(--vp-c-text-2);
  padding: 1px 0;
}
.cs-legend {
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
