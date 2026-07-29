<script setup>
import HttpDemo from '../components/HttpDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
import FlowRow from '../components/figures/FlowRow.vue'

const stack = [
  { label: 'アプリ', note: 'HTTP(この章)' },
  { label: 'TCP', note: '信頼できるバイト列', state: 'hot' },
  { label: 'IP', note: 'パケットを届ける' },
  { label: 'イーサネット等', note: '物理' },
]
</script>

# HTTPサーバ

> 実装: [`network/httpserver/`](https://github.com/esh2n/sharin/tree/main/network/httpserver) / 実行: `go test ./network/httpserver/`

<Summary>
Express も Rails も Go の net/http も、下でやっていることは同じで、届いた「ただのテキスト」を読んで、それに応じたテキストを返しているだけになる。テキストを欠けず順番どおり運ぶ仕事は、下の層(TCP)が請け負ってくれる。この章では生のバイト列を受け取り、HTTP リクエストを手でパースして、レスポンスを手で組む。フレームワークが隠しているものの正体が、200行ほどで丸見えになる。
</Summary>

## この章で作るもの

net.Listen("tcp") で待ち受けて、ブラウザや curl から来た HTTP を自分でパースし、
ルーティングして、レスポンスを返す。Go の `net/http` に頼らず、その中身を作る。

押さえることは3つある。

- HTTP は **TCP の上を流れる人間可読なテキスト**。特別なバイナリではない
- リクエストの形は「**リクエストライン → ヘッダ群 → 空行 → ボディ**」の4部構成
- サーバの本体は **Accept ループ**。接続を得て、1接続ごとに goroutine で捌く

## 前提: HTTP は何の上に乗っているか

ネットワークは層(レイヤ)でできている。下の層が上の層に「サービス」を提供する。

<FigureBox caption="ネットワークの層。この章が作るのは一番上の HTTP。その下の TCP が「順序通り・欠落なくバイト列を届ける」を保証してくれるので、HTTP はテキストの読み書きに集中できる">
  <FlowRow :steps="stack" />
</FigureBox>

**TCP** が「信頼できるバイトストリーム」を用意してくれる。相手と接続を確立し
(3-way handshake)、送ったバイトが順序通り・欠落なく届くことを保証する。
その上に乗る HTTP は、だから「届いたテキストを読んで、テキストを返す」だけでいい。
この章は TCP そのものは Go の `net` に任せ(中身は [TCP](./tcp) の章で作った)、HTTP の層を作る。

## リクエストはただのテキスト

curl が `GET /hello` するとき、TCP の上を流れているのはこういうテキスト:

```
GET /hello?name=world HTTP/1.1\r\n   ← リクエストライン(メソッド パス バージョン)
Host: example.com\r\n                ← ヘッダ
User-Agent: curl/8.0\r\n             ← ヘッダ
\r\n                                 ← 空行(ここでヘッダ終わり)
（ボディ。GET なら無い）
```

`\r\n`(改行)で区切られた、人間が読めるテキスト。これを手でパースする:

<<< ../../network/httpserver/request.go#request{go}

### コードの読みどころ: ボディの切れ目は Content-Length で知る

TCP はバイトストリームなので、「どこまでが1つのリクエストか」は HTTP 自身が
決めないといけない。ヘッダの後の空行まででヘッダは終わり、その先のボディは
**Content-Length ヘッダのバイト数**だけ読む。これがないと、受け手はいつ読み終えたか
分からない(TCP はただのバイトの流れで、切れ目を教えてくれない)。

## レスポンスもただのテキスト

返す側も同じ形式。ステータス行 → ヘッダ → 空行 → ボディ。

<<< ../../network/httpserver/request.go#response{go}

Content-Length を自分で計算して付けているのがポイント。受け手(ブラウザ)が
「本文をあと何バイト読めばいいか」を知るために要る。

## サーバの本体: Accept ループ

<<< ../../network/httpserver/server.go#server{go}

`Serve` の中の `for { ln.Accept() }` がサーバの心臓。
**Accept は「クライアントとの TCP 接続が1つできるまで待つ」**関数で、接続ができたら
その conn を goroutine に渡して次の接続を待つ。だから複数のクライアントを同時に捌ける。

1接続の処理はこれだけだ。生のバイト列からパースし、ルートを引いて、書き戻す:

<<< ../../network/httpserver/server.go#handle{go}

## 試す: テキストが構造になる

リクエストのテキストを編集すると、パースされた構造(メソッド・パス・クエリ・ヘッダ・ボディ)と、
サーバが返すレスポンスのテキストが見える。壊れたリクエストを入れると 400 になる。

<HttpDemo />

これはブラウザ内での再現だが、Go 実装は本物の TCP で同じことをしている。
テストでは実際にポートを開き、`net.Dial` で生の HTTP テキストを流して確認している。
`curl http://localhost:...` でも叩ける本物のサーバだ。

## メリット / デメリット(この自作実装の)

**メリット**

- HTTP の全体像が見える。フレームワークが裏で隠していた処理が見えるようになる
- 依存ゼロ(標準の net だけ)。何が起きているか完全に追える

**デメリット(実運用に足りないもの)**

- keep-alive なし(毎回接続を張り直すのは遅い。実物は1接続で複数リクエスト)
- chunked encoding 非対応(サイズ不明のストリーミングができない)
- タイムアウト・最大ボディサイズ・並行数制限なし(そのまま公開すると DoS に弱い)
- HTTPS(TLS)なし

**実例**

- Go の `net/http`(この章はその素朴版。実物は keep-alive・HTTP/2 まで持つ)
- 全ての Web フレームワーク(Express, Rails, Django...)の下にこの層がいる
- [Rate Limiter](./rate-limiter) や proxy は、この HTTP サーバの前に立つ部品

## 実物との距離: HTTP/2, HTTP/3

この章の HTTP/1.1 は「1接続で1リクエスト、テキストで」だった。実物はさらに進んでいる。

- **HTTP/2**: 1つの TCP 接続で複数リクエストを同時に運ぶ。テキストをやめてバイナリの枠(フレーム)で区切る。[HTTP/2](./http2) の章で作る
- **HTTP/3**: TCP をやめて UDP ベースの QUIC に載せ替え、接続確立を速くした
- **TLS**: 全部を暗号化する層。[TLS ハンドシェイク](./tls-handshake)の章で中身を作る

## 簡略化したこと

- **keep-alive なし**: 1接続1リクエストで閉じる
- **chunked transfer なし**: ボディは Content-Length 必須
- **ルーティングは完全一致**: パスパラメータ(`/users/:id`)やワイルドカードなし
- **TCP は net 任せ**: 3-way handshake や再送は Go が担う。その中身は [TCP](./tcp) の章で作った

## 参考資料

- [RFC 9110/9112](https://www.rfc-editor.org/rfc/rfc9112) — HTTP/1.1 メッセージ形式の規格
- [Go net/http のソース](https://github.com/golang/go/tree/master/src/net/http) — 実物のリクエストパーサ
- [Beej's Guide to Network Programming](https://beej.us/guide/bgnet/) — ソケットプログラミングの定番入門
