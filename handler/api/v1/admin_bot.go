package v1

import (
"context"
"fmt"
"strconv"

"github.com/clawhost/clawhost/model"
"github.com/clawhost/clawhost/service/k8s"
"github.com/clawhost/clawhost/util"
"github.com/labstack/echo/v4"
)

// AdminCreateBot creates a new bot (admin only)
func AdminCreateBot(c echo.Context) error {
var req struct {
AppID  string `json:"app_id"`
UserID string `json:"user_id"`
Name   string `json:"name"`
Slug   string `json:"slug"`
}
if err := c.Bind(&req); err != nil {
return util.BadRequest(c, "invalid request body")
}
if req.Name == "" {
return util.BadRequest(c, "name is required")
}
if req.AppID == "" {
return util.BadRequest(c, "app_id is required")
}
if req.UserID == "" {
req.UserID = "admin"
}

bot := &model.Bot{
AppID:  req.AppID,
UserID: req.UserID,
Name:   req.Name,
Slug:   req.Slug,
Status: model.BotStatusCreated,
}

if err := model.CreateBot(bot); err != nil {
return util.InternalError(c, "failed to create bot")
}

return util.Success(c, bot)
}

// AdminListBots lists all bots across all apps (admin only)
func AdminListBots(c echo.Context) error {
bots, err := model.ListAllBots()
if err != nil {
return util.InternalError(c, "failed to list bots")
}

ctx := context.Background()
responses := make([]*GetBotResponse, 0, len(bots))
for _, bot := range bots {
if bot == nil {
continue
}
resp, buildErr := buildBotResponse(ctx, bot)
if buildErr != nil {
resp = &GetBotResponse{Bot: bot}
enrichBotResponseFromConfig(resp, bot)
}
responses = append(responses, resp)
}
return util.Success(c, responses)
}

// AdminStartBot starts a bot by ID (admin only)
func AdminStartBot(c echo.Context) error {
botID := c.Param("id")
bot, err := model.GetBotByID(botID)
if err != nil {
return util.NotFound(c, "bot not found")
}

if bot.Status == model.BotStatusRunning {
return util.BadRequest(c, "bot is already running")
}

ctx := context.Background()
openclawConfig, _ := bot.GetOpenClawConfig()
k8sConfig := convertToK8sConfig(bot, openclawConfig)

if err := k8s.CreateDeployment(ctx, bot.ID, bot.UserID, bot.AccessToken, k8sConfig); err != nil {
return util.InternalError(c, "failed to create deployment: "+err.Error())
}

endpoint, err := k8s.CreateService(ctx, bot.ID, bot.UserID)
if err != nil {
k8s.DeleteDeployment(ctx, bot.ID)
return util.InternalError(c, "failed to create service: "+err.Error())
}

if err := model.UpdateBotStatus(bot.ID, model.BotStatusStarting, endpoint); err != nil {
return util.InternalError(c, "failed to update bot status")
}

go func() {
bgCtx := context.Background()
_, err := k8s.WaitForPodReady(bgCtx, bot.ID, 120)
if err != nil {
model.UpdateBotStatus(bot.ID, model.BotStatusError, endpoint)
return
}
model.UpdateBotStatus(bot.ID, model.BotStatusRunning, endpoint)
if k8sConfig.AccessToken != "" {
k8s.WriteConfigToBot(bgCtx, bot.ID, k8sConfig, false)
}
}()

bot.Status = model.BotStatusStarting
bot.Endpoint = endpoint
return util.Success(c, bot)
}

// AdminStopBot stops a bot by ID (admin only)
func AdminStopBot(c echo.Context) error {
botID := c.Param("id")
bot, err := model.GetBotByID(botID)
if err != nil {
return util.NotFound(c, "bot not found")
}

if bot.Status != model.BotStatusRunning {
return util.BadRequest(c, "bot is not running")
}

ctx := context.Background()
if err := k8s.DeleteDeployment(ctx, bot.ID); err != nil {
return util.InternalError(c, "failed to delete deployment: "+err.Error())
}
if err := k8s.DeleteService(ctx, bot.ID); err != nil {
return util.InternalError(c, "failed to delete service: "+err.Error())
}

if err := model.UpdateBotStatus(bot.ID, model.BotStatusStopped, ""); err != nil {
return util.InternalError(c, "failed to update bot status")
}

bot.Status = model.BotStatusStopped
bot.Endpoint = ""
return util.Success(c, bot)
}

// AdminDeleteBot deletes a bot by ID (admin only)
func AdminDeleteBot(c echo.Context) error {
botID := c.Param("id")
bot, err := model.GetBotByID(botID)
if err != nil {
return util.NotFound(c, "bot not found")
}

ctx := context.Background()
if bot.Status == model.BotStatusRunning {
k8s.DeleteDeployment(ctx, bot.ID)
k8s.DeleteService(ctx, bot.ID)
}

if err := model.DeleteBot(bot.ID); err != nil {
return util.InternalError(c, "failed to delete bot")
}

return util.Success(c, map[string]string{"message": "bot deleted"})
}

type AdminGetBotDebugResponse struct {
Bot  *GetBotResponse `json:"bot"`
Pod  *k8s.PodInfo    `json:"pod,omitempty"`
Logs string          `json:"logs,omitempty"`
Tail int64           `json:"tail"`
}

type AdminSetBotProviderRequest struct {
ProviderName string `json:"provider_name"`
}

// AdminGetBotDebug returns current bot status, latest pod info, and recent logs.
func AdminGetBotDebug(c echo.Context) error {
botID := c.Param("id")
bot, err := model.GetBotByID(botID)
if err != nil {
return util.NotFound(c, "bot not found")
}

tail := int64(200)
if raw := c.QueryParam("tail"); raw != "" {
parsed, err := strconv.ParseInt(raw, 10, 64)
if err != nil || parsed <= 0 {
return util.BadRequest(c, "tail must be a positive integer")
}
if parsed > 2000 {
parsed = 2000
}
tail = parsed
}

resp, err := buildBotResponse(context.Background(), bot)
if err != nil {
return util.InternalError(c, "failed to build bot response")
}

debugResp := &AdminGetBotDebugResponse{
Bot:  resp,
Tail: tail,
}

pod, err := k8s.GetLatestPodInfo(context.Background(), bot.ID)
if err == nil && pod != nil {
debugResp.Pod = pod
logs, logErr := k8s.GetPodLogs(context.Background(), pod.Name, pod.Container, tail)
if logErr == nil {
debugResp.Logs = logs
}
}

return util.Success(c, debugResp)
}

// AdminSetBotProvider sets a configured provider for a specific bot.
func AdminSetBotProvider(c echo.Context) error {
botID := c.Param("id")
bot, err := model.GetBotByID(botID)
if err != nil {
return util.NotFound(c, "bot not found")
}

var req AdminSetBotProviderRequest
if err := c.Bind(&req); err != nil {
return util.BadRequest(c, "invalid request body")
}
if req.ProviderName == "" {
return util.BadRequest(c, "provider_name is required")
}

entry, ok := findProvisionProviderEntry(req.ProviderName)
if !ok {
return util.NotFound(c, "provider not found")
}

openclawConfig, err := bot.GetOpenClawConfig()
if err != nil || openclawConfig == nil {
openclawConfig = &model.OpenClawConfig{}
}
if err := applyProvisionProviderEntryToBot(bot, openclawConfig, entry); err != nil {
return util.InternalError(c, err.Error())
}
if bot.Status == model.BotStatusRunning {
if err := k8s.SyncSectionsToPod(context.Background(), bot.ID, "models", "agents"); err != nil {
return util.InternalError(c, "failed to sync bot config: "+err.Error())
}
}

resp, err := buildBotResponse(context.Background(), bot)
if err != nil {
return util.InternalError(c, "failed to build bot response")
}
return util.Success(c, resp)
}

func findProvisionProviderEntry(name string) (k8s.ProvisionProviderEntry, bool) {
settings := k8s.GetProvisionSettings()
for _, provider := range settings.Providers {
if provider.Name == name {
return provider, true
}
}
if settings.Model.Provider == name {
return k8s.ProvisionProviderEntry{
Name:      settings.Model.Provider,
ID:        settings.Model.ID,
BaseURL:   settings.Model.BaseURL,
API:       settings.Model.API,
Auth:      settings.Model.Auth,
APIKey:    settings.Model.APIKey,
IsDefault: true,
}, true
}
return k8s.ProvisionProviderEntry{}, false
}

func applyProvisionProviderEntryToBot(bot *model.Bot, openclawConfig *model.OpenClawConfig, entry k8s.ProvisionProviderEntry) error {
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
openclawConfig.Models.Providers[entry.Name] = &model.ProviderConfig{
BaseURL: entry.BaseURL,
APIKey:  entry.APIKey,
Auth:    entry.Auth,
API:     entry.API,
Models: []model.ProviderModelConfig{{
ID:            entry.ID,
Name:          entry.ID,
Input:         []string{"text"},
ContextWindow: 200000,
MaxTokens:     8192,
}},
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
openclawConfig.Agents.Defaults.Model.Primary = fmt.Sprintf("%s/%s", entry.Name, entry.ID)
if err := bot.SetOpenClawConfig(openclawConfig); err != nil {
return fmt.Errorf("failed to set config for bot %s: %w", bot.ID, err)
}
if err := model.UpdateBot(bot); err != nil {
return fmt.Errorf("failed to update bot %s: %w", bot.ID, err)
}
return nil
}
