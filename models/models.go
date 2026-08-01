package models

import (
	"strings"
)

type ModelConfig struct {
	ModelType       string // "default", "expert", or "vision"
	ThinkingEnabled bool
	SearchEnabled   bool
	DisplayName     string
}

func ResolveModel(requestedModel string, thinkingExplicitlyEnabled bool) ModelConfig {
	m := strings.ToLower(strings.TrimSpace(requestedModel))

	switch m {
	case "deepseek flash", "deepseek-flash", "flash", "haiku", "deepseek-chat", "default":
		return ModelConfig{
			ModelType:       "default",
			ThinkingEnabled: thinkingExplicitlyEnabled,
			SearchEnabled:   false,
			DisplayName:     "DeepSeek Flash",
		}
	case "deepseek flash thinking", "deepseek-flash-thinking", "deepseek-reasoner", "deepseek-r1", "reasoner", "r1":
		return ModelConfig{
			ModelType:       "default",
			ThinkingEnabled: true,
			SearchEnabled:   false,
			DisplayName:     "DeepSeek Flash Thinking",
		}
	case "deepseek pro", "deepseek-pro", "deepseek expert", "deepseek-expert", "pro", "expert", "opus":
		return ModelConfig{
			ModelType:       "expert",
			ThinkingEnabled: thinkingExplicitlyEnabled,
			SearchEnabled:   false,
			DisplayName:     "DeepSeek Pro",
		}
	case "deepseek pro thinking", "deepseek-pro-thinking", "deepseek expert thinking", "deepseek-expert-thinking", "deepseek-v4-pro", "v4-pro", "fable":
		return ModelConfig{
			ModelType:       "expert",
			ThinkingEnabled: true,
			SearchEnabled:   false,
			DisplayName:     "DeepSeek Pro Thinking",
		}
	case "deepseek vision", "deepseek-vision", "vision":
		return ModelConfig{
			ModelType:       "vision",
			ThinkingEnabled: thinkingExplicitlyEnabled,
			SearchEnabled:   false,
			DisplayName:     "DeepSeek Vision",
		}
	case "deepseek vision thinking", "deepseek-vision-thinking":
		return ModelConfig{
			ModelType:       "vision",
			ThinkingEnabled: true,
			SearchEnabled:   false,
			DisplayName:     "DeepSeek Vision Thinking",
		}
	default:
		if strings.Contains(m, "vision") {
			return ModelConfig{
				ModelType:       "vision",
				ThinkingEnabled: strings.Contains(m, "thinking") || thinkingExplicitlyEnabled,
				SearchEnabled:   false,
				DisplayName:     "DeepSeek Vision",
			}
		}
		if strings.Contains(m, "pro") || strings.Contains(m, "expert") || strings.Contains(m, "v4") {
			return ModelConfig{
				ModelType:       "expert",
				ThinkingEnabled: strings.Contains(m, "thinking") || thinkingExplicitlyEnabled,
				SearchEnabled:   false,
				DisplayName:     "DeepSeek Pro",
			}
		}
		return ModelConfig{
			ModelType:       "default",
			ThinkingEnabled: strings.Contains(m, "thinking") || strings.Contains(m, "reasoner") || thinkingExplicitlyEnabled,
			SearchEnabled:   false,
			DisplayName:     "DeepSeek Flash",
		}
	}
}

func ApplyEffortInstruction(effort string, prompt string) string {
	e := strings.ToLower(strings.TrimSpace(effort))
	var instruction string

	switch e {
	case "low":
		instruction = "[System Directive: Be extremely concise, direct, and brief. Minimize reasoning output and answer directly.]\n\n"
	case "medium":
		instruction = "[System Directive: Provide a balanced, moderate reasoning process before answering.]\n\n"
	case "high":
		return prompt
	case "xhigh", "max", "ultracode":
		instruction = "[System Directive: Perform maximum deep analysis. Thoroughly analyze architecture, edge cases, potential bugs, security, and performance optimizations before generating code.]\n\n"
	default:
		return prompt
	}

	return instruction + prompt
}
