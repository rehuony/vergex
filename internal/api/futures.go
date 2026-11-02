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
	"strconv"
	"strings"
	"sync"
	"time"
	"vergex/internal/account"
	"vergex/internal/agent"
	"vergex/internal/database"
	"vergex/internal/futures"
	futuresagent "vergex/internal/futures/agent"
	"vergex/internal/futures/forge"
	"vergex/internal/futures/plan"
	"vergex/internal/futures/plugins"
	"vergex/internal/futures/run"
	"vergex/internal/logger"
	"vergex/internal/market/instrument"
	"vergex/internal/venue/binance"
	"vergex/internal/venue/okx"
)

type futuresBrokerCache struct {
	stamp   string
	broker  run.Broker
	created time.Time
}

type futuresServices struct {
	brokers    sync.Map
	feedSlots  chan struct{}
	source     *futures.Market
	controller *run.Controller
	turns      sync.Map
}

func (s *Server) futuresService() (*futuresServices, error) {
	s.futuresMu.RLock()
	closing := s.futuresClosing
	s.futuresMu.RUnlock()
	if closing {
		return nil, errors.New("server is shutting down")
	}
	s.futuresOnce.Do(func() {
		s.futuresMu.Lock()
		defer s.futuresMu.Unlock()
		if s.futuresClosing {
			s.futuresError = errors.New("server is shutting down")
			return
		}
		source := futures.NewMarket()
		p := &futuresServices{source: source, feedSlots: make(chan struct{}, 16)}
		controller, e := run.New(s.stores.DB, s.futuresBroker, source)
		if e != nil {
			s.futuresError = e
			return
		}
		p.controller = controller
		s.futures = p
	})
	return s.futures, s.futuresError
}

func (s *Server) futuresBroker(ctx context.Context, user, id string) (run.Broker, string, error) {
	ex, e := s.futuresAccount(user, id)
	if e != nil {
		return nil, "", e
	}
	running, er := s.stores.Traders.ListByAccountID(user, id)
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
	if s.futures != nil {
		if cached, ok := s.futures.brokers.Load(user + "/" + id); ok {
			v := cached.(futuresBrokerCache)
			if v.stamp == stamp && time.Since(v.created) < 5*time.Minute {
				return v.broker, ex.Identity, nil
			}
		}
	}
	cache := func(b run.Broker, key string) (run.Broker, string, error) {
		if s.futures != nil {
			s.futures.brokers.Store(user+"/"+id, futuresBrokerCache{stamp: stamp, broker: b, created: time.Now()})
		}
		return b, key, nil
	}
	key := ex.Identity
	switch ex.Venue {
	case "binance":
		if ex.APIKey == "" || ex.SecretKey == "" {
			return nil, "", errors.New("Connect a Binance futures account in Vergex settings first")
		}
		t, err := binance.NewReadOnlyAccount(ctx, string(ex.APIKey), string(ex.SecretKey))
		if err != nil {
			return nil, "", err
		}
		return cache(run.ExecutorBroker{Executor: t}, key)
	case "okx":
		if ex.APIKey == "" || ex.SecretKey == "" || ex.Passphrase == "" {
			return nil, "", errors.New("Connect the exchange API key, secret and passphrase in Vergex first")
		}
		return cache(run.ExecutorBroker{Executor: okx.NewReadOnlyAccount(string(ex.APIKey), string(ex.SecretKey), string(ex.Passphrase))}, key)
	default:
		return nil, "", errors.New("This exchange is not supported for futures trading; no exchange fallback is allowed")
	}
}
func (s *Server) futuresAccount(user, id string) (*account.Account, error) {
	ex, e := s.stores.Accounts.GetByID(user, id)
	if e != nil {
		return nil, errors.New("Select a Vergex trading account owned by the current user")
	}
	if !futuresVenue(ex.Venue) {
		return nil, errors.New("Futures trading supports Binance and OKX perpetual accounts")
	}
	return ex, nil
}

// Public-market planning does not require private exchange credentials. Only
// approval/execution routes may call futuresAccount/futuresBroker.
func (s *Server) futuresWorkspace(user, id, venue string) (*account.Account, error) {
	if id != "" {
		ex, e := s.futuresAccount(user, id)
		if e != nil {
			return nil, e
		}
		if venue != "" && venue != ex.Venue {
			return nil, errors.New("Selected account does not match the market venue")
		}
		return ex, nil
	}
	if !futuresVenue(venue) {
		return nil, errors.New("Select Binance or OKX")
	}
	return &account.Account{UserID: user, Venue: venue}, nil
}

func futuresError(c *gin.Context, e error) { c.JSON(http.StatusBadRequest, gin.H{"error": e.Error()}) }
func (s *Server) registerFutures(api, protected *gin.RouterGroup) {
	api.GET("/futures/markets", s.handleFuturesMarkets)
	for _, path := range []string{"instrument", "candles", "feed"} {
		api.GET("/futures/"+path, s.handleFuturesMarket)
	}
	protected.GET("/futures/account", s.handleFuturesReadAccount)
	protected.POST("/futures/close-position", s.handleFuturesAccountClose)
	protected.GET("/futures/state", s.handleFuturesState)
	protected.GET("/futures/results", s.handleFuturesResults)
	protected.POST("/futures/chat", s.handleFuturesChat)
	protected.POST("/futures/compile", s.handleFuturesCompile)
	protected.POST("/futures/approve", s.handleFuturesApprove)
	protected.POST("/futures/execute", s.handleFuturesExecute)
	protected.POST("/futures/stop", s.handleFuturesStop)
	protected.POST("/futures/reconcile", s.handleFuturesReconcile)
	protected.POST("/futures/check-status", s.handleFuturesCheckStatus)
	protected.GET("/futures/conversations", s.handleFuturesConversations)
	protected.GET("/futures/conversations/:id", s.handleFuturesConversation)
	protected.GET("/futures/preferences", s.handleFuturesPreferences)
	protected.PUT("/futures/preferences", s.handleFuturesPreferences)
	protected.GET("/futures/entry-plugins", s.handleFuturesEntryPlugins)
}
func (s *Server) handleFuturesMarket(c *gin.Context) {
	p, e := s.futuresService()
	if e != nil {
		c.JSON(503, gin.H{"error": "planning service unavailable"})
		return
	}
	if strings.HasSuffix(c.Request.URL.Path, "/instrument") {
		b, e := p.source.Snapshot(c.Request.Context(), c.Query("venue"), c.Query("symbol"))
		if e != nil {
			futuresError(c, e)
			return
		}
		c.JSON(200, b)
		return
	}
	var q futures.CandleQuery
	if e = c.ShouldBindQuery(&q); e != nil {
		futuresError(c, e)
		return
	}
	for name, target := range map[string]*int64{"from": &q.From, "to": &q.To} {
		if value := c.Query(name); value != "" {
			n, er := strconv.ParseInt(value, 10, 64)
			if er != nil {
				futuresError(c, errors.New("invalid chart range"))
				return
			}
			*target = n
		}
	}
	if value := c.Query("limit"); value != "" {
		n, er := strconv.Atoi(value)
		if er != nil {
			futuresError(c, errors.New("invalid chart count"))
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
		futuresError(c, e)
		return
	}
	if strings.HasSuffix(c.Request.URL.Path, "/candles") {
		rows, e := p.source.Candles(c.Request.Context(), q)
		if e != nil {
			futuresError(c, e)
			return
		}
		c.JSON(200, gin.H{"candles": rows, "interval": q.Interval, "venue": q.Venue, "symbol": q.Symbol, "mode": "public", "received": time.Now().UTC()})
		return
	}
	if q.From != 0 || q.To != 0 {
		futuresError(c, errors.New("feed cannot use historical bounds"))
		return
	}
	select {
	case p.feedSlots <- struct{}{}:
		defer func() { <-p.feedSlots }()
	default:
		c.JSON(429, gin.H{"error": "too many planning chart feeds"})
		return
	}
	s.serveFuturesFeed(c, p.source, q)
}
func (s *Server) handleFuturesState(c *gin.Context) {
	p, e := s.futuresService()
	if e != nil {
		futuresError(c, e)
		return
	}
	user := c.GetString("user_id")
	drafts := futures.NewPlanStore(s.stores.DB, user, "", "")
	runs, e := p.controller.ListContext(c.Request.Context(), user)
	if e != nil {
		futuresError(c, e)
		return
	}
	c.JSON(200, gin.H{"records": drafts.Records(), "accounts": drafts.Accounts(), "runs": runs, "mode": "live", "now": time.Now().UTC()})
}
func (s *Server) handleFuturesCompile(c *gin.Context) {
	var in struct {
		AccountID   string           `json:"account_id"`
		Venue       string           `json:"venue"`
		Symbol      string           `json:"symbol"`
		Idea        plan.Idea        `json:"idea"`
		Constraints plan.Constraints `json:"constraints"`
		AutoSize    bool             `json:"auto_size"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
	if e := c.ShouldBindJSON(&in); e != nil {
		futuresError(c, errors.New("invalid draft input"))
		return
	}
	ex, e := s.futuresWorkspace(c.GetString("user_id"), in.AccountID, in.Venue)
	if e != nil {
		futuresError(c, e)
		return
	}
	p, e := s.futuresService()
	if e != nil {
		futuresError(c, e)
		return
	}
	snapshot, e := p.source.Snapshot(c.Request.Context(), ex.Venue, in.Symbol)
	if e != nil {
		futuresError(c, e)
		return
	}
	if in.Idea.Execution == nil {
		in.Idea.Execution = &plan.Execution{Kind: "limit"}
	}
	if in.AutoSize {
		in.Constraints, e = s.futuresAccountLimits(c.Request.Context(), c.GetString("user_id"), ex.ID, in.Symbol, in.Constraints, false)
		if e != nil {
			futuresError(c, e)
			return
		}
	}
	draft, e := plan.Compile(snapshot, in.Constraints, in.Idea, time.Now().UTC())
	if e != nil {
		futuresError(c, e)
		return
	}
	ds := futures.NewPlanStore(s.stores.DB, c.GetString("user_id"), ex.ID, ex.Venue)
	ds.Origin = futures.OriginManual
	if e = ds.Add([]plan.Plan{draft}); e != nil {
		futuresError(c, e)
		return
	}
	c.JSON(200, draft)
}
func (s *Server) handleFuturesChat(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var in struct {
		futuresagent.Input
		AccountID    string           `json:"account_id"`
		AgentModelID string           `json:"agent_model_id"`
		Constraints  plan.Constraints `json:"constraints"`
		AutoSize     bool             `json:"auto_size"`
	}
	if e := c.ShouldBindJSON(&in); e != nil {
		futuresError(c, errors.New("invalid planning input"))
		return
	}
	user := c.GetString("user_id")
	ex, e := s.futuresWorkspace(user, in.AccountID, in.Venue)
	if e != nil {
		futuresError(c, e)
		return
	}
	in.Venue = ex.Venue
	if e = in.Validate(); e != nil {
		futuresError(c, e)
		return
	}
	if in.AutoSize {
		in.Constraints, e = s.futuresAccountLimits(c.Request.Context(), user, ex.ID, in.Symbol, in.Constraints, true)
		if e != nil {
			futuresError(c, e)
			return
		}
	}
	if e = in.Constraints.Validate(); e != nil {
		futuresError(c, e)
		return
	}
	agentModel, e := s.stores.Models.Get(user, in.AgentModelID)
	if e != nil || !agentModel.Enabled || agentModel.APIKey == "" {
		futuresError(c, errors.New("Select a configured and enabled model owned by the current user"))
		return
	}
	client := agent.NewClientByProvider(agentModel.Provider)
	if client == nil {
		futuresError(c, errors.New("This model provider is not registered in Vergex"))
		return
	}
	client.SetAPIKey(string(agentModel.APIKey), agentModel.CustomAPIURL, agentModel.CustomModelName)
	client.SetTimeout(90 * time.Second)
	agent.TagUsage(client, agent.FeatureFutures, "")
	p, e := s.futuresService()
	if e != nil {
		futuresError(c, e)
		return
	}
	if _, loaded := p.turns.LoadOrStore(user, true); loaded {
		c.JSON(409, gin.H{"error": "A planning request is still running. Wait for it or stop it first."})
		return
	}
	defer p.turns.Delete(user)
	ds := futures.NewPlanStore(s.stores.DB, user, ex.ID, ex.Venue)
	ds.Origin = futures.OriginAssistant
	if in.PlanID != "" && !ds.HasDraft(in.PlanID) {
		futuresError(c, errors.New("Only unapproved drafts on this account can be revised; revoke locked approval first"))
		return
	}
	registry, e := plugins.Open(plugins.Builtins(p.source)...)
	if e != nil {
		futuresError(c, e)
		return
	}
	engine := futuresagent.Engine{Store: ds, Market: p.source, Charts: p.source, Model: &futuresagent.ClientAdapter{Client: client}, Limits: in.Constraints, Forge: forge.NewRegistry(s.stores.DB), Plugins: registry}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 180*time.Second)
	defer cancel()
	if in.SessionID != "" {
		conversation := agent.Conversation{
			ID:           in.SessionID,
			UserID:       user,
			Feature:      agent.FeatureFutures,
			AgentModelID: agentModel.ID,
			Title:        conversationTitle(in.Message),
			Context:      map[string]string{"account_id": ex.ID, "symbol": in.Symbol},
		}
		if e = s.stores.Conversations.Open(&conversation); e != nil {
			futuresError(c, errors.New("invalid planning input"))
			return
		}
		engine.Record = s.recordFuturesMessage(in.SessionID)
		ctx = agent.WithConversation(ctx, in.SessionID)
		ds.ConversationID = in.SessionID
	}
	var preferences futures.Preferences
	if _, e := database.LoadSetting(s.stores.DB, futures.SettingsModule, &preferences); e == nil && preferences.Text != "" {
		engine.Memory = []string{preferences.Text}
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("X-Accel-Buffering", "no")
	emit := func(event futuresagent.Event) error {
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
	_ = emit(futuresagent.Event{Type: "status", Data: gin.H{"message": "Loading market context; preparing the planning model"}})
	_ = emit(futuresagent.Event{Type: "model_transport", Data: gin.H{"transport": "native_tools"}})
	e = engine.Run(ctx, in.Input, emit)
	if e != nil && c.Request.Context().Err() == nil {
		message := e.Error()
		if ctx.Err() != nil || errors.Is(e, context.DeadlineExceeded) {
			message = "Planning reached its time limit. No orders were submitted. Review saved drafts before starting another request."
		}
		_ = emit(futuresagent.Event{Type: "error", Data: gin.H{"message": message}})
	}
	if c.Request.Context().Err() == nil {
		_ = emit(futuresagent.Event{Type: "done", Data: gin.H{"ok": e == nil}})
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

// recordFuturesMessage keeps a turn's messages as they happen. A failure to record
// is logged and never stops the turn.
func (s *Server) recordFuturesMessage(conversationID string) func(string, futuresagent.Message) {
	return func(role string, message futuresagent.Message) {
		row := agent.ConversationMessage{Role: role, Content: message.Content, Thinking: message.ReasoningContent, ToolCallID: message.ToolCallID}
		if len(message.ToolCalls) > 0 {
			calls, e := json.Marshal(message.ToolCalls)
			if e != nil {
				logger.Warnf("Futures: encode tool calls: %v", e)
				return
			}
			row.ToolCalls = string(calls)
		}
		if e := s.stores.Conversations.Append(conversationID, row); e != nil {
			logger.Warnf("Futures: record message: %v", e)
		}
	}
}
func (s *Server) handleFuturesApprove(c *gin.Context) {
	var in struct {
		PlanID      string `json:"plan_id"`
		Hash        string `json:"hash"`
		Confirmed   bool   `json:"confirmed"`
		ReplaceID   string `json:"replace_approval_id"`
		ReplaceHash string `json:"replace_approval_hash"`
	}
	if c.ShouldBindJSON(&in) != nil || !in.Confirmed {
		futuresError(c, errors.New("explicit click confirmation is required"))
		return
	}
	p, e := s.futuresService()
	if e != nil {
		futuresError(c, e)
		return
	}
	d, e := futures.NewPlanStore(s.stores.DB, c.GetString("user_id"), "", "").Find(in.PlanID)
	if e != nil {
		futuresError(c, errors.New("plan not found"))
		return
	}
	unlock, e := s.lockFuturesAccount(c.GetString("user_id"), d.AccountID)
	if e != nil {
		futuresError(c, e)
		return
	}
	defer unlock()
	v, e := p.controller.ApproveReplacing(c.Request.Context(), c.GetString("user_id"), in.PlanID, in.Hash, "APPROVE LIVE", in.ReplaceID, in.ReplaceHash)
	if e != nil {
		futuresError(c, e)
		return
	}
	c.JSON(200, v)
}
func (s *Server) handleFuturesExecute(c *gin.Context) {
	var in struct {
		ID   string `json:"id"`
		Hash string `json:"hash"`
	}
	if c.ShouldBindJSON(&in) != nil {
		futuresError(c, errors.New("invalid execution"))
		return
	}
	p, e := s.futuresService()
	if e != nil {
		futuresError(c, e)
		return
	}
	v, e := p.controller.Execute(c.Request.Context(), c.GetString("user_id"), in.ID, in.Hash)
	if e != nil {
		futuresError(c, e)
		return
	}
	c.JSON(200, v)
}
func (s *Server) handleFuturesStop(c *gin.Context) {
	var in struct {
		ID   string `json:"id"`
		Hash string `json:"hash"`
	}
	if c.ShouldBindJSON(&in) != nil {
		futuresError(c, errors.New("invalid stop request"))
		return
	}
	p, e := s.futuresService()
	if e != nil {
		futuresError(c, e)
		return
	}
	v, e := p.controller.StopContext(c.Request.Context(), c.GetString("user_id"), in.ID, in.Hash)
	if e != nil {
		futuresError(c, e)
		return
	}
	c.JSON(200, v)
}
func (s *Server) handleFuturesCheckStatus(c *gin.Context) {
	var in struct {
		ID string `json:"id"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ID == "" {
		futuresError(c, errors.New("Select an execution record to check"))
		return
	}
	p, e := s.futuresService()
	if e != nil {
		futuresError(c, e)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	v, e := p.controller.CheckStatus(ctx, c.GetString("user_id"), in.ID)
	if e != nil {
		futuresError(c, e)
		return
	}
	c.JSON(200, v)
}
func (s *Server) handleFuturesReconcile(c *gin.Context) {
	var in struct {
		ID string `json:"id"`
	}
	if c.ShouldBindJSON(&in) != nil {
		futuresError(c, errors.New("invalid reconciliation"))
		return
	}
	p, e := s.futuresService()
	if e != nil {
		futuresError(c, e)
		return
	}
	v, e := p.controller.ReconcileContext(c.Request.Context(), c.GetString("user_id"), in.ID)
	if e != nil {
		futuresError(c, e)
		return
	}
	c.JSON(200, v)
}
func (s *Server) handleFuturesConversations(c *gin.Context) {
	rows, e := s.stores.Conversations.List(c.GetString("user_id"), agent.FeatureFutures, 50)
	if e != nil {
		futuresError(c, e)
		return
	}
	out := make([]futuresConversation, 0, len(rows))
	for _, row := range rows {
		out = append(out, newFuturesConversation(row))
	}
	c.JSON(200, out)
}

// handleFuturesConversation returns a conversation with what was said in it.
func (s *Server) handleFuturesConversation(c *gin.Context) {
	row, e := s.stores.Conversations.Get(c.GetString("user_id"), c.Param("id"))
	if e != nil || row.Feature != agent.FeatureFutures {
		c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
		return
	}
	messages, e := s.stores.Conversations.Transcript(row.ID, 0)
	if e != nil {
		futuresError(c, e)
		return
	}
	c.JSON(200, gin.H{"conversation": newFuturesConversation(*row), "messages": messages})
}

// futuresConversation is a conversation as the Futures page lists it.
type futuresConversation struct {
	ID           string    `json:"id"`
	AccountID    string    `json:"account_id"`
	AgentModelID string    `json:"agent_model_id"`
	Symbol       string    `json:"symbol"`
	Title        string    `json:"title"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func newFuturesConversation(row agent.Conversation) futuresConversation {
	return futuresConversation{
		ID:           row.ID,
		AccountID:    row.Context["account_id"],
		AgentModelID: row.AgentModelID,
		Symbol:       row.Context["symbol"],
		Title:        row.Title,
		UpdatedAt:    row.UpdatedAt,
	}
}
func (s *Server) handleFuturesPreferences(c *gin.Context) {
	p, e := s.futuresService()
	if e != nil {
		futuresError(c, e)
		return
	}
	_ = p
	if c.Request.Method == http.MethodPut {
		var in struct {
			Text string `json:"text"`
		}
		if c.ShouldBindJSON(&in) != nil || len(in.Text) > 1600 || futuresagent.HasCredential(in.Text) {
			futuresError(c, errors.New("Preferences must be at most 1600 bytes and cannot contain credentials"))
			return
		}
		if e := database.PatchSetting(s.stores.DB, futures.SettingsModule, map[string]any{"text": in.Text}); e != nil {
			futuresError(c, e)
			return
		}
		c.JSON(200, gin.H{"saved": true})
		return
	}
	var row futures.Preferences
	if _, e := database.LoadSetting(s.stores.DB, futures.SettingsModule, &row); e != nil {
		futuresError(c, e)
		return
	}
	c.JSON(200, gin.H{"text": row.Text})
}
func (s *Server) handleFuturesEntryPlugins(c *gin.Context) {
	items, e := forge.NewRegistry(s.stores.DB).List(c.Query("session_id"))
	if e != nil {
		futuresError(c, e)
		return
	}
	c.JSON(200, items)
}

func (s *Server) lockFuturesAccount(user, id string) (func(), error) {
	p, e := s.futuresService()
	if e != nil {
		return nil, e
	}
	ex, e := s.stores.Accounts.GetByID(user, id)
	if e != nil {
		return nil, errors.New("account not found")
	}
	_ = p
	return account.Lock(ex.ID), nil
}
func (s *Server) handleFuturesReadAccount(c *gin.Context) {
	if !instrument.ValidSymbol(c.Query("symbol")) {
		futuresError(c, errors.New("Invalid symbol format"))
		return
	}
	p, e := s.futuresService()
	if e != nil {
		futuresError(c, e)
		return
	}
	b, _, e := s.futuresBroker(c.Request.Context(), c.GetString("user_id"), c.Query("account_id"))
	if e != nil {
		futuresError(c, e)
		return
	}
	_ = p
	a, e := b.Account(c.Request.Context(), c.Query("symbol"))
	if e != nil {
		futuresError(c, e)
		return
	}
	c.JSON(200, gin.H{"equity": a.Equity, "available": a.Available, "fee_bps": a.FeeBPS, "position": a.Position, "open_orders": a.OpenOrders})
}

func (s *Server) futuresAccountLimits(ctx context.Context, user, accountID, symbol string, constraints plan.Constraints, preview bool) (plan.Constraints, error) {
	if accountID == "" {
		return constraints, errors.New("connect a trading account for automatic sizing")
	}
	b, _, e := s.futuresBroker(ctx, user, accountID)
	if e != nil {
		return constraints, e
	}
	a, e := b.Account(ctx, symbol)
	if e != nil {
		return constraints, e
	}
	if preview {
		return run.DraftAccountLimits(a, constraints)
	}
	return run.AccountLimits(a, constraints)
}

func futuresVenue(venue string) bool {
	return venue == "binance" || venue == "okx"
}
