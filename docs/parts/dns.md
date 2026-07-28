<script setup>
import DnsDemo from '../components/DnsDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
import FlowRow from '../components/figures/FlowRow.vue'

const flow = [
  { label: 'ブラウザ', note: '"example.com" の IP は?' },
  { label: 'リゾルバ', note: 'UDP で問い合わせ', state: 'hot' },
  { label: 'DNS サーバ', note: '8.8.8.8 など' },
  { label: '応答', note: '93.184.216.34' },
]
</script>

# DNSリゾルバ

> 実装: [`network/dns/`](https://github.com/esh2n/sharin/tree/main/network/dns) / 実行: `go test ./network/dns/`

<Summary>
ブラウザに "example.com" と打つと、通信の一番最初に「その名前の IP アドレスは?」を問い合わせる。それが DNS。この問い合わせは、HTTP のような読めるテキストではなく、UDP で送る詰まったバイナリメッセージ。この章ではそのバイト列を手で組み立てて実際に DNS サーバに送り、返ってきたバイト列から IP を取り出す。インターネットの「電話帳を引く」部分の中身。
</Summary>

## この章で作るもの

[HTTP サーバ](./http-server)は「IP アドレスが分かった後」の通信だった。
その手前で、**ドメイン名から IP アドレスを引く**のが DNS。`example.com` に
アクセスする前に、必ずこの解決が走っている。

<FigureBox caption="名前解決の流れ。ブラウザは通信の前に、まず名前の IP をリゾルバ経由で DNS サーバに問い合わせる">
  <FlowRow :steps="flow" />
</FigureBox>

この章の肝は3つ。

- DNS の問い合わせは **UDP で送るバイナリメッセージ**。HTTP のようなテキストではない
- メッセージは「**12バイトのヘッダ + 質問 + 回答**」の固定フォーマット
- ドメイン名は **[長さ][ラベル]...** の形で詰め、応答では名前を**ポインタで圧縮**する

## なぜ UDP でバイナリなのか

HTTP は TCP の上のテキストだった。DNS は違う。**UDP**(接続を張らずにパケットを1発
投げる)の上に、**詰まったバイナリ**を載せる。理由は速度と量。名前解決は通信のたびに
何度も走るので、TCP の接続確立(3-way handshake)を省き、1往復で終わらせたい。
バイナリなのも、1パケット(昔は512バイト)に収めるため。

## メッセージを組み立てる

問い合わせメッセージは、ヘッダ・名前・種別を順に詰めるだけ。

<<< ../../network/dns/message.go#encode{go}

**ヘッダ**の先頭2バイトは **ID**。同時に複数の問い合わせを投げるので、
どの応答がどの問い合わせのものかを ID で照合する。次の2バイトはフラグで、
`RD=1`(Recursion Desired)は「あなたが再帰的に最後まで解決して、答えだけください」の意味。
これで自分はルートサーバから辿る必要がなくなる。

**名前**は `[3]www[7]example[3]com[0]` のように、各ラベルの前に長さを置いて 0 で終える。

**試す**: ドメイン名を変えると、組み上がるバイト列が色分けで見える(青=ヘッダ、
緑=名前、黄=種別)。名前の部分に、ラベルの長さバイトと文字(下の小さい ascii)が
交互に並ぶのが見える。このバイト列がそのまま UDP で 8.8.8.8:53 に飛ぶ。

<DnsDemo />

## 応答を解く

DNS サーバは、同じフォーマットに**回答セクション**を足して返す。回答から IP を取り出す:

<<< ../../network/dns/message.go#decode{go}

### コードの読みどころ: 名前のポインタ圧縮

応答には質問と同じドメイン名がもう一度出てくる。それをまるごと繰り返すと無駄なので、
DNS は **「さっき出た名前は、メッセージの N バイト目を見て」というポインタ**で参照する
(先頭2bitが `11` なら圧縮ポインタ)。`skipName` がこれを扱っていて、ポインタに
当たったら「そこで名前は終わり」と判断して2バイト進める。この圧縮があるおかげで
DNS メッセージは小さく収まる。

## 実際に問い合わせる

組み立てた問い合わせを UDP ソケットで送り、応答を受け取る:

<<< ../../network/dns/resolver.go#resolver{go}

`net.Dial("udp", ...)` で UDP の口を開き、問い合わせを `Write`、応答を `Read`。
TCP と違って接続確立の往復がないので、これで1往復。テストでは実際に
ローカルの UDP サーバを立てて、この送受信の経路を検証している。`8.8.8.8` に
向ければ本物の名前解決になる。

## メリット / デメリット / 実物との距離

このミニ実装は「再帰リゾルバに丸投げ」する一番簡単な形。実物の世界はもっと広い。

- **反復解決**: 本当のリゾルバは、ルート → `.com` → `example.com` の権威サーバ、と
  階層を自分で辿る。この章は `RD=1` で 8.8.8.8 に丸投げしている
- **キャッシュ**: 同じ名前を何度も引かないよう、TTL の間は結果を保持する。DNS が
  速いのはキャッシュのおかげ
- **レコード種別**: A(IPv4)だけでなく AAAA(IPv6)、CNAME(別名)、MX(メール)、TXT など
- **セキュリティ**: 平文の UDP は改竄・なりすましに弱い。DNSSEC(署名)、
  DoH/DoT(HTTPS/TLS で暗号化)が対策。ID の照合はなりすまし対策の最小版

**実例**

- `dig` / `nslookup` — この章と同じことをするコマンド
- OS のスタブリゾルバ(`/etc/resolv.conf` の DNS サーバに問い合わせる)
- 8.8.8.8(Google)、1.1.1.1(Cloudflare)などのパブリック再帰リゾルバ

## 簡略化したこと

- **A レコードのみ**: AAAA/CNAME/MX 等は未対応
- **再帰リゾルバ丸投げ**: ルートから辿る反復解決はしない
- **キャッシュ・TTL なし**: 毎回問い合わせる
- **圧縮は読み飛ばしのみ**: ポインタを展開して名前を復元まではしない
- **TCP フォールバック・EDNS・DNSSEC なし**

## 参考資料

- [RFC 1035](https://www.rfc-editor.org/rfc/rfc1035) — DNS メッセージフォーマットの規格
- [Implement DNS in a weekend](https://implement-dns.wizardzines.com/) — DNS を手で実装する定番チュートリアル(反復解決まで)
- [dig の使い方](https://man.archlinux.org/man/dig.1) — 本物の問い合わせを覗く道具
