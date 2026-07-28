# config — 設定を外に出す。ただし更新は勝手には届かない

設定をイメージに焼き込むと環境ごとに別のイメージが要る。外に出せば同じイメージを使い回せる。だが受け取り方によって、更新が届くかどうかが変わる。

## 肝

- **受け取り方が更新の振る舞いを決める**: 環境変数は起動時に写し取るので後から変わらない。ファイルは実体が差し替わるので変わる
- **写し取るという一語がすべて**: 環境変数はプロセスに渡された時点で確定する。届けるには作り直すしかない
- **古いことは検出できる**: 置き場の版と、起動時に見えていた版を比べればよい
- **書き換えは差分**: 触れていない鍵は残る。全部を書き直す必要はない
- **Secret も仕組みは同じ**: 違うのは扱いの慎重さで、仕組みとしての秘匿はほとんど無い
- **決定性**: 鍵も Pod も名前順。乱数も時計も使わない

## 効果の固定(テスト)

- `TestUpdateReachesFilesOnly`: 同じ更新でも、ファイルで受け取る側だけが新しい値を見る
- `TestStaleDetection`: 環境変数で受け取っている側だけが古くなる
- `TestRestartPicksUpNewValue`: 作り直せば写し取り直される
- `TestMixedSourcesInOnePod`: 1つの Pod が両方を混ぜても、それぞれの性質のまま動く
- `TestPutMergesKeys`: 触れていない鍵は残り、版だけ上がる
- `TestSecretBehavesLikeConfigMap`: Secret も同じように読める

## 使い方

```go
c := config.New()
c.Store.Put("app-config", config.ConfigMap, map[string]string{"log_level": "info"})

env := c.Start("env-pod", config.Ref{Entry: "app-config", Source: config.EnvVar})
file := c.Start("file-pod", config.Ref{Entry: "app-config", Source: config.FileMount})

c.Store.Put("app-config", config.ConfigMap, map[string]string{"log_level": "debug"})

file.Read("app-config", "log_level") // "debug"(実体を見に行く)
env.Read("app-config", "log_level")  // "info"(起動時に写し取った値)
env.Stale()                          // true

c.Restart("env-pod") // 作り直せば新しい値になる
```

## 簡略化したこと

- **反映の遅れなし**: 実物のファイル更新にも伝播の遅れがある。ここでは即座に見える
- **Secret の保存形式なし**: 実物は base64 で持つが、暗号ではない点は同じ
- **subPath なし**: 実物で subPath を使うと、ファイルでも更新が届かなくなる
- **参照の追跡なし**: 実物は設定の変更で Pod を作り直す仕組みを別途組む必要がある
- **鍵の削除なし**: 書き換えは足し算のみ

## 章

教科書: [ConfigMapとSecret](https://sharin-2a1.pages.dev/parts/config)

実行: `go test ./orchestration/config/`
