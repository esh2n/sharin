import { describe, it, expect } from "vitest";
import { collectReachable, topoOrder, hasCycle, treeShake, CycleError, type Registry } from "./bundler";

// 典型的な依存関係:
//   entry ─ import {add} ─▶ math (exports add, sub, mul)
//   entry ─ import {log} ─▶ util (exports log)
//   orphan (どこからも import されない)
const registry: Registry = {
  entry: {
    id: "entry",
    imports: [
      { from: "math", names: ["add"] },
      { from: "util", names: ["log"] },
    ],
    exports: [],
  },
  math: { id: "math", imports: [], exports: ["add", "sub", "mul"] },
  util: { id: "util", imports: [{ from: "math", names: ["sub"] }], exports: ["log"] },
  orphan: { id: "orphan", imports: [], exports: ["x"] },
};

describe("collectReachable", () => {
  it("entry から辿れるモジュールだけを集める（孤立モジュールは含まない）", () => {
    const r = collectReachable("entry", registry);
    expect([...r].sort()).toEqual(["entry", "math", "util"]);
    expect(r.has("orphan")).toBe(false); // どこからも import されない
  });

  it("存在しないモジュールはエラー", () => {
    const bad: Registry = { entry: { id: "entry", imports: [{ from: "nope", names: [] }], exports: [] } };
    expect(() => collectReachable("entry", bad)).toThrow(/not found/);
  });
});

describe("topoOrder", () => {
  it("依存を先に、それを使う側を後に並べる", () => {
    const order = topoOrder("entry", registry);
    // math と util は entry より前。math は util より前(util が math に依存)。
    expect(order.indexOf("math")).toBeLessThan(order.indexOf("util"));
    expect(order.indexOf("util")).toBeLessThan(order.indexOf("entry"));
    expect(order.indexOf("math")).toBeLessThan(order.indexOf("entry"));
    // entry は最後(誰の依存でもない)。
    expect(order[order.length - 1]).toBe("entry");
  });
});

describe("循環依存の検出", () => {
  it("import が輪を作ると CycleError で経路を返す", () => {
    const cyclic: Registry = {
      a: { id: "a", imports: [{ from: "b", names: [] }], exports: [] },
      b: { id: "b", imports: [{ from: "c", names: [] }], exports: [] },
      c: { id: "c", imports: [{ from: "a", names: [] }], exports: [] }, // a に戻る
    };
    expect(() => topoOrder("a", cyclic)).toThrow(CycleError);
    const path = hasCycle("a", cyclic);
    expect(path).not.toBeNull();
    // 経路は a -> b -> c -> a のように輪を閉じる。
    expect(path![0]).toBe(path![path!.length - 1]);
    expect(path).toContain("a");
    expect(path).toContain("b");
    expect(path).toContain("c");
  });

  it("循環がなければ null", () => {
    expect(hasCycle("entry", registry)).toBeNull();
  });
});

describe("tree-shaking", () => {
  it("実際に import される export だけ残し、使われないものを落とす", () => {
    const kept = treeShake("entry", registry);
    // math は add(entry から) と sub(util から) が使われ、mul は誰も使わない → 落ちる。
    expect(kept.get("math")!.sort()).toEqual(["add", "sub"]);
    expect(kept.get("math")).not.toContain("mul");
    // util は log が使われる。
    expect(kept.get("util")).toEqual(["log"]);
    // entry は入口なので export をそのまま(ここでは空)。
    expect(kept.get("entry")).toEqual([]);
    // orphan は到達不能なので結果に含まれない。
    expect(kept.has("orphan")).toBe(false);
  });

  it("どの export も使われないモジュールは空になる", () => {
    const reg: Registry = {
      entry: { id: "entry", imports: [{ from: "lib", names: [] }], exports: [] },
      lib: { id: "lib", imports: [], exports: ["a", "b", "c"] }, // 名前を何も import しない
    };
    const kept = treeShake("entry", reg);
    expect(kept.get("lib")).toEqual([]); // 全部落ちる
  });
});
