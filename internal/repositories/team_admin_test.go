package repositories

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"EpicScoreBot/internal/migrator"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// newTestRepository поднимает Repository поверх тестовой PostgreSQL,
// накатывая миграции в изолированную схему для каждого теста. DSN берётся из
// переменной окружения TEST_DATABASE_DSN, либо (по умолчанию) из
// docker-compose.yml проекта (postgres/epicbot на localhost:5432) — этого
// достаточно для `docker compose up postgres` при локальной разработке и CI.
// Если БД недоступна, тест пропускается (t.Skip), а не падает — совместимо с
// `go test ./...` без поднятой БД.
func newTestRepository(t *testing.T) (*Repository, func()) {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		dsn = "host=localhost port=5432 user=epicbot password=EpicBot357@ dbname=epic-score-db sslmode=disable"
	}

	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Skipf("skipping: cannot connect to test database (%v); set TEST_DATABASE_DSN or run `docker compose up postgres`", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("skipping: cannot ping test database (%v)", err)
	}

	schema := fmt.Sprintf("team_admin_test_%s", uuid.New().String()[:8])
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	m := migrator.NewMigrator(db, log, schema)
	if err := m.Run(); err != nil {
		db.Close()
		t.Fatalf("failed to run migrations: %v", err)
	}
	if _, err := db.Exec(fmt.Sprintf("SET search_path TO %s, public", schema)); err != nil {
		db.Close()
		t.Fatalf("failed to set search_path: %v", err)
	}

	cleanup := func() {
		db.Exec(fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schema)) //nolint:errcheck
		db.Close()
	}

	return &Repository{DB: db, log: log, schema: schema}, cleanup
}

// seedTeamAndUsers создаёт команду и указанное число пользователей для теста.
func seedTeamAndUsers(t *testing.T, repo *Repository, n int) (uuid.UUID, []uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	team, err := repo.CreateTeam(ctx, "Team-"+uuid.New().String()[:8], "")
	if err != nil {
		t.Fatalf("failed to create team: %v", err)
	}

	userIDs := make([]uuid.UUID, n)
	for i := range n {
		u, err := repo.CreateUser(ctx, "First", "Last", "tg_"+uuid.New().String(), 100)
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		userIDs[i] = u.ID
	}
	return team.ID, userIDs
}

func TestTeamAdmin_AssignRemoveIsAdmin(t *testing.T) {
	repo, cleanup := newTestRepository(t)
	defer cleanup()
	ctx := context.Background()

	teamID, userIDs := seedTeamAndUsers(t, repo, 1)
	userID := userIDs[0]

	// Изначально не team-admin.
	isAdmin, err := repo.IsTeamAdmin(ctx, userID, teamID)
	if err != nil {
		t.Fatalf("IsTeamAdmin error: %v", err)
	}
	if isAdmin {
		t.Fatal("expected user not to be team-admin before assignment")
	}

	// Назначение с несуществующим assignedBy (uuid.Nil) должно сохраниться
	// как NULL, не нарушая FK (superadmin не обязан быть строкой в users).
	if err := repo.AssignTeamAdmin(ctx, userID, teamID, uuid.Nil); err != nil {
		t.Fatalf("AssignTeamAdmin error: %v", err)
	}

	isAdmin, err = repo.IsTeamAdmin(ctx, userID, teamID)
	if err != nil {
		t.Fatalf("IsTeamAdmin error: %v", err)
	}
	if !isAdmin {
		t.Error("expected user to be team-admin after assignment")
	}

	isAdminOfAny, err := repo.IsTeamAdminOfAny(ctx, userID)
	if err != nil {
		t.Fatalf("IsTeamAdminOfAny error: %v", err)
	}
	if !isAdminOfAny {
		t.Error("expected IsTeamAdminOfAny to be true")
	}

	// Идемпотентность повторного назначения.
	if err := repo.AssignTeamAdmin(ctx, userID, teamID, uuid.Nil); err != nil {
		t.Fatalf("AssignTeamAdmin (idempotent) error: %v", err)
	}

	// Снятие.
	if err := repo.RemoveTeamAdmin(ctx, userID, teamID); err != nil {
		t.Fatalf("RemoveTeamAdmin error: %v", err)
	}
	isAdmin, err = repo.IsTeamAdmin(ctx, userID, teamID)
	if err != nil {
		t.Fatalf("IsTeamAdmin error: %v", err)
	}
	if isAdmin {
		t.Error("expected user not to be team-admin after removal")
	}
}

func TestTeamAdmin_MultipleTeamsAndAdmins(t *testing.T) {
	repo, cleanup := newTestRepository(t)
	defer cleanup()
	ctx := context.Background()

	teamA, usersA := seedTeamAndUsers(t, repo, 2)
	teamB, _ := seedTeamAndUsers(t, repo, 0)
	userID := usersA[0]
	userID2 := usersA[1]

	// Один пользователь администрирует несколько команд.
	if err := repo.AssignTeamAdmin(ctx, userID, teamA, uuid.Nil); err != nil {
		t.Fatalf("assign teamA: %v", err)
	}
	if err := repo.AssignTeamAdmin(ctx, userID, teamB, uuid.Nil); err != nil {
		t.Fatalf("assign teamB: %v", err)
	}
	teamIDs, err := repo.GetTeamIDsByAdminUserID(ctx, userID)
	if err != nil {
		t.Fatalf("GetTeamIDsByAdminUserID: %v", err)
	}
	if len(teamIDs) != 2 {
		t.Errorf("expected user to admin 2 teams, got %d", len(teamIDs))
	}

	// У одной команды несколько администраторов.
	if err := repo.AssignTeamAdmin(ctx, userID2, teamA, uuid.Nil); err != nil {
		t.Fatalf("assign second admin to teamA: %v", err)
	}
	admins, err := repo.GetTeamAdminsByTeamID(ctx, teamA)
	if err != nil {
		t.Fatalf("GetTeamAdminsByTeamID: %v", err)
	}
	if len(admins) != 2 {
		t.Errorf("expected teamA to have 2 admins, got %d", len(admins))
	}

	// Снятие одной привязки не затрагивает другую.
	if err := repo.RemoveTeamAdmin(ctx, userID, teamA); err != nil {
		t.Fatalf("remove teamA: %v", err)
	}
	stillAdminB, err := repo.IsTeamAdmin(ctx, userID, teamB)
	if err != nil {
		t.Fatalf("IsTeamAdmin teamB: %v", err)
	}
	if !stillAdminB {
		t.Error("expected user to remain team-admin of teamB after removal from teamA")
	}
}

func TestTeamAdmin_CascadeOnUserAndTeamDeletion(t *testing.T) {
	repo, cleanup := newTestRepository(t)
	defer cleanup()
	ctx := context.Background()

	teamID, userIDs := seedTeamAndUsers(t, repo, 1)
	userID := userIDs[0]

	if err := repo.AssignTeamAdmin(ctx, userID, teamID, uuid.Nil); err != nil {
		t.Fatalf("assign: %v", err)
	}

	// Удаление пользователя должно каскадно убрать привязку team_admins.
	if err := repo.DeleteUser(ctx, userID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	isAdmin, err := repo.IsTeamAdmin(ctx, userID, teamID)
	if err != nil {
		t.Fatalf("IsTeamAdmin after user delete: %v", err)
	}
	if isAdmin {
		t.Error("expected team_admins row to be cascaded on user deletion")
	}

	// Повторяем для удаления команды.
	teamID2, userIDs2 := seedTeamAndUsers(t, repo, 1)
	userID2 := userIDs2[0]
	if err := repo.AssignTeamAdmin(ctx, userID2, teamID2, uuid.Nil); err != nil {
		t.Fatalf("assign 2: %v", err)
	}
	if _, err := repo.DB.ExecContext(ctx, `DELETE FROM teams WHERE id = $1`, teamID2); err != nil {
		t.Fatalf("delete team: %v", err)
	}
	stillAdmin, err := repo.IsTeamAdmin(ctx, userID2, teamID2)
	if err != nil {
		t.Fatalf("IsTeamAdmin after team delete: %v", err)
	}
	if stillAdmin {
		t.Error("expected team_admins row to be cascaded on team deletion")
	}
}

func TestTeamAdminAuth_TelegramIDBased(t *testing.T) {
	repo, cleanup := newTestRepository(t)
	defer cleanup()
	ctx := context.Background()

	teamID, userIDs := seedTeamAndUsers(t, repo, 1)
	userID := userIDs[0]
	user, err := repo.GetUserByID(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}

	auth := NewTeamAdminAuth(repo)

	// Незарегистрированный telegram_id — не admin, без ошибки.
	isAdminAny, err := auth.IsTeamAdminOfAny(ctx, "unknown_telegram_id")
	if err != nil {
		t.Fatalf("IsTeamAdminOfAny(unknown): %v", err)
	}
	if isAdminAny {
		t.Error("expected unknown telegram_id not to be team-admin")
	}

	if err := repo.AssignTeamAdmin(ctx, userID, teamID, uuid.Nil); err != nil {
		t.Fatalf("assign: %v", err)
	}

	isAdminAny, err = auth.IsTeamAdminOfAny(ctx, user.TelegramID)
	if err != nil {
		t.Fatalf("IsTeamAdminOfAny: %v", err)
	}
	if !isAdminAny {
		t.Error("expected registered team-admin telegram_id to be admin of any")
	}

	isAdminOf, err := auth.IsTeamAdminOf(ctx, user.TelegramID, teamID)
	if err != nil {
		t.Fatalf("IsTeamAdminOf: %v", err)
	}
	if !isAdminOf {
		t.Error("expected IsTeamAdminOf to be true for the assigned team")
	}

	teamIDs, err := auth.AdminTeamIDs(ctx, user.TelegramID)
	if err != nil {
		t.Fatalf("AdminTeamIDs: %v", err)
	}
	if len(teamIDs) != 1 || teamIDs[0] != teamID {
		t.Errorf("expected AdminTeamIDs to return [%v], got %v", teamID, teamIDs)
	}
}
