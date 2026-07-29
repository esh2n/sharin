<script setup>
import BTreeDemo from '../components/BTreeDemo.vue'
import BTreeNodeView from '../components/BTreeNodeView.vue'
import FigureBox from '../components/figures/FigureBox.vue'
import BarScale from '../components/figures/BarScale.vue'
import Summary from '../components/Summary.vue'

// 図: 1億件を探すときのページ読み回数
const pageReads = [
  { label: '二分探索木', text: '約27回', frac: 1, tone: 'bad' },
  { label: 'B-Tree(分岐300)', text: '4回', frac: 4 / 27, tone: 'good' },
]

// 図: 分割の前後
const beforeSplit = { id: 1, keys: [10, 20, 30], children: [] }
const afterSplit = {
  id: 2,
  keys: [20],
  children: [
    { id: 3, keys: [10], children: [] },
    { id: 4, keys: [30], children: [] },
  ],
}
</script>

# B-Tree

> 実装: [`data-structures/btree/`](https://github.com/esh2n/sharin/tree/main/data-structures/btree) / 実行: `go test ./data-structures/btree/`

<Summary>
ほとんどのデータベースのインデックスの中身、B-Tree を作る。二分探索木を「ディスク向けに太らせた」木で、1つのノードに数百のキーを詰めて枝分かれを増やすと、数十億件あってもページを数回読むだけで目当てに届く。二分探索木とディスクとページ、2つの前提の上に立つ章。
</Summary>

## この章で作るもの

ほぼすべての RDB のインデックスの正体である B-Tree を、挿入と検索に絞って実装する。
[ID Generation](./id-generation) の章で予告した
「なぜ UUIDv7 はインデックスに優しいのか」をデモで実際に見るのがゴール。

::: tip 前提章
この章は2つの前提章の上に立っている。
[二分探索木](./binary-search-tree)(半分ずつ捨てる速さと、崩壊の問題)と、
[ディスクとページ](./disk-and-pages)(ページ読み回数という速さの物差し)を先に読んでほしい。
:::

先に押さえることが3つある。

- B-Tree の存在理由は**ディスク**。速さの単位は比較の回数ではなく**ページ読みの回数**
- だから1ノード = 1ページにキーを大量に詰めて枝分かれを太くし、**木を浅くする**
- 挿入は「降りながら、満杯のノードを先に割っておく」(proactive split)だけで書ける

## なぜ二分探索木のままではダメなのか

[二分探索木](./binary-search-tree)は、平衡さえ保てば比較 log2(n) 回で探せた。
メモリの中ならこれで話は終わり。しかしインデックスはディスクに置かれる。
[ディスクとページ](./disk-and-pages)で見たとおり、ディスクはページ単位でしか読めず、
1回のページ読みはメモリアクセスの数千〜10万倍重い。だから
**速さはページ読みの回数で数える**。

二分木のノードは小さい(キー1個とポインタ2本)ので、1億件も入れるとノードは
バラバラのページに散らばる。高さ約27の木をたどる = **最悪27回のページ読み**。

<FigureBox caption="1億件から1件探すときのページ読み回数。比較回数はほぼ同じでも、ディスクに触る回数が違う">
  <BarScale :bars="pageReads" />
</FigureBox>

ここで発想を変える。ページはどうせ8KBまるごと読まれるのだから、
**1ページに収まるだけキーを詰めてしまえばいい**。8KB にはキーとポインタが
数百組入る。枝分かれが300なら:

```
高さ0:                1 ノード ×  300 キー
高さ1:              300 ノード → 9万件
高さ2:            9万 ノード   → 2,700万件
高さ3:                          → 81億件
```

**81億件でもページ読み4回**。比較回数は二分木と大差ない(ノード内でも二分探索する)が、
ページ読み回数が27回から4回になる。これが B-Tree のすべてで、
「浅さを枝分かれの太さで買う」木と言える。

## ノード構造

<<< ../../data-structures/btree/btree.go#node{go}

最小次数 `t` がノードの太さを決める(キー数は最大 2t-1、root 以外は最小 t-1)。
「最小」があるのが重要で、どのノードも半分以上詰まっていることが保証されるから、
木がスカスカに伸びることがない。テストではこの不変条件
(キー数の上下限、全部の葉が同じ深さ、ノード内昇順)を全ノードで検証している。

## 検索

<<< ../../data-structures/btree/btree.go#search{go}

各ノードでキー列を二分探索し、外れたら「挟まる位置」の子へ降りるだけ。
ループ回数 = 高さ+1 = ページ読み回数。

## 挿入と分割

満杯のノード(2t-1 キー)に挿入はできないので、どこかで**分割**が要る。
素朴にやると「葉に入れる → 溢れたら割って親に昇格 → 親も溢れたら…」と
上に波及して戻る処理になるが、CLRS 流はこれを逆転させる:
**降りる途中で、満杯の子を先に割っておく**。こうすると親は必ず空きがある状態で
昇格を受け取れるので、処理が一方通行になる。

<<< ../../data-structures/btree/btree.go#insert{go}

分割はこう動く。真ん中のキーが親に昇格し、残りが左右に分かれる。
t=2(1ノード最大3キー)の木に 10, 20, 30 まで入れて、4件目を入れようとした瞬間:

<FigureBox caption="左: 満杯のノード(これ以上入らない)。右: 分割後 — 真ん中の20が昇格して新しい root になり、木が1段高くなる">
  <div class="split-fig">
    <BTreeNodeView :node="beforeSplit" :touched="[]" />
    <span class="split-arrow">分割</span>
    <BTreeNodeView :node="afterSplit" :touched="[2]" />
  </div>
</FigureBox>

<style>
.split-fig { display: flex; align-items: center; justify-content: center; gap: 24px; }
.split-arrow { font-size: 13px; color: var(--vp-c-text-2); white-space: nowrap; }
</style>

コードではこうなる:

<<< ../../data-structures/btree/btree.go#split{go}

### コードの読みどころ: 木が高くなる瞬間は1箇所だけ

`Insert` の冒頭だけが木の高さを変える。root が満杯のとき、新しい root を作って
旧 root を割る。**B-Tree は上に向かって伸びる**のだ。葉から伸びる二分木と逆で、
これがあるから「すべての葉が同じ深さ」が常に保たれる。

## 実験: 昇順挿入とランダム挿入で「触るノード」を見る

t=2(1ノード最大3キー)の小さな木で、挿入のたびに触ったノードが光る。

**試してみる**:

- 「昇順で1件挿入」を連打すると、光るのは**常に右端の経路だけ**。
  これが UUIDv7 / Snowflake 主キーの世界。触るページが固定されるから
  キャッシュに乗り続け、ページは順に満杯になっていく
- リセットして「ランダムに1件挿入」を連打すると、光る場所が**毎回バラバラ**になる。
  これが UUIDv4 主キーの世界。全ページが中途半端に埋まり、
  どのページもキャッシュから追い出されうる

<BTreeDemo />

実物の DB では、この「触るページの散らばり」が挿入スループットの差になって現れる。
[ID Generation](./id-generation) の「なぜソート可能が効くのか」の実体がこれ。

## 実物との距離: B+Tree

実物の RDB が使うのは正確には **B+Tree** という変種で、この章の実装と2点違う。

- **値(行データやそのポインタ)を葉だけに置く**。内部ノードはキーだけになり、
  さらに大量のキーを詰めて枝分かれを太くできる
- **葉同士を横に連結する**。`WHERE id BETWEEN a AND b` のような範囲検索が
  「a まで降りて、あとは葉を右に歩くだけ」になる

**実例**

- PostgreSQL のインデックス(nbtree)、MySQL InnoDB(テーブル本体が主キーの B+Tree)
- SQLite(テーブルもインデックスも B-Tree ファミリ)
- ファイルシステム(NTFS、Btrfs はその名の通り)

なお「等価検索しかしない」ならハッシュインデックスが O(1) で勝ち、
「書き込みが洪水のように来る」なら LSM-Tree(RocksDB 等)という別解がある。
このあたりの使い分けは db 編で扱う。

## 簡略化したこと

- **削除は未実装**: 隣接ノードからの借用・併合が入り、挿入より一段複雑になる
- **メモリ内のみ**: 実物は1ノード = 1ページをディスクに置き、バッファプールで
  キャッシュする。「ページ読み回数」はこの章では概念で、db 編で実物になる
- **B+Tree ではない**: 上記の通り
- **並行制御なし**: 実物は複数トランザクションが同時に触るためのラッチ制御がある

## 参考資料

- CLRS『アルゴリズムイントロダクション』18章 — この章の実装の出典
- [PostgreSQL nbtree README](https://github.com/postgres/postgres/blob/master/src/backend/access/nbtree/README) — 実物の設計メモ
- [CMU 15-445: Database Systems](https://15445.courses.cs.cmu.edu/) — B+Tree とバッファプールの講義。db 編の主教材候補
