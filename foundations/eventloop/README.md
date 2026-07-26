# event-loop — epoll 風の I/O 多重化

「1 本のスレッドで、何千もの接続を同時にさばく」——Node.js・nginx・Redis が採る
仕組みを Go でモデル化する。ランタイム内部編の2つ目のパーツ。実ソケットも
実スレッドも syscall も使わず、**ノンブロッキング I/O + readiness 多重化 +
イベントループ**の骨格を純粋なデータ構造で決定的に再現する。

教科書の章: [event-loop](https://sharin-2a1.pages.dev/parts/event-loop)

## これは何か

接続を 1 本さばくのは簡単だ——`read` し、返事を `write` する。難しいのは
**1 万本を同時に**さばくとき。素朴な方法は「1 接続 = 1 スレッド」。だが read が
データを待ってスレッドを止める(ブロックする)ので、接続の数だけスレッドが要り、
文脈切り替えとメモリで破綻する(C10K 問題)。

イベントループはこれをひっくり返す:

1. **FD を全部ノンブロッキングにする**: `read`/`write` は決して待たない。今すぐ
   進められなければ即 `EAGAIN`(このパッケージの `ErrWouldBlock`)を返す
2. **「どれか準備できたら教えて」と一括で尋ねる**: 監視したい FD を epoll に登録し、
   `epoll_wait` 1 回で「今 ready な FD 全部」を受け取る
3. **1 本のスレッドで ready な FD だけを順に処理する**: これを回し続けるのがループ

止まるのは「全接続まとめて待つ `epoll_wait` の中」だけで、どれか 1 本の `read` の
中では決して止まらない。だから 1 スレッドで多重化できる。

```
        ┌──────────────── Event Loop(1 スレッド)────────────────┐
        │                                                        │
        │   registered FDs(epoll に登録)                        │
        │   ┌ c1: r ┐ ┌ c2: r ┐ ┌ c3: r ┐                        │
        │   └───────┘ └───────┘ └───────┘                        │
        │        │        │         │                            │
        │        ▼        ▼         ▼                            │
        │   ┌──────────── epoll_wait ─────────────┐              │
        │   │ ready {c1:r, c3:r}  ← 準備できたものだけ返る       │
        │   └──────────────────────────────────────┘              │
        │        │                                                │
        │        ▼  dispatch: ready な FD のハンドラを順に呼ぶ    │
        │   c1.handler(read→echo) → c3.handler(read→echo)        │
        │        │                                                │
        │        └──▶ また epoll_wait へ(ready 無しなら眠る)    │
        └──────────────────────────────────────────────────────────┘
```

os 編の協調スケジューラと同じ発想がここにもある——**どれかで立ち止まらず、
準備できたものへ制御を回し続ける**。違いは、切り替えの合図が `yield` ではなく
「FD の readiness」であることだ。

## 肝は3つ

1. **ノンブロッキング I/O = 待たない約束**: `read`/`write` はスレッドを止めず、
   進められなければ `ErrWouldBlock` を即返す。ブロックした瞬間、他の全接続が
   巻き添えで止まるので、ハンドラは決してブロックしてはならない
2. **readiness 多重化 = 一括問い合わせ**: 多数の FD を 1 回の `Wait`(epoll_wait)で
   走査し、準備できたものだけ返す。接続が増えても「準備できた分」しか仕事しない。
   関心(Interest)でマスクするので、待っていないイベントは報告されない
3. **level-triggered とバックプレッシャ**: 未読が残る限り Readable は報告され続ける
   (level-triggered)。逆に、送りたいデータが残ったら Writable を登録し、送信バッファが
   空いたら書き足す——これが送信側の詰まり(バックプレッシャ)の扱い。Writable を
   張りっぱなしにするとループが空回りするので、掃けたら外す

## ファイル

- `fd.go` — ノンブロッキング FD。`Interest`(Readable/Writable)、`Read`/`Write`
  (即返る。空/満杯なら `ErrWouldBlock`)、readiness の導出
- `poller.go` — `Poller`(epoll のモデル)。`Add`/`Modify`/`Remove` で関心を登録し、
  `Wait` で今 ready な FD をまとめて返す
- `loop.go` — `Loop`(リアクタ)。`Open`/`Register` で接続を登録し、`Tick`/`Run` で
  poll → dispatch を回す。外界の到着は tick で予定して決定化する

## 設計メモ

- **決定的**: 乱数もタイマも実ソケットも使わない。「ネットワーク到着」「送信バッファ
  解放」を tick で予定し、同時刻は登録順で適用する。同じ入力なら常に同じ順で dispatch
  されるので、テストとデモが安定する
- **readiness は状態から導出**: 受信バッファに未読があれば Readable、送信バッファに
  空きがあれば Writable。`Wait` はこれと登録した関心の AND を取る
- **idle = epoll_wait のブロック**: ready が 1 つも無ければ、次の到着時刻まで論理時計を
  飛ばして「眠る」。os 編の idle 空転(HLT)と同じ骨格
- **エコーサーバを題材に**: ハンドラは「読んで書き返す」。バックプレッシャ・EOF での
  クローズ・level-triggered の複数回読み、といった実務の勘所をテストで押さえる

## 簡略化したこと

- **実ソケット・syscall なし**: FD はバイト列を持つ構造体。`epoll_create`/`epoll_ctl`/
  `epoll_wait` の意味論だけを取り出す
- **level-triggered のみ**: edge-triggered(状態が変化した瞬間だけ通知)は扱わない
  (違いは章で説明する)。実機の epoll は両対応
- **単一スレッド固定**: マルチスレッド epoll・SO_REUSEPORT による負荷分散・
  ワーカプールは省略
- **到着は予定制**: 実ネットワークの非同期到着を、決定的な tick スケジュールに置き換える
- **タイマ/シグナルは無し**: `epoll` に混ぜる `timerfd`/`signalfd`/`eventfd` は扱わない。
  監視対象は FD の read/write readiness のみ

## 動かす

```bash
go test ./foundations/eventloop/ -race -cover
go vet ./foundations/eventloop/
```

## 参考

- Dan Kegel, "The C10K problem" — 1 万接続をどうさばくか、という問題設定の原典
- `man epoll` / `man epoll_ctl` / `man epoll_wait` — Linux の readiness 通知 API
- libuv(Node.js の裏)・nginx の event module・Redis の `ae` イベントループ — 実装の実例
- Remzi & Andrea Arpaci-Dusseau, *Operating Systems: Three Easy Pieces* — I/O とイベント駆動
