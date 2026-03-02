package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"EpicScoreBot/internal/repositories"
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

type epicRisksArgs struct {
	EpicNumber string `json:"epic_number" jsonschema_description:"Epic number, e.g. EP-1"`
}

type scoringResultsArgs struct {
	EpicNumber string `json:"epic_number" jsonschema_description:"Epic number, e.g. EP-1"`
}

type listEpicsArgs struct {
	Status string `json:"status" jsonschema_description:"Filter by status: NEW, SCORING, SCORED, or empty for all"`
}

// ─── Tool definitions ──────────────────────────────────────────────────────

func buildTools() ([]openrouter.Tool, error) {
	tools := []struct {
		name        string
		description string
		schema      any
	}{
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
			"Get all members of a team with their roles and weights",
			teamByNameArgs{},
		},
		{
			"get_scoring_results",
			"Get the final scoring results for an epic (weighted averages per role and final score)",
			scoringResultsArgs{},
		},
		{
			"get_user_info",
			"Get information about a user: their role, teams, and weight",
			userByTelegramIDArgs{},
		},
		{
			"list_risks",
			"Get all risks for an epic with their statuses and scores",
			epicRisksArgs{},
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
func executeTool(ctx context.Context, repo *repositories.Repository, name, argsJSON string) (string, error) {
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
			Weight   int    `json:"weight"`
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
				Weight:   u.Weight,
			})
		}
		result := map[string]any{"team": team.Name, "members": rows}
		b, _ := json.Marshal(result)
		return string(b), nil

	case "get_scoring_results":
		var args scoringResultsArgs
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
		var args epicRisksArgs
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

	default:
		return fmt.Sprintf(`{"error":"unknown tool %q"}`, name), nil
	}
}
