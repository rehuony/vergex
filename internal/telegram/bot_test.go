package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf16"

	"vergex/internal/agent"
	"vergex/internal/config"
	"vergex/internal/database/testdb"
	"vergex/internal/security/auth"
	"vergex/internal/service"
	"vergex/internal/user"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// fakeBotAPI answers getMe for tokens starting with "good" and reports every
// getMe call on the returned channel.
func fakeBotAPI(t *testing.T) <-chan string {
	t.Helper()
	calls := make(chan string, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Paths look like /bot<token>/<method>.
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/bot"), "/", 2)
		token, method := parts[0], parts[1]
		w.Header().Set("Content-Type", "application/json")
		switch {
		case method == "getMe":
			calls <- token
			if !strings.HasPrefix(token, "good") {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"ok":false,"error_code":401,"description":"Unauthorized"}`))
				return
			}
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Vergex","username":"vergex_test_bot"}}`))
		case method == "getUpdates":
			time.Sleep(20 * time.Millisecond)
			_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
		default:
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		}
	}))
	t.Cleanup(srv.Close)

	origEndpoint, origDelay := apiEndpoint, retryDelay
	apiEndpoint = srv.URL + "/bot%s/%s"
	retryDelay = time.Hour // only a reload may restart a failed bot here
	t.Cleanup(func() { apiEndpoint, retryDelay = origEndpoint, origDelay })
	return calls
}

func expectGetMe(t *testing.T, calls <-chan string, want string) {
	t.Helper()
	select {
	case got := <-calls:
		if got != want {
			t.Fatalf("bot started with token %q, want %q", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("bot never started with token %q", want)
	}
}

func TestStartReloadsWithSavedTokenAndRecoversFromBadToken(t *testing.T) {
	calls := fakeBotAPI(t)
	st := NewStores(testdb.Open(t))

	reload := make(chan struct{}, 1)
	save := func(token string) {
		if err := st.Telegram.Save(token, ""); err != nil {
			t.Fatalf("save token: %v", err)
		}
		reload <- struct{}{}
	}

	// No token yet: the bot waits for one.
	go Start(&config.Config{}, st, reload)
	select {
	case got := <-calls:
		t.Fatalf("bot started without a token (%q)", got)
	case <-time.After(100 * time.Millisecond):
	}

	save("good-1")
	expectGetMe(t, calls, "good-1")

	// A running bot restarts with a new token.
	save("bad-2")
	expectGetMe(t, calls, "bad-2")

	// A failed start does not end the supervisor: the next save is used.
	save("good-3")
	expectGetMe(t, calls, "good-3")
}

// fakeTelegram is enough of Telegram to drive the bot: it hands out queued updates and
// records what the bot sends.
type fakeTelegram struct {
	allowedUpdates string
	mu             sync.Mutex
	updates        []string
	sent           []sentMessage
	nextID         int
	// refuseHTML makes Telegram turn down anything sent as HTML, as it does a bad tag.
	refuseHTML bool
	// tooLong is how many messages over Telegram's limit it was asked to send.
	tooLong int
	deleted []string
	menus   map[string][]string
	// refuseMenus is how many times more it turns down a command menu.
	refuseMenus int
}

// sentMessage is one call the bot made that shows the user something.
type sentMessage struct {
	Method    string
	ChatID    string
	Text      string
	ParseMode string
	Markup    string
	MessageID string
}

func newFakeTelegram(t *testing.T) *fakeTelegram {
	t.Helper()
	f := &fakeTelegram{nextID: 100, menus: map[string][]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		_ = r.ParseMultipartForm(1 << 20)
		w.Header().Set("Content-Type", "application/json")
		switch method {
		case "getMe":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Vergex","username":"vergex_test_bot"}}`))
		case "getUpdates":
			f.mu.Lock()
			f.allowedUpdates = r.FormValue("allowed_updates")
			batch := f.updates
			f.updates = nil
			f.mu.Unlock()
			if len(batch) == 0 {
				time.Sleep(5 * time.Millisecond)
			}
			_, _ = w.Write([]byte(`{"ok":true,"result":[` + strings.Join(batch, ",") + `]}`))
		case "sendMessage", "editMessageText", "editMessageReplyMarkup":
			f.mu.Lock()
			if len(utf16.Encode([]rune(r.FormValue("text")))) > 4096 {
				f.tooLong++
				f.mu.Unlock()
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: message is too long"}`))
				return
			}
			if f.refuseHTML && r.FormValue("parse_mode") == "HTML" {
				f.mu.Unlock()
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: can't parse entities"}`))
				return
			}
			f.nextID++
			id := f.nextID
			f.sent = append(f.sent, sentMessage{
				Method: method, ChatID: r.FormValue("chat_id"), Text: r.FormValue("text"),
				ParseMode: r.FormValue("parse_mode"), Markup: r.FormValue("reply_markup"), MessageID: r.FormValue("message_id"),
			})
			f.mu.Unlock()
			_, _ = fmt.Fprintf(w, `{"ok":true,"result":{"message_id":%d,"chat":{"id":1,"type":"private"},"date":1}}`, id)
		case "setMyCommands", "deleteMyCommands":
			f.mu.Lock()
			if method == "setMyCommands" && f.refuseMenus > 0 {
				f.refuseMenus--
				f.mu.Unlock()
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(`{"ok":false,"error_code":502,"description":"Bad Gateway"}`))
				return
			}
			f.menus[method] = append(f.menus[method], r.FormValue("commands")+" "+r.FormValue("scope"))
			f.mu.Unlock()
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		case "deleteMessage":
			f.mu.Lock()
			f.deleted = append(f.deleted, r.FormValue("message_id"))
			f.mu.Unlock()
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		default:
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		}
	}))
	t.Cleanup(srv.Close)
	orig := apiEndpoint
	apiEndpoint = srv.URL + "/bot%s/%s"
	t.Cleanup(func() { apiEndpoint = orig })
	return f
}

var fakeUpdateID atomic.Int64

// say queues a text message from a chat, by the user whose chat it is. chatType is private,
// group and so on.
func (f *fakeTelegram) say(chatID int64, chatType, username, text string) {
	f.sayAs(chatID, chatID, chatType, username, text)
}

// sayAs queues a text message from one user in a chat.
func (f *fakeTelegram) sayAs(from, chatID int64, chatType, username, text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	raw, _ := json.Marshal(text)
	f.updates = append(f.updates, fmt.Sprintf(
		`{"update_id":%d,"message":{"message_id":1,"date":1,"from":{"id":%d,"is_bot":false,"first_name":"x","username":%q},"chat":{"id":%d,"type":%q},"text":%s}}`,
		fakeUpdateID.Add(1), from, username, chatID, chatType, raw))
}

// press queues a button press, from a person in a chat.
func (f *fakeTelegram) press(from, chatID int64, messageID int, data string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	raw, _ := json.Marshal(data)
	f.updates = append(f.updates, fmt.Sprintf(
		`{"update_id":%d,"callback_query":{"id":"q","from":{"id":%d,"is_bot":false,"first_name":"x"},"message":{"message_id":%d,"date":1,"chat":{"id":%d,"type":"private"}},"chat_instance":"c","data":%s}}`,
		fakeUpdateID.Add(1), from, messageID, chatID, raw))
}

func (f *fakeTelegram) menuCalls(method string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.menus[method]...)
}

// sentTo lists what was sent to a chat so far.
func (f *fakeTelegram) sentTo(chatID int64) []sentMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []sentMessage
	for _, m := range f.sent {
		if m.ChatID == fmt.Sprint(chatID) {
			out = append(out, m)
		}
	}
	return out
}

// waitFor polls until cond holds.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// runningBot starts the bot on a fake Telegram, with a registered user and a token saved,
// and stops it when the test ends.
func runningBot(t *testing.T, st *Stores) *fakeTelegram {
	t.Helper()
	return runningBotOn(t, st, 0)
}

// runningBotOn is runningBot with the skills' API on a port of the test's.
func runningBotOn(t *testing.T, st *Stores, port int) *fakeTelegram {
	t.Helper()
	f := newFakeTelegram(t)
	startBot(t, st, f, port)
	return f
}

// startBot runs the bot on a fake Telegram until the test ends.
func startBot(t *testing.T, st *Stores, f *fakeTelegram, port int) {
	t.Helper()
	auth.SetJWTSecret("test-secret-for-the-telegram-bot")
	if err := st.Users.Create(&user.User{ID: testdb.UserID, Email: "alice@example.com", PasswordHash: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Telegram.Save("good-token", ""); err != nil {
		t.Fatal(err)
	}
	reload := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runBot("good-token", &config.Config{APIServerPort: port}, st, reload)
	}()
	t.Cleanup(func() {
		reload <- struct{}{}
		<-done
	})
}

const (
	ownerChat    = int64(1001)
	strangerChat = int64(2002)
	groupChat    = int64(-3003)
)

func TestOnlyTheLinkBindsAChat(t *testing.T) {
	st := NewStores(testdb.Open(t))
	f := runningBot(t, st)
	bound := func() int64 {
		id, err := st.Telegram.GetBoundChatID()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}

	// A bare /start, a wrong code and a used-up one bind nothing.
	f.say(strangerChat, "private", "mallory", "/start")
	waitFor(t, "an answer to /start", func() bool { return len(f.sentTo(strangerChat)) == 1 })
	f.say(strangerChat, "private", "mallory", "/start wrong-code")
	waitFor(t, "an answer to a wrong code", func() bool { return len(f.sentTo(strangerChat)) == 2 })
	if bound() != 0 {
		t.Fatal("a chat bound without the code")
	}

	// The code in a group chat does nothing, and does not use the code up.
	link, err := NewBindLink(context.Background(), st.Telegram, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if want := "https://t.me/vergex_test_bot?start=" + link.Code; link.Link != want {
		t.Fatalf("link = %q, want %q", link.Link, want)
	}
	f.say(groupChat, "group", "mallory", "/start "+link.Code)

	// The code in a private chat binds it.
	f.say(ownerChat, "private", "alice", "/start "+link.Code)
	waitFor(t, "the binding", func() bool { return bound() == ownerChat })
	if len(f.sentTo(groupChat)) != 0 {
		t.Fatal("the bot answered a group chat")
	}

	// It works once.
	if useBindCode(link.Code, time.Now()) {
		t.Fatal("a code worked twice")
	}
}

func TestClearingTheTokenStartsOverAndTakesTheCodeWithIt(t *testing.T) {
	st := NewStores(testdb.Open(t))
	newFakeTelegram(t)
	if err := st.Telegram.Save("good-token", ""); err != nil {
		t.Fatal(err)
	}
	link, err := NewBindLink(context.Background(), st.Telegram, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if err := ClearToken(st.Telegram); err != nil {
		t.Fatal(err)
	}
	cfg, err := st.Telegram.Get()
	if err != nil || cfg.BotToken != "" {
		t.Fatalf("config = %v, err = %v", cfg, err)
	}
	if useBindCode(link.Code, time.Now()) {
		t.Fatal("the code outlived the bot it was for")
	}
	if _, err := NewBindLink(context.Background(), st.Telegram, "", time.Now()); !errors.Is(err, ErrNoToken) {
		t.Fatalf("err = %v, want no link without a token", err)
	}

	// While someone is bound, unbinding has to come first.
	if err := st.Telegram.Save("good-token", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.Telegram.BindUser(ownerChat, "@alice"); err != nil {
		t.Fatal(err)
	}
	if err := ClearToken(st.Telegram); !errors.Is(err, ErrAlreadyBound) {
		t.Fatalf("err = %v, want it refused while someone is bound", err)
	}
	if cfg, err := st.Telegram.Get(); err != nil || cfg.BotToken != "good-token" {
		t.Fatalf("config = %v, err = %v", cfg, err)
	}
}

func TestPressingStartLeadsToSendingTheCode(t *testing.T) {
	st := NewStores(testdb.Open(t))
	f := runningBot(t, st)
	bound := func() int64 {
		id, err := st.Telegram.GetBoundChatID()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}

	// Start is met with what to do next.
	f.say(ownerChat, "private", "alice", "/start")
	waitFor(t, "the answer to Start", func() bool { return len(f.sentTo(ownerChat)) == 1 })
	if text := f.sentTo(ownerChat)[0].Text; !strings.Contains(text, "Not bound") || !strings.Contains(text, "Send the code here") {
		t.Fatalf("heard %q", text)
	}

	// A code that has run out says so, where an ordinary message does not.
	f.say(ownerChat, "private", "alice", strings.Repeat("a", 32))
	waitFor(t, "the answer to a stale code", func() bool { return len(f.sentTo(ownerChat)) == 2 })
	if text := f.sentTo(ownerChat)[1].Text; !strings.Contains(text, "Code not valid or expired") {
		t.Fatalf("heard %q", text)
	}
	f.say(ownerChat, "private", "alice", "hello")
	waitFor(t, "the answer to a message", func() bool { return len(f.sentTo(ownerChat)) == 3 })
	if text := f.sentTo(ownerChat)[2].Text; !strings.Contains(text, "Not bound") {
		t.Fatalf("heard %q", text)
	}

	link, err := NewBindLink(context.Background(), st.Telegram, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	// Sent on its own it does nothing in a group, and is not kept.
	f.say(groupChat, "group", "mallory", link.Code)
	waitFor(t, "the group's message", func() bool {
		visitors, err := st.Visitors.List()
		if err != nil {
			t.Fatal(err)
		}
		for _, visitor := range visitors {
			if visitor.ChatID == groupChat {
				if visitor.LastText != "" {
					t.Fatalf("kept %q", visitor.LastText)
				}
				return true
			}
		}
		return false
	})

	// In a private chat it binds it, and welcomes them.
	f.say(ownerChat, "private", "alice", link.Code)
	waitFor(t, "the binding", func() bool { return bound() == ownerChat })
	waitFor(t, "the welcome", func() bool {
		got := f.sentTo(ownerChat)
		return strings.Contains(got[len(got)-1].Text, "Welcome to Vergex")
	})
}

func TestStartAnswersEveryTimeWithWhereYouStand(t *testing.T) {
	st := NewStores(testdb.Open(t))
	if err := st.Telegram.BindUser(ownerChat, "@alice"); err != nil {
		t.Fatal(err)
	}
	f := runningBot(t, st)

	// Someone else hears they have no access each time they press it, not only the first.
	for want := 1; want <= 2; want++ {
		f.say(strangerChat, "private", "mallory", "/start")
		waitFor(t, "the refusal", func() bool { return len(f.sentTo(strangerChat)) == want })
		if text := f.sentTo(strangerChat)[want-1].Text; !strings.Contains(text, "Access denied") {
			t.Fatalf("heard %q", text)
		}
	}

	// The owner is welcomed.
	f.say(ownerChat, "private", "alice", "/start")
	waitFor(t, "the welcome", func() bool {
		for _, m := range f.sentTo(ownerChat) {
			if strings.Contains(m.Text, "Welcome to Vergex") {
				return true
			}
		}
		return false
	})
}

func TestBindCodeExpiresAndANewOneReplacesIt(t *testing.T) {
	st := NewStores(testdb.Open(t))
	runningBot(t, st)
	now := time.Now()
	first, err := NewBindLink(context.Background(), st.Telegram, "", now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewBindLink(context.Background(), st.Telegram, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if useBindCode(first.Code, now) {
		t.Fatal("a replaced code still worked")
	}
	if useBindCode(second.Code, now.Add(bindCodeLife+time.Second)) {
		t.Fatal("an expired code still worked")
	}
}

func TestStrangersReachNothingOnceAChatIsBound(t *testing.T) {
	st := NewStores(testdb.Open(t))
	if err := st.Telegram.BindUser(ownerChat, "@alice"); err != nil {
		t.Fatal(err)
	}
	f := runningBot(t, st)

	// /lang from a stranger is refused, and the owner's next message is not read as a
	// choice of language.
	f.say(strangerChat, "private", "mallory", "/lang")
	waitFor(t, "the refusal", func() bool { return len(f.sentTo(strangerChat)) == 1 })
	if text := f.sentTo(strangerChat)[0].Text; !strings.Contains(text, "Private bot") {
		t.Fatalf("stranger heard %q", text)
	}
	waitFor(t, "the owner hearing of it", func() bool { return len(f.sentTo(ownerChat)) == 1 })
	if text := f.sentTo(ownerChat)[0].Text; !strings.Contains(text, "mallory") {
		t.Fatalf("owner heard %q", text)
	}

	f.say(ownerChat, "private", "alice", "/help")
	waitFor(t, "an answer to the owner", func() bool { return len(f.sentTo(ownerChat)) >= 2 })

	// Asking again changes nothing and sends nothing more.
	f.say(strangerChat, "private", "mallory", "/help")
	f.say(ownerChat, "private", "alice", "/help")
	waitFor(t, "the owner's help", func() bool { return len(f.sentTo(ownerChat)) >= 3 })
	if got := f.sentTo(strangerChat); len(got) != 1 {
		t.Fatalf("stranger got %d messages, want 1", len(got))
	}

	// The owner can, by a button that a stranger pressing changes nothing with.
	f.say(ownerChat, "private", "alice", "/lang")
	waitFor(t, "the language menu", func() bool {
		got := f.sentTo(ownerChat)
		return strings.Contains(got[len(got)-1].Markup, "lang:zh")
	})
	f.press(strangerChat, ownerChat, 1, "lang:zh")
	time.Sleep(50 * time.Millisecond)
	if st.Telegram.GetLanguage() == "zh" {
		t.Fatal("a stranger changed the language")
	}
	f.press(ownerChat, ownerChat, 1, "lang:zh")
	waitFor(t, "the language change", func() bool { return st.Telegram.GetLanguage() == "zh" })

	// So can naming it.
	f.say(ownerChat, "private", "alice", "/lang en")
	waitFor(t, "the language change", func() bool { return st.Telegram.GetLanguage() == "en" })
}

func TestOnlyTheBoundUserInTheirOwnChatGetsThrough(t *testing.T) {
	st := NewStores(testdb.Open(t))
	if err := st.Telegram.BindUser(ownerChat, "@alice"); err != nil {
		t.Fatal(err)
	}
	f := runningBot(t, st)

	// The right chat is not enough: the sender has to be the one who bound it.
	f.sayAs(strangerChat, ownerChat, "private", "mallory", "/help")
	waitFor(t, "the refusal", func() bool { return len(f.sentTo(ownerChat)) > 0 })
	for _, m := range f.sentTo(ownerChat) {
		if strings.Contains(m.Text, "/status") {
			t.Fatalf("someone else in the chat was answered: %q", m.Text)
		}
	}

	// While someone is bound, no new link can be asked for.
	if _, err := NewBindLink(context.Background(), st.Telegram, "", time.Now()); !errors.Is(err, ErrAlreadyBound) {
		t.Fatalf("err = %v, want it refused while someone is bound", err)
	}
}

func TestAnUnbindingHoldsAtOnceAndANewBindingNamesWhoItIs(t *testing.T) {
	st := NewStores(testdb.Open(t))
	if err := st.Telegram.BindUser(ownerChat, "@alice"); err != nil {
		t.Fatal(err)
	}
	f := runningBot(t, st)
	f.say(ownerChat, "private", "alice", "/help")
	waitFor(t, "the owner's help", func() bool { return len(f.sentTo(ownerChat)) == 1 })

	// Nothing has told the running bot yet.
	if err := st.Telegram.Unbind(); err != nil {
		t.Fatal(err)
	}
	f.say(ownerChat, "private", "alice", "/help")
	waitFor(t, "the refusal", func() bool { return len(f.sentTo(ownerChat)) == 2 })
	if text := f.sentTo(ownerChat)[1].Text; !strings.Contains(text, "Private bot") {
		t.Fatalf("heard %q", text)
	}

	// Binding again goes by the code, and someone with no @username goes by their name.
	link, err := NewBindLink(context.Background(), st.Telegram, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	f.say(ownerChat, "private", "", "/start "+link.Code)
	waitFor(t, "the binding", func() bool {
		id, err := st.Telegram.GetBoundChatID()
		return err == nil && id == ownerChat
	})
	cfg, err := st.Telegram.Get()
	if err != nil || cfg.Username != "x" {
		t.Fatalf("name = %q, err = %v", cfg.Username, err)
	}
}

func TestOnlyWhatIsHandledIsAskedFor(t *testing.T) {
	st := NewStores(testdb.Open(t))
	f := runningBot(t, st)
	waitFor(t, "the first poll", func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.allowedUpdates != ""
	})
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.allowedUpdates != `["message","callback_query"]` {
		t.Fatalf("asked for %s", f.allowedUpdates)
	}
}

func TestARefusalIsStructuredAndInEnglish(t *testing.T) {
	st := NewStores(testdb.Open(t))
	f := runningBot(t, st)

	// Before anyone is bound, it says how.
	f.say(strangerChat, "private", "mallory", "/start")
	waitFor(t, "the answer to /start", func() bool { return len(f.sentTo(strangerChat)) == 1 })
	unbound := f.sentTo(strangerChat)[0]
	for _, want := range []string{"🔒 <b>Private bot</b>", "<i>Not bound · binding code from Settings → Telegram</i>", fmt.Sprintf("• Your ID: <code>%d</code>", strangerChat), " UTC</b>"} {
		if !strings.Contains(unbound.Text, want) {
			t.Fatalf("no %q in %q", want, unbound.Text)
		}
	}
	if unbound.ParseMode != "HTML" {
		t.Fatalf("parse mode = %q", unbound.ParseMode)
	}

	if err := st.Telegram.BindUser(ownerChat, "@alice"); err != nil {
		t.Fatal(err)
	}
	f.say(strangerChat+1, "private", "mallory", "hello")
	waitFor(t, "the refusal", func() bool { return len(f.sentTo(strangerChat+1)) == 1 })
	refused := f.sentTo(strangerChat + 1)[0].Text
	if !strings.Contains(refused, "<i>Access denied · bound owner only</i>") || !strings.Contains(refused, "<i>This attempt was recorded</i>") {
		t.Fatalf("refused with %q", refused)
	}
	for _, text := range []string{unbound.Text, refused} {
		for _, r := range text {
			if r >= 0x4E00 && r <= 0x9FFF {
				t.Fatalf("not only English: %q", text)
			}
		}
	}
}

func TestEveryAttemptIsKeptForSettingsToShow(t *testing.T) {
	st := NewStores(testdb.Open(t))
	if err := st.Telegram.BindUser(ownerChat, "@alice"); err != nil {
		t.Fatal(err)
	}
	f := runningBot(t, st)
	attempts := func(chatID int64) Visitor {
		visitors, err := st.Visitors.List()
		if err != nil {
			t.Fatal(err)
		}
		for _, visitor := range visitors {
			if visitor.ChatID == chatID {
				return visitor
			}
		}
		return Visitor{}
	}

	// A message gets one answer a day and Start always does, and every one counts. What follows
	// /start stays out.
	f.say(strangerChat, "private", "mallory", "hello")
	f.say(strangerChat, "private", "mallory", "/start some-code")
	f.say(groupChat, "group", "mallory", "/status")
	waitFor(t, "all three", func() bool { return attempts(strangerChat).Attempts == 2 && attempts(groupChat).Attempts == 1 })
	stranger := attempts(strangerChat)
	if stranger.SenderID != strangerChat || stranger.Username != "mallory" || stranger.Name != "x" || stranger.ChatType != "private" || stranger.LastText != "/start" {
		t.Fatalf("visitor = %+v", stranger)
	}
	if stranger.FirstAt == 0 || stranger.LastAt < stranger.FirstAt {
		t.Fatalf("times = %d, %d", stranger.FirstAt, stranger.LastAt)
	}
	waitFor(t, "both answers", func() bool { return len(f.sentTo(strangerChat)) == 2 })

	// The owner is not a visitor.
	f.say(ownerChat, "private", "alice", "/help")
	waitFor(t, "the owner's help", func() bool {
		for _, m := range f.sentTo(ownerChat) {
			if strings.Contains(m.Text, "/status") {
				return true
			}
		}
		return false
	})
	if attempts(ownerChat).Attempts != 0 {
		t.Fatal("the owner was recorded")
	}

	// Binding makes a visitor the owner, and takes them off the list.
	if err := st.Telegram.Unbind(); err != nil {
		t.Fatal(err)
	}
	link, err := NewBindLink(context.Background(), st.Telegram, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	f.say(strangerChat, "private", "mallory", "/start "+link.Code)
	waitFor(t, "the binding", func() bool {
		id, err := st.Telegram.GetBoundChatID()
		return err == nil && id == strangerChat
	})
	waitFor(t, "the visitor forgotten", func() bool { return attempts(strangerChat).Attempts == 0 })
}

func TestTheVisitorListKeepsTheNewestAndAShortText(t *testing.T) {
	orig := maxVisitors
	maxVisitors = 2
	t.Cleanup(func() { maxVisitors = orig })
	store := NewVisitorStore(testdb.Open(t))
	now := time.Now()
	for i := int64(1); i <= 3; i++ {
		if err := store.Record(Visitor{ChatID: i, SenderID: i, ChatType: "private"}, now.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	visitors, err := store.List()
	if err != nil || len(visitors) != 2 || visitors[0].ChatID != 3 || visitors[1].ChatID != 2 {
		t.Fatalf("visitors = %+v, err = %v", visitors, err)
	}

	long := strings.Repeat("字", visitorTextRunes+50)
	if got := visitorText(&tgbotapi.Message{Text: long}); len([]rune(got)) != visitorTextRunes {
		t.Fatalf("kept %d runes", len([]rune(got)))
	}
	if got := visitorText(&tgbotapi.Message{Caption: "a photo"}); got != "a photo" {
		t.Fatalf("caption = %q", got)
	}
}

func TestStartIsInEveryPrivateMenuAndTheRestOnlyInTheOwners(t *testing.T) {
	st := NewStores(testdb.Open(t))
	f := runningBot(t, st)
	waitFor(t, "the first menu", func() bool { return len(f.menuCalls("setMyCommands")) == 1 })
	call := f.menuCalls("setMyCommands")[0]
	if !strings.Contains(call, `"type":"all_private_chats"`) || !strings.Contains(call, `"command":"start"`) || strings.Contains(call, `"command":"positions"`) {
		t.Fatalf("with no chat bound: %s", call)
	}

	link, err := NewBindLink(context.Background(), st.Telegram, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	f.say(ownerChat, "private", "alice", "/start "+link.Code)
	waitFor(t, "the owner's menu", func() bool { return len(f.menuCalls("setMyCommands")) == 3 })
	calls := f.menuCalls("setMyCommands")
	call = calls[len(calls)-1]
	for _, want := range []string{fmt.Sprintf(`"chat_id":%d`, ownerChat), `"type":"chat"`, `"command":"positions"`, `"command":"stop"`} {
		if !strings.Contains(call, want) {
			t.Fatalf("menu call is missing %q: %s", want, call)
		}
	}
}

func TestTheBotRunsOnlyOnceItsCommandsAreIn(t *testing.T) {
	origDelay := menuRetryDelay
	menuRetryDelay = 10 * time.Millisecond
	t.Cleanup(func() { menuRetryDelay = origDelay })
	state := func() service.Entry {
		for _, entry := range service.All() {
			if entry.Name == service.Telegram {
				return entry
			}
		}
		return service.Entry{}
	}
	st := NewStores(testdb.Open(t))
	service.Report(service.Telegram, service.Off, "")

	// Telegram refuses until the bot has said so, and then lets its retries through.
	f := newFakeTelegram(t)
	f.refuseMenus = 1 << 30
	startBot(t, st, f, 0)
	waitFor(t, "the refusal reported", func() bool {
		entry := state()
		return entry.State == service.Degraded && strings.Contains(entry.Detail, "commands not published")
	})
	f.mu.Lock()
	f.refuseMenus = 0
	f.mu.Unlock()
	waitFor(t, "the retry", func() bool { return len(f.menuCalls("setMyCommands")) == 1 && state().State == service.Running })
}

func TestReadCommandsDoNotCallTheModel(t *testing.T) {
	st := NewStores(testdb.Open(t))
	if err := st.Telegram.BindUser(ownerChat, "@alice"); err != nil {
		t.Fatal(err)
	}
	client := &answeringClient{answer: "from the model"}
	replaceClient(t, client)
	port, _ := skillsAPI(t)
	f := runningBotOn(t, st, port)
	f.say(ownerChat, "private", "alice", "/traders")
	waitFor(t, "the answer", func() bool {
		for _, m := range f.sentTo(ownerChat) {
			if strings.Contains(m.Text, "ETH Grid") && strings.Contains(m.Text, "running") {
				return true
			}
		}
		return false
	})
	if client.turn.Load() != 0 {
		t.Fatal("/traders called the model")
	}
}

func TestABindingToAGroupCountsAsNone(t *testing.T) {
	st := NewStores(testdb.Open(t))
	if err := st.Telegram.BindUser(groupChat, ""); err != nil {
		t.Fatal(err)
	}
	f := runningBot(t, st)
	f.say(groupChat, "group", "alice", "/help")
	f.say(ownerChat, "private", "alice", "/help")
	waitFor(t, "the refusal", func() bool { return len(f.sentTo(ownerChat)) == 1 })
	if text := f.sentTo(ownerChat)[0].Text; !strings.Contains(text, "Private bot") {
		t.Fatalf("heard %q", text)
	}
	if len(f.sentTo(groupChat)) != 0 {
		t.Fatal("the bot answered the group it was once bound to")
	}
}

// holdingClient asks to stop a trader, then answers.
type holdingClient struct {
	scriptedClient
	turns atomic.Int32
}

func (c *holdingClient) CallWithRequestFull(*agent.Request) (*agent.Response, error) {
	if c.turns.Add(1)%2 == 1 {
		call := agent.ToolCall{ID: "call_1", Type: "function"}
		call.Function.Name = "trader_management"
		call.Function.Arguments = `{"action":"stop","target_ref":{"name":"ETH Grid"}}`
		return &agent.Response{ToolCalls: []agent.ToolCall{call}}, nil
	}
	return &agent.Response{Content: "I have asked the system to stop it."}, nil
}

// skillsAPI answers what a stop needs, and records whether the stop happened.
func skillsAPI(t *testing.T) (port int, stops *atomic.Int32) {
	t.Helper()
	stops = &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/traders":
			_, _ = w.Write([]byte(`[{"trader_id":"t2","trader_name":"ETH Grid","running":true,"kind":"grid","account_id":"ex1"}]`))
		case "/api/accounts":
			_, _ = w.Write([]byte(`[{"id":"ex1","venue":"binance","name":"Main"}]`))
		case "/api/traders/t2/stop":
			stops.Add(1)
			_, _ = w.Write([]byte(`{}`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	t.Cleanup(srv.Close)
	_, _ = fmt.Sscanf(srv.Listener.Addr().String(), "127.0.0.1:%d", &port)
	return port, stops
}

// confirmRequest waits for the message with the buttons and returns its ID and batch.
func confirmRequest(t *testing.T, f *fakeTelegram, skip int) (messageID int, batch string, text string) {
	t.Helper()
	var found sentMessage
	waitFor(t, "the request to confirm", func() bool {
		seen := 0
		for _, m := range f.sentTo(ownerChat) {
			if strings.Contains(m.Markup, "confirm:") {
				if seen == skip {
					found = m
					return true
				}
				seen++
			}
		}
		return false
	})
	if found.ParseMode != "HTML" {
		t.Fatalf("parse mode = %q", found.ParseMode)
	}
	at := strings.Index(found.Markup, "confirm:") + len("confirm:")
	batch = found.Markup[at : at+8]
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, m := range f.sent {
		if m.Markup == found.Markup {
			messageID = 101 + i
		}
	}
	return messageID, batch, found.Text
}

func replaceClient(t *testing.T, client agent.Client) {
	t.Helper()
	orig := newClient
	newClient = func(*Stores, string) agent.Client { return client }
	t.Cleanup(func() { newClient = orig })
}

func TestButtonsConfirmOnlyForTheOwnerAndOnlyTheirBatch(t *testing.T) {
	st := NewStores(testdb.Open(t))
	if err := st.Telegram.BindUser(ownerChat, "@alice"); err != nil {
		t.Fatal(err)
	}
	replaceClient(t, &holdingClient{})
	port, stops := skillsAPI(t)
	f := runningBotOn(t, st, port)

	f.say(ownerChat, "private", "alice", "stop eth grid")
	messageID, batch, text := confirmRequest(t, f, 0)
	if !strings.Contains(text, "⚠️ <b>Waiting for your confirmation · real money</b>") || !strings.Contains(text, "<b>Stop trader</b> · ETH Grid\n• Account: <b>Main · Real money</b>") {
		t.Fatalf("request to confirm = %q", text)
	}

	// Someone else pressing it, even with the right data, does nothing. Nor does a button
	// from another batch.
	f.press(strangerChat, ownerChat, messageID, "confirm:"+batch)
	f.press(strangerChat, strangerChat, messageID, "confirm:"+batch)
	f.press(ownerChat, ownerChat, messageID, "confirm:deadbeef")
	waitFor(t, "the stale button's answer", func() bool {
		for _, m := range f.sentTo(ownerChat) {
			if strings.Contains(m.Text, "no longer valid") {
				return true
			}
		}
		return false
	})
	if stops.Load() != 0 {
		t.Fatal("a stop ran without the owner's press")
	}

	// The right press from the owner runs it and says so.
	f.press(ownerChat, ownerChat, messageID, "confirm:"+batch)
	waitFor(t, "the stop", func() bool { return stops.Load() == 1 })
	waitFor(t, "the result", func() bool {
		for _, m := range f.sentTo(ownerChat) {
			if strings.Contains(m.Text, "Done") && strings.Contains(m.Text, "ETH Grid") {
				return true
			}
		}
		return false
	})

	// Pressing it again runs nothing more.
	f.press(ownerChat, ownerChat, messageID, "confirm:"+batch)
	time.Sleep(50 * time.Millisecond)
	if stops.Load() != 1 {
		t.Fatalf("stops = %d", stops.Load())
	}
}

func TestAnsweringInWordsTakesTheButtonsAway(t *testing.T) {
	st := NewStores(testdb.Open(t))
	if err := st.Telegram.BindUser(ownerChat, "@alice"); err != nil {
		t.Fatal(err)
	}
	replaceClient(t, &holdingClient{})
	port, stops := skillsAPI(t)
	f := runningBotOn(t, st, port)

	f.say(ownerChat, "private", "alice", "stop eth grid")
	confirmRequest(t, f, 0)
	f.say(ownerChat, "private", "alice", "confirm")
	waitFor(t, "the stop", func() bool { return stops.Load() == 1 })
	waitFor(t, "the buttons going", func() bool {
		for _, m := range f.sentTo(ownerChat) {
			if m.Method == "editMessageReplyMarkup" {
				return true
			}
		}
		return false
	})
}

// answeringClient always answers with the same text.
type answeringClient struct {
	scriptedClient
	answer string
}

func (c *answeringClient) CallWithRequestFull(*agent.Request) (*agent.Response, error) {
	return &agent.Response{Content: c.answer}, nil
}

func boundBot(t *testing.T, answer string) (*fakeTelegram, *Stores) {
	t.Helper()
	st := NewStores(testdb.Open(t))
	if err := st.Telegram.BindUser(ownerChat, "@alice"); err != nil {
		t.Fatal(err)
	}
	replaceClient(t, &answeringClient{answer: answer})
	return runningBot(t, st), st
}

func TestALongReplyArrivesInPartsAndNeverStaysOnThePlaceholder(t *testing.T) {
	f, _ := boundBot(t, strings.Repeat("**A** line of the answer, long enough to matter.\n\n", 400))
	f.say(ownerChat, "private", "alice", "tell me everything")
	waitFor(t, "more than one part", func() bool { return len(f.sentTo(ownerChat)) >= 4 })
	time.Sleep(50 * time.Millisecond)

	got := f.sentTo(ownerChat)
	if got[0].Text != "⏳" {
		t.Fatalf("first message = %q", got[0].Text)
	}
	parts := 0
	for _, m := range got[1:] {
		if m.ParseMode != "HTML" || !strings.Contains(m.Text, "<b>A</b>") {
			continue
		}
		if parts == 0 && (m.Method != "editMessageText" || m.MessageID != "101" || m.Markup != "") {
			t.Fatalf("the first part should replace the placeholder and its button: %+v", m)
		}
		parts++
	}
	if parts < 2 {
		t.Fatalf("%d parts", parts)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.tooLong != 0 {
		t.Fatalf("%d messages were over the limit", f.tooLong)
	}
}

func TestAReplyTelegramRefusesAsHTMLGoesOutAsText(t *testing.T) {
	f, _ := boundBot(t, "**bold** answer")
	f.mu.Lock()
	f.refuseHTML = true
	f.mu.Unlock()
	f.say(ownerChat, "private", "alice", "hello")
	waitFor(t, "the reply", func() bool {
		for _, m := range f.sentTo(ownerChat) {
			if m.Method == "editMessageText" && m.Text == "**bold** answer" && m.ParseMode == "" {
				return true
			}
		}
		return false
	})
}

func TestAnEmptyAnswerSaysSoInsteadOfStayingOnThePlaceholder(t *testing.T) {
	f, _ := boundBot(t, "   ")
	f.say(ownerChat, "private", "alice", "hello")
	waitFor(t, "the notice", func() bool {
		for _, m := range f.sentTo(ownerChat) {
			if m.Method == "editMessageText" && strings.Contains(m.Text, "returned nothing") {
				return true
			}
		}
		return false
	})
}

// slowClient answers each message in its order, after waiting to be told to.
type slowClient struct {
	scriptedClient
	started chan string
	release chan struct{}
}

func (c *slowClient) CallWithRequestFull(req *agent.Request) (*agent.Response, error) {
	last := req.Messages[len(req.Messages)-1].Content
	c.started <- last
	select {
	case <-c.release:
	case <-req.Ctx.Done():
		return nil, req.Ctx.Err()
	}
	return &agent.Response{Content: "answered"}, nil
}

func boundBotWith(t *testing.T, client agent.Client) (*fakeTelegram, *Stores) {
	t.Helper()
	st := NewStores(testdb.Open(t))
	if err := st.Telegram.BindUser(ownerChat, "@alice"); err != nil {
		t.Fatal(err)
	}
	replaceClient(t, client)
	return runningBot(t, st), st
}

func TestMessagesAreAnsweredInTheOrderTheyCame(t *testing.T) {
	client := &slowClient{started: make(chan string, 8), release: make(chan struct{}, 8)}
	f, _ := boundBotWith(t, client)
	for _, text := range []string{"first", "second", "third"} {
		f.say(ownerChat, "private", "alice", text)
	}
	for _, want := range []string{"first", "second", "third"} {
		select {
		case got := <-client.started:
			if !strings.Contains(got, want) {
				t.Fatalf("got %q, want %q", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%q never started", want)
		}
		client.release <- struct{}{}
	}
}

func TestSlashStopStopsTheTurnAndWhatWaitsBehindIt(t *testing.T) {
	client := &slowClient{started: make(chan string, 8), release: make(chan struct{}, 8)}
	f, _ := boundBotWith(t, client)
	f.say(ownerChat, "private", "alice", "first")
	f.say(ownerChat, "private", "alice", "second")
	<-client.started
	waitFor(t, "the placeholder", func() bool { return len(f.sentTo(ownerChat)) >= 1 })
	f.say(ownerChat, "private", "alice", "/stop")
	waitFor(t, "the turn stopping", func() bool {
		for _, m := range f.sentTo(ownerChat) {
			if strings.Contains(m.Text, "Stopped") {
				return true
			}
		}
		return false
	})
	select {
	case got := <-client.started:
		t.Fatalf("%q ran after /stop", got)
	case <-time.After(100 * time.Millisecond):
	}

	// With nothing under way, it says so.
	f.say(ownerChat, "private", "alice", "/stop")
	waitFor(t, "the answer", func() bool {
		for _, m := range f.sentTo(ownerChat) {
			if strings.Contains(m.Text, "Nothing is running") {
				return true
			}
		}
		return false
	})
}

func TestThePlaceholderSaysWhatIsUnderWayAndCanBeStopped(t *testing.T) {
	client := &slowClient{started: make(chan string, 8), release: make(chan struct{}, 8)}
	f, _ := boundBotWith(t, client)
	f.say(ownerChat, "private", "alice", "first")
	<-client.started
	waitFor(t, "the placeholder to say what is happening", func() bool {
		for _, m := range f.sentTo(ownerChat) {
			if m.Method == "editMessageText" && strings.Contains(m.Text, "Thinking") && strings.Contains(m.Markup, "stop") {
				return true
			}
		}
		return false
	})
	// The owner's Stop button ends it; someone else's does nothing.
	f.press(strangerChat, ownerChat, 101, "stop")
	time.Sleep(50 * time.Millisecond)
	for _, m := range f.sentTo(ownerChat) {
		if strings.Contains(m.Text, "Stopped") {
			t.Fatal("a stranger's button stopped the turn")
		}
	}
	f.press(ownerChat, ownerChat, 101, "stop")
	waitFor(t, "the turn stopping", func() bool {
		for _, m := range f.sentTo(ownerChat) {
			if strings.Contains(m.Text, "Stopped") {
				return true
			}
		}
		return false
	})
}

// panickingClient panics once, as a bug in a skill would.
type panickingClient struct {
	scriptedClient
	calls atomic.Int32
}

func (c *panickingClient) CallWithRequestFull(*agent.Request) (*agent.Response, error) {
	if c.calls.Add(1) == 1 {
		panic("a bug")
	}
	return &agent.Response{Content: "still here"}, nil
}

func TestAPanicEndsOnlyThatTurn(t *testing.T) {
	f, _ := boundBotWith(t, &panickingClient{})
	f.say(ownerChat, "private", "alice", "first")
	waitFor(t, "the apology", func() bool {
		for _, m := range f.sentTo(ownerChat) {
			if strings.Contains(m.Text, "Something went wrong") {
				return true
			}
		}
		return false
	})
	f.say(ownerChat, "private", "alice", "second")
	waitFor(t, "the next answer", func() bool {
		for _, m := range f.sentTo(ownerChat) {
			if strings.Contains(m.Text, "still here") {
				return true
			}
		}
		return false
	})
}

func TestRetryAfterIsReadFromTelegramsRefusal(t *testing.T) {
	refused := &tgbotapi.Error{Code: 429, Message: "Too Many Requests", ResponseParameters: tgbotapi.ResponseParameters{RetryAfter: 2}}
	if got := retryAfter(refused); got != 2*time.Second {
		t.Fatalf("retryAfter = %v", got)
	}
	huge := &tgbotapi.Error{Code: 429, ResponseParameters: tgbotapi.ResponseParameters{RetryAfter: 3600}}
	if got := retryAfter(huge); got != maxRetryAfter {
		t.Fatalf("a long wait should be capped: %v", got)
	}
	if got := retryAfter(errors.New("other")); got != 0 {
		t.Fatalf("retryAfter of another error = %v", got)
	}
}

func TestLanguageComesFromSettingsThenFromHowYouWrite(t *testing.T) {
	st := NewStores(testdb.Open(t))
	f := runningBot(t, st)
	link, err := NewBindLink(context.Background(), st.Telegram, "zh", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	f.say(ownerChat, "private", "alice", "/start "+link.Code)
	waitFor(t, "the language", func() bool { return st.Telegram.GetLanguage() == "zh" })
	waitFor(t, "a Chinese welcome", func() bool {
		for _, m := range f.sentTo(ownerChat) {
			if strings.Contains(m.Text, "需要先完成设置") {
				return true
			}
		}
		return false
	})
}

func TestWritingChineseSettlesALanguageNeverChosen(t *testing.T) {
	st := NewStores(testdb.Open(t))
	if err := st.Telegram.BindUser(ownerChat, "@alice"); err != nil {
		t.Fatal(err)
	}
	f := runningBot(t, st)
	f.say(ownerChat, "private", "alice", "hello")
	waitFor(t, "an answer", func() bool { return len(f.sentTo(ownerChat)) >= 1 })
	if st.Telegram.GetLanguage() != "en" {
		t.Fatal("English should not settle anything")
	}
	f.say(ownerChat, "private", "alice", "看看持仓")
	waitFor(t, "the language", func() bool { return st.Telegram.GetLanguage() == "zh" })

	// A choice made stays, whatever is written after it.
	f.say(ownerChat, "private", "alice", "/lang en")
	waitFor(t, "the choice", func() bool { return st.Telegram.GetLanguage() == "en" })
	f.say(ownerChat, "private", "alice", "看看持仓")
	time.Sleep(50 * time.Millisecond)
	if st.Telegram.GetLanguage() != "en" {
		t.Fatal("writing Chinese overrode a language that was chosen")
	}
}

func TestAWrongTokenIsRefusedAndANewBotStartsUnbound(t *testing.T) {
	st := NewStores(testdb.Open(t))
	srv := fakeBotAPI(t)
	_ = srv
	ctx := context.Background()
	if err := SaveToken(ctx, st.Telegram, "bad-token", ""); !errors.Is(err, ErrBadToken) {
		t.Fatalf("err = %v", err)
	}
	if cfg, err := st.Telegram.Get(); err == nil && cfg.BotToken != "" {
		t.Fatal("a wrong token was saved")
	}

	if err := SaveToken(ctx, st.Telegram, "good-1", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.Telegram.BindUser(ownerChat, "@alice"); err != nil {
		t.Fatal(err)
	}
	// The same token again keeps the binding, and another bot does not.
	if err := SaveToken(ctx, st.Telegram, "good-1", ""); err != nil {
		t.Fatal(err)
	}
	if id, _ := st.Telegram.GetBoundChatID(); id != ownerChat {
		t.Fatal("saving the same token dropped the binding")
	}
	if err := SaveToken(ctx, st.Telegram, "good-2", ""); err != nil {
		t.Fatal(err)
	}
	if id, _ := st.Telegram.GetBoundChatID(); id != 0 {
		t.Fatal("a new bot kept the old binding")
	}
}

func TestTheTokenStaysOutOfErrorsAndLogs(t *testing.T) {
	line := `Post "https://api.telegram.org/bot123456:AAbb_cc-DD/getUpdates": dial tcp: i/o timeout`
	if got := scrubToken(line); strings.Contains(got, "AAbb") || !strings.Contains(got, "i/o timeout") {
		t.Fatalf("scrubbed = %q", got)
	}
}

// streamingClient writes its answer in pieces, and waits to be told to finish.
type streamingClient struct {
	scriptedClient
	release chan struct{}
}

func (c *streamingClient) StreamWithRequest(req *agent.Request, onText func(string)) (*agent.Response, error) {
	onText("**The answer")
	onText(" so far**")
	select {
	case <-c.release:
	case <-req.Ctx.Done():
		return nil, req.Ctx.Err()
	}
	return &agent.Response{Content: "**The answer** in full"}, nil
}

func TestAReplyShowsAsItIsWrittenThenInItsFinalForm(t *testing.T) {
	client := &streamingClient{release: make(chan struct{})}
	f, _ := boundBotWith(t, client)
	f.say(ownerChat, "private", "alice", "hello")
	waitFor(t, "the reply so far", func() bool {
		for _, m := range f.sentTo(ownerChat) {
			// The reply so far shows as it will read, never with its marks.
			if m.Method == "editMessageText" && m.ParseMode == "HTML" && strings.Contains(m.Text, "<b>The answer so far</b>") && strings.Contains(m.Text, "⏳") {
				return true
			}
		}
		return false
	})
	close(client.release)
	waitFor(t, "the final form", func() bool {
		for _, m := range f.sentTo(ownerChat) {
			if m.Method == "editMessageText" && m.ParseMode == "HTML" && strings.Contains(m.Text, "<b>The answer</b> in full") {
				return true
			}
		}
		return false
	})
}
