package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"vergex/internal/agent"
	"vergex/internal/exchange/binance"
	"vergex/internal/exchange/okx"
	"vergex/internal/logger"
	"vergex/internal/planning"
	planningagent "vergex/internal/planning/agent"
	"vergex/internal/planning/agent/model"
	"vergex/internal/planning/core"
	"vergex/internal/planning/forge"
	"vergex/internal/planning/instrument"
	"vergex/internal/planning/live"
	"vergex/internal/planning/market"
	"vergex/internal/planning/plugins"
	"vergex/internal/store"
)

type planningBrokerCache struct {
	stamp   string
	broker  live.Broker
	created time.Time
}

type planningServices struct {
	brokers     sync.Map
	dataPlugins sync.Map
	feedSlots   chan struct{}
	source      *market.Client
	live        *live.Controller
	turns       sync.Map
	mutex       sync.Mutex
}

func (s *Server) planningService() (*planningServices, error) {
	s.planningMu.RLock()
	closing := s.planningClosing
	s.planningMu.RUnlock()
	if closing {
		return nil, errors.New("server is shutting down")
	}
	s.planningOnce.Do(func() {
		s.planningMu.Lock()
		defer s.planningMu.Unlock()
		if s.planningClosing {
			s.planningError = errors.New("server is shutting down")
			return
		}
		source := market.New()
		p := &planningServices{source: source, feedSlots: make(chan struct{}, 16)}
		controller, e := live.New(s.store, s.planningBroker, source)
		if e != nil {
			s.planningError = e
			return
		}
		p.live = controller
		s.planning = p
	})
	return s.planning, s.planningError
}

func (s *Server) planningBroker(ctx context.Context, user, id string) (live.Broker, string, error) {
	ex, e := s.planningAccount(user, id)
	if e != nil {
		return nil, "", e
	}
	running, er := s.store.Trader().ListByExchangeID(user, id)
	if er != nil {
		return nil, "", errors.New("cannot verify automatic trader state")
	}
	for _, t := range running {
		active := t.IsRunning
		if s.traderManager != nil {
			if runtime, e := s.traderManager.GetTrader(t.ID); e == nil {
				if yes, ok := runtime.GetStatus()["is_running"].(bool); ok && yes {
					active = true
				}
			}
		}
		if active {
			return nil, "", errors.New("Automatic trading is running on this account. Stop it before using an approved one-time plan.")
		}
	}
	stampBytes, _ := json.Marshal([]any{ex.UpdatedAt, string(ex.APIKey), string(ex.SecretKey), string(ex.Passphrase)})
	stampHash := sha256.Sum256(stampBytes)
	stamp := hex.EncodeToString(stampHash[:])
	if s.planning != nil {
		if cached, ok := s.planning.brokers.Load(user + "/" + id); ok {
			v := cached.(planningBrokerCache)
			if v.stamp == stamp && time.Since(v.created) < 5*time.Minute {
				return v.broker, ex.Identity, nil
			}
		}
	}
	cache := func(b live.Broker, key string) (live.Broker, string, error) {
		if s.planning != nil {
			s.planning.brokers.Store(user+"/"+id, planningBrokerCache{stamp: stamp, broker: b, created: time.Now()})
		}
		return b, key, nil
	}
	key := ex.Identity
	switch ex.ExchangeType {
	case "binance":
		if ex.APIKey == "" || ex.SecretKey == "" {
			return nil, "", errors.New("Connect a Binance futures account in Vergex settings first")
		}
		t, err := binance.NewAccountTrader(ctx, string(ex.APIKey), string(ex.SecretKey))
		if err != nil {
			return nil, "", err
		}
		return cache(live.TraderBroker{Executor: t}, key)
	case "okx":
		if ex.APIKey == "" || ex.SecretKey == "" || ex.Passphrase == "" {
			return nil, "", errors.New("Connect the exchange API key, secret and passphrase in Vergex first")
		}
		return cache(live.TraderBroker{Executor: okx.NewAccountTrader(string(ex.APIKey), string(ex.SecretKey), string(ex.Passphrase))}, key)
	default:
		return nil, "", errors.New("This exchange is not supported by Frontier; no exchange fallback is allowed")
	}
}
func (s *Server) planningAccount(user, id string) (*store.Exchange, error) {
	ex, e := s.store.Exchange().GetByID(user, id)
	if e != nil {
		return nil, errors.New("Select a Vergex trading account owned by the current user")
	}
	if !frontierVenue(ex.ExchangeType) {
		return nil, errors.New("Frontier supports Binance and OKX perpetual accounts")
	}
	return ex, nil
}

// Public-market planning does not require private exchange credentials. Only
// approval/execution routes may call planningAccount/planningBroker.
func (s *Server) planningWorkspace(user, id, venue string) (*store.Exchange, error) {
	if id != "" {
		ex, e := s.planningAccount(user, id)
		if e != nil {
			return nil, e
		}
		if venue != "" && venue != ex.ExchangeType {
			return nil, errors.New("Selected account does not match the market venue")
		}
		return ex, nil
	}
	if !frontierVenue(venue) {
		return nil, errors.New("Select Binance or OKX")
	}
	return &store.Exchange{UserID: user, ExchangeType: venue}, nil
}

func planningError(c *gin.Context, e error) { c.JSON(http.StatusBadRequest, gin.H{"error": e.Error()}) }
func (s *Server) registerPlanning(api, protected *gin.RouterGroup) {
	api.GET("/planning/markets", s.handlePlanningMarkets)
	for _, path := range []string{"instrument", "candles", "feed"} {
		api.GET("/planning/"+path, s.handlePlanningMarket)
	}
	protected.GET("/planning/account", s.handlePlanningReadAccount)
	protected.POST("/planning/close-position", s.handlePlanningAccountClose)
	protected.GET("/planning/state", s.handlePlanningState)
	protected.GET("/planning/results", s.handlePlanningResults)
	protected.POST("/planning/chat", s.handlePlanningChat)
	protected.POST("/planning/compile", s.handlePlanningCompile)
	protected.POST("/planning/approve", s.handlePlanningApprove)
	protected.POST("/planning/execute", s.handlePlanningExecute)
	protected.POST("/planning/stop", s.handlePlanningStop)
	protected.POST("/planning/reconcile", s.handlePlanningReconcile)
	protected.POST("/planning/check-status", s.handlePlanningCheckStatus)
	protected.GET("/planning/conversations", s.handlePlanningConversations)
	protected.GET("/planning/conversations/:id", s.handlePlanningConversation)
	protected.GET("/planning/preferences", s.handlePlanningPreferences)
	protected.PUT("/planning/preferences", s.handlePlanningPreferences)
	protected.GET("/planning/entry-plugins", s.handlePlanningEntryPlugins)
}
func (s *Server) handlePlanningMarket(c *gin.Context) {
	p, e := s.planningService()
	if e != nil {
		c.JSON(503, gin.H{"error": "planning service unavailable"})
		return
	}
	if strings.HasSuffix(c.Request.URL.Path, "/instrument") {
		b, e := p.source.Snapshot(c.Request.Context(), c.Query("venue"), c.Query("symbol"))
		if e != nil {
			planningError(c, e)
			return
		}
		c.JSON(200, b)
		return
	}
	var q market.CandleQuery
	if e = c.ShouldBindQuery(&q); e != nil {
		planningError(c, e)
		return
	}
	for name, target := range map[string]*int64{"from": &q.From, "to": &q.To} {
		if value := c.Query(name); value != "" {
			n, er := strconv.ParseInt(value, 10, 64)
			if er != nil {
				planningError(c, errors.New("invalid chart range"))
				return
			}
			*target = n
		}
	}
	if value := c.Query("limit"); value != "" {
		n, er := strconv.Atoi(value)
		if er != nil {
			planningError(c, errors.New("invalid chart count"))
			return
		}
		q.Limit = n
	}
	q.Venue = c.Query("venue")
	q.Symbol = c.Query("symbol")
	q.Interval = c.Query("interval")
	if q.Limit == 0 {
		q.Limit = 300
	}
	if e = q.Validate(); e != nil {
		planningError(c, e)
		return
	}
	if strings.HasSuffix(c.Request.URL.Path, "/candles") {
		rows, e := p.source.Candles(c.Request.Context(), q)
		if e != nil {
			planningError(c, e)
			return
		}
		c.JSON(200, gin.H{"candles": rows, "interval": q.Interval, "venue": q.Venue, "symbol": q.Symbol, "mode": "public", "received": time.Now().UTC()})
		return
	}
	if q.From != 0 || q.To != 0 {
		planningError(c, errors.New("feed cannot use historical bounds"))
		return
	}
	select {
	case p.feedSlots <- struct{}{}:
		defer func() { <-p.feedSlots }()
	default:
		c.JSON(429, gin.H{"error": "too many planning chart feeds"})
		return
	}
	s.servePlanningFeed(c, p.source, q)
}
func (s *Server) handlePlanningState(c *gin.Context) {
	p, e := s.planningService()
	if e != nil {
		planningError(c, e)
		return
	}
	user := c.GetString("user_id")
	drafts := planning.NewDraftStore(s.store, user, "", "")
	runs, e := p.live.ListContext(c.Request.Context(), user)
	if e != nil {
		planningError(c, e)
		return
	}
	c.JSON(200, gin.H{"records": drafts.Records(), "accounts": drafts.Accounts(), "runs": runs, "mode": "live", "now": time.Now().UTC()})
}
func (s *Server) handlePlanningCompile(c *gin.Context) {
	var in struct {
		ExchangeID  string           `json:"exchange_id"`
		Venue       string           `json:"venue"`
		Symbol      string           `json:"symbol"`
		Idea        core.Idea        `json:"idea"`
		Constraints core.Constraints `json:"constraints"`
		AutoSize    bool             `json:"auto_size"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
	if e := c.ShouldBindJSON(&in); e != nil {
		planningError(c, errors.New("invalid draft input"))
		return
	}
	ex, e := s.planningWorkspace(c.GetString("user_id"), in.ExchangeID, in.Venue)
	if e != nil {
		planningError(c, e)
		return
	}
	p, e := s.planningService()
	if e != nil {
		planningError(c, e)
		return
	}
	snapshot, e := p.source.Snapshot(c.Request.Context(), ex.ExchangeType, in.Symbol)
	if e != nil {
		planningError(c, e)
		return
	}
	if in.Idea.Execution == nil {
		in.Idea.Execution = &core.Execution{Kind: "limit"}
	}
	if in.AutoSize {
		in.Constraints, e = s.planningAccountLimits(c.Request.Context(), c.GetString("user_id"), ex.ID, in.Symbol, in.Constraints, false)
		if e != nil {
			planningError(c, e)
			return
		}
	}
	draft, e := core.Compile(snapshot, in.Constraints, in.Idea, time.Now().UTC())
	if e != nil {
		planningError(c, e)
		return
	}
	ds := planning.NewDraftStore(s.store, c.GetString("user_id"), ex.ID, ex.ExchangeType)
	ds.Origin = planning.OriginManual
	if e = ds.Add([]core.Plan{draft}); e != nil {
		planningError(c, e)
		return
	}
	c.JSON(200, draft)
}
func (s *Server) handlePlanningChat(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var in struct {
		planningagent.Input
		ExchangeID   string           `json:"exchange_id"`
		AgentModelID string           `json:"agent_model_id"`
		Constraints  core.Constraints `json:"constraints"`
		AutoSize     bool             `json:"auto_size"`
	}
	if e := c.ShouldBindJSON(&in); e != nil {
		planningError(c, errors.New("invalid planning input"))
		return
	}
	user := c.GetString("user_id")
	ex, e := s.planningWorkspace(user, in.ExchangeID, in.Venue)
	if e != nil {
		planningError(c, e)
		return
	}
	in.Venue = ex.ExchangeType
	if e = in.Validate(); e != nil {
		planningError(c, e)
		return
	}
	if in.AutoSize {
		in.Constraints, e = s.planningAccountLimits(c.Request.Context(), user, ex.ID, in.Symbol, in.Constraints, true)
		if e != nil {
			planningError(c, e)
			return
		}
	}
	if e = in.Constraints.Validate(); e != nil {
		planningError(c, e)
		return
	}
	agentModel, e := s.store.AgentModel().Get(user, in.AgentModelID)
	if e != nil || !agentModel.Enabled || agentModel.APIKey == "" {
		planningError(c, errors.New("Select a configured and enabled model owned by the current user"))
		return
	}
	client := agent.NewClientByProvider(agentModel.Provider)
	if client == nil {
		planningError(c, errors.New("This model provider is not registered in Vergex"))
		return
	}
	client.SetAPIKey(string(agentModel.APIKey), agentModel.CustomAPIURL, agentModel.CustomModelName)
	client.SetTimeout(90 * time.Second)
	agent.TagUsage(client, agent.FeatureFrontier, "")
	p, e := s.planningService()
	if e != nil {
		planningError(c, e)
		return
	}
	if _, loaded := p.turns.LoadOrStore(user, true); loaded {
		c.JSON(409, gin.H{"error": "A planning request is still running. Wait for it or stop it first."})
		return
	}
	defer p.turns.Delete(user)
	ds := planning.NewDraftStore(s.store, user, ex.ID, ex.ExchangeType)
	ds.Origin = planning.OriginAssistant
	if in.PlanID != "" && !ds.HasDraft(in.PlanID) {
		planningError(c, errors.New("Only unapproved drafts on this account can be revised; revoke locked approval first"))
		return
	}
	hash := sha256.Sum256([]byte(user))
	root := filepath.Join("data", "planning", hex.EncodeToString(hash[:16]))
	if e = os.MkdirAll(root, 0700); e != nil {
		planningError(c, errors.New("planning workspace unavailable"))
		return
	}
	forgeRegistry, e := forge.Open(filepath.Join(root, "entry-plugins"))
	if e != nil {
		planningError(c, e)
		return
	}
	registry, e := s.planningDataRegistry(user)
	if e != nil {
		planningError(c, e)
		return
	}
	engine := planningagent.Engine{Store: ds, Market: p.source, Charts: p.source, Model: &model.ClientAdapter{Client: client}, Limits: in.Constraints, Forge: forgeRegistry, Plugins: registry}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 180*time.Second)
	defer cancel()
	if in.SessionID != "" {
		conversation := store.AgentConversation{
			ID:           in.SessionID,
			UserID:       user,
			Feature:      store.FeatureFrontier,
			AgentModelID: agentModel.ID,
			Title:        conversationTitle(in.Message),
			Context:      map[string]string{"exchange_id": ex.ID, "symbol": in.Symbol},
		}
		if e = s.store.AgentConversation().Open(&conversation); e != nil {
			planningError(c, errors.New("invalid planning input"))
			return
		}
		engine.Record = s.recordPlanningMessage(in.SessionID)
		ctx = agent.WithConversation(ctx, in.SessionID)
		ds.ConversationID = in.SessionID
	}
	var preferences planning.Preferences
	if _, e := s.store.LoadSetting(planning.SettingsModule, &preferences); e == nil && preferences.Text != "" {
		engine.Memory = []string{preferences.Text}
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("X-Accel-Buffering", "no")
	emit := func(event planningagent.Event) error {
		raw, e := json.Marshal(event)
		if e != nil {
			return e
		}
		_, e = fmt.Fprintf(c.Writer, "data: %s\n\n", raw)
		if e == nil {
			c.Writer.Flush()
		}
		return e
	}
	_ = emit(planningagent.Event{Type: "status", Data: gin.H{"message": "Loading market context; preparing the planning model"}})
	_ = emit(planningagent.Event{Type: "model_transport", Data: gin.H{"transport": "native_tools"}})
	e = engine.Run(ctx, in.Input, emit)
	if e != nil && c.Request.Context().Err() == nil {
		message := e.Error()
		if ctx.Err() != nil || errors.Is(e, context.DeadlineExceeded) {
			message = "Planning reached its time limit. No orders were submitted. Review saved drafts before starting another request."
		}
		_ = emit(planningagent.Event{Type: "error", Data: gin.H{"message": message}})
	}
	if c.Request.Context().Err() == nil {
		_ = emit(planningagent.Event{Type: "done", Data: gin.H{"ok": e == nil}})
	}
}

// conversationTitle is how a conversation is named: the start of its first message.
func conversationTitle(message string) string {
	message = strings.Join(strings.Fields(message), " ")
	if runes := []rune(message); len(runes) > 80 {
		return string(runes[:80])
	}
	return message
}

// recordPlanningMessage keeps a turn's messages as they happen. A failure to record
// is logged and never stops the turn.
func (s *Server) recordPlanningMessage(conversationID string) func(string, model.Message) {
	return func(role string, message model.Message) {
		row := store.AgentMessage{Role: role, Content: message.Content, Thinking: message.ReasoningContent, ToolCallID: message.ToolCallID}
		if len(message.ToolCalls) > 0 {
			calls, e := json.Marshal(message.ToolCalls)
			if e != nil {
				logger.Warnf("Frontier: encode tool calls: %v", e)
				return
			}
			row.ToolCalls = string(calls)
		}
		if e := s.store.AgentConversation().Append(conversationID, row); e != nil {
			logger.Warnf("Frontier: record message: %v", e)
		}
	}
}
func (s *Server) handlePlanningApprove(c *gin.Context) {
	var in struct {
		PlanID      string `json:"plan_id"`
		Hash        string `json:"hash"`
		Confirmed   bool   `json:"confirmed"`
		ReplaceID   string `json:"replace_approval_id"`
		ReplaceHash string `json:"replace_approval_hash"`
	}
	if c.ShouldBindJSON(&in) != nil || !in.Confirmed {
		planningError(c, errors.New("explicit click confirmation is required"))
		return
	}
	p, e := s.planningService()
	if e != nil {
		planningError(c, e)
		return
	}
	d, e := planning.NewDraftStore(s.store, c.GetString("user_id"), "", "").Find(in.PlanID)
	if e != nil {
		planningError(c, errors.New("plan not found"))
		return
	}
	unlock, e := s.lockPlanningAccount(c.GetString("user_id"), d.ExchangeID)
	if e != nil {
		planningError(c, e)
		return
	}
	defer unlock()
	v, e := p.live.ApproveReplacing(c.Request.Context(), c.GetString("user_id"), in.PlanID, in.Hash, "APPROVE LIVE", in.ReplaceID, in.ReplaceHash)
	if e != nil {
		planningError(c, e)
		return
	}
	c.JSON(200, v)
}
func (s *Server) handlePlanningExecute(c *gin.Context) {
	var in struct {
		ID   string `json:"id"`
		Hash string `json:"hash"`
	}
	if c.ShouldBindJSON(&in) != nil {
		planningError(c, errors.New("invalid execution"))
		return
	}
	p, e := s.planningService()
	if e != nil {
		planningError(c, e)
		return
	}
	v, e := p.live.Execute(c.Request.Context(), c.GetString("user_id"), in.ID, in.Hash)
	if e != nil {
		planningError(c, e)
		return
	}
	c.JSON(200, v)
}
func (s *Server) handlePlanningStop(c *gin.Context) {
	var in struct {
		ID   string `json:"id"`
		Hash string `json:"hash"`
	}
	if c.ShouldBindJSON(&in) != nil {
		planningError(c, errors.New("invalid stop request"))
		return
	}
	p, e := s.planningService()
	if e != nil {
		planningError(c, e)
		return
	}
	v, e := p.live.StopContext(c.Request.Context(), c.GetString("user_id"), in.ID, in.Hash)
	if e != nil {
		planningError(c, e)
		return
	}
	c.JSON(200, v)
}
func (s *Server) handlePlanningCheckStatus(c *gin.Context) {
	var in struct {
		ID string `json:"id"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ID == "" {
		planningError(c, errors.New("Select an execution record to check"))
		return
	}
	p, e := s.planningService()
	if e != nil {
		planningError(c, e)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	v, e := p.live.CheckStatus(ctx, c.GetString("user_id"), in.ID)
	if e != nil {
		planningError(c, e)
		return
	}
	c.JSON(200, v)
}
func (s *Server) handlePlanningReconcile(c *gin.Context) {
	var in struct {
		ID string `json:"id"`
	}
	if c.ShouldBindJSON(&in) != nil {
		planningError(c, errors.New("invalid reconciliation"))
		return
	}
	p, e := s.planningService()
	if e != nil {
		planningError(c, e)
		return
	}
	v, e := p.live.ReconcileContext(c.Request.Context(), c.GetString("user_id"), in.ID)
	if e != nil {
		planningError(c, e)
		return
	}
	c.JSON(200, v)
}
func (s *Server) handlePlanningConversations(c *gin.Context) {
	rows, e := s.store.AgentConversation().List(c.GetString("user_id"), store.FeatureFrontier, 50)
	if e != nil {
		planningError(c, e)
		return
	}
	out := make([]planningConversation, 0, len(rows))
	for _, row := range rows {
		out = append(out, newPlanningConversation(row))
	}
	c.JSON(200, out)
}

// handlePlanningConversation returns a conversation with what was said in it.
func (s *Server) handlePlanningConversation(c *gin.Context) {
	row, e := s.store.AgentConversation().Get(c.GetString("user_id"), c.Param("id"))
	if e != nil || row.Feature != store.FeatureFrontier {
		c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
		return
	}
	messages, e := s.store.AgentConversation().Transcript(row.ID, 0)
	if e != nil {
		planningError(c, e)
		return
	}
	c.JSON(200, gin.H{"conversation": newPlanningConversation(*row), "messages": messages})
}

// planningConversation is a conversation as the Frontier page lists it.
type planningConversation struct {
	ID           string    `json:"id"`
	ExchangeID   string    `json:"exchange_id"`
	AgentModelID string    `json:"agent_model_id"`
	Symbol       string    `json:"symbol"`
	Title        string    `json:"title"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func newPlanningConversation(row store.AgentConversation) planningConversation {
	return planningConversation{
		ID:           row.ID,
		ExchangeID:   row.Context["exchange_id"],
		AgentModelID: row.AgentModelID,
		Symbol:       row.Context["symbol"],
		Title:        row.Title,
		UpdatedAt:    row.UpdatedAt,
	}
}
func (s *Server) handlePlanningPreferences(c *gin.Context) {
	p, e := s.planningService()
	if e != nil {
		planningError(c, e)
		return
	}
	_ = p
	if c.Request.Method == http.MethodPut {
		var in struct {
			Text string `json:"text"`
		}
		if c.ShouldBindJSON(&in) != nil || len(in.Text) > 1600 || planningagent.HasCredential(in.Text) {
			planningError(c, errors.New("Preferences must be at most 1600 bytes and cannot contain credentials"))
			return
		}
		if e := s.store.PatchSetting(planning.SettingsModule, map[string]any{"text": in.Text}); e != nil {
			planningError(c, e)
			return
		}
		c.JSON(200, gin.H{"saved": true})
		return
	}
	var row planning.Preferences
	if _, e := s.store.LoadSetting(planning.SettingsModule, &row); e != nil {
		planningError(c, e)
		return
	}
	c.JSON(200, gin.H{"text": row.Text})
}
func (s *Server) handlePlanningEntryPlugins(c *gin.Context) {
	user := c.GetString("user_id")
	hash := sha256.Sum256([]byte(user))
	r, e := forge.Open(filepath.Join("data", "planning", hex.EncodeToString(hash[:16]), "entry-plugins"))
	if e != nil {
		planningError(c, e)
		return
	}
	c.JSON(200, r.List(c.Query("session_id")))
}

func (s *Server) lockPlanningAccount(user, id string) (func(), error) {
	p, e := s.planningService()
	if e != nil {
		return nil, e
	}
	ex, e := s.store.Exchange().GetByID(user, id)
	if e != nil {
		return nil, errors.New("account not found")
	}
	_ = p
	return store.LockAccount(ex.ID), nil
}
func (s *Server) handlePlanningReadAccount(c *gin.Context) {
	if !instrument.ValidSymbol(c.Query("symbol")) {
		planningError(c, errors.New("Invalid symbol format"))
		return
	}
	p, e := s.planningService()
	if e != nil {
		planningError(c, e)
		return
	}
	b, _, e := s.planningBroker(c.Request.Context(), c.GetString("user_id"), c.Query("exchange_id"))
	if e != nil {
		planningError(c, e)
		return
	}
	_ = p
	a, e := b.Account(c.Request.Context(), c.Query("symbol"))
	if e != nil {
		planningError(c, e)
		return
	}
	c.JSON(200, gin.H{"equity": a.Equity, "available": a.Available, "fee_bps": a.FeeBPS, "position": a.Position, "open_orders": a.OpenOrders})
}

// Registry state is scoped to the logged-in user and shared with its tool loop.
func (s *Server) planningDataRegistry(user string) (*plugins.Registry, error) {
	p, e := s.planningService()
	if e != nil {
		return nil, e
	}
	if cached, ok := p.dataPlugins.Load(user); ok {
		return cached.(*plugins.Registry), nil
	}
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if cached, ok := p.dataPlugins.Load(user); ok {
		return cached.(*plugins.Registry), nil
	}
	hash := sha256.Sum256([]byte(user))
	root := filepath.Join("data", "planning", hex.EncodeToString(hash[:16]))
	registry, e := plugins.Open(filepath.Join(root, "plugins.json"), plugins.Builtins(p.source)...)
	if e != nil {
		return nil, e
	}
	p.dataPlugins.Store(user, registry)
	return registry, nil
}

func (s *Server) planningAccountLimits(ctx context.Context, user, exchangeID, symbol string, constraints core.Constraints, preview bool) (core.Constraints, error) {
	if exchangeID == "" {
		return constraints, errors.New("connect a trading account for automatic sizing")
	}
	b, _, e := s.planningBroker(ctx, user, exchangeID)
	if e != nil {
		return constraints, e
	}
	a, e := b.Account(ctx, symbol)
	if e != nil {
		return constraints, e
	}
	if preview {
		return live.DraftAccountLimits(a, constraints)
	}
	return live.AccountLimits(a, constraints)
}

func frontierVenue(venue string) bool {
	return venue == "binance" || venue == "okx"
}
