import { describe, it, expect, vi } from "vitest";
import { h } from "../vdom/vdom";
import { renderToString } from "./ssr";
import { createRouter } from "./router";
import { hydrate } from "./hydrate";
import { renderRoute, hydrateRoute } from "./app";

describe("renderToString（SSR: 仮想DOM → HTML文字列）", () => {
  it("属性つき要素と子をHTMLにする", () => {
    const html = renderToString(h("div", { id: "a", class: "c" }, h("span", null, "hi")));
    expect(html).toBe('<div id="a" class="c"><span>hi</span></div>');
  });

  it("テキストをエスケープする（XSS対策の初歩）", () => {
    const html = renderToString(h("p", null, "<script>&\"x\""));
    expect(html).toBe("<p>&lt;script&gt;&amp;\"x\"</p>");
  });

  it("属性値をエスケープする", () => {
    const html = renderToString(h("a", { title: 'a"b&c' }, "x"));
    expect(html).toBe('<a title="a&quot;b&amp;c">x</a>');
  });

  it("イベントプロップは出力しない（関数は文字列化できない）", () => {
    const html = renderToString(h("button", { onClick: () => {}, id: "b" }, "ok"));
    expect(html).toBe('<button id="b">ok</button>');
  });

  it("値が false/null の属性は出さない", () => {
    const html = renderToString(h("input", { disabled: false, required: true }));
    expect(html).toBe('<input required="true">');
  });

  it("void要素は閉じタグを出さない", () => {
    expect(renderToString(h("br", null))).toBe("<br>");
    expect(renderToString(h("img", { src: "/a.png" }))).toBe('<img src="/a.png">');
  });
});

describe("createRouter（URL → ページ）", () => {
  const home = () => h("h1", null, "home");
  const post = (p: Record<string, string>) => h("h1", null, "post " + p.id);

  it("静的パスを解決する", () => {
    const r = createRouter([{ path: "/", page: home }]);
    const m = r.resolve("/");
    expect(m).not.toBeNull();
    expect(renderToString(m!.page(m!.params))).toBe("<h1>home</h1>");
  });

  it("動的セグメント :id を params に取り出す", () => {
    const r = createRouter([{ path: "/posts/:id", page: post }]);
    const m = r.resolve("/posts/42");
    expect(m!.params).toEqual({ id: "42" });
    expect(renderToString(m!.page(m!.params))).toBe("<h1>post 42</h1>");
  });

  it("クエリ文字列を無視してパスだけ見る", () => {
    const r = createRouter([{ path: "/posts/:id", page: post }]);
    expect(r.resolve("/posts/7?ref=x")!.params).toEqual({ id: "7" });
  });

  it("最初に一致した route を採用する（登録順）", () => {
    const a = () => h("p", null, "A");
    const b = () => h("p", null, "B");
    const r = createRouter([
      { path: "/x", page: a },
      { path: "/:slug", page: b },
    ]);
    expect(renderToString(r.resolve("/x")!.page({}))).toBe("<p>A</p>");
  });

  it("未登録パスは null", () => {
    const r = createRouter([{ path: "/", page: home }]);
    expect(r.resolve("/missing")).toBeNull();
  });
});

describe("hydrate（サーバHTMLにイベントを後付け）", () => {
  it("既存DOMを作り直さず、リスナだけを付ける", () => {
    const onClick = vi.fn();
    const tree = h("div", null, h("button", { onClick, id: "b" }, "ok"));
    // サーバが吐いた HTML を container に流し込む（実DOMは既にある状態を再現）
    const container = document.createElement("div");
    container.innerHTML = renderToString(tree);
    const serverButton = container.querySelector("#b");

    hydrate(tree, container.firstChild!);

    expect(container.querySelector("#b")).toBe(serverButton); // 同一ノード（作り直していない）
    (serverButton as HTMLElement).dispatchEvent(new Event("click"));
    expect(onClick).toHaveBeenCalledOnce();
  });

  it("入れ子の子にもリスナを付ける", () => {
    const outer = vi.fn();
    const inner = vi.fn();
    const tree = h("section", { onClick: outer }, h("button", { onClick: inner }, "x"));
    const container = document.createElement("div");
    container.innerHTML = renderToString(tree);
    hydrate(tree, container.firstChild!);

    container.querySelector("button")!.dispatchEvent(new Event("click", { bubbles: true }));
    expect(inner).toHaveBeenCalledOnce();
  });
});

describe("app（SSR → hydrate の一連の流れ）", () => {
  const routes = [
    { path: "/", page: () => h("main", null, h("h1", null, "top")) },
    {
      path: "/count",
      page: () => {
        const onClick = vi.fn();
        return h("button", { onClick, id: "c" }, "click");
      },
    },
  ];

  it("renderRoute はパスに対応する HTML を返す", () => {
    const r = createRouter(routes);
    expect(renderRoute(r, "/")).toBe("<main><h1>top</h1></main>");
  });

  it("renderRoute は未一致パスで空文字（404 は呼び出し側の責務）", () => {
    const r = createRouter(routes);
    expect(renderRoute(r, "/nope")).toBe("");
  });

  it("hydrateRoute は未一致パスなら何もしない", () => {
    const r = createRouter(routes);
    const container = document.createElement("div");
    container.innerHTML = "<main></main>";
    expect(() => hydrateRoute(r, "/nope", container)).not.toThrow();
  });

  it("hydrateRoute はサーバHTMLの上でイベントを有効化する", () => {
    let clicked = 0;
    const r = createRouter([
      { path: "/", page: () => h("button", { onClick: () => clicked++, id: "b" }, "x") },
    ]);
    const container = document.createElement("div");
    container.innerHTML = renderRoute(r, "/"); // サーバ描画を模す
    hydrateRoute(r, "/", container);
    container.querySelector("#b")!.dispatchEvent(new Event("click"));
    expect(clicked).toBe(1);
  });
});
