package service

import (
	"context"
	"encoding/json"
	"errors"
)

func (s *SettingService) GetAffiliateRewardProgramConfig(ctx context.Context) AffiliateRewardProgramConfig {
	fallback := DefaultAffiliateRewardProgramConfig()
	if s == nil || s.settingRepo == nil {
		return fallback
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyAffiliateRewardProgramConfig)
	if err != nil {
		return fallback
	}
	var config AffiliateRewardProgramConfig
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return fallback
	}
	normalized, err := NormalizeAffiliateRewardProgramConfig(config)
	if err != nil {
		return fallback
	}
	return normalized
}

func (s *SettingService) SetAffiliateRewardProgramConfig(ctx context.Context, config AffiliateRewardProgramConfig) (AffiliateRewardProgramConfig, error) {
	if s == nil || s.settingRepo == nil {
		return AffiliateRewardProgramConfig{}, errors.New("setting service unavailable")
	}
	normalized, err := NormalizeAffiliateRewardProgramConfig(config)
	if err != nil {
		return AffiliateRewardProgramConfig{}, err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return AffiliateRewardProgramConfig{}, err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyAffiliateRewardProgramConfig, string(encoded)); err != nil {
		return AffiliateRewardProgramConfig{}, err
	}
	if s.onUpdate != nil {
		s.onUpdate()
	}
	return normalized, nil
}
