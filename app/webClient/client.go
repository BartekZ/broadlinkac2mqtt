package webClient

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/ArtemVladimirov/broadlinkac2mqtt/app"
	"github.com/ArtemVladimirov/broadlinkac2mqtt/app/webClient/models"
)

const udpTimeout = 10 * time.Second

type webClient struct{}

func NewWebClient() app.WebClient {
	return &webClient{}
}

func (w *webClient) SendCommand(ctx context.Context, input *models.SendCommandInput) (*models.SendCommandReturn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	dialer := net.Dialer{Timeout: udpTimeout}
	conn, err := dialer.DialContext(ctx, "udp", net.JoinHostPort(input.Ip, strconv.Itoa(int(input.Port))))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		slog.ErrorContext(ctx, "Failed to dial address", slog.Any("err", err))
		return nil, err
	}

	var closeOnce sync.Once
	closeConn := func() {
		closeOnce.Do(func() {
			if err := conn.Close(); err != nil {
				slog.ErrorContext(ctx, "Failed to close client connection", slog.Any("err", err))
			}
		})
	}
	defer closeConn()

	deadline := time.Now().Add(udpTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		slog.ErrorContext(ctx, "Failed to set deadline", slog.Any("err", err))
		return nil, err
	}

	if _, err := conn.Write(input.Payload); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		slog.ErrorContext(ctx, "Failed to write the payload", slog.Any("err", err))
		return nil, err
	}

	type readResult struct {
		n   int
		err error
	}
	response := make([]byte, 1024)
	resultCh := make(chan readResult, 1)
	go func() {
		n, err := conn.Read(response)
		resultCh <- readResult{n: n, err: err}
	}()

	select {
	case <-ctx.Done():
		closeConn()
		<-resultCh
		return nil, ctx.Err()
	case res := <-resultCh:
		if res.err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			slog.ErrorContext(ctx, "Failed to read the response", slog.Any("err", res.err))
			return nil, res.err
		}
		return &models.SendCommandReturn{Payload: response}, nil
	}
}
