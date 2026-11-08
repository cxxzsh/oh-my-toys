//go:build !darwin && !windows && !linux

package pcap

import "context"

// Every other system: no capture here, so the UI offers the guidance
// instead of a way to capture.

func list() ([]Interface, error) { return nil, ErrUnsupported }

func defaultInterface() (string, error) { return "", ErrUnsupported }

func open(string) (Source, error) { return nil, ErrUnsupported }

func grant(context.Context, string) error { return ErrUnsupported }
