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
	EpicNumber string `json:"epic_number" jsonschema_description:"Epic number, e.g. EP-1"`
}

type teamByNameArgs struct {
	TeamName string `json:"team_name" jsonschema_description:"Team name"`
}

type userByTelegramIDArgs struct {
	TelegramUsername string `json:"telegram_username" jsonschema_description:"Telegram username without @"`
}

type listEpicsArgs struct {
	Status string `json:"status" jsonschema_description:"Filter by status: NEW, SCORING, SCORED, or empty for all"`
}

type teamEpicsArgs struct {
	TeamName string `json:"team_name" jsonschema_description:"Team name"`
	Status   string `json:"status" jsonschema_description:"Filter by status: NEW, SCORING, SCORED, or empty for all"`
}

type userEpicArgs struct {
	TelegramUsername string `json:"telegram_username" jsonschema_description:"Telegram username without @"`
	EpicNumber       string `json:"epic_number" jsonschema_description:"Epic number, e.g. EP-1"`
}

type userTeamArgs struct {
	TelegramUsername string `json:"telegram_username" jsonschema_description:"Telegram username without @"`
	TeamName         string `json:"team_name" jsonschema_description:"Team name"`
}

type teamRoleArgs struct {
	TeamName string `json:"team_name" jsonschema_description:"Team name"`
	RoleName string `json:"role_name" jsonschema_description:"Role name, e.g. Аналитик, Разработчик"`
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
			"Get the scoring status of an epic: who has scored it and who hasn't",
			epicByNumberArgs{},
		},
		{
			"list_epics",
			"List all epics, optionally filtered by status (NEW, SCORING, SCORED)",
			listEpicsArgs{},
		},
		{
			"get_team_members",
			"Get all members of a team with their roles",
			teamByNameArgs{},
		},
		{
			"get_scoring_results",
			"Get the final scoring results for an epic (weighted averages per role and final score)",
			epicByNumberArgs{},
		},
		{
			"get_user_info",
			"Get information about a user: their role, teams, and weight",
			userByTelegramIDArgs{},
		},
		{
			"list_risks",
			"Get all risks for an epic with their statuses and scores",
			epicByNumberArgs{},
		},
		// ── New tools ──
		{
			"list_teams",
			"List all teams",
			emptyArgs{},
		},
		{
			"list_users",
			"List all registered users with their roles",
			emptyArgs{},
		},
		{
			"list_roles",
			"List all available roles in the system",
			emptyArgs{},
		},
		{
			"get_team_epics",
			"Get epics for a specific team, optionally filtered by status (NEW, SCORING, SCORED)",
			teamEpicsArgs{},
		},
		{
			"get_unscored_epics",
			"Get epics that a specific user has not yet finished scoring (effort or risks) in a team",
			userTeamArgs{},
		},
		{
			"get_unscored_risks",
			"Get risks for an epic that a user has not yet scored",
			userEpicArgs{},
		},
		{
			"get_epic_individual_scores",
			"Get individual epic scores from each user (not aggregated, per-person scores)",
			epicByNumberArgs{},
		},
		{
			"get_risk_individual_scores",
			"Get individual risk scores (probability and impact) from each user for all risks of an epic",
			epicByNumberArgs{},
		},
		{
			"check_user_scored_epic",
			"Check if a specific user has already scored the effort of an epic",
			userEpicArgs{},
		},
		{
			"get_users_who_scored_risk",
			"Get users who have submitted risk scores for all risks of an epic",
			epicByNumberArgs{},
		},
		{
			"get_users_by_role_in_team",
			"Get members of a team filtered by a specific role",
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
		user, err := repo.FindUserByTelegramID(ctx, args.TelegramUsername)
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
		user, err := repo.FindUserByTelegramID(ctx, args.TelegramUsername)
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
		user, err := repo.FindUserByTelegramID(ctx, args.TelegramUsername)
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
		user, err := repo.FindUserByTelegramID(ctx, args.TelegramUsername)
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
