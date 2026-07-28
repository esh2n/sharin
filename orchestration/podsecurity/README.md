# podsecurity — どこまでできるかを、外から一律に決める

[RBAC](../rbac/) は「誰が操作してよいか」を決め、[admission](../admission/) は
「どんな内容なら受け入れるか」を決めた。残っているのが「受け入れた Pod がノードの上で
どこまでできるか」になる。

コンテナは[隔離](../../foundations/container/)されていると言うが、その隔離はいくらでも緩められる。
ホストのネットワークをそのまま使う、ホストのディレクトリを覗く、特権つきで動かす。
どれも設定ひとつで外せる。外せることには理由があって、監視や収集のように本当にホストを
見なければならないものがあるからだ。

問題は既定にある。`SecurityContext` は書けば効くが、書かなければ何も制限しない。
[ResourceQuota](../quota/) の章で見た「既定値が無いと書き忘れが 0 として通る」のと同じ形で、
書き忘れた Pod がいちばん緩い設定で動く。だから外側から一律に決める層が要る。

## 3つの段階

| Level | 何を見るか | 素朴に書いた Pod は |
|---|---|---|
| `Privileged` | 何も見ない(既定) | 通る |
| `Baseline` | 隔離を外していないか | 通る |
| `Restricted` | 安全側を明示したか | **落ちる** |

`Baseline` は「危ないことをしていないか」、`Restricted` は「安全を書いたか」を見る。
向きが違うので、何も書いていない Pod は `Baseline` を通って `Restricted` で落ちる。

`Restricted` は `Baseline` を含む。上の段階だけを別に書くと、下の規則が抜けても気づけない。

## 判定は1つ、扱いが3つ

```go
pol := podsecurity.Policy{
    Enforce: podsecurity.Baseline,   // これに反したら拒む
    Audit:   podsecurity.Restricted, // これに反したら記録する(通す)
    Warn:    podsecurity.Restricted, // これに反したら警告する(通す)
}
d := pol.Admit(pod)
d.Admitted // true
d.Warned   // restricted の違反(何を直せばよいか)
```

検査そのものは `Check(pod, level)` を3回呼ぶだけで、段階だけが違う。
拒否しか無ければ、締めるという操作が常に危険になる。今動いているものが落ちるかどうかを、
落とさずに知る手段が要る。

```go
ok, breaks := podsecurity.Tighten(runningPods, podsecurity.Restricted)
// ok=false, breaks=["agent", "web"] ← 上げると落ちるもの
```

## 「書かなかった」と「false と書いた」は別

```go
type SecurityContext struct {
    AllowPrivilegeEscalation *bool // nil = 書かなかった
    RunAsNonRoot             *bool
    RunAsUser                *int64
    ...
}
```

`Restricted` では書かなかったことが違反になるので、ポインタで区別する。
区別できないと検査そのものが成り立たない。

## API

| 関数・メソッド | 役割 |
|---|---|
| `Check(pod, level) []Violation` | 段階に照らして違反を返す。コンテナ名・規則名の順 |
| `(Policy) Admit(pod) Decision` | 拒否・記録・警告の3つに分けて判定 |
| `Tighten(pods, to) (bool, []string)` | 段階を上げたときに落ちるものを、落とさずに調べる |
| `Bool(b)` / `Int64(i)` | 「書いた」ことを表す補助 |

## 決定性

違反はコンテナ名、規則名の順に並べる。Pod 全体の設定はコンテナ名が空なので先頭に来る。
実時間も乱数も使わない。

## テスト

```
go test -race -cover ./orchestration/podsecurity/
```

カバレッジ 100%。
