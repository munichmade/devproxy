//go:build !darwin

package service

import (
	"testing"
)

func TestActivatedListener_NonDarwin(t *testing.T) {
	// On non-Darwin platforms, ActivatedListener should always return nil
	listener, err := ActivatedListener("HTTPListener")
	if err != nil {
		t.Errorf("ActivatedListener should not return error on non-Darwin: %v", err)
	}
	if listener != nil {
		listener.Close()
		t.Error("ActivatedListener should return nil on non-Darwin platforms")
	}
}

func TestIsSocketActivated_NonDarwin(t *testing.T) {
	if IsSocketActivated() {
		t.Error("IsSocketActivated should always return false on non-Darwin platforms")
	}
}
