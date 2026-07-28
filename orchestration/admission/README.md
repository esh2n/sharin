# admission — 作られる前に、書き換えるか拒否する

RBAC は「誰が」を見た。こちらは「何を」を見る。保存される直前のオブジェクトを受け取り、書き換えてから検証する。

## 肝

- **書き換えが先、検証が後**: この順序でなければならない。書き換えた結果も検証を通る
- **元は変えない**: 関門を通るのは複製で、通った結果が返る
- **応答が無いときを決めておく**: `Fail` は拒否、`Ignore` は素通し。どちらも危険で、失うものが違う
- **自分自身を止められる**: webhook もクラスタの中の Pod。対象に自分を含めたまま `Fail` にすると、直すための Pod すら作れなくなる
- **書き換えは積み上がる**: 後の関門は前の書き換えの結果を見る
- **決定性**: 関門は段の順、鍵は名前順

## 効果の固定(テスト)

- `TestMutateRunsBeforeValidate`: 検証を先に足しても、段の順序で書き換えが先に走る
- `TestOriginalIsNotMutated`: 元のオブジェクトは変わらない
- `TestFailurePolicySplitsTheRisk`: 応答が無いとき、`Fail` は合格するものまで拒否し、`Ignore` は検証を素通しする
- `TestIgnoreLetsBadObjectsThrough`: `Ignore` は本来なら拒否されるものを通す
- `TestFailPolicyCanLockOutRecovery`: 対象に自分を含めたまま `Fail` にすると、復旧できなくなる
- `TestMutationsCompose`: 後の書き換えが前の結果を見られる

## 使い方

```go
c := admission.New()
c.Add(&admission.Webhook{
    Name: "add-team-label", Stage: admission.Mutating, Kinds: []string{"Pod"},
    Available: true, Failure: admission.Fail,
    Mutate: func(o *admission.Object) string {
        if o.Labels["team"] != "" { return "" }
        o.Labels["team"] = "unknown"
        return "team=unknown を付けた"
    },
})
c.Add(&admission.Webhook{
    Name: "require-team-label", Stage: admission.Validating, Kinds: []string{"Pod"},
    Available: true, Failure: admission.Fail,
    Check: func(o *admission.Object) string {
        if o.Labels["team"] == "" { return "team ラベルが無い" }
        return ""
    },
})

r := c.Admit(admission.NewObject("Pod", "web-1"))
r.Allowed  // true(書き換えで足したラベルが検証を通る)
r.Applied  // ["add-team-label: team=unknown を付けた"]
```

## 簡略化したこと

- **通信なし**: 実物は HTTP で外部のサーバに問い合わせる。ここでは関数を呼ぶだけ
- **タイムアウトなし**: 実物は応答待ちに上限があり、超えると失敗として扱う
- **JSON Patch なし**: 実物の書き換えはパッチとして返される
- **順序の指定なし**: 実物も mutating どうしの順序は保証されない
- **組み込みの関門なし**: 実物には webhook より前に多数の内蔵 admission controller がある

## 章

教科書: [admission webhook](https://sharin-2a1.pages.dev/parts/admission)

実行: `go test ./orchestration/admission/`
