package node

import (
	"context"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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
	q, err := ch.QueueDeclare(
		"hello", // name
		false,   // durable
		false,   // delete when unused
		false,   // exclusive
		false,   // no-wait
		nil,     // arguments
	)
	if err != nil {
		n.Sugar.Errorf("Declare queue error: %s", err)
		return err
	}
	commands, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
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
	n.Sugar.Infof("received command: %s", msg.Body)
}
