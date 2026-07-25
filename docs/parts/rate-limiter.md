# Rate Limiter

> 実装: [`rate-limiter/`](https://github.com/esh2n/sharin/tree/main/rate-limiter) / 実行: `go test ./rate-limiter/`

## この章で作るもの

代表的なレートリミットアルゴリズム4方式を、同じインターフェースで実装して比較する。

1. **Token Bucket** — バーストを許しつつ平均レートを守る
2. **Leaky Bucket** — 流出速度を一定に保つ
3. **Fixed Window** — 実装は最も単純、ただし境界に弱点がある
4. **Sliding Window Log** — 正確、ただしメモリを食う

この章の肝は3つ。

- レートリミットとは「**バーストをどこまで許すか**」と「**平滑化**」のトレードオフの選択である
- タイマーは要らない。**呼ばれた瞬間に経過時間から計算すればいい**(lazy refill)
- fixed window は境界をまたぐと limit の**2倍**が通る。この弱点はテストで固定してある

## なぜレートリミットが必要か

サーバーの処理能力は有限で、クライアントの要求速度は制御できない。この非対称を放置すると、
悪意の有無に関係なく(バグったリトライループ1つで)サービス全体が落ちる。

レートリミッターは両者の間に立って「単位時間あたり何件まで通すか」を強制する部品で、
API ゲートウェイ、ログインの試行制限、スクレイピング対策、LLM API のトークン制限など、
外に開いた口のほぼすべてに入っている。

設計上の本質的な問いは1つだけ:

> 「平均 10 req/s まで」と言ったとき、**一瞬に10発**は許すのか、**100msに1発ずつ**しか許さないのか。

前者に寄せたのが token bucket、後者に寄せたのが leaky bucket、
「そもそも平均をどう測るか」を単純化したのが window 系、と整理できる。

## 共通インターフェース

4方式とも「今この1リクエストを通すか」だけを答える。

<<< ../../rate-limiter/limiter.go#limiter{go}

テストでは時計を注入して(`now func() time.Time`)、時間を自由に進めながら挙動を検証する。
時刻を外から与えられるようにするのは、時間が絡むコードをテスト可能にする定石。

## 1. Token Bucket

バケツにトークンが一定速度で補充され、リクエストは1トークン消費できたときだけ通る。
バケツの容量ぶんだけ「溜め」ができるので、**しばらく静かだったクライアントの瞬間的なバーストを許せる**。
これが token bucket が実務で最もよく使われる理由(GitHub API も AWS もこの系統)。

<<< ../../rate-limiter/token_bucket.go#tokenbucket{go}

実装の見どころは `Allow` の冒頭。トークンをタイマーで定期的に足すのではなく、
**前回呼ばれてからの経過時間 × 補充速度**をその場で計算して足している(lazy refill)。
これで goroutine もタイマーも不要になり、状態は「残量」と「最終計算時刻」の2つで済む。

```
時刻:     0s          1s          2s
補充:     満タン(3)    +1/s で回復
リクエスト: ●●●○        ●○
           ↑3発は通る   ↑1秒で1トークン回復したので1発だけ通る
           ↑4発目は空
```

## 2. Leaky Bucket

こちらは「水の入ったバケツの底に穴が空いている」モデル。リクエストは水1杯ぶんで、
溢れなければ受け付ける。水は一定速度で漏れていく。

<<< ../../rate-limiter/leaky_bucket.go#leakybucket{go}

コードを見比べると token bucket とほぼ対称なことが分かる。
`tokens`(残り許可)を減らしていくか、`water`(使用量)を増やしていくかの違いで、
数学的には同じ制約を課している。区別する意味があるのは、leaky bucket を
**キュー**として実装した場合(溢れたら捨てるのではなく並ばせ、一定速度で処理する)。
その形は流量が完全に平滑化される代わりに遅延が生まれる。トラフィックシェーピングの文脈で
leaky bucket と呼ばれるのは主にこちら。

## 3. Fixed Window

時間軸を1秒なら1秒で固定に区切り、各区間のカウントが limit を超えたら拒否する。
カウンタ1個で済むので実装も分散化(Redis の `INCR` + `EXPIRE`)も一番簡単。

<<< ../../rate-limiter/fixed_window.go#fixedwindow{go}

### 境界バースト問題

fixed window には有名な弱点がある。**区間の境界をまたぐと、窓幅より短い時間に limit の2倍が通る**。

```
limit = 3/秒 のとき:

窓A [0.0s ────────── 1.0s) 窓B [1.0s ────────── 2.0s)
              ●●●          ●●●
             t=0.70       t=1.05

→ 0.35秒間に6リクエスト = 実質 17 req/s が通過
```

窓Aの終わり際に3発、窓Bが始まった直後に3発。どちらの窓も制限は守っているのに、
「直近1秒」で見ると2倍通っている。この挙動は
[`fixed_window_test.go`](https://github.com/esh2n/sharin/blob/main/rate-limiter/fixed_window_test.go)
に「仕様上の弱点」としてテストで固定してある。弱点を文章でなくテストで残しておくと、
後から読んだときに挙動として再現・確認できる。

## 4. Sliding Window Log

境界バーストの根本原因は「窓が固定されていて、判定時刻を中心に見ていない」こと。
なら通したリクエストの時刻を全部記録して、毎回「**直近 window の間に何件通したか**」を
正確に数えればいい。それが sliding window log。

<<< ../../rate-limiter/sliding_window_log.go#slidingwindowlog{go}

fixed window で通ってしまった境界バーストのシナリオが、こちらでは正しく拒否される
(テストで対比している)。代償はメモリで、キーごとに最大 limit 件の時刻を保持する。
limit が大きい・キーが多い環境では効いてくる。

なお中間案として、前の窓のカウントを経過割合で按分して足す **sliding window counter**
(Cloudflare 方式)があり、メモリはカウンタ2個のまま境界バーストをほぼ抑えられる。
近似で十分なら実務ではこれが有力。

## 4方式の比較

| 方式 | 状態 | バースト | 特徴 |
|---|---|---|---|
| Token Bucket | 残量 + 時刻 | 容量まで許す | 実務の既定。バースト許容量を明示的に設計できる |
| Leaky Bucket | 水位 + 時刻 | 容量まで許す | token bucket と双対。キュー形にすると完全平滑化 |
| Fixed Window | カウンタ + 窓開始 | 境界で limit の2倍 | 最も単純。分散化しやすい。弱点を許容できるなら十分 |
| Sliding Window Log | 時刻ログ | 正確に limit 以内 | 正確さ最優先。メモリ O(limit) |

## 発展: 分散環境では

この章の実装は単一プロセスの mutex で守っているが、サーバーが複数台になると
「残量」をどこに置くかという問題になる。定石は Redis に状態を置き、
**読み取り、計算、書き込みを Lua スクリプトで原子的に実行する**こと。
アルゴリズム自体はこの章と同じで、変わるのは原子性の担保方法だけ。
これは db 編・proxy 編をやった後に戻ってくると解像度が上がるテーマ。

## 簡略化したこと

- **単一プロセスのみ**: 分散時の原子性(Redis + Lua)は扱っていない
- **単一キーのみ**: 実務ではユーザーごと・IPごとに Limiter を持つ(map + 掃除が必要)
- **二値判定のみ**: `golang.org/x/time/rate` にある `Wait`(空きが出るまで待つ)や `ReserveN` は未実装
- **sliding window log のメモリ管理は素朴**: スライスの詰め直しで済ませている

## 参考資料

- [golang.org/x/time/rate](https://pkg.go.dev/golang.org/x/time/rate) — Go 公式の token bucket 実装。`Wait` / `Reserve` まで含む設計が学びになる
- [Cloudflare: How we built rate limiting capable of scaling to millions of domains](https://blog.cloudflare.com/counting-things-a-lot-of-different-things/) — sliding window counter 方式の出典
- [Stripe: Scaling your API with rate limiters](https://stripe.com/blog/rate-limiters) — 実務での使い分け(rate limiter と load shedder)
