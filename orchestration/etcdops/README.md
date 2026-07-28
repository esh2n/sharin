# etcdops — 履歴と容量の綱渡り

[apiserver](../apiserver/) の章で、写しが古すぎると差分で追いつけず全件を取り直すことになる、と書いた。
その「古すぎる」を作っているのが、この層で定期的に走っている圧縮になる。

## 履歴があることの表と裏

書くたびに版が1つ増え、古い版もしばらく残る。残っているから、写しは
「版5まで見た。それ以降をくれ」と言える。**履歴があることが watch の成立条件そのもの**になっている。

だが履歴は増え続ける。上書きも削除も、履歴としては書き込みなので量が増える。

```go
s := etcdops.New(0)
s.Put("a", "1")
s.Put("a", "2")
s.Logical()  // 1 ← キーは1つ
s.Physical() // 2 ← 書いた回数ぶん
```

## 捨てる操作は2段ある

| 操作 | 何をするか | ファイルの大きさ |
|---|---|---|
| `Compact(rev)` | rev より古い履歴を捨てる | **変わらない** |
| `Defrag()` | 余っている場所を実際に返す | 小さくなる |

「圧縮したのに容量が減らない」で止まるのは、この二段構えを知らないからになる。
実物では `Defrag` の間そのメンバーが応答しないので、1台ずつ順に行う。

## 使い切ると自分では抜けられない

```go
s := etcdops.New(5)
// ... 上限を超えるまで書く
s.Alarm()               // true
s.Put("b", "x")         // ErrNoSpace
s.Get("a")              // 読める

s.Compact(s.Rev())
s.Disarm()              // false ← 捨てただけでは戻らない
s.Defrag()
s.Disarm()              // true  ← 捨てる、返す、解除する の3手
```

書き込みが止まるので、設定を変えることもできない。決まった順番の3手でしか抜けられない。

## 写しは今の値だけを運ぶ

```go
snap := s.Take()
r := etcdops.Restore(snap, 0)
r.Since(1) // ErrCompacted ← 復元先に過去は無い
```

復元した直後、写しを持っていた informer は全件を取り直すことになる。

## API

| メソッド | 役割 |
|---|---|
| `New(quota) *Store` | 上限を決めて作る(0 なら上限なし) |
| `Put` / `Delete` | 書く。止まっていれば `ErrNoSpace` |
| `Get` / `GetAt(key, rev)` | 今の値・過去の版。捨てた版は `ErrCompacted` |
| `Since(from) ([]Event, error)` | 差分。追えなければ `ErrCompacted` |
| `Compact(rev)` / `Defrag()` / `Disarm()` | 捨てる・返す・解除する |
| `Take()` / `Restore(snap, quota)` | 写しを取る・復元する |

## 決定性

実時間も乱数も使わない。版は1から単調に増え、差分は版の順で返る。
同じ手順なら必ず同じ結果になる。

## テスト

```
go test -race -cover ./orchestration/etcdops/
```

カバレッジ 100%。
