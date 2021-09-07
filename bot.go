package tgb

import (
	"context"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
	"time"
)

const (
	maxPerGroup = 20 // 20 messages per minute to the same group
	indexName   = "expireByTime"
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

func (b *Bot) Init(ctx context.Context) error {
	return b.initIndex(ctx)
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

func (b *Bot) initIndex(ctx context.Context) error {
	// list index first
	indexView := b.coll.Indexes()
	cursor, err := indexView.List(ctx, options.ListIndexes().SetMaxTime(time.Second*2))
	if err != nil {
		return err
	}
	var indexes []bson.M
	if err = cursor.All(ctx, &indexes); err != nil {
		return err
	}
	b.Sugar.Debugf("all indexes in collection: %v", indexes)
	for _, index := range indexes {
		if index["name"] == indexName {
			b.Sugar.Infof("index %s already exist", indexName)
			return nil
		}
	}
	index := mongo.IndexModel{
		Keys:    bson.D{{"createdAt", 1}},
		Options: options.Index().SetExpireAfterSeconds(2 * 60).SetName(indexName),
	}
	name, err := indexView.CreateOne(ctx, index)
	if err != nil {
		return err
	}
	b.Sugar.Infof("create index: %s", name)
	return nil
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
	chatId := c.ChatID
	count, err := b.coll.CountDocuments(ctx, bson.D{
		{"chatId", chatId},
	})

	if err == nil && count < maxPerGroup {
		b.Sugar.Infof("sent %d messages to chat %d", count, chatId)
		_, err := b.bot.Send(c)
		return err
	}
	retry := 0
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
			count, err := b.coll.CountDocuments(ctx, bson.D{
				{"chatId", chatId},
			})

			if err == nil && count < maxPerGroup {
				b.Sugar.Infof("sent %d messages to chat %d", count, chatId)
				_, err := b.bot.Send(c)
				return err
			}
			retry++
			if retry >= 10 {
				return nil // return anyway
			}
		}
	}
}
