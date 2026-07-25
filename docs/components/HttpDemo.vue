<script setup lang="ts">
import { computed, ref } from "vue";
import DemoShell from "./DemoShell.vue";

// Go 版 httpserver の ParseRequest 相当を JS で。生テキスト → 構造 → レスポンス。
const PRESETS: Record<string, string> = {
  GET: "GET /hello?name=world HTTP/1.1\r\nHost: example.com\r\nUser-Agent: demo\r\n\r\n",
  POST: "POST /echo HTTP/1.1\r\nHost: example.com\r\nContent-Length: 7\r\n\r\npayload",
  壊れた: "GARBAGE REQUEST\r\n\r\n",
};

const raw = ref(PRESETS.GET);

interface Parsed {
  method: string;
  path: string;
  version: string;
  query: [string, string][];
  headers: [string, string][];
  body: string;
}

const parsed = computed<{ ok: true; req: Parsed } | { ok: false; err: string }>(() => {
  // textarea は改行を \n に正規化するので、一度 \n に統一してから \r\n に揃える。
  const text = raw.value.replace(/\r\n|\r|\n/g, "\n").replace(/\n/g, "\r\n");
  const idx = text.indexOf("\r\n\r\n");
  const head = idx >= 0 ? text.slice(0, idx) : text;
  const body = idx >= 0 ? text.slice(idx + 4) : "";
  const lines = head.split("\r\n");
  const parts = lines[0].trim().split(/\s+/);
  if (parts.length !== 3) return { ok: false, err: `リクエストラインが不正: "${lines[0]}"` };
  const [method, target, version] = parts;
  const [path, qs] = target.split("?");
  const query: [string, string][] = qs
    ? qs.split("&").map((p) => {
        const [k, v = ""] = p.split("=");
        return [k, v];
      })
    : [];
  const headers: [string, string][] = [];
  for (const line of lines.slice(1)) {
    const c = line.indexOf(":");
    if (c < 0) return { ok: false, err: `ヘッダが不正: "${line}"` };
    headers.push([line.slice(0, c).trim(), line.slice(c + 1).trim()]);
  }
  return { ok: true, req: { method, path, version, query, headers, body } };
});

const response = computed(() => {
  if (!parsed.value.ok) return "HTTP/1.1 400 Bad Request\r\nContent-Length: 11\r\n\r\nbad request";
  const req = parsed.value.req;
  let bodyText: string;
  let status = 200;
  if (req.path === "/hello") {
    const name = req.query.find(([k]) => k === "name")?.[1] ?? "";
    bodyText = `hello, ${name}`;
  } else if (req.path === "/echo") {
    bodyText = req.body;
  } else {
    status = 404;
    bodyText = "not found";
  }
  const statusText = status === 200 ? "OK" : "Not Found";
  return `HTTP/1.1 ${status} ${statusText}\r\nContent-Type: text/plain\r\nContent-Length: ${new TextEncoder().encode(bodyText).length}\r\n\r\n${bodyText}`;
});

function usePreset(k: string) {
  raw.value = PRESETS[k];
}
</script>

<template>
  <DemoShell title="HTTP リクエストのパース" :badge="parsed.ok ? '200/404' : '400'" :badge-tone="parsed.ok ? 'ok' : 'ng'">
    <div class="sd-controls">
      <span class="ht-caption">例:</span>
      <button v-for="k in Object.keys(PRESETS)" :key="k" class="sd-btn sd-btn--primary" type="button" @click="usePreset(k)">
        {{ k }}
      </button>
    </div>

    <div class="ht-grid">
      <div class="ht-col">
        <p class="ht-head">1. 生のリクエスト(TCP で届くテキスト)</p>
        <textarea v-model="raw" class="ht-raw" spellcheck="false" rows="6"></textarea>
      </div>
      <div class="ht-col">
        <p class="ht-head">2. パースした構造</p>
        <div v-if="parsed.ok" class="ht-parsed">
          <div class="ht-line">
            <span class="ht-tag method">{{ parsed.req.method }}</span>
            <span class="ht-tag path">{{ parsed.req.path }}</span>
            <span class="ht-tag ver">{{ parsed.req.version }}</span>
          </div>
          <div v-if="parsed.req.query.length" class="ht-kv-group">
            <span class="ht-kv-label">query</span>
            <span v-for="[k, v] in parsed.req.query" :key="k" class="ht-kv">{{ k }}={{ v }}</span>
          </div>
          <div class="ht-kv-group">
            <span class="ht-kv-label">headers</span>
            <span v-for="[k, v] in parsed.req.headers" :key="k" class="ht-kv">{{ k }}: {{ v }}</span>
          </div>
          <div v-if="parsed.req.body" class="ht-kv-group">
            <span class="ht-kv-label">body</span>
            <span class="ht-kv">{{ parsed.req.body }}</span>
          </div>
        </div>
        <p v-else class="ht-err">{{ parsed.err }}</p>
      </div>
      <div class="ht-col">
        <p class="ht-head">3. 返すレスポンス</p>
        <pre class="ht-resp">{{ response }}</pre>
      </div>
    </div>
  </DemoShell>
</template>

<style scoped>
.ht-caption {
  font-size: 12px;
  font-weight: 600;
  color: var(--vp-c-text-2);
}
.ht-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px;
  margin-top: 14px;
}
@media (max-width: 780px) {
  .ht-grid {
    grid-template-columns: 1fr;
  }
}
.ht-col {
  min-width: 0;
}
.ht-head {
  margin: 0 0 6px;
  font-size: 12px;
  font-weight: 700;
  color: var(--vp-c-text-2);
}
.ht-raw {
  width: 100%;
  padding: 8px 10px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 6px;
  background-color: var(--vp-c-bg);
  color: var(--vp-c-text-1);
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  resize: vertical;
}
.ht-parsed {
  border: 1px solid var(--vp-c-divider);
  border-radius: 6px;
  background-color: var(--vp-c-bg);
  padding: 8px 10px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.ht-line {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}
.ht-tag {
  padding: 2px 7px;
  border-radius: 4px;
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  background-color: var(--vp-c-default-soft);
}
.ht-tag.method {
  background-color: var(--vp-c-brand-soft);
  color: var(--vp-c-brand-1);
  font-weight: 700;
}
.ht-tag.path {
  color: var(--vp-c-green-1);
}
.ht-kv-group {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  align-items: baseline;
}
.ht-kv-label {
  font-size: 10px;
  font-weight: 700;
  color: var(--vp-c-text-3);
  min-width: 48px;
}
.ht-kv {
  padding: 1px 6px;
  border-radius: 4px;
  background-color: var(--vp-c-default-soft);
  font-family: var(--vp-font-family-mono);
  font-size: 11px;
}
.ht-err {
  margin: 0;
  padding: 8px 10px;
  border: 1px solid var(--vp-c-danger-1);
  border-radius: 6px;
  color: var(--vp-c-danger-1);
  font-size: 12px;
}
.ht-resp {
  margin: 0;
  padding: 8px 10px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 6px;
  background-color: var(--vp-c-bg);
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
