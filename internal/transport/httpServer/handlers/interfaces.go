package handlers

import (
	"EpicScoreBot/internal/gantt"
	"EpicScoreBot/internal/models/domain"
	"EpicScoreBot/internal/report"
	"context"
	"time"

	"github.com/google/uuid"
)

// GanttService defines the business-logic contract used by handlers.
type GanttService interface {
	GenerateTasksForEpic(ctx context.Context, epicID uuid.UUID, startDate time.Time) ([]domain.GanttTask, error)
	// GenerateTasksForQuarter (пере)генерирует задачи Ганта для всех
	// заскоренных топ-эпиков команды за указанные год и квартал одним
	// пересчётом расписания вместо N пересчётов по одному на эпик.
	GenerateTasksForQuarter(ctx context.Context, teamID uuid.UUID, year, quarter int, startDate time.Time) (gantt.QuarterGenerationResult, error)
	ReorderTask(ctx context.Context, taskID uuid.UUID, newSortOrder int) ([]domain.GanttTask, error)
	// ReorderEpic меняет позицию топ-эпика в очереди конвейерного планировщика команды.
	ReorderEpic(ctx context.Context, epicID uuid.UUID, newSortOrder int) ([]domain.GanttTask, error)
	// ReorderStory меняет позицию стори в очереди сторей родительского эпика.
	ReorderStory(ctx context.Context, storyID uuid.UUID, newSortOrder int) ([]domain.GanttTask, error)
	// SetTaskProgress выставляет прогресс листовой (ролевой) задачи и
	// автоматически фиксирует факт завершения при достижении 100%.
	SetTaskProgress(ctx context.Context, taskID uuid.UUID, progress float64) ([]domain.GanttTask, error)
	// SetTaskStartOffset выставляет смещение (lead/lag, в днях) старта
	// листовой (ролевой) задачи относительно окончания предыдущей ролевой
	// группы внутри той же стори.
	SetTaskStartOffset(ctx context.Context, taskID uuid.UUID, offsetDays int) ([]domain.GanttTask, error)
	GetTeamTasks(ctx context.Context, teamID uuid.UUID) ([]domain.GanttTask, error)
	// GetTeamTasksWithAssignments — то же самое, что GetTeamTasks, плюс
	// для каждой листовой (ролевой) задачи, у которой есть запись
	// task_assignments (закрепление и/или смещение старта), её значение —
	// с ключом по ID самой задачи. Единый источник данных для полей
	// assignee_* и актуального start_offset_days в ganttTaskResp (см.
	// openspec/changes/add-gantt-task-assignees). Третье возвращаемое
	// значение — множество пар "стори + роль", которым СЕЙЧАС
	// соответствует сгенерированная листовая задача (backend §3.1,
	// add-task-start-constraints): побочный продукт того же прохода, без
	// отдельного запроса, используется, чтобы отметить ссылку "не ранее
	// задачи" недействующей, если её цель в этот набор не входит.
	GetTeamTasksWithAssignments(ctx context.Context, teamID uuid.UUID) ([]domain.GanttTask, map[uuid.UUID]domain.TaskAssignment, map[gantt.TaskRef]bool, error)
	// SetTaskAssignee закрепляет (userID != nil) либо снимает закрепление
	// (userID == nil — возврат к автоматическому распределению)
	// исполнителя ролевой задачи и пересчитывает расписание команды.
	SetTaskAssignee(ctx context.Context, taskID uuid.UUID, userID *uuid.UUID) ([]domain.GanttTask, error)
	// GetTeamMembers возвращает состав команды с полным перечнем ролей
	// каждого участника (в отличие от Repository.GetRoleByUserID —
	// одна роль на пользователя, тогда как user_roles — M:N).
	GetTeamMembers(ctx context.Context, teamID uuid.UUID) ([]domain.TeamMember, error)
	// SetTeamBackfillBlockPast переключает настройку команды «не занимать
	// промежутки расписания, оставшиеся в прошлом» и пересчитывает
	// расписание команды по новому правилу (backend §3.2,
	// openspec/changes/backfill-idle-gaps-in-schedule).
	SetTeamBackfillBlockPast(ctx context.Context, teamID uuid.UUID, blocked bool) ([]domain.GanttTask, error)
	// SetTaskStartConstraint задаёт (либо снимает, через nil) ограничение
	// "Начать не ранее" листовой (ролевой) задачи: дату и/или ссылку на
	// другую задачу той же команды, парой "стори + роль" — и
	// пересчитывает расписание команды (backend §3.2,
	// openspec/changes/add-task-start-constraints).
	SetTaskStartConstraint(
		ctx context.Context,
		taskID uuid.UUID,
		notBeforeDate *time.Time,
		waitForStoryID, waitForRoleID *uuid.UUID,
	) ([]domain.GanttTask, error)
	// GetTeamTaskOptionsFor возвращает состав задач команды для
	// двухшагового выбора цели ограничения "Начать не ранее" (стори,
	// затем роль внутри неё) — задачи, недоступные для выбора (сама
	// задача и всё, что замкнуло бы цикл), помечены, а не исключены
	// (backend §3.3, design.md Решение 6).
	GetTeamTaskOptionsFor(ctx context.Context, taskID uuid.UUID) ([]gantt.TeamTaskOption, error)
}

// Repository defines the data-access contract used by handlers.
type Repository interface {
	// Teams
	CreateTeam(ctx context.Context, name, description string) (*domain.Team, error)
	GetAllTeams(ctx context.Context) ([]domain.Team, error)
	GetTeamsByUserTelegramID(ctx context.Context, telegramID string) ([]domain.Team, error)
	GetTeamByID(ctx context.Context, teamID uuid.UUID) (*domain.Team, error)
	GetTeamByName(ctx context.Context, name string) (*domain.Team, error)

	CreateUser(ctx context.Context, firstName, lastName string, telegramID string, weight int) (*domain.User, error)
	CreateUserWithRelations(ctx context.Context, user *domain.User, teamUUIDs []uuid.UUID, roleUUIDs []uuid.UUID) error
	UpdateUserWithRelations(ctx context.Context, userID uuid.UUID, firstName, lastName string, weight int, teamUUIDs []uuid.UUID, roleUUIDs []uuid.UUID) error
	GetUserRelations(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, []uuid.UUID, error)
	GetUserTeams(ctx context.Context, userID uuid.UUID) ([]domain.Team, error)
	GetUserRoles(ctx context.Context, userID uuid.UUID) ([]domain.Role, error)
	GetAllUsers(ctx context.Context) ([]domain.User, error)
	BulkCreateUsers(ctx context.Context, users []domain.User, teamID *uuid.UUID, roleID *uuid.UUID) error
	FindUserByTelegramID(ctx context.Context, telegramID string) (*domain.User, error)
	GetUserByID(ctx context.Context, userID uuid.UUID) (*domain.User, error)
	GetUsersByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.User, error)
	// GetUsersByTeamIDAndRoleID — пул кандидатов на роль исполнителя
	// (используется при валидации PUT /tasks/{id}/assignee: закрепляемый
	// user_id обязан быть кандидатом роли задачи).
	GetUsersByTeamIDAndRoleID(ctx context.Context, teamID, roleID uuid.UUID) ([]domain.User, error)
	AssignUserTeam(ctx context.Context, userID, teamID uuid.UUID) error
	RemoveUserTeam(ctx context.Context, userID, teamID uuid.UUID) error
	AssignUserRole(ctx context.Context, userID, roleID uuid.UUID) error
	RemoveUserRole(ctx context.Context, userID, roleID uuid.UUID) error
	UpdateUserWeight(ctx context.Context, userID uuid.UUID, weight int) error
	DeleteUser(ctx context.Context, userID uuid.UUID) error

	// Team-admins (team-scoped роль admin). HTTP-сессия идентифицирует
	// пользователя по telegram_id (middleware.UserSession.TelegramID), поэтому
	// эти методы принимают telegram_id, а не UUID пользователя — в отличие от
	// одноимённых UUID-ориентированных методов Repository, используемых
	// Telegram-ботом (см. repositories.TeamAdminAuth).
	IsTeamAdminOfAny(ctx context.Context, telegramID string) (bool, error)
	IsTeamAdminOf(ctx context.Context, telegramID string, teamID uuid.UUID) (bool, error)
	AdminTeamIDs(ctx context.Context, telegramID string) ([]uuid.UUID, error)
	// Управление привязками team_admins (только для superadmin, см.
	// handlers/team_admin.go) — UUID-ориентированные, т.к. работают с уже
	// резолвленным пользователем-целью.
	AssignTeamAdmin(ctx context.Context, userID, teamID, assignedBy uuid.UUID) error
	RemoveTeamAdmin(ctx context.Context, userID, teamID uuid.UUID) error
	GetTeamAdminsByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.User, error)

	// Epics
	CreateEpic(ctx context.Context, number, name, description string, teamID uuid.UUID, year, quarter int, epicType string, evaluatingRoleIDs []uuid.UUID) (*domain.Epic, error)
	GetEvaluatingRoleIDs(ctx context.Context, epicID uuid.UUID) ([]uuid.UUID, error)
	GetEpicsByTeamYearQuarter(ctx context.Context, teamID uuid.UUID, year, quarter int) ([]domain.Epic, error)
	GetExpectedScorersCount(ctx context.Context, epicID uuid.UUID, teamID uuid.UUID) (int, error)
	GetSubmittedEpicScorersCount(ctx context.Context, epicID uuid.UUID, teamID uuid.UUID) (int, error)
	GetSubmittedRiskScorersCount(ctx context.Context, riskID uuid.UUID, epicID uuid.UUID, teamID uuid.UUID) (int, error)
	GetEpicByID(ctx context.Context, epicID uuid.UUID) (*domain.Epic, error)
	GetEpicByNumber(ctx context.Context, number string) (*domain.Epic, error)
	GetEpicsByTeamIDAndStatus(ctx context.Context, teamID uuid.UUID, status domain.Status) ([]domain.Epic, error)
	UpdateEpicStatus(ctx context.Context, epicID uuid.UUID, status domain.Status) error
	StartEpicScoring(ctx context.Context, epicID uuid.UUID) error
	DeleteEpic(ctx context.Context, epicID uuid.UUID) error
	GetAllEpics(ctx context.Context) ([]domain.Epic, error)
	GetStoriesByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Epic, error)
	CountStoriesByEpicID(ctx context.Context, epicID uuid.UUID) (int, error)
	UpdateEpic(ctx context.Context, epic *domain.Epic, newEvaluatingRoles []uuid.UUID, oldNumber string) error
	UpdateStory(ctx context.Context, story *domain.Epic) error
	CreateStory(ctx context.Context, parentEpicID uuid.UUID, number, name, description string, teamID uuid.UUID, year, quarter int, epicType string, evaluatingRoleIDs []uuid.UUID) (*domain.Epic, error)

	// Risks
	CreateRisk(ctx context.Context, description string, epicID uuid.UUID) (*domain.Risk, error)
	GetRiskByID(ctx context.Context, riskID uuid.UUID) (*domain.Risk, error)
	GetRisksByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.Risk, error)
	UpdateRisk(ctx context.Context, riskID uuid.UUID, description string) error
	DeleteRisk(ctx context.Context, riskID uuid.UUID) error

	// Roles
	GetAllRoles(ctx context.Context) ([]domain.Role, error)
	GetRoleByID(ctx context.Context, roleID uuid.UUID) (*domain.Role, error)
	GetRoleByName(ctx context.Context, name string) (*domain.Role, error)
	GetRoleByUserID(ctx context.Context, userID uuid.UUID) (*domain.Role, error)

	// Scoring
	CreateEpicScore(ctx context.Context, epicID, userID, roleID uuid.UUID, score int) error
	CreateRiskScore(ctx context.Context, riskID, userID uuid.UUID, probability, impact int) error
	// HasUserScoredEpic и GetUnscoredRisksByUser используются, помимо прочего,
	// notify.BuildEpicScoringReminders (см. handlers.NotifyEpicReminders) —
	// GanttHandler удовлетворяет notify.ReminderRepository напрямую через
	// Repository, без отдельного адаптера (в отличие от internal/telegram, где
	// доступ к данным разделён на три узких сервисных интерфейса).
	HasUserScoredEpic(ctx context.Context, epicID, userID uuid.UUID) (bool, error)
	GetUnscoredRisksByUser(ctx context.Context, userID, epicID uuid.UUID) ([]domain.Risk, error)
	GetEpicScoresByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.EpicScore, error)
	GetEpicScoresByUserID(ctx context.Context, userID uuid.UUID) ([]domain.EpicScore, error)
	GetEpicRoleScoresByEpicID(ctx context.Context, epicID uuid.UUID) ([]domain.EpicRoleScore, error)
	GetRiskScoresByRiskID(ctx context.Context, riskID uuid.UUID) ([]domain.RiskScore, error)
	GetRiskScoresByUserID(ctx context.Context, userID uuid.UUID) ([]domain.RiskScore, error)
	GetUsersWhoScoredEpic(ctx context.Context, epicID uuid.UUID) ([]domain.User, error)
	GetUsersWhoScoredRisk(ctx context.Context, riskID uuid.UUID) ([]domain.User, error)
	CountTeamMembers(ctx context.Context, teamID uuid.UUID) (int, error)
	CountEpicScores(ctx context.Context, epicID uuid.UUID) (int, error)
	CountRiskScores(ctx context.Context, riskID uuid.UUID) (int, error)

	// Gantt tasks
	GetGanttTasksByTeamID(ctx context.Context, teamID uuid.UUID) ([]domain.GanttTask, error)
	GetGanttTaskByID(ctx context.Context, taskID uuid.UUID) (*domain.GanttTask, error)
	UpdateGanttTaskProgress(ctx context.Context, taskID uuid.UUID, progress float64) error
	DeleteGanttTasksByEpicID(ctx context.Context, epicID uuid.UUID) error
}

// ScoringService defines the contract for epic and risk scoring completion checks.
type ScoringService interface {
	TryCompleteEpicScoring(ctx context.Context, epicID uuid.UUID) error
	TryCompleteRiskScoring(ctx context.Context, riskID uuid.UUID) error
	// SetManualFinalScore позволяет вручную переопределить итоговую оценку
	// (final_score) уже оцененного эпика/стори (статус SCORED), с каскадным
	// пересчётом родительского эпика при необходимости.
	SetManualFinalScore(ctx context.Context, epicID uuid.UUID, finalScore float64) (*domain.Epic, error)
	// SetManualRoleScore позволяет вручную переопределить агрегированную оценку
	// (weighted_avg) конкретной роли уже оцененного эпика/стори (статус SCORED),
	// без изменения оценок отдельных участников и без каскадного пересчёта
	// финальной оценки/родительского эпика.
	SetManualRoleScore(ctx context.Context, epicID, roleID uuid.UUID, score float64) (*domain.EpicRoleScore, error)
	// PreviewFinalScore считает финальную оценку стори по стандартной формуле
	// на основе текущих сохранённых epic_role_scores и оценок рисков, без
	// сохранения результата.
	PreviewFinalScore(ctx context.Context, epicID uuid.UUID) (float64, error)
	// SubmitExpertRoleScore проставляет одну и ту же экспертную оценку сразу
	// за всех участников команды с указанной ролью, пока скоринг эпика/стори
	// ещё идёт (статус SCORING), и запускает попытку завершения скоринга.
	// Возвращает количество затронутых участников.
	SubmitExpertRoleScore(ctx context.Context, epicID, roleID uuid.UUID, score int) (int, error)
}

// TeamAdminScoper предоставляет точечные team-scoped проверки роли admin по
// telegram_id HTTP-сессии — используется в хендлерах, где team_id целевого
// ресурса известен только после разбора тела запроса/URL-параметров (в
// отличие от middleware.RoleAuth, грубого гейта на уровне группы роутов).
// Удовлетворяется тем же значением Repository (см. repositories.TeamAdminAuth).
type TeamAdminScoper interface {
	IsTeamAdminOf(ctx context.Context, telegramID string, teamID uuid.UUID) (bool, error)
	AdminTeamIDs(ctx context.Context, telegramID string) ([]uuid.UUID, error)
}

// ReportDataProvider предоставляет агрегированные данные для PDF-отчёта
// команды (см. epicService.GetReportData) — используется ExportTeamReport
// (format=pdf); GanttHandler получает конкретную реализацию (epicService)
// через WithReportServices, не завязываясь на конкретный тип services.EpicService.
type ReportDataProvider interface {
	GetReportData(ctx context.Context, teamID uuid.UUID, year, quarter int) (*report.ReportData, error)
}

// PDFReportGenerator генерирует PDF-файл отчёта команды через Gotenberg по
// данным ReportDataProvider (см. report.Generator.GenerateReport).
type PDFReportGenerator interface {
	GenerateReport(ctx context.Context, data report.ReportData) (string, error)
}

// AIClient defines the contract for interacting with the AI assistant.
type AIClient interface {
	Ask(ctx context.Context, question string) (string, error)
}

// TelegramNotifier defines the contract for sending a direct Telegram message
// to a user, used by NotifyEpicReminders (см. handlers/notify.go) to deliver
// epic scoring reminders triggered from the web panel. Реализуется
// *telegram.Bot (см. internal/telegram.Bot.SendDirectMessage) структурно,
// без явного импорта пакета telegram в handlers.
type TelegramNotifier interface {
	SendDirectMessage(ctx context.Context, chatID int64, text string) error
}
