package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"EpicScoreBot/internal/scoring"

	openrouter "github.com/revrost/go-openrouter"
	"github.com/revrost/go-openrouter/jsonschema"
)

// ─── Tool argument schemas ─────────────────────────────────────────────────

type epicByNumberArgs struct {
	EpicNumber string `json:"epic_number" jsonschema_description:"Epic number exactly as stored, e.g. EP-1 (exact match)"`
}

type teamByNameArgs struct {
	TeamName string `json:"team_name" jsonschema_description:"Team name, exact match; call list_teams if unsure of the spelling"`
}

type userByTelegramIDArgs struct {
	TelegramUsername string `json:"telegram_username" jsonschema_description:"Telegram username; case and a leading @ are ignored"`
}

type listEpicsArgs struct {
	Status string `json:"status" jsonschema_description:"Epic status filter: NEW (created, not yet sent for scoring), SCORING (scoring in progress), SCORED (final score computed), or empty string for all"`
}

type teamEpicsArgs struct {
	TeamName string `json:"team_name" jsonschema_description:"Team name, exact match; call list_teams if unsure of the spelling"`
	Status   string `json:"status" jsonschema_description:"Epic status filter: NEW (created, not yet sent for scoring), SCORING (scoring in progress), SCORED (final score computed), or empty string for all"`
}

type userEpicArgs struct {
	TelegramUsername string `json:"telegram_username" jsonschema_description:"Telegram username; case and a leading @ are ignored"`
	EpicNumber       string `json:"epic_number" jsonschema_description:"Epic number exactly as stored, e.g. EP-1 (exact match)"`
}

type userTeamArgs struct {
	TelegramUsername string `json:"telegram_username" jsonschema_description:"Telegram username; case and a leading @ are ignored"`
	TeamName         string `json:"team_name" jsonschema_description:"Team name, exact match; call list_teams if unsure of the spelling"`
}

type teamRoleArgs struct {
	TeamName string `json:"team_name" jsonschema_description:"Team name, exact match; call list_teams if unsure of the spelling"`
	RoleName string `json:"role_name" jsonschema_description:"Role name, exact match, e.g. Аналитик; call list_roles if unsure of the spelling"`
}

type emptyArgs struct{}

// ─── Tool definitions ──────────────────────────────────────────────────────

func buildTools() ([]openrouter.Tool, error) {
	tools := []struct {
		name        string
		description string
		schema      any
	}{
		// ── Existing tools ──
		{
			"get_epic_status",
			"Effort-scoring progress of one epic: its status, final_score (null until SCORED), how many team members submitted an effort score, and the names of those who have not. Covers effort scores only — for risk scoring progress use get_users_who_scored_risk",
			epicByNumberArgs{},
		},
		{
			"list_epics",
			"All epics across all teams with number, name, status and final_score (omitted until SCORED). Use get_team_epics when the question is about one team",
			listEpicsArgs{},
		},
		{
			"get_team_members",
			"Members of one team: full name, Telegram username and role. Use get_users_by_role_in_team to list only one role",
			teamByNameArgs{},
		},
		{
			"get_scoring_results",
			"Scoring result of one epic: final_score in person-days (effort adjusted by risk coefficients; null until SCORED) and weighted_avg effort per role. For each person's raw score use get_epic_individual_scores",
			epicByNumberArgs{},
		},
		{
			"get_user_info",
			"One user by Telegram username: full name, role, teams and weight (the multiplier applied to their scores in weighted averages)",
			userByTelegramIDArgs{},
		},
		{
			"list_risks",
			"Risks of one epic: description, status, weighted_score (weighted average of probability × impact, range 1–16) and risk_coefficient (the multiplier it applies to the epic's effort); the last two are omitted until the risk is scored",
			epicByNumberArgs{},
		},
		// ── New tools ──
		{
			"list_teams",
			"All teams with name and description. Use it to find the exact team name other tools expect",
			emptyArgs{},
		},
		{
			"list_users",
			"All registered users with full name, Telegram username, role and weight",
			emptyArgs{},
		},
		{
			"list_roles",
			"All roles with name and description. Use it to find the exact role name other tools expect",
			emptyArgs{},
		},
		{
			"get_team_epics",
			"Epics of one team with number, name, status and final_score (omitted until SCORED), optionally filtered by status",
			teamEpicsArgs{},
		},
		{
			"get_unscored_epics",
			"Epics of a team that the user has not finished scoring — either the effort score or at least one of the epic's risks is still missing. Returns epic number and name only",
			userTeamArgs{},
		},
		{
			"get_unscored_risks",
			"Risks of one epic that the given user has not scored yet (descriptions only)",
			userEpicArgs{},
		},
		{
			"get_epic_individual_scores",
			"Each person's effort score for one epic in person-days (0–500), with their role; not aggregated and not weighted. For the aggregated result use get_scoring_results",
			epicByNumberArgs{},
		},
		{
			"get_risk_individual_scores",
			"Per-person risk scores for every risk of one epic: probability (1–4), impact (1–4) and their product score (1–16)",
			epicByNumberArgs{},
		},
		{
			"check_user_scored_epic",
			"Whether one user has submitted an effort score for one epic (true/false). Effort only — for risks use get_unscored_risks",
			userEpicArgs{},
		},
		{
			"get_users_who_scored_risk",
			"For each risk of one epic, the list of users who have scored it. Use it to see risk-scoring progress",
			epicByNumberArgs{},
		},
		{
			"get_users_by_role_in_team",
			"Members of one team who hold the given role: full name and Telegram username",
			teamRoleArgs{},
		},
	}

	var result []openrouter.Tool
	for _, t := range tools {
		schema, err := jsonschema.GenerateSchemaForType(t.schema)
		if err != nil {
			return nil, fmt.Errorf("generate schema for %s: %w", t.name, err)
		}
		result = append(result, openrouter.Tool{
			Type: openrouter.ToolTypeFunction,
			Function: &openrouter.FunctionDefinition{
				Name:        t.name,
				Description: t.description,
				Parameters:  schema,
			},
		})
	}
	return result, nil
}

// ─── Tool executor ─────────────────────────────────────────────────────────

// normalizeUsername приводит username к ключу справочника users: нижний
// регистр, без ведущего "@" и пробельных краёв — FindUserByTelegramID
// нормализует только сторону БД, а модель передаёт username как в вопросе.
func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(username), "@"))
}

// executeTool runs a single tool call and returns a JSON-serialisable result.
func executeTool(ctx context.Context, repo Repository, name, argsJSON string) (string, error) {
	switch name {
	case "get_epic_status":
		var args epicByNumberArgs
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("parse args: %w", err)
		}
		epic, err := repo.GetEpicByNumber(ctx, args.EpicNumber)
		if err != nil || epic == nil {
			return `{"error":"epic not found"}`, nil
		}
		scored, err := repo.GetUsersWhoScoredEpic(ctx, epic.ID)
		if err != nil {
			return "", err
		}
		teamMembers, _ := repo.GetUsersByTeamID(ctx, epic.TeamID)

		scoredIDs := make(map[string]bool)
		for _, u := range scored {
			scoredIDs[u.TelegramID] = true
		}
		var missing []string
		for _, u := range teamMembers {
			if !scoredIDs[u.TelegramID] {
				missing = append(missing, fmt.Sprintf("%s %s (@%s)", u.FirstName, u.LastName, u.TelegramID))
			}
		}
		result := map[string]any{
			"number":       epic.Number,
			"name":         epic.Name,
			"status":       string(epic.Status),
			"scored_count": len(scored),
			"total":        len(teamMembers),
			"not_scored":   missing,
			"final_score":  epic.FinalScore,
		}
		b, _ := json.Marshal(result)
		return string(b), nil

	case "list_epics":
		var args listEpicsArgs
		_ = json.Unmarshal([]byte(argsJSON), &args)

		epics, err := repo.GetAllEpics(ctx)
		if err != nil {
			return "", err
		}
		type epicRow struct {
			Number     string   `json:"number"`
			Name       string   `json:"name"`
			Status     string   `json:"status"`
			FinalScore *float64 `json:"final_score,omitempty"`
		}
		var rows []epicRow
		for _, e := range epics {
			if args.Status != "" && !strings.EqualFold(string(e.Status), args.Status) {
				continue
			}
			rows = append(rows, epicRow{
				Number:     e.Number,
				Name:       e.Name,
				Status:     string(e.Status),
				FinalScore: e.FinalScore,
			})
		}
		b, _ := json.Marshal(rows)
		return string(b), nil

	case "get_team_members":
		var args teamByNameArgs
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("parse args: %w", err)
		}
		team, err := repo.GetTeamByName(ctx, args.TeamName)
		if err != nil || team == nil {
			return `{"error":"team not found"}`, nil
		}
		members, err := repo.GetUsersByTeamID(ctx, team.ID)
		if err != nil {
			return "", err
		}
		type memberRow struct {
			Name     string `json:"name"`
			Username string `json:"username"`
			Role     string `json:"role"`
		}
		var rows []memberRow
		for _, u := range members {
			roleName := "—"
			if role, err := repo.GetRoleByUserID(ctx, u.ID); err == nil {
				roleName = role.Name
			}
			rows = append(rows, memberRow{
				Name:     fmt.Sprintf("%s %s", u.FirstName, u.LastName),
				Username: u.TelegramID,
				Role:     roleName,
			})
		}
		result := map[string]any{"team": team.Name, "members": rows}
		b, _ := json.Marshal(result)
		return string(b), nil

	case "get_scoring_results":
		var args epicByNumberArgs
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("parse args: %w", err)
		}
		epic, err := repo.GetEpicByNumber(ctx, args.EpicNumber)
		if err != nil || epic == nil {
			return `{"error":"epic not found"}`, nil
		}
		roleScores, err := repo.GetEpicRoleScoresByEpicID(ctx, epic.ID)
		if err != nil {
			return "", err
		}
		type roleRow struct {
			Role        string  `json:"role"`
			WeightedAvg float64 `json:"weighted_avg"`
		}
		var rows []roleRow
		for _, rs := range roleScores {
			name := rs.RoleID.String()
			if role, err := repo.GetRoleByID(ctx, rs.RoleID); err == nil {
				name = role.Name
			}
			rows = append(rows, roleRow{Role: name, WeightedAvg: rs.WeightedAvg})
		}
		result := map[string]any{
			"number":      epic.Number,
			"name":        epic.Name,
			"status":      string(epic.Status),
			"final_score": epic.FinalScore,
			"role_scores": rows,
		}
		b, _ := json.Marshal(result)
		return string(b), nil

	case "get_user_info":
		var args userByTelegramIDArgs
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("parse args: %w", err)
		}
		user, err := repo.FindUserByTelegramID(ctx, normalizeUsername(args.TelegramUsername))
		if err != nil || user == nil {
			return `{"error":"user not found"}`, nil
		}
		roleName := "—"
		if role, err := repo.GetRoleByUserID(ctx, user.ID); err == nil {
			roleName = role.Name
		}
		teams, _ := repo.GetTeamsByUserTelegramID(ctx, user.TelegramID)
		var teamNames []string
		for _, t := range teams {
			teamNames = append(teamNames, t.Name)
		}
		result := map[string]any{
			"name":     fmt.Sprintf("%s %s", user.FirstName, user.LastName),
			"username": user.TelegramID,
			"role":     roleName,
			"weight":   user.Weight,
			"teams":    teamNames,
		}
		b, _ := json.Marshal(result)
		return string(b), nil

	case "list_risks":
		var args epicByNumberArgs
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("parse args: %w", err)
		}
		epic, err := repo.GetEpicByNumber(ctx, args.EpicNumber)
		if err != nil || epic == nil {
			return `{"error":"epic not found"}`, nil
		}
		risks, err := repo.GetRisksByEpicID(ctx, epic.ID)
		if err != nil {
			return "", err
		}
		type riskRow struct {
			Description   string   `json:"description"`
			Status        string   `json:"status"`
			WeightedScore *float64 `json:"weighted_score,omitempty"`
			Coefficient   *float64 `json:"risk_coefficient,omitempty"`
		}
		var rows []riskRow
		for _, r := range risks {
			row := riskRow{
				Description:   r.Description,
				Status:        string(r.Status),
				WeightedScore: r.WeightedScore,
			}
			if r.WeightedScore != nil {
				c := scoring.RiskCoefficient(*r.WeightedScore)
				row.Coefficient = &c
			}
			rows = append(rows, row)
		}
		b, _ := json.Marshal(rows)
		return string(b), nil

	// ─── New tools ─────────────────────────────────────────────────────

	case "list_teams":
		teams, err := repo.GetAllTeams(ctx)
		if err != nil {
			return "", err
		}
		type teamRow struct {
			Name        string `json:"name"`
			Description string `json:"description,omitempty"`
		}
		var rows []teamRow
		for _, t := range teams {
			rows = append(rows, teamRow{Name: t.Name, Description: t.Description})
		}
		b, _ := json.Marshal(rows)
		return string(b), nil

	case "list_users":
		users, err := repo.GetAllUsers(ctx)
		if err != nil {
			return "", err
		}
		type userRow struct {
			Name     string `json:"name"`
			Username string `json:"username"`
			Role     string `json:"role"`
			Weight   int    `json:"weight"`
		}
		var rows []userRow
		for _, u := range users {
			roleName := "—"
			if role, err := repo.GetRoleByUserID(ctx, u.ID); err == nil {
				roleName = role.Name
			}
			rows = append(rows, userRow{
				Name:     fmt.Sprintf("%s %s", u.FirstName, u.LastName),
				Username: u.TelegramID,
				Role:     roleName,
				Weight:   u.Weight,
			})
		}
		b, _ := json.Marshal(rows)
		return string(b), nil

	case "list_roles":
		roles, err := repo.GetAllRoles(ctx)
		if err != nil {
			return "", err
		}
		type roleRow struct {
			Name        string `json:"name"`
			Description string `json:"description,omitempty"`
		}
		var rows []roleRow
		for _, r := range roles {
			rows = append(rows, roleRow{Name: r.Name, Description: r.Description})
		}
		b, _ := json.Marshal(rows)
		return string(b), nil

	case "get_team_epics":
		var args teamEpicsArgs
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("parse args: %w", err)
		}
		team, err := repo.GetTeamByName(ctx, args.TeamName)
		if err != nil || team == nil {
			return `{"error":"team not found"}`, nil
		}
		type epicRow struct {
			Number     string   `json:"number"`
			Name       string   `json:"name"`
			Status     string   `json:"status"`
			FinalScore *float64 `json:"final_score,omitempty"`
		}
		// If status filter is provided, use it; otherwise get all statuses
		// by querying each status individually is complex — just get all and filter.
		allEpics, err := repo.GetAllEpics(ctx)
		if err != nil {
			return "", err
		}
		var rows []epicRow
		for _, e := range allEpics {
			if e.TeamID != team.ID {
				continue
			}
			if args.Status != "" && !strings.EqualFold(string(e.Status), args.Status) {
				continue
			}
			rows = append(rows, epicRow{
				Number:     e.Number,
				Name:       e.Name,
				Status:     string(e.Status),
				FinalScore: e.FinalScore,
			})
		}
		result := map[string]any{"team": team.Name, "epics": rows}
		b, _ := json.Marshal(result)
		return string(b), nil

	case "get_unscored_epics":
		var args userTeamArgs
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("parse args: %w", err)
		}
		user, err := repo.FindUserByTelegramID(ctx, normalizeUsername(args.TelegramUsername))
		if err != nil || user == nil {
			return `{"error":"user not found"}`, nil
		}
		team, err := repo.GetTeamByName(ctx, args.TeamName)
		if err != nil || team == nil {
			return `{"error":"team not found"}`, nil
		}
		epics, err := repo.GetUnscoredEpicsByUser(ctx, user.ID, team.ID)
		if err != nil {
			return "", err
		}
		type epicRow struct {
			Number string `json:"number"`
			Name   string `json:"name"`
		}
		var rows []epicRow
		for _, e := range epics {
			rows = append(rows, epicRow{Number: e.Number, Name: e.Name})
		}
		result := map[string]any{
			"user":           fmt.Sprintf("%s %s", user.FirstName, user.LastName),
			"team":           team.Name,
			"unscored_epics": rows,
		}
		b, _ := json.Marshal(result)
		return string(b), nil

	case "get_unscored_risks":
		var args userEpicArgs
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("parse args: %w", err)
		}
		user, err := repo.FindUserByTelegramID(ctx, normalizeUsername(args.TelegramUsername))
		if err != nil || user == nil {
			return `{"error":"user not found"}`, nil
		}
		epic, err := repo.GetEpicByNumber(ctx, args.EpicNumber)
		if err != nil || epic == nil {
			return `{"error":"epic not found"}`, nil
		}
		risks, err := repo.GetUnscoredRisksByUser(ctx, user.ID, epic.ID)
		if err != nil {
			return "", err
		}
		type riskRow struct {
			Description string `json:"description"`
		}
		var rows []riskRow
		for _, r := range risks {
			rows = append(rows, riskRow{Description: r.Description})
		}
		result := map[string]any{
			"user":           fmt.Sprintf("%s %s", user.FirstName, user.LastName),
			"epic":           epic.Number,
			"unscored_risks": rows,
		}
		b, _ := json.Marshal(result)
		return string(b), nil

	case "get_epic_individual_scores":
		var args epicByNumberArgs
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("parse args: %w", err)
		}
		epic, err := repo.GetEpicByNumber(ctx, args.EpicNumber)
		if err != nil || epic == nil {
			return `{"error":"epic not found"}`, nil
		}
		scores, err := repo.GetEpicScoresByEpicID(ctx, epic.ID)
		if err != nil {
			return "", err
		}
		type scoreRow struct {
			User  string `json:"user"`
			Role  string `json:"role"`
			Score int    `json:"score"`
		}
		var rows []scoreRow
		for _, s := range scores {
			userName := s.UserID.String()
			if u, err := repo.GetUserByID(ctx, s.UserID); err == nil {
				userName = fmt.Sprintf("%s %s (@%s)", u.FirstName, u.LastName, u.TelegramID)
			}
			roleName := s.RoleID.String()
			if r, err := repo.GetRoleByID(ctx, s.RoleID); err == nil {
				roleName = r.Name
			}
			rows = append(rows, scoreRow{User: userName, Role: roleName, Score: s.Score})
		}
		result := map[string]any{
			"epic":   epic.Number,
			"name":   epic.Name,
			"scores": rows,
		}
		b, _ := json.Marshal(result)
		return string(b), nil

	case "get_risk_individual_scores":
		var args epicByNumberArgs
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("parse args: %w", err)
		}
		epic, err := repo.GetEpicByNumber(ctx, args.EpicNumber)
		if err != nil || epic == nil {
			return `{"error":"epic not found"}`, nil
		}
		risks, err := repo.GetRisksByEpicID(ctx, epic.ID)
		if err != nil {
			return "", err
		}
		type riskScoreRow struct {
			User        string `json:"user"`
			Probability int    `json:"probability"`
			Impact      int    `json:"impact"`
			Score       int    `json:"score"`
		}
		type riskResult struct {
			Description string         `json:"description"`
			Status      string         `json:"status"`
			Scores      []riskScoreRow `json:"scores"`
		}
		var riskResults []riskResult
		for _, risk := range risks {
			riskScores, err := repo.GetRiskScoresByRiskID(ctx, risk.ID)
			if err != nil {
				continue
			}
			var scoreRows []riskScoreRow
			for _, rs := range riskScores {
				userName := rs.UserID.String()
				if u, err := repo.GetUserByID(ctx, rs.UserID); err == nil {
					userName = fmt.Sprintf("%s %s (@%s)", u.FirstName, u.LastName, u.TelegramID)
				}
				scoreRows = append(scoreRows, riskScoreRow{
					User:        userName,
					Probability: rs.Probability,
					Impact:      rs.Impact,
					Score:       rs.Probability * rs.Impact,
				})
			}
			riskResults = append(riskResults, riskResult{
				Description: risk.Description,
				Status:      string(risk.Status),
				Scores:      scoreRows,
			})
		}
		result := map[string]any{"epic": epic.Number, "risks": riskResults}
		b, _ := json.Marshal(result)
		return string(b), nil

	case "check_user_scored_epic":
		var args userEpicArgs
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("parse args: %w", err)
		}
		user, err := repo.FindUserByTelegramID(ctx, normalizeUsername(args.TelegramUsername))
		if err != nil || user == nil {
			return `{"error":"user not found"}`, nil
		}
		epic, err := repo.GetEpicByNumber(ctx, args.EpicNumber)
		if err != nil || epic == nil {
			return `{"error":"epic not found"}`, nil
		}
		scored, err := repo.HasUserScoredEpic(ctx, epic.ID, user.ID)
		if err != nil {
			return "", err
		}
		result := map[string]any{
			"user":   fmt.Sprintf("%s %s", user.FirstName, user.LastName),
			"epic":   epic.Number,
			"scored": scored,
		}
		b, _ := json.Marshal(result)
		return string(b), nil

	case "get_users_who_scored_risk":
		var args epicByNumberArgs
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("parse args: %w", err)
		}
		epic, err := repo.GetEpicByNumber(ctx, args.EpicNumber)
		if err != nil || epic == nil {
			return `{"error":"epic not found"}`, nil
		}
		risks, err := repo.GetRisksByEpicID(ctx, epic.ID)
		if err != nil {
			return "", err
		}
		type riskInfo struct {
			Description string   `json:"description"`
			ScoredBy    []string `json:"scored_by"`
		}
		var riskInfos []riskInfo
		for _, risk := range risks {
			users, _ := repo.GetUsersWhoScoredRisk(ctx, risk.ID)
			var names []string
			for _, u := range users {
				names = append(names, fmt.Sprintf("%s %s (@%s)", u.FirstName, u.LastName, u.TelegramID))
			}
			riskInfos = append(riskInfos, riskInfo{
				Description: risk.Description,
				ScoredBy:    names,
			})
		}
		result := map[string]any{"epic": epic.Number, "risks": riskInfos}
		b, _ := json.Marshal(result)
		return string(b), nil

	case "get_users_by_role_in_team":
		var args teamRoleArgs
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("parse args: %w", err)
		}
		team, err := repo.GetTeamByName(ctx, args.TeamName)
		if err != nil || team == nil {
			return `{"error":"team not found"}`, nil
		}
		role, err := repo.GetRoleByName(ctx, args.RoleName)
		if err != nil || role == nil {
			return `{"error":"role not found"}`, nil
		}
		users, err := repo.GetUsersByTeamIDAndRoleID(ctx, team.ID, role.ID)
		if err != nil {
			return "", err
		}
		type userRow struct {
			Name     string `json:"name"`
			Username string `json:"username"`
		}
		var rows []userRow
		for _, u := range users {
			rows = append(rows, userRow{
				Name:     fmt.Sprintf("%s %s", u.FirstName, u.LastName),
				Username: u.TelegramID,
			})
		}
		result := map[string]any{
			"team":  team.Name,
			"role":  role.Name,
			"users": rows,
		}
		b, _ := json.Marshal(result)
		return string(b), nil

	default:
		return fmt.Sprintf(`{"error":"unknown tool %q"}`, name), nil
	}
}
