package domain

import (
	"time"

	"github.com/google/uuid"
)

// GanttTask represents a task on the Gantt chart.
// Parent tasks correspond to epics; child tasks correspond to roles.
type GanttTask struct {
	ID           uuid.UUID
	EpicID       uuid.UUID
	RoleID       *uuid.UUID // nil for parent (epic) tasks
	Name         string
	StartDate    time.Time
	EndDate      time.Time
	Progress     float64
	SortOrder    int
	IsParent     bool
	ParentTaskID *uuid.UUID // nil for parent tasks
	// AssigneeID — исполнитель листовой (ролевой) задачи: участник команды,
	// выбранный планировщиком автоматически из пула кандидатов роли либо
	// закреплённый вручную через task_assignments (см. TaskAssignment ниже).
	// nil у родительских (стори/эпик) задач и у ролевых задач, для которых
	// пул роли в команде пуст (роль без кандидатов планируется как единый
	// ресурс — см. openspec/changes/add-gantt-task-assignees). Это
	// ПРОИЗВОДНОЕ поле, как StartDate/EndDate — источник истины для него
	// сам планировщик (RecalculateTeamSchedule). Единственное исключение —
	// замороженные задачи (Progress > 0 | ActualEndDate != nil): для них
	// поле, наоборот, читается как факт и не пересчитывается (design.md
	// Решение 6).
	AssigneeID *uuid.UUID `db:"assignee_id"`
	// ActualEndDate — фактическая дата завершения задачи (проставляется
	// автоматически при простановке 100% прогресса), nil пока не завершена.
	ActualEndDate *time.Time
	// ActualEffortDays — фактическая трудоёмкость в рабочих днях между
	// плановым стартом задачи и ActualEndDate, вычисляется автоматически.
	ActualEffortDays *int
	// StartOffsetDays — ИСТОРИЧЕСКОЕ поле lead/lag планового старта листовой
	// (ролевой) задачи, ранее хранившееся прямо в строке Ганта и терявшееся
	// при каждой перегенерации задач эпика. Начиная с миграции
	// 011_task_assignees источником истины для смещения старта служит
	// TaskAssignment.StartOffsetDays (таблица task_assignments, ключ —
	// пара «стори (или эпик без сторей) + роль»), которая переживает
	// перегенерацию в отличие от этой колонки. Колонка
	// gantt_tasks.start_offset_days в БД сохранена (мигратор проекта
	// forward-only, удаление сломало бы откат релиза), но планировщиком
	// больше не читается и не пишется — см. design.md Решение 5.
	StartOffsetDays int `db:"start_offset_days"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// TaskAssignment — пользовательский ввод для ролевой задачи диаграммы
// Ганта: кто за ней закреплён и на сколько дней вручную сдвинут её плановый
// старт. EpicID здесь — идентификатор СТОРИ (или самого эпика для
// legacy-эпиков без сторей), а НЕ gantt_tasks.epic_id (тот у всех листовых
// задач одного эпика одинаков и указывает на верхний эпик, а не на
// конкретную стори). Ключ (EpicID, RoleID) уникален — см. таблицу
// task_assignments (миграция 011_task_assignees.sql) и design.md Решение 4.
//
// Это ВВОД (намерение человека): он переживает перегенерацию gantt_tasks
// (которая удаляет и пересоздаёт все строки), в отличие от производного
// состояния диаграммы (даты, GanttTask.AssigneeID, progress), которое
// планировщик пересчитывает заново при каждом вызове.
type TaskAssignment struct {
	EpicID uuid.UUID `db:"epic_id"`
	RoleID uuid.UUID `db:"role_id"`
	// UserID — закреплённый исполнитель, nil означает «автоматически»
	// (закрепление снято либо никогда не выставлялось). Закрепление на
	// человека, переставшего быть кандидатом роли (вышел из команды или
	// лишился роли), НЕ удаляется — строка сохраняется, но не действует,
	// пока кандидат не вернётся в пул (design.md Решение 7).
	UserID *uuid.UUID `db:"user_id"`
	// StartOffsetDays — смещение (lead/lag, в днях) планового старта
	// задачи относительно окончания предыдущей ролевой группы внутри
	// стори. См. комментарий у GanttTask.StartOffsetDays.
	StartOffsetDays int `db:"start_offset_days"`
}

// TeamMember — участник команды с полным перечнем его ролей. В отличие от
// GetRoleByUserID (возвращает одну роль на пользователя), корректно
// отражает то, что user_roles — связь M:N: участник с двумя ролями
// возвращается со всеми ними. Используется как состав команды — источник
// списка кандидатов при закреплении исполнителя за задачей (design.md
// Решение 9).
type TeamMember struct {
	ID        uuid.UUID
	FirstName string
	LastName  string
	RoleIDs   []uuid.UUID
}
