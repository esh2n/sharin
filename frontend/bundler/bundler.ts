// モジュールバンドラの中核（依存解決）を最小構成でフルスクラッチする。
//
// ブラウザは何百ものファイルを個別に読むのが苦手だ。バンドラは、入口(entry)の
// モジュールから import を辿って依存グラフを作り、依存が先に来るように並べ替えて
// 1 つにまとめる。ついでに、どこからも使われない export を落として無駄を削る
// (tree-shaking)。import が輪を作っていれば(循環依存)、それも検出する。
//
// 肝は3つ:
//   1. 依存グラフ: entry から import を辿り、到達できるモジュールだけを集める
//   2. トポロジカル順序: 依存を先に、それを使う側を後に並べる（循環なら検出）
//   3. tree-shaking: 実際に import される export だけ残し、使われない export を落とす

// #region graph{ts}

// Import は「from から names を取り込む」を表す（import { names } from "from"）。
export interface Import {
  from: string;
  names: string[];
}

// Module は 1 つのモジュール。何を import し、何を export するか。
export interface Module {
  id: string;
  imports: Import[];
  exports: string[];
}

// Registry は id からモジュールを引く表（ファイル解決の結果に相当）。
export type Registry = Record<string, Module>;

// collectReachable は entry から import を辿って到達できるモジュール ID を集める。
// どこからも import されないモジュールは、ここに入らない＝バンドルに含まれない。
export function collectReachable(entry: string, registry: Registry): Set<string> {
  const reachable = new Set<string>();
  const stack = [entry];
  while (stack.length > 0) {
    const id = stack.pop() as string;
    if (reachable.has(id)) continue;
    reachable.add(id);
    const mod = registry[id];
    if (!mod) throw new Error(`bundler: module not found: ${id}`);
    for (const imp of mod.imports) stack.push(imp.from);
  }
  return reachable;
}

// #endregion graph{ts}

// #region topo{ts}

// CycleError は import が輪を作っているとき。path に循環の経路を持つ。
export class CycleError extends Error {
  constructor(public path: string[]) {
    super(`bundler: circular dependency: ${path.join(" -> ")}`);
    this.name = "CycleError";
  }
}

// topoOrder は依存を先に、それを使う側を後に並べた順序を返す。
// DFS の帰りがけ順が「依存が先」になる。訪問中(gray)のノードに戻れば循環。
export function topoOrder(entry: string, registry: Registry): string[] {
  const order: string[] = [];
  const state = new Map<string, "gray" | "black">(); // gray=訪問中, black=完了
  const path: string[] = [];

  function visit(id: string): void {
    const s = state.get(id);
    if (s === "black") return;
    if (s === "gray") {
      // 訪問中に戻った＝循環。今の経路から循環部分を切り出す。
      const start = path.indexOf(id);
      throw new CycleError([...path.slice(start), id]);
    }
    state.set(id, "gray");
    path.push(id);
    const mod = registry[id];
    if (!mod) throw new Error(`bundler: module not found: ${id}`);
    for (const imp of mod.imports) visit(imp.from); // 依存を先に訪ねる
    path.pop();
    state.set(id, "black");
    order.push(id); // 依存を訪ね終えてから自分を積む＝依存が先に並ぶ
  }

  visit(entry);
  return order;
}

// hasCycle は循環の有無だけを返す（あれば経路、なければ null）。
export function hasCycle(entry: string, registry: Registry): string[] | null {
  try {
    topoOrder(entry, registry);
    return null;
  } catch (e) {
    if (e instanceof CycleError) return e.path;
    throw e;
  }
}

// #endregion topo{ts}

// #region treeshake{ts}

// treeShake は各モジュールについて「実際に使われる export だけ」を返す。
// 到達可能なモジュールが import する名前だけを used とし、他は落とす。
// entry 自身はアプリの入口なので、その export はすべて残す。
export function treeShake(entry: string, registry: Registry): Map<string, string[]> {
  const reachable = collectReachable(entry, registry);

  // 誰かに import される名前を集める（モジュールID → 使われる export の集合）。
  const used = new Map<string, Set<string>>();
  for (const id of reachable) {
    const src = registry[id];
    if (!src) continue; // reachable なので実際には存在する
    for (const imp of src.imports) {
      if (!reachable.has(imp.from)) continue;
      const set = used.get(imp.from) ?? new Set<string>();
      for (const n of imp.names) set.add(n);
      used.set(imp.from, set);
    }
  }

  // 各モジュールの export を、used に含まれるものだけに絞る（entry は全部残す）。
  const kept = new Map<string, string[]>();
  for (const id of reachable) {
    const mod = registry[id];
    if (!mod) continue;
    if (id === entry) {
      kept.set(id, [...mod.exports]);
      continue;
    }
    const u = used.get(id) ?? new Set<string>();
    kept.set(id, mod.exports.filter((e) => u.has(e)));
  }
  return kept;
}

// #endregion treeshake{ts}
