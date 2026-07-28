package config

import "testing"

func setup() *Cluster {
	c := New()
	c.Store.Put("app-config", ConfigMap, map[string]string{"log_level": "info", "timeout": "30"})
	return c
}

// 起動した Pod は、どちらの受け取り方でも同じ値を読む。
func TestBothSourcesReadTheSameAtStart(t *testing.T) {
	c := setup()
	env := c.Start("env-pod", Ref{Entry: "app-config", Source: EnvVar})
	file := c.Start("file-pod", Ref{Entry: "app-config", Source: FileMount})

	for _, p := range []*Pod{env, file} {
		if got := p.Read("app-config", "log_level"); got != "info" {
			t.Fatalf("%s が %q を読んだ", p.Name, got)
		}
	}
}

// 置き場を書き換えると、ファイルで受け取っている側だけが新しい値を見る。
// 環境変数は起動時に写し取られているので、変わらない。
func TestUpdateReachesFilesOnly(t *testing.T) {
	c := setup()
	env := c.Start("env-pod", Ref{Entry: "app-config", Source: EnvVar})
	file := c.Start("file-pod", Ref{Entry: "app-config", Source: FileMount})

	c.Store.Put("app-config", ConfigMap, map[string]string{"log_level": "debug"})

	if got := file.Read("app-config", "log_level"); got != "debug" {
		t.Fatalf("ファイルなら新しい値のはずが %q", got)
	}
	if got := env.Read("app-config", "log_level"); got != "info" {
		t.Fatalf("環境変数なら起動時のままのはずが %q", got)
	}
}

// 古い値を持ったままであることは検出できる。
func TestStaleDetection(t *testing.T) {
	c := setup()
	env := c.Start("env-pod", Ref{Entry: "app-config", Source: EnvVar})
	file := c.Start("file-pod", Ref{Entry: "app-config", Source: FileMount})

	if env.Stale() || file.Stale() {
		t.Fatal("更新前は古くないはず")
	}
	c.Store.Put("app-config", ConfigMap, map[string]string{"log_level": "debug"})

	if !env.Stale() {
		t.Fatal("環境変数で受け取っている側は古くなるはず")
	}
	if file.Stale() {
		t.Fatal("ファイルで受け取っている側は古くならないはず")
	}
}

// 反映するには作り直すしかない。これが設定の外出しの一部になっている。
func TestRestartPicksUpNewValue(t *testing.T) {
	c := setup()
	c.Start("env-pod", Ref{Entry: "app-config", Source: EnvVar})
	c.Store.Put("app-config", ConfigMap, map[string]string{"log_level": "debug"})

	fresh := c.Restart("env-pod")
	if got := fresh.Read("app-config", "log_level"); got != "debug" {
		t.Fatalf("作り直せば新しい値のはずが %q", got)
	}
	if fresh.Stale() {
		t.Fatal("作り直した後は古くないはず")
	}
}

// 書き換えは差分。触れていない鍵は残る。
func TestPutMergesKeys(t *testing.T) {
	c := setup()
	c.Store.Put("app-config", ConfigMap, map[string]string{"log_level": "debug"})
	e := c.Store.Get("app-config")
	if e.Get("timeout") != "30" {
		t.Fatalf("触れていない鍵は残るはずが %q", e.Get("timeout"))
	}
	if e.Version != 2 {
		t.Fatalf("版が上がるはずが %d", e.Version)
	}
}

// Secret も仕組みは同じ。違うのは種類の表示だけで、暗号ではない。
func TestSecretBehavesLikeConfigMap(t *testing.T) {
	c := New()
	c.Store.Put("db-secret", Secret, map[string]string{"password": "hunter2"})
	p := c.Start("app", Ref{Entry: "db-secret", Source: FileMount})

	if got := p.Read("db-secret", "password"); got != "hunter2" {
		t.Fatalf("そのまま読めるはずが %q", got)
	}
	if c.Store.Get("db-secret").Kind.String() != "Secret" {
		t.Fatal("種類が違う")
	}
}

// 同じ Pod が両方の受け取り方を混ぜても、それぞれの性質のまま動く。
func TestMixedSourcesInOnePod(t *testing.T) {
	c := setup()
	c.Store.Put("db-secret", Secret, map[string]string{"password": "old"})
	p := c.Start("app",
		Ref{Entry: "app-config", Source: EnvVar},
		Ref{Entry: "db-secret", Source: FileMount})

	c.Store.Put("app-config", ConfigMap, map[string]string{"log_level": "debug"})
	c.Store.Put("db-secret", Secret, map[string]string{"password": "new"})

	if got := p.Read("app-config", "log_level"); got != "info" {
		t.Fatalf("環境変数側は変わらないはずが %q", got)
	}
	if got := p.Read("db-secret", "password"); got != "new" {
		t.Fatalf("ファイル側は変わるはずが %q", got)
	}
	if !p.Stale() {
		t.Fatal("環境変数側が古いので古い扱いのはず")
	}
}

// 参照していない設定や、存在しない鍵は空を返す。
func TestUnknownReadsAreEmpty(t *testing.T) {
	c := setup()
	p := c.Start("app", Ref{Entry: "app-config", Source: FileMount})
	if p.Read("nosuch", "k") != "" {
		t.Fatal("参照していない設定は空のはず")
	}
	if p.Read("app-config", "nosuch") != "" {
		t.Fatal("存在しない鍵は空のはず")
	}
	if c.Restart("nosuch") != nil {
		t.Fatal("存在しない Pod は作り直せないはず")
	}
}

// 存在しない設定を参照して起動しても壊れない。
func TestStartWithMissingEntry(t *testing.T) {
	c := New()
	p := c.Start("app", Ref{Entry: "nosuch", Source: EnvVar})
	if p.Stale() {
		t.Fatal("参照先が無いなら古くもならないはず")
	}
	if len(p.Sources()) != 1 {
		t.Fatal("参照は記録されるはず")
	}
}

func TestStrings(t *testing.T) {
	if ConfigMap.String() != "ConfigMap" || Secret.String() != "Secret" {
		t.Fatal("Kind の文字列が違う")
	}
	if EnvVar.String() != "環境変数" || FileMount.String() != "ファイル" {
		t.Fatal("Source の文字列が違う")
	}
	if itoa(0) != "0" || itoa(7) != "7" {
		t.Fatal("itoa が違う")
	}
}
