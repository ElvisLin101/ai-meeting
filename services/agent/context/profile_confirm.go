package agentctx

import (
	"context"
	"fmt"
	"strings"

	"ai-meeting/models"
)

// ListPendingProfileConfirmations 查询当前用户待确认的画像项。
func ListPendingProfileConfirmations(ctx context.Context, userID string) ([]models.ProfileConfirmation, error) {
	if userID == "" {
		return nil, nil
	}
	return currentRepository().ListPendingProfileConfirmations(userID, 50)
}

// ResolveProfileConfirmation 处理用户确认/拒绝。
//
// confirm: 把确认项写成 active fact（Force=true，允许覆盖同主题旧值），
//
//	用 version CAS 写 MySQL，成功后把确认项标 confirmed。
//
// reject:  只标记 rejected，不改画像。
func ResolveProfileConfirmation(ctx context.Context, userID string, id uint, action string) error {
	if userID == "" {
		return fmt.Errorf("missing user")
	}
	action = strings.ToLower(strings.TrimSpace(action))
	if action != "confirm" && action != "reject" {
		return fmt.Errorf("action must be confirm or reject")
	}

	confirmation, err := currentRepository().FindProfileConfirmation(id, userID)
	if err != nil {
		return err
	}
	if confirmation == nil || confirmation.Status != "pending" {
		return fmt.Errorf("confirmation not found or already resolved")
	}

	if action == "reject" {
		ok, err := currentRepository().MarkProfileConfirmation(id, userID, "rejected")
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("confirmation already resolved")
		}
		return nil
	}

	profile := LoadProfile(ctx, userID)
	isNew := profile == nil || profile.Version <= 0
	if profile == nil {
		profile = &UserProfileData{Version: 1}
	}
	NormalizeProfile(profile)

	diff := ProfileDiff{
		Reason: "user confirmed profile fact",
		Force:  true,
		Add: []ProfileFact{{
			Field: confirmation.Field, Key: confirmation.Key, Value: confirmation.Value,
			Confidence: 1.0, Source: "user_confirm", Quote: confirmation.Question,
		}},
	}
	normalizeProfileDiff(&diff)
	if len(diff.Add) == 0 {
		return fmt.Errorf("invalid confirmation fact")
	}
	next, _, _, err := ApplyProfileDiff(profile, diff)
	if err != nil {
		return err
	}
	next.LastConsolidatedSeq = profile.LastConsolidatedSeq
	if err := persistConsolidatedProfile(ctx, userID, next, isNew); err != nil {
		return err
	}
	ok, err := currentRepository().MarkProfileConfirmation(id, userID, "confirmed")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("confirmation already resolved")
	}
	return nil
}
