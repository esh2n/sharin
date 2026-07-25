// Package blockchain はブロックチェーンの最小実装(ハッシュチェーン + Proof of Work)。
//
// ブロックチェーンの正体は「改竄コストを計算量で買う、追記専用のログ」。
// 各ブロックは前のブロックのハッシュを含む。だから過去のブロックを1つ書き換えると、
// そのハッシュが変わり、次のブロックが持つ「前のハッシュ」と食い違い、
// そこから先が全部壊れる。改竄を隠すには全ブロックを作り直すしかなく、
// Proof of Work がその作り直しに莫大な計算を要求する。だから改竄が割に合わなくなる。
//
// これは db 編の「追記だけのログ」(log-structured-kv)と同じ発想を、
// 「信頼できる第三者なしで」成立させたもの。
package blockchain

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// #region block
// Block は1つのブロック。前ブロックのハッシュ(PrevHash)を含むのが鎖の要。
type Block struct {
	Index    int
	Data     string // このブロックの中身(実物は取引の集まり)
	PrevHash string
	Nonce    int    // Proof of Work で総当たりする値
	Hash     string // このブロックのハッシュ(Nonce 込みで計算)
}

// computeHash はブロックの中身から SHA-256 ハッシュを求める。
// Nonce を含めるので、Nonce を変えるとハッシュが総取っ替えになる(PoW で使う)。
func computeHash(b Block) string {
	record := fmt.Sprintf("%d%s%s%d", b.Index, b.Data, b.PrevHash, b.Nonce)
	sum := sha256.Sum256([]byte(record))
	return fmt.Sprintf("%x", sum)
}

// #endregion block

// #region chain
// Chain はブロックの並び。difficulty はハッシュ先頭に要求する 0 の数。
type Chain struct {
	Blocks     []Block
	difficulty int
}

// New は genesis(最初の)ブロックだけを持つチェーンを作る。
func New(difficulty int) *Chain {
	if difficulty < 1 {
		panic("blockchain: difficulty must be >= 1")
	}
	genesis := Block{Index: 0, Data: "genesis", PrevHash: ""}
	remine(&genesis, difficulty)
	return &Chain{Blocks: []Block{genesis}, difficulty: difficulty}
}

// Add は新しいブロックを作り、マイニングして鎖の末尾に繋ぐ。
func (c *Chain) Add(data string) {
	prev := c.Blocks[len(c.Blocks)-1]
	block := Block{
		Index:    prev.Index + 1,
		Data:     data,
		PrevHash: prev.Hash, // 前ブロックのハッシュを取り込む = 鎖
	}
	remine(&block, c.difficulty)
	c.Blocks = append(c.Blocks, block)
}

// #endregion chain

// #region pow
// remine は Proof of Work: ハッシュが難易度ぶんの 0 で始まるまで Nonce を総当たりする。
// ハッシュは予測できないので、条件を満たす Nonce を見つけるには「ひたすら試す」しかない。
// この計算こそが、改竄を割に合わなくする「コスト」の正体。
func remine(b *Block, difficulty int) {
	prefix := strings.Repeat("0", difficulty)
	for b.Nonce = 0; ; b.Nonce++ {
		h := computeHash(*b)
		if strings.HasPrefix(h, prefix) {
			b.Hash = h
			return
		}
	}
}

// #endregion pow

// #region valid
// Valid はチェーン全体の整合を検証する。
// 各ブロックについて: (1)保存されたハッシュが中身と一致するか、
// (2)難易度を満たすか、(3)PrevHash が本当に前ブロックのハッシュか、を確かめる。
func (c *Chain) Valid() bool {
	prefix := strings.Repeat("0", c.difficulty)
	for i, b := range c.Blocks {
		// (1) 中身から計算し直したハッシュが、保存値と一致するか(改竄検出)。
		if computeHash(b) != b.Hash {
			return false
		}
		// (2) Proof of Work を満たしているか。
		if !strings.HasPrefix(b.Hash, prefix) {
			return false
		}
		// (3) 前ブロックとの繋がり。
		if i > 0 && b.PrevHash != c.Blocks[i-1].Hash {
			return false
		}
	}
	return true
}

// #endregion valid
