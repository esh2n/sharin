import { describe, it, expect } from "vitest";
import { parseHTML, type Element } from "./html";
import { parseCSS, specificity } from "./css";
import { styleTree, display } from "./style";
import { layout } from "./layout";
import { paint } from "./paint";
import { render } from "./pipeline";

describe("parseHTML（HTML文字列 → DOM木）", () => {
  it("属性つき要素とテキストをパースする", () => {
    const dom = parseHTML('<div id="a" class="box">hi</div>') as Element;
    expect(dom.kind).toBe("element");
    expect(dom.tag).toBe("div");
    expect(dom.attrs).toEqual({ id: "a", class: "box" });
    expect(dom.children).toEqual([{ kind: "text", text: "hi" }]);
  });

  it("入れ子をパースする", () => {
    const dom = parseHTML("<ul><li>a</li><li>b</li></ul>") as Element;
    expect(dom.children).toHaveLength(2);
    expect((dom.children[0] as Element).tag).toBe("li");
    expect((dom.children[0] as Element).children[0]).toEqual({ kind: "text", text: "a" });
  });

  it("複数ルートは暗黙の <html> で包む", () => {
    const dom = parseHTML("<p>a</p><p>b</p>") as Element;
    expect(dom.tag).toBe("html");
    expect(dom.children).toHaveLength(2);
  });

  it("空白は要素間で無視される", () => {
    const dom = parseHTML("<div>\n  <span>x</span>\n</div>") as Element;
    expect(dom.children).toHaveLength(1);
    expect((dom.children[0] as Element).tag).toBe("span");
  });

  it("壊れた閉じ忘れは例外", () => {
    expect(() => parseHTML("<div>")).toThrow();
  });
});

describe("parseCSS（CSS文字列 → 規則）", () => {
  it("セレクタと宣言をパースする", () => {
    const sheet = parseCSS("h1, .note { color: red; margin: 10px; }");
    expect(sheet.rules).toHaveLength(1);
    expect(sheet.rules[0]!.declarations).toEqual([
      { name: "color", value: "red" },
      { name: "margin", value: "10px" },
    ]);
  });

  it("単純セレクタ tag/.class/#id を分解する", () => {
    const sheet = parseCSS("div.note#main { color: blue; }");
    expect(sheet.rules[0]!.selectors[0]).toEqual({ tag: "div", id: "main", classes: ["note"] });
  });

  it("詳細度は id > class > tag", () => {
    expect(specificity({ id: "x", classes: [] })).toEqual([1, 0, 0]);
    expect(specificity({ classes: ["a", "b"], tag: "div" })).toEqual([0, 2, 1]);
  });

  it("複数規則をパースする", () => {
    const sheet = parseCSS("a { color: red; } b { color: blue; }");
    expect(sheet.rules).toHaveLength(2);
  });
});

describe("styleTree（DOM × CSS → スタイル木・カスケード）", () => {
  it("マッチした宣言を要素に貼る", () => {
    const dom = parseHTML('<div class="box">x</div>');
    const sheet = parseCSS(".box { color: red; }");
    const styled = styleTree(dom, sheet);
    expect(styled.specified.color).toBe("red");
  });

  it("詳細度の高い規則が勝つ（カスケード）", () => {
    const dom = parseHTML('<div id="m" class="box">x</div>');
    const sheet = parseCSS("div { color: black; } .box { color: red; } #m { color: green; }");
    const styled = styleTree(dom, sheet);
    expect(styled.specified.color).toBe("green"); // #id が最優先
  });

  it("display:none / inline / 既定block を判定する", () => {
    const dom = parseHTML('<div class="h"><span class="i">x</span></div>');
    const sheet = parseCSS(".h { display: none; } .i { display: inline; }");
    const styled = styleTree(dom, sheet);
    expect(display(styled)).toBe("none");
    expect(display(styled.children[0]!)).toBe("inline");
  });

  it("要素の既定は block、テキストの既定は inline", () => {
    const styled = styleTree(parseHTML("<div>x</div>"), parseCSS(""));
    expect(display(styled)).toBe("block");
    expect(display(styled.children[0]!)).toBe("inline");
  });
});

describe("layout（ブロックレイアウト: 位置と大きさ）", () => {
  it("ブロックは親幅を満たす", () => {
    const box = layout(styleTree(parseHTML("<div></div>"), parseCSS("")), 800);
    expect(box.rect.width).toBe(800);
    expect(box.rect.x).toBe(0);
  });

  it("margin ぶん内側にずれ、幅が縮む", () => {
    const box = layout(styleTree(parseHTML("<div></div>"), parseCSS("div { margin: 20px; }")), 800);
    expect(box.rect.x).toBe(20);
    expect(box.rect.width).toBe(800 - 40);
  });

  it("子ブロックは縦に積まれる", () => {
    const dom = parseHTML("<div><p>a</p><p>b</p></div>");
    const box = layout(styleTree(dom, parseCSS("p { height: 30px; }")), 400);
    const [p1, p2] = box.children;
    expect(p1!.rect.y).toBe(0);
    expect(p2!.rect.y).toBe(30); // p1 の下
    expect(box.rect.height).toBe(60); // 子の合計
  });

  it("padding は自分の高さを増やし、子を内側へ寄せる", () => {
    const dom = parseHTML("<div><p>x</p></div>");
    const box = layout(styleTree(dom, parseCSS("div { padding: 10px; } p { height: 20px; }")), 400);
    expect(box.children[0]!.rect.x).toBe(10); // padding ぶん内側
    expect(box.rect.height).toBe(20 + 20); // 子20 + padding上下20
  });

  it("display:none の子は積まれない", () => {
    const dom = parseHTML('<div><p class="g">a</p><p>b</p></div>');
    const box = layout(styleTree(dom, parseCSS(".g { display: none; } p { height: 10px; }")), 400);
    expect(box.children).toHaveLength(1);
    expect(box.rect.height).toBe(10);
  });
});

describe("paint（レイアウト → 描画コマンド）", () => {
  it("背景色つきブロックは矩形コマンドになる", () => {
    const box = layout(styleTree(parseHTML("<div></div>"), parseCSS("div { background: #f00; height: 50px; }")), 100);
    const cmds = paint(box);
    expect(cmds).toContainEqual({ type: "rect", x: 0, y: 0, width: 100, height: 50, color: "#f00" });
  });

  it("テキストは text コマンドになる", () => {
    const cmds = paint(layout(styleTree(parseHTML("<div>hello</div>"), parseCSS("div{color:blue}")), 100));
    const t = cmds.find((c) => c.type === "text");
    expect(t).toMatchObject({ type: "text", text: "hello", color: "blue" });
  });

  it("親→子の順にコマンドが並ぶ（子が手前）", () => {
    const dom = parseHTML('<div class="bg"><span class="fg"></span></div>');
    const css = parseCSS(".bg { background: white; height: 40px; } .fg { background: black; height: 10px; }");
    const cmds = paint(layout(styleTree(dom, css), 100)).filter((c) => c.type === "rect");
    expect(cmds[0]!.color).toBe("white"); // 親の背景が先
    expect(cmds[1]!.color).toBe("black"); // 子が後（手前）
  });
});

describe("render（パイプライン全体）", () => {
  it("HTML + CSS から描画コマンド一覧を作る", () => {
    const html = '<div class="card"><h1>Title</h1><p>body</p></div>';
    const css = ".card { background: #eee; padding: 8px; } h1 { height: 24px; background: #fff; } p { height: 16px; }";
    const { layout: box, display: cmds } = render(html, css, 320);

    expect(box.rect.width).toBe(320);
    // card = padding上下16 + h1(24) + p(16) = 56
    expect(box.rect.height).toBe(24 + 16 + 16);
    // 背景矩形が2つ（card, h1）、テキストが2つ（Title, body）
    expect(cmds.filter((c) => c.type === "rect")).toHaveLength(2);
    expect(cmds.filter((c) => c.type === "text").map((c) => (c.type === "text" ? c.text : ""))).toEqual(["Title", "body"]);
  });
});
