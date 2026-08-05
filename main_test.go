package main

import (
	"strings"
	"testing"
	"time"
)

func TestParseUsageJSON(t *testing.T) {
	raw := []byte(`{
		"login": "Antupis",
		"copilot_plan": "business",
		"quota_reset_date_utc": "2026-09-01T00:00:00.000Z",
		"quota_snapshots": {
			"premium_interactions": {
				"has_quota": true,
				"unlimited": false,
				"percent_remaining": 91.9,
				"credits_used": 4859,
				"remaining": 55140,
				"entitlement": 60000
			}
		}
	}`)

	fetchedAt := time.Date(2026, time.August, 5, 5, 54, 14, 0, time.UTC)
	data, err := parseUsageJSON(raw, fetchedAt)
	if err != nil {
		t.Fatalf("parseUsageJSON() error = %v", err)
	}
	if data.Login != "Antupis" || data.Plan != "business" {
		t.Fatalf("identity = %q/%q, want Antupis/business", data.Login, data.Plan)
	}
	if !data.ResetAt.Equal(time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("ResetAt = %s", data.ResetAt)
	}
	if !data.Quota.HasQuota || data.Quota.Unlimited {
		t.Fatalf("quota flags = %+v", data.Quota)
	}
	if data.Quota.CreditsUsed != 4859 || data.Quota.Entitlement != 60000 {
		t.Fatalf("quota values = %+v", data.Quota)
	}
	if !data.FetchedAt.Equal(fetchedAt) {
		t.Fatalf("FetchedAt = %s, want %s", data.FetchedAt, fetchedAt)
	}
}

func TestParseUsageJSONFallbacks(t *testing.T) {
	raw := []byte(`{
		"quota_reset_date": "2026-09-01",
		"quota_snapshots": {
			"premium_interactions": {
				"unlimited": true
			}
		}
	}`)

	data, err := parseUsageJSON(raw, time.Time{})
	if err != nil {
		t.Fatalf("parseUsageJSON() error = %v", err)
	}
	if !data.ResetAt.Equal(time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("ResetAt = %s", data.ResetAt)
	}
	if !data.Quota.HasQuota || !data.Quota.Unlimited {
		t.Fatalf("quota = %+v", data.Quota)
	}
}

func TestParseUsageJSONInvalid(t *testing.T) {
	if _, err := parseUsageJSON([]byte(`{"not":`), time.Time{}); err == nil {
		t.Fatal("parseUsageJSON() error = nil, want error")
	}
}

func TestMonthProgress(t *testing.T) {
	reset := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		now      time.Time
		want     float64
		fallback bool
	}{
		{
			name: "start",
			now:  time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC),
			want: 0,
		},
		{
			name: "middle",
			now:  time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC),
			want: 0.5,
		},
		{
			name: "after reset",
			now:  time.Date(2026, time.September, 2, 0, 0, 0, 0, time.UTC),
			want: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _, _, fallback := monthProgress(test.now, reset)
			if got != test.want {
				t.Fatalf("monthProgress() = %v, want %v", got, test.want)
			}
			if fallback != test.fallback {
				t.Fatalf("fallback = %v, want %v", fallback, test.fallback)
			}
		})
	}
}

func TestMonthProgressFallback(t *testing.T) {
	now := time.Date(2026, time.August, 5, 12, 0, 0, 0, time.UTC)
	progress, start, end, fallback := monthProgress(now, time.Time{})
	if progress <= 0 || progress >= 1 {
		t.Fatalf("progress = %v, want between 0 and 1", progress)
	}
	if !fallback {
		t.Fatal("fallback = false, want true")
	}
	if start.Day() != 1 || end.Day() != 1 || start.Month() != time.August || end.Month() != time.September {
		t.Fatalf("period = %s to %s", start, end)
	}
}

func TestMonthPeriodHandlesCalendarMonthLengths(t *testing.T) {
	tests := []struct {
		name      string
		resetAt   time.Time
		wantStart time.Time
		wantDays  int
	}{
		{
			name:      "31 day month",
			resetAt:   time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC),
			wantStart: time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC),
			wantDays:  31,
		},
		{
			name:      "30 day month",
			resetAt:   time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC),
			wantStart: time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
			wantDays:  30,
		},
		{
			name:      "leap February",
			resetAt:   time.Date(2028, time.March, 1, 0, 0, 0, 0, time.UTC),
			wantStart: time.Date(2028, time.February, 1, 0, 0, 0, 0, time.UTC),
			wantDays:  29,
		},
		{
			name:      "28 day February",
			resetAt:   time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
			wantStart: time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC),
			wantDays:  28,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			start, end, fallback := monthPeriod(test.resetAt, test.resetAt)
			if fallback {
				t.Fatal("monthPeriod() unexpectedly used fallback")
			}
			if !start.Equal(test.wantStart) || !end.Equal(test.resetAt) {
				t.Fatalf("period = %s to %s, want %s to %s", start, end, test.wantStart, test.resetAt)
			}
			if days := int(end.Sub(start).Hours() / 24); days != test.wantDays {
				t.Fatalf("period length = %d days, want %d", days, test.wantDays)
			}
		})
	}
}

func TestProgressBar(t *testing.T) {
	if got, want := progressBar(0, 10, true), "[----------]"; got != want {
		t.Errorf("empty bar = %q, want %q", got, want)
	}
	if got, want := progressBar(0.5, 10, true), "[#####-----]"; got != want {
		t.Errorf("half bar = %q, want %q", got, want)
	}
	if got, want := progressBar(2, 10, true), "[##########]"; got != want {
		t.Errorf("full bar = %q, want %q", got, want)
	}
	if got := progressBar(0.25, 4, false); !strings.Contains(got, "\u2588") || !strings.Contains(got, "\u2591") {
		t.Errorf("Unicode bar = %q, want block characters", got)
	}
}

func TestUsageProgress(t *testing.T) {
	got, available := usageProgress(quota{CreditsUsed: 4859, Entitlement: 60000})
	if !available || got < 0.0809 || got > 0.0810 {
		t.Fatalf("usageProgress() = %v, %v", got, available)
	}

	got, available = usageProgress(quota{PercentRemaining: 75, HasPercent: true})
	if !available || got != 0.25 {
		t.Fatalf("percent fallback = %v, %v", got, available)
	}

	if _, available = usageProgress(quota{HasQuota: true}); available {
		t.Fatal("quota without usage fields reported as available")
	}

	if _, available = usageProgress(quota{Unlimited: true}); available {
		t.Fatal("unlimited quota reported as available usage")
	}
}

func TestFormatNumber(t *testing.T) {
	for _, test := range []struct {
		value float64
		want  string
	}{
		{value: 0, want: "0"},
		{value: 4859, want: "4,859"},
		{value: 60000, want: "60,000"},
		{value: 55140.8, want: "55,140.8"},
	} {
		if got := formatNumber(test.value); got != test.want {
			t.Errorf("formatNumber(%v) = %q, want %q", test.value, got, test.want)
		}
	}
}

func TestParseConfig(t *testing.T) {
	cfg, err := parseConfig([]string{"-interval", "10m", "-width", "30", "-ascii"}, nil)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.Interval != 10*time.Minute || cfg.BarWidth != 30 || !cfg.ASCII {
		t.Fatalf("config = %+v", cfg)
	}

	if _, err := parseConfig([]string{"-interval", "30s"}, nil); err == nil {
		t.Fatal("short interval accepted in dashboard mode")
	}
	if _, err := parseConfig([]string{"-interval", "30s", "-once"}, nil); err != nil {
		t.Fatalf("short interval rejected in one-shot mode: %v", err)
	}
}

func TestDashboardText(t *testing.T) {
	data := usageData{
		Login:     "Antupis",
		Plan:      "business",
		ResetAt:   time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC),
		FetchedAt: time.Date(2026, time.August, 5, 5, 54, 14, 0, time.UTC),
		Quota: quota{
			HasQuota:    true,
			CreditsUsed: 4859,
			Entitlement: 60000,
		},
	}
	state := appState{Data: &data}
	text := dashboardText(state, time.Date(2026, time.August, 5, 12, 0, 0, 0, time.UTC), config{BarWidth: 10, ASCII: true, Once: true})
	for _, want := range []string{
		"GitHub Copilot Usage",
		"Account: Antupis | Plan: Business",
		"Month elapsed",
		"Premium usage",
		"4,859 / 60,000 premium credits",
		"One-shot mode",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("dashboard does not contain %q:\n%s", want, text)
		}
	}
}
