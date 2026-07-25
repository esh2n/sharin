import { defineConfig } from "vitepress";

export default defineConfig({
  lang: "ja",
  title: "sharin",
  description: "車輪の再発明で学び直す教科書",
  themeConfig: {
    nav: [{ text: "教科書", link: "/parts/rate-limiter" }],
    sidebar: [
      {
        text: "トラフィック制御と観測",
        items: [
          { text: "Rate Limiter", link: "/parts/rate-limiter" },
          { text: "Trace Sampling", link: "/parts/trace-sampling" },
        ],
      },
      {
        text: "ネットワーク下層",
        items: [
          { text: "HTTPサーバ", link: "/parts/http-server" },
          { text: "DNSリゾルバ", link: "/parts/dns" },
          { text: "プロキシ", link: "/parts/proxy" },
        ],
      },
      {
        text: "データの持ち方",
        items: [
          { text: "ID Generation", link: "/parts/id-generation" },
          { text: "二分探索木", link: "/parts/binary-search-tree" },
          { text: "LRUキャッシュ", link: "/parts/lru-cache" },
          { text: "ディスクとページ", link: "/parts/disk-and-pages" },
          { text: "B-Tree", link: "/parts/btree" },
          { text: "ログ構造KV", link: "/parts/log-structured-kv" },
          { text: "WAL", link: "/parts/wal" },
          { text: "バッファプール", link: "/parts/buffer-pool" },
          { text: "B-Treeページストア", link: "/parts/btree-page-store" },
          { text: "B-Tree + WAL", link: "/parts/btree-wal" },
          { text: "ミニSQL", link: "/parts/mini-sql" },
          { text: "ハッシュマップ", link: "/parts/hash-map" },
          { text: "ブルームフィルタ", link: "/parts/bloom-filter" },
          { text: "スキップリスト", link: "/parts/skip-list" },
        ],
      },
      {
        text: "暗号と認証",
        items: [{ text: "暗号", link: "/parts/crypto" }],
      },
      {
        text: "LLMのなかみ",
        items: [{ text: "LLM Sampling", link: "/parts/llm-sampling" }],
      },
    ],
    socialLinks: [{ icon: "github", link: "https://github.com/esh2n/sharin" }],
    outline: { label: "この章の目次", level: [2, 3] },
    docFooter: { prev: "前の章", next: "次の章" },
    lastUpdated: { text: "最終更新" },
  },
});
