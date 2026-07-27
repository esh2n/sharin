# typeinfer — Hindley–Milner 型推論(Algorithm W)

型注釈を一切書かなくても、式から型を導く。未知の型を型変数で置き、制約を単一化(unify)で解き、let 束縛で一般化する(let 多相)。ML / Haskell / Rust / TypeScript の型推論の源流。

## 肝

- **型変数**: まだ決まっていない型を穴(t1, t2…)として置く
- **単一化(unify)**: 「この型とこの型は等しい」制約を解き、型変数の中身を埋める。合わなければ型エラー
- **occurs check**: 型変数がそれ自身を含む無限型(t = t→t)を弾く。自己適用 λx.x x で起きる
- **let 多相**: let で束縛した値の型を「どんな型でもよい」と一般化し、使うたびに新しい型変数へ具体化する。だから id を Int にも Bool にも使える
- **注釈ゼロで健全**: 人が型を書かなくても、最も一般的な型(principal type)が一意に定まる

## 効果の固定(テスト)

- `TestIdentityIsPolymorphic`: 注釈なしの λx.x が (a -> a) と推論される
- `TestLetPolymorphism`: let id = λx.x を Int と Bool の両方に使える(単相なら型エラー)
- `TestOccursCheck`: 自己適用 λx.x x を occurs check で弾く
- 型エラー検出: if の枝の型不一致、条件が非 Bool、非関数の適用

## 使い方

```go
inf := typeinfer.New()
// λx.x
id := &typeinfer.ELam{Param: "x", Body: &typeinfer.EVar{Name: "x"}}
ty, err := inf.Infer(typeinfer.Env{}, id) // (t1 -> t1)
```

## 簡略化したこと

- **型は Int/Bool/関数のみ**: リスト・タプル・レコード・ユーザ定義型は扱わない
- **再帰なし**: `let rec`(再帰束縛)は省略。相互再帰も扱わない
- **型クラスなし**: Haskell の型クラス(制約付き多相)は範囲外
- **エラーは最初の 1 つ**: 型エラーの位置や複数報告はしない

## 章

教科書: [型推論(Hindley–Milner)](https://sharin-2a1.pages.dev/parts/type-inference)

実行: `go test ./foundations/typeinfer/`
