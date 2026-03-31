package nats

import (
	"context"
	"log/slog"
	"strings"

	"github.com/NoTIPswe/notip-simulator-backend/internal/ports"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

type NATSDecommissionListener struct {
	js       nats.JetStreamContext
	receiver ports.DecommissionEventReceiver
}

func NewNATSDecommissionListener(js nats.JetStreamContext, receiver ports.DecommissionEventReceiver) *NATSDecommissionListener {
	return &NATSDecommissionListener{
		js:       js,
		receiver: receiver,
	}
}

func (l *NATSDecommissionListener) Run(ctx context.Context) error {
	sub, err := l.js.Subscribe("gateway.decommissioned.>", func(msg *nats.Msg) {
		l.dispatchFromSubject(msg.Subject)
		_ = msg.Ack()
	},
		nats.Durable("simulator-decommission"),
		nats.ManualAck(),
		nats.DeliverNew(), // Only receive decommission events that occur after this listener starts.
	)
	if err != nil {
		return err
	}

	<-ctx.Done()
	return sub.Drain()
}

func (l *NATSDecommissionListener) dispatchFromSubject(subject string) {
	parts := strings.Split(subject, ".")
	if len(parts) != 4 {
		slog.Warn("invalid decommission subject", "subject", subject)
		return
	}
	tenantID := parts[2]
	gatewayID := parts[3]
	if _, err := uuid.Parse(gatewayID); err != nil {
		slog.Warn("invalid gatewayID", "gatewayID", gatewayID)
		return
	}
	if _, err := uuid.Parse(tenantID); err != nil {
		slog.Warn("invalid tenantID", "tenantID", tenantID)
		return
	}
	l.receiver.HandleDecommission(tenantID, gatewayID)
}
