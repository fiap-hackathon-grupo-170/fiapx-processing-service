package rabbitmq

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type MessageHandler func(ctx context.Context, body []byte) error

type Consumer struct {
	conn        *amqp.Connection
	channel     *amqp.Channel
	queue       string
	exchange    string
	workerCount int
	baseDelay   time.Duration
	handler     MessageHandler
	logger      *zap.Logger
	wg          sync.WaitGroup
}

type ConsumerConfig struct {
	URL         string
	Queue       string
	Exchange    string
	DLQ         string
	StatusQueue string
	Prefetch    int
	WorkerCount int
	BaseDelayMs int
}

func NewConsumer(cfg ConsumerConfig, handler MessageHandler, logger *zap.Logger) (*Consumer, error) {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("dial rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("open channel: %w", err)
	}

	// Declare exchange
	err = ch.ExchangeDeclare(cfg.Exchange, "topic", true, false, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("declare exchange: %w", err)
	}

	// Declare queues
	for _, q := range []string{cfg.Queue, cfg.DLQ, cfg.StatusQueue} {
		_, err = ch.QueueDeclare(q, true, false, false, false, nil)
		if err != nil {
			return nil, fmt.Errorf("declare queue %s: %w", q, err)
		}
	}

	// Bind queues to exchange
	err = ch.QueueBind(cfg.Queue, "video.processing", cfg.Exchange, false, nil)
	if err != nil {
		return nil, fmt.Errorf("bind processing queue: %w", err)
	}

	err = ch.QueueBind(cfg.StatusQueue, "video.status", cfg.Exchange, false, nil)
	if err != nil {
		return nil, fmt.Errorf("bind status queue: %w", err)
	}

	// Set prefetch
	err = ch.Qos(cfg.Prefetch, 0, false)
	if err != nil {
		return nil, fmt.Errorf("set qos: %w", err)
	}

	return &Consumer{
		conn:        conn,
		channel:     ch,
		queue:       cfg.Queue,
		exchange:    cfg.Exchange,
		workerCount: cfg.WorkerCount,
		baseDelay:   time.Duration(cfg.BaseDelayMs) * time.Millisecond,
		handler:     handler,
		logger:      logger,
	}, nil
}

func (c *Consumer) Start(ctx context.Context) error {
	deliveries, err := c.channel.ConsumeWithContext(
		ctx,
		c.queue,
		"",
		false, // autoAck=false
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}

	c.logger.Info("starting worker pool",
		zap.Int("workers", c.workerCount),
		zap.String("queue", c.queue),
	)

	for i := 0; i < c.workerCount; i++ {
		c.wg.Add(1)
		go c.worker(ctx, i, deliveries)
	}

	<-ctx.Done()
	c.logger.Info("context cancelled, waiting for workers to finish")
	c.wg.Wait()
	return nil
}

func (c *Consumer) worker(ctx context.Context, id int, deliveries <-chan amqp.Delivery) {
	defer c.wg.Done()
	log := c.logger.With(zap.Int("worker_id", id))
	log.Info("worker started")

	for {
		select {
		case <-ctx.Done():
			log.Info("worker shutting down")
			return
		case d, ok := <-deliveries:
			if !ok {
				log.Info("delivery channel closed")
				return
			}
			c.processDelivery(ctx, d, log)
		}
	}
}

func (c *Consumer) processDelivery(ctx context.Context, d amqp.Delivery, log *zap.Logger) {
	err := c.handler(ctx, d.Body)
	if err != nil {
		log.Warn("message processing failed, nacking",
			zap.Error(err),
			zap.Uint64("delivery_tag", d.DeliveryTag),
		)

		attempt := c.getAttemptFromHeaders(d)
		delay := c.calculateBackoff(attempt)
		log.Info("backoff before requeue", zap.Duration("delay", delay), zap.Int("attempt", attempt))

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			_ = d.Nack(false, false)
			return
		}

		_ = d.Nack(false, true) // requeue=true
		return
	}

	_ = d.Ack(false)
}

func (c *Consumer) getAttemptFromHeaders(d amqp.Delivery) int {
	if d.Headers == nil {
		return 1
	}
	if xDeath, ok := d.Headers["x-death"]; ok {
		if deaths, ok := xDeath.([]interface{}); ok && len(deaths) > 0 {
			return len(deaths)
		}
	}
	return 1
}

func (c *Consumer) calculateBackoff(attempt int) time.Duration {
	delay := c.baseDelay * time.Duration(math.Pow(2, float64(attempt-1)))
	if delay > 60*time.Second {
		delay = 60 * time.Second
	}
	return delay
}

func (c *Consumer) Close() error {
	if c.channel != nil {
		c.channel.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
