package app

import (
	"context"

	"github.com/NoTIPswe/notip-simulator-backend/internal/metrics"
	"github.com/NoTIPswe/notip-simulator-backend/internal/ports"
)

type MessageBuffer struct {
	ch        chan []byte
	subject   string
	publisher ports.GatewayPublisher
	metrics   *metrics.Metrics
	gatewayID string
}

func NewMessageBuffer(capacity int, subject string, gatewayID string, pub ports.GatewayPublisher, met *metrics.Metrics) *MessageBuffer {
	return &MessageBuffer{
		ch:        make(chan []byte, capacity),
		subject:   subject,
		publisher: pub,
		metrics:   met,
		gatewayID: gatewayID,
	}
}

func (b *MessageBuffer) Send(payload []byte) {
	select {
	case b.ch <- payload:
		// Sent to channel
	default:
		// Buffer is full, drop the message and record the drop.
		select {
		case <-b.ch:
			b.metrics.BufferDropped.WithLabelValues(b.gatewayID).Inc()
		default:
		}

		//Insert the new message.
		select {
		case b.ch <- payload:
		default:
			// If the buffer is still full, we can't drop any more messages.
		}
	}
	b.metrics.BufferFill.WithLabelValues(b.gatewayID).Set(float64(len(b.ch)))
}

func (b *MessageBuffer) Flush(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-b.ch:
			if err := b.publisher.Publish(ctx, b.subject, msg); err != nil {
				b.metrics.PublishErrors.WithLabelValues(b.gatewayID).Inc()
			} else {
				b.metrics.EnvelopesPublished.WithLabelValues(b.gatewayID).Inc()
			}
			b.metrics.BufferFill.WithLabelValues(b.gatewayID).Set(float64(len(b.ch)))
		}
	}
}
