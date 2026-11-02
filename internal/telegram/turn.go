package telegram

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"vergex/internal/logger"
	"vergex/internal/service"
	telegramagent "vergex/internal/telegram/agent"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	// turnLimit is how long one turn may take, across all its model and skill calls.
	turnLimit = 3 * time.Minute
	// turnQueue is how many messages can wait behind the one being answered.
	turnQueue = 16
	// statusEvery is how often the placeholder is looked at, and the least time between edits.
	statusEvery = time.Second
	// elapsedStep is how coarsely it counts: a change of step shows at once, the time only
	// every so often, since each edit is a request to Telegram.
	elapsedStep = 5 * time.Second
	typingEvery = 5 * time.Second
	// maxRetryAfter bounds how long a message waits on Telegram's rate limit.
	maxRetryAfter = 30 * time.Second
	// maxPartialRunes is how much of a reply still being written the placeholder shows.
	maxPartialRunes = 3000
)

// buttonStop is the data of the button that stops a turn.
const buttonStop = "stop"

// work runs what the chat asked for, one at a time and in the order it came.
func (r *runner) work() {
	for {
		select {
		case <-r.stopped:
			return
		case job := <-r.jobs:
			done := make(chan struct{})
			// A panic in a skill must end this turn only, not the backend and its trading.
			service.Go("telegram-turn", func() {
				defer close(done)
				job()
			}, func(any) {
				sendMsg(r.bot, r.owner, pick(r.st.Telegram.GetLanguage(), "❌ Something went wrong handling that. Nothing further will happen", "❌ 处理这条消息时出错了，不会再继续"))
			})
			<-done
		}
	}
}

// enqueue adds to what the chat is waiting for.
func (r *runner) enqueue(job func()) {
	select {
	case r.jobs <- job:
	default:
		sendMsg(r.bot, r.owner, pick(r.st.Telegram.GetLanguage(), "⏳ Too much is waiting already. Send /stop or try again in a moment", "⏳ 等待处理的消息太多了。可以发 /stop，或稍后再试"))
	}
}

// runTurn shows a placeholder for a piece of work, keeps it saying what is under way, and
// replaces it with what the work came to.
func (r *runner) runTurn(run func(ctx context.Context, progress *telegramagent.Progress) telegramagent.Reply) {
	ctx, cancel := context.WithTimeout(context.Background(), turnLimit)
	defer cancel()
	r.turnMu.Lock()
	r.cancelTurn = cancel
	r.turnMu.Unlock()
	defer func() {
		r.turnMu.Lock()
		r.cancelTurn = nil
		r.turnMu.Unlock()
	}()

	status := r.startStatus()
	reply := run(ctx, &telegramagent.Progress{Stage: status.set, Text: status.addText})
	messageID := status.finish()
	// Whatever the buttons were for has been answered by now.
	r.closeConfirming()
	deliver(r.bot, r.owner, messageID, reply.Text)
	r.askToConfirm(reply.Confirm)
}

// stopTurn stops what is under way and drops what waits behind it. It reports whether
// there was anything to stop.
func (r *runner) stopTurn() bool {
	stopped := false
	for {
		select {
		case <-r.jobs:
			stopped = true
			continue
		default:
		}
		break
	}
	r.turnMu.Lock()
	defer r.turnMu.Unlock()
	if r.cancelTurn != nil {
		r.cancelTurn()
		return true
	}
	return stopped
}

// turnStatus keeps a turn's placeholder current. Updates coalesce: only the latest step
// goes out, never less than a second apart, and never when nothing changed.
type turnStatus struct {
	r         *runner
	messageID int
	lang      string
	started   time.Time

	mu    sync.Mutex
	stage string
	// partial is the reply so far, for as long as the model is writing it.
	partial strings.Builder

	stop chan struct{}
	done chan struct{}
}

func (r *runner) startStatus() *turnStatus {
	s := &turnStatus{r: r, lang: r.st.Telegram.GetLanguage(), started: time.Now(), stop: make(chan struct{}), done: make(chan struct{})}
	msg := tgbotapi.NewMessage(r.owner, "⏳")
	msg.ReplyMarkup = s.buttons()
	if sent, err := r.bot.Send(msg); err == nil {
		s.messageID = sent.MessageID
	} else {
		logger.Warnf("Telegram: send the placeholder: %v", err)
	}
	service.Go("telegram-status", s.keep, func(any) { closeOnce(s.done) })
	return s
}

func closeOnce(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}

func (s *turnStatus) buttons() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🛑 "+pick(s.lang, "Stop", "停止"), buttonStop),
	))
}

// set says what the turn is doing now. What the model wrote before moving on to a skill is
// not its answer, so it goes.
func (s *turnStatus) set(stage string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stage = stage
	s.partial.Reset()
}

// addText adds to the reply as it is written.
func (s *turnStatus) addText(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.partial.WriteString(text)
}

func (s *turnStatus) text() string {
	s.mu.Lock()
	stage, partial := s.stage, s.partial.String()
	s.mu.Unlock()
	if partial != "" {
		// The end of it is what is being written, and all of it is coming once it is done.
		if runes := []rune(partial); len(runes) > maxPartialRunes {
			partial = "…" + string(runes[len(runes)-maxPartialRunes:])
		}
		return partial + "\n\n⏳"
	}
	if stage == "" {
		return "⏳"
	}
	elapsed := time.Since(s.started)
	if elapsed < elapsedStep {
		return "⏳ " + stage
	}
	return "⏳ " + stage + " · " + strconv.Itoa(int(elapsed/elapsedStep)*int(elapsedStep/time.Second)) + "s"
}

// keep is the placeholder's one writer, so edits never overtake one another.
func (s *turnStatus) keep() {
	defer closeOnce(s.done)
	if s.messageID == 0 {
		<-s.stop
		return
	}
	ticker := time.NewTicker(statusEvery)
	defer ticker.Stop()
	shown, nextEdit, nextTyping := "⏳", time.Time{}, time.Time{}
	for {
		select {
		case <-s.stop:
			return
		case now := <-ticker.C:
			if !now.Before(nextTyping) {
				nextTyping = now.Add(typingEvery)
				s.r.bot.Request(tgbotapi.NewChatAction(s.r.owner, tgbotapi.ChatTyping)) //nolint:errcheck
			}
			text := s.text()
			if text == shown || now.Before(nextEdit) {
				continue
			}
			edit := tgbotapi.NewEditMessageTextAndMarkup(s.r.owner, s.messageID, text, s.buttons())
			if _, err := s.r.bot.Send(edit); err != nil {
				nextEdit = now.Add(retryAfter(err))
				continue
			}
			shown = text
		}
	}
}

// finish stops the updates and returns the placeholder for the reply to replace.
func (s *turnStatus) finish() int {
	close(s.stop)
	<-s.done
	return s.messageID
}

// retryAfter says how long Telegram asked to wait after a refusal, none if it did not ask.
func retryAfter(err error) time.Duration {
	var refused *tgbotapi.Error
	if errors.As(err, &refused) && refused.RetryAfter > 0 {
		return min(time.Duration(refused.RetryAfter)*time.Second, maxRetryAfter)
	}
	return 0
}

// requestPatiently asks for something with no message to show for it, waiting once if Telegram
// says it is going too fast.
func requestPatiently(bot *tgbotapi.BotAPI, c tgbotapi.Chattable) error {
	_, err := bot.Request(c)
	if wait := retryAfter(err); wait > 0 {
		time.Sleep(wait)
		_, err = bot.Request(c)
	}
	return err
}

// sendPatiently sends something, waiting once if Telegram says it is going too fast.
func sendPatiently(bot *tgbotapi.BotAPI, c tgbotapi.Chattable) error {
	_, err := bot.Send(c)
	if wait := retryAfter(err); wait > 0 {
		time.Sleep(wait)
		_, err = bot.Send(c)
	}
	return err
}
