# websocket — Upgrade とフレーミング

WebSocket の要である Upgrade ハンドシェイクとフレーミングを最小構成で実装する。Accept 計算のため SHA-1 と base64 も自作し、RFC 6455 の例と一致することを確かめる。

## 肝

- **Upgrade ハンドシェイク**: 普通の HTTP リクエストとして始まり、Upgrade ヘッダで接続を WebSocket に昇格させる。要求応答の型を脱ぎ、全二重の通路になる
- **Accept 計算**: `base64(SHA-1(Sec-WebSocket-Key + magic GUID))`。サーバがこれを返せることが「WebSocket を理解している」証明。偶然の昇格を防ぐ
- **フレーム**: FIN・オペコード(text/binary/close/ping/pong)・マスク有無・長さ・ペイロード。長さは 125 以下=1 バイト、以上は 126/127 で拡張
- **マスキング**: クライアント→サーバのフレームは必ず鍵で XOR する。中継プロキシのキャッシュ汚染攻撃を防ぐための決まり。サーバ→クライアントはマスクしない
- **全二重**: 昇格後は両側がいつでもフレームを送れる。要求と応答の対でなくなる

## 効果の固定(テスト)

- `TestAcceptRFCExample`: RFC 6455 の例と一致(`dGhlIHNhbXBsZSBub25jZQ==` → `s3pPLMBiTxaQ9kYGzzhZRbK+xOo=`)
- `TestSHA1KnownVectors`: 自作 SHA-1 が既知ベクトル(abc・空)と一致
- `TestMaskingHidesPlaintext`: マスクしたフレームのワイヤ上に平文が現れず、復号で戻る

## 使い方

```go
accept := websocket.Accept(clientKey) // ハンドシェイク応答鍵

f := websocket.Frame{Fin: true, Opcode: websocket.OpText, Payload: []byte("hi")}
wire := websocket.Encode(f)          // サーバ→クライアント(マスクなし)
got, n, err := websocket.Decode(wire) // 1 フレーム読む
```

## 簡略化したこと

- **ハンドシェイクの HTTP 部分は Accept のみ**: 実際のヘッダ交換全体は扱わない
- **制御フレームの意味論なし**: close/ping/pong はオペコードのみ。クローズ手順や自動 pong は省略
- **フラグメント再結合なし**: FIN=false の継続フレームの結合は扱わない
- **マスク鍵は手で与える**: 実物は暗号的乱数で毎フレーム生成する

## 章

教科書: [WebSocket](https://sharin-2a1.pages.dev/parts/websocket)

実行: `go test ./network/websocket/`
