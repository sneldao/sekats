package adapters

import (
	"testing"

	"github.com/sneldao/sekats/internal/adapters/poker"
)

func TestRegisterAndGet(t *testing.T) {
	Register(poker.New())
	got, err := Get("poker")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name() != "poker" || got.Agent() != "DevFun Arena poker agent" {
		t.Fatalf("poker adapter: %+v", got)
	}
}

func TestGetUnknown(t *testing.T) {
	Register(poker.New())
	_, err := Get("unknown-agent")
	if err == nil {
		t.Fatal("expected error for unknown adapter")
	}
	e, ok := err.(*UnknownAdapterError)
	if !ok {
		t.Fatalf("wrong error type: %T", err)
	}
	if e.Name != "unknown-agent" {
		t.Fatalf("name = %q", e.Name)
	}
	if len(e.Known) == 0 || e.Known[0] != "poker" {
		t.Fatalf("known = %v", e.Known)
	}
}

func TestNames(t *testing.T) {
	Register(poker.New())
	names := Names()
	if len(names) == 0 || names[0] != "poker" {
		t.Fatalf("names = %v", names)
	}
}
