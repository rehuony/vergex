package api

import (
	"net/http"
	"strconv"
	"time"

	"vergex/internal/agent"
	"vergex/internal/futures"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	// conversationsListed is how many conversations a list holds unless it asks for fewer.
	conversationsListed = 200
	// conversationsDeleted bounds how many one request deletes.
	conversationsDeleted = 500
)

func (s *Server) registerConversations(protected *gin.RouterGroup) {
	protected.GET("/conversations", s.handleConversations)
	protected.DELETE("/conversations", s.handleConversationsDelete)
}

// chatConversation is one of the user's chats as any feature's list shows it, with what its
// feature keeps about it.
type chatConversation struct {
	ID        string    `json:"id"`
	Feature   string    `json:"feature"`
	Title     string    `json:"title"`
	AccountID string    `json:"account_id,omitempty"`
	Symbol    string    `json:"symbol,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// handleConversations lists the user's chats in every feature people chat in, or in one, and
// on one account when asked.
func (s *Server) handleConversations(c *gin.Context) {
	feature := c.Query("feature")
	if feature != "" && !agent.IsChat(feature) {
		SafeBadRequest(c, "Unknown conversation feature")
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	if limit <= 0 || limit > conversationsListed {
		limit = conversationsListed
	}
	rows, err := s.stores.Conversations.ListChats(c.GetString("user_id"), feature, 0)
	if err != nil {
		SafeInternalError(c, "Read the conversations", err)
		return
	}
	account := c.Query("account_id")
	out := []chatConversation{}
	for _, row := range rows {
		if account != "" && row.Context[futuresConversationAccount] != account {
			continue
		}
		if len(out) == limit {
			break
		}
		out = append(out, chatConversation{
			ID: row.ID, Feature: row.Feature, Title: row.Title, UpdatedAt: row.UpdatedAt,
			AccountID: row.Context[futuresConversationAccount], Symbol: row.Context["symbol"],
		})
	}
	c.JSON(http.StatusOK, out)
}

// handleConversationsDelete deletes the chats a request names, whatever their feature, and
// says how many went. An ID that names none of the user's chats is passed over.
func (s *Server) handleConversationsDelete(c *gin.Context) {
	var in struct {
		IDs []string `json:"ids"`
	}
	if c.ShouldBindJSON(&in) != nil || len(in.IDs) == 0 || len(in.IDs) > conversationsDeleted {
		SafeBadRequest(c, "Name the conversations to delete")
		return
	}
	user := c.GetString("user_id")
	rows, err := s.stores.Conversations.FindChats(user, in.IDs)
	if err != nil {
		SafeInternalError(c, "Read the conversations", err)
		return
	}
	deleted, err := s.deleteConversations(user, rows)
	if err != nil {
		SafeInternalError(c, "Delete the conversations", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

// deleteConversations is the one way chats are deleted, so each takes the whole of what it
// held wherever it is deleted from. What was said, its model calls and its entry
// calculators go with it in the database. The Futures plans drafted in it go here, unless
// they ran: a plan that ran is the record of its orders, and stays without the link. Last,
// the web assistant forgets what it remembered of them. A feature that keeps more of its
// own about a conversation removes it here.
func (s *Server) deleteConversations(userID string, rows []agent.Conversation) (int64, error) {
	var deleted int64
	err := s.stores.DB.Transaction(func(tx *gorm.DB) error {
		n, err := deleteConversationsIn(tx, userID, rows)
		deleted = n
		return err
	})
	if err != nil {
		return 0, err
	}
	// Telegram notices by itself, on the chat's next message.
	if known, ok := s.assistants.Load(userID); ok {
		for _, row := range rows {
			if row.Feature == agent.FeatureAssistant {
				known.(*assistantService).manager.Forget(row.ID)
			}
		}
	}
	return deleted, nil
}

// deleteConversationsIn is what deleteConversations writes, within a transaction that may
// do more, as deleting an account does.
func deleteConversationsIn(tx *gorm.DB, userID string, rows []agent.Conversation) (int64, error) {
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	if err := futures.NewPlanStore(tx, userID, "", "").DeleteDraftedIn(ids); err != nil {
		return 0, err
	}
	return agent.NewConversationStore(tx).Delete(userID, ids)
}
