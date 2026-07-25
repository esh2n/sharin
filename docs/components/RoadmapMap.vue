<script setup lang="ts">
// トップページの全体図。パーツを groups 別に並べ、状態バッジと読む順を示す。
// PLAN.md のロードマップと同じ状態を反映する(手動同期)。
interface Part {
  name: string;
  link?: string;
  state: "done" | "next" | "todo";
  base?: boolean; // 前提章
}
interface Group {
  title: string;
  note?: string;
  parts: Part[];
}

const groups: Group[] = [
  {
    title: "トラフィック制御と観測",
    note: "何を通すか / 何を残すか",
    parts: [
      { name: "Rate Limiter", link: "/parts/rate-limiter", state: "done" },
      { name: "Trace Sampling", link: "/parts/trace-sampling", state: "done" },
      { name: "Proxy", state: "todo" },
    ],
  },
  {
    title: "ネットワーク下層",
    note: "ソケットから積み上げる",
    parts: [
      { name: "TCP/IP 自作", state: "todo" },
      { name: "HTTP サーバ", state: "todo" },
      { name: "DNS リゾルバ", state: "todo" },
    ],
  },
  {
    title: "データの持ち方",
    note: "DB を部品から組み上げる",
    parts: [
      { name: "ID Generation", link: "/parts/id-generation", state: "done" },
      { name: "二分探索木", link: "/parts/binary-search-tree", state: "done", base: true },
      { name: "LRUキャッシュ", link: "/parts/lru-cache", state: "done", base: true },
      { name: "ディスクとページ", link: "/parts/disk-and-pages", state: "done", base: true },
      { name: "B-Tree", link: "/parts/btree", state: "done" },
      { name: "ログ構造KV", link: "/parts/log-structured-kv", state: "done" },
      { name: "WAL", link: "/parts/wal", state: "done" },
      { name: "バッファプール", link: "/parts/buffer-pool", state: "done" },
      { name: "B-Treeページストア", state: "next" },
      { name: "B-Tree + WAL 統合", state: "next" },
      { name: "ミニSQL", state: "next" },
      { name: "hash map", state: "todo" },
      { name: "bloom filter", state: "todo" },
      { name: "skip list", state: "todo" },
    ],
  },
  {
    title: "分散システム",
    note: "db 編の延長",
    parts: [
      { name: "分散合意 (Raft)", state: "todo" },
      { name: "レプリケーション", state: "todo" },
      { name: "consistent hashing", state: "todo" },
      { name: "分散ロック", state: "todo" },
    ],
  },
  {
    title: "メッセージングとRPC",
    note: "サービス間通信",
    parts: [
      { name: "message queue", state: "todo" },
      { name: "pub/sub", state: "todo" },
      { name: "protobuf・gRPC 自作", state: "todo" },
    ],
  },
  {
    title: "暗号と認証",
    parts: [
      { name: "Crypto", state: "todo" },
      { name: "Auth", state: "todo" },
      { name: "Blockchain", state: "todo" },
    ],
  },
  {
    title: "LLMのなかみ",
    parts: [
      { name: "LLM Sampling", link: "/parts/llm-sampling", state: "done" },
      { name: "LLM (tensorから)", state: "todo" },
    ],
  },
  {
    title: "画面が出るまで",
    parts: [
      { name: "仮想DOM", state: "todo" },
      { name: "mini Next", state: "todo" },
      { name: "ブラウザ", state: "todo" },
    ],
  },
  {
    title: "ランタイム内部",
    note: "言語処理系の裏側",
    parts: [
      { name: "ガベージコレクタ", state: "todo" },
      { name: "goroutine スケジューラ", state: "todo" },
      { name: "イベントループ", state: "todo" },
      { name: "WASM", state: "todo" },
    ],
  },
  {
    title: "計算機の土台",
    parts: [
      { name: "自作言語", state: "todo" },
      { name: "コンテナ", state: "todo" },
      { name: "自作OS", state: "todo" },
    ],
  },
];
</script>

<template>
  <div class="rm">
    <h2 class="rm-title">全体地図</h2>
    <p class="rm-legend">
      <span class="rm-dot done"></span>公開済み
      <span class="rm-dot next"></span>制作中
      <span class="rm-dot todo"></span>予定
      <span class="rm-base-mark">前</span>= 前提章
    </p>
    <div class="rm-groups">
      <section v-for="g in groups" :key="g.title" class="rm-group">
        <h3 class="rm-group-title">{{ g.title }}</h3>
        <p v-if="g.note" class="rm-group-note">{{ g.note }}</p>
        <ul class="rm-parts">
          <li v-for="p in g.parts" :key="p.name">
            <a v-if="p.link" :href="p.link" class="rm-part" :class="p.state">
              <span class="rm-part-dot" :class="p.state"></span>
              <span class="rm-part-name">{{ p.name }}</span>
              <span v-if="p.base" class="rm-base-mark">前</span>
            </a>
            <span v-else class="rm-part" :class="p.state">
              <span class="rm-part-dot" :class="p.state"></span>
              <span class="rm-part-name">{{ p.name }}</span>
            </span>
          </li>
        </ul>
      </section>
    </div>
  </div>
</template>

<style scoped>
.rm {
  max-width: 1152px;
  margin: 24px auto 0;
  padding: 0 24px;
}
.rm-title {
  margin: 0;
  font-size: 22px;
  font-weight: 700;
  border: none;
  padding: 0;
}
.rm-legend {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 8px 0 16px;
  font-size: 12px;
  color: var(--vp-c-text-2);
}
.rm-legend .rm-dot:not(:first-child) {
  margin-left: 12px;
}
.rm-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 4px;
}
.rm-dot.done,
.rm-part-dot.done {
  background-color: var(--vp-c-green-1);
}
.rm-dot.next,
.rm-part-dot.next {
  background-color: var(--vp-c-yellow-1);
}
.rm-dot.todo,
.rm-part-dot.todo {
  background-color: var(--vp-c-gray);
}
.rm-groups {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(230px, 1fr));
  gap: 16px;
}
.rm-group {
  padding: 14px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 10px;
  background-color: var(--vp-c-bg-soft);
}
.rm-group-title {
  margin: 0;
  font-size: 14px;
  font-weight: 700;
  border: none;
  padding: 0;
}
.rm-group-note {
  margin: 2px 0 10px;
  font-size: 12px;
  color: var(--vp-c-text-3);
}
.rm-parts {
  list-style: none;
  margin: 8px 0 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.rm-part {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 6px;
  border-radius: 6px;
  font-size: 13px;
  color: var(--vp-c-text-1);
  text-decoration: none;
}
a.rm-part:hover {
  background-color: var(--vp-c-default-soft);
}
.rm-part.todo,
.rm-part.next {
  color: var(--vp-c-text-3);
}
.rm-part-dot {
  flex-shrink: 0;
  width: 8px;
  height: 8px;
  border-radius: 50%;
}
.rm-part-name {
  flex: 1;
}
.rm-base-mark {
  padding: 0 5px;
  border-radius: 4px;
  background-color: var(--vp-c-default-soft);
  font-size: 10px;
  font-weight: 700;
  color: var(--vp-c-text-2);
}
</style>
