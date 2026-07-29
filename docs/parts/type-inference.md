<script setup>
import TypeInferDemo from '../components/TypeInferDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# 型推論(Hindley–Milner)

> 実装: [`foundations/typeinfer/`](https://github.com/esh2n/sharin/tree/main/foundations/typeinfer) / 実行: `go test ./foundations/typeinfer/`

<Summary>
型注釈を一切書かなくても、式から型を導ける。let id = λx.x に注釈なしでa→aと分かる。種明かしは3つの道具にある。まず未知の型を型変数で置く。次に関数適用などの制約から「この型とこの型は等しい」を集め、単一化で型変数の中身を解く。let束縛では残った型変数を一般化し、使うたび新しい型変数へ具体化する。Algorithm Wを実装し、注釈なしで最も一般的な型が一意に定まる仕組みを見る。
</Summary>

## この章で作るもの

[小さな言語](/parts/lang)でインタプリタを作ったとき、型は動的だった。値を実行時に見て種類を知る。だが ML・Haskell・Rust のような静的型付き言語は、実行する前に型を確定させる。しかも驚くことに、型注釈をほとんど書かせない。`let id = fun x -> x` と書けば、人が「id は何型か」を指定しなくても、コンパイラが「どんな型 a に対しても a→a の関数だ」と自力で導く。この一見不思議な芸当を実現するのが Hindley–Milner 型推論で、その手続きが Algorithm W だ。

仕掛けは数式を解くのに似ている。まず、まだ分からない型を型変数(t1, t2…)という「穴」で置く。λx.x なら、x の型は分からないので t1 と置く。次に、式の構造から制約を集める。関数適用 `f a` があれば、「f は a を受け取る関数のはずだ」という等式が立つ。こうした「この型とこの型は等しい」という制約を、単一化(unify)という手続きで解いていく。連立方程式を解くように、型変数の中身が次々と埋まる。最後に let 束縛では、解いても残った型変数を「どんな型でもよい」と一般化する(let 多相)。この章では、この一連の手続きを実装する。

<FigureBox caption="型推論の流れ。未知の型を型変数で置き、式の構造から制約を集め、単一化で解く。let では残った変数を一般化して多相にする">

```
λx.x           x の型 = t1(未知)      → 本体は x なので t1 を返す
               結果: (t1 -> t1)

(λx.x) 42      引数 42 は Int という制約  → t1 = Int に解ける
               unify(t1, Int) → 結果: Int

let id = λx.x  id の型 (t1->t1) を一般化 → ∀a. (a -> a)
in id 1, id T  使うたび新しい変数に具体化 → Int にも Bool にも使える
```

</FigureBox>

順に見ていく。

1. **型変数で未知を置く**: 分からない型を穴(t1, t2…)にして、後で解く
2. **単一化で制約を解く**: 「この型とこの型は等しい」を集め、型変数の中身を埋める。合わなければ型エラー
3. **let 多相で一般化する**: let の型の残った変数を「どんな型でもよい」とし、使うたび新しい変数へ具体化する

## ① 型と単一化: 制約を解く

まず型を定義する。型定数(Int, Bool)・型変数・関数型の 3 つだ。そして中心の道具である単一化を作る。単一化は、2 つの型を等しくするような型変数への割り当て(置換)を求める:

<<< ../../foundations/typeinfer/typeinfer.go#unify{go}

`unify(t1, t2)` は、両者を等しくする置換を返す。片方が型変数なら、それを相手に束縛する(t1 = Int なら「t1 は Int」)。両方が関数型なら、引数どうし・結果どうしを再帰的に単一化する。両方が同じ型定数なら、何もしなくてよい。どれにも当てはまらなければ型エラーだ(Int と Bool を等しくはできない)。ここで occurs check が効く。型変数 t を、t 自身を含む型(t→t など)に束縛しようとすると、無限に入れ子の型になってしまう。これを検出して弾く。自己適用 λx.x x が型付けできないのは、まさにこの occurs check のためだ。テストで、Int と Bool の単一化が失敗すること、occurs check が自己適用を弾くことを固定した。

## ② Algorithm W: 式から型を導く

単一化を道具に、式の型を導く。式の各形(リテラル・変数・ラムダ・適用・let・if)ごとに、型変数を作り、制約を単一化で解きながら型を組み立てていく:

<<< ../../foundations/typeinfer/typeinfer.go#infer{go}

要は適用の枝だ。`f a` の型を求めるとき、f の型と a の型をそれぞれ推論し、新しい型変数 tv を用意して、「f の型は (a の型 → tv) と等しい」と単一化する。これで tv に f の返り値の型が定まる。λx.body なら、x に新しい型変数を割り当てて body を推論し、(x の型 → body の型) を返す。if なら、条件を Bool と単一化し、then と else を同じ型に単一化する。テストで、λx.x が (a → a) の多相関数になること、(λx.x) 42 が Int に解けること、if の枝の型が違えば型エラーになることを固定した。注釈を一切書いていないのに、式の構造だけから型が定まる。

## ③ let 多相: 同じ関数を別の型で使う

最後が Hindley–Milner の真骨頂だ。`let id = λx.x in ...` の id を考える。id の型は (t1→t1) だが、これをそのまま使うと問題が起きる。id を最初に `id 1` と使って t1 = Int に確定させると、次に `id true` と使ったとき t1 は Int なので、Bool を渡せずに型エラーになってしまう。だが直感的には、恒等関数はどんな型にも使えるはずだ。

これを解くのが一般化(generalize)と具体化(instantiate)だ。let で値の型を推論したら、そこに残った型変数を「∀a. (a→a)」のように「どんな型 a でもよい」と量化する(一般化)。そして id を使うたびに、その量化された変数を新しい型変数に置き換える(具体化)。`id 1` は (t2→t2) の新鮮なコピーを使って t2 = Int に、`id true` は (t3→t3) の別のコピーを使って t3 = Bool に、それぞれ独立に解ける。互いに干渉しない。テストで、`let id = λx.x in if (id true) then (id 1) else 2` が Int に型付けできることを固定した。単相(一般化なし)ならここで失敗する。この let 多相が、注釈なしで再利用可能な関数を書ける鍵だ。

### 動かす

下のデモは、いくつかの式を選んで、型推論が段階的に型を導く様子を見る。型変数がどこで生まれ、単一化でどう解け、let でどう一般化されるか、そして型エラーがどこで検出されるかを確かめてほしい。

<TypeInferDemo />

## 設計の観点

- **principal type**: Hindley–Milner は、注釈なしでも「最も一般的な型」を一意に導ける。人間が書くより一般的な型を機械が見つけることさえある。この一意性が実用の土台
- **let 多相の位置**: 多相化が起きるのは let 束縛だけで、ラムダの引数は単相のまま。この制限(value restriction を含む)が、推論を決定可能に保つ。全面的な多相は推論不能になる
- **単一化はグラフの問題**: 型変数の解決は、Union-Find で効率化できる。素朴な置換の合成は遅く、実用実装は破壊的な Union-Find を使う
- **型注釈の役割**: 推論できるのに注釈を書くのは、ドキュメントとエラーの局所化のため。注釈があると、型エラーが遠くでなく宣言の近くで出る
- **拡張の難所**: 型クラス(制約付き多相)、部分型、高階多相を足すと、推論は一気に難しくなる。TypeScript の推論が完全でないのはこの複雑さゆえ

## 対照と実例

| 方式 | 注釈 | 例 |
|---|---|---|
| 動的型付け | なし(実行時に判定) | Python, Ruby, [lang の Monkey](/parts/lang) |
| 明示的型付け | 全部書く | Java(旧), C |
| 局所型推論 | 部分的に書く | TypeScript, Go(:=), Rust(関数境界は必須) |
| Hindley–Milner | ほぼ不要 | ML, Haskell, OCaml, Elm |

裏どり:

- **Milner: A Theory of Type Polymorphism (1978)**: Algorithm W と let 多相の原典
- **Damas–Milner**: 型システムの健全性と principal type の存在を示した理論
- **OCaml / Haskell**: Hindley–Milner を土台にした実用言語。型クラスや GADT で拡張
- **Algorithm W vs J**: W は理論的に明快、J(や実装)は Union-Find で効率化した実用版

## 簡略化したこと

- **型は Int/Bool/関数のみ**: リスト・タプル・レコード・ユーザ定義型は扱わない
- **再帰なし**: `let rec`(再帰束縛)や相互再帰は省略
- **型クラスなし**: 制約付き多相(Haskell の型クラス)は範囲外
- **単一化は素朴**: Union-Find でなく置換の合成。小さな式では十分だが実用実装より遅い

## 参考資料

- [Milner: A Theory of Type Polymorphism in Programming (1978)](https://homepages.inf.ed.ac.uk/wadler/papers/papers-we-love/milner-type-polymorphism.pdf) — 原典
- [Write You a Haskell: Type Inference](https://smunix.github.io/dev.stephendiehl.com/fun/006_hindley_milner.html) — Algorithm W の実装解説
- [So You Want to Learn Type Inference](https://okmij.org/ftp/ML/generalization.html) — 一般化と let 多相の要点
- 実装: [foundations/typeinfer](https://github.com/esh2n/sharin/tree/main/foundations/typeinfer)
