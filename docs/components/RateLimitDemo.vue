<script setup lang="ts">
import { ref } from "vue";

const props = defineProps<{ algo: string }>();

const DEMO_BASE = "https://sharin-ratelimit-demo.esh2n.workers.dev/check";

interface Entry {
  at: string;
  status: number | string;
  remaining: number | string;
  retryAfterMs: number | string;
}

const log = ref<Entry[]>([]);
const remaining = ref<number | null>(null);
const busy = ref(false);

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
  <div class="rl-demo">
    <div class="rl-head">
      <button class="rl-fire" type="button" :disabled="busy" @click="fire">
        リクエストを送る
      </button>
      <div v-if="remaining !== null" class="rl-meter" aria-hidden="true">
        <span v-for="i in 5" :key="i" class="rl-dot" :class="{ on: i <= (remaining ?? 0) }" />
        <span class="rl-meter-label">残り {{ remaining }}</span>
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
  </div>
</template>

<style scoped>
.rl-demo {
  margin: 16px 0 24px;
  padding: 16px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  background-color: var(--vp-c-bg-soft);
}
.rl-head {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
}
.rl-fire {
  padding: 8px 20px;
  border-radius: 6px;
  font-weight: 600;
  color: var(--vp-button-brand-text);
  background-color: var(--vp-button-brand-bg);
  transition: background-color 0.2s;
}
.rl-fire:hover {
  background-color: var(--vp-button-brand-hover-bg);
}
.rl-fire:disabled {
  opacity: 0.6;
}
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
.rl-meter-label {
  margin-left: 6px;
  font-size: 13px;
  color: var(--vp-c-text-2);
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
