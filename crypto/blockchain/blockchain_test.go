package blockchain

import (
	"strings"
	"testing"
)

func TestGenesisAndAppend(t *testing.T) {
	chain := New(2) // 難易度2(ハッシュ先頭に "00")
	if len(chain.Blocks) != 1 {
		t.Fatalf("最初は genesis ブロック1つのはず: %d", len(chain.Blocks))
	}
	chain.Add("alice pays bob 10")
	chain.Add("bob pays carol 5")
	if len(chain.Blocks) != 3 {
		t.Errorf("3ブロックになるべき: %d", len(chain.Blocks))
	}
}

// 各ブロックは前のブロックのハッシュを持つ。これが「チェーン」。
func TestBlocksAreLinked(t *testing.T) {
	chain := New(2)
	chain.Add("tx1")
	chain.Add("tx2")
	for i := 1; i < len(chain.Blocks); i++ {
		if chain.Blocks[i].PrevHash != chain.Blocks[i-1].Hash {
			t.Errorf("ブロック%d の PrevHash が前ブロックの Hash と一致しない", i)
		}
	}
}

// Proof of Work: マイニング後のハッシュは難易度ぶんの 0 で始まる。
func TestProofOfWork(t *testing.T) {
	chain := New(3)
	chain.Add("tx")
	last := chain.Blocks[len(chain.Blocks)-1]
	if !strings.HasPrefix(last.Hash, "000") {
		t.Errorf("難易度3なら先頭 000 のはず: %s", last.Hash)
	}
	// nonce を総当たりした結果、実際にそのハッシュになることを再計算で確認。
	if computeHash(last) != last.Hash {
		t.Error("保存されたハッシュがブロック内容と一致しない")
	}
}

func TestValidChain(t *testing.T) {
	chain := New(2)
	chain.Add("tx1")
	chain.Add("tx2")
	if !chain.Valid() {
		t.Error("正当なチェーンが Valid() で false")
	}
}

// 改竄検出: 過去ブロックの中身を書き換えると、そのブロックのハッシュが変わり、
// 次ブロックの PrevHash と食い違って全体が壊れる。
func TestTamperingBreaksChain(t *testing.T) {
	chain := New(2)
	chain.Add("alice pays bob 10")
	chain.Add("tx2")

	// ブロック1の中身をこっそり書き換える。
	chain.Blocks[1].Data = "alice pays bob 1000000"

	if chain.Valid() {
		t.Error("改竄後も Valid() が true(改竄が検出できていない)")
	}
}

// 改竄を隠すにはそのブロックを再マイニングする必要があるが、
// 難易度が高いほどそのコストが上がる。PoW の「改竄コスト」を確認する。
func TestReminingAfterTamper(t *testing.T) {
	chain := New(2)
	chain.Add("tx1")
	chain.Blocks[1].Data = "tampered"
	// 再マイニングすれば単体のハッシュは有効になるが、後続の PrevHash は依然食い違う。
	remine(&chain.Blocks[1], chain.difficulty)
	if computeHash(chain.Blocks[1])[:2] != "00" {
		t.Error("再マイニングで難易度を満たすべき")
	}
	// genesis しか後続がないので、この例では2ブロック目まで直せば通る。
	// より長いチェーンでは全後続を直す必要がある = コストが積み上がる。
}

func TestNewValidation(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("難易度0以下は panic すべき")
		}
	}()
	New(0)
}
