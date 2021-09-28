package node

import (
	"context"
	"encoding/json"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	mqi "github.com/quant-nft/mq-interface"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/xyths/hs"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

type Config struct {
	Log   hs.LogConf         `json:"log"`
	Mongo hs.MongoConf       `json:"mongo"`
	Bot   hs.TelegramBotConf `json:"bot"`
	MQ    hs.QueueConf       `json:"mq"`
}

type Node struct {
	cfg Config

	Sugar *zap.SugaredLogger
	db    *mongo.Database
	tg    *tgbotapi.BotAPI
	mq    *amqp.Connection
}

func New(cfg Config) *Node {
	return &Node{cfg: cfg}
}

func (n *Node) Init(ctx context.Context) error {
	l, err := hs.NewZapLogger(n.cfg.Log)
	if err != nil {
		return err
	}
	n.Sugar = l.Sugar()
	n.Sugar.Info("logger initialized")
	db, err := hs.ConnectMongo(ctx, n.cfg.Mongo)
	if err != nil {
		n.Sugar.Errorf("connect mongo error: %s", err)
		return err
	}
	n.db = db
	n.Sugar.Info("database initialized")

	n.tg, err = tgbotapi.NewBotAPI(n.cfg.Bot.Token)
	if err != nil {
		n.Sugar.Errorf("New Telegram bot error: %n", err)
		return err
	}
	n.Sugar.Info("Telegram bot API initialized")
	n.mq, err = amqp.Dial(n.cfg.MQ.URI)
	if err != nil {
		n.Sugar.Errorf("dial to RabbitMQ error: %n", err)
		return err
	}
	n.Sugar.Info("Connected to RabbitMQ")

	n.Sugar.Info("Telegram Bot Server initialized")
	return nil
}

func (n *Node) Close(ctx context.Context) {
	if err := n.db.Client().Disconnect(ctx); err != nil {
		n.Sugar.Errorf("db close error: %s", err)
	}
	if err := n.mq.Close(); err != nil {
		n.Sugar.Errorf("close RabbitQM connection error: %s", err)
	}
	n.Sugar.Info("Telegram Bot Server closed")
}

func (n *Node) Serve(ctx context.Context) error {
	n.Sugar.Info("Telegram Bot Server started")
	defer n.Sugar.Info("Telegram Bot Server stopped")
	ch, err := n.mq.Channel()
	if err != nil {
		n.Sugar.Errorf("Failed to open a channel: %s", err)
		return err
	}
	defer ch.Close()
	_, err = ch.QueueDeclare(
		n.cfg.MQ.Name, // name
		false,         // durable
		false,         // delete when unused
		false,         // exclusive
		false,         // no-wait
		nil,           // arguments
	)
	if err != nil {
		n.Sugar.Errorf("Declare queue error: %s", err)
		return err
	}
	commands, err := ch.Consume(
		n.cfg.MQ.Name, // queue
		"",            // consumer
		true,          // auto-ack
		false,         // exclusive
		false,         // no-local
		false,         // no-wait
		nil,           // args
	)
	if err != nil {
		n.Sugar.Errorf("Consume message error: %s", err)
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg := <-commands:
			go n.doCommand(ctx, msg)
		}
	}
}

func (n *Node) doCommand(ctx context.Context, msg amqp.Delivery) {
	var command mqi.TelegramMessage
	if err := json.Unmarshal(msg.Body, &command); err != nil {
		n.Sugar.Errorf("Json unmarshal error: %s", err)
		return
	}

	tgMsg := tgbotapi.NewMessage(command.ChatId, command.Content)
	if command.ParseMode != nil {
		tgMsg.ParseMode = *command.ParseMode
	}
	if command.DisablePreview != nil {
		tgMsg.DisableWebPagePreview = *command.DisablePreview
	}

	if _, err := n.tg.Send(tgMsg); err != nil {
		n.Sugar.Errorf("Send Telegram message error: %s", err)
		return
	}
}
