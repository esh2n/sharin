import { describe, it, expect, vi } from "vitest";
import { h, mount, diff, type VNode } from "./vdom";

// 実DOMに mount してから、その要素へ patch を当てて結果を検証する。
function setup(vnode: VNode): { root: HTMLElement; node: Node } {
  const root = document.createElement("div");
  const node = mount(vnode);
  root.appendChild(node);
  return { root, node };
}

describe("h（仮想DOMの組み立て）", () => {
  it("タグ・props・子を持つ VElement を作る", () => {
    const v = h("div", { id: "a" }, "hello");
    expect(v).toEqual({
      kind: "element",
      type: "div",
      props: { id: "a" },
      children: [{ kind: "text", text: "hello" }],
    });
  });

  it("文字列と数値の子はテキストノードに正規化される", () => {
    const v = h("p", null, "x", 42);
    expect(v.children).toEqual([
      { kind: "text", text: "x" },
      { kind: "text", text: "42" },
    ]);
  });

  it("null・undefined・false の子は捨てる（条件付きレンダリング用）", () => {
    const show = false;
    const v = h("ul", null, h("li", null, "a"), show && h("li", null, "b"), null, undefined);
    expect(v.children).toHaveLength(1);
    expect(v.children[0]).toMatchObject({ type: "li" });
  });

  it("配列の子は平坦化される（map の結果をそのまま渡せる）", () => {
    const items = ["a", "b"];
    const v = h("ul", null, items.map((t) => h("li", null, t)));
    expect(v.children).toHaveLength(2);
  });
});

describe("mount（仮想DOM → 実DOM）", () => {
  it("属性つき要素とテキストを実DOMにする", () => {
    const { node } = setup(h("div", { id: "box", class: "c" }, "hi"));
    const el = node as HTMLElement;
    expect(el.tagName).toBe("DIV");
    expect(el.id).toBe("box");
    expect(el.getAttribute("class")).toBe("c");
    expect(el.textContent).toBe("hi");
  });

  it("入れ子を再帰的に mount する", () => {
    const { node } = setup(h("ul", null, h("li", null, "a"), h("li", null, "b")));
    const el = node as HTMLElement;
    expect(el.querySelectorAll("li")).toHaveLength(2);
    expect(el.textContent).toBe("ab");
  });

  it("値が false の属性は付けない（条件付き属性）", () => {
    const { node } = setup(h("button", { disabled: false, id: "b" }, "x"));
    const el = node as HTMLElement;
    expect(el.hasAttribute("disabled")).toBe(false);
    expect(el.id).toBe("b");
  });

  it("on* プロップはイベントリスナになる", () => {
    const onClick = vi.fn();
    const { node } = setup(h("button", { onClick }, "ok"));
    (node as HTMLElement).dispatchEvent(new Event("click"));
    expect(onClick).toHaveBeenCalledOnce();
  });
});

describe("diff / patch（最小差分の適用）", () => {
  it("変化なしなら同じ実ノードを返し、子ノードも作り直さない", () => {
    const a = h("div", { id: "x" }, h("span", null, "same"));
    const { node } = setup(a);
    const child = node.firstChild; // 目印: 作り直したら別インスタンスになる

    const patched = diff(a, h("div", { id: "x" }, h("span", null, "same")))(node);
    expect(patched).toBe(node);
    expect(node.firstChild).toBe(child); // 同じ span 実ノードを使い回している
  });

  it("テキストだけ変わったらテキストノードを書き換える", () => {
    const before = h("p", null, "old");
    const { node } = setup(before);
    const textNode = node.firstChild;
    diff(before, h("p", null, "new"))(node);
    expect(node.textContent).toBe("new");
    expect(node.firstChild).toBe(textNode); // 差し替えでなく更新
  });

  it("属性の追加・変更・削除を反映する", () => {
    const before = h("div", { id: "a", title: "t" });
    const { node } = setup(before);
    diff(before, h("div", { id: "b", class: "c" }))(node);
    const el = node as HTMLElement;
    expect(el.id).toBe("b"); // 変更
    expect(el.getAttribute("class")).toBe("c"); // 追加
    expect(el.hasAttribute("title")).toBe(false); // 削除
  });

  it("タグが変わったら丸ごと差し替える", () => {
    const before = h("div", null, "x");
    const { root, node } = setup(before);
    const patched = diff(before, h("span", null, "x"))(node);
    expect((patched as HTMLElement).tagName).toBe("SPAN");
    expect(root.firstChild).toBe(patched);
    expect(root.querySelector("div")).toBeNull();
  });

  it("子の追加を反映する", () => {
    const before = h("ul", null, h("li", null, "a"));
    const { node } = setup(before);
    diff(before, h("ul", null, h("li", null, "a"), h("li", null, "b")))(node);
    expect((node as HTMLElement).querySelectorAll("li")).toHaveLength(2);
    expect(node.textContent).toBe("ab");
  });

  it("子の削除を反映する（末尾から消える）", () => {
    const before = h("ul", null, h("li", null, "a"), h("li", null, "b"));
    const { node } = setup(before);
    diff(before, h("ul", null, h("li", null, "a")))(node);
    expect((node as HTMLElement).querySelectorAll("li")).toHaveLength(1);
    expect(node.textContent).toBe("a");
  });

  it("イベントリスナを差し替える（古いのは外す）", () => {
    const oldClick = vi.fn();
    const newClick = vi.fn();
    const before = h("button", { onClick: oldClick }, "x");
    const { node } = setup(before);
    diff(before, h("button", { onClick: newClick }, "x"))(node);
    (node as HTMLElement).dispatchEvent(new Event("click"));
    expect(oldClick).not.toHaveBeenCalled();
    expect(newClick).toHaveBeenCalledOnce();
  });

  it("新しい木が undefined ならノードを外す", () => {
    const before = h("div", null, "x");
    const { root, node } = setup(before);
    const patched = diff(before, undefined)(node);
    expect(patched).toBeUndefined();
    expect(root.childNodes).toHaveLength(0);
  });

  it("複数回の patch を連続で当てられる（宣言的更新のループ）", () => {
    let tree = h("div", null, h("span", null, "0"));
    const { node } = setup(tree);
    for (let i = 1; i <= 3; i++) {
      const next = h("div", null, h("span", null, String(i)));
      diff(tree, next)(node);
      tree = next;
    }
    expect(node.textContent).toBe("3");
  });
});
