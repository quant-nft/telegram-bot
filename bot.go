package tgb

import (
	"context"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

const (
	maxPerGroup = 20 // 20 messages per minute to the same group
)

type Bot struct {
	token string
	coll  *mongo.Collection
	Sugar *zap.SugaredLogger
	bot   *tgbotapi.BotAPI
}

func New(token string, coll *mongo.Collection, Sugar *zap.SugaredLogger) (*Bot, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}

	return &Bot{
		token: token,
		coll:  coll,
		Sugar: Sugar,
		bot:   bot,
	}, nil
}

func (b *Bot) Serve(ctx context.Context, ch <-chan tgbotapi.Chattable) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case c := <-ch:
			go b.send(ctx, c)
		}
	}
}
func (b *Bot) send(ctx context.Context, c tgbotapi.Chattable) {
	switch c.(type) {
	case tgbotapi.MessageConfig:
		if err := b.sendMessage(ctx, c.(tgbotapi.MessageConfig)); err != nil {
			b.Sugar.Errorf("Telegram send message error: %s", err)
		}
	default:
	}
	//return nil
}

func (b *Bot) sendMessage(ctx context.Context, c tgbotapi.MessageConfig) error {
	b.Sugar.Infof("send msg %s", c.Text)
	if _, err := b.bot.Send(c); err != nil {
		return err
	}
	return nil
}
