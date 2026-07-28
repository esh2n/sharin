// Package job は Kubernetes の Job と CronJob を最小構成で実装する。
//
// この編で扱ってきたワークロードは、どれも動き続けるものだった。Pod は
// 起動して、リクエストを受けて、止められるまで生きている。ヘルスチェックが
// 見ていたのも「まだ受けられるか」で、終わることは想定していなかった。
//
// だが終わる仕事もある。バッチ集計、データの移行、バックアップ。これらは
// 走って、終わって、消える。終わることが正常な結末なので、扱いが逆になる。
// 動き続けるものは「止まったら異常」だが、こちらは「終わったら正常」で、
// 「終わらないほうが異常」になる。
//
// 数え方も変わる。ready な数でなく、成功した数を数える。そして失敗という
// 結末が加わるので、何回まで試すかを決めておく必要が出てくる。決めておかないと、
// 直らない失敗を永久に再試行し続ける。
//
// 周期実行(CronJob)には、さらに固有の問題がある。前の実行がまだ終わって
// いないのに次の時刻が来たら、どうするか。重ねるか、飛ばすか、置き換えるか。
// どれを選んでも何かを失うので、仕事の性質で決めることになる。
package job

// #region job

// Phase は Job の結末。
type Phase int

const (
	Running  Phase = iota // まだ走っている
	Complete              // 必要な数だけ成功した
	Failed                // 再試行の上限を超えた
)

func (p Phase) String() string {
	switch p {
	case Running:
		return "Running"
	case Complete:
		return "Complete"
	case Failed:
		return "Failed"
	}
	return "Unknown"
}

// Config は Job の設定。3つの数が別々の軸を決める。
type Config struct {
	// Completions は何個成功すれば終わりか。仕事の量。
	Completions int
	// Parallelism は同時に何個走らせるか。速さ。
	Parallelism int
	// BackoffLimit は失敗を何回まで許すか。これを超えると Job ごと失敗にする。
	BackoffLimit int
	// FailFirst は最初の何回の試行が失敗するか(台本。乱数を避けるため)。
	FailFirst int
}

// Job は「必要な数だけ成功したら終わる」ワークロード。
type Job struct {
	cfg    Config
	active int // 今走っている数

	Succeeded int
	Failed    int
	Attempts  int
	Phase     Phase
	Log       []string
}

// New は設定 cfg の Job を作る。
func New(cfg Config) *Job {
	if cfg.Parallelism < 1 {
		cfg.Parallelism = 1
	}
	if cfg.Completions < 1 {
		cfg.Completions = 1
	}
	return &Job{cfg: cfg}
}

// Active は今走っている数を返す。
func (j *Job) Active() int { return j.active }

// Done は結末が決まったかを返す。
func (j *Job) Done() bool { return j.Phase != Running }

// Step は1周期進める。走っているものに結末をつけ、足りなければ起動する。
//
// 数えるのは ready な数ではなく、成功した数になる。動き続けるものとは
// 数え方が逆で、目標に達したら終わりであって、目標を保ち続けるのではない。
func (j *Job) Step() {
	if j.Done() {
		return
	}

	// ① 走っているものに結末がつく。台本の回数までは失敗する。
	for ; j.active > 0; j.active-- {
		j.Attempts++
		if j.Attempts <= j.cfg.FailFirst {
			j.Failed++
			j.logf("試行 " + itoa(j.Attempts) + " が失敗(通算 " + itoa(j.Failed) + " 回目)")
			continue
		}
		j.Succeeded++
		j.logf("試行 " + itoa(j.Attempts) + " が成功(通算 " + itoa(j.Succeeded) + " 個)")
	}

	// ② 失敗が上限を超えたら、Job ごと失敗にする。
	// 上限を決めておかないと、直らない失敗を永久に再試行し続ける。
	if j.Failed > j.cfg.BackoffLimit {
		j.Phase = Failed
		j.logf("失敗が上限 " + itoa(j.cfg.BackoffLimit) + " を超えた。Job を失敗として終える")
		return
	}

	// ③ 必要な数だけ成功したら終わり。
	if j.Succeeded >= j.cfg.Completions {
		j.Phase = Complete
		j.logf("必要な " + itoa(j.cfg.Completions) + " 個が成功した")
		return
	}

	// ④ 残りを、同時実行の上限まで起動する。
	remaining := j.cfg.Completions - j.Succeeded
	n := j.cfg.Parallelism
	if remaining < n {
		n = remaining // 要る数より多くは走らせない
	}
	j.active = n
}

// Run は結末がつくまで最大 max 周期まわす。
func (j *Job) Run(max int) {
	for i := 0; i < max && !j.Done(); i++ {
		j.Step()
	}
}

// #endregion job

// #region cron

// Policy は前の実行がまだ終わっていないときの振る舞い。
type Policy int

const (
	// Allow は重ねて走らせる。処理が重複しても構わない仕事向け。
	Allow Policy = iota
	// Forbid はこの回を飛ばす。重複が許されない仕事向け。
	Forbid
	// Replace は前の実行を止めて置き換える。最新だけが要る仕事向け。
	Replace
)

func (p Policy) String() string {
	switch p {
	case Allow:
		return "Allow"
	case Forbid:
		return "Forbid"
	case Replace:
		return "Replace"
	}
	return "Unknown"
}

// CronConfig は周期実行の設定。
type CronConfig struct {
	// Every は何周期ごとに起動するか。
	Every int
	// Policy は前の実行が残っているときの扱い。
	Policy Policy
	// Job は起動する Job の設定。
	Job Config
}

// CronJob は決まった周期で Job を起動する。
type CronJob struct {
	cfg  CronConfig
	now  int
	runs []*Job

	Started  int // 起動した数
	Skipped  int // 飛ばした数
	Replaced int // 置き換えた数
	Log      []string
}

// NewCron は設定 cfg の周期実行を作る。
func NewCron(cfg CronConfig) *CronJob {
	if cfg.Every < 1 {
		cfg.Every = 1
	}
	return &CronJob{cfg: cfg}
}

// Runs はこれまでに起動した Job を起動順に返す。
func (c *CronJob) Runs() []*Job { return c.runs }

// Active はまだ終わっていない Job の数を返す。
func (c *CronJob) Active() int {
	n := 0
	for _, j := range c.runs {
		if !j.Done() {
			n++
		}
	}
	return n
}

// Tick は時刻を1つ進める。走っている Job を進め、起動の時刻なら方針に従う。
func (c *CronJob) Tick() {
	c.now++
	for _, j := range c.runs {
		j.Step()
	}
	if c.now%c.cfg.Every != 0 {
		return
	}

	// 起動の時刻。前の実行が残っているかで振る舞いが変わる。
	if c.Active() > 0 {
		switch c.cfg.Policy {
		case Forbid:
			c.Skipped++
			c.logf("前の実行が終わっていないので、この回は飛ばす")
			return
		case Replace:
			for _, j := range c.runs {
				if !j.Done() {
					j.Phase = Failed
					j.logf("次の実行に置き換えられた")
				}
			}
			c.Replaced++
			c.logf("前の実行を止めて置き換える")
		}
	}
	j := New(c.cfg.Job)
	c.runs = append(c.runs, j)
	c.Started++
	c.logf("実行を起動(" + itoa(c.Started) + " 回目)")
}

// Completed は成功して終わった Job の数を返す。
func (c *CronJob) Completed() int {
	n := 0
	for _, j := range c.runs {
		if j.Phase == Complete {
			n++
		}
	}
	return n
}

// #endregion cron

func (j *Job) logf(msg string)     { j.Log = append(j.Log, msg) }
func (c *CronJob) logf(msg string) { c.Log = append(c.Log, "t="+itoa(c.now)+" "+msg) }

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
