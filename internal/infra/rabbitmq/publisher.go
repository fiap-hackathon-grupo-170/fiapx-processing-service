package rabbitmq

import (
	"context"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Publisher struct {
	channel  *amqp.Channel
	exchange string
}

func NewPublisher(conn *amqp.Connection, exchange string) (*Publisher, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("open publisher channel: %w", err)
	}
	return &Publisher{channel: ch, exchange: exchange}, nil
}

type StatusPublisher struct {
	pub        *Publisher
	routingKey string
}

func NewStatusPublisher(pub *Publisher) *StatusPublisher {
	return &StatusPublisher{pub: pub, routingKey: "video.status"}
}

func (sp *StatusPublisher) PublishStatus(ctx context.Context, msg []byte) error {
	return sp.pub.channel.PublishWithContext(ctx,
		sp.pub.exchange,
		sp.routingKey,
		false, false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         msg,
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now().UTC(),
		},
	)
}

type DLQPublisher struct {
	pub   *Publisher
	queue string
}

func NewDLQPublisher(pub *Publisher, dlqQueue string) *DLQPublisher {
	return &DLQPublisher{pub: pub, queue: dlqQueue}
}

func (dp *DLQPublisher) PublishToDLQ(ctx context.Context, msg []byte, reason string) error {
	return dp.pub.channel.PublishWithContext(ctx,
		"",
		dp.queue,
		false, false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         msg,
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now().UTC(),
			Headers: amqp.Table{
				"x-dlq-reason": reason,
			},
		},
	)
}
