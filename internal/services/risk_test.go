package services

import (
	"context"
	"errors"
	"testing"

	"EpicScoreBot/internal/models/domain"

	"github.com/google/uuid"
)

func TestRiskService(t *testing.T) {
	ctx := context.Background()
	log := newDiscardLogger()
	epicID := uuid.New()
	riskID := uuid.New()
	userID := uuid.New()
	dbErr := errors.New("database error")

	t.Run("CreateRisk трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			CreateRiskFunc: func(ctx context.Context, description string, eID uuid.UUID) (*domain.Risk, error) {
				called = true
				if description != "Test Risk" || eID != epicID {
					t.Errorf("неверные параметры: desc=%s, eID=%v", description, eID)
				}
				return &domain.Risk{ID: riskID, Description: description, EpicID: eID}, nil
			},
		}

		s := NewRiskService(log, repo)
		risk, err := s.CreateRisk(ctx, "Test Risk", epicID)
		if err != nil || !called || risk == nil || risk.ID != riskID {
			t.Errorf("ошибка трансляции CreateRisk: err=%v, called=%t, risk=%v", err, called, risk)
		}

		// Тест ошибки
		repo.CreateRiskFunc = func(ctx context.Context, description string, eID uuid.UUID) (*domain.Risk, error) {
			return nil, dbErr
		}
		_, err = s.CreateRisk(ctx, "Test Risk", epicID)
		if !errors.Is(err, dbErr) {
			t.Errorf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
	})

	t.Run("GetRisksByEpicID трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			GetRisksByEpicIDFunc: func(ctx context.Context, eID uuid.UUID) ([]domain.Risk, error) {
				called = true
				if eID != epicID {
					t.Errorf("неверный eID: %v", eID)
				}
				return []domain.Risk{{ID: riskID, EpicID: eID}}, nil
			},
		}

		s := NewRiskService(log, repo)
		risks, err := s.GetRisksByEpicID(ctx, epicID)
		if err != nil || !called || len(risks) != 1 || risks[0].ID != riskID {
			t.Errorf("ошибка трансляции GetRisksByEpicID: err=%v, called=%t, risks=%v", err, called, risks)
		}

		// Тест ошибки
		repo.GetRisksByEpicIDFunc = func(ctx context.Context, eID uuid.UUID) ([]domain.Risk, error) {
			return nil, dbErr
		}
		_, err = s.GetRisksByEpicID(ctx, epicID)
		if !errors.Is(err, dbErr) {
			t.Errorf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
	})

	t.Run("GetRiskByID трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			GetRiskByIDFunc: func(ctx context.Context, rID uuid.UUID) (*domain.Risk, error) {
				called = true
				if rID != riskID {
					t.Errorf("неверный rID: %v", rID)
				}
				return &domain.Risk{ID: rID}, nil
			},
		}

		s := NewRiskService(log, repo)
		risk, err := s.GetRiskByID(ctx, riskID)
		if err != nil || !called || risk == nil || risk.ID != riskID {
			t.Errorf("ошибка трансляции GetRiskByID: err=%v, called=%t, risk=%v", err, called, risk)
		}

		// Тест ошибки
		repo.GetRiskByIDFunc = func(ctx context.Context, rID uuid.UUID) (*domain.Risk, error) {
			return nil, dbErr
		}
		_, err = s.GetRiskByID(ctx, riskID)
		if !errors.Is(err, dbErr) {
			t.Errorf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
	})

	t.Run("GetUnscoredRisksByUser трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			GetUnscoredRisksByUserFunc: func(ctx context.Context, uID, eID uuid.UUID) ([]domain.Risk, error) {
				called = true
				if uID != userID || eID != epicID {
					t.Errorf("неверные параметры: uID=%v, eID=%v", uID, eID)
				}
				return []domain.Risk{{ID: riskID}}, nil
			},
		}

		s := NewRiskService(log, repo)
		risks, err := s.GetUnscoredRisksByUser(ctx, userID, epicID)
		if err != nil || !called || len(risks) != 1 || risks[0].ID != riskID {
			t.Errorf("ошибка трансляции GetUnscoredRisksByUser: err=%v, called=%t, risks=%v", err, called, risks)
		}

		// Тест ошибки
		repo.GetUnscoredRisksByUserFunc = func(ctx context.Context, uID, eID uuid.UUID) ([]domain.Risk, error) {
			return nil, dbErr
		}
		_, err = s.GetUnscoredRisksByUser(ctx, userID, epicID)
		if !errors.Is(err, dbErr) {
			t.Errorf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
	})

	t.Run("UpdateRiskStatus трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			UpdateRiskStatusFunc: func(ctx context.Context, rID uuid.UUID, status domain.Status) error {
				called = true
				if rID != riskID || status != domain.StatusScored {
					t.Errorf("неверные параметры: rID=%v, status=%s", rID, status)
				}
				return nil
			},
		}

		s := NewRiskService(log, repo)
		err := s.UpdateRiskStatus(ctx, riskID, domain.StatusScored)
		if err != nil || !called {
			t.Errorf("ошибка трансляции UpdateRiskStatus: err=%v, called=%t", err, called)
		}

		// Тест ошибки
		repo.UpdateRiskStatusFunc = func(ctx context.Context, rID uuid.UUID, status domain.Status) error {
			return dbErr
		}
		err = s.UpdateRiskStatus(ctx, riskID, domain.StatusScored)
		if !errors.Is(err, dbErr) {
			t.Errorf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
	})

	t.Run("DeleteRisk трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			DeleteRiskFunc: func(ctx context.Context, rID uuid.UUID) error {
				called = true
				if rID != riskID {
					t.Errorf("неверный rID: %v", rID)
				}
				return nil
			},
		}

		s := NewRiskService(log, repo)
		err := s.DeleteRisk(ctx, riskID)
		if err != nil || !called {
			t.Errorf("ошибка трансляции DeleteRisk: err=%v, called=%t", err, called)
		}

		// Тест ошибки
		repo.DeleteRiskFunc = func(ctx context.Context, rID uuid.UUID) error {
			return dbErr
		}
		err = s.DeleteRisk(ctx, riskID)
		if !errors.Is(err, dbErr) {
			t.Errorf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
	})

	t.Run("CreateRiskScore трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			CreateRiskScoreFunc: func(ctx context.Context, rID, uID uuid.UUID, prob, imp int) error {
				called = true
				if rID != riskID || uID != userID || prob != 3 || imp != 4 {
					t.Errorf("неверные параметры: rID=%v, uID=%v, prob=%d, imp=%d", rID, uID, prob, imp)
				}
				return nil
			},
		}

		s := NewRiskService(log, repo)
		err := s.CreateRiskScore(ctx, riskID, userID, 3, 4)
		if err != nil || !called {
			t.Errorf("ошибка трансляции CreateRiskScore: err=%v, called=%t", err, called)
		}

		// Тест ошибки
		repo.CreateRiskScoreFunc = func(ctx context.Context, rID, uID uuid.UUID, prob, imp int) error {
			return dbErr
		}
		err = s.CreateRiskScore(ctx, riskID, userID, 3, 4)
		if !errors.Is(err, dbErr) {
			t.Errorf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
	})

	t.Run("GetRiskScoresByRiskID трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			GetRiskScoresByRiskIDFunc: func(ctx context.Context, rID uuid.UUID) ([]domain.RiskScore, error) {
				called = true
				if rID != riskID {
					t.Errorf("неверный rID: %v", rID)
				}
				return []domain.RiskScore{{RiskID: rID, UserID: userID, Probability: 3, Impact: 4}}, nil
			},
		}

		s := NewRiskService(log, repo)
		scores, err := s.GetRiskScoresByRiskID(ctx, riskID)
		if err != nil || !called || len(scores) != 1 || scores[0].RiskID != riskID {
			t.Errorf("ошибка трансляции GetRiskScoresByRiskID: err=%v, called=%t, scores=%v", err, called, scores)
		}

		// Тест ошибки
		repo.GetRiskScoresByRiskIDFunc = func(ctx context.Context, rID uuid.UUID) ([]domain.RiskScore, error) {
			return nil, dbErr
		}
		_, err = s.GetRiskScoresByRiskID(ctx, riskID)
		if !errors.Is(err, dbErr) {
			t.Errorf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
	})

	t.Run("GetUsersWhoScoredRisk трансляция", func(t *testing.T) {
		called := false
		repo := &MockRepository{
			GetUsersWhoScoredRiskFunc: func(ctx context.Context, rID uuid.UUID) ([]domain.User, error) {
				called = true
				if rID != riskID {
					t.Errorf("неверный rID: %v", rID)
				}
				return []domain.User{{ID: userID}}, nil
			},
		}

		s := NewRiskService(log, repo)
		users, err := s.GetUsersWhoScoredRisk(ctx, riskID)
		if err != nil || !called || len(users) != 1 || users[0].ID != userID {
			t.Errorf("ошибка трансляции GetUsersWhoScoredRisk: err=%v, called=%t, users=%v", err, called, users)
		}

		// Тест ошибки
		repo.GetUsersWhoScoredRiskFunc = func(ctx context.Context, rID uuid.UUID) ([]domain.User, error) {
			return nil, dbErr
		}
		_, err = s.GetUsersWhoScoredRisk(ctx, riskID)
		if !errors.Is(err, dbErr) {
			t.Errorf("ожидалась ошибка %v, получена: %v", dbErr, err)
		}
	})
}
