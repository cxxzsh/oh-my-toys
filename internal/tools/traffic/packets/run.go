package packets

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/cxxzsh/oh-my-toys/internal/platform/pcap"
)

// Run reads src until it fails or ctx ends, and records one flow per
// packet. It returns nil when the source reaches io.EOF, and the wrapped
// context error when ctx ended first. Run closes src however it returns:
// closing is what unblocks a Read that waits, so a goroutine Run owns does
// it and Run waits for that goroutine before returning.
func Run(ctx context.Context, src pcap.Source, log Recorder) error {
	stop := make(chan struct{})
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		select {
		case <-ctx.Done():
		case <-stop:
		}
		_ = src.Close()
	}()
	err := readPackets(ctx, src, log)
	close(stop)
	<-closed
	return err
}

// readPackets records packets until the source fails; the caller closes it
// once this returns.
func readPackets(ctx context.Context, src pcap.Source, log Recorder) error {
	for {
		packet, err := src.Read()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return fmt.Errorf("read packets: %w", ctxErr)
			}
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("read packets: %w", err)
		}
		log.Put(Decode(packet))
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("read packets: %w", err)
		}
	}
}
