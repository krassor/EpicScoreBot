package gantt

import (
	"EpicScoreBot/internal/models/domain"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestRecalculateTeamSchedule_ConcurrentSameTeamSerializes проверяет, что per-team
// мьютекс (design.md Decision 1-2) реально сериализует конкурентные вызовы
// RecalculateTeamSchedule для ОДНОЙ команды: пока первый вызов ещё выполняется
// (искусственно задержан внутри GetTeamEpicsOrdered — первого repo-вызова функции),
// второй не должен успеть начать выполняться параллельно. Без блокировки внутри
// RecalculateTeamSchedule обе горутины одновременно читали/писали общие map
// fakeRepo — maxActiveRecalcs поднялся бы до 2 (а под -race тест также падал бы
// с "concurrent map read and map write").
func TestRecalculateTeamSchedule_ConcurrentSameTeamSerializes(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamID := uuid.New()
	epicID := uuid.New()
	roleID := uuid.New()

	f.addRole(&domain.Role{ID: roleID, Name: "Аналитик"})
	f.addEpic(&domain.Epic{ID: epicID, Number: "E-1", Name: "Epic", TeamID: teamID})
	f.roleScores[epicID] = []domain.EpicRoleScore{{EpicID: epicID, RoleID: roleID, WeightedAvg: 2.0}}

	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC) // Monday
	if _, err := svc.GenerateTasksForEpic(ctx, epicID, start); err != nil {
		t.Fatalf("setup: unexpected error: %v", err)
	}

	// Сбрасываем счётчики после setup (GenerateTasksForEpic сам один раз
	// вызывает RecalculateTeamSchedule) и включаем искусственную задержку
	// внутри первого repo-вызова, чтобы гарантированно создать окно
	// конкуренции для второй горутины.
	f.concurrencyMu.Lock()
	f.activeRecalcs = 0
	f.maxActiveRecalcs = 0
	f.recalcDelay = 50 * time.Millisecond
	f.recalcDelayTeamID = teamID
	f.concurrencyMu.Unlock()

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := svc.RecalculateTeamSchedule(ctx, teamID)
			errs[idx] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: unexpected error: %v", i, err)
		}
	}

	f.concurrencyMu.Lock()
	maxActive := f.maxActiveRecalcs
	f.concurrencyMu.Unlock()

	if maxActive > 1 {
		t.Errorf("maxActiveRecalcs = %d, want <= 1 — конкурентные пересчёты одной команды не сериализованы", maxActive)
	}
}

// TestRecalculateTeamSchedule_ConcurrentDifferentTeamsDoNotBlock проверяет, что
// per-team мьютекс не задерживает пересчёты РАЗНЫХ команд друг относительно
// друга: пока пересчёт команды A искусственно задержан, пересчёт команды B
// должен успеть завершиться, не дожидаясь A.
func TestRecalculateTeamSchedule_ConcurrentDifferentTeamsDoNotBlock(t *testing.T) {
	ctx := context.Background()
	f := newFakeRepo()
	svc := New(newTestLogger(), f)

	teamA := uuid.New()
	teamB := uuid.New()
	roleID := uuid.New()
	epicA := uuid.New()
	epicB := uuid.New()

	f.addRole(&domain.Role{ID: roleID, Name: "Аналитик"})
	f.addEpic(&domain.Epic{ID: epicA, Number: "E-A", Name: "Epic A", TeamID: teamA})
	f.addEpic(&domain.Epic{ID: epicB, Number: "E-B", Name: "Epic B", TeamID: teamB})
	f.roleScores[epicA] = []domain.EpicRoleScore{{EpicID: epicA, RoleID: roleID, WeightedAvg: 2.0}}
	f.roleScores[epicB] = []domain.EpicRoleScore{{EpicID: epicB, RoleID: roleID, WeightedAvg: 2.0}}

	start := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC) // Monday
	if _, err := svc.GenerateTasksForEpic(ctx, epicA, start); err != nil {
		t.Fatalf("setup: unexpected error: %v", err)
	}
	if _, err := svc.GenerateTasksForEpic(ctx, epicB, start); err != nil {
		t.Fatalf("setup: unexpected error: %v", err)
	}

	const delay = 200 * time.Millisecond
	f.concurrencyMu.Lock()
	f.recalcDelay = delay
	f.recalcDelayTeamID = teamA // задержка применяется только к команде A
	f.concurrencyMu.Unlock()

	var wg sync.WaitGroup
	var teamADone, teamBDone time.Time
	var errA, errB error

	wg.Add(2)
	go func() {
		defer wg.Done()
		_, errA = svc.RecalculateTeamSchedule(ctx, teamA)
		teamADone = time.Now()
	}()
	go func() {
		defer wg.Done()
		// Даём команде A гарантированно захватить свой мьютекс первой,
		// чтобы точно проверить именно независимость по teamID, а не
		// случайный порядок запуска горутин.
		time.Sleep(10 * time.Millisecond)
		_, errB = svc.RecalculateTeamSchedule(ctx, teamB)
		teamBDone = time.Now()
	}()
	wg.Wait()

	if errA != nil {
		t.Errorf("team A: unexpected error: %v", errA)
	}
	if errB != nil {
		t.Errorf("team B: unexpected error: %v", errB)
	}

	if !teamBDone.Before(teamADone) {
		t.Errorf("пересчёт команды B (%v) должен завершиться раньше задержанного пересчёта команды A (%v) — "+
			"похоже, разные команды блокируют друг друга", teamBDone, teamADone)
	}
}
