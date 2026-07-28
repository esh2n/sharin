// Package podsecurity は SecurityContext と Pod Security Standards を最小構成で実装する。
//
// [RBAC](rbac)は「誰が操作してよいか」を決め、[admission webhook](admission)は
// 「どんな内容なら受け入れるか」を決めた。残っているのが「受け入れた Pod が
// ノードの上でどこまでできるか」になる。
//
// コンテナは[隔離](container)されていると言うが、その隔離はいくらでも緩められる。
// ホストのネットワークをそのまま使う、ホストのディレクトリを覗く、特権つきで
// 動かす。どれも設定ひとつで外せる。外せることには理由があって、監視や収集の
// ように本当にホストを見なければならないものがあるからだ。
//
// 問題は既定にある。SecurityContext は書けば効くが、書かなければ何も制限しない。
// [ResourceQuota](quota)の章で見た「既定値が無いと書き忘れが 0 として通る」のと
// 同じ形で、書き忘れた Pod がいちばん緩い設定で動く。
//
// だから外側から一律に決める層が要る。それが Pod Security Standards で、
// 名前空間にラベルを1つ貼ると、そこに来る Pod がまとめて検査される。
//
// そして検査の結果の扱いが3つに分かれているのが、この仕組みの実用的なところに
// なる。拒否する、記録する、警告する。判定は同じで、扱いだけが違う。
// [ヘルスチェック](probe)の章と同じ構造で、そのおかげで既存を壊さずに締められる。
package podsecurity

import "sort"

// #region level

// Level は許す範囲の段階。上の段階は下の段階を含む。
type Level int

const (
	// Privileged は何も制限しない。既定はこれになる。
	Privileged Level = iota
	// Baseline は既知の権限昇格を塞ぐ。だいたいのアプリはそのまま通る。
	Baseline
	// Restricted は現在の強化指針まで守らせる。書き足しが要る。
	Restricted
)

func (l Level) String() string {
	return [...]string{"privileged", "baseline", "restricted"}[l]
}

// Violation は1件の違反。どの規則に、どのコンテナが、どう反したか。
type Violation struct {
	Rule      string
	Container string // Pod 全体の設定なら空
	Detail    string
	// Level は、この規則がどの段階から効くかを表す。
	Level Level
}

// #endregion level

// #region pod

// Capabilities は外す権限と足す権限。
type Capabilities struct {
	Drop []string
	Add  []string
}

// SecurityContext はコンテナに与える権限の設定。
//
// ポインタになっている項目は「書かなかった」と「false と書いた」を区別する
// 必要があるものになる。書かなかったことが違反になる規則があるので、
// 区別できないと検査ができない。
type SecurityContext struct {
	Privileged               bool
	AllowPrivilegeEscalation *bool
	RunAsNonRoot             *bool
	RunAsUser                *int64
	ReadOnlyRootFilesystem   bool
	Capabilities             Capabilities
	// SeccompProfile は "RuntimeDefault" / "Localhost" / "Unconfined" / 空(未指定)。
	SeccompProfile string
}

// Container は1つのコンテナ。
type Container struct {
	Name      string
	Ports     []int // ホスト側に開くポート
	Security  SecurityContext
	HostPaths []string // このコンテナが使うホストのパス
}

// Pod は検査する対象。
type Pod struct {
	Name      string
	Namespace string
	// ホストの名前空間をそのまま使うかどうか。
	HostNetwork bool
	HostPID     bool
	HostIPC     bool
	Containers  []Container
}

// Bool と Int64 は、書いたことを表すための補助。
func Bool(b bool) *bool    { return &b }
func Int64(i int64) *int64 { return &i }
func deref(b *bool) bool   { return b != nil && *b }
func written(b *bool) bool { return b != nil }
func has(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// #endregion pod

// #region check

// baselineCaps は Baseline で足してよい権限。これ以外を足すと違反になる。
var baselineCaps = []string{"NET_BIND_SERVICE"}

// Check は Pod を段階 level に照らして、違反を返す。
//
// 段階が積み重なっているので、Restricted の検査は Baseline の検査を含む。
// 上の段階だけを別に書くと、下の規則が抜けても気づけない。
func Check(p Pod, level Level) []Violation {
	if level == Privileged {
		return nil
	}
	var vs []Violation
	vs = append(vs, checkBaseline(p)...)
	if level == Restricted {
		vs = append(vs, checkRestricted(p)...)
	}
	sort.SliceStable(vs, func(i, j int) bool {
		if vs[i].Container != vs[j].Container {
			return vs[i].Container < vs[j].Container
		}
		return vs[i].Rule < vs[j].Rule
	})
	return vs
}

// checkBaseline は、既知の権限昇格を塞ぐ規則を当てる。
//
// どれも「ホストとの隔離を外していないか」を見ている。隔離を外せば、
// コンテナの中から外に手が届く。
func checkBaseline(p Pod) []Violation {
	var vs []Violation
	if p.HostNetwork {
		vs = append(vs, Violation{Rule: "hostNetwork", Level: Baseline,
			Detail: "ホストのネットワークをそのまま使っている。同居する他の Pod の通信が見える"})
	}
	if p.HostPID {
		vs = append(vs, Violation{Rule: "hostPID", Level: Baseline,
			Detail: "ホストのプロセスが見える。他の Pod のプロセスに手が届く"})
	}
	if p.HostIPC {
		vs = append(vs, Violation{Rule: "hostIPC", Level: Baseline,
			Detail: "ホストのプロセス間通信を共有している"})
	}
	for _, c := range p.Containers {
		if c.Security.Privileged {
			vs = append(vs, Violation{Rule: "privileged", Container: c.Name, Level: Baseline,
				Detail: "特権つき。ホストの root とほぼ変わらない"})
		}
		for _, path := range c.HostPaths {
			vs = append(vs, Violation{Rule: "hostPath", Container: c.Name, Level: Baseline,
				Detail: "ホストの " + path + " をそのまま見ている"})
		}
		for _, port := range c.Ports {
			vs = append(vs, Violation{Rule: "hostPort", Container: c.Name, Level: Baseline,
				Detail: "ホストのポート " + itoa(port) + " を占有している"})
		}
		for _, cap := range c.Security.Capabilities.Add {
			if !has(baselineCaps, cap) {
				vs = append(vs, Violation{Rule: "capabilities.add", Container: c.Name, Level: Baseline,
					Detail: cap + " を足している。baseline が許すのは " + baselineCaps[0] + " だけ"})
			}
		}
		if c.Security.SeccompProfile == "Unconfined" {
			vs = append(vs, Violation{Rule: "seccompProfile", Container: c.Name, Level: Baseline,
				Detail: "システムコールの制限を明示的に外している"})
		}
	}
	return vs
}

// checkRestricted は、書き足しを求める規則を当てる。
//
// Baseline との違いは、危ないことをしていないかではなく、安全側を明示したかを
// 見ている点になる。書かなかったことが違反になるので、既存の Pod はたいてい落ちる。
func checkRestricted(p Pod) []Violation {
	var vs []Violation
	for _, c := range p.Containers {
		s := c.Security
		if !written(s.AllowPrivilegeEscalation) || deref(s.AllowPrivilegeEscalation) {
			vs = append(vs, Violation{Rule: "allowPrivilegeEscalation", Container: c.Name, Level: Restricted,
				Detail: "false と明示していない。書かなければ昇格できてしまう"})
		}
		if !written(s.RunAsNonRoot) || !deref(s.RunAsNonRoot) {
			vs = append(vs, Violation{Rule: "runAsNonRoot", Container: c.Name, Level: Restricted,
				Detail: "true と明示していない。既定では root で動く"})
		}
		if s.RunAsUser != nil && *s.RunAsUser == 0 {
			vs = append(vs, Violation{Rule: "runAsUser", Container: c.Name, Level: Restricted,
				Detail: "0(root)を指定している"})
		}
		if !has(s.Capabilities.Drop, "ALL") {
			vs = append(vs, Violation{Rule: "capabilities.drop", Container: c.Name, Level: Restricted,
				Detail: "ALL を外していない。要るものだけ足し直す形にする"})
		}
		if s.SeccompProfile != "RuntimeDefault" && s.SeccompProfile != "Localhost" {
			vs = append(vs, Violation{Rule: "seccompProfile", Container: c.Name, Level: Restricted,
				Detail: "RuntimeDefault を明示していない"})
		}
	}
	return vs
}

// #endregion check

// #region policy

// Policy は名前空間に貼るラベル。同じ検査を、3つの扱いで使い分ける。
//
// 拒否だけしか無ければ、締めるという操作が常に危険になる。今動いているものが
// 落ちるかどうかを、落とさずに知る手段が要る。
type Policy struct {
	// Enforce は、この段階に反する Pod を拒む。
	Enforce Level
	// Audit は、この段階に反する Pod を記録に残す(通しはする)。
	Audit Level
	// Warn は、この段階に反する Pod を作った人に警告する(通しはする)。
	Warn Level
}

// Decision は1つの Pod に対する判定。
type Decision struct {
	Admitted bool
	Denied   []Violation
	Audited  []Violation
	Warned   []Violation
}

// Admit は Pod を判定する。
//
// 検査そのものは同じ関数を3回呼ぶだけで、段階だけが違う。判定は1つ、
// 扱いが3つという形になっている。
func (p Policy) Admit(pod Pod) Decision {
	d := Decision{
		Denied:  Check(pod, p.Enforce),
		Audited: Check(pod, p.Audit),
		Warned:  Check(pod, p.Warn),
	}
	d.Admitted = len(d.Denied) == 0
	return d
}

// Tighten は、今ある Pod をすべて通したまま拒否の段階を上げられるかを返す。
//
// 段階的に締めるとき、いちばん知りたいのはこれになる。上げてよいか、
// 上げたら何が落ちるか。
func Tighten(pods []Pod, to Level) (ok bool, breaks []string) {
	for _, pod := range pods {
		if len(Check(pod, to)) > 0 {
			breaks = append(breaks, pod.Name)
		}
	}
	sort.Strings(breaks)
	return len(breaks) == 0, breaks
}

// #endregion policy

// itoa は小さな非負整数を文字列にする(strconv を避ける)。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
