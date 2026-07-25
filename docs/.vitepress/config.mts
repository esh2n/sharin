import { defineConfig } from "vitepress";

export default defineConfig({
  lang: "ja",
  title: "sharin",
  description: "車輪の再発明で学び直す教科書",
  themeConfig: {
    nav: [
      { text: "教科書", link: "/parts/rate-limiter" },
      { text: "バックログ", link: "/backlog" },
    ],
    sidebar: [
      {
        text: "Tier 1: 小さく完結する",
        items: [
          { text: "Rate Limiter", link: "/parts/rate-limiter" },
          { text: "Sampling", link: "/parts/sampling" },
        ],
      },
    ],
    socialLinks: [{ icon: "github", link: "https://github.com/esh2n/sharin" }],
    outline: { label: "この章の目次", level: [2, 3] },
    docFooter: { prev: "前の章", next: "次の章" },
    lastUpdated: { text: "最終更新" },
  },
});
