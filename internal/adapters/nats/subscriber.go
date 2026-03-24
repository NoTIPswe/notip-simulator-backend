package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
	"github.com/NoTIPswe/notip-simulator-backend/internal/ports"
	"github.com/nats-io/nats.go"
)

type NATSGatewaySubscriber struct {
	sub        *nats.Subscription
	ch         chan domain.IncomingCommand
	publisher  ports.GatewayPublisher
	clock      ports.Clock
	ackSubject string
}

func NewNATSGatewaySubscriber(js nats.JetStreamContext, tenantID, managementGatewayID string, pub ports.GatewayPublisher, clock ports.Clock) (*NATSGatewaySubscriber, error) {
	subject := fmt.Sprintf("command.gw.%s.%s", tenantID, managementGatewayID)
	ackSubject := fmt.Sprintf("command.ack.%s.%s", tenantID, managementGatewayID)
	durableName := fmt.Sprintf("gw-%s", managementGatewayID)

	subscriber := &NATSGatewaySubscriber{
		ch:         make(chan domain.IncomingCommand, 8),
		publisher:  pub,
		clock:      clock,
		ackSubject: ackSubject,
	}

	sub, err := js.Subscribe(subject, subscriber.handleMsg, nats.Durable(durableName), nats.MaxDeliver(3))
	if err != nil {
		return nil, err
	}

	subscriber.sub = sub
	return subscriber, nil
}

func (s *NATSGatewaySubscriber) Messages() <-chan domain.IncomingCommand {
	return s.ch
}

func (s *NATSGatewaySubscriber) Close() error {
	err := s.sub.Drain()
	close(s.ch)
	return err
}

func (s *NATSGatewaySubscriber) handleMsg(msg *nats.Msg) {
	var cmd domain.IncomingCommand

	if err := json.Unmarshal(msg.Data, &cmd); err != nil {
		slog.Error("Failed to unmarshal incoming command", "err", err)
		_ = msg.Term()
		return
	}

	if s.clock.Now().Sub(cmd.IssuedAt) > 60*time.Second {
		slog.Warn("Received expired command, discarding", "commandID", cmd.CommandID)

		ack := domain.CommandACK{
			CommandID: cmd.CommandID,
			Status:    domain.Expired,
			Timestamp: s.clock.Now(),
		}
		ackBytes, _ := json.Marshal(ack)
		_ = s.publisher.Publish(context.Background(), s.ackSubject, ackBytes)

		_ = msg.Ack()
		return
	}

	select {
	case s.ch <- cmd:
		_ = msg.Ack()
	default:
		slog.Warn("Subscriber channel full, NAKing message to retry later", "commandID", cmd.CommandID)
		_ = msg.Nak()
	}
}
