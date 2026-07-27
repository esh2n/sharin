package gqa

import (
	"math"
	"testing"

	"github.com/esh2n/sharin/llm/tensor"
)

func input(seq, dModel int) *tensor.Tensor {
	x := tensor.New(seq, dModel)
	v := float32(0.13)
	for i := range x.Data {
		v = float32(math.Mod(float64(v)*7.13+0.37, 1.0))
		x.Data[i] = v*2 - 1
	}
	return x
}

func TestNewValidates(t *testing.T) {
	cases := []Config{
		{DModel: 7, NHeads: 2, NKVHeads: 2}, // DModel が NHeads で割れない
		{DModel: 8, NHeads: 3, NKVHeads: 2}, // NHeads が NKVHeads で割れない
		{DModel: 8, NHeads: 4, NKVHeads: 0}, // KV ヘッドなし
		{DModel: 8, NHeads: 4, NKVHeads: 8}, // KV ヘッドが Q ヘッドより多い
		{DModel: 0, NHeads: 1, NKVHeads: 1}, // 空
	}
	for _, c := range cases {
		if _, err := New(c); err == nil {
			t.Errorf("config %+v should be rejected", c)
		}
	}
	if _, err := New(Config{DModel: 8, NHeads: 4, NKVHeads: 2}); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func TestKVHeadMapping(t *testing.T) {
	// 4 Q ヘッド、2 KV ヘッド → Q ヘッド 0,1 が KV0、2,3 が KV1 を共有する。
	a, _ := New(Config{DModel: 8, NHeads: 4, NKVHeads: 2})
	want := []int{0, 0, 1, 1}
	for h, w := range want {
		if got := a.KVHeadFor(h); got != w {
			t.Errorf("KVHeadFor(%d) = %d, want %d", h, got, w)
		}
	}
	// MQA: 全ヘッドが KV0 を共有。
	m, _ := New(Config{DModel: 8, NHeads: 4, NKVHeads: 1})
	for h := 0; h < 4; h++ {
		if m.KVHeadFor(h) != 0 {
			t.Errorf("MQA: KVHeadFor(%d) should be 0", h)
		}
	}
}

func TestForwardShapeAndDeterminism(t *testing.T) {
	cfg := Config{DModel: 16, NHeads: 4, NKVHeads: 2}
	a, _ := New(cfg)
	b, _ := New(cfg)
	x := input(5, 16)
	oa := a.Forward(x)
	ob := b.Forward(x)
	if oa.Rows != 5 || oa.Cols != 16 {
		t.Fatalf("shape = (%d,%d), want (5,16)", oa.Rows, oa.Cols)
	}
	for i := range oa.Data {
		if oa.Data[i] != ob.Data[i] {
			t.Fatal("same config should give identical outputs")
		}
	}
}

func TestCausality(t *testing.T) {
	// 末尾トークンを書き換えても、過去の位置の出力は 1 ビットも動かない。
	a, _ := New(Config{DModel: 16, NHeads: 4, NKVHeads: 2})
	x1 := input(6, 16)
	x2 := input(6, 16)
	for c := 0; c < 16; c++ {
		x2.Set(5, c, x2.At(5, c)+1.5)
	}
	o1 := a.Forward(x1)
	o2 := a.Forward(x2)
	for r := 0; r < 5; r++ {
		for c := 0; c < 16; c++ {
			if o1.At(r, c) != o2.At(r, c) {
				t.Fatalf("row %d changed when future token changed", r)
			}
		}
	}
}

func TestGQAEqualsMHAWhenKVShared(t *testing.T) {
	// KV 重みを全 KV ヘッドで同一にすれば、KV ヘッド数を減らしても出力は変わらない。
	// 「GQA は K/V を共有しているだけで、attention の計算自体は同じ」ことの検証。
	mha, _ := New(Config{DModel: 16, NHeads: 4, NKVHeads: 4})
	gqa, _ := New(Config{DModel: 16, NHeads: 4, NKVHeads: 2})
	mqa, _ := New(Config{DModel: 16, NHeads: 4, NKVHeads: 1})
	for _, a := range []*Attention{mha, gqa, mqa} {
		a.SetUniformKV()
	}
	x := input(5, 16)
	om := mha.Forward(x)
	og := gqa.Forward(x)
	oq := mqa.Forward(x)
	for i := range om.Data {
		if math.Abs(float64(om.Data[i]-og.Data[i])) > 1e-6 || math.Abs(float64(om.Data[i]-oq.Data[i])) > 1e-6 {
			t.Fatal("with identical KV weights, MHA/GQA/MQA must agree")
		}
	}
}

func TestKVCacheFloats(t *testing.T) {
	// KV キャッシュは 2(KとV) × 系列長 × KVヘッド数 × ヘッド次元。
	mha := Config{DModel: 4096, NHeads: 32, NKVHeads: 32}
	gqa := Config{DModel: 4096, NHeads: 32, NKVHeads: 8}
	mqa := Config{DModel: 4096, NHeads: 32, NKVHeads: 1}
	if got := mha.KVCacheFloats(1000); got != 2*1000*32*128 {
		t.Fatalf("mha cache = %d", got)
	}
	if mha.KVCacheFloats(1000)/gqa.KVCacheFloats(1000) != 4 {
		t.Fatal("GQA(32/8) should use 1/4 the cache of MHA")
	}
	if mha.KVCacheFloats(1000)/mqa.KVCacheFloats(1000) != 32 {
		t.Fatal("MQA should use 1/32 the cache of MHA")
	}
}
