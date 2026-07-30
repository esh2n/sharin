<script setup>
import ClaudeLineageDemo from '../components/ClaudeLineageDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# Claude(Constitutional AI)

<Summary>
Claude の系譜を特徴づけるのは、後段学習の設計思想だ。RLHF は人間のラベラーに依存し、有害コンテンツの選別を人に負わせ、判断基準がラベラー集団の中に暗黙に埋まる。Constitutional AI は基準を明文化した原則の文書に置き換え、モデル自身が原則に照らして自分の応答を批評・修正し、AI が選好を採点する(RLAIF)。この章ではその 2 段階の仕組みと、Claude 各世代の展開を追う。
</Summary>

## この章で読むもの

[学習パイプライン](/parts/llm-training) で見たとおり、モデルの答え方を決めるのは後段の選好学習だった。各社の違いが最も出るのもここだ。Anthropic(2021 年創業、OpenAI の安全性研究者らが設立)が作る Claude は、この後段を Constitutional AI という公開された手法で組み立てている点で系譜として区別できる。この章はその仕組みを RLHF の問題点から出発して追い、Claude 各世代が何を積んできたかを整理する。

なお、この教科書の執筆を手伝っているのも Claude であり、その意味でこの章だけは対象と語り手が重なる。記述は公開されている論文とモデルカードに基づける。

<FigureBox caption="RLHF と Constitutional AI の対比。判断基準の置き場所が、人間ラベラーの頭の中から、明文化された原則の文書へ移る">

```
RLHF                             Constitutional AI
人間が応答ペアの優劣を選ぶ          AI が「憲法」(明文化された原則)に
   ↓                              照らして自分の応答を批評・修正
基準はラベラー集団に暗黙            ↓
有害内容の選別を人が浴びる          AI が原則に基づき選好を採点(RLAIF)
基準の変更 = ラベルの取り直し       基準の変更 = 文書の改訂
```

</FigureBox>

順に見ていく。

1. **基準の明文化**: 「何が良い応答か」を原則の文書(憲法)に書き下す。基準が検査可能・改訂可能になる
2. **自己批評 → 修正**: モデル自身が原則に照らして応答を批評し、書き直す。その修正版が学習データになる
3. **選好の採点も AI へ(RLAIF)**: 人間の選好ラベルの代わりに、原則を参照する AI の採点で報酬モデルを作る

## ① 出発点: HHH と RLHF の限界

Anthropic の初期論文(Askell ら、2021)は、アシスタントの目標を helpful・harmless・honest(役に立つ・無害・正直)の 3 語に整理した。この HHH は業界共通の語彙になったが、実現手段の RLHF には構造的な問題が残っていた。

まず人的コストと倫理の問題がある。harmless を学習させるには有害な応答例の比較が要り、その選別作業を人間のラベラーが浴び続けることになる。次に一貫性の問題で、数千人のラベラーの判断は揺れ、「何が良いか」の基準はどの文書にも書かれないまま報酬モデルの重みに溶け込む。基準を変えたければラベルを取り直すしかなく、なぜこの応答が選ばれるのかを後から検査する方法もない。

## ② Constitutional AI: 批評と修正の自動化

Bai らの論文(2022)が示した Constitutional AI(CAI)は、この構造を 2 段階で置き換える。

第 1 段階は教師データの生成だ(SL-CAI)。有害な誘導を含むプロンプトにモデルがまず素直に応答し、次に憲法から原則を 1 つ引いて「この応答は原則に照らしてどこが問題か」を自分で批評させ、批評を踏まえて応答を書き直させる。この「初回応答 → 自己批評 → 修正版」の修正版を集めて SFT する。有害例の選別を人間が行う工程が消える。

第 2 段階が選好学習で、ここが RLAIF(RL from AI feedback)と呼ばれる部分になる。応答ペアの優劣を、人間ではなく原則を参照する AI が採点し、その選好データで報酬モデルを作って RL を回す。[学習パイプライン](/parts/llm-training) の RLHF と骨組みは同じで、選好の供給源だけが人間から AI + 憲法に変わっている。

憲法の中身は国連人権宣言や各社の利用規約などから採られた数十の原則で、文書として公開されている。重要なのは中身より形式だ。判断基準がテキストになったことで、基準は差分レビューでき、バージョン管理でき、批判を受けて改訂できる。RLHF では重みの中に埋まっていたものが、検査可能な人工物になった。

論文の実験では、CAI で学習したモデルは RLHF 版より harmless の評価が上がり、かつ「答えられません」だけの回避的応答が減った。断る場合も理由を説明する方向に寄る。無害化が有用性の犠牲を伴うという想定に対し、両立の余地を示したのが技術的な貢献になる。

## ③ 系譜: 各世代が積んだもの

後段学習の思想を土台に、Claude の各世代は文脈長と道具の使用を積み上げてきた。

- **Claude 1 / 2(2023)**: CAI を載せた最初の製品世代。Claude 2 で文脈窓を 100k トークンへ広げ、長文書の読解を早期の差別化にした([RoPE](/parts/rope) で見た長コンテキスト技術の応用先)
- **Claude 3(2024)**: Haiku / Sonnet / Opus の 3 段構成を導入。速度・費用と能力のトレードオフをモデル選択として利用者に渡す設計で、以後の各社が追随した。画像入力にも対応し、文脈窓は 200k に
- **Claude 3.5 / 3.7(2024-25)**: コーディング能力の強化と computer use(画面を見てマウスとキーボードを操作する)の導入。テキスト補完から、環境に作用するエージェントへの一歩
- **Claude 4 系(2025〜)**: 長時間のエージェント作業(数時間規模のコーディングタスク)へ最適化。thinking(推論モード)も加わり、[推論モデル](/parts/reasoning-models)の軸と合流した

運用面の特徴として、システムプロンプトの公開、モデルカードでの評価開示、能力の閾値ごとに安全対策を定める RSP(責任あるスケーリング方針)がある。いずれも「基準を文書にして外部から検査可能にする」という CAI と同じ形をしている。

### 動かす

下のデモは 2 つの見方を用意した。「CAIの流れ」は有害な誘導を含む依頼に対する、初回応答 → 原則の引用 → 自己批評 → 修正版という CAI のデータ生成 1 サイクルを追う。「系譜」は Claude 各世代の文脈窓の伸びと積まれた能力を年表で確かめられる。

<ClaudeLineageDemo />

## 設計の観点

- **RLHF と RLAIF の使い分け**: RLAIF は安く一貫するが、採点者もモデルなので盲点を共有しうる。実運用は人間の監査を要所に残すハイブリッドが標準で、CAI も harmless 側を RLAIF に、helpful 側は人間の選好を使った
- **基準の明文化はバージョン管理**: 憲法の改訂は差分として追跡できる。暗黙の基準(ラベラーの傾向)にはできない運用で、モデルの挙動変更に説明責任を持たせる仕組みとして読める
- **回避的応答は安全ではない**: 「お答えできません」を連発するモデルは harmless に見えて helpful を失う。CAI の評価が「無害かつ非回避」を測ったのは、この 2 軸が独立に動くから
- **文脈長は製品の軸になる**: 100k を最初に切った判断は、RAG では代替しにくい「文書を丸ごと読む」用途を開いた。長コンテキストの計算コスト([attention変種](/parts/attention-variants))を払う価値の実例
- **エージェント化と安全性の結合**: 画面操作やコード実行は失敗の影響が実世界に及ぶ。後段学習の設計と能力の解放順序(RSP)が結びついているのは、その帰結として説明できる

## 対照と裏どり

| 手法 / 世代 | 選好の供給源 | 基準の所在 | 特徴 |
|---|---|---|---|
| RLHF(InstructGPT) | 人間ラベラー | 暗黙(報酬モデルの重み) | 原型。人的コスト大 |
| Constitutional AI | AI + 憲法 | 明文の原則文書 | 検査・改訂可能、回避的応答が減る |
| Claude 3 以降の運用 | ハイブリッド + RSP | 公開文書群 | モデルカード・システムプロンプト公開 |

裏どり:

- **Askell et al.(2021)**: [A General Language Assistant as a Laboratory for Alignment](https://arxiv.org/abs/2112.00861)。HHH の枠組みの初出
- **Bai et al.(2022a)**: [Training a Helpful and Harmless Assistant with RLHF](https://arxiv.org/abs/2204.05862)。Anthropic 版 RLHF の基礎
- **Bai et al.(2022b)**: [Constitutional AI: Harmlessness from AI Feedback](https://arxiv.org/abs/2212.08073)。CAI/RLAIF の原典
- **Anthropic の公開文書**: [憲法の全文](https://www.anthropic.com/news/claudes-constitution)、モデルカード、RSP。基準を文書で公開する運用の実例

## 簡略化したこと

- **憲法の個々の原則の内容には踏み込まない**: 原文が公開されているため、形式(明文化・改訂可能性)の意義に絞った
- **内部の学習詳細は公開情報の範囲**: Claude 3 以降の学習構成は論文化されておらず、モデルカードに書かれた範囲で記述した
- **評価数値は書かない**: ベンチマークは世代交代で陳腐化が速いため、何が積まれたかの構造だけを追った

## 参考資料

- Bai et al., [Constitutional AI: Harmlessness from AI Feedback](https://arxiv.org/abs/2212.08073)(2022) — この章の中心文献
- Askell et al., [A General Language Assistant as a Laboratory for Alignment](https://arxiv.org/abs/2112.00861)(2021) — HHH
- [Claude's Constitution](https://www.anthropic.com/news/claudes-constitution) — 憲法の全文と選定理由
- [Anthropic Responsible Scaling Policy](https://www.anthropic.com/news/anthropics-responsible-scaling-policy) — 能力閾値と安全対策の対応表
