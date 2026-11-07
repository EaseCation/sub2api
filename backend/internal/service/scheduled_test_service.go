package service

import (
	"context"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

var scheduledTestCronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// ScheduledTestService provides CRUD operations for scheduled test plans and results.
type ScheduledTestService struct {
	planRepo    ScheduledTestPlanRepository
	resultRepo  ScheduledTestResultRepository
	accountRepo AccountRepository
}

// NewScheduledTestService creates a new ScheduledTestService.
func NewScheduledTestService(
	planRepo ScheduledTestPlanRepository,
	resultRepo ScheduledTestResultRepository,
	accountRepo AccountRepository,
) *ScheduledTestService {
	return &ScheduledTestService{
		planRepo:    planRepo,
		resultRepo:  resultRepo,
		accountRepo: accountRepo,
	}
}

// CreatePlan validates the cron expression, computes next_run_at, and persists the plan.
func (s *ScheduledTestService) CreatePlan(ctx context.Context, plan *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	if err := s.validateMode(ctx, plan); err != nil {
		return nil, err
	}
	if plan.Mode == ScheduledTestModeClaudeWindow {
		existing, err := s.planRepo.ListByAccountID(ctx, plan.AccountID)
		if err != nil {
			return nil, err
		}
		for _, item := range existing {
			if item.Mode == ScheduledTestModeClaudeWindow {
				return nil, fmt.Errorf("this account already has a Claude window startup plan; edit or enable it instead")
			}
		}
	}
	nextRun, err := computeNextRun(plan.CronExpression, time.Now())
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}
	plan.NextRunAt = &nextRun

	if plan.MaxResults <= 0 {
		plan.MaxResults = 50
	}

	return s.planRepo.Create(ctx, plan)
}

// GetPlan retrieves a plan by ID.
func (s *ScheduledTestService) GetPlan(ctx context.Context, id int64) (*ScheduledTestPlan, error) {
	return s.planRepo.GetByID(ctx, id)
}

// ListPlansByAccount returns all plans for a given account.
func (s *ScheduledTestService) ListPlansByAccount(ctx context.Context, accountID int64) ([]*ScheduledTestPlan, error) {
	return s.planRepo.ListByAccountID(ctx, accountID)
}

// UpdatePlan validates cron and updates the plan.
func (s *ScheduledTestService) UpdatePlan(ctx context.Context, plan *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	if err := s.validateMode(ctx, plan); err != nil {
		return nil, err
	}
	// Preserve the persisted window deadline when editing or re-enabling a plan.
	if plan.Mode == ScheduledTestModeClaudeWindow && plan.NextRunAt != nil {
		return s.planRepo.Update(ctx, plan)
	}
	nextRun, err := computeNextRun(plan.CronExpression, time.Now())
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}
	plan.NextRunAt = &nextRun

	return s.planRepo.Update(ctx, plan)
}

func (s *ScheduledTestService) validateMode(ctx context.Context, plan *ScheduledTestPlan) error {
	if plan.Mode == "" {
		plan.Mode = ScheduledTestModeConnectivity
	}
	switch plan.Mode {
	case ScheduledTestModeConnectivity:
		return nil
	case ScheduledTestModeClaudeWindow:
		account, err := s.accountRepo.GetByID(ctx, plan.AccountID)
		if err != nil {
			return err
		}
		// Setup tokens cannot query weekly usage, which must be known before sending.
		if !supportsClaudeWindowStartup(account) {
			return fmt.Errorf("claude window startup requires an Anthropic OAuth account with usage access")
		}
		if plan.ModelID == "" {
			return fmt.Errorf("a model is required for Claude window startup")
		}
		plan.CronExpression = "* * * * *"
		plan.AutoRecover = false
		return nil
	default:
		return fmt.Errorf("invalid scheduled test mode: %q", plan.Mode)
	}
}

// DeletePlan removes a plan and its results (via CASCADE).
func (s *ScheduledTestService) DeletePlan(ctx context.Context, id int64) error {
	return s.planRepo.Delete(ctx, id)
}

// ListResults returns the most recent results for a plan.
func (s *ScheduledTestService) ListResults(ctx context.Context, planID int64, limit int) ([]*ScheduledTestResult, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.resultRepo.ListByPlanID(ctx, planID, limit)
}

// SaveResult inserts a result and prunes old entries beyond maxResults.
func (s *ScheduledTestService) SaveResult(ctx context.Context, planID int64, maxResults int, result *ScheduledTestResult) error {
	result.PlanID = planID
	if _, err := s.resultRepo.Create(ctx, result); err != nil {
		return err
	}
	return s.resultRepo.PruneOldResults(ctx, planID, maxResults)
}

func computeNextRun(cronExpr string, from time.Time) (time.Time, error) {
	sched, err := scheduledTestCronParser.Parse(cronExpr)
	if err != nil {
		return time.Time{}, err
	}
	return sched.Next(from), nil
}
