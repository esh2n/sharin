<script setup lang="ts">
import { reactive, ref } from "vue";

import DemoShell from "./DemoShell.vue";

// frontend/reactivity(TS)を移植。signal/computed/effect の依存グラフを組み、
// ある signal を変えたときに反応するノードだけが光る様子を見せる。

// --- 移植したリアクティビティ核 ---
interface Eff {
  run: () => void;
  deps: Set<Set<Eff>>;
}
let active: Eff | null = null;
function track(subs: Set<Eff>) {
  if (active) {
    subs.add(active);
    active.deps.add(subs);
  }
}
function cleanup(e: Eff) {
  for (const d of e.deps) d.delete(e);
  e.deps.clear();
}
function makeSignal<T>(v: T) {
  const subs = new Set<Eff>();
  return {
    get() {
      track(subs);
      return v;
    },
    set(nv: T) {
      if (Object.is(nv, v)) return;
      v = nv;
      for (const e of [...subs]) e.run();
    },
  };
}
function makeEffect(fn: () => void): Eff {
  const e: Eff = {
    deps: new Set(),
    run() {
      cleanup(e);
      const prev = active;
      active = e;
      try {
        fn();
      } finally {
        active = prev;
      }
    },
  };
  e.run();
  return e;
}
function makeComputed<T>(fn: () => T, onRecompute: () => void) {
  const subs = new Set<Eff>();
  let value: T;
  let stale = true;
  const runner: Eff = {
    deps: new Set(),
    run() {
      if (!stale) {
        stale = true;
        for (const e of [...subs]) e.run();
      }
    },
  };
  return {
    get() {
      if (stale) {
        cleanup(runner);
        const prev = active;
        active = runner;
        try {
          value = fn();
        } finally {
          active = prev;
        }
        stale = false;
        onRecompute();
      }
      track(subs);
      return value;
    },
  };
}

// --- グラフの構築 ---
// signals: width, height, label / computed: area=w*h / effects: 3 つ
const state = reactive({
  width: 2,
  height: 3,
  label: "box",
  area: 6,
  areaComputes: 0,
  e1Runs: 0, // area を表示
  e2Runs: 0, // label を表示
  e3Runs: 0, // width を表示
  areaText: "",
  labelText: "",
  widthText: "",
});
const fired = ref<Set<string>>(new Set());

const width = makeSignal(2);
const height = makeSignal(3);
const label = makeSignal("box");
const area = makeComputed(
  () => width.get() * height.get(),
  () => {
    state.areaComputes++;
    fired.value.add("area");
  },
);

// effect たちを張る(初回実行で count が 1 になる)。
makeEffect(() => {
  state.areaText = `面積 ${area.get()}`;
  state.area = width.get() * height.get();
  state.e1Runs++;
  fired.value.add("e1");
});
makeEffect(() => {
  state.labelText = `ラベル ${label.get()}`;
  state.e2Runs++;
  fired.value.add("e2");
});
makeEffect(() => {
  state.widthText = `幅 ${width.get()}`;
  state.e3Runs++;
  fired.value.add("e3");
});

// 初回マウントの実行ぶんはハイライトしない(操作後だけ光らせる)。
fired.value = new Set();

let labelIdx = 0;
const labels = ["box", "card", "panel"];

function change(which: "width" | "height" | "label") {
  fired.value = new Set(); // このティックで反応したノードを記録
  if (which === "width") width.set(++state.width);
  else if (which === "height") height.set(++state.height);
  else {
    labelIdx = (labelIdx + 1) % labels.length;
    state.label = labels[labelIdx];
    label.set(state.label);
  }
  // Vue の描画更新のため新しい Set 参照に。
  fired.value = new Set(fired.value);
}

function fx(id: string) {
  return fired.value.has(id);
}
</script>

<template>
  <DemoShell title="リアクティビティ(signal)" badge-tone="neutral" :badge="`area 再計算 ${state.areaComputes}回`">
    <div class="rx-actions">
      <button class="sd-btn" @click="change('width')">width を +1</button>
      <button class="sd-btn" @click="change('height')">height を +1</button>
      <button class="sd-btn" @click="change('label')">label を切替</button>
      <span class="rx-hint">変えたときに光るノード = 再実行された購読者</span>
    </div>

    <div class="rx-graph">
      <div class="rx-col">
        <div class="rx-col-h">signal</div>
        <div class="rx-node sig" :class="{ fire: fx('width') }">
          <span class="rx-node-n mono">width</span><span class="rx-node-v mono">{{ state.width }}</span>
        </div>
        <div class="rx-node sig" :class="{ fire: fx('height') }">
          <span class="rx-node-n mono">height</span><span class="rx-node-v mono">{{ state.height }}</span>
        </div>
        <div class="rx-node sig" :class="{ fire: fx('label') }">
          <span class="rx-node-n mono">label</span><span class="rx-node-v mono">{{ state.label }}</span>
        </div>
      </div>

      <div class="rx-arrow">→</div>

      <div class="rx-col">
        <div class="rx-col-h">computed</div>
        <div class="rx-node comp" :class="{ fire: fx('area') }">
          <span class="rx-node-n mono">area = w×h</span><span class="rx-node-v mono">{{ state.area }}</span>
          <span class="rx-node-c mono">計算 {{ state.areaComputes }}回</span>
        </div>
      </div>

      <div class="rx-arrow">→</div>

      <div class="rx-col">
        <div class="rx-col-h">effect</div>
        <div class="rx-node eff" :class="{ fire: fx('e1') }">
          <span class="rx-node-n mono">E1: {{ state.areaText }}</span><span class="rx-node-c mono">{{ state.e1Runs }}回</span>
        </div>
        <div class="rx-node eff" :class="{ fire: fx('e3') }">
          <span class="rx-node-n mono">E3: {{ state.widthText }}</span><span class="rx-node-c mono">{{ state.e3Runs }}回</span>
        </div>
        <div class="rx-node eff" :class="{ fire: fx('e2') }">
          <span class="rx-node-n mono">E2: {{ state.labelText }}</span><span class="rx-node-c mono">{{ state.e2Runs }}回</span>
        </div>
      </div>
    </div>

    <p class="rx-legend">
      width を変えると area が再計算され、area を読む E1 と width を読む E3 が再実行される。だが label を読む
      E2 は動かない(読んでいない signal の変更には反応しない=きめ細かさ)。label を変えれば E2 だけが動く。
      area は依存(width/height)が変わるまで再計算せず、height を変えなければ計算回数は増えない(キャッシュ)。
      仮想DOMのように木を丸ごと比べず、変わった値に繋がる購読者だけがピンポイントで動く。
    </p>
  </DemoShell>
</template>

<style scoped>
.rx-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.rx-hint {
  font-size: 11px;
  color: var(--vp-c-text-3);
}
.rx-graph {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 16px;
}
.rx-col {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.rx-col-h {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-2);
  margin-bottom: 2px;
}
.rx-arrow {
  color: var(--vp-c-text-3);
  font-size: 14px;
}
.rx-node {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px 10px;
  border: 1px solid var(--vp-c-divider);
  border-left: 3px solid var(--vp-c-divider);
  border-radius: 0;
  background-color: var(--vp-c-bg-soft);
  transition: border-color 0.15s, background-color 0.15s;
}
.rx-node.sig {
  border-left-color: var(--vp-c-brand-1);
}
.rx-node.comp {
  border-left-color: var(--vp-c-warning-1);
}
.rx-node.eff {
  border-left-color: var(--vp-c-green-1);
}
.rx-node.fire {
  background-color: var(--vp-c-yellow-soft, var(--vp-c-warning-soft));
  border-color: var(--vp-c-warning-1);
  border-left-color: var(--vp-c-warning-1);
}
.rx-node-n {
  font-size: 11.5px;
  font-weight: 600;
  color: var(--vp-c-text-1);
}
.rx-node-v {
  font-size: 11px;
  color: var(--vp-c-brand-1);
}
.rx-node-c {
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.rx-legend {
  margin: 16px 0 0;
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
