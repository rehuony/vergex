package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
	"vergex/internal/agent"
	_ "vergex/internal/agent/provider"
	"vergex/internal/logger"
	"vergex/internal/trader"
	traderagent "vergex/internal/trader/agent"
	"vergex/internal/trader/kind"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// exceedsEveryModel refuses a config too large for any known model to read.
func exceedsEveryModel(config kind.Config) (*kind.TokenEstimate, bool) {
	agentConfig, ok := config.(*kind.AgentConfig)
	if !ok {
		return nil, false
	}
	estimate := agentConfig.EstimateTokens()
	for _, limit := range estimate.ModelLimits {
		if limit.UsagePct <= 100 {
			return nil, false
		}
	}
	return &estimate, len(estimate.ModelLimits) > 0
}

// handleTraderPresets lists the starting configs of a kind.
func (s *Server) handleTraderPresets(c *gin.Context) {
	name := c.DefaultQuery("kind", kind.Agent)
	presets, err := kind.Presets(name)
	if err != nil {
		SafeBadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"kind": name, "presets": presets})
}

// handleEstimateTokens estimates token usage for an agent config (pure computation)
func (s *Server) handleEstimateTokens(c *gin.Context) {
	var req struct {
		Config kind.AgentConfig `json:"config" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}
	c.JSON(http.StatusOK, req.Config.EstimateTokens())
}

// handleDuplicateTrader creates a stopped copy of a trader: its kind, config, model,
// account, interval and margin mode. What it has done stays behind, and its starting
// balance is read from the exchange again.
func (s *Server) handleDuplicateTrader(c *gin.Context) {
	userID := c.GetString("user_id")
	source, err := s.stores.Traders.Get(userID, c.Param("id"))
	if err != nil {
		SafeNotFound(c, "Trader")
		return
	}
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}
	copied := &trader.Trader{
		ID:                  uuid.NewString(),
		UserID:              userID,
		Name:                req.Name,
		Kind:                source.Kind,
		Config:              source.Config,
		AgentModelID:        source.AgentModelID,
		AccountID:           source.AccountID,
		ScanIntervalMinutes: source.ScanIntervalMinutes,
		MarginMode:          source.MarginMode,
	}
	if err := s.stores.Traders.Create(copied); err != nil {
		SafeInternalError(c, "Failed to duplicate trader", err)
		return
	}
	if err := s.traderManager.LoadUserTradersFromStore(userID); err != nil {
		logger.Infof("⚠️ Failed to load user traders into memory: %v", err)
	}
	c.JSON(http.StatusCreated, gin.H{"trader_id": copied.ID, "trader_name": copied.Name, "kind": copied.Kind})
}

// handleTraderDryRun builds the prompts a config would send on live public data and,
// with a model named, sends them. It never places an order.
func (s *Server) handleTraderDryRun(c *gin.Context) {
	userID := c.GetString("user_id")
	var req struct {
		Kind         string          `json:"kind"`
		Config       json.RawMessage `json:"config" binding:"required"`
		AgentModelID string          `json:"agent_model_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}
	if req.Kind == "" {
		req.Kind = kind.Agent
	}
	config, err := kind.Decode(req.Kind, req.Config)
	if err != nil {
		SafeBadRequest(c, err.Error())
		return
	}

	var systemPrompt, userPrompt string
	var candidates []traderagent.CandidateCoin
	switch cfg := config.(type) {
	case *kind.AgentConfig:
		systemPrompt, userPrompt, candidates, err = agentDryRunPrompts(cfg)
	case *kind.GridConfig:
		systemPrompt, userPrompt, candidates, err = gridDryRunPrompts(cfg)
	}
	if err != nil {
		SafeInternalError(c, "Failed to build the prompts", err)
		return
	}

	result := gin.H{
		"system_prompt":   systemPrompt,
		"user_prompt":     userPrompt,
		"candidate_count": len(candidates),
		"candidates":      candidates,
	}
	if req.AgentModelID != "" {
		response, err := s.callDryRunModel(c.Request.Context(), userID, req.AgentModelID, systemPrompt, userPrompt)
		if err != nil {
			result["agent_error"] = err.Error()
		} else {
			result["agent_response"] = response
		}
	}
	c.JSON(http.StatusOK, result)
}

func agentDryRunPrompts(config *kind.AgentConfig) (string, string, []traderagent.CandidateCoin, error) {
	const promptVariant = "balanced"
	engine := traderagent.NewEngine(config)

	candidates, err := engine.GetCandidateCoins()
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to get candidate coins: %w", err)
	}

	klines := config.Indicators.Klines
	timeframes := klines.SelectedTimeframes
	primaryTimeframe := klines.PrimaryTimeframe
	klineCount := klines.PrimaryCount
	if len(timeframes) == 0 {
		if primaryTimeframe != "" {
			timeframes = append(timeframes, primaryTimeframe)
		} else {
			timeframes = append(timeframes, "3m")
		}
		if klines.LongerTimeframe != "" {
			timeframes = append(timeframes, klines.LongerTimeframe)
		}
	}
	if primaryTimeframe == "" {
		primaryTimeframe = timeframes[0]
	}

	marketDataMap := make(map[string]*traderagent.Data)
	for _, coin := range candidates {
		data, err := traderagent.GetWithTimeframes(coin.Symbol, timeframes, primaryTimeframe, klineCount)
		if err != nil {
			logger.Warnf("⚠️  Failed to get market data for %s: %v", coin.Symbol, err)
			continue
		}
		marketDataMap[coin.Symbol] = data
	}

	context := &traderagent.Context{
		CurrentTime:    time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
		CallCount:      1,
		Account:        traderagent.AccountInfo{TotalEquity: 1000.0, AvailableBalance: 1000.0},
		Positions:      []traderagent.PositionInfo{},
		CandidateCoins: candidates,
		PromptVariant:  promptVariant,
		MarketDataMap:  marketDataMap,
		OITopDataMap:   engine.RankingContext(),
	}
	return engine.BuildSystemPrompt(1000.0, promptVariant), engine.BuildUserPrompt(context), candidates, nil
}

func gridDryRunPrompts(config *kind.GridConfig) (string, string, []traderagent.CandidateCoin, error) {
	data, err := traderagent.GetWithTimeframes(config.Symbol, []string{"5m", "4h"}, "5m", 50)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to get market data: %w", err)
	}
	context := traderagent.BuildGridContextFromMarketData(data, config)
	context.CurrentTime = time.Now().UTC().Format("2006-01-02 15:04:05 UTC")
	context.TotalEquity = 1000.0
	context.AvailableBalance = 1000.0
	candidates := []traderagent.CandidateCoin{{Symbol: config.Symbol, Sources: []string{kind.Grid}}}
	return traderagent.BuildGridSystemPrompt(config), traderagent.BuildGridUserPrompt(context), candidates, nil
}

func (s *Server) callDryRunModel(ctx context.Context, userID, modelID, systemPrompt, userPrompt string) (string, error) {
	model, err := s.stores.Models.Get(userID, modelID)
	if err != nil {
		return "", errors.New("the model was not found")
	}
	if !model.Enabled {
		return "", fmt.Errorf("model %s is not enabled", model.Name)
	}
	if model.APIKey == "" {
		return "", fmt.Errorf("model %s is missing API Key", model.Name)
	}

	client := agent.NewClientByProvider(model.Provider)
	if client == nil {
		client = agent.NewClient()
	}
	client.SetAPIKey(string(model.APIKey), model.BaseURL, model.ModelName)
	agent.TagUsage(client, agent.FeatureTest, "")

	// Leaving the dialog stops the call rather than paying for an answer nobody will read.
	response, err := client.CallWithRequest(&agent.Request{
		Ctx:      ctx,
		Messages: []agent.Message{agent.NewSystemMessage(systemPrompt), agent.NewUserMessage(userPrompt)},
	})
	if err != nil {
		return "", agent.Classify(client, err)
	}
	return response, nil
}
