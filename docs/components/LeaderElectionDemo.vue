<script setup lang="ts">
import { ref, computed } from "vue";
import DemoShell from "./DemoShell.vue";

// orchestration/leaderelection(Go)を移植。同じ切り離しを2つの設定に与えて、
// 降りるほうが先なら空位ができ、奪うほうが先なら重なりができることを見る。

interface Config {
  lease: number;
  renew: number;
  retry: number;
}
interface Cand {
  name: string;
  leader: boolean;
  lastRenew: number;
  obsVersion: number;
  obsAt: number;
  nextAct: number;
  down: boolean;
}
type Cell = "lead" | "lead-cut" | "idle" | "cut";
interface Run {
  label: string;
  cfg: Config;
  hist: Record<string, Cell[]>;
  status: ("ok" | "overlap" | "vacant")[];
  overlap: number;
  vacant: number;
  doubleActs: number;
  log: string[];
}

const TOTAL = 46;
const PARTITION_AT = 7;
const HEAL_AT = 24;
const NAMES = ["c1", "c2"];

const SAFE: Config = { lease: 15, renew: 10, retry: 2 };
const RISKY: Config = { lease: 15, renew: 20, retry: 2 };

function simulate(label: string, cfg: Config, healAt: number | null): Run {
  const cands: Cand[] = NAMES.map((name) => ({
    name,
    leader: false,
    lastRenew: 0,
    obsVersion: 0,
    obsAt: 0,
    nextAct: 0,
    down: false,
  }));
  const lease = { holder: "", version: 0 };
  const hist: Record<string, Cell[]> = {};
  for (const n of NAMES) hist[n] = [];
  const status: ("ok" | "overlap" | "vacant")[] = [];
  const log: string[] = [];
  let overlap = 0;
  let vacant = 0;
  let doubleActs = 0;

  const find = (n: string) => cands.find((c) => c.name === n)!;
  const write = (c: Cand, now: number) => {
    lease.version++;
    c.lastRenew = now;
    c.obsVersion = lease.version;
    c.obsAt = now;
  };

  for (let now = 0; now < TOTAL; now++) {
    if (now === PARTITION_AT) {
      find("c1").down = true;
      log.push(`t=${now} c1 が置き場に届かなくなった`);
    }
    if (healAt !== null && now === healAt && find("c1").down) {
      find("c1").down = false;
      log.push(`t=${now} c1 が置き場に届くようになった`);
    }

    for (const c of cands) {
      if (c.down) {
        if (c.leader && now - c.lastRenew >= cfg.renew) {
          c.leader = false;
          log.push(`t=${now} ${c.name} は更新できないまま猶予 ${cfg.renew} を使い切った。自分から降りる`);
        }
        continue;
      }
      // 観測が先、判断が後。
      if (lease.version !== c.obsVersion) {
        c.obsVersion = lease.version;
        c.obsAt = now;
      }
      if (now < c.nextAct) continue;
      c.nextAct = now + cfg.retry;

      if (c.leader) {
        if (lease.holder !== c.name) {
          c.leader = false;
          log.push(`t=${now} ${c.name} は持ち主が ${lease.holder} に変わっているのを見た。降りる`);
          continue;
        }
        write(c, now);
        continue;
      }
      if (lease.holder === "" || now - c.obsAt >= cfg.lease) {
        const prev = lease.holder;
        const waited = now - c.obsAt;
        lease.holder = c.name;
        write(c, now);
        c.leader = true;
        log.push(
          prev === ""
            ? `t=${now} ${c.name} が持ち主になった`
            : `t=${now} ${c.name} が ${waited} のあいだ変化を見なかったので ${prev} から奪った`,
        );
      }
    }

    for (const c of cands) {
      hist[c.name].push(c.leader ? (c.down ? "lead-cut" : "lead") : c.down ? "cut" : "idle");
    }
    const believers = cands.filter((c) => c.leader).length;
    if (believers === 0) {
      vacant++;
      status.push("vacant");
    } else if (believers > 1) {
      overlap++;
      doubleActs += believers - 1;
      status.push("overlap");
    } else {
      status.push("ok");
    }
  }
  return { label, cfg, hist, status, overlap, vacant, doubleActs, log };
}

const healed = ref(false);
const runs = computed<Run[]>(() => {
  const h = healed.value ? HEAL_AT : null;
  return [simulate("降りるほうが先(猶予 10 / 期限 15)", SAFE, h), simulate("奪うほうが先(猶予 20 / 期限 15)", RISKY, h)];
});

const cols = computed(() => Array.from({ length: TOTAL }, (_, i) => i));
const badge = computed(() => `重なり ${runs.value[1].overlap} / 空位 ${runs.value[0].vacant}`);
const cellLabel: Record<Cell, string> = {
  lead: "自分が持ち主だと思っている",
  "lead-cut": "持ち主だと思っているが、置き場に届かない",
  idle: "持ち主ではない",
  cut: "置き場に届かない",
};
</script>

<template>
  <DemoShell title="leader election" :badge="badge" badge-tone="ng">
    <div class="le-actions">
      <button class="sd-btn" :class="healed ? 'sd-btn--primary' : ''" @click="healed = !healed">
        t={{ HEAL_AT }} で c1 を繋ぎ直す: {{ healed ? "繋ぎ直す" : "繋がないまま" }}
      </button>
      <span class="le-spacer" />
      <span class="le-hint mono">t={{ PARTITION_AT }} で持ち主 c1 が置き場に届かなくなる。以降 c1 は更新できない</span>
    </div>

    <div v-for="r in runs" :key="r.label" class="le-run" :class="r.overlap > 0 ? 'bad' : ''">
      <div class="le-run-h">
        <span class="le-title">{{ r.label }}</span>
        <span class="le-counts mono">
          重なり {{ r.overlap }} ・ 空位 {{ r.vacant }} ・ 二重になった操作 {{ r.doubleActs }}
        </span>
      </div>

      <div v-for="n in NAMES" :key="n" class="le-row">
        <span class="le-name mono">{{ n }}</span>
        <span class="le-track">
          <span
            v-for="t in cols"
            :key="t"
            class="le-cell"
            :class="'c-' + r.hist[n][t]"
            :title="'t=' + t + ' ' + cellLabel[r.hist[n][t]]"
          />
        </span>
      </div>
      <div class="le-row le-status">
        <span class="le-name mono">持ち主</span>
        <span class="le-track">
          <span v-for="t in cols" :key="t" class="le-cell" :class="'s-' + r.status[t]" />
        </span>
      </div>
      <div class="le-scale mono">
        <span>t=0</span><span>t={{ PARTITION_AT }} 切り離し</span><span>t={{ TOTAL - 1 }}</span>
      </div>

      <div class="le-log">
        <div v-for="(l, i) in r.log" :key="i" class="le-log-line mono">{{ l }}</div>
      </div>
    </div>

    <div class="le-verdict bad">
      同じ切り離しから違う結果が出ている。降りるほうが先なら誰も持ち主でない時間ができ、その間の調整は止まる。
      奪うほうが先なら止まらないが、2人が同時に働く時間ができて、外に出る操作がその回数だけ二重になる
    </div>

    <p class="le-legend">
      緑が「自分が持ち主だ」と思っている時刻、黄が届かないまま持ち主だと思っている時刻、破線が置き場に
      届かない時刻。いちばん下の帯が全体の様子で、
      赤が2人とも持ち主だと思っている時刻、黄が誰も持ち主でない時刻。上下で違うのは猶予の長さだけで、
      切り離す時刻も期限も同じになっている。繋ぎ直すと、古い持ち主は次に置き場を読んだ瞬間に降りるので、
      重なりはそこで終わる。届かない間は降りる以外に取れる行動が無い、というのがこの仕組みの土台になっている。
    </p>
  </DemoShell>
</template>

<style scoped>
.le-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.le-spacer {
  flex: 1;
}
.le-hint {
  font-size: 10px;
  color: var(--vp-c-text-3);
}
.le-run {
  margin-top: 14px;
  border: 1px solid var(--vp-c-divider);
  padding: 10px 12px;
  background-color: var(--vp-c-bg-soft);
}
.le-run.bad {
  border-color: var(--vp-c-danger-1);
}
.le-run-h {
  display: flex;
  align-items: baseline;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 8px;
}
.le-title {
  font-size: 12px;
  font-weight: 700;
  color: var(--vp-c-text-1);
}
.le-counts {
  font-size: 10px;
  color: var(--vp-c-text-3);
  margin-left: auto;
}
.le-row {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 2px;
}
.le-name {
  width: 40px;
  flex: none;
  font-size: 10px;
  color: var(--vp-c-text-2);
}
.le-track {
  display: flex;
  gap: 1px;
  flex: 1;
  min-width: 0;
}
.le-cell {
  flex: 1;
  min-width: 2px;
  height: 13px;
  border: 1px solid transparent;
}
.c-idle {
  background-color: var(--vp-c-bg-alt);
  border-color: var(--vp-c-divider);
}
.c-lead {
  background-color: var(--vp-c-green-soft);
  border-color: var(--vp-c-green-1);
}
.c-lead-cut {
  background-color: var(--vp-c-warning-soft);
  border-color: var(--vp-c-warning-1);
}
.c-cut {
  background-color: transparent;
  border-color: var(--vp-c-divider);
  border-style: dashed;
}
.le-status {
  margin-top: 5px;
  padding-top: 5px;
  border-top: 1px solid var(--vp-c-divider);
}
.s-ok {
  background-color: var(--vp-c-bg-alt);
}
.s-vacant {
  background-color: var(--vp-c-warning-1);
}
.s-overlap {
  background-color: var(--vp-c-danger-1);
}
.le-scale {
  display: flex;
  justify-content: space-between;
  margin-left: 46px;
  margin-top: 3px;
  font-size: 9px;
  color: var(--vp-c-text-3);
}
.le-log {
  margin-top: 8px;
  border-top: 1px solid var(--vp-c-divider);
  padding-top: 6px;
}
.le-log-line {
  font-size: 10px;
  color: var(--vp-c-text-2);
  padding: 1px 0;
}
.le-verdict {
  margin-top: 12px;
  padding: 8px 12px;
  border-left: 3px solid var(--vp-c-danger-1);
  font-size: 12.5px;
  font-weight: 600;
  line-height: 1.6;
  color: var(--vp-c-danger-1);
  background-color: var(--vp-c-danger-soft);
}
.le-legend {
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
