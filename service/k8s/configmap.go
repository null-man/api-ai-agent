package k8s

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/clawhost/clawhost/model"
	"github.com/spf13/viper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultClawhostConfigMapName  = "clawhost-config"
	defaultClawhostDeploymentName = "clawhost"
)

type ProvisionSettings struct {
	Model     ProvisionModelSettings    `json:"model"`
	Telegram  ProvisionTelegramSettings `json:"telegram"`
	Providers []ProvisionProviderEntry  `json:"providers,omitempty"`
}

type ProvisionModelSettings struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	BaseURL  string `json:"base_url"`
	API      string `json:"api"`
	Auth     string `json:"auth"`
	APIKey   string `json:"api_key"`
}

type ProvisionTelegramSettings struct {
	DMPolicy    string `json:"dm_policy"`
	GroupPolicy string `json:"group_policy"`
}

type ProvisionProviderEntry struct {
	Name      string `json:"name"`
	ID        string `json:"id"`
	BaseURL   string `json:"base_url"`
	API       string `json:"api"`
	Auth      string `json:"auth"`
	APIKey    string `json:"api_key"`
	IsDefault bool   `json:"is_default,omitempty"`
}

type ProviderTestResult struct {
	Success    bool   `json:"success"`
	StatusCode int    `json:"status_code"`
	Message    string `json:"message,omitempty"`
	RawBody    string `json:"raw_body,omitempty"`
}

func GetProvisionSettings() ProvisionSettings {
	settings := ProvisionSettings{
		Model: ProvisionModelSettings{
			Provider: viper.GetString("provision.model.provider"),
			ID:       viper.GetString("provision.model.id"),
			BaseURL:  viper.GetString("provision.model.base_url"),
			API:      viper.GetString("provision.model.api"),
			Auth:     viper.GetString("provision.model.auth"),
			APIKey:   viper.GetString("provision.model.api_key"),
		},
		Telegram: ProvisionTelegramSettings{
			DMPolicy:    viper.GetString("provision.telegram.dm_policy"),
			GroupPolicy: viper.GetString("provision.telegram.group_policy"),
		},
	}

	settings.Providers = GetConfiguredProviders(settings.Model)
	return settings
}

func UpdateProvisionSettings(ctx context.Context, settings ProvisionSettings) error {
	client := GetClient()
	if client == nil {
		return fmt.Errorf("k8s client is not initialized")
	}

	previous := GetProvisionSettings()
	namespace := GetNamespace()
	configMap, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, defaultClawhostConfigMapName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get configmap: %w", err)
	}

	configToml := configMap.Data["config.toml"]
	configToml = upsertTomlValue(configToml, "provision.model", "provider", quoteToml(settings.Model.Provider))
	configToml = upsertTomlValue(configToml, "provision.model", "id", quoteToml(settings.Model.ID))
	configToml = upsertTomlValue(configToml, "provision.model", "base_url", quoteToml(settings.Model.BaseURL))
	configToml = upsertTomlValue(configToml, "provision.model", "api", quoteToml(settings.Model.API))
	configToml = upsertTomlValue(configToml, "provision.model", "auth", quoteToml(settings.Model.Auth))
	configToml = upsertTomlValue(configToml, "provision.model", "api_key", quoteToml(settings.Model.APIKey))
	configToml = upsertTomlValue(configToml, "provision.telegram", "dm_policy", quoteToml(settings.Telegram.DMPolicy))
	configToml = upsertTomlValue(configToml, "provision.telegram", "group_policy", quoteToml(settings.Telegram.GroupPolicy))

	configMap.Data["config.toml"] = ensureTrailingNewline(configToml)
	if _, err := client.CoreV1().ConfigMaps(namespace).Update(ctx, configMap, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update configmap: %w", err)
	}

	if err := syncExistingBotsToProvisionSettings(ctx, settings.Model, &previous.Model); err != nil {
		return err
	}

	if err := restartClawhostDeployment(ctx, namespace); err != nil {
		return err
	}

	return nil
}

func GetConfiguredProviders(current ProvisionModelSettings) []ProvisionProviderEntry {
	entries := make([]ProvisionProviderEntry, 0)
	seen := map[string]bool{}

	if rawProviders := viper.GetStringMap("provider_catalog"); len(rawProviders) > 0 {
		for name, rawValue := range rawProviders {
			providerMap, ok := rawValue.(map[string]interface{})
			if !ok {
				continue
			}
			entry := ProvisionProviderEntry{
				Name:      name,
				ID:        stringValue(providerMap["id"]),
				BaseURL:   stringValue(providerMap["base_url"]),
				API:       stringValue(providerMap["api"]),
				Auth:      stringValue(providerMap["auth"]),
				APIKey:    stringValue(providerMap["api_key"]),
				IsDefault: name == current.Provider,
			}
			entries = append(entries, entry)
			seen[name] = true
		}
	}

	if current.Provider != "" && !seen[current.Provider] {
		entries = append(entries, ProvisionProviderEntry{
			Name:      current.Provider,
			ID:        current.ID,
			BaseURL:   current.BaseURL,
			API:       current.API,
			Auth:      current.Auth,
			APIKey:    current.APIKey,
			IsDefault: true,
		})
	}

	return entries
}

func UpsertConfiguredProvider(ctx context.Context, entry ProvisionProviderEntry, setDefault bool, telegram ProvisionTelegramSettings) error {
	client := GetClient()
	if client == nil {
		return fmt.Errorf("k8s client is not initialized")
	}

	namespace := GetNamespace()
	configMap, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, defaultClawhostConfigMapName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get configmap: %w", err)
	}

	configToml := configMap.Data["config.toml"]
	previous := GetProvisionSettings()
	section := fmt.Sprintf("provider_catalog.%s", entry.Name)
	configToml = upsertTomlValue(configToml, section, "id", quoteToml(entry.ID))
	configToml = upsertTomlValue(configToml, section, "base_url", quoteToml(entry.BaseURL))
	configToml = upsertTomlValue(configToml, section, "api", quoteToml(entry.API))
	configToml = upsertTomlValue(configToml, section, "auth", quoteToml(entry.Auth))
	configToml = upsertTomlValue(configToml, section, "api_key", quoteToml(entry.APIKey))

	if setDefault {
		configToml = upsertTomlValue(configToml, "provision.model", "provider", quoteToml(entry.Name))
		configToml = upsertTomlValue(configToml, "provision.model", "id", quoteToml(entry.ID))
		configToml = upsertTomlValue(configToml, "provision.model", "base_url", quoteToml(entry.BaseURL))
		configToml = upsertTomlValue(configToml, "provision.model", "api", quoteToml(entry.API))
		configToml = upsertTomlValue(configToml, "provision.model", "auth", quoteToml(entry.Auth))
		configToml = upsertTomlValue(configToml, "provision.model", "api_key", quoteToml(entry.APIKey))
		configToml = upsertTomlValue(configToml, "provision.telegram", "dm_policy", quoteToml(telegram.DMPolicy))
		configToml = upsertTomlValue(configToml, "provision.telegram", "group_policy", quoteToml(telegram.GroupPolicy))
	}

	configMap.Data["config.toml"] = ensureTrailingNewline(configToml)
	if _, err := client.CoreV1().ConfigMaps(namespace).Update(ctx, configMap, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update configmap: %w", err)
	}

	if setDefault {
		modelSettings := ProvisionModelSettings{
			Provider: entry.Name,
			ID:       entry.ID,
			BaseURL:  entry.BaseURL,
			API:      entry.API,
			Auth:     entry.Auth,
			APIKey:   entry.APIKey,
		}
		if err := syncExistingBotsToProvisionSettings(ctx, modelSettings, &previous.Model); err != nil {
			return err
		}
	}

	return restartClawhostDeployment(ctx, namespace)
}

func TestConfiguredProvider(ctx context.Context, entry ProvisionProviderEntry) (*ProviderTestResult, error) {
	baseURL := strings.TrimRight(entry.BaseURL, "/")
	if baseURL == "" {
		return nil, fmt.Errorf("base_url is required")
	}
	if entry.ID == "" {
		return nil, fmt.Errorf("model id is required")
	}
	if entry.APIKey == "" {
		return nil, fmt.Errorf("api_key is required")
	}

	var (
		url     string
		payload map[string]interface{}
		headers = map[string]string{
			"Content-Type": "application/json",
		}
	)

	switch entry.API {
	case "openai-completions":
		url = baseURL + "/chat/completions"
		payload = map[string]interface{}{
			"model": entry.ID,
			"messages": []map[string]string{
				{"role": "user", "content": "ping"},
			},
			"max_tokens": 1,
		}
		headers["Authorization"] = "Bearer " + entry.APIKey
	case "openai-responses":
		url = baseURL + "/responses"
		payload = map[string]interface{}{
			"model":             entry.ID,
			"input":             "ping",
			"max_output_tokens": 1,
		}
		headers["Authorization"] = "Bearer " + entry.APIKey
	case "anthropic-messages":
		url = baseURL + "/messages"
		payload = map[string]interface{}{
			"model": entry.ID,
			"messages": []map[string]string{
				{"role": "user", "content": "ping"},
			},
			"max_tokens": 1,
		}
		headers["x-api-key"] = entry.APIKey
		headers["anthropic-version"] = "2023-06-01"
	default:
		return nil, fmt.Errorf("unsupported api type: %s", entry.API)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal test payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("provider test failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	result := &ProviderTestResult{
		Success:    resp.StatusCode >= 200 && resp.StatusCode < 300,
		StatusCode: resp.StatusCode,
		RawBody:    string(respBody),
	}
	if result.Success {
		result.Message = "provider test passed"
	} else {
		result.Message = "provider test failed"
	}
	return result, nil
}

func restartClawhostDeployment(ctx context.Context, namespace string) error {
	client := GetClient()
	if client == nil {
		return fmt.Errorf("k8s client is not initialized")
	}

	deployment, err := client.AppsV1().Deployments(namespace).Get(ctx, defaultClawhostDeploymentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = map[string]string{}
	}
	deployment.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().UTC().Format(time.RFC3339)

	if _, err := client.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to restart deployment: %w", err)
	}

	return nil
}

func upsertTomlValue(doc, section, key, value string) string {
	lines := strings.Split(doc, "\n")
	sectionHeader := "[" + section + "]"
	sectionStart := -1
	sectionEnd := len(lines)

	for i, line := range lines {
		if strings.TrimSpace(line) == sectionHeader {
			sectionStart = i
			for j := i + 1; j < len(lines); j++ {
				trimmed := strings.TrimSpace(lines[j])
				if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
					sectionEnd = j
					break
				}
			}
			break
		}
	}

	keyLine := fmt.Sprintf("%s = %s", key, value)

	if sectionStart == -1 {
		if strings.TrimSpace(doc) != "" && !strings.HasSuffix(doc, "\n") {
			doc += "\n"
		}
		if !strings.HasSuffix(doc, "\n\n") && strings.TrimSpace(doc) != "" {
			doc += "\n"
		}
		return doc + sectionHeader + "\n" + keyLine + "\n"
	}

	for i := sectionStart + 1; i < sectionEnd; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, key+" ") || strings.HasPrefix(trimmed, key+"=") {
			lines[i] = keyLine
			return strings.Join(lines, "\n")
		}
	}

	newLines := append([]string{}, lines[:sectionEnd]...)
	newLines = append(newLines, keyLine)
	newLines = append(newLines, lines[sectionEnd:]...)
	return strings.Join(newLines, "\n")
}

func quoteToml(value string) string {
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	return fmt.Sprintf("\"%s\"", escaped)
}

func ensureTrailingNewline(s string) string {
	if strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}

func stringValue(v interface{}) string {
	if value, ok := v.(string); ok {
		return value
	}
	return ""
}


func syncExistingBotsToProvisionSettings(ctx context.Context, modelSettings ProvisionModelSettings, previous *ProvisionModelSettings) error {
if modelSettings.Provider == "" || modelSettings.ID == "" {
return nil
}

bots, err := model.ListAllBots()
if err != nil {
return fmt.Errorf("failed to list bots for config sync: %w", err)
}

for _, bot := range bots {
if bot == nil || bot.Status == model.BotStatusDeleted {
continue
}

openclawConfig, err := bot.GetOpenClawConfig()
if err != nil || openclawConfig == nil {
openclawConfig = &model.OpenClawConfig{}
}
if !botMatchesProvisionModel(openclawConfig, previous) {
continue
}
if err := applyProvisionModelToBot(bot, openclawConfig, modelSettings); err != nil {
return err
}
if bot.Status == model.BotStatusRunning {
if err := SyncSectionsToPod(ctx, bot.ID, "models", "agents"); err != nil {
return fmt.Errorf("failed to sync running bot %s: %w", bot.ID, err)
}
}
}

return nil
}

func botMatchesProvisionModel(config *model.OpenClawConfig, previous *ProvisionModelSettings) bool {
if previous == nil || strings.TrimSpace(previous.Provider) == "" || strings.TrimSpace(previous.ID) == "" {
return true
}
if config == nil {
return true
}
primary := ""
if config.Agents != nil && config.Agents.Defaults != nil && config.Agents.Defaults.Model != nil {
primary = strings.TrimSpace(config.Agents.Defaults.Model.Primary)
}
if primary == "" {
return true
}
expectedPrimary := fmt.Sprintf("%s/%s", previous.Provider, previous.ID)
if primary != expectedPrimary {
return false
}
if config.Models == nil || config.Models.Providers == nil {
return true
}
providerConfig, ok := config.Models.Providers[previous.Provider]
if !ok || providerConfig == nil {
return true
}
if strings.TrimSpace(previous.APIKey) != "" && strings.TrimSpace(providerConfig.APIKey) != strings.TrimSpace(previous.APIKey) {
return false
}
return true
}

func applyProvisionModelToBot(bot *model.Bot, openclawConfig *model.OpenClawConfig, modelSettings ProvisionModelSettings) error {
if openclawConfig == nil {
openclawConfig = &model.OpenClawConfig{}
}
if openclawConfig.Models == nil {
openclawConfig.Models = &model.ModelsConfig{Mode: "merge", Providers: map[string]*model.ProviderConfig{}}
}
if openclawConfig.Models.Providers == nil {
openclawConfig.Models.Providers = map[string]*model.ProviderConfig{}
}
openclawConfig.Models.Mode = "merge"
openclawConfig.Models.Providers[modelSettings.Provider] = &model.ProviderConfig{
BaseURL: modelSettings.BaseURL,
APIKey:  modelSettings.APIKey,
Auth:    modelSettings.Auth,
API:     modelSettings.API,
Models: []model.ProviderModelConfig{{ID: modelSettings.ID, Name: modelSettings.ID, Input: []string{"text"}, ContextWindow: 200000, MaxTokens: 8192}},
}
if openclawConfig.Agents == nil {
openclawConfig.Agents = &model.AgentsConfig{}
}
if openclawConfig.Agents.Defaults == nil {
openclawConfig.Agents.Defaults = &model.AgentDefaultsConfig{}
}
if openclawConfig.Agents.Defaults.Model == nil {
openclawConfig.Agents.Defaults.Model = &model.AgentModelConfig{}
}
openclawConfig.Agents.Defaults.Model.Primary = fmt.Sprintf("%s/%s", modelSettings.Provider, modelSettings.ID)
if err := bot.SetOpenClawConfig(openclawConfig); err != nil {
return fmt.Errorf("failed to set config for bot %s: %w", bot.ID, err)
}
if err := model.UpdateBot(bot); err != nil {
return fmt.Errorf("failed to update bot %s: %w", bot.ID, err)
}
return nil
}
