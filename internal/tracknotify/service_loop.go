package tracknotify

import (
	"context"
	"time"
)

func (s *Service) Run(ctx context.Context) {
	if s == nil || s.database == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	s.runOnce(ctx)
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runOnce(ctx)
		}
	}
}

func (s *Service) runOnce(parent context.Context) {
	ctx := parent
	cancel := func() {}
	if s.loopTimeout > 0 {
		ctx, cancel = context.WithTimeout(parent, s.loopTimeout)
	}
	defer cancel()

	now := time.Now().UTC()
	if _, err := s.database.CleanupTrackMatchNotifications(ctx, now.AddDate(0, 0, -defaultRetentionDays)); err != nil {
		s.logger.Warn("Failed to cleanup track notifications", "error", err)
	}

	targets, err := s.database.ListTrackNotificationTargets(ctx)
	if err != nil {
		s.logger.Error("Failed to list track notification targets", "error", err)
		return
	}
	s.logger.Debug("=== Track notify tick ===", "targets", len(targets))

	activeMatches := s.buildActiveMatches(ctx, targets)
	s.logger.Debug("Track notify active matches", "count", len(activeMatches))
	s.publishLiveEmbeds(ctx, activeMatches)
	s.publishPostEmbeds(ctx, activeMatches, targets)
	s.logger.Debug("=== Track notify tick ended ===")
}
