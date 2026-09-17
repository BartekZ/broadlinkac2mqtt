package webClient

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/ArtemVladimirov/broadlinkac2mqtt/app/webClient/models"
)

func TestSendCommandCancelsOnContext(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()

	addr := pc.LocalAddr().(*net.UDPAddr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		_, sendErr := NewWebClient().SendCommand(ctx, &models.SendCommandInput{
			Payload: []byte("ping"),
			Ip:      addr.IP.String(),
			Port:    uint16(addr.Port),
		})
		errCh <- sendErr
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case sendErr := <-errCh:
		if !errors.Is(sendErr, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", sendErr)
		}
	case <-time.After(time.Second):
		t.Fatal("SendCommand did not return after context cancel")
	}
}
