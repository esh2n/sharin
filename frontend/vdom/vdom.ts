// 仮想DOM（Virtual DOM）を最小構成でフルスクラッチする。
//
// 肝は3つ:
//   1. 宣言的UI = 「あるべき木」を毎回まるごと作り、前回との差分だけを実DOMに当てる
//   2. diff/patch = 2つの木を再帰比較し、最小の DOM 操作を導く
//   3. diff は「パッチ関数」を返す純粋関数。実DOMを触るのは patch を当てる瞬間だけ

// #region vnode{ts}
export interface VElement {
  kind: "element";
  type: string; // タグ名 "div" など
  props: Props;
  children: VNode[];
}
export interface VText {
  kind: "text";
  text: string;
}
export type VNode = VElement | VText;

export type Props = Record<string, unknown>;

// h() が受け取れる子。false/null/undefined は「描かない」を表す（条件付きレンダリング）
type Child = VNode | string | number | boolean | null | undefined;

// h(type, props, ...children): 仮想DOMノードを組み立てる。
// JSX の <div id="a">hi</div> は h("div", {id:"a"}, "hi") に変換される、その h。
export function h(type: string, props: Props | null, ...children: Array<Child | Child[]>): VElement {
  return {
    kind: "element",
    type,
    props: props ?? {},
    children: normalize(children),
  };
}

// 子を1次元の VNode 配列に均す:
//   - 配列は平坦化（items.map(...) をそのまま渡せる）
//   - false/null/undefined は捨てる（show && h(...) が書ける）
//   - 文字列・数値はテキストノードにする
function normalize(children: Array<Child | Child[]>): VNode[] {
  const out: VNode[] = [];
  for (const c of children.flat()) {
    if (c === null || c === undefined || c === false || c === true) continue;
    out.push(typeof c === "object" ? c : { kind: "text", text: String(c) });
  }
  return out;
}
// #endregion vnode{ts}

// #region mount{ts}
// mount: 仮想DOM木から実DOMノードを作る（初回描画）。
export function mount(vnode: VNode): Node {
  if (vnode.kind === "text") {
    return document.createTextNode(vnode.text);
  }
  const el = document.createElement(vnode.type);
  for (const [key, value] of Object.entries(vnode.props)) {
    setProp(el, key, value);
  }
  for (const child of vnode.children) {
    el.appendChild(mount(child)); // 子を再帰的に mount
  }
  return el;
}

// props を実DOMに反映する。onClick 等は addEventListener、それ以外は属性。
function setProp(el: HTMLElement, key: string, value: unknown): void {
  if (isEventProp(key)) {
    el.addEventListener(eventName(key), value as EventListener);
  } else if (value === false || value === null || value === undefined) {
    el.removeAttribute(key);
  } else {
    el.setAttribute(key, String(value));
  }
}

// on* を判定/変換するヘルパ。次章 mini-next のハイドレーションでも使うので公開する。
export const isEventProp = (key: string): boolean => key.startsWith("on") && key.length > 2;
export const eventName = (key: string): string => key.slice(2).toLowerCase(); // onClick → click
// #endregion mount{ts}

// #region diff{ts}
// patch は「実ノードを受け取り、更新後のノードを返す」関数。
// diff は木を比べてこの patch を組み立てる（この時点では実DOMを触らない）。
export type Patch = (node: Node) => Node | undefined;

const noop: Patch = (node) => node;

export function diff(oldV: VNode, newV: VNode | undefined): Patch {
  // 1. 消えた: ノードを外す
  if (newV === undefined) {
    return (node) => {
      node.parentNode?.removeChild(node);
      return undefined;
    };
  }
  // 2. 種類が違う（テキスト ⇔ 要素）/ タグが違う: 丸ごと差し替え
  if (oldV.kind !== newV.kind || !sameType(oldV, newV)) {
    return (node) => {
      const created = mount(newV);
      node.parentNode?.replaceChild(created, node);
      return created;
    };
  }
  // 3. どちらもテキスト: 内容が違えば nodeValue をその場更新（作り直さない）
  if (oldV.kind === "text" && newV.kind === "text") {
    if (oldV.text === newV.text) return noop;
    return (node) => {
      node.nodeValue = newV.text;
      return node;
    };
  }
  // 4. 同じタグの要素: props と子だけを差分更新する
  //    ここに来る時点で 2・3 により両方 element 確定（型を明示的に絞る）
  if (oldV.kind === "element" && newV.kind === "element") {
    const patchProps = diffProps(oldV.props, newV.props);
    const patchChildren = diffChildren(oldV.children, newV.children);
    return (node) => {
      patchProps(node as HTMLElement);
      patchChildren(node as HTMLElement);
      return node;
    };
  }
  return noop; // 到達しない（網羅性のための保険）
}

// 「同じ枠」として使い回せるか。要素はタグが同じなら使い回す（props/子は後で差分）。
// テキストは常に使い回す（内容が違っても nodeValue 更新で済む）。
function sameType(a: VNode, b: VNode): boolean {
  if (a.kind === "text" && b.kind === "text") return true;
  if (a.kind === "element" && b.kind === "element") return a.type === b.type;
  return false;
}
// #endregion diff{ts}

// #region props{ts}
// props の差分: 消えた属性を外し、増えた/変わった属性を当てる。
function diffProps(oldProps: Props, newProps: Props): (el: HTMLElement) => void {
  const patches: Array<(el: HTMLElement) => void> = [];

  // 消えた or 変わった: 古いのを撤去（イベントは removeEventListener）
  for (const [key, oldValue] of Object.entries(oldProps)) {
    if (!(key in newProps) || newProps[key] !== oldValue) {
      patches.push((el) => unsetProp(el, key, oldValue));
    }
  }
  // 増えた or 変わった: 新しいのを適用
  for (const [key, newValue] of Object.entries(newProps)) {
    if (oldProps[key] !== newValue) {
      patches.push((el) => setProp(el, key, newValue));
    }
  }
  return (el) => patches.forEach((p) => p(el));
}

function unsetProp(el: HTMLElement, key: string, value: unknown): void {
  if (isEventProp(key)) {
    el.removeEventListener(eventName(key), value as EventListener);
  } else {
    el.removeAttribute(key);
  }
}
// #endregion props{ts}

// #region children{ts}
// 子の差分: 同じ位置(index)どうしを付き合わせる素朴な方式。
//   - 共通の範囲は再帰 diff で更新（差し替えても index は動かない）
//   - new の方が長い → 末尾に追加
//   - old の方が長い → 末尾から削除（末尾から消せば index がずれない）
function diffChildren(oldCh: VNode[], newCh: VNode[]): (parent: HTMLElement) => void {
  const common = Math.min(oldCh.length, newCh.length);
  const pairPatches: Patch[] = [];
  for (let i = 0; i < common; i++) {
    // ↑ で common に絞っているので oldCh[i]/newCh[i] は必ず存在する（非null確定）
    pairPatches.push(diff(oldCh[i] as VNode, newCh[i] as VNode));
  }
  const added = newCh.slice(common); // 追加ぶん

  return (parent) => {
    // 1. 共通範囲を更新（childNodes[i] は在る。差し替えは同じ位置に入る）
    pairPatches.forEach((p, i) => {
      const target = parent.childNodes[i];
      if (target) p(target);
    });
    // 2. 余った古い子を末尾から削除
    while (parent.childNodes.length > newCh.length) {
      parent.removeChild(parent.lastChild as Node);
    }
    // 3. 足りない新しい子を末尾に追加
    for (const child of added) {
      parent.appendChild(mount(child));
    }
  };
}
// #endregion children{ts}
