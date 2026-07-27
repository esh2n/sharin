# http2 — 多重化とヘッダ圧縮

HTTP/2 の要である、1 接続上のストリーム多重化と HPACK ヘッダ圧縮を最小構成で実装する。中心は「HTTP/1.1 のヘッドオブラインブロッキングを、フレームの交互配置で解く」を完了時刻の比較で示すこと。

## 肝

- **フレーム**: 接続を流れる最小単位。どのストリームのものかを StreamID で示す。HEADERS と DATA
- **多重化**: 1 本の接続の上に複数ストリーム。各応答をフレームに刻んで交互に流す。大きな応答の合間に小さな応答を差し込める
- **ヘッドオブラインブロッキング(HTTP/1.1)**: 1 接続で一度に 1 応答。大きな応答が詰まると後ろの小さな応答が待たされる
- **HPACK**: 一度送ったヘッダを動的表に覚え、次からは索引で参照する。毎回同じ Host / User-Agent を送る無駄を省く
- **仕事量は同じ**: 多重化は総送信量を減らさない。小さな応答を先に完了させて体感を良くする(全完了時刻はむしろ少し延びる)

## 効果の固定(テスト)

- `TestHeadOfLineBlocking`: 大 1・小 2 の応答で、小さな応答が H1 では 11/12 tick、H2 では 2/3 tick で完了
- `TestHPACKCompressesRepeats`: 二度目の同一ヘッダは索引参照になり、バイト数がヘッダ数ぶんに縮む

## 使い方

```go
enc, dec := http2.NewEncoder(), http2.NewDecoder()
fields := enc.Encode(headers)       // 初出は literal、既出は索引
headers2 := dec.Decode(fields)      // 表を同期して復元

h1 := http2.CompletionTicksH1(sizes) // 直列(HoL)
h2 := http2.CompletionTicksH2(sizes) // 多重化
frames := http2.Multiplex(sizes)     // 交互配置されたフレーム列
```

## 簡略化したこと

- **HPACK は動的表のみ**: 実物は静的表 + ハフマン符号化 + サイズ上限。ここは索引参照の発想だけ
- **フロー制御・優先度なし**: 実物の WINDOW_UPDATE や依存木は扱わない
- **TCP レベルの HoL は残る**: HTTP/2 はアプリ層の HoL を解くが、TCP のパケット順序による HoL は残る。それを解くのが HTTP/3(QUIC)
- **論理的な tick**: 1 フレーム送信を 1 と数える。実際の帯域・RTT は扱わない

## 章

教科書: [HTTP/2 多重化](https://sharin-2a1.pages.dev/parts/http2)

実行: `go test ./network/http2/`
