<script setup>
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# グラフを CI に写す

<Summary>
自分で組んだグラフは節と辺だったが、CI のワークフローも節と辺になっている。写すと対応がそのまま付く。決める節を1つに保てば呼び出しは1回で済む。増やしても選べることは増えず、費用だけが上がる。そして出す操作と公開する操作は、Cloud Run でも Workers でも別のコマンドに分かれている。フラグの仕組みを別に持たなくても、出荷の関門はそこにある。
</Summary>

## この章で読むもの

この章もコードを書かないが、動かす形は書く。

先に、言葉を2つ置く。どちらも「言語モデルに仕事をさせる枠組み」の形の名前になる。

**共通しているのは、止まるまで繰り返すこと**だ。どちらも放っておけば回り続けるので、どちらにも上限が要る。違うのは**回る道**で、そこが分かれ目になる。

- **ループ**: 1手ごとにモデルへ「次はどうする?」と訊く。順番もモデルが決める
- **グラフ**: 順番を先に書いておく。書けないところだけモデルに訊く

同じ仕事を両方でやると、こうなる。

<FigureBox caption="バグを1つ直す仕事を、ループとグラフで。道具を使う回数は同じ4手なのに、モデルに訊く回数が違う">

```
  ループ   次に何をするかを毎回訊く

    [訊く] ─→ search      観測を見て
    [訊く] ─→ read        また訊く
    [訊く] ─→ edit
    [訊く] ─→ test
    [訊く] ─→「終わり」                     道具 4 手 / モデル 5 回


  グラフ   順番は先に書いてある。訊くのは 1 箇所だけ

    search ──→ read ──→ [訊く] edit ──→ test ──┐
      ↑固定      ↑固定       ↑ここだけ    ↑固定  │
      └──────────────────── edit へ戻す ◀──── 落ちたら

                                            道具 4 手 / モデル 1 回
```

</FigureBox>

**節**というのは、グラフの1つ1つの箱のことだ。`search` も `edit` も節になる。**辺**は箱と箱をつなぐ矢印で、次にどこへ行くかを表す。落ちたときに戻る先も、辺として先に書いておく。

分かれ目は1つだけになる。**経路を先に描けるかどうか**。描けるならグラフ、走ってみないと分からないならループになる。詳しくは[エージェントの枠組み](/parts/agent-harness)で作って測った。

この章では、その**グラフのほうを実務に置く**。置き場所はもう手元にある。**CI のワークフローが、すでに節と辺でできている**からだ。ステップが節で、その並びが辺になる。

<FigureBox caption="自分で組んだグラフと、CI のワークフローの対応。名前が違うだけで、構造は同じものになる">

```
  本書のグラフ                CI のワークフロー
  ------------------------------------------------------------------
  節 (Node)                  ジョブ / ステップ
  辺 (Next)                  needs / ステップの順番
  Decide: true               モデルを呼ぶステップ(1 つだけ)
  Check                      終了コード
  Retry                      失敗したときに戻る先
  MaxVisits                  再実行の上限 / timeout
  道具 (Tool)                コマンド
  観測 (Obs)                 標準出力と終了コード
  ------------------------------------------------------------------

  違うのは 1 つだけ。CI には「出荷の節」がある
```

</FigureBox>

順に見ていく。

1. **CI はすでにグラフになっている**: 節と辺があり、失敗の戻り先と上限もある
2. **決める節は1つに保つ**: 増やしても選べることは増えず、費用だけが上がる
3. **落ちたらどこへ戻るか**: やり直しの単位は、ジョブの切り方で決まる
4. **出す節と、開ける節を分ける**: Cloud Run も Workers も、この2つが別のコマンドになっている

## ① CI はすでにグラフになっている

図の対応表がそのまま効く。順番は先に書いてあり、失敗したら止まり、上限がある。自分で `Graph` を実装しなくても、この形はもう手元にある。

小さな例を1つ置く。issue を1件受け取って、直して、確かめて、PR にするところまで。

```yaml
name: fix-issue

on:
  issues:
    types: [labeled]

permissions:
  contents: write
  pull-requests: write
  id-token: write        # 鍵を置かずに認証するのに要る

jobs:
  fix:
    if: github.event.label.name == 'agent-fix'
    runs-on: ubuntu-latest
    timeout-minutes: 20   # 実時間の上限。回数の上限とは別に置く
    steps:
      # ── 固定の節。順番は変わらないので訊かない ──
      - uses: actions/checkout@v4
      - run: npm ci
      - run: npm run lint

      # ── 決める節。ここだけモデルに訊く ──
      - uses: anthropics/claude-code-action@v1
        with:
          anthropic_api_key: ${{ secrets.ANTHROPIC_API_KEY }}
          prompt: |
            issue #${{ github.event.issue.number }} を直す。
            直したら npm test を走らせ、通るまで直す。
            通ったら差分の要約を出す。
          claude_args: |
            --max-turns 12
            --allowedTools Read,Edit,Bash(npm test)

      # ── 固定の節。ここが関門になる ──
      - run: npm test
      - run: npm run e2e
      - uses: peter-evans/create-pull-request@v6
        with:
          branch: agent/issue-${{ github.event.issue.number }}
          title: "fix: #${{ github.event.issue.number }}"
```

`--max-turns 12` が枠組みの章の `MaxVisits` に当たる。**これを置かないと止まらない**のは、あそこで測ったとおりだ。`timeout-minutes` は別の上限で、回数ではなく実時間を見る。[Hermes と Pi で読み替える](/parts/agent-stack)の③で「止まらなくなる形が2つあるうち、片方しか塞がれていない」と書いたが、CI では両方置ける。

`--allowedTools` で道具を絞っているのは、[隔離して走らせる](/parts/agent-isolation)の②で見た形になる。ここでは `Bash` を丸ごと渡さず、`npm test` だけに絞った。走っているのが使い捨ての実行環境なので隔離の側は最初から効いているが、**絞れるものは絞っておくほうが読みやすい**。

## ② 決める節は1つに保つ

上のワークフローで、モデルを呼んでいるステップは1つだけだ。ここを崩すと何が起きるかを、測ってみる。

やりがちなのは、各ステップにモデルを置くことになる。「lint の結果を見て判断させる」「テストの失敗をモデルに読ませてから次を決めさせる」。1つずつは自然に見える。

枠組みの章の実装で、**経路も道具の手数もそのままにして、訊く節の数だけを変えた**。

<<< ../../llm/harness/harness_test.go#decide{go}

| 形 | 道具の手数 | モデルの呼び出し | 渡した文字 | 経路を選べるか |
|---|---|---|---|---|
| ループ(毎手訊く) | 4 | 5 | 189 | **選べる** |
| グラフ 訊く節1つ | 4 | **1** | **47** | 選べない |
| グラフ 訊く節2つ | 4 | 2 | 103 | 選べない |
| グラフ 訊く節3つ | 4 | 3 | 125 | 選べない |
| グラフ 訊く節4つ | 4 | 4 | 125 | 選べない |

読みどころは右端の列にある。**訊く節を4つに増やしても、モデルが選べることは1つも増えていない**。経路は先に描いたまま変わらないので、各節で使う道具が選べるだけだ。ループは順番そのものをモデルが決めるので、**同じ費用を払うなら、選べることは多い**。

つまり「ループに戻る」というのは正確ではない。**戻るのはループの費用のほうだけで、ループの自由度は戻ってこない**。ループとグラフの中間ではなく、両方の悪いところに寄っていく。

やめどきの目安もここから出る。訊く節を増やすなら、**その節で本当に判断が要るのか**を見る。要らないなら費用だけが増える。全部の節で訊くところまで行ったら、いっそループにしたほうが筋が通る。

### 訊く位置で値段が変わる

もう1つ、同じ「訊く節1つ」でも置く場所で値段が変わる。

| 訊く節 | モデルの呼び出し | 渡した文字 |
|---|---|---|
| `search`(最初) | 1 | **0** |
| `read` | 1 | 22 |
| `edit` | 1 | 47 |
| `test`(最後) | 1 | **56** |

呼び出しはどれも1回なのに、渡す量は単調に増える。**その節に着くまでの記録を全部運ぶ**からだ。最初の節では記録がまだ空なので、渡すものが無い。テストで、この単調増加と、最初の節が 0 になることを固定した。

上の表で訊く節3つと4つが同じ125文字なのも、これで説明がつく。増やしたのが `search`、つまり記録が空の位置だったからだ。**節を1つ足す費用は、その節がどこにあるかで決まる**。

実務に直すとこうなる。ワークフローの後ろのほうにモデルを足すのは、前のほうに足すより高い。後段ほど、それまでのログや差分を抱えているからだ。

### 訊く節かどうかの見分け方

見分け方は、**そのステップの次に来るものが、結果によって変わるか**になる。変わらないなら訊く必要が無い。`npm ci` の次が `npm run lint` なのは決まっている。`npm test` が落ちたときに `edit` へ戻るのも決まっている。決まっていないのは、**どう直すか**だけだ。

判断が要るように見えて要らないものが2種類ある。

**分岐の条件が書けるもの**。「テストが落ちたら直す節へ戻る」はモデルに訊く必要が無い。終了コードで分かる。訊くべきなのは「戻るかどうか」ではなく「どう直すか」だ。ここを取り違えると、`test` の節が決める節になる。上の表で言えば、いちばん高い位置に1つ足すことになる。

**そもそも決定的にできるもの**。「lint が落ちたらモデルに直させる」は `npm run lint --fix` で済むことがある。**決定的にできるものを決定的にする**のが、この層でいちばん効く。訊く節を減らすだけでなく、揺らぎも減る。

裏を返すと、訊く節が1つに保てているかどうかは、**その仕事をどこまで書き下せたかの指標**になる。増えているなら、まだ書き下せていない判断が残っているか、書き下せるのに書いていないかのどちらかだ。

## ③ 落ちたらどこへ戻るか

枠組みの章のグラフでは、`test` が落ちたら `edit` へ戻ると先に書いてあった。だから `search` と `read` はやり直さない。手数は8から6、呼び出しは9から2になった。

CI に写すと、**戻る先はジョブの切り方で決まる**。1つのジョブに全部入れれば、落ちたときにやり直すのはジョブ全体になる。分ければ、落ちたところから戻れる。

| 切り方 | 落ちたときにやり直すもの | 何が要るか |
|---|---|---|
| 1ジョブに全部 | 全部(チェックアウトから) | 何も |
| 準備 / 直す / 確かめる で分ける | 落ちたジョブから | 成果物の受け渡し |
| 直す節の中で閉じる | その節だけ | モデルが自分でテストを走らせる |

3番目がいちばん安い。上のワークフローで `prompt` に「通るまで直す」と書いたのは、そのためになる。**往復がステップの中で閉じていれば、ジョブをやり直す必要が無い**。[人が見る前に落とす](/parts/agent-gates)の③で見た「自分で読める合否」が、ここで効いている。

そのうえで、外側の `npm test` を消してはいけない。モデルの中で走ったテストと、固定の節で走るテストは**役割が違う**。前者は直しの往復を回すためのもので、後者は**通ったと主張していることを確かめるため**のものだ。同じコマンドでも、置いてある層が違う。

外側にもう1つ足すなら、書いた本人でない目になる。

```yaml
      - uses: anthropics/claude-code-action@v1
        with:
          anthropic_api_key: ${{ secrets.ANTHROPIC_API_KEY }}
          plugin_marketplaces: "https://github.com/anthropics/claude-code.git"
          plugins: "code-review@claude-code-plugins"
          prompt: "/code-review:code-review ${{ github.repository }}/pull/${{ github.event.pull_request.number }}"
```

これは差分だけを新しいコンテキストで見る形で、[人が見る前に落とす](/parts/agent-gates)の④に当たる。**同じ実行の中で書いた本人に見せるのではなく、PR ができてから別の起動で見る**ところに意味がある。

## ④ 出す節と、開ける節を分ける

ここが、自分で組んだグラフには無かった節になる。

[人が見る前に落とす](/parts/agent-gates)の⑤で、デプロイと公開を切り離す形を見た。フラグの仕組みを別に持つ話として書いたが、**実際には多くの実行基盤が、この2つを最初から別のコマンドにしている**。

### Cloud Run

リビジョンとトラフィックが別になっている。出しても、送らなければ誰にも見えない。

```bash
# 出す。トラフィックは 0 のまま。tag を付けると専用の URL が生える
gcloud run deploy SERVICE --image IMAGE_URL --no-traffic --tag canary

# 自分だけで確かめる(canary の URL を叩く)

# 少しだけ開ける
gcloud run services update-traffic SERVICE \
  --to-revisions SERVICE-00042-abc=10,SERVICE-00041-xyz=90

# 全部開ける
gcloud run services update-traffic SERVICE --to-latest

# 閉じる。前のリビジョンへ 100 を戻すだけ
gcloud run services update-traffic SERVICE --to-revisions SERVICE-00041-xyz=100
```

閉じる操作が**デプロイのやり直しではない**ところが要点になる。ビルドし直さないので速い。前の章で「閉じるのを速くする」と書いたのは、この形のことだ。

### Cloudflare Workers

同じ構造を、別の名前で持っている。上げるのと配るのが別のコマンドになる。

```bash
# 上げる。まだ配られない
npx wrangler versions upload

# 一覧を見る(id を取る)
npx wrangler versions list --json

# 割合を指定して配る。対話を挟まないなら -y
npx wrangler versions deploy <new-version-id>@10 <old-version-id>@90 -y

# 全部を新しい版に
npx wrangler versions deploy <new-version-id>@100 -y

# 閉じる
npx wrangler rollback <old-version-id> --message "error rate up"
```

**2つの製品が、別々に同じ形に行き着いている**のが読みどころになる。「出す」と「公開する」を分けるのは、フラグという製品を買うかどうかの話ではなく、**出荷という操作の性質**だということだ。より細かく、コードの経路単位で切りたくなったときに、専用のフラグの仕組みが要る。

### 資格情報を CI に置かない

最後に1つ。[隔離して走らせる](/parts/agent-isolation)の④で「渡した資格情報は境界を越える」と書いた。CI はまさにその場所になる。長命の鍵を秘密として置くと、そのワークフローを踏める全員が使えることになる。

GCP なら、鍵を置かずに済む形がある。GitHub が発行する短命の証明を、Google 側が受け取って交換する。

```yaml
permissions:
  id-token: write        # これが無いと証明が発行されない

steps:
  - uses: google-github-actions/auth@v2
    with:
      workload_identity_provider: ${{ secrets.GCP_WORKLOAD_IDENTITY_PROVIDER }}
      service_account: ${{ secrets.GCP_SERVICE_ACCOUNT }}
```

秘密として置くのは**識別子だけ**で、鍵そのものは無い。得られる資格情報は1時間で切れる。「最小限で、短命のものを渡す」を、置き場所ごと消す形で満たしていることになる。

## 設計の観点

- **既にあるグラフを使う**: CI は節と辺と失敗の辺と上限を持っている。自分で組み直す前に写せないか見る
- **決める節を数える**: モデルを呼ぶステップが2つ以上あるなら、グラフではなくループになっていないか疑う
- **次が結果で変わるかで判断する**: 変わらないなら訊かない。変わるように見えて `--fix` で済むこともある
- **回数と実時間の両方に上限を置く**: CI では両方置けるので、片方だけにしない
- **やり直しの単位はジョブの切り方**: 全部1つに入れると、落ちるたびに全部やり直す
- **内側のテストと外側のテストを両方持つ**: 前者は往復を回すため、後者は主張を確かめるため
- **採点は別の起動で**: 同じ実行の中で書いた本人に見せるのは、採点にならない
- **出すと公開するを分ける**: 多くの実行基盤が、既に別のコマンドとして持っている
- **閉じる操作を速くする**: 巻き戻しがビルドのやり直しなら、閉じる判断そのものが遅れる
- **鍵を置く代わりに交換する**: 秘密として置くのが識別子だけで済むなら、そのほうが漏れようがない

## 対照と実例

| | 本書のグラフ | GitHub Actions | Cloud Run | Cloudflare Workers |
|---|---|---|---|---|
| 節 | `Node` | ジョブ / ステップ | — | — |
| 決める節 | `Decide: true` | モデルを呼ぶステップ | — | — |
| 合否 | `Check` | 終了コード | — | — |
| 戻る先 | `Retry` | ジョブの切り方 | — | — |
| 回数の上限 | `MaxVisits` | `--max-turns` | — | — |
| 実時間の上限 | 無し | `timeout-minutes` | — | — |
| 出す | 無し | — | `--no-traffic --tag` | `versions upload` |
| 開ける | 無し | — | `update-traffic --to-revisions` | `versions deploy <id>@N` |
| 閉じる | 無し | — | 前のリビジョンへ 100 | `rollback` |

裏どり:

- **Cloud Run**: 「`gcloud run deploy myservice --image IMAGE_URL --no-traffic --tag TAG_NAME`」でトラフィックを送らずに出せて、tag を付けると専用の URL が生える。割り振りは `gcloud run services update-traffic SERVICE --to-revisions LIST` で、`REVISION1=PERCENTAGE1,REVISION2=PERCENTAGE2` の形。最新へ全部送るのが `--to-latest`、巻き戻しは前のリビジョンに `=100` を指定する
- **Cloudflare Workers**: `npx wrangler versions upload` は「自動では配られない新しい版を作る」。配るのが `npx wrangler versions deploy` で、`[<version-id>@<percentage>..]` の短縮形と `--yes`(`-y`)がある。巻き戻しは `wrangler rollback [<VERSION_ID>]` で、省略すると1つ前になる。一覧は `wrangler versions list`(`--json` あり)
- **GitHub Actions の口**: `anthropics/claude-code-action@v1`。`prompt` に指示、`claude_args` に CLI の引数をそのまま渡す形で、`--max-turns` は既定 10。`prompt` にはスキルの呼び出しも書ける。ベータからの移行で `max_turns` などの個別入力が `claude_args` に集約されている
- **鍵を置かない認証**: `google-github-actions/auth@v2` に `workload_identity_provider` と `service_account` を渡す形。ジョブに `id-token: write` が要る。公式の手引きも「Workload Identity Federation はダウンロード可能なサービスアカウント鍵を不要にし、安全性を高める」と書いている。**得られる資格情報が1時間で切れるという点は二次資料でしか確認できていない**
- **決める節を1つに保つ効果**: 枠組みの章で測った 5回 対 1回、189字 対 47字がそのまま当たる。CI に写した場合の実測は**していない**
- **上で書いたワークフロー**: 動かしていない。書式は公式の例に合わせたが、そのまま貼って通ることは確かめていない

## 簡略化したこと

- **走らせていない**: この章のワークフローもコマンドも、手元で実行していない。書式は公式の記述に合わせただけになる
- **測っていない**: CI に写したときに呼び出しが何回になるか、費用がどうなるかは測っていない
- **並行を扱っていない**: 複数の issue を同時に流す、matrix で分ける、といった形は書いていない
- **成果物の受け渡しを省いた**: ジョブを分けると、ビルド結果や差分を渡す仕組みが要る。そこは書いていない
- **指標を見て閉じる自動化に踏み込んでいない**: 開けたあと何を見て閉じるかは、それ自体が観測の話になる
- **フラグの製品を並べていない**: 経路単位で切りたくなったときに何を使うかは、比べていない
- **版が動く**: 動作の名前も既定値も版で変わる。ここに書いたのは執筆時点のもの

## 参考資料

- [Claude Code GitHub Actions](https://code.claude.com/docs/en/github-actions) — 決める節をワークフローに置く形
- [ロールアウト、ロールバック、トラフィック移行(Cloud Run)](https://docs.cloud.google.com/run/docs/rollouts-rollbacks-traffic-migration) — 出すと開けるを分けるコマンド
- [Gradual deployments(Cloudflare Workers)](https://developers.cloudflare.com/workers/configuration/versions-and-deployments/gradual-deployments/) — 同じ形を別の名前で
- [Wrangler の Workers コマンド](https://developers.cloudflare.com/workers/wrangler/commands/workers/) — `versions upload` / `versions deploy` / `rollback` の書式そのもの
- [google-github-actions/auth](https://github.com/google-github-actions/auth) — 鍵を置かない認証
- 前の章: [エージェントの枠組み](/parts/agent-harness) / [隔離して走らせる](/parts/agent-isolation) / [人が見る前に落とす](/parts/agent-gates)
