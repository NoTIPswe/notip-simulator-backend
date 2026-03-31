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
	clock      ports.Nower
	ackSubject string
}

func NewNATSGatewaySubscriber(js nats.JetStreamContext, tenantID, managementGatewayID string, pub ports.GatewayPublisher, clock ports.Nower) (*NATSGatewaySubscriber, error) {
	subject := fmt.Sprintf("command.gw.%s.%s", tenantID, managementGatewayID)
	ackSubject := fmt.Sprintf("command.ack.%s.%s", tenantID, managementGatewayID)
	durableName := fmt.Sprintf("gw-%s", managementGatewayID)

	subscriber := &NATSGatewaySubscriber{
		ch:         make(chan domain.IncomingCommand, 8),
		publisher:  pub,
		clock:      clock,
		ackSubject: ackSubject,
	}

	sub, err := js.Subscribe(subject, subscriber.handleMsg, nats.Durable(durableName), nats.MaxDeliver(3), nats.ManualAck())
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
		ackBytes, err := json.Marshal(ack)
		if err != nil {
			slog.Error("failed to marshal expired command ACK", "commandID", cmd.CommandID, "err", err)
		} else {
			pubCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.publisher.Publish(pubCtx, s.ackSubject, ackBytes); err != nil {
				slog.Warn("failed to publish expired command ACK", "commandID", cmd.CommandID, "err", err)
			}
		}

		if err := msg.Ack(); err != nil {
			slog.Warn("failed to ack expired command message", "commandID", cmd.CommandID, "err", err)
		}
		return
	}

	select {
	case s.ch <- cmd:
		if err := msg.Ack(); err != nil {
			slog.Warn("failed to ack command message", "commandID", cmd.CommandID, "err", err)
		}
	default:
		slog.Warn("Subscriber channel full, NAKing message to retry later", "commandID", cmd.CommandID)
		if err := msg.Nak(); err != nil {
			slog.Warn("failed to nak command message", "commandID", cmd.CommandID, "err", err)
		}
	}
}
