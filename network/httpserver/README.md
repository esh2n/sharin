# network/httpserver

HTTP/1.1 サーバを TCP ソケットから自作。ネットワーク編の入口。

## 肝

- HTTP は「TCP の上を流れる人間可読なテキスト」。net.Listen("tcp") で生のバイト列を受け、
  リクエストのテキストを手でパースし、レスポンスのテキストを手で組む
- リクエストの形は「リクエストライン → ヘッダ群 → 空行 → ボディ」。ボディの切れ目は
  Content-Length で知る
- サーバの本体は Accept ループ。接続を得て、1接続ごとに goroutine で処理する

## 簡略化したこと

- HTTP/1.1 の一部のみ。keep-alive なし(1接続1リクエストで閉じる)
- chunked transfer-encoding 非対応(Content-Length のみ)
- ルーティングは「メソッド+パス」完全一致(パスパラメータ・ワイルドカードなし)
- TLS(HTTPS)なし。TCP そのものは Go の net に任せる(自作 TCP は発展)

本文: [教科書の章](../../docs/parts/http-server.md) / 実行: `go test ./network/httpserver/`
