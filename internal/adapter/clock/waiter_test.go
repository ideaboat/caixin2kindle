package clock

import (
	"context"
	"errors"
	"testing"
	"time"

	"caixin2kindle/internal/config"
)

func TestNewWaiter_UsesConfigDelays(t *testing.T) {
	cfg := config.Default()
	cfg.DelayMin = 2 * time.Second
	cfg.DelayMax = 5 * time.Second
	cfg.ClickDelayMin = 100 * time.Millisecond
	cfg.ClickDelayMax = 200 * time.Millisecond

	waiter := NewWaiter(cfg)

	if waiter.ArticleMin != cfg.DelayMin || waiter.ArticleMax != cfg.DelayMax {
		t.Fatalf("篇间区间 = [%v,%v], want [%v,%v]", waiter.ArticleMin, waiter.ArticleMax, cfg.DelayMin, cfg.DelayMax)
	}
	if waiter.ClickMin != cfg.ClickDelayMin || waiter.ClickMax != cfg.ClickDelayMax {
		t.Fatalf("点击区间 = [%v,%v], want [%v,%v]", waiter.ClickMin, waiter.ClickMax, cfg.ClickDelayMin, cfg.ClickDelayMax)
	}
}

func TestRandomDuration(t *testing.T) {
	tests := []struct {
		name    string
		minimum time.Duration
		maximum time.Duration
		sample  float64
		want    time.Duration
	}{
		{name: "middle sample", minimum: time.Second, maximum: 3 * time.Second, sample: 0.5, want: 2 * time.Second},
		{name: "zero sample is minimum", minimum: time.Second, maximum: 3 * time.Second, sample: 0, want: time.Second},
		{name: "full sample is maximum", minimum: time.Second, maximum: 3 * time.Second, sample: 1, want: 3 * time.Second},
		{name: "equal bounds use minimum", minimum: 2 * time.Second, maximum: 2 * time.Second, sample: 0.5, want: 2 * time.Second},
		{name: "inverted bounds use minimum", minimum: 3 * time.Second, maximum: time.Second, sample: 0.9, want: 3 * time.Second},
		{name: "zero bounds", minimum: 0, maximum: 0, sample: 0.5, want: 0},
		{name: "negative result clamps to zero", minimum: -2 * time.Second, maximum: time.Second, sample: 0.5, want: 0},
		{name: "negative bounds clamp to zero", minimum: -3 * time.Second, maximum: -time.Second, sample: 0.5, want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := randomDuration(test.minimum, test.maximum, test.sample)

			if got != test.want {
				t.Fatalf("randomDuration(%v, %v, %v) = %v, want %v", test.minimum, test.maximum, test.sample, got, test.want)
			}
		})
	}
}

func TestWaiter_BetweenArticles_UsesInjectedRandAndSleep(t *testing.T) {
	var slept []time.Duration
	waiter := &Waiter{
		ArticleMin: time.Second,
		ArticleMax: 3 * time.Second,
		Rand:       func() float64 { return 0.5 },
		Sleep:      func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil },
	}

	err := waiter.BetweenArticles(context.Background())

	if err != nil {
		t.Fatalf("BetweenArticles() error = %v", err)
	}
	if len(slept) != 1 || slept[0] != 2*time.Second {
		t.Fatalf("Sleep 参数 = %v, want [2s]", slept)
	}
}

func TestWaiter_BetweenClicks_UsesInjectedRandAndSleep(t *testing.T) {
	var slept []time.Duration
	waiter := &Waiter{
		ClickMin: time.Second,
		ClickMax: 3 * time.Second,
		Rand:     func() float64 { return 0.25 },
		Sleep:    func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil },
	}

	err := waiter.BetweenClicks(context.Background())

	if err != nil {
		t.Fatalf("BetweenClicks() error = %v", err)
	}
	if len(slept) != 1 || slept[0] != 1500*time.Millisecond {
		t.Fatalf("Sleep 参数 = %v, want [1.5s]", slept)
	}
}

func TestWaiter_ZeroRange_StillCallsSleepWithZero(t *testing.T) {
	var slept []time.Duration
	waiter := &Waiter{
		ArticleMin: 0,
		ArticleMax: 0,
		Sleep:      func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil },
	}

	err := waiter.BetweenArticles(context.Background())

	if err != nil {
		t.Fatalf("BetweenArticles() error = %v", err)
	}
	if len(slept) != 1 || slept[0] != 0 {
		t.Fatalf("Sleep 参数 = %v, want [0]", slept)
	}
}

func TestWaiter_BetweenArticles_PropagatesSleepError(t *testing.T) {
	sentinel := errors.New("sleep failed")
	waiter := &Waiter{
		ArticleMin: time.Second,
		ArticleMax: time.Second,
		Sleep:      func(context.Context, time.Duration) error { return sentinel },
	}

	err := waiter.BetweenArticles(context.Background())

	if !errors.Is(err, sentinel) {
		t.Fatalf("BetweenArticles() error = %v, want %v", err, sentinel)
	}
}

func TestWaiter_DefaultSleepAndRand_ZeroRangeReturns(t *testing.T) {
	waiter := &Waiter{ClickMin: 0, ClickMax: 0}

	err := waiter.BetweenClicks(context.Background())

	if err != nil {
		t.Fatalf("BetweenClicks() error = %v", err)
	}
}

func TestWaiter_DefaultSleep_RespectsCancellation(t *testing.T) {
	waiter := &Waiter{ArticleMin: time.Hour, ArticleMax: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waiter.BetweenArticles(ctx)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("BetweenArticles() error = %v, want context.Canceled", err)
	}
}

func TestSleepContext_ZeroDurationReturnsNil(t *testing.T) {
	err := sleepContext(context.Background(), 0)

	if err != nil {
		t.Fatalf("sleepContext() error = %v, want nil", err)
	}
}

func TestSleepContext_CanceledContextReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := sleepContext(ctx, 0)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("sleepContext() error = %v, want context.Canceled", err)
	}
}
