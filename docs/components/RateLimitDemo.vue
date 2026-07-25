<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

const props = defineProps<{ algo: string }>();

const DEMO_BASE = "https://sharin-ratelimit-demo.esh2n.workers.dev/check";

const ALGO_LABEL: Record<string, string> = {
  "token-bucket": "Token Bucket",
  "leaky-bucket": "Leaky Bucket",
  "fixed-window": "Fixed Window",
  "sliding-window-log": "Sliding Window Log",
};

interface Entry {
  at: string;
  status: number | string;
  remaining: number | string;
  retryAfterMs: number | string;
}

const log = ref<Entry[]>([]);
const remaining = ref<number | null>(null);
const busy = ref(false);

const badge = computed(() => (remaining.value === null ? undefined : `残り ${remaining.value}`));
const badgeTone = computed(() => (remaining.value === null ? "neutral" : remaining.value > 0 ? "ok" : "ng"));

async function fire() {
  busy.value = true;
  const at = new Date().toLocaleTimeString("ja-JP", { hour12: false });
  try {
    const res = await fetch(`${DEMO_BASE}?algo=${props.algo}`);
    const body = await res.json();
    remaining.value = body.remaining;
    log.value = [
      { at, status: res.status, remaining: body.remaining, retryAfterMs: body.retryAfterMs },
      ...log.value,
    ].slice(0, 10);
  } catch {
    log.value = [{ at, status: "error", remaining: "-", retryAfterMs: "-" }, ...log.value].slice(0, 10);
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <DemoShell :title="ALGO_LABEL[algo] ?? algo" :badge="badge" :badge-tone="badgeTone">
    <div class="sd-controls">
      <button class="sd-btn sd-btn--primary" type="button" :disabled="busy" @click="fire">
        リクエストを送る
      </button>
      <div v-if="remaining !== null" class="rl-meter" aria-hidden="true">
        <span v-for="i in 5" :key="i" class="rl-dot" :class="{ on: i <= (remaining ?? 0) }" />
      </div>
    </div>
    <table v-if="log.length" class="rl-log">
      <thead>
        <tr>
          <th>時刻</th>
          <th>結果</th>
          <th>残り</th>
          <th>次に通るまで</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(e, i) in log" :key="log.length - i">
          <td>{{ e.at }}</td>
          <td>
            <span :class="e.status === 200 ? 'rl-ok' : 'rl-ng'">{{ e.status }}</span>
          </td>
          <td>{{ e.remaining }}</td>
          <td>{{ typeof e.retryAfterMs === "number" && e.retryAfterMs > 0 ? `${e.retryAfterMs} ms` : "-" }}</td>
        </tr>
      </tbody>
    </table>
  </DemoShell>
</template>

<style scoped>
.rl-meter {
  display: flex;
  align-items: center;
  gap: 5px;
}
.rl-dot {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background-color: var(--vp-c-default-soft);
  border: 1px solid var(--vp-c-divider);
}
.rl-dot.on {
  background-color: var(--vp-c-brand-1);
  border-color: var(--vp-c-brand-1);
}
.rl-log {
  margin: 12px 0 0;
  font-size: 13px;
  display: table;
  width: 100%;
}
.rl-log th,
.rl-log td {
  padding: 4px 12px;
  text-align: left;
}
.rl-log th {
  color: var(--vp-c-text-2);
  font-weight: 600;
  border-bottom: 1px solid var(--vp-c-divider);
}
.rl-ok {
  color: var(--vp-c-green-1);
  font-weight: 600;
}
.rl-ng {
  color: var(--vp-c-danger-1);
  font-weight: 600;
}
</style>
